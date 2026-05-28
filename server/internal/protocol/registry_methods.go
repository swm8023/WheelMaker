package protocol

import "strings"

const (
	RegistryEnvelopeTypeRequest  = "request"
	RegistryEnvelopeTypeResponse = "response"
	RegistryEnvelopeTypeEvent    = "event"
	RegistryEnvelopeTypeError    = "error"
)

type RegistryRole string

const (
	RegistryRoleHub       RegistryRole = "hub"
	RegistryRoleClient    RegistryRole = "client"
	RegistryRoleMonitor   RegistryRole = "monitor"
	RegistryRoleLocalRead RegistryRole = "local_read"
)

type RegistryRouteKind string

const (
	RegistryRouteConnect         RegistryRouteKind = "connect"
	RegistryRouteBatch           RegistryRouteKind = "batch"
	RegistryRouteHubControl      RegistryRouteKind = "hub_control"
	RegistryRouteHubReport       RegistryRouteKind = "hub_report"
	RegistryRouteHubSessionEvent RegistryRouteKind = "hub_session_event"
	RegistryRouteProjectCache    RegistryRouteKind = "project_cache"
	RegistryRouteProjectForward  RegistryRouteKind = "project_forward"
	RegistryRouteSessionForward  RegistryRouteKind = "session_forward"
	RegistryRouteHubCommand      RegistryRouteKind = "hub_command"
	RegistryRouteMonitorCache    RegistryRouteKind = "monitor_cache"
	RegistryRouteMonitorForward  RegistryRouteKind = "monitor_forward"
	RegistryRouteRelayControl    RegistryRouteKind = "relay_control"
	RegistryRouteRelayHub        RegistryRouteKind = "relay_hub"
	RegistryRouteSpeech          RegistryRouteKind = "speech"
	RegistryRouteClientEvent     RegistryRouteKind = "client_event"
	RegistryRouteLocalRead       RegistryRouteKind = "local_read"
)

const (
	RegistryMethodConnectInit       = "connect.init"
	RegistryMethodConnectionClosing = "connection.closing"
	RegistryMethodLocalReadProof    = "local_read.proof"
	RegistryMethodBatch             = "batch"
	RegistryMethodHubPing           = "hub.ping"

	RegistryMethodRegistryReportProjects = "registry.reportProjects"
	RegistryMethodRegistryUpdateProject  = "registry.updateProject"
	RegistryMethodRegistrySessionUpdated = "registry.session.updated"
	RegistryMethodRegistrySessionMessage = "registry.session.message"

	RegistryMethodProjectList      = "project.list"
	RegistryMethodProjectSyncCheck = "project.syncCheck"
	RegistryMethodProjectOnline    = "project.online"
	RegistryMethodProjectOffline   = "project.offline"

	RegistryMethodSessionUpdated = "session.updated"
	RegistryMethodSessionMessage = "session.message"

	RegistryMethodSessionList               = "session.list"
	RegistryMethodSessionRead               = "session.read"
	RegistryMethodSessionSearch             = "session.search"
	RegistryMethodSessionNew                = "session.new"
	RegistryMethodSessionResumeList         = "session.resume.list"
	RegistryMethodSessionResumeImport       = "session.resume.import"
	RegistryMethodSessionReload             = "session.reload"
	RegistryMethodSessionArchive            = "session.archive"
	RegistryMethodSessionDelete             = "session.delete"
	RegistryMethodSessionRename             = "session.rename"
	RegistryMethodSessionSend               = "session.send"
	RegistryMethodSessionCancel             = "session.cancel"
	RegistryMethodSessionMarkRead           = "session.markRead"
	RegistryMethodSessionSetConfig          = "session.setConfig"
	RegistryMethodSessionAttachmentStart    = "session.attachment.start"
	RegistryMethodSessionAttachmentChunk    = "session.attachment.chunk"
	RegistryMethodSessionAttachmentFinish   = "session.attachment.finish"
	RegistryMethodSessionAttachmentCancel   = "session.attachment.cancel"
	RegistryMethodSessionAttachmentDelete   = "session.attachment.delete"
	RegistryMethodSessionTokenProviders     = "session.token.providers"
	RegistryMethodSessionTokenDeepSeekStats = "session.token.deepseek.stats"
	RegistryMethodSessionTokenScan          = "session.token.scan"

	RegistryMethodFSList   = "fs.list"
	RegistryMethodFSInfo   = "fs.info"
	RegistryMethodFSRead   = "fs.read"
	RegistryMethodFSSearch = "fs.search"
	RegistryMethodFSGrep   = "fs.grep"

	RegistryMethodGitRefs                = "git.refs"
	RegistryMethodGitBranchesLegacy      = "git.branches"
	RegistryMethodGitLog                 = "git.log"
	RegistryMethodGitCommitFiles         = "git.commit.files"
	RegistryMethodGitCommitFileDiff      = "git.commit.fileDiff"
	RegistryMethodGitDiff                = "git.diff"
	RegistryMethodGitDiffFileDiff        = "git.diff.fileDiff"
	RegistryMethodGitStatus              = "git.status"
	RegistryMethodGitWorkingTreeFileDiff = "git.workingTree.fileDiff"

	RegistryMethodMonitorListHub = "monitor.listHub"
	RegistryMethodMonitorStatus  = "monitor.status"
	RegistryMethodMonitorLog     = "monitor.log"
	RegistryMethodMonitorDB      = "monitor.db"
	RegistryMethodMonitorAction  = "monitor.action"

	RegistryMethodCmdNPM    = "cmd.npm"
	RegistryMethodCmdUpdate = "cmd.update"
	RegistryMethodCmdSkills = "cmd.skills"
	RegistryMethodCmdToken  = "cmd.token"

	RegistryMethodRelayEnable               = "relay.enable"
	RegistryMethodRelayDisable              = "relay.disable"
	RegistryMethodRelayStatus               = "relay.status"
	RegistryMethodRelayRegenerateAccessCode = "relay.regenerateAccessCode"
	RegistryMethodRelayOpen                 = "relay.open"
	RegistryMethodRelayClose                = "relay.close"

	RegistryMethodSpeechStart  = "speech.start"
	RegistryMethodSpeechChunk  = "speech.chunk"
	RegistryMethodSpeechFinish = "speech.finish"
	RegistryMethodSpeechCancel = "speech.cancel"

	LegacyRegistryMethodChatSend = "chat.send"
)

type RegistryMethodDescriptor struct {
	Method            string
	Route             RegistryRouteKind
	Roles             []RegistryRole
	RequiresProjectID bool
	RequiresHubID     bool
	Batchable         bool
	LocalRead         bool
	ClientEventMethod string
}

var RegistryMethodDescriptors = map[string]RegistryMethodDescriptor{
	RegistryMethodConnectInit:       registryMethod(RegistryMethodConnectInit, RegistryRouteConnect, nil),
	RegistryMethodConnectionClosing: registryClientEventMethod(RegistryMethodConnectionClosing),
	RegistryMethodLocalReadProof:    registryMethod(RegistryMethodLocalReadProof, RegistryRouteLocalRead, []RegistryRole{RegistryRoleLocalRead}),
	RegistryMethodBatch:             registryMethod(RegistryMethodBatch, RegistryRouteBatch, []RegistryRole{RegistryRoleClient, RegistryRoleMonitor}),
	RegistryMethodHubPing:           registryMethod(RegistryMethodHubPing, RegistryRouteHubControl, []RegistryRole{RegistryRoleHub}),

	RegistryMethodRegistryReportProjects: registryMethod(RegistryMethodRegistryReportProjects, RegistryRouteHubReport, []RegistryRole{RegistryRoleHub}),
	RegistryMethodRegistryUpdateProject:  registryMethod(RegistryMethodRegistryUpdateProject, RegistryRouteHubReport, []RegistryRole{RegistryRoleHub}),
	RegistryMethodRegistrySessionUpdated: registryHubSessionEventMethod(RegistryMethodRegistrySessionUpdated, RegistryMethodSessionUpdated),
	RegistryMethodRegistrySessionMessage: registryHubSessionEventMethod(RegistryMethodRegistrySessionMessage, RegistryMethodSessionMessage),

	RegistryMethodProjectList:      registryLocalReadMethod(RegistryMethodProjectList, RegistryRouteProjectCache, []RegistryRole{RegistryRoleClient, RegistryRoleMonitor}),
	RegistryMethodProjectSyncCheck: registryLocalReadMethod(RegistryMethodProjectSyncCheck, RegistryRouteProjectCache, []RegistryRole{RegistryRoleClient}),
	RegistryMethodProjectOnline:    registryClientEventMethod(RegistryMethodProjectOnline),
	RegistryMethodProjectOffline:   registryClientEventMethod(RegistryMethodProjectOffline),
	RegistryMethodSessionUpdated:   registryClientEventMethod(RegistryMethodSessionUpdated),
	RegistryMethodSessionMessage:   registryClientEventMethod(RegistryMethodSessionMessage),

	RegistryMethodSessionList:               registryProjectMethod(RegistryMethodSessionList, RegistryRouteSessionForward),
	RegistryMethodSessionRead:               registryProjectMethod(RegistryMethodSessionRead, RegistryRouteSessionForward),
	RegistryMethodSessionSearch:             registryProjectMethod(RegistryMethodSessionSearch, RegistryRouteSessionForward),
	RegistryMethodSessionNew:                registryProjectMethod(RegistryMethodSessionNew, RegistryRouteSessionForward),
	RegistryMethodSessionResumeList:         registryProjectMethod(RegistryMethodSessionResumeList, RegistryRouteSessionForward),
	RegistryMethodSessionResumeImport:       registryProjectMethod(RegistryMethodSessionResumeImport, RegistryRouteSessionForward),
	RegistryMethodSessionReload:             registryProjectMethod(RegistryMethodSessionReload, RegistryRouteSessionForward),
	RegistryMethodSessionArchive:            registryProjectMethod(RegistryMethodSessionArchive, RegistryRouteSessionForward),
	RegistryMethodSessionDelete:             registryProjectMethod(RegistryMethodSessionDelete, RegistryRouteSessionForward),
	RegistryMethodSessionRename:             registryProjectMethod(RegistryMethodSessionRename, RegistryRouteSessionForward),
	RegistryMethodSessionSend:               registryProjectMethod(RegistryMethodSessionSend, RegistryRouteSessionForward),
	RegistryMethodSessionCancel:             registryProjectMethod(RegistryMethodSessionCancel, RegistryRouteSessionForward),
	RegistryMethodSessionMarkRead:           registryProjectMethod(RegistryMethodSessionMarkRead, RegistryRouteSessionForward),
	RegistryMethodSessionSetConfig:          registryProjectMethod(RegistryMethodSessionSetConfig, RegistryRouteSessionForward),
	RegistryMethodSessionAttachmentStart:    registryProjectMethod(RegistryMethodSessionAttachmentStart, RegistryRouteSessionForward),
	RegistryMethodSessionAttachmentChunk:    registryProjectMethod(RegistryMethodSessionAttachmentChunk, RegistryRouteSessionForward),
	RegistryMethodSessionAttachmentFinish:   registryProjectMethod(RegistryMethodSessionAttachmentFinish, RegistryRouteSessionForward),
	RegistryMethodSessionAttachmentCancel:   registryProjectMethod(RegistryMethodSessionAttachmentCancel, RegistryRouteSessionForward),
	RegistryMethodSessionAttachmentDelete:   registryProjectMethod(RegistryMethodSessionAttachmentDelete, RegistryRouteSessionForward),
	RegistryMethodSessionTokenProviders:     registryProjectMethod(RegistryMethodSessionTokenProviders, RegistryRouteSessionForward),
	RegistryMethodSessionTokenDeepSeekStats: registryProjectMethod(RegistryMethodSessionTokenDeepSeekStats, RegistryRouteSessionForward),
	RegistryMethodSessionTokenScan:          registryProjectMethod(RegistryMethodSessionTokenScan, RegistryRouteSessionForward),

	RegistryMethodFSList:   registryLocalReadProjectMethod(RegistryMethodFSList),
	RegistryMethodFSInfo:   registryLocalReadProjectMethod(RegistryMethodFSInfo),
	RegistryMethodFSRead:   registryLocalReadProjectMethod(RegistryMethodFSRead),
	RegistryMethodFSSearch: registryLocalReadProjectMethod(RegistryMethodFSSearch),
	RegistryMethodFSGrep:   registryLocalReadProjectMethod(RegistryMethodFSGrep),

	RegistryMethodGitRefs:                registryLocalReadProjectMethod(RegistryMethodGitRefs),
	RegistryMethodGitLog:                 registryLocalReadProjectMethod(RegistryMethodGitLog),
	RegistryMethodGitCommitFiles:         registryLocalReadProjectMethod(RegistryMethodGitCommitFiles),
	RegistryMethodGitCommitFileDiff:      registryLocalReadProjectMethod(RegistryMethodGitCommitFileDiff),
	RegistryMethodGitDiff:                registryLocalReadProjectMethod(RegistryMethodGitDiff),
	RegistryMethodGitDiffFileDiff:        registryLocalReadProjectMethod(RegistryMethodGitDiffFileDiff),
	RegistryMethodGitStatus:              registryLocalReadProjectMethod(RegistryMethodGitStatus),
	RegistryMethodGitWorkingTreeFileDiff: registryLocalReadProjectMethod(RegistryMethodGitWorkingTreeFileDiff),

	RegistryMethodMonitorListHub: registryMethod(RegistryMethodMonitorListHub, RegistryRouteMonitorCache, []RegistryRole{RegistryRoleMonitor}),
	RegistryMethodMonitorStatus:  registryMethod(RegistryMethodMonitorStatus, RegistryRouteMonitorForward, []RegistryRole{RegistryRoleMonitor}),
	RegistryMethodMonitorLog:     registryMethod(RegistryMethodMonitorLog, RegistryRouteMonitorForward, []RegistryRole{RegistryRoleMonitor}),
	RegistryMethodMonitorDB:      registryMethod(RegistryMethodMonitorDB, RegistryRouteMonitorForward, []RegistryRole{RegistryRoleMonitor}),
	RegistryMethodMonitorAction:  registryMethod(RegistryMethodMonitorAction, RegistryRouteMonitorForward, []RegistryRole{RegistryRoleMonitor}),

	RegistryMethodCmdNPM:    registryHubCommandMethod(RegistryMethodCmdNPM),
	RegistryMethodCmdUpdate: registryHubCommandMethod(RegistryMethodCmdUpdate),
	RegistryMethodCmdSkills: registryHubCommandMethod(RegistryMethodCmdSkills),
	RegistryMethodCmdToken:  registryHubCommandMethod(RegistryMethodCmdToken),

	RegistryMethodRelayEnable:               registryMethod(RegistryMethodRelayEnable, RegistryRouteRelayControl, []RegistryRole{RegistryRoleClient}),
	RegistryMethodRelayDisable:              registryMethod(RegistryMethodRelayDisable, RegistryRouteRelayControl, []RegistryRole{RegistryRoleClient}),
	RegistryMethodRelayStatus:               registryMethod(RegistryMethodRelayStatus, RegistryRouteRelayControl, []RegistryRole{RegistryRoleClient}),
	RegistryMethodRelayRegenerateAccessCode: registryMethod(RegistryMethodRelayRegenerateAccessCode, RegistryRouteRelayControl, []RegistryRole{RegistryRoleClient}),
	RegistryMethodRelayOpen:                 registryMethod(RegistryMethodRelayOpen, RegistryRouteRelayHub, nil),
	RegistryMethodRelayClose:                registryMethod(RegistryMethodRelayClose, RegistryRouteRelayHub, nil),

	RegistryMethodSpeechStart:  registrySpeechMethod(RegistryMethodSpeechStart),
	RegistryMethodSpeechChunk:  registrySpeechMethod(RegistryMethodSpeechChunk),
	RegistryMethodSpeechFinish: registrySpeechMethod(RegistryMethodSpeechFinish),
	RegistryMethodSpeechCancel: registrySpeechMethod(RegistryMethodSpeechCancel),
}

func registryMethod(method string, route RegistryRouteKind, roles []RegistryRole) RegistryMethodDescriptor {
	return RegistryMethodDescriptor{Method: method, Route: route, Roles: roles}
}

func registryProjectMethod(method string, route RegistryRouteKind) RegistryMethodDescriptor {
	desc := registryMethod(method, route, []RegistryRole{RegistryRoleClient})
	desc.RequiresProjectID = true
	desc.Batchable = true
	return desc
}

func registryLocalReadMethod(method string, route RegistryRouteKind, roles []RegistryRole) RegistryMethodDescriptor {
	desc := registryMethod(method, route, roles)
	desc.LocalRead = true
	desc.Batchable = true
	return desc
}

func registryLocalReadProjectMethod(method string) RegistryMethodDescriptor {
	desc := registryProjectMethod(method, RegistryRouteProjectForward)
	desc.LocalRead = true
	return desc
}

func registryHubCommandMethod(method string) RegistryMethodDescriptor {
	desc := registryMethod(method, RegistryRouteHubCommand, []RegistryRole{RegistryRoleClient})
	desc.RequiresHubID = true
	desc.Batchable = true
	return desc
}

func registrySpeechMethod(method string) RegistryMethodDescriptor {
	return registryMethod(method, RegistryRouteSpeech, []RegistryRole{RegistryRoleClient})
}

func registryHubSessionEventMethod(method string, clientEventMethod string) RegistryMethodDescriptor {
	desc := registryMethod(method, RegistryRouteHubSessionEvent, []RegistryRole{RegistryRoleHub})
	desc.RequiresProjectID = true
	desc.ClientEventMethod = clientEventMethod
	return desc
}

func registryClientEventMethod(method string) RegistryMethodDescriptor {
	return registryMethod(method, RegistryRouteClientEvent, nil)
}

func RegistryMethod(method string) (RegistryMethodDescriptor, bool) {
	desc, ok := RegistryMethodDescriptors[strings.TrimSpace(method)]
	return desc, ok
}

func RegistryMethodAllowed(role string, method string) bool {
	desc, ok := RegistryMethod(method)
	if !ok {
		return false
	}
	want := RegistryRole(strings.TrimSpace(role))
	for _, allowed := range desc.Roles {
		if allowed == want {
			return true
		}
	}
	return false
}

func RegistryMethodHasRoute(method string, route RegistryRouteKind) bool {
	desc, ok := RegistryMethod(method)
	return ok && desc.Route == route
}

func RegistryClientForwardMethod(method string) bool {
	desc, ok := RegistryMethod(method)
	return ok && (desc.Route == RegistryRouteSessionForward || desc.Route == RegistryRouteProjectForward)
}

func RegistrySessionForwardMethod(method string) bool {
	return RegistryMethodHasRoute(method, RegistryRouteSessionForward)
}

func RegistryHubCommandMethod(method string) bool {
	return RegistryMethodHasRoute(method, RegistryRouteHubCommand)
}

func RegistryMonitorForwardMethod(method string) bool {
	return RegistryMethodHasRoute(method, RegistryRouteMonitorForward)
}

func RegistryRelayControlMethod(method string) bool {
	return RegistryMethodHasRoute(method, RegistryRouteRelayControl)
}

func RegistryRelayHubMethod(method string) bool {
	return RegistryMethodHasRoute(method, RegistryRouteRelayHub)
}

func RegistrySpeechMethod(method string) bool {
	return RegistryMethodHasRoute(method, RegistryRouteSpeech)
}

func RegistryLocalReadMethodAllowed(method string) bool {
	desc, ok := RegistryMethod(method)
	return ok && desc.LocalRead
}

func RegistryHubSessionEventMethod(method string) (string, bool) {
	desc, ok := RegistryMethod(method)
	if !ok || desc.Route != RegistryRouteHubSessionEvent || strings.TrimSpace(desc.ClientEventMethod) == "" {
		return "", false
	}
	return desc.ClientEventMethod, true
}
