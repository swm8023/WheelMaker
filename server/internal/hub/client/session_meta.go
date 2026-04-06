package client

import acp "github.com/swm8023/wheelmaker/internal/protocol"

// clientInitMeta holds agent-level metadata from the initialize handshake.
type clientInitMeta struct {
	ProtocolVersion       string
	AgentCapabilities     acp.AgentCapabilities
	AgentInfo             *acp.AgentInfo
	AuthMethods           []acp.AuthMethod
	ClientProtocolVersion int
	ClientCapabilities    acp.ClientCapabilities
	ClientInfo            *acp.AgentInfo
}

// clientSessionMeta holds session-level metadata updated by session/update notifications.
type clientSessionMeta struct {
	ConfigOptions     []acp.ConfigOption
	AvailableCommands []acp.AvailableCommand
	Title             string
	UpdatedAt         string
}

// cloneClientSessionMeta deep-copies session meta slices for persistence/snapshot safety.
func cloneClientSessionMeta(src clientSessionMeta) clientSessionMeta {
	return clientSessionMeta{
		ConfigOptions:     append([]acp.ConfigOption(nil), src.ConfigOptions...),
		AvailableCommands: append([]acp.AvailableCommand(nil), src.AvailableCommands...),
		Title:             src.Title,
		UpdatedAt:         src.UpdatedAt,
	}
}
