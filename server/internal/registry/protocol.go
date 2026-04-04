package registry

import rp "github.com/swm8023/wheelmaker/internal/protocol"

const (
	codeUnauthorized    = rp.CodeUnauthorized
	codeInvalidArgument = rp.CodeInvalidArgument
	codeForbidden       = rp.CodeForbidden
	codeNotFound        = rp.CodeNotFound
	codeConflict        = rp.CodeConflict
	codeUnavailable     = rp.CodeUnavailable
	codeInternal        = rp.CodeInternal
	codeTimeout         = rp.CodeTimeout
)

type envelope = rp.Envelope

type errorPayload = rp.ErrorPayload

type connectInitPayload = rp.ConnectInitPayload

type connectInitResponsePayload = rp.ConnectInitResponsePayload

type hubReportProjectsPayload = rp.HubReportProjectsPayload

type hubUpdateProjectPayload = rp.HubUpdateProjectPayload

type projectListItem = rp.ProjectListItem

type syncCheckPayload = rp.SyncCheckPayload

type syncCheckResponsePayload = rp.SyncCheckResponsePayload
