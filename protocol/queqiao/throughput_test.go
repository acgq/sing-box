package queqiao

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	Q "github.com/sagernet/sing-box/third_party/queqiao"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type downloadRouter struct{ adapter.Router }

func (*downloadRouter) RouteConnectionEx(_ context.Context, conn net.Conn, _ adapter.InboundContext, _ N.CloseHandlerFunc) {
	go serveDownload(conn)
}

const downloadBytes = 32 << 20

func serveDownload(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	block := make([]byte, 32<<10)
	for i := range block {
		block[i] = byte(i)
	}
	for sent := 0; sent < downloadBytes; {
		n, err := conn.Write(block)
		sent += n
		if err != nil {
			return
		}
	}
}

// Opt-in measurement, not a timing assertion. Loopback measures CPU/queue
// costs, not WAN capacity. Run with QUEQIAO_THROUGHPUT=1 and -count=3.
func TestDownloadProfiles(t *testing.T) {
	if os.Getenv("QUEQIAO_THROUGHPUT") != "1" {
		t.Skip("opt-in throughput measurement")
	}
	delay, _ := time.ParseDuration(os.Getenv("QUEQIAO_TEST_DELAY"))
	for _, mode := range []string{"direct", "tcp-default", "tcp-low", "quic-low-fixed", "quic-low", "quic-default"} {
		t.Run(mode, func(t *testing.T) {
			var conn net.Conn
			var out *Outbound
			var err error
			if mode == "direct" {
				listener, e := net.Listen("tcp", "127.0.0.1:0")
				if e != nil {
					t.Fatal(e)
				}
				defer listener.Close()
				go func() {
					c, e := listener.Accept()
					if e == nil {
						serveDownload(c)
					}
				}()
				conn, err = net.Dial("tcp", listener.Addr().String())
			} else {
				transport := "quic"
				if mode == "tcp-default" || mode == "tcp-low" {
					transport = "tcp"
				}
				out = newLifecycleOutboundDelayed(t, transport, &downloadRouter{}, delay, mode == "tcp-low" || mode == "quic-low" || mode == "quic-low-fixed")
				if mode == "quic-low-fixed" {
					// Recreate the released fixed-window profile for reproducible A/B runs.
					out.client.Close()
					credentials, e := out.profile.Credentials()
					if e != nil {
						t.Fatal(e)
					}
					config := Q.ClientConfig{RemoteAddr: out.remote, Credentials: credentials, Transport: Q.TransportKind(transport), EnableQUICPool: true, OuterDialer: out.identityDialer, Logger: newLogger(out.logger)}
					applyLowMemory(&config)
					config.MaxStreamReceiveWindow = config.StreamReceiveWindow
					config.MaxConnectionReceiveWindow = config.ConnectionReceiveWindow
					out.client, e = Q.NewClient(config)
					if e != nil {
						t.Fatal(e)
					}
				}
				conn, err = out.DialContext(context.Background(), "tcp", M.ParseSocksaddr("download.invalid:443"))
			}
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(15 * time.Second))
			start := time.Now()
			n, err := io.CopyN(io.Discard, conn, downloadBytes)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("received %d: %v", n, err)
			}
			stats := ""
			if out != nil {
				stats = fmt.Sprintf(" budgets=%+v", out.client.MemoryStats())
			}
			t.Logf("%.1f Mbit/s, %s%s", float64(n)*8/elapsed.Seconds()/1e6, elapsed, stats)
		})
	}
}

// A FIFO scheduler delays packets without sleeping in the socket reader or
// spawning one goroutine per datagram. This models latency, not bandwidth/loss.
func delayedUDPProxy(t *testing.T, target string, delay time.Duration) string {
	t.Helper()
	front, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	back, err := net.Dial("udp4", target)
	if err != nil {
		front.Close()
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var peerMu sync.Mutex
	var peer net.Addr
	type packet struct {
		data []byte
		due  time.Time
	}
	for _, upstream := range []bool{true, false} {
		queue := make(chan packet, 4096)
		wg.Add(2)
		go func(up bool) {
			defer wg.Done()
			buffer := make([]byte, 65535)
			for {
				var n int
				var err error
				if up {
					var addr net.Addr
					n, addr, err = front.ReadFrom(buffer)
					peerMu.Lock()
					peer = addr
					peerMu.Unlock()
				} else {
					n, err = back.Read(buffer)
				}
				if err != nil {
					return
				}
				p := packet{append([]byte(nil), buffer[:n]...), time.Now().Add(delay)}
				select {
				case queue <- p:
				case <-ctx.Done():
					return
				}
			}
		}(upstream)
		go func(up bool) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case p := <-queue:
					timer := time.NewTimer(time.Until(p.due))
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
					if up {
						_, _ = back.Write(p.data)
					} else {
						peerMu.Lock()
						addr := peer
						peerMu.Unlock()
						if addr != nil {
							_, _ = front.WriteTo(p.data, addr)
						}
					}
				}
			}
		}(upstream)
	}
	t.Cleanup(func() { cancel(); front.Close(); back.Close(); wg.Wait() })
	return front.LocalAddr().String()
}
