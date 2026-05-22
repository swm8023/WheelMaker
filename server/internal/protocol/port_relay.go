package protocol

const (
	MethodRelayEnable               = "relay.enable"
	MethodRelayDisable              = "relay.disable"
	MethodRelayStatus               = "relay.status"
	MethodRelayRegenerateAccessCode = "relay.regenerateAccessCode"
	MethodRelayOpen                 = "relay.open"
	MethodRelayClose                = "relay.close"
)

type RelayStatus string

const (
	RelayStatusDisabled RelayStatus = "Disabled"
	RelayStatusOpening  RelayStatus = "Opening"
	RelayStatusUp       RelayStatus = "Up"
	RelayStatusError    RelayStatus = "Error"
)

type RelayEnablePayload struct {
	ListenPort int    `json:"listenPort"`
	HubID      string `json:"hubId"`
	TargetHost string `json:"targetHost"`
	TargetPort int    `json:"targetPort"`
	AccessCode string `json:"accessCode"`
}

type RelayRegenerateAccessCodePayload struct {
	AccessCode string `json:"accessCode"`
}

type RelaySnapshot struct {
	OK                   bool        `json:"ok"`
	Enabled              bool        `json:"enabled"`
	Status               RelayStatus `json:"status"`
	ListenPort           int         `json:"listenPort,omitempty"`
	HubID                string      `json:"hubId,omitempty"`
	TargetHost           string      `json:"targetHost,omitempty"`
	TargetPort           int         `json:"targetPort,omitempty"`
	AccessCodeGeneration int64       `json:"accessCodeGeneration,omitempty"`
	RelayURL             string      `json:"relayUrl,omitempty"`
	TunnelConnectedAt    string      `json:"tunnelConnectedAt,omitempty"`
	Error                string      `json:"error,omitempty"`
}

type RelayOpenPayload struct {
	RelayID    string `json:"relayId"`
	RelayURL   string `json:"relayURL"`
	Nonce      string `json:"nonce"`
	TargetHost string `json:"targetHost"`
	TargetPort int    `json:"targetPort"`
	UserAgent  string `json:"userAgent"`
	OpenedAt   string `json:"openedAt"`
}

type RelayClosePayload struct {
	RelayID string `json:"relayId"`
	Reason  string `json:"reason"`
}
