package shared

import (
	"encoding/json"

	gp "github.com/swm8023/wheelmaker/internal/protocol"
)

const (
	DefaultProtocolVersion = gp.DefaultProtocolVersion

	CodeUnauthorized    = gp.CodeUnauthorized
	CodeInvalidArgument = gp.CodeInvalidArgument
	CodeForbidden       = gp.CodeForbidden
	CodeNotFound        = gp.CodeNotFound
	CodeConflict        = gp.CodeConflict
	CodeUnavailable     = gp.CodeUnavailable
	CodeRateLimited     = gp.CodeRateLimited
	CodeInternal        = gp.CodeInternal
	CodeTimeout         = gp.CodeTimeout
)

type ProjectGitState = gp.ProjectGitState

type ProjectInfo = gp.ProjectInfo

type HubSnapshot = gp.HubSnapshot

type Envelope = gp.Envelope

type ErrorPayload = gp.ErrorPayload

type ConnectInitPayload = gp.ConnectInitPayload

type ConnectPrincipal = gp.ConnectPrincipal

type ConnectServerInfo = gp.ConnectServerInfo

type ConnectFeatures = gp.ConnectFeatures

type ConnectInitResponsePayload = gp.ConnectInitResponsePayload

type HubReportProjectsPayload = gp.HubReportProjectsPayload

type HubUpdateProjectPayload = gp.HubUpdateProjectPayload

type SyncCheckPayload = gp.SyncCheckPayload

type SyncCheckResponsePayload = gp.SyncCheckResponsePayload

type ProjectListItem = gp.ProjectListItem

func ProjectID(hubID, projectName string) string {
	return gp.ProjectID(hubID, projectName)
}

func MustRaw(v any) json.RawMessage {
	return gp.MustRaw(v)
}
