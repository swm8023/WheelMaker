package registry

import "encoding/json"

const (
	codeUnauthorized    = "UNAUTHORIZED"
	codeInvalidArgument = "INVALID_ARGUMENT"
	codeNotFound        = "NOT_FOUND"
	codeInternal        = "INTERNAL"
	codeTimeout         = "TIMEOUT"
)

type envelope struct {
	Version   string          `json:"version,omitempty"`
	RequestID string          `json:"requestId,omitempty"`
	Type      string          `json:"type"`
	Method    string          `json:"method,omitempty"`
	ProjectID string          `json:"projectId,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     *protocolError  `json:"error,omitempty"`
}

type protocolError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type errorEnvelope struct {
	Version   string        `json:"version,omitempty"`
	RequestID string        `json:"requestId,omitempty"`
	Type      string        `json:"type"`
	Error     protocolError `json:"error"`
}

type authPayload struct {
	Token string `json:"token,omitempty"`
}

type hubReportProjectsPayload struct {
	HubID    string        `json:"hubId,omitempty"`
	Projects []ProjectInfo `json:"projects"`
}
