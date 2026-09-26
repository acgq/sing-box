package queqiao

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// parseHopPorts expands explicit UDP ports and inclusive ranges. The primary
// server port is always part of the QUIC pool, so an occurrence in a range is
// ignored. Keep the pool bounded: the gateway opens one socket per port.
func parseHopPorts(entries []string, primary uint16) ([]int, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	if primary == 0 {
		return nil, errors.New("hop_ports requires a fixed server_port or listen_port")
	}
	seen := map[int]bool{int(primary): true}
	ports := make([]int, 0, 32)
	for _, entry := range entries {
		parts := strings.Split(entry, ":")
		if len(parts) < 1 || len(parts) > 2 {
			return nil, fmt.Errorf("invalid hop_ports entry %q", entry)
		}
		start, err := parseHopPort(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid hop_ports entry %q: %w", entry, err)
		}
		end := start
		if len(parts) == 2 {
			end, err = parseHopPort(parts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid hop_ports entry %q: %w", entry, err)
			}
		}
		if end < start {
			return nil, fmt.Errorf("invalid hop_ports range %q: end precedes start", entry)
		}
		for port := start; port <= end; port++ {
			if seen[port] {
				continue
			}
			seen[port] = true
			ports = append(ports, port)
			if len(ports) > 99 {
				return nil, errors.New("hop_ports may contain at most 99 additional ports")
			}
		}
	}
	if len(ports) == 0 {
		return nil, errors.New("hop_ports contains no port besides the primary port")
	}
	return ports, nil
}

func parseHopPort(value string) (int, error) {
	if value == "" || strings.Trim(value, "0123456789") != "" {
		return 0, errors.New("port must be a decimal number")
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("port must be between 1 and 65535")
	}
	return port, nil
}
