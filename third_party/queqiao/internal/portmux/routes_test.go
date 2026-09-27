package portmux

import (
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestRouteCacheBoundsAndHopRefresh(t *testing.T) {
	var cache routeCache
	now := time.Now()
	first, second := &net.UDPConn{}, &net.UDPConn{}
	address := func(port int) netip.AddrPort {
		return netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port))
	}
	for i := 1; i <= maxRoutes; i++ {
		cache.remember(address(i), first, now)
	}
	cache.remember(address(1), second, now.Add(time.Second))
	cache.remember(address(maxRoutes+1), first, now.Add(time.Second))
	if len(cache.entries) != maxRoutes || cache.lookup(address(2), now) != nil {
		t.Fatal("route cache failed to evict the least recently received address")
	}
	if cache.lookup(address(1), now.Add(time.Second)) != second {
		t.Fatal("active route lost its updated hop socket")
	}
	later := now.Add(routeIdleTimeout + 2*time.Second)
	if cache.lookup(address(1), later) != nil {
		t.Fatal("expired route was retained")
	}
	cache.remember(address(3), first, later)
	if len(cache.entries) != 1 {
		t.Fatalf("expired routes retained: %d", len(cache.entries))
	}
}

func BenchmarkRouteCacheActive(b *testing.B) {
	var cache routeCache
	address := netip.MustParseAddrPort("127.0.0.1:12345")
	conn := &net.UDPConn{}
	now := time.Now()
	cache.remember(address, conn, now)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.remember(address, conn, now)
		cache.lookup(address, now)
	}
}
