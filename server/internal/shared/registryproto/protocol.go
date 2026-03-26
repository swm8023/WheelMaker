package registryproto

import "encoding/json"

const (
	DefaultProtocolVersion = "1.0"

	CodeUnauthorized    = "UNAUTHORIZED"
	CodeInvalidArgument = "INVALID_ARGUMENT"
	CodeNotFound        = "NOT_FOUND"
	CodeInternal        = "INTERNAL"
	CodeTimeout         = "TIMEOUT"
)

type ProjectInfo struct {
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Agent  string `json:"agent,omitempty"`
	IMType string `json:"imType,omitempty"`
}

type HubSnapshot struct {
	HubID     string        `json:"hubId"`
	Projects  []ProjectInfo `json:"projects"`
	UpdatedAt string        `json:"updatedAt"`
}

type Envelope struct {
	Version   string          `json:"version,omitempty"`
	RequestID string          `json:"requestId,omitempty"`
	Type      string          `json:"type"`
	Method    string          `json:"method,omitempty"`
	ProjectID string          `json:"projectId,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     *ProtocolError  `json:"error,omitempty"`
}

type ProtocolError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type ErrorEnvelope struct {
	Version   string        `json:"version,omitempty"`
	RequestID string        `json:"requestId,omitempty"`
	Type      string        `json:"type"`
	Error     ProtocolError `json:"error"`
}

type AuthPayload struct {
	Token string `json:"token,omitempty"`
}

type HubReportProjectsPayload struct {
	HubID    string        `json:"hubId,omitempty"`
	Projects []ProjectInfo `json:"projects"`
}

func MustRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
