package agentv2

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

func TestInstance_NewAndLoadWithoutACPReady(t *testing.T) {
	fc := &fakeConn{}
	inst := NewInstance("codex", fc, nil)

	newRes, err := inst.SessionNew(context.Background(), protocol.SessionNewParams{CWD: "."})
	if err != nil {
		t.Fatalf("session new: %v", err)
	}
	if newRes.SessionID == "" {
		t.Fatal("expected session id from session/new")
	}

	loadRes, err := inst.SessionLoad(context.Background(), protocol.SessionLoadParams{SessionID: "loaded-1", CWD: "."})
	if err != nil {
		t.Fatalf("session load: %v", err)
	}
	_ = loadRes

	impl := inst.(*instance)
	if !impl.acpSessionReady || impl.acpSessionID != "loaded-1" {
		t.Fatalf("acp session state not updated: ready=%v sid=%q", impl.acpSessionReady, impl.acpSessionID)
	}
}

func TestInstance_HandleInboundDispatch(t *testing.T) {
	fc := &fakeConn{}
	cb := &fakeCallbacks{}
	inst := NewInstance("codex", fc, cb)
	if fc.resp == nil || fc.req == nil {
		t.Fatal("expected ACP request/response handler registration")
	}

	updateRaw, _ := json.Marshal(protocol.SessionUpdateParams{
		SessionID: "acp-1",
		Update:    protocol.SessionUpdate{SessionUpdate: "agent_message_chunk"},
	})
	fc.resp(context.Background(), protocol.MethodSessionUpdate, updateRaw)
	if cb.updateCount != 1 {
		t.Fatalf("updateCount=%d, want 1", cb.updateCount)
	}

	permRaw, _ := json.Marshal(protocol.PermissionRequestParams{
		SessionID: "acp-1",
		ToolCall:  protocol.ToolCallRef{ToolCallID: "tc-1"},
		Options:   []protocol.PermissionOption{{OptionID: "allow", Name: "Allow", Kind: "once"}},
	})
	resp, err := fc.req(context.Background(), protocol.MethodRequestPermission, permRaw)
	if err != nil {
		t.Fatalf("permission dispatch: %v", err)
	}
	permResp, ok := resp.(protocol.PermissionResponse)
	if !ok {
		t.Fatalf("response type=%T, want protocol.PermissionResponse", resp)
	}
	if permResp.Outcome.Outcome != "allow_once" {
		t.Fatalf("permission outcome=%q", permResp.Outcome.Outcome)
	}
	if cb.permissionCount != 1 {
		t.Fatalf("permissionCount=%d, want 1", cb.permissionCount)
	}

	_ = inst
}

type fakeConn struct {
	req  ACPRequestHandler
	resp ACPResponseHandler
}

func (f *fakeConn) Send(_ context.Context, method string, _ any, result any) error {
	switch method {
	case protocol.MethodInitialize:
		if out, ok := result.(*protocol.InitializeResult); ok {
			out.ProtocolVersion = json.Number("1")
		}
	case protocol.MethodSessionNew:
		if out, ok := result.(*protocol.SessionNewResult); ok {
			out.SessionID = "new-1"
		}
	case protocol.MethodSessionLoad:
		if out, ok := result.(*protocol.SessionLoadResult); ok {
			out.ConfigOptions = []protocol.ConfigOption{{ID: "mode", CurrentValue: "code"}}
		}
	case protocol.MethodSessionPrompt:
		if out, ok := result.(*protocol.SessionPromptResult); ok {
			out.StopReason = "end_turn"
		}
	}
	return nil
}

func (f *fakeConn) Notify(_ string, _ any) error { return nil }

func (f *fakeConn) OnACPRequest(h ACPRequestHandler) { f.req = h }

func (f *fakeConn) OnACPResponse(h ACPResponseHandler) { f.resp = h }

func (f *fakeConn) Close() error { return nil }

type fakeCallbacks struct {
	updateCount     int
	permissionCount int
}

func (f *fakeCallbacks) SessionUpdate(_ protocol.SessionUpdateParams) {
	f.updateCount++
}

func (f *fakeCallbacks) SessionRequestPermission(_ context.Context, _ protocol.PermissionRequestParams) (protocol.PermissionResult, error) {
	f.permissionCount++
	return protocol.PermissionResult{Outcome: "allow_once", OptionID: "allow"}, nil
}

func (f *fakeCallbacks) FSRead(_ protocol.FSReadTextFileParams) (protocol.FSReadTextFileResult, error) {
	return protocol.FSReadTextFileResult{Content: ""}, nil
}

func (f *fakeCallbacks) FSWrite(_ protocol.FSWriteTextFileParams) error { return nil }

func (f *fakeCallbacks) TerminalCreate(_ protocol.TerminalCreateParams) (protocol.TerminalCreateResult, error) {
	return protocol.TerminalCreateResult{TerminalID: "t-1"}, nil
}

func (f *fakeCallbacks) TerminalOutput(_ protocol.TerminalOutputParams) (protocol.TerminalOutputResult, error) {
	return protocol.TerminalOutputResult{}, nil
}

func (f *fakeCallbacks) TerminalWaitForExit(_ protocol.TerminalWaitForExitParams) (protocol.TerminalWaitForExitResult, error) {
	return protocol.TerminalWaitForExitResult{}, nil
}

func (f *fakeCallbacks) TerminalKill(_ protocol.TerminalKillParams) error { return nil }

func (f *fakeCallbacks) TerminalRelease(_ protocol.TerminalReleaseParams) error { return nil }
