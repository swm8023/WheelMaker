package client

// client_internal_test.go consolidates all white-box (package client) tests and
// the export helpers that expose internals to the external test package.

import (
	"context"
	"strings"
	"testing"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/hub/acp"
	"github.com/swm8023/wheelmaker/internal/hub/agentv2"
	"github.com/swm8023/wheelmaker/internal/hub/im"
)

// ---------------------------------------------------------------------------
// Export helpers (used by package client_test via export_test.go convention)
// ---------------------------------------------------------------------------

// InjectForwarder keeps compatibility with existing tests: it now injects a ready
// ACP connection-backed runtime instance (no legacy Forwarder layer).
func (c *Client) InjectForwarder(conn *acp.Conn, sessionID string) {
	sess := c.activeSession
	c.mu.Lock()
	name := defaultAgentName
	if c.state != nil && strings.TrimSpace(c.state.ActiveAgent) != "" {
		name = c.state.ActiveAgent
	}
	if c.state == nil {
		c.state = defaultProjectState()
		sess.state = c.state
	}
	c.mu.Unlock()

	runtime := agentv2.NewInstance(name, wrapACPConn(conn), sess)
	sess.mu.Lock()
	sess.instance = runtime
	sess.acpSessionID = sessionID
	sess.ready = true
	sess.mu.Unlock()
}

// InjectState replaces the persisted state.
func (c *Client) InjectState(st *ProjectState) {
	c.mu.Lock()
	c.state = st
	c.mu.Unlock()
	c.activeSession.mu.Lock()
	c.activeSession.state = st
	c.activeSession.mu.Unlock()
}

// InjectIMChannel sets the IM bridge over the provided IM channel.
func (c *Client) InjectIMChannel(p im.Channel) {
	c.imBridge = im.NewBridge(p)
	c.activeSession.mu.Lock()
	c.activeSession.imBridge = c.imBridge
	c.activeSession.mu.Unlock()
}

// DefaultState returns a freshly initialised default state.
func DefaultState() *ProjectState {
	return defaultProjectState()
}

// ---------------------------------------------------------------------------
// Keepalive / error-detection helpers
// ---------------------------------------------------------------------------

func TestIsAgentExitError(t *testing.T) {
	cases := []string{
		"acp rpc error -1: agent process exited",
		"io: broken pipe",
		"read tcp ... connection reset by peer",
		"EOF",
	}
	for _, c := range cases {
		if !isAgentExitError(internalTestErr(c)) {
			t.Fatalf("expected agent-exit match for %q", c)
		}
	}
}

func TestIsAgentExitErrorFalse(t *testing.T) {
	if isAgentExitError(internalTestErr("Selected model is at capacity")) {
		t.Fatalf("capacity error must not be treated as process exit")
	}
}

func TestHasSandboxRefreshError(t *testing.T) {
	u := acp.Update{
		Type:    acp.UpdateToolCall,
		Content: "tool failed: windows sandbox: spawn setup refresh",
	}
	if !hasSandboxRefreshError(u) {
		t.Fatal("expected sandbox refresh detection")
	}
}

type internalTestErr string

func (e internalTestErr) Error() string { return string(e) }

// ---------------------------------------------------------------------------
// Permission router
// ---------------------------------------------------------------------------

func TestPermissionRouterDecisionSlotSerializes(t *testing.T) {
	r := newPermissionRouter(&Session{})

	if !r.acquireDecisionSlot(context.Background()) {
		t.Fatal("first acquire failed")
	}

	acquiredSecond := make(chan struct{})
	releaseSecond := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		if !r.acquireDecisionSlot(context.Background()) {
			return
		}
		close(acquiredSecond)
		<-releaseSecond
		r.releaseDecisionSlot()
	}()

	select {
	case <-acquiredSecond:
		t.Fatal("second acquire should block until first release")
	case <-time.After(120 * time.Millisecond):
	}

	r.releaseDecisionSlot()

	select {
	case <-acquiredSecond:
	case <-time.After(2 * time.Second):
		t.Fatal("second acquire did not proceed after first release")
	}

	close(releaseSecond)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second goroutine did not finish")
	}
}

func TestPermissionRouterDecisionSlotRespectsContextCancel(t *testing.T) {
	r := newPermissionRouter(&Session{})

	if !r.acquireDecisionSlot(context.Background()) {
		t.Fatal("first acquire failed")
	}
	defer r.releaseDecisionSlot()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if r.acquireDecisionSlot(ctx) {
		t.Fatal("acquire should fail when context is cancelled")
	}
	if time.Since(start) < 80*time.Millisecond {
		t.Fatal("acquire returned too early; expected to wait for context cancellation")
	}
}

func TestChooseAutoAllowOption(t *testing.T) {
	opts := []acp.PermissionOption{
		{OptionID: "reject", Kind: "reject_once"},
		{OptionID: "allow", Kind: "allow_once"},
	}
	if got := chooseAutoAllowOption(opts); got != "allow" {
		t.Fatalf("chooseAutoAllowOption()=%q, want allow", got)
	}
}

func TestChooseAutoAllowOptionFallbackFirst(t *testing.T) {
	opts := []acp.PermissionOption{
		{OptionID: "abort", Kind: "reject_once"},
		{OptionID: "deny", Kind: "reject_always"},
	}
	if got := chooseAutoAllowOption(opts); got != "" {
		t.Fatalf("chooseAutoAllowOption()=%q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// Help model / config arg resolution
// ---------------------------------------------------------------------------

func TestResolveHelpModel_ExcludesDebugStatusAction(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	c.RegisterAgent("codex", nil)
	c.activeSession.ready = true

	model, err := c.activeSession.resolveHelpModel(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("resolveHelpModel error: %v", err)
	}

	hasDebugStatus := false
	for _, opt := range model.Options {
		if opt.Label == "Project Debug Status" && opt.Command == "/debug" && opt.Value == "" {
			hasDebugStatus = true
		}
	}
	if hasDebugStatus {
		t.Fatalf("help options should not include debug status action: %+v", model.Options)
	}
}

func TestResolveHelpModel_RootHasConfigEntriesAndAgentSubmenu(t *testing.T) {
	c := New(&noopStore{}, nil, "test", "/tmp")
	c.RegisterAgent("codex", nil)
	c.RegisterAgent("claude", nil)
	c.activeSession.ready = true
	c.activeSession.sessionMeta.ConfigOptions = []acp.ConfigOption{
		{
			ID:           "mode",
			CurrentValue: "plan",
			Options: []acp.ConfigOptionValue{
				{Name: "Plan", Value: "plan"},
				{Name: "Run", Value: "run"},
			},
		},
		{
			ID:           "theme",
			CurrentValue: "dark",
			Options: []acp.ConfigOptionValue{
				{Name: "Dark", Value: "dark"},
				{Name: "Light", Value: "light"},
			},
		},
	}

	model, err := c.activeSession.resolveHelpModel(context.Background(), "chat-1")
	if err != nil {
		t.Fatalf("resolveHelpModel error: %v", err)
	}

	hasAgentSwitch := false
	hasModeAtRoot := false
	hasThemeAtRoot := false
	for _, opt := range model.Options {
		switch {
		case opt.Label == "Agent Switch" && strings.TrimSpace(opt.MenuID) != "":
			hasAgentSwitch = true
		case strings.HasPrefix(opt.Label, "Config: mode"):
			hasModeAtRoot = true
		case strings.HasPrefix(opt.Label, "Config: theme"):
			hasThemeAtRoot = true
		}
	}
	if !hasAgentSwitch {
		t.Fatalf("help root menu missing agent switch entry: %+v", model.Options)
	}
	if !hasModeAtRoot || !hasThemeAtRoot {
		t.Fatalf("help root menu missing config entries: %+v", model.Options)
	}
}

func TestResolveConfigArg_ValidatesOptionValue(t *testing.T) {
	st := &SessionState{
		ConfigOptions: []acp.ConfigOption{
			{
				ID: "theme",
				Options: []acp.ConfigOptionValue{
					{Name: "Dark", Value: "dark"},
					{Name: "Light", Value: "light"},
				},
			},
		},
	}

	id, value, err := resolveConfigArg("theme Dark", st)
	if err != nil {
		t.Fatalf("resolveConfigArg returned error: %v", err)
	}
	if id != "theme" || value != "dark" {
		t.Fatalf("resolveConfigArg = (%q,%q), want (%q,%q)", id, value, "theme", "dark")
	}

	if _, _, err := resolveConfigArg("theme blue", st); err == nil {
		t.Fatalf("expected unknown config value error")
	}
}

func TestCanonicalIMBlockType(t *testing.T) {
	cases := map[string]string{
		"tool":                "tool_call",
		"tool_call":           "tool_call",
		"tool_call_update":    "tool_call",
		"tool_call_cancelled": "tool_call",
		"system":              "error",
		"thought":             "thought",
		"  TEXT  ":            "text",
		"":                    "",
	}
	for in, want := range cases {
		if got := canonicalIMBlockType(in); got != want {
			t.Fatalf("canonicalIMBlockType(%q)=%q, want %q", in, got, want)
		}
	}
}

// noopStore is a Store that always returns a default state and discards saves.
type noopStore struct{}

func (s *noopStore) Load() (*ProjectState, error) { return defaultProjectState(), nil }
func (s *noopStore) Save(_ *ProjectState) error   { return nil }
