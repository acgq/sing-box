package queqiao

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	mDNS "github.com/miekg/dns"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"golang.org/x/net/http2"
)

type echoRouter struct{ adapter.Router }

func (*echoRouter) RouteConnectionEx(_ context.Context, conn net.Conn, _ adapter.InboundContext, _ N.CloseHandlerFunc) {
	go func() { defer conn.Close(); _, _ = io.Copy(conn, conn) }()
}

func TestPacketAssociationSurvivesListenContextCancellation(t *testing.T) {
	out := newLifecycleOutbound(t, "quic", &recordingRouter{seen: make(chan adapter.InboundContext, 8)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := out.ListenPacket(ctx, M.Socksaddr{})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	cancel()
	time.Sleep(30 * time.Millisecond)
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = conn.WriteTo([]byte("dns"), M.ParseSocksaddr("dns.example:53")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 16)
	n, address, err := conn.ReadFrom(reply)
	if err != nil || string(reply[:n]) != "dns" || address.String() != "dns.example:53" {
		t.Fatalf("UDP response: %q %v %v", reply[:n], address, err)
	}
	if err = out.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err = conn.ReadFrom(reply); err == nil {
		t.Fatal("outbound shutdown left the association open")
	}
}

func TestCanceledDialStillFails(t *testing.T) {
	out := newLifecycleOutbound(t, "tcp", &echoRouter{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn, err := out.DialContext(ctx, "tcp", M.ParseSocksaddr("echo.invalid:443"))
	if conn != nil {
		conn.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled dial: %v", err)
	}
	expired, expire := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer expire()
	conn, err = out.DialContext(expired, "tcp", M.ParseSocksaddr("echo.invalid:443"))
	if conn != nil {
		conn.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired dial: %v", err)
	}
}

type forwardingTestRouter struct{ adapter.Router }

func (*forwardingTestRouter) RouteConnectionEx(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, _ N.CloseHandlerFunc) {
	go func() {
		defer conn.Close()
		remote, err := (&net.Dialer{}).DialContext(ctx, "tcp", metadata.Destination.String())
		if err != nil {
			return
		}
		defer remote.Close()
		go func() { _, _ = io.Copy(remote, conn); _ = remote.(*net.TCPConn).CloseWrite() }()
		_, _ = io.Copy(conn, remote)
	}()
}

func newLifecycleOutbound(t *testing.T, transport string, router adapter.Router, lowMemory ...bool) *Outbound {
	return newLifecycleOutboundDelayed(t, transport, router, 0, lowMemory...)
}

func newLifecycleOutboundDelayed(t *testing.T, transport string, router adapter.Router, delay time.Duration, lowMemory ...bool) *Outbound {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	var endpoint string
	var port uint16
	if transport == "quic" {
		udp, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		endpoint = udp.LocalAddr().String()
		port = uint16(udp.LocalAddr().(*net.UDPAddr).Port)
		udp.Close()
	} else {
		tcp, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		endpoint = tcp.Addr().String()
		port = uint16(tcp.Addr().(*net.TCPAddr).Port)
		tcp.Close()
	}
	if delay > 0 && transport == "quic" {
		endpoint = delayedUDPProxy(t, endpoint, delay)
	}
	provider, profile := testProfile(t, endpoint)
	logger := log.NewNOPFactory().NewLogger("test")
	in, err := NewInbound(ctx, router, logger, "q-in", option.QueqiaoInboundOptions{
		ProviderPath: provider, Transport: transport, ListenOptions: option.ListenOptions{ListenPort: port},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = in.(adapter.Lifecycle).Start(adapter.StartStateStart); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close() })
	options := option.QueqiaoOutboundOptions{ProfilePath: profile, Transport: transport}
	if len(lowMemory) > 0 {
		options.LowMemory = &lowMemory[0]
	}
	out, err := NewOutbound(ctx, nil, logger, "q-out", options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = out.(*Outbound).Close() })
	return out.(*Outbound)
}

// HTTP/2 pools the connection dialed for the first DNS query. Finishing that
// query must not close the connection while another query is using it.
func TestDoHQueryCancellationDoesNotCloseSharedConnection(t *testing.T) {
	for _, transport := range []string{"tcp", "quic"} {
		t.Run(transport, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{}, 1)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				payload, err := io.ReadAll(r.Body)
				if err != nil {
					return
				}
				var request mDNS.Msg
				if request.Unpack(payload) != nil || len(request.Question) != 1 {
					w.WriteHeader(400)
					return
				}
				if request.Question[0].Name == "slow.example." {
					close(started)
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				response := new(mDNS.Msg).SetReply(&request)
				response.Answer = []mDNS.RR{&mDNS.A{Hdr: mDNS.RR_Header{Name: request.Question[0].Name, Rrtype: mDNS.TypeA, Class: mDNS.ClassINET, Ttl: 60}, A: net.IPv4(192, 0, 2, 1)}}
				answer, _ := response.Pack()
				w.Header().Set("Content-Type", "application/dns-message")
				_, _ = w.Write(answer)
			}))
			server.EnableHTTP2 = true
			server.StartTLS()
			defer server.Close()
			defer close(release)
			out := newLifecycleOutbound(t, transport, &forwardingTestRouter{})
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			var dials atomic.Int32
			h2 := &http2.Transport{DialTLSContext: func(ctx context.Context, _, address string, _ *tls.Config) (net.Conn, error) {
				dials.Add(1)
				conn, err := out.DialContext(ctx, "tcp", M.ParseSocksaddr(address))
				if err != nil {
					return nil, err
				}
				secure := tls.Client(conn, &tls.Config{RootCAs: roots, ServerName: "example.com", NextProtos: []string{"h2"}})
				if err = secure.HandshakeContext(ctx); err != nil {
					conn.Close()
					return nil, err
				}
				return secure, nil
			}}
			defer h2.CloseIdleConnections()
			query := func(ctx context.Context, name string) error {
				wire, _ := new(mDNS.Msg).SetQuestion(name, mDNS.TypeA).Pack()
				request, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/dns-query", bytes.NewReader(wire))
				request.Header.Set("Content-Type", "application/dns-message")
				response, err := h2.RoundTrip(request)
				if err != nil {
					return err
				}
				defer response.Body.Close()
				answer, err := io.ReadAll(response.Body)
				if err != nil {
					return err
				}
				var message mDNS.Msg
				return message.Unpack(answer)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			first, cancelFirst := context.WithCancel(ctx)
			defer cancelFirst()
			if err := query(first, "first.example."); err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { result <- query(ctx, "slow.example.") }()
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			cancelFirst()
			time.Sleep(50 * time.Millisecond)
			release <- struct{}{}
			select {
			case err := <-result:
				if err != nil {
					t.Fatalf("in-flight DoH query lost its shared connection: %v", err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if err := query(ctx, "third.example."); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 16; i++ {
				queryCtx, cancelQuery := context.WithCancel(ctx)
				err := query(queryCtx, "reused.example.")
				cancelQuery()
				if err != nil {
					t.Fatalf("reused DNS query %d: %v", i, err)
				}
			}
			if dials.Load() != 1 {
				t.Fatalf("DoH did not reuse its connection: %d dials", dials.Load())
			}
		})
	}
}

func TestEstablishedStreamSurvivesDialContextCancellation(t *testing.T) {
	for _, transport := range []string{"tcp", "quic"} {
		t.Run(transport, func(t *testing.T) {
			out := newLifecycleOutbound(t, transport, &echoRouter{})
			dialCtx, cancelDial := context.WithCancel(context.Background())
			defer cancelDial()
			conn, err := out.DialContext(dialCtx, "tcp", M.ParseSocksaddr("echo.invalid:443"))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			cancelDial()
			time.Sleep(30 * time.Millisecond)
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			for i := 0; i < 3; i++ {
				if _, err = conn.Write([]byte("DNS over a reused connection")); err != nil {
					t.Fatal(err)
				}
				reply := make([]byte, len("DNS over a reused connection"))
				if _, err = io.ReadFull(conn, reply); err != nil {
					t.Fatal(err)
				}
				if string(reply) != "DNS over a reused connection" {
					t.Fatalf("corrupt reply: %q", reply)
				}
			}
		})
	}
}
