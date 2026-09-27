package queqiao

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	Q "github.com/sagernet/sing-box/third_party/queqiao"
	M "github.com/sagernet/sing/common/metadata"
)

func TestLowMemorySelection(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{"MemTotal: 233472 kB\nMemFree: 100 kB\n", true},
		{"MemTotal: 524288 kB", true}, {"MemTotal: 524289 kB", false},
		{"MemTotal: 0 kB", false}, {"MemTotal: invalid kB", false}, {"", false},
	} {
		if got := smallSystemMemory(tc.input); got != tc.want {
			t.Errorf("%q: %v", tc.input, got)
		}
	}
	for _, value := range []bool{false, true} {
		if useLowMemory(&value) != value {
			t.Fatal("explicit setting ignored")
		}
	}
	config := Q.ClientConfig{MaxSessions: 42}
	applyLowMemory(&config)
	if config.MaxSessions != 42 {
		t.Fatal("explicit session limit overwritten")
	}
	if config.MaxStreamReceiveWindow <= config.StreamReceiveWindow || config.MaxConnectionReceiveWindow <= config.ConnectionReceiveWindow {
		t.Fatal("low-memory windows cannot adapt to a higher bandwidth-delay product")
	}
	if config.MaxStreamReceiveWindow > config.MaxConnectionReceiveWindow || config.MaxConnectionReceiveWindow > 8<<20 {
		t.Fatal("low-memory QUIC connection cap exceeded")
	}
}

func TestLowMemoryConcurrentFlowsReleaseBudgets(t *testing.T) {
	for _, transport := range []string{"tcp", "quic"} {
		t.Run(transport, func(t *testing.T) {
			out := newLifecycleOutbound(t, transport, &echoRouter{}, true)
			var wg sync.WaitGroup
			for i := 0; i < 16; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					conn, err := out.DialContext(context.Background(), "tcp", M.ParseSocksaddr("echo.invalid:443"))
					if err != nil {
						t.Error(err)
						return
					}
					defer conn.Close()
					conn.SetDeadline(time.Now().Add(10 * time.Second))
					payload := bytes.Repeat([]byte("memory-budget"), 8192)
					written := make(chan error, 1)
					go func() { _, err := conn.Write(payload); written <- err }()
					response := make([]byte, len(payload))
					_, err = io.ReadFull(conn, response)
					if err != nil {
						t.Error(err)
					} else if !bytes.Equal(response, payload) {
						t.Error("payload corrupted")
					}
					if err := <-written; err != nil {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			out.Close()
			deadline := time.Now().Add(3 * time.Second)
			for {
				stats := out.client.MemoryStats()
				if stats.Send.Capacity != 4<<20 || stats.Receive.Capacity != 8<<20 {
					t.Fatalf("unexpected budgets: %+v", stats)
				}
				if stats.Send.Peak == 0 {
					t.Fatal("send budget was never exercised")
				}
				if stats.Send.Peak > stats.Send.Capacity || stats.Receive.Peak > stats.Receive.Capacity {
					t.Fatal("memory budget exceeded")
				}
				if stats.Send.Used == 0 && stats.Receive.Used == 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("shutdown retained payload: %+v", stats)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}
