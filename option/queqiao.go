package option

type QueqiaoOutboundOptions struct {
	DialerOptions
	ServerOptions
	ProfilePath       string `json:"profile_path,omitempty"`
	ProviderID        string `json:"provider_id,omitempty"`
	GatewayID         string `json:"gateway_id,omitempty"`
	RootCertificate   string `json:"root_certificate,omitempty"`
	DeviceCertificate string `json:"device_certificate,omitempty"`
	DevicePrivateKey  string `json:"device_private_key,omitempty"`
	Transport         string `json:"transport,omitempty"`
	Congestion        string `json:"congestion,omitempty"`
	MaxSessions       int    `json:"max_sessions,omitempty"`
	HopPortCount      int    `json:"hop_port_count,omitempty"`
}

type QueqiaoInboundOptions struct {
	ListenOptions
	ProviderPath       string               `json:"provider_path,omitempty"`
	ProviderID         string               `json:"provider_id,omitempty"`
	GatewayID          string               `json:"gateway_id,omitempty"`
	RootCertificate    string               `json:"root_certificate,omitempty"`
	GatewayCertificate string               `json:"gateway_certificate,omitempty"`
	GatewayPrivateKey  string               `json:"gateway_private_key,omitempty"`
	Users              []QueqiaoInboundUser `json:"users,omitempty"`
	Transport          string               `json:"transport,omitempty"`
	Congestion         string               `json:"congestion,omitempty"`
	MaxSessions        int                  `json:"max_sessions,omitempty"`
	HopPortCount       int                  `json:"hop_port_count,omitempty"`
}

type QueqiaoInboundUser struct {
	Name       string `json:"name"`
	DeviceName string `json:"device_name,omitempty"`
	AccountID  string `json:"account_id"`
	DeviceID   string `json:"device_id"`
	PublicKey  string `json:"public_key"`
	MaxFlows   int    `json:"max_flows,omitempty"`
	MaxClients int    `json:"max_clients,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}
