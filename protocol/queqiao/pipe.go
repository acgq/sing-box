package queqiao

import (
	"errors"
	"net"
	"os"
	"sync"
	"time"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/pipe"
)

// Separate directional pipes preserve TCP half-close without loopback sockets.
type streamPipe struct{ reader, writer net.Conn }

func newStreamPipe() (net.Conn, net.Conn) {
	ar, bw := net.Pipe()
	br, aw := net.Pipe()
	return &streamPipe{ar, aw}, &streamPipe{br, bw}
}
func (p *streamPipe) Read(b []byte) (int, error)         { return p.reader.Read(b) }
func (p *streamPipe) Write(b []byte) (int, error)        { return p.writer.Write(b) }
func (p *streamPipe) CloseWrite() error                  { return p.writer.Close() }
func (p *streamPipe) CloseRead() error                   { return p.reader.Close() }
func (p *streamPipe) Close() error                       { return errors.Join(p.reader.Close(), p.writer.Close()) }
func (p *streamPipe) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (p *streamPipe) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (p *streamPipe) SetReadDeadline(t time.Time) error  { return p.reader.SetReadDeadline(t) }
func (p *streamPipe) SetWriteDeadline(t time.Time) error { return p.writer.SetWriteDeadline(t) }
func (p *streamPipe) SetDeadline(t time.Time) error {
	return errors.Join(p.SetReadDeadline(t), p.SetWriteDeadline(t))
}

type packet struct {
	payload []byte
	address net.Addr
}
type packetPipe struct {
	incoming, outgoing          chan packet
	done                        chan struct{}
	once                        *sync.Once
	readDeadline, writeDeadline pipe.Deadline
}

func (p *packetPipe) ReadPacket(b *buf.Buffer) (M.Socksaddr, error) {
	n, address, err := p.ReadFrom(b.FreeBytes())
	if err != nil {
		return M.Socksaddr{}, err
	}
	b.Truncate(n)
	return M.ParseSocksaddr(address.String()), nil
}
func (p *packetPipe) WritePacket(b *buf.Buffer, address M.Socksaddr) error {
	defer b.Release()
	_, err := p.WriteTo(b.Bytes(), address)
	return err
}

func newPacketPipe() (*packetPipe, *packetPipe) {
	a, b := make(chan packet, 4), make(chan packet, 4)
	done := make(chan struct{})
	once := new(sync.Once)
	return &packetPipe{a, b, done, once, pipe.MakeDeadline(), pipe.MakeDeadline()}, &packetPipe{b, a, done, once, pipe.MakeDeadline(), pipe.MakeDeadline()}
}
func (p *packetPipe) ReadFrom(b []byte) (int, net.Addr, error) {
	select {
	case <-p.done:
		return 0, nil, net.ErrClosed
	case <-p.readDeadline.Wait():
		return 0, nil, os.ErrDeadlineExceeded
	default:
	}
	select {
	case v := <-p.incoming:
		return copy(b, v.payload), v.address, nil
	case <-p.done:
		return 0, nil, net.ErrClosed
	case <-p.readDeadline.Wait():
		return 0, nil, os.ErrDeadlineExceeded
	}
}
func (p *packetPipe) WriteTo(b []byte, a net.Addr) (int, error) {
	if a == nil {
		return 0, errors.New("missing UDP address")
	}
	if len(b) > 65507 {
		return 0, errors.New("UDP packet too large")
	}
	select {
	case <-p.done:
		return 0, net.ErrClosed
	case <-p.writeDeadline.Wait():
		return 0, os.ErrDeadlineExceeded
	default:
	}
	v := packet{append([]byte(nil), b...), M.ParseSocksaddr(a.String())}
	select {
	case p.outgoing <- v:
		return len(b), nil
	case <-p.done:
		return 0, net.ErrClosed
	case <-p.writeDeadline.Wait():
		return 0, os.ErrDeadlineExceeded
	}
}
func (p *packetPipe) Close() error                       { p.once.Do(func() { close(p.done) }); return nil }
func (p *packetPipe) LocalAddr() net.Addr                { return &net.UDPAddr{} }
func (p *packetPipe) SetReadDeadline(t time.Time) error  { p.readDeadline.Set(t); return nil }
func (p *packetPipe) SetWriteDeadline(t time.Time) error { p.writeDeadline.Set(t); return nil }
func (p *packetPipe) SetDeadline(t time.Time) error {
	p.readDeadline.Set(t)
	p.writeDeadline.Set(t)
	return nil
}
