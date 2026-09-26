//go:build with_quic

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	Q "github.com/sagernet/sing-box/third_party/queqiao"
	"github.com/spf13/cobra"
)

var generateQueqiaoFlags struct {
	server, listen, providerName, accountName, deviceName string
	serverBase, serverOutput, clientOutput, inboundTag    string
	port, clientListenPort, hopPortCount, validYears      int
}

var commandGenerateQueqiao = &cobra.Command{
	Use:   "queqiao",
	Short: "Generate self-contained Queqiao server and client configurations",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return generateQueqiaoConfigs()
	},
}

func init() {
	f := commandGenerateQueqiao.Flags()
	f.StringVar(&generateQueqiaoFlags.server, "server", "", "Public server IP address or domain")
	f.StringVar(&generateQueqiaoFlags.listen, "listen", "0.0.0.0", "Server listen address")
	f.IntVar(&generateQueqiaoFlags.port, "server-port", 18443, "Queqiao server TCP/UDP port")
	f.StringVar(&generateQueqiaoFlags.providerName, "provider-name", "Queqiao", "Provider name")
	f.StringVar(&generateQueqiaoFlags.accountName, "account-name", "owner", "First account name")
	f.StringVar(&generateQueqiaoFlags.deviceName, "device-name", "device", "First device name")
	f.IntVar(&generateQueqiaoFlags.validYears, "valid-years", 10, "Certificate validity in calendar years (1-10)")
	f.IntVar(&generateQueqiaoFlags.hopPortCount, "hop-port-count", 4, "Number of Queqiao UDP hop ports (0-100)")
	f.IntVar(&generateQueqiaoFlags.clientListenPort, "client-listen-port", 1080, "Client mixed proxy listen port")
	f.StringVar(&generateQueqiaoFlags.inboundTag, "inbound-tag", "queqiao-in", "Server inbound tag to add or replace")
	f.StringVar(&generateQueqiaoFlags.serverBase, "server-base", "", "Existing server JSON config to preserve other services and replace the tagged Queqiao inbound")
	f.StringVar(&generateQueqiaoFlags.serverOutput, "server-output", "queqiao-server.json", "New server config file (must not exist)")
	f.StringVar(&generateQueqiaoFlags.clientOutput, "client-output", "queqiao-client.json", "New client config file (must not exist)")
	commandGenerate.AddCommand(commandGenerateQueqiao)
}

func generateQueqiaoConfigs() error {
	f := generateQueqiaoFlags
	if f.server == "" || f.server != strings.TrimSpace(f.server) || strings.ContainsAny(f.server, " \t\r\n") {
		return errors.New("--server must be a public IP address or domain")
	}
	if f.port < 1 || f.port > 65535 || f.clientListenPort < 1 || f.clientListenPort > 65535 {
		return errors.New("server and client listen ports must be between 1 and 65535")
	}
	if f.hopPortCount < 0 || f.hopPortCount > 100 {
		return errors.New("--hop-port-count must be between 0 and 100")
	}
	if f.inboundTag == "" || f.serverOutput == "" || f.clientOutput == "" || filepath.Clean(f.serverOutput) == filepath.Clean(f.clientOutput) {
		return errors.New("inbound tag and distinct output paths are required")
	}
	for _, path := range []string{f.serverOutput, f.clientOutput} {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("output already exists: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	bundle, err := Q.GenerateInlineBundle(f.providerName, f.accountName, f.deviceName, f.validYears, time.Now())
	if err != nil {
		return err
	}
	serverInbound := map[string]any{
		"type": "queqiao", "tag": f.inboundTag,
		"listen": f.listen, "listen_port": f.port,
		"provider_id": bundle.ProviderID, "gateway_id": bundle.GatewayID,
		"root_certificate":    bundle.RootCertificate,
		"gateway_certificate": bundle.GatewayCertificate,
		"gateway_private_key": bundle.GatewayPrivateKey,
		"users": []map[string]any{{
			"name": f.accountName, "device_name": f.deviceName,
			"account_id": bundle.AccountID, "device_id": bundle.DeviceID,
			"public_key": bundle.DevicePublicKey,
		}},
		"transport": "auto", "hop_port_count": f.hopPortCount,
	}
	server, err := queqiaoServerConfig(f.serverBase, f.inboundTag, serverInbound)
	if err != nil {
		return err
	}
	client := map[string]any{
		"inbounds": []map[string]any{{
			"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": f.clientListenPort,
		}},
		"outbounds": []map[string]any{{
			"type": "queqiao", "tag": "queqiao-out", "server": f.server, "server_port": f.port,
			"provider_id": bundle.ProviderID, "gateway_id": bundle.GatewayID,
			"root_certificate":   bundle.RootCertificate,
			"device_certificate": bundle.DeviceCertificate,
			"device_private_key": bundle.DevicePrivateKey,
			"transport":          "auto", "hop_port_count": f.hopPortCount,
		}},
	}
	serverBytes, err := json.MarshalIndent(server, "", "  ")
	if err != nil {
		return err
	}
	clientBytes, err := json.MarshalIndent(client, "", "  ")
	if err != nil {
		return err
	}
	if err = writeQueqiaoConfigNew(f.serverOutput, append(serverBytes, '\n')); err != nil {
		return err
	}
	if err = writeQueqiaoConfigNew(f.clientOutput, append(clientBytes, '\n')); err != nil {
		os.Remove(f.serverOutput)
		return err
	}
	fmt.Printf("Server: %s\nClient: %s\nCertificates expire: %s\n", f.serverOutput, f.clientOutput, bundle.ExpiresAt.UTC().Format(time.RFC3339))
	return nil
}

func queqiaoServerConfig(basePath, tag string, inbound map[string]any) (map[string]json.RawMessage, error) {
	var config map[string]json.RawMessage
	if basePath != "" {
		data, err := os.ReadFile(basePath)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("invalid server base JSON: %w", err)
		}
		if config == nil {
			return nil, errors.New("server base JSON must be an object")
		}
	} else {
		config = make(map[string]json.RawMessage)
		config["outbounds"] = json.RawMessage(`[{"type":"direct","tag":"direct"}]`)
	}
	var inbounds []json.RawMessage
	if raw := config["inbounds"]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &inbounds); err != nil {
			return nil, fmt.Errorf("invalid server inbounds: %w", err)
		}
	}
	encoded, err := json.Marshal(inbound)
	if err != nil {
		return nil, err
	}
	replaced := false
	for index, raw := range inbounds {
		var existing struct {
			Type string `json:"type"`
			Tag  string `json:"tag"`
		}
		if err := json.Unmarshal(raw, &existing); err != nil {
			return nil, fmt.Errorf("invalid server inbound: %w", err)
		}
		if existing.Tag != tag {
			continue
		}
		if existing.Type != "queqiao" || replaced {
			return nil, fmt.Errorf("inbound tag %q is not a unique Queqiao inbound", tag)
		}
		inbounds[index] = encoded
		replaced = true
	}
	if !replaced {
		inbounds = append(inbounds, encoded)
	}
	config["inbounds"], err = json.Marshal(inbounds)
	return config, err
}

func writeQueqiaoConfigNew(path string, data []byte) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(path)
		}
	}()
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
