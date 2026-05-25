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
	if body.Operation == nil || body.Operation.Action != "scan_latest" || !body.Operation.Running {
		t.Fatalf("operation=%#v, want running scan_latest", body.Operation)
	}
	if runner.hasCall("node", "--version") || runner.hasCall("npm", "--version") || runner.hasCall("npm", "prefix", "-g") {
		t.Fatalf("scan should not call hidden metadata commands: %#v", runner.calls)
	}

	codex := findNPMTestPackage(t, body.Hub.Packages, "@openai/codex")
	if codex.Status != "checking_latest" || codex.InstalledVersion != "0.129.0" || codex.LatestVersion != "" {
		t.Fatalf("codex package before latest=%#v", codex)
	}
	if !reflect.DeepEqual(codex.AgentTypes, []string{"codex"}) {
		t.Fatalf("@openai/codex agentTypes=%v, want [codex]", codex.AgentTypes)
	}
	if codex.CanInstall || codex.CanUpdate || codex.CanUninstall {
		t.Fatalf("codex action flags should be disabled while latest is checking: %#v", codex)
	}

	waitForNPMTestOperation(t, cmd)
	resp, cmdErr = cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action": "scan",
		"hubId":  "hub-a",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle second scan error: %#v", cmdErr)
	}
	codex = findNPMTestPackage(t, resp.(npmCommandResponse).Hub.Packages, "@openai/codex")
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

func TestNPMCommandScanDeprecatedCodexACPUsesEmptyAgentTypes(t *testing.T) {
	runner := newFakeNPMRunner()
	runner.set("npm", []string{"list", "-g", "--depth=0", "--json"}, npmCommandResult{
		Stdout:   `{"dependencies":{"@zed-industries/codex-acp":{"version":"0.1.0"}}}`,
		ExitCode: 0,
	})

	cmd := newNPMCommandWithRunner(runner)
	resp, cmdErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action": "scan",
		"hubId":  "hub-a",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle scan error: %#v", cmdErr)
	}
	deprecatedCodex := findNPMTestPackage(t, resp.(npmCommandResponse).Hub.Packages, "@zed-industries/codex-acp")
	if deprecatedCodex.AgentTypes == nil {
		t.Fatalf("deprecated codex-acp agentTypes is nil; UI requires an empty array")
	}
	if len(deprecatedCodex.AgentTypes) != 0 {
		t.Fatalf("deprecated codex-acp agentTypes=%v, want empty", deprecatedCodex.AgentTypes)
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
	if pkg.Status != "checking_latest" || pkg.CanInstall || pkg.CanUpdate || pkg.Error != "" {
		t.Fatalf("pkg=%#v, want checking_latest package without actions while latest query runs", pkg)
	}
	waitForNPMTestOperation(t, cmd)
	resp, cmdErr = cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action": "scan",
		"hubId":  "hub-a",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle second scan error: %#v", cmdErr)
	}
	pkg = findNPMTestPackage(t, resp.(npmCommandResponse).Hub.Packages, "@openai/codex")
	if pkg.Status != "latest_unknown" || pkg.CanInstall || pkg.CanUpdate || pkg.Error == "" {
		t.Fatalf("pkg=%#v, want latest_unknown package without actions after latest failure", pkg)
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
	if installBody.Operation == nil || installBody.Operation.Action != "install" || installBody.Operation.PackageName != "@openai/codex" {
		t.Fatalf("install operation=%#v", installBody.Operation)
	}
	waitForNPMTestOperation(t, cmd)
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
	if uninstallBody.Operation == nil || uninstallBody.Operation.Action != "uninstall" || uninstallBody.Operation.PackageName != "@zed-industries/claude-agent-acp" {
		t.Fatalf("uninstall operation=%#v", uninstallBody.Operation)
	}
	waitForNPMTestOperation(t, cmd)
	if !runner.hasCall("npm", "uninstall", "-g", "@zed-industries/claude-agent-acp") {
		t.Fatalf("uninstall call not found: %#v", runner.calls)
	}
}

func TestNPMCommandAcceptsBulkRuntimeInstallAsSingleOperation(t *testing.T) {
	runner := newFakeNPMRunner()
	cmd := newNPMCommandWithRunner(runner)

	resp, cmdErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action":       "install_many",
		"hubId":        "hub-a",
		"packageNames": []string{"@openai/codex", "@anthropic-ai/claude-code"},
		"version":      "latest",
	}))
	if cmdErr != nil {
		t.Fatalf("bulk install error: %#v", cmdErr)
	}
	body := resp.(npmCommandResponse)
	if !body.Accepted || body.Operation == nil || body.Operation.Action != "install_many" {
		t.Fatalf("response=%#v, want accepted install_many operation", body)
	}
	if !reflect.DeepEqual(body.Operation.PackageNames, []string{"@openai/codex", "@anthropic-ai/claude-code"}) {
		t.Fatalf("package names=%#v", body.Operation.PackageNames)
	}

	operation := waitForNPMTestOperation(t, cmd)
	if operation.Status != "succeeded" || !strings.Contains(operation.Message, "Installed 2 npm packages") {
		t.Fatalf("operation=%#v, want succeeded bulk install", operation)
	}
	if !runner.hasCall("npm", "install", "-g", "@openai/codex@latest") {
		t.Fatalf("codex install call not found: %#v", runner.calls)
	}
	if !runner.hasCall("npm", "install", "-g", "@anthropic-ai/claude-code@latest") {
		t.Fatalf("claude install call not found: %#v", runner.calls)
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

func TestNPMCommandRejectsConcurrentOperationsAndQueryIsUnsupported(t *testing.T) {
	runner := newFakeNPMRunner()
	block := runner.block("npm", []string{"install", "-g", "@openai/codex@latest"})
	cmd := newNPMCommandWithRunner(runner)

	_, queryErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
		"action": "query",
		"hubId":  "hub-a",
	}))
	if queryErr == nil || queryErr.Code != rp.CodeInvalidArgument {
		t.Fatalf("queryErr=%#v, want INVALID_ARGUMENT", queryErr)
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
	waitForNPMTestOperation(t, cmd)
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
	operation := waitForNPMTestOperation(t, cmd)
	if operation.Status != "failed" || operation.ExitCode == nil || *operation.ExitCode != 7 {
		t.Fatalf("operation=%#v, want failed exit 7", operation)
	}
	if !strings.Contains(operation.ErrorSummary, "exit code 7") {
		t.Fatalf("summary=%q, want exit code", operation.ErrorSummary)
	}
	if strings.Contains(operation.ErrorSummary, "first failure") {
		t.Fatalf("summary=%q should use last non-empty stderr segment", operation.ErrorSummary)
	}
	if len(operation.ErrorSummary) > len("exit code 7: ")+500 {
		t.Fatalf("summary length=%d, want truncated to 500 char segment", len(operation.ErrorSummary))
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

func waitForNPMTestOperation(t *testing.T, cmd *NPMCommand) *npmOperationSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, cmdErr := cmd.Handle(context.Background(), rawNPMCommandPayload(t, map[string]any{
			"action": "scan",
			"hubId":  "hub-a",
		}))
		if cmdErr != nil {
			t.Fatalf("scan operation: %#v", cmdErr)
		}
		operation := resp.(npmCommandResponse).Operation
		if operation != nil && !operation.Running {
			return operation
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("operation did not finish")
	return nil
}
