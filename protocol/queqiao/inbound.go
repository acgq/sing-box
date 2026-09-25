package queqiao

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/inbound"
	"github.com/sagernet/sing-box/common/listener"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	Q "github.com/sagernet/sing-box/third_party/queqiao"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/service/filemanager"
)

func RegisterInbound(r *inbound.Registry) {
	inbound.Register[option.QueqiaoInboundOptions](r, Type, NewInbound)
}

type Inbound struct {
	inbound.Adapter
	ctx      context.Context
	cancel   context.CancelFunc
	router   adapter.Router
	logger   log.ContextLogger
	listener *listener.Listener
	server   *Q.Server
	tcp, udp bool
	wg       sync.WaitGroup
	provider *Q.Provider
}

func NewInbound(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options option.QueqiaoInboundOptions) (adapter.Inbound, error) {
	if options.ProviderPath == "" {
		return nil, errors.New("missing provider_path")
	}
	provider, err := Q.LoadProvider(filemanager.BasePath(ctx, os.ExpandEnv(options.ProviderPath)))
	if err != nil {
		return nil, err
	}
	tcp, udp := true, true
	switch options.Transport {
	case "", "auto":
	case "tcp":
		udp = false
	case "quic":
		tcp = false
	default:
		return nil, errors.New("invalid queqiao transport")
	}
	runCtx, cancel := context.WithCancel(ctx)
	i := &Inbound{Adapter: inbound.NewAdapter(Type, tag), ctx: runCtx, cancel: cancel, router: router, logger: logger, tcp: tcp, udp: udp}
	i.listener = listener.New(listener.Options{Context: runCtx, Logger: logger, Listen: options.ListenOptions})
	server, err := Q.NewServer(Q.ServerConfig{ListenAddr: ":0", Credentials: provider.ServerCredentials(), Enrollment: &Q.EnrollmentService{Provider: provider}, EnableTCP: tcp, EnableQUIC: udp, Congestion: Q.CongestionControlKind(options.Congestion), MaxSessions: options.MaxSessions, DialDestination: i.dialDestination, ListenDestinationPacket: i.listenDestinationPacket, Logger: newLogger(logger)})
	if err != nil {
		cancel()
		return nil, err
	}
	i.server = server
	i.provider = provider
	return i, nil
}
func (i *Inbound) metadata(ctx context.Context, destination string, principal Q.Principal) adapter.InboundContext {
	return adapter.InboundContext{Inbound: i.Tag(), InboundType: Type, Destination: M.ParseSocksaddr(destination), User: principal.AccountID, Source: M.SocksaddrFromNet(Q.PeerAddressFromContext(ctx))}
}
func (i *Inbound) dialDestination(ctx context.Context, destination string, principal Q.Principal) (net.Conn, error) {
	engine, routed := newStreamPipe()
	i.router.RouteConnectionEx(ctx, routed, i.metadata(ctx, destination, principal), nil)
	return engine, nil
}
func (i *Inbound) Start(stage adapter.StartStage) error {
	if stage != adapter.StartStateStart {
		return nil
	}
	var tcp net.Listener
	var udp net.PacketConn
	var err error
	if i.tcp {
		tcp, err = i.listener.ListenTCP()
		if err != nil {
			return err
		}
	}
	if i.udp {
		udp, err = i.listener.ListenUDP()
		if err != nil {
			if tcp != nil {
				tcp.Close()
			}
			return err
		}
	}
	run := func(f func() error) {
		i.wg.Add(1)
		go func() {
			defer i.wg.Done()
			if e := f(); e != nil && i.ctx.Err() == nil {
				i.logger.ErrorContext(i.ctx, e)
				i.cancel()
			}
		}()
	}
	run(func() error { i.server.WatchAuthorization(i.ctx); return nil })
	run(func() error {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-i.ctx.Done():
				return nil
			case <-ticker.C:
				if _, e := i.provider.RenewGatewayIdentity(time.Now(), 7*24*time.Hour); e != nil {
					i.logger.ErrorContext(i.ctx, "renew gateway identity: ", e)
				}
			}
		}
	})
	if tcp != nil {
		run(func() error { return i.server.ServeListener(i.ctx, tcp) })
	}
	if udp != nil {
		run(func() error { return i.server.ServePacketConn(i.ctx, udp) })
	}
	return nil
}
func (i *Inbound) Close() error {
	i.cancel()
	err := i.listener.Close()
	i.wg.Wait()
	i.server.CloseRetainedPackets()
	return err
}

// Each destination is routed independently, including domain names. One Queqiao
// association can therefore carry DNS and other UDP traffic through different outbounds.
func (i *Inbound) listenDestinationPacket(ctx context.Context, principal Q.Principal) (net.PacketConn, error) {
	engine, bridge := newPacketPipe()
	relayCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(relayCtx, func() { bridge.Close() })
	go func() {
		defer cancel()
		defer stop()
		defer bridge.Close()
		var mu sync.Mutex
		flows := make(map[string]*packetPipe)
		defer func() {
			mu.Lock()
			defer mu.Unlock()
			for _, flow := range flows {
				flow.Close()
			}
		}()
		buffer := make([]byte, 65535)
		for {
			n, address, err := bridge.ReadFrom(buffer)
			if err != nil {
				return
			}
			destination := M.ParseSocksaddr(address.String())
			if !destination.IsValid() {
				continue
			}
			key := destination.String()
			mu.Lock()
			flow := flows[key]
			if flow == nil {
				if len(flows) >= 256 {
					mu.Unlock()
					continue
				}
				local, routed := newPacketPipe()
				flow = local
				flows[key] = flow
				go func(key string, local *packetPipe) {
					defer local.Close()
					defer func() {
						mu.Lock()
						if flows[key] == local {
							delete(flows, key)
						}
						mu.Unlock()
					}()
					response := make([]byte, 65535)
					for {
						local.SetReadDeadline(time.Now().Add(5 * time.Minute))
						count, source, e := local.ReadFrom(response)
						if e != nil {
							return
						}
						bridge.SetWriteDeadline(time.Now().Add(5 * time.Second))
						if _, e = bridge.WriteTo(response[:count], source); e != nil {
							return
						}
					}
				}(key, local)
				i.router.RoutePacketConnectionEx(relayCtx, bufio.NewPacketConn(routed), i.metadata(ctx, key, principal), func(error) { routed.Close() })
			}
			mu.Unlock()
			flow.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err = flow.WriteTo(buffer[:n], destination); err != nil {
				flow.Close()
			}
		}
	}()
	return &cancelPacketConn{engine, cancel}, nil
}
