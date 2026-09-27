package queqiao

import (
	"os"
	"runtime"
	"strconv"
	"strings"

	Q "github.com/sagernet/sing-box/third_party/queqiao"
)

func useLowMemory(configured *bool) bool {
	if configured != nil {
		return *configured
	}
	if runtime.GOOS != "linux" {
		return false
	}
	data, err := os.ReadFile("/proc/meminfo")
	return err == nil && smallSystemMemory(string(data))
}

func smallSystemMemory(meminfo string) bool {
	for _, line := range strings.Split(meminfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "MemTotal:" && fields[2] == "kB" {
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			return err == nil && kb > 0 && kb <= 512*1024
		}
	}
	return false
}

func applyLowMemory(config *Q.ClientConfig) {
	if config.MaxSessions == 0 {
		config.MaxSessions = 128
	}
	config.MaxPendingOpens = 32
	config.StreamReceiveWindow, config.MaxStreamReceiveWindow = 1<<20, 1<<20
	config.ConnectionReceiveWindow, config.MaxConnectionReceiveWindow = 4<<20, 4<<20
	config.MaxIncomingStreams = 128
	config.MemoryLimits = &Q.MemoryLimits{
		SendBudgetBytes: 4 << 20, ReceiveBudgetBytes: 8 << 20,
		MaxFlowSendBytes: 1 << 20, MaxFlowReceiveBytes: 2 << 20,
		MaxFlowOutstanding: 128, MaxFlowReceiveFrames: 128,
		EventQueueFrames: 4, LaneWriteQueueFrames: 8, LaneInteractiveReserve: 2,
		// Each UDP association holds one read buffer of MaxUDPPacketBytes,
		// so 65535 would retain 64 KiB per association. 4096 still covers
		// EDNS0-sized DNS and any path-MTU datagram; larger local datagrams
		// are truncated instead of relayed.
		FrameReadBufferBytes: 4096, MaxUDPPacketBytes: 4096, MaxBulkConnections: 1,
	}
}
