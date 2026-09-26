//go:build with_quic

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestQueqiaoServerConfigReplacesOnlyMatchingInbound(t *testing.T) {
	base := filepath.Join(t.TempDir(), "base.json")
	data := []byte(`{"inbounds":[{"type":"vless","tag":"existing","listen_port":8443},{"type":"queqiao","tag":"queqiao-in","listen_port":18443}],"outbounds":[{"type":"direct","tag":"direct"}],"route":{"final":"direct"}}`)
	if err := os.WriteFile(base, data, 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := queqiaoServerConfig(base, "queqiao-in", map[string]any{"type": "queqiao", "tag": "queqiao-in", "listen_port": 18445})
	if err != nil {
		t.Fatal(err)
	}
	var inbounds []struct {
		Type       string `json:"type"`
		ListenPort int    `json:"listen_port"`
	}
	if err := json.Unmarshal(config["inbounds"], &inbounds); err != nil {
		t.Fatal(err)
	}
	if len(inbounds) != 2 || inbounds[0].Type != "vless" || inbounds[0].ListenPort != 8443 || inbounds[1].ListenPort != 18445 {
		t.Fatalf("unexpected inbounds: %+v", inbounds)
	}
	if string(config["route"]) != `{"final":"direct"}` {
		t.Fatalf("route changed: %s", config["route"])
	}
}
