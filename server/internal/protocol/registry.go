package protocol

import (
	"encoding/json"
	"strings"
)

const (
	DefaultProtocolVersion = "2.2"

	CodeUnauthorized    = "UNAUTHORIZED"
	CodeInvalidArgument = "INVALID_ARGUMENT"
	CodeForbidden       = "FORBIDDEN"
	CodeNotFound        = "NOT_FOUND"
	CodeConflict        = "CONFLICT"
	CodeUnavailable     = "UNAVAILABLE"
	CodeRateLimited     = "RATE_LIMITED"
	CodeInternal        = "INTERNAL"
	CodeTimeout         = "TIMEOUT"
)

type ProjectGitState struct {
	Branch      string `json:"branch"`
	HeadSHA     string `json:"headSha"`
	Dirty       bool   `json:"dirty"`
	GitRev      string `json:"gitRev"`
	WorktreeRev string `json:"worktreeRev"`
}

type ProjectInfo struct {
	Name       string          `json:"name"`
	Path       string          `json:"path"`
	Online     bool            `json:"online"`
	Agent      string          `json:"agent"`
	Agents     []string        `json:"agents,omitempty"`
	IMType     string          `json:"imType"`
	ProjectRev string          `json:"projectRev"`
	Git        ProjectGitState `json:"git"`
}

type HubSnapshot struct {
	HubID           string        `json:"hubId"`
	ConnectionEpoch int64         `json:"connectionEpoch"`
	Projects        []ProjectInfo `json:"projects"`
	UpdatedAt       string        `json:"updatedAt"`
}

type Envelope struct {
	RequestID int64           `json:"requestId,omitempty"`
	Type      string          `json:"type"`
	Method    string          `json:"method,omitempty"`
	ProjectID string          `json:"projectId,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type ErrorPayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type ConnectInitPayload struct {
	ClientName      string `json:"clientName"`
	ClientVersion   string `json:"clientVersion"`
	ProtocolVersion string `json:"protocolVersion"`
	Role            string `json:"role"`
	HubID           string `json:"hubId,omitempty"`
	Token           string `json:"token"`
	TS              int64  `json:"ts,omitempty"`
	Nonce           string `json:"nonce,omitempty"`
}

type ConnectPrincipal struct {
	Role            string `json:"role"`
	HubID           string `json:"hubId,omitempty"`
	ConnectionEpoch int64  `json:"connectionEpoch"`
}

type ConnectServerInfo struct {
	ServerVersion   string `json:"serverVersion"`
	ProtocolVersion string `json:"protocolVersion"`
}

type ConnectFeatures struct {
	HubReportProjects       bool `json:"hubReportProjects"`
	PushHint                bool `json:"pushHint"`
	PingPong                bool `json:"pingPong"`
	SupportsHashNegotiation bool `json:"supportsHashNegotiation"`
	SupportsBatch           bool `json:"supportsBatch"`
}

type ConnectInitResponsePayload struct {
	OK             bool              `json:"ok"`
	Principal      ConnectPrincipal  `json:"principal"`
	ServerInfo     ConnectServerInfo `json:"serverInfo"`
	Features       ConnectFeatures   `json:"features"`
	HashAlgorithms []string          `json:"hashAlgorithms"`
}

type HubReportProjectsPayload struct {
	HubID           string        `json:"hubId"`
	ConnectionEpoch int64         `json:"connectionEpoch"`
	Projects        []ProjectInfo `json:"projects"`
}

type HubUpdateProjectPayload struct {
	HubID           string      `json:"hubId"`
	ConnectionEpoch int64       `json:"connectionEpoch"`
	Seq             int64       `json:"seq"`
	Project         ProjectInfo `json:"project"`
	ChangedDomains  []string    `json:"changedDomains,omitempty"`
	UpdatedAt       string      `json:"updatedAt"`
}

type SyncCheckPayload struct {
	KnownProjectRev  string `json:"knownProjectRev,omitempty"`
	KnownGitRev      string `json:"knownGitRev,omitempty"`
	KnownWorktreeRev string `json:"knownWorktreeRev,omitempty"`
}

type SyncCheckResponsePayload struct {
	ProjectRev   string   `json:"projectRev"`
	GitRev       string   `json:"gitRev"`
	WorktreeRev  string   `json:"worktreeRev"`
	StaleDomains []string `json:"staleDomains"`
}

type ProjectListItem struct {
	ProjectID  string          `json:"projectId"`
	Name       string          `json:"name"`
	Path       string          `json:"path"`
	Online     bool            `json:"online"`
	Agent      string          `json:"agent"`
	Agents     []string        `json:"agents,omitempty"`
	IMType     string          `json:"imType"`
	ProjectRev string          `json:"projectRev"`
	Git        ProjectGitState `json:"git"`
}

type MonitorHubRefPayload struct {
	HubID string `json:"hubId"`
}

type MonitorActionPayload struct {
	HubID  string `json:"hubId"`
	Action string `json:"action"`
}

type MonitorLogPayload struct {
	HubID string `json:"hubId"`
	File  string `json:"file,omitempty"`
	Level string `json:"level,omitempty"`
	Tail  int    `json:"tail,omitempty"`
}

func ProjectID(hubID, projectName string) string {
	hubID = strings.TrimSpace(hubID)
	projectName = strings.TrimSpace(projectName)
	if hubID == "" {
		return projectName
	}
	if projectName == "" {
		return hubID + ":"
	}
	return hubID + ":" + projectName
}

func MustRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
