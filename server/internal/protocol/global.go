package protocol

import "encoding/json"

const (
	GlobalSchema  = "wm.global"
	GlobalVersion = "0.1"
)

type Component string

const (
	ComponentAgent    Component = "agent"
	ComponentClient   Component = "client"
	ComponentIM       Component = "im"
	ComponentRegistry Component = "registry"
)

type MessageType string

const (
	MessageTypeRequest  MessageType = "request"
	MessageTypeResponse MessageType = "response"
	MessageTypeEvent    MessageType = "event"
	MessageTypeError    MessageType = "error"
)

type Endpoint struct {
	Component Component `json:"component"`
	ID        string    `json:"id,omitempty"`
	SessionID string    `json:"sessionId,omitempty"`
	Channel   string    `json:"channel,omitempty"`
}

type GlobalEnvelope struct {
	Schema        string          `json:"schema"`
	Version       string          `json:"version"`
	MessageID     string          `json:"messageId"`
	CorrelationID string          `json:"correlationId,omitempty"`
	Type          MessageType     `json:"type"`
	Method        string          `json:"method"`
	ProjectID     string          `json:"projectId,omitempty"`
	Source        Endpoint        `json:"source"`
	Target        Endpoint        `json:"target"`
	TS            int64           `json:"ts"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

const (
	MethodAuthInit          = "auth.init"
	MethodSessionOpen       = "session.open"
	MethodSessionClose      = "session.close"
	MethodMessageUser       = "message.user"
	MethodMessageAgentDelta = "message.agent.delta"
	MethodDecisionRequest   = "decision.request"
	MethodDecisionReply     = "decision.reply"
	MethodProjectSyncCheck  = "project.syncCheck"
	MethodRegistryReport    = "registry.reportProjects"
	MethodRegistryUpdate    = "registry.updateProject"
	MethodHeartbeatPing     = "heartbeat.ping"
	MethodHeartbeatPong     = "heartbeat.pong"
)

type SessionOpenPayload struct {
	ProjectID string `json:"projectId"`
	Agent     string `json:"agent"`
	IMType    string `json:"imType"`
}

type UserMessagePayload struct {
	Text      string `json:"text"`
	RequestID string `json:"requestId,omitempty"`
}

type AgentDeltaPayload struct {
	Text  string `json:"text"`
	Final bool   `json:"final"`
}

type DecisionRequestPayload struct {
	DecisionID string           `json:"decisionId"`
	Title      string           `json:"title,omitempty"`
	Options    []DecisionOption `json:"options"`
}

type DecisionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type DecisionReplyPayload struct {
	DecisionID string `json:"decisionId"`
	OptionID   string `json:"optionId"`
}
