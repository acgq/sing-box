package pep

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/sagernet/sing-box/third_party/queqiao/internal/portmux"
	"github.com/sagernet/sing-box/third_party/queqiao/internal/session"
)

// OuterDialer lets the embedding application own DNS and socket routing.
type OuterDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
	ListenPacket(context.Context, string) (net.PacketConn, *net.UDPAddr, error)
}

// PacketAddress preserves a domain destination until the host router resolves it.
type PacketAddress string

type peerAddressKey struct{}

func PeerAddressFromContext(ctx context.Context) net.Addr {
	address, _ := ctx.Value(peerAddressKey{}).(net.Addr)
	return address
}
func withPeerAddress(ctx context.Context, conn streamConn) context.Context {
	var address net.Addr
	switch c := conn.(type) {
	case *quicStreamConn:
		address = c.conn.RemoteAddr()
	case interface{ RemoteAddr() net.Addr }:
		address = c.RemoteAddr()
	}
	if address != nil {
		return context.WithValue(ctx, peerAddressKey{}, address)
	}
	return ctx
}

func (a PacketAddress) Network() string { return "udp" }
func (a PacketAddress) String() string  { return string(a) }
func validatedPacketAddress(a net.Addr) (string, error) {
	if a == nil {
		return "", errors.New("missing packet address")
	}
	_, err := session.EncodeDestination(a.String())
	return a.String(), err
}

// ServeStream consumes an application stream without a SOCKS listener or handshake.
// ready reports admission/open errors exactly once before payload processing starts.
func (c *Client) ServeStream(ctx context.Context, inner net.Conn, destination string, ready func(error)) {
	defer inner.Close()
	slot, admitted := c.sessionLimit.acquire()
	if !admitted {
		ready(errors.New("session limit reached"))
		return
	}
	defer slot.release()
	if !c.admitPendingOpen() {
		ready(errors.New("pending-open limit reached"))
		return
	}
	flow, err := c.openFlowWithRetries(ctx, destination)
	c.releasePendingOpen()
	if err != nil {
		ready(err)
		return
	}
	ready(nil)
	c.runStream(ctx, inner, flow)
}

func (c *Client) Reset(ctx context.Context)              { c.onUplinkChanged(ctx) }
func (c *Client) Close() error                           { c.closeQUICPool(); return nil }
func (c *Client) Warmup(ctx context.Context)             { c.prewarmPath(ctx) }
func (s *Server) WatchAuthorization(ctx context.Context) { s.watchAuthorizationStore(ctx) }
func (s *Server) CloseRetainedPackets()                  { s.udpRelays.closeAll() }

// ServePacketConnWithHops uses host-bound UDP sockets for every native
// Queqiao hop port. The host chooses socket options and network namespace.
func (s *Server) ServePacketConnWithHops(ctx context.Context, primary net.PacketConn, bind func(int) (net.PacketConn, error)) error {
	if s.cfg.HopPortCount < 2 {
		return s.ServePacketConn(ctx, primary)
	}
	address, ok := primary.LocalAddr().(*net.UDPAddr)
	if !ok || address.Port == 0 {
		_ = primary.Close()
		return errors.New("Queqiao hop listener requires a bound UDP address")
	}
	ports := portmux.HopPorts(s.cfg.Credentials.ProviderID, address.Port, s.cfg.HopPortCount)
	mux, err := portmux.NewServerPortMux(primary, ports, bind)
	if err != nil {
		_ = primary.Close()
		return fmt.Errorf("create Queqiao hop listener: %w", err)
	}
	s.cfg.Logger.Info("Queqiao port hop listener ready", "primary_addr", address,
		"hop_ports_configured", len(ports), "hop_ports_unavailable", mux.SkippedPorts())
	return s.ServePacketConn(ctx, mux)
}
