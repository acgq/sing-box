package queqiao

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/common/dialer"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	Q "github.com/sagernet/sing-box/third_party/queqiao"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
	"github.com/sagernet/sing/service/filemanager"
)

const Type = C.TypeQueqiao

func RegisterOutbound(r *outbound.Registry) {
	outbound.Register[option.QueqiaoOutboundOptions](r, Type, NewOutbound)
}

type Outbound struct {
	outbound.Adapter
	ctx            context.Context
	cancel         context.CancelFunc
	client         *Q.Client
	mu             sync.Mutex
	closed         bool
	wg             sync.WaitGroup
	profile        Q.ClientProfile
	profilePath    string
	remote         string
	identityDialer *outerDialer
	logger         log.ContextLogger
}

func NewOutbound(ctx context.Context, _ adapter.Router, logger log.ContextLogger, tag string, options option.QueqiaoOutboundOptions) (adapter.Outbound, error) {
	if options.HopPortCount < 0 || options.HopPortCount > 100 {
		return nil, errors.New("hop_port_count must be between 0 and 100")
	}
	var profile Q.ClientProfile
	var credentials Q.ClientCredentials
	var remote string
	var profilePath string
	hopPortCount := options.HopPortCount
	if options.ProfilePath != "" {
		if options.ProviderID != "" || options.GatewayID != "" || options.RootCertificate != "" ||
			options.DeviceCertificate != "" || options.DevicePrivateKey != "" {
			return nil, errors.New("profile_path and inline Queqiao identity are mutually exclusive")
		}
		profilePath = filemanager.BasePath(ctx, os.ExpandEnv(options.ProfilePath))
		var err error
		profile, err = Q.LoadClientProfile(profilePath)
		if err != nil {
			return nil, err
		}
		credentials, err = profile.Credentials()
		if err != nil {
			return nil, err
		}
		remote = profile.Endpoint
		if hopPortCount == 0 {
			hopPortCount = profile.HopPortCount
		}
	} else {
		var err error
		credentials, err = inlineClientCredentials(options)
		if err != nil {
			return nil, err
		}
	}
	if hopPortCount < 0 || hopPortCount > 100 {
		return nil, errors.New("profile hop_port_count must be between 0 and 100")
	}
	if options.Server == "" && options.ServerPort != 0 {
		return nil, errors.New("server_port requires server")
	}
	if options.Server != "" {
		if options.ServerPort == 0 {
			return nil, errors.New("missing server_port")
		}
		remote = options.ServerOptions.Build().String()
	}
	if remote == "" {
		return nil, errors.New("missing Queqiao server and server_port")
	}
	destination := M.ParseSocksaddr(remote)
	transport := options.Transport
	if transport == "" {
		transport = "auto"
	}
	d, err := dialer.NewWithOptions(dialer.Options{Context: ctx, Options: options.DialerOptions, RemoteIsDomain: destination.IsDomain()})
	if err != nil {
		return nil, err
	}
	var query adapter.DNSQueryOptions
	if destination.IsDomain() {
		query, err = dialer.NewDNSQueryOptions(ctx, options.DomainResolver, true)
		if err != nil {
			return nil, err
		}
	}
	hostDialer := &outerDialer{d, service.FromContext[adapter.DNSRouter](ctx), query}
	engine, err := Q.NewClient(Q.ClientConfig{RemoteAddr: remote, Credentials: credentials, Transport: Q.TransportKind(transport), Congestion: Q.CongestionControlKind(options.Congestion), EnableQUICPool: true, MaxSessions: options.MaxSessions, HopPortCount: hopPortCount, OuterDialer: hostDialer, Logger: newLogger(logger)})
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	return &Outbound{Adapter: outbound.NewAdapterWithDialerOptions(Type, tag, []string{N.NetworkTCP, N.NetworkUDP}, options.DialerOptions), ctx: runCtx, cancel: cancel, client: engine, profile: profile, profilePath: profilePath, remote: remote, identityDialer: hostDialer, logger: logger}, nil
}

func (o *Outbound) Start(stage adapter.StartStage) error {
	if stage != adapter.StartStateStarted {
		return nil
	}
	ctx, finish, err := o.begin(o.ctx)
	if err != nil {
		return err
	}
	go func(ctx context.Context, finish func()) { defer finish(); o.client.Warmup(ctx) }(ctx, finish)
	if o.profilePath == "" {
		return nil
	}
	ctx, finish, err = o.begin(o.ctx)
	if err != nil {
		return err
	}
	go func() {
		defer finish()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				needed, e := o.profile.NeedsRenewal(time.Now(), 7*24*time.Hour)
				if e != nil {
					o.logger.ErrorContext(ctx, "check device identity: ", e)
					continue
				}
				if !needed {
					continue
				}
				profile := o.profile
				profile.Endpoint = o.remote
				renewed, e := Q.RenewProfileWithOptions(ctx, profile, Q.DialOptions{Timeout: 30 * time.Second, DialContext: o.identityDialer.DialContext})
				if e != nil {
					o.logger.WarnContext(ctx, "renew device identity: ", e)
					continue
				}
				renewed.Endpoint = o.profile.Endpoint
				if e = renewed.Save(o.profilePath); e != nil {
					o.logger.ErrorContext(ctx, "save device identity: ", e)
					continue
				}
				credentials, e := renewed.Credentials()
				if e == nil {
					e = o.client.UpdateCredentials(credentials)
				}
				if e != nil {
					o.logger.ErrorContext(ctx, "activate device identity: ", e)
					continue
				}
				o.profile = renewed
			}
		}
	}()
	return nil
}
func (o *Outbound) begin(ctx context.Context) (context.Context, func(), error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil, nil, net.ErrClosed
	}
	child, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(o.ctx, cancel)
	o.wg.Add(1)
	return child, func() { stop(); cancel(); o.wg.Done() }, nil
}
func (o *Outbound) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	if N.NetworkName(network) == N.NetworkUDP {
		c, e := o.ListenPacket(ctx, destination)
		if e != nil {
			return nil, e
		}
		return bufio.NewBindPacketConn(c, destination), nil
	}
	if N.NetworkName(network) != N.NetworkTCP {
		return nil, errors.New("unsupported network")
	}
	child, finish, err := o.begin(ctx)
	if err != nil {
		return nil, err
	}
	local, engine := newStreamPipe()
	ready := make(chan error, 1)
	stop := context.AfterFunc(child, func() { _ = engine.Close() })
	go func() {
		defer finish()
		defer stop()
		o.client.ServeStream(child, engine, destination.String(), func(e error) { ready <- e })
	}()
	select {
	case err = <-ready:
		if err != nil {
			local.Close()
			return nil, err
		}
		return local, nil
	case <-child.Done():
		local.Close()
		return nil, child.Err()
	}
}
func (o *Outbound) ListenPacket(ctx context.Context, _ M.Socksaddr) (net.PacketConn, error) {
	child, finish, err := o.begin(ctx)
	if err != nil {
		return nil, err
	}
	local, engine := newPacketPipe()
	ready := make(chan error, 1)
	runCtx, cancel := context.WithCancel(child)
	stop := context.AfterFunc(runCtx, func() { _ = engine.Close() })
	go func() {
		defer finish()
		defer cancel()
		defer stop()
		o.client.ServePacket(runCtx, engine, func(e error) { ready <- e })
	}()
	select {
	case err = <-ready:
		if err != nil {
			cancel()
			local.Close()
			return nil, err
		}
		return &cancelPacketConn{local, cancel}, nil
	case <-child.Done():
		cancel()
		local.Close()
		return nil, child.Err()
	}
}
func (o *Outbound) InterfaceUpdated(ctx context.Context) { o.client.Reset(ctx) }
func (o *Outbound) MultiplexEnabled() bool               { return true }
func (o *Outbound) Close() error {
	o.mu.Lock()
	o.closed = true
	o.cancel()
	o.mu.Unlock()
	err := o.client.Close()
	o.wg.Wait()
	return err
}

type cancelPacketConn struct {
	net.PacketConn
	cancel context.CancelFunc
}

func (c *cancelPacketConn) ReadPacket(b *buf.Buffer) (M.Socksaddr, error) {
	n, a, e := c.ReadFrom(b.FreeBytes())
	if e != nil {
		return M.Socksaddr{}, e
	}
	b.Truncate(n)
	return M.ParseSocksaddr(a.String()), nil
}
func (c *cancelPacketConn) WritePacket(b *buf.Buffer, a M.Socksaddr) error {
	defer b.Release()
	_, e := c.WriteTo(b.Bytes(), a)
	return e
}

func (c *cancelPacketConn) Close() error { c.cancel(); return c.PacketConn.Close() }

type outerDialer struct {
	dialer N.Dialer
	dns    adapter.DNSRouter
	query  adapter.DNSQueryOptions
}

func (d *outerDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.dialer.DialContext(ctx, network, M.ParseSocksaddr(address))
}
func (d *outerDialer) ListenPacket(ctx context.Context, address string) (net.PacketConn, *net.UDPAddr, error) {
	target := M.ParseSocksaddr(address)
	if target.IsDomain() {
		if d.dns == nil {
			return nil, nil, errors.New("missing DNS router")
		}
		addresses, err := d.dns.Lookup(ctx, target.Fqdn, d.query)
		if err != nil {
			return nil, nil, err
		}
		if len(addresses) == 0 {
			return nil, nil, errors.New("empty server DNS answer")
		}
		target = M.SocksaddrFrom(addresses[0], target.Port)
	}
	conn, err := d.dialer.ListenPacket(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	return conn, target.UDPAddr(), nil
}
