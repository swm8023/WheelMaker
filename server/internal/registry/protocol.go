package registry

import rp "github.com/swm8023/wheelmaker/internal/shared"

const (
	codeUnauthorized    = rp.CodeUnauthorized
	codeInvalidArgument = rp.CodeInvalidArgument
	codeNotFound        = rp.CodeNotFound
	codeInternal        = rp.CodeInternal
	codeTimeout         = rp.CodeTimeout
)

type envelope = rp.Envelope

type protocolError = rp.ProtocolError

type errorEnvelope = rp.ErrorEnvelope

type authPayload = rp.AuthPayload

type hubReportProjectsPayload = rp.HubReportProjectsPayload
