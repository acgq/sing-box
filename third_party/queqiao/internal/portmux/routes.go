package portmux

import (
	"container/list"
	"net"
	"net/netip"
	"sync"
	"time"
)

const maxRoutes = 16384
const routeIdleTimeout = 2 * time.Minute

// Bound unauthenticated source-address state, including scans and NAT churn.
// Receive activity refreshes a route; QUIC keepalives preserve live mappings.
// AddrPort keys also avoid formatting an address on every packet.
type routeCache struct {
	mu      sync.Mutex
	entries map[netip.AddrPort]*list.Element
	order   list.List
}

type routeEntry struct {
	address netip.AddrPort
	conn    net.PacketConn
	seen    time.Time
}

func (c *routeCache) remove(e *list.Element) {
	delete(c.entries, e.Value.(*routeEntry).address)
	c.order.Remove(e)
}

func (c *routeCache) remember(address netip.AddrPort, conn net.PacketConn, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[netip.AddrPort]*list.Element)
	}
	for e := c.order.Front(); e != nil && now.Sub(e.Value.(*routeEntry).seen) >= routeIdleTimeout; e = c.order.Front() {
		c.remove(e)
	}
	if e := c.entries[address]; e != nil {
		entry := e.Value.(*routeEntry)
		entry.conn, entry.seen = conn, now
		c.order.MoveToBack(e)
		return
	}
	if len(c.entries) >= maxRoutes {
		c.remove(c.order.Front())
	}
	c.entries[address] = c.order.PushBack(&routeEntry{address, conn, now})
}

func (c *routeCache) lookup(address netip.AddrPort, now time.Time) net.PacketConn {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.entries[address]; e != nil {
		entry := e.Value.(*routeEntry)
		if now.Sub(entry.seen) < routeIdleTimeout {
			return entry.conn
		}
		c.remove(e)
	}
	return nil
}
