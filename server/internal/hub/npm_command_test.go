package hub

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

type fakeNPMRunner struct {
	mu        sync.Mutex
	calls     []npmCommandCall
	results   map[string]npmCommandResult
	blockKeys map[string]chan struct{}
}

func newFakeNPMRunner() *fakeNPMRunner {
	return &fakeNPMRunner{
		results:   map[string]npmCommandResult{},
		blockKeys: map[string]chan struct{}{},
	}
}

func (f *fakeNPMRunner) Run(ctx context.Context, name string, args ...string) npmCommandResult {
	key := npmCallKey(name, args...)
	f.mu.Lock()
	f.calls = append(f.calls, npmCommandCall{Name: name, Args: append([]string(nil), args...)})
	block := f.blockKeys[key]
	result, ok := f.results[key]
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return npmCommandResult{ExitCode: -1, Err: ctx.Err()}
		}
	}
	if ok {
		return result
	}
	if name == "npm" && len(args) == 3 && args[0] == "view" && args[2] == "version" {
		return npmCommandResult{Stdout: "9.9.9\n", ExitCode: 0}
	}
	return npmCommandResult{Stdout: "", ExitCode: 0}
}

func (f *fakeNPMRunner) set(name string, args []string, result npmCommandResult) {
	f.mu.Lock()
	f.results[npmCallKey(name, args...)] = result
	f.mu.Unlock()
}

func (f *fakeNPMRunner) block(name string, args []string) chan struct{} {
	ch := make(chan struct{})
	f.mu.Lock()
	f.blockKeys[npmCallKey(name, args...)] = ch
	f.mu.Unlock()
	return ch
}

func (f *fakeNPMRunner) hasCall(name string, args ...string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	want := npmCommandCall{Name: name, Args: args}
	for _, call := range f.calls {
		if reflect.DeepEqual(call, want) {
			return true
		}
	}
	return false
}

func npmCallKey(name string, args ...string) string {
	return name + "\x00" + strings.Join(args, "\x00")
}

func TestNPMCommandScanReturnsRuntimeAndDeprecatedPackageRows(t *testing.T) {
	runner := newFakeNPMRunner()
	runner.set("node", []string{"--version"}, npmCommandResult{Stdout: "v22.11.0\n", ExitCode: 0})
	runner.set("npm", []string{"--version"}, npmCommandResult{Stdout: "11.12.1\n", ExitCode: 0})
	runner.set("npm", []string{"prefix", "-g"}, npmCommandResult{Stdout: "C:\\Users\\you\\npm\n", ExitCode: 0})
	runner.set("npm", []string{"list", "-g", "--depth=0", "--json"}, npmCommandResult{
		Stdout:   `{"dependencies":{"@openai/codex":{"version":"0.129.0"},"@zed-industries/claude-agent-acp":{"version":"0.13.0"}}}`,
		ExitCode: 0,
	})
	runner.set("npm", []string{"view", "@openai/codex", "version"}, npmCommandResult{Stdout: "0.130.0\n", ExitCode: 0})

	cmd := newNPMCommandWithRunner(runner)
	resp, cmdErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action": "scan",
		"hubId":  "hub-a",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle scan error: %#v", cmdErr)
	}
	body := resp.(npmCommandResponse)
	if !body.OK || body.Hub.HubID != "hub-a" {
		t.Fatalf("response=%#v", body)
	}
	if body.Hub.NodeVersion != "v22.11.0" || body.Hub.NPMVersion != "11.12.1" || body.Hub.NPMPrefix != "C:\\Users\\you\\npm" {
		t.Fatalf("metadata=%#v", body.Hub)
	}

	codex := findNPMTestPackage(t, body.Hub.Packages, "@openai/codex")
	if codex.Status != "update_available" || codex.InstalledVersion != "0.129.0" || codex.LatestVersion != "0.130.0" {
		t.Fatalf("codex package=%#v", codex)
	}
	if !codex.CanUpdate || codex.CanUninstall {
		t.Fatalf("codex action flags=%#v", codex)
	}

	deprecated := findNPMTestPackage(t, body.Hub.Packages, "@zed-industries/claude-agent-acp")
	if deprecated.Kind != "deprecated" || deprecated.Status != "deprecated" || !deprecated.CanUninstall {
		t.Fatalf("deprecated package=%#v", deprecated)
	}
}

func TestNPMCommandScanListFailureReturnsHubError(t *testing.T) {
	runner := newFakeNPMRunner()
	runner.set("npm", []string{"list", "-g", "--depth=0", "--json"}, npmCommandResult{
		Stderr:   "npm list exploded\n",
		ExitCode: 1,
		Err:      errors.New("exit status 1"),
	})
	cmd := newNPMCommandWithRunner(runner)

	resp, cmdErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action": "scan",
		"hubId":  "hub-a",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle scan error: %#v", cmdErr)
	}
	body := resp.(npmCommandResponse)
	if body.OK || body.Hub.Error == "" || len(body.Hub.Packages) != 0 {
		t.Fatalf("body=%#v, want hub-level scan error with empty packages", body)
	}
}

func TestNPMCommandScanMarksMissingPackageViewFailureAsCheckingFailed(t *testing.T) {
	runner := newFakeNPMRunner()
	runner.set("npm", []string{"list", "-g", "--depth=0", "--json"}, npmCommandResult{
		Stdout:   `{"dependencies":{}}`,
		ExitCode: 0,
	})
	runner.set("npm", []string{"view", "@openai/codex", "version"}, npmCommandResult{
		Stderr:   "registry unavailable\n",
		ExitCode: 1,
		Err:      errors.New("exit status 1"),
	})
	cmd := newNPMCommandWithRunner(runner)

	resp, cmdErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action": "scan",
		"hubId":  "hub-a",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle scan error: %#v", cmdErr)
	}
	pkg := findNPMTestPackage(t, resp.(npmCommandResponse).Hub.Packages, "@openai/codex")
	if pkg.Status != "checking_failed" || !pkg.CanInstall || pkg.Error == "" {
		t.Fatalf("pkg=%#v, want checking_failed installable package with error", pkg)
	}
}

func TestNPMCommandAcceptsRuntimeInstallAndDeprecatedUninstall(t *testing.T) {
	runner := newFakeNPMRunner()
	cmd := newNPMCommandWithRunner(runner)

	installResp, installErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action":      "install",
		"hubId":       "hub-a",
		"packageName": "@openai/codex",
		"version":     "latest",
	}))
	if installErr != nil {
		t.Fatalf("install error: %#v", installErr)
	}
	installBody := installResp.(npmCommandResponse)
	if installBody.Task == nil || installBody.Task.Action != "install" || installBody.Task.PackageName != "@openai/codex" {
		t.Fatalf("install task=%#v", installBody.Task)
	}
	waitForNPMTestTask(t, cmd)
	if !runner.hasCall("npm", "install", "-g", "@openai/codex@latest") {
		t.Fatalf("install call not found: %#v", runner.calls)
	}

	uninstallResp, uninstallErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action":      "uninstall",
		"hubId":       "hub-a",
		"packageName": "@zed-industries/claude-agent-acp",
	}))
	if uninstallErr != nil {
		t.Fatalf("uninstall error: %#v", uninstallErr)
	}
	uninstallBody := uninstallResp.(npmCommandResponse)
	if uninstallBody.Task == nil || uninstallBody.Task.Action != "uninstall" || uninstallBody.Task.PackageName != "@zed-industries/claude-agent-acp" {
		t.Fatalf("uninstall task=%#v", uninstallBody.Task)
	}
	waitForNPMTestTask(t, cmd)
	if !runner.hasCall("npm", "uninstall", "-g", "@zed-industries/claude-agent-acp") {
		t.Fatalf("uninstall call not found: %#v", runner.calls)
	}
}

func TestNPMCommandRejectsUnsupportedPackagePolicy(t *testing.T) {
	cmd := newNPMCommandWithRunner(newFakeNPMRunner())
	cases := []struct {
		name    string
		payload map[string]any
		code    string
	}{
		{
			name: "unsupported install package",
			payload: map[string]any{
				"action":      "install",
				"hubId":       "hub-a",
				"packageName": "left-pad",
			},
			code: rp.CodeForbidden,
		},
		{
			name: "deprecated install package",
			payload: map[string]any{
				"action":      "install",
				"hubId":       "hub-a",
				"packageName": "@zed-industries/claude-agent-acp",
			},
			code: rp.CodeForbidden,
		},
		{
			name: "runtime uninstall package",
			payload: map[string]any{
				"action":      "uninstall",
				"hubId":       "hub-a",
				"packageName": "@openai/codex",
			},
			code: rp.CodeForbidden,
		},
		{
			name: "unsupported version",
			payload: map[string]any{
				"action":      "install",
				"hubId":       "hub-a",
				"packageName": "@openai/codex",
				"version":     "0.130.0",
			},
			code: rp.CodeInvalidArgument,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, cmdErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, tc.payload))
			if cmdErr == nil || cmdErr.Code != tc.code {
				t.Fatalf("cmdErr=%#v, want code %s", cmdErr, tc.code)
			}
		})
	}
}

func TestNPMCommandRejectsConcurrentTasksAndQueryCanReturnNoHistory(t *testing.T) {
	runner := newFakeNPMRunner()
	block := runner.block("npm", []string{"install", "-g", "@openai/codex@latest"})
	cmd := newNPMCommandWithRunner(runner)

	queryResp, queryErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action": "query",
		"hubId":  "hub-a",
	}))
	if queryErr != nil {
		t.Fatalf("query error: %#v", queryErr)
	}
	if queryResp.(npmCommandResponse).Task != nil {
		t.Fatalf("query task=%#v, want nil", queryResp.(npmCommandResponse).Task)
	}

	_, installErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action":      "install",
		"hubId":       "hub-a",
		"packageName": "@openai/codex",
	}))
	if installErr != nil {
		t.Fatalf("first install error: %#v", installErr)
	}
	_, conflictErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action":      "install",
		"hubId":       "hub-a",
		"packageName": "@agentclientprotocol/claude-agent-acp",
	}))
	if conflictErr == nil || conflictErr.Code != rp.CodeConflict {
		t.Fatalf("conflictErr=%#v, want CONFLICT", conflictErr)
	}
	close(block)
	waitForNPMTestTask(t, cmd)
}

func TestNPMCommandFailedTaskSummarizesLastStderrSegment(t *testing.T) {
	longTail := strings.Repeat("x", 650)
	runner := newFakeNPMRunner()
	runner.set("npm", []string{"install", "-g", "@openai/codex@latest"}, npmCommandResult{
		Stderr:   "first failure\n\n" + longTail,
		ExitCode: 7,
		Err:      errors.New("exit status 7"),
	})
	cmd := newNPMCommandWithRunner(runner)

	_, installErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action":      "install",
		"hubId":       "hub-a",
		"packageName": "@openai/codex",
	}))
	if installErr != nil {
		t.Fatalf("install error: %#v", installErr)
	}
	task := waitForNPMTestTask(t, cmd)
	if task.Status != "failed" || task.ExitCode == nil || *task.ExitCode != 7 {
		t.Fatalf("task=%#v, want failed exit 7", task)
	}
	if !strings.Contains(task.ErrorSummary, "exit code 7") {
		t.Fatalf("summary=%q, want exit code", task.ErrorSummary)
	}
	if strings.Contains(task.ErrorSummary, "first failure") {
		t.Fatalf("summary=%q should use last non-empty stderr segment", task.ErrorSummary)
	}
	if len(task.ErrorSummary) > len("exit code 7: ")+500 {
		t.Fatalf("summary length=%d, want truncated to 500 char segment", len(task.ErrorSummary))
	}
}

func rawNPMCommandPayload(t *testing.T, payload map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return raw
}

func findNPMTestPackage(t *testing.T, packages []npmPackageStatus, name string) npmPackageStatus {
	t.Helper()
	for _, pkg := range packages {
		if pkg.PackageName == name {
			return pkg
		}
	}
	t.Fatalf("package %s not found in %#v", name, packages)
	return npmPackageStatus{}
}

func waitForNPMTestTask(t *testing.T, cmd *NPMCommand) *npmTaskSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, cmdErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
			"action": "query",
			"hubId":  "hub-a",
		}))
		if cmdErr != nil {
			t.Fatalf("query task: %#v", cmdErr)
		}
		task := resp.(npmCommandResponse).Task
		if task != nil && !task.Running {
			return task
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("task did not finish")
	return nil
}
