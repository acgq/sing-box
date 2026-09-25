package option

type QueqiaoOutboundOptions struct {
	DialerOptions
	ServerOptions
	ProfilePath string `json:"profile_path"`
	Transport   string `json:"transport,omitempty"`
	Congestion  string `json:"congestion,omitempty"`
	MaxSessions int    `json:"max_sessions,omitempty"`
}

type QueqiaoInboundOptions struct {
	ListenOptions
	ProviderPath string `json:"provider_path"`
	Transport    string `json:"transport,omitempty"`
	Congestion   string `json:"congestion,omitempty"`
	MaxSessions  int    `json:"max_sessions,omitempty"`
}
