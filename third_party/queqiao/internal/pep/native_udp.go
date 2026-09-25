package pep

import (
	"context"
	"errors"
	"github.com/sagernet/sing-box/third_party/queqiao/internal/protocol"
	"github.com/sagernet/sing-box/third_party/queqiao/internal/session"
	"github.com/sagernet/sing-box/third_party/queqiao/internal/udperr"
	"net"
	"time"
)

// ServePacket exposes UDP associations without a SOCKS control connection.
func (c *Client) ServePacket(ctx context.Context, endpoint net.PacketConn, ready func(error)) {
	defer endpoint.Close()
	slot, admitted := c.sessionLimit.acquire()
	if !admitted {
		ready(errors.New("session limit reached"))
		return
	}
	defer slot.release()
	c.runUDPAssociation(ctx, endpoint, ctx.Done(), func(err error) error { ready(err); return nil },
		func(ctx context.Context, lane *authenticatedLane, id uint64, activity chan<- struct{}, counters *udpCounters) error {
			return c.runNativeUDPUplink(ctx, endpoint, lane.fc, lane.sessionID, id, activity, counters)
		},
		func(ctx context.Context, lane *authenticatedLane, id uint64, activity chan<- struct{}, counters *udpCounters) error {
			return runNativeUDPDownlink(ctx, endpoint, lane.fc, lane.sessionID, id, activity, counters)
		})
}

func (c *Client) runNativeUDPUplink(ctx context.Context, udpConn net.PacketConn, fc *frameConn, sessionID [16]byte, flowID uint64, activity chan<- struct{}, counters *udpCounters) error {
	buf := make([]byte, c.memoryLimits.maxUDPPacketBytes)
	var sequence uint64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_ = udpConn.SetReadDeadline(time.Now().Add(udpReadPoll))
		n, addr, err := udpConn.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// One datagram's problem is not the association's. A send to a
			// peer that has gone away draws an ICMP port-unreachable, which
			// the host reports on a later read -- on Windows even for an
			// unconnected socket. Returning here would end a live SOCKS5 UDP
			// association because one destination stopped listening.
			if udperr.Transient(err) {
				continue
			}
			return err
		}
		payload, err := session.EncodeUDPPacket(addr.String(), buf[:n])
		if err != nil {
			continue
		}
		if err := fc.WriteContext(ctx, protocol.Frame{Header: protocol.Header{
			Version: protocol.Version, Type: protocol.TypePacket, SessionID: sessionID,
			FlowID: flowID, Sequence: sequence, Class: protocol.ClassInteractive,
		}, Payload: payload}); err != nil {
			return err
		}
		// packet count/byte counters are deliberately payload-only.
		sequence++
		counters.up.Add(uint64(n))
		notifyActivity(activity)
	}
}

func runNativeUDPDownlink(ctx context.Context, udpConn net.PacketConn, fc *frameConn, sessionID [16]byte, flowID uint64, activity chan<- struct{}, counters *udpCounters) error {
	frames, errs := udpFrames(ctx, fc, flowID)
	defer fc.releaseBulk(flowID)
	var window packetWindow
	for {
		var frame protocol.Frame
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errs:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		case frame = <-frames:
		}
		if frame.Header.SessionID != sessionID || frame.Header.FlowID != flowID {
			return errors.New("invalid UDP association frame")
		}
		if frame.Header.Type == protocol.TypeAck && frame.Header.Flags == protocol.FlagAckFinal && frame.Header.Sequence == 0 && len(frame.Payload) == 0 {
			return errUDPAssociationCloseAck
		}
		if frame.Header.Type != protocol.TypePacket || frame.Header.Flags != 0 {
			return errors.New("invalid UDP association frame")
		}
		if !window.admit(frame.Header.Sequence) {
			// Already delivered, or so far behind that it cannot be told
			// apart from one that was. Either way it is dropped rather than
			// fatal: a duplicate is not a peer misbehaving, it is a datagram
			// substrate doing what one does.
			continue
		}
		destination, payload, err := session.DecodeUDPPacket(frame.Payload)
		if err != nil {
			return err
		}
		_ = udpConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := udpConn.WriteTo(payload, PacketAddress(destination)); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		notifyActivity(activity)
		counters.down.Add(uint64(len(payload)))
	}
}
