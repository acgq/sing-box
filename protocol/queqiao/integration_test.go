package queqiao

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	Q "github.com/sagernet/sing-box/third_party/queqiao"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type recordingRouter struct {
	adapter.Router
	seen chan adapter.InboundContext
}

func (r *recordingRouter) RouteConnectionEx(ctx context.Context, c net.Conn, m adapter.InboundContext, closed N.CloseHandlerFunc) {
	r.seen <- m
	go func() {
		defer c.Close()
		payload, e := io.ReadAll(c)
		if e == nil {
			_, e = c.Write(append([]byte("reply:"), payload...))
		}
		if closed != nil {
			closed(e)
		}
	}()
}
func (r *recordingRouter) RoutePacketConnectionEx(ctx context.Context, c N.PacketConn, m adapter.InboundContext, closed N.CloseHandlerFunc) {
	r.seen <- m
	go func() {
		defer c.Close()
		defer func() {
			if closed != nil {
				closed(nil)
			}
		}()
		for {
			b := buf.NewPacket()
			address, e := c.ReadPacket(b)
			if e != nil {
				b.Release()
				return
			}
			if e = c.WritePacket(b, address); e != nil {
				return
			}
		}
	}()
}
func testProfile(t *testing.T, endpoint string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	providerPath := filepath.Join(dir, "provider")
	now := time.Now()
	p, e := Q.InitProvider(providerPath, "integration", endpoint, now)
	if e != nil {
		t.Fatal(e)
	}
	a, e := p.Store.AddAccount("integration", time.Time{}, Q.AccountLimits{}, now)
	if e != nil {
		t.Fatal(e)
	}
	_, invite, e := p.CreateInvitation(a.ID, time.Hour, now)
	if e != nil {
		t.Fatal(e)
	}
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	_, device, e := p.Store.ConsumeInvite(invite.Token, "integration", pub, now)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := p.IssueDevice(a.ID, device.ID, pub, now)
	if e != nil {
		t.Fatal(e)
	}
	der, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	profile := Q.ClientProfile{Version: 1, Name: "integration", Endpoint: endpoint, ProviderID: p.Metadata.ProviderID, GatewayID: p.Metadata.GatewayID, RootPin: p.Metadata.RootPin, AccountID: a.ID, DeviceID: device.ID, DeviceName: "integration", RootCertificate: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: p.RootCert.Raw})), DeviceCertificate: string(cert), DevicePrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), CreatedAt: now.UTC().Format(time.RFC3339)}
	path := filepath.Join(dir, "profile.json")
	data, e := json.Marshal(profile)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	return providerPath, path
}
func TestNativeTCPHalfCloseAndUDPMultipleDestinations(t *testing.T) {
	for _, transport := range []string{"tcp", "quic", "auto"} {
		t.Run(transport, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			reserve, e := net.ListenPacket("udp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			endpoint := reserve.LocalAddr().String()
			port := uint16(reserve.LocalAddr().(*net.UDPAddr).Port)
			if transport != "quic" {
				tcp, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				endpoint = tcp.Addr().String()
				port = uint16(tcp.Addr().(*net.TCPAddr).Port)
				tcp.Close()
			}
			reserve.Close()
			provider, profile := testProfile(t, endpoint)
			logger := log.NewNOPFactory().NewLogger("test")
			router := &recordingRouter{seen: make(chan adapter.InboundContext, 8)}
			serverTransport := transport
			if transport == "auto" {
				serverTransport = "tcp"
			}
			in, e := NewInbound(ctx, router, logger, "q-in", option.QueqiaoInboundOptions{ProviderPath: provider, Transport: serverTransport, ListenOptions: option.ListenOptions{ListenPort: port}})
			if e != nil {
				t.Fatal(e)
			}
			if e = in.(adapter.Lifecycle).Start(adapter.StartStateStart); e != nil {
				t.Fatal(e)
			}
			defer in.Close()
			out, e := NewOutbound(ctx, nil, logger, "q-out", option.QueqiaoOutboundOptions{ProfilePath: profile, Transport: transport})
			if e != nil {
				t.Fatal(e)
			}
			defer out.(*Outbound).Close()
			conn, e := out.DialContext(ctx, "tcp", M.ParseSocksaddr("unresolved.example:8080"))
			if e != nil {
				t.Fatal(e)
			}
			conn.SetDeadline(time.Now().Add(10 * time.Second))
			payload := bytes.Repeat([]byte("half-close-check"), 4096)
			if _, e = conn.Write(payload); e != nil {
				t.Fatal(e)
			}
			if e = conn.(interface{ CloseWrite() error }).CloseWrite(); e != nil {
				t.Fatal(e)
			}
			response, e := io.ReadAll(conn)
			conn.Close()
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(response, append([]byte("reply:"), payload...)) {
				t.Fatalf("TCP response corrupted: %d", len(response))
			}
			packets, e := out.ListenPacket(ctx, M.Socksaddr{})
			if e != nil {
				t.Fatal(e)
			}
			defer packets.Close()
			for _, target := range []string{"dns.invalid:53", "192.0.2.1:443"} {
				for _, value := range [][]byte{nil, []byte("udp payload")} {
					packets.SetDeadline(time.Now().Add(5 * time.Second))
					if _, e = packets.WriteTo(value, M.ParseSocksaddr(target)); e != nil {
						t.Fatal(e)
					}
					buffer := make([]byte, 1024)
					n, source, e := packets.ReadFrom(buffer)
					if e != nil {
						t.Fatal(e)
					}
					if source.String() != target || !bytes.Equal(buffer[:n], value) {
						t.Fatalf("UDP mismatch: %s %q", source, buffer[:n])
					}
				}
			}
			for range 3 {
				select {
				case m := <-router.seen:
					if m.Inbound != "q-in" || m.User == "" || !m.Source.IsValid() {
						t.Fatalf("metadata lost: %+v", m)
					}
				case <-ctx.Done():
					t.Fatal("missing routed flow")
				}
			}
		})
	}
}

func TestInlineIdentityAndPortHopping(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reserve, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(reserve.LocalAddr().(*net.UDPAddr).Port)
	reserve.Close()
	providerPath, profilePath := testProfile(t, net.JoinHostPort("127.0.0.1", fmt.Sprint(port)))
	provider, err := Q.LoadProvider(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := Q.LoadClientProfile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	identityPEM, err := os.ReadFile(filepath.Join(providerPath, "gateway-identity.pem"))
	if err != nil {
		t.Fatal(err)
	}
	users := make([]option.QueqiaoInboundUser, 0)
	for _, account := range provider.Store.Accounts() {
		for _, device := range provider.Store.Devices(account.ID) {
			users = append(users, option.QueqiaoInboundUser{
				Name: account.Name, DeviceName: device.Name,
				AccountID: account.ID, DeviceID: device.ID,
				PublicKey: device.PublicKey, MaxFlows: account.MaxFlows,
				MaxClients: account.MaxClients, ExpiresAt: account.ExpiresAt,
			})
		}
	}
	rootPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.RootCert.Raw}))
	if err = os.RemoveAll(providerPath); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(profilePath); err != nil {
		t.Fatal(err)
	}
	logger := log.NewNOPFactory().NewLogger("test")
	router := &recordingRouter{seen: make(chan adapter.InboundContext, 8)}
	in, err := NewInbound(ctx, router, logger, "inline-in", option.QueqiaoInboundOptions{
		ListenOptions: option.ListenOptions{ListenPort: port},
		ProviderID:    provider.Metadata.ProviderID, GatewayID: provider.Metadata.GatewayID,
		RootCertificate: rootPEM, GatewayCertificate: string(identityPEM),
		GatewayPrivateKey: string(identityPEM), Users: users,
		Transport: "quic", HopPortCount: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = in.(adapter.Lifecycle).Start(adapter.StartStateStart); err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := NewOutbound(ctx, nil, logger, "inline-out", option.QueqiaoOutboundOptions{
		ServerOptions: option.ServerOptions{Server: "127.0.0.1", ServerPort: port},
		ProviderID:    profile.ProviderID, GatewayID: profile.GatewayID,
		RootCertificate: profile.RootCertificate, DeviceCertificate: profile.DeviceCertificate,
		DevicePrivateKey: profile.DevicePrivateKey, Transport: "quic", HopPortCount: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer out.(*Outbound).Close()
	connection, err := out.DialContext(ctx, "tcp", M.ParseSocksaddr("inline.example:443"))
	if err != nil {
		t.Fatal(err)
	}
	connection.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err = connection.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err = connection.(interface{ CloseWrite() error }).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	answer, err := io.ReadAll(connection)
	connection.Close()
	if err != nil || string(answer) != "reply:hello" {
		t.Fatalf("inline TCP response = %q, %v", answer, err)
	}
	packets, err := out.ListenPacket(ctx, M.Socksaddr{})
	if err != nil {
		t.Fatal(err)
	}
	defer packets.Close()
	packets.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err = packets.WriteTo([]byte("udp"), M.ParseSocksaddr("inline.example:53")); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 32)
	n, source, err := packets.ReadFrom(buffer)
	if err != nil || source.String() != "inline.example:53" || string(buffer[:n]) != "udp" {
		t.Fatalf("inline UDP response = %s %q, %v", source, buffer[:n], err)
	}
}

func TestPacketPipeDeadlineAndBoundaries(t *testing.T) {
	a, b := newPacketPipe()
	defer a.Close()
	defer b.Close()
	a.SetReadDeadline(time.Now().Add(-time.Second))
	if _, _, e := a.ReadFrom(make([]byte, 1)); !os.IsTimeout(e) {
		t.Fatalf("deadline: %v", e)
	}
	a.SetReadDeadline(time.Time{})
	for _, value := range [][]byte{[]byte("long"), nil, []byte("next")} {
		if _, e := b.WriteTo(value, Q.PacketAddress("example.org:53")); e != nil {
			t.Fatal(e)
		}
	}
	for _, want := range []string{"lo", "", "ne"} {
		p := make([]byte, 2)
		n, _, e := a.ReadFrom(p)
		if e != nil || string(p[:n]) != want {
			t.Fatalf("boundary: %q %v", p[:n], e)
		}
	}
}
