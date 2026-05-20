package hub

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

type fakeSkillsRunner struct {
	mu      sync.Mutex
	calls   []skillsCommandCall
	results map[string]skillsCommandResult
	blocks  map[string]chan struct{}
}

func newFakeSkillsRunner() *fakeSkillsRunner {
	return &fakeSkillsRunner{results: map[string]skillsCommandResult{}, blocks: map[string]chan struct{}{}}
}

func (f *fakeSkillsRunner) Run(_ context.Context, dir string, name string, args ...string) skillsCommandResult {
	key := skillsCallKey(dir, name, args...)
	f.mu.Lock()
	call := skillsCommandCall{Dir: dir, Name: name, Args: append([]string(nil), args...)}
	f.calls = append(f.calls, call)
	result, ok := f.results[key]
	block := f.blocks[key]
	f.mu.Unlock()
	if block != nil {
		<-block
	}
	if ok {
		return result
	}
	return skillsCommandResult{ExitCode: 0, Stdout: "[]"}
}

func (f *fakeSkillsRunner) set(dir string, name string, args []string, result skillsCommandResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[skillsCallKey(dir, name, args...)] = result
}

func (f *fakeSkillsRunner) block(dir string, name string, args ...string) chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan struct{})
	f.blocks[skillsCallKey(dir, name, args...)] = ch
	return ch
}

func (f *fakeSkillsRunner) hasCall(dir string, name string, args ...string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	want := skillsCommandCall{Dir: dir, Name: name, Args: append([]string(nil), args...)}
	for _, call := range f.calls {
		if reflect.DeepEqual(call, want) {
			return true
		}
	}
	return false
}

func skillsCallKey(dir string, name string, args ...string) string {
	return dir + "\x00" + name + "\x00" + strings.Join(args, "\x00")
}

func waitForSkillsCall(t *testing.T, runner *fakeSkillsRunner, dir string, name string, args ...string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if runner.hasCall(dir, name, args...) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("call not found: dir=%q name=%q args=%#v calls=%#v", dir, name, args, runner.calls)
}

func waitForSkillsOperationDone(t *testing.T, cmd *SkillsCommand) *skillsOperationSnapshot {
	t.Helper()
	var operation *skillsOperationSnapshot
	for i := 0; i < 100; i++ {
		operation = cmd.currentOperationSnapshot()
		if operation != nil && !operation.Running {
			return operation
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("operation still running: %#v", operation)
	return operation
}

func assertDirExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", path)
	}
}

func TestSkillsCommandScanReturnsHubAndProjectSkillsWithCategories(t *testing.T) {
	baseDir := t.TempDir()
	projectRoot := filepath.Join(baseDir, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	globalLock := filepath.Join(baseDir, "global-lock.json")
	writeSkillsLockForTest(t, globalLock, map[string]string{"tdd": "mattpocock-skills"})
	writeSkillsLockForTest(t, filepath.Join(projectRoot, "skills-lock.json"), map[string]string{"diagnose": "mattpocock-skills"})

	runner := newFakeSkillsRunner()
	runner.set("", "skills", []string{"list", "-g", "--json"}, skillsCommandResult{
		Stdout:   `[{"name":"tdd","path":"C:/skills/tdd","scope":"global","agents":["Codex"]},{"name":"manual-global","path":"C:/skills/manual-global","scope":"global","agents":["Codex"]}]`,
		ExitCode: 0,
	})
	runner.set(projectRoot, "skills", []string{"list", "--json"}, skillsCommandResult{
		Stdout:   `[{"name":"diagnose","path":"C:/skills/diagnose","scope":"project","agents":["Codex","Claude Code"]},{"name":"manual-project","path":"C:/skills/manual-project","scope":"project","agents":["Codex"]}]`,
		ExitCode: 0,
	})
	cmd := newSkillsCommandWithRunner(runner, skillsCommandConfig{
		HubID:          "hub-a",
		GlobalLockPath: globalLock,
		Projects:       []ProjectInfo{{Name: "WheelMaker", Path: projectRoot, Online: true}},
	})

	resp, cmdErr := cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action": "scan",
		"hubId":  "hub-a",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle scan error: %#v", cmdErr)
	}
	body := resp.(skillsCommandResponse)
	if !body.OK || body.HubID != "hub-a" || body.HubSkills.Scope != "hub" {
		t.Fatalf("response=%#v", body)
	}
	if len(body.HubSkills.Skills) != 2 || body.HubSkills.Skills[0].Name != "tdd" || body.HubSkills.Skills[1].Name != "manual-global" {
		t.Fatalf("hub skills=%#v", body.HubSkills.Skills)
	}
	if body.HubSkills.Skills[0].Category != "Mattpocock Skills" || body.HubSkills.Skills[0].CategoryKey != "mattpocock-skills" {
		t.Fatalf("hub skill category=%#v", body.HubSkills.Skills[0])
	}
	if body.HubSkills.Skills[1].Category != "General" || body.HubSkills.Skills[1].CategoryKey != "general" {
		t.Fatalf("unlocked hub skill category=%#v", body.HubSkills.Skills[1])
	}
	hubManaged := skillManagedFlagsForTest(t, body.HubSkills.Skills)
	if !hubManaged["tdd"] || hubManaged["manual-global"] {
		t.Fatalf("hub managed flags=%#v", hubManaged)
	}
	if len(body.Projects) != 1 || body.Projects[0].ProjectName != "WheelMaker" || len(body.Projects[0].Skills) != 2 {
		t.Fatalf("projects=%#v", body.Projects)
	}
	if body.Projects[0].Skills[0].Name != "diagnose" || body.Projects[0].Skills[0].Category != "Mattpocock Skills" {
		t.Fatalf("project skill=%#v", body.Projects[0].Skills[0])
	}
	projectManaged := skillManagedFlagsForTest(t, body.Projects[0].Skills)
	if !projectManaged["diagnose"] || projectManaged["manual-project"] {
		t.Fatalf("project managed flags=%#v", projectManaged)
	}
}

func TestSkillsCommandListParsesGroupedSourceOutput(t *testing.T) {
	runner := newFakeSkillsRunner()
	runner.set("", "skills", []string{"add", "mattpocock/skills", "--list"}, skillsCommandResult{
		Stdout: `Source: mattpocock/skills

Available Skills

Mattpocock Skills
  tdd
    Practice test-driven development
  diagnose
    Debug with a disciplined loop
`,
		ExitCode: 0,
	})
	cmd := newSkillsCommandWithRunner(runner, skillsCommandConfig{HubID: "hub-a"})

	resp, cmdErr := cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action": "list",
		"hubId":  "hub-a",
		"source": "mattpocock/skills",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle list error: %#v", cmdErr)
	}
	body := resp.(skillsCommandResponse)
	if !body.OK || len(body.Candidates) != 2 {
		t.Fatalf("response=%#v, want two candidates", body)
	}
	if body.Candidates[0].Name != "tdd" || body.Candidates[0].Category != "Mattpocock Skills" || body.Candidates[0].CategoryKey != "mattpocock-skills" {
		t.Fatalf("first candidate=%#v", body.Candidates[0])
	}
	if body.Candidates[1].Name != "diagnose" || !strings.Contains(body.Candidates[1].Description, "Debug") {
		t.Fatalf("second candidate=%#v", body.Candidates[1])
	}
}

func TestSkillsCommandInstallUsesGlobalSymlinkAndProjectCopy(t *testing.T) {
	baseDir := t.TempDir()
	homeRoot := filepath.Join(baseDir, "home")
	projectRoot := filepath.Join(baseDir, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(homeRoot, ".claude"))
	runner := newFakeSkillsRunner()
	runner.set("", "skills", []string{"list", "-g", "--json"}, skillsCommandResult{Stdout: "[]", ExitCode: 0})
	runner.set(projectRoot, "skills", []string{"list", "--json"}, skillsCommandResult{Stdout: "[]", ExitCode: 0})
	cmd := newSkillsCommandWithRunner(runner, skillsCommandConfig{
		HubID:   "hub-a",
		HomeDir: homeRoot,
		Projects: []ProjectInfo{{
			Name:   "WheelMaker",
			Path:   projectRoot,
			Online: true,
		}},
	})

	_, cmdErr := cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action": "install",
		"hubId":  "hub-a",
		"scope":  "hub",
		"source": "mattpocock/skills",
		"skills": []string{"tdd"},
	}))
	if cmdErr != nil {
		t.Fatalf("hub install error: %#v", cmdErr)
	}
	waitForSkillsCall(t, runner, "", "skills", "add", "mattpocock/skills", "-g", "--agent", "codex", "claude-code", "opencode", "github-copilot", "--skill", "tdd", "-y")
	assertDirExists(t, filepath.Join(homeRoot, ".agents", "skills"))
	assertDirExists(t, filepath.Join(homeRoot, ".claude", "skills"))
	if operation := waitForSkillsOperationDone(t, cmd); operation.Status != "succeeded" {
		t.Fatalf("operation=%#v, want succeeded", operation)
	}
	if runner.hasCall("", "skills", "add", "mattpocock/skills", "-g", "--agent", "codex", "claude-code", "opencode", "github-copilot", "--skill", "tdd", "--copy", "-y") {
		t.Fatalf("global install should not request copy mode: %#v", runner.calls)
	}

	_, cmdErr = cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action":      "install",
		"hubId":       "hub-a",
		"scope":       "project",
		"projectName": "WheelMaker",
		"source":      "mattpocock/skills",
		"skills":      []string{"diagnose"},
	}))
	if cmdErr != nil {
		t.Fatalf("project install error: %#v", cmdErr)
	}
	waitForSkillsCall(t, runner, projectRoot, "skills", "add", "mattpocock/skills", "--agent", "codex", "claude-code", "opencode", "github-copilot", "--skill", "diagnose", "--copy", "-y")
	assertDirExists(t, filepath.Join(projectRoot, ".agents", "skills"))
	assertDirExists(t, filepath.Join(projectRoot, ".claude", "skills"))
}

func TestSkillsCommandUninstallRemovesAllLinkedAgents(t *testing.T) {
	runner := newFakeSkillsRunner()
	runner.set("", "skills", []string{"list", "-g", "--json"}, skillsCommandResult{Stdout: "[]", ExitCode: 0})
	cmd := newSkillsCommandWithRunner(runner, skillsCommandConfig{HubID: "hub-a"})

	_, cmdErr := cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action": "uninstall",
		"hubId":  "hub-a",
		"scope":  "hub",
		"skills": []string{"tdd"},
	}))
	if cmdErr != nil {
		t.Fatalf("uninstall error: %#v", cmdErr)
	}
	waitForSkillsCall(t, runner, "", "skills", "remove", "-g", "--skill", "tdd", "--agent", "codex", "claude-code", "opencode", "github-copilot", "-y")
}

func TestSkillsCommandUpdateUsesHubAndProjectScopes(t *testing.T) {
	baseDir := t.TempDir()
	homeRoot := filepath.Join(baseDir, "home")
	projectRoot := filepath.Join(baseDir, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	globalLock := filepath.Join(baseDir, "global-lock.json")
	writeSkillsLockSourcesForTest(t, globalLock, map[string]string{"tdd": "mattpocock/skills"})
	writeSkillsLockSourcesForTest(t, filepath.Join(projectRoot, "skills-lock.json"), map[string]string{"diagnose": "mattpocock/skills"})
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(homeRoot, ".claude"))
	runner := newFakeSkillsRunner()
	runner.set("", "skills", []string{"list", "-g", "--json"}, skillsCommandResult{Stdout: "[]", ExitCode: 0})
	runner.set(projectRoot, "skills", []string{"list", "--json"}, skillsCommandResult{Stdout: "[]", ExitCode: 0})
	cmd := newSkillsCommandWithRunner(runner, skillsCommandConfig{
		HubID:          "hub-a",
		HomeDir:        homeRoot,
		GlobalLockPath: globalLock,
		Projects:       []ProjectInfo{{Name: "WheelMaker", Path: projectRoot, Online: true}},
	})

	_, cmdErr := cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action": "update",
		"hubId":  "hub-a",
		"scope":  "hub",
	}))
	if cmdErr != nil {
		t.Fatalf("hub update error: %#v", cmdErr)
	}
	waitForSkillsCall(t, runner, "", "skills", "add", "mattpocock/skills", "-g", "--agent", "codex", "claude-code", "opencode", "github-copilot", "--skill", "tdd", "-y")
	assertDirExists(t, filepath.Join(homeRoot, ".agents", "skills"))
	assertDirExists(t, filepath.Join(homeRoot, ".claude", "skills"))
	waitForSkillsOperationDone(t, cmd)
	_, cmdErr = cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action":      "update",
		"hubId":       "hub-a",
		"scope":       "project",
		"projectName": "WheelMaker",
	}))
	if cmdErr != nil {
		t.Fatalf("project update error: %#v", cmdErr)
	}
	waitForSkillsCall(t, runner, projectRoot, "skills", "add", "mattpocock/skills", "--agent", "codex", "claude-code", "opencode", "github-copilot", "--skill", "diagnose", "--copy", "-y")
}

func TestSkillsCommandWriteActionsReturnAcceptedOperation(t *testing.T) {
	runner := newFakeSkillsRunner()
	block := runner.block("", "skills", "remove", "-g", "--skill", "tdd", "--agent", "codex", "claude-code", "opencode", "github-copilot", "-y")
	cmd := newSkillsCommandWithRunner(runner, skillsCommandConfig{HubID: "hub-a"})

	resp, cmdErr := cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action": "uninstall",
		"hubId":  "hub-a",
		"scope":  "hub",
		"skills": []string{"tdd"},
	}))
	if cmdErr != nil {
		t.Fatalf("uninstall error: %#v", cmdErr)
	}
	body := resp.(skillsCommandResponse)
	if !body.OK || !body.Accepted || body.Operation == nil || !body.Operation.Running {
		t.Fatalf("response=%#v, want accepted running operation", body)
	}
	if body.Operation.Action != "uninstall" || body.Operation.Scope != "hub" || body.Operation.Status != "running" {
		t.Fatalf("operation=%#v, want uninstall hub running", body.Operation)
	}
	close(block)
	waitForSkillsOperationDone(t, cmd)
}

func TestSkillsCommandRejectsConcurrentWriteOperations(t *testing.T) {
	runner := newFakeSkillsRunner()
	block := runner.block("", "skills", "remove", "-g", "--skill", "tdd", "--agent", "codex", "claude-code", "opencode", "github-copilot", "-y")
	cmd := newSkillsCommandWithRunner(runner, skillsCommandConfig{HubID: "hub-a"})

	_, cmdErr := cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action": "uninstall",
		"hubId":  "hub-a",
		"scope":  "hub",
		"skills": []string{"tdd"},
	}))
	if cmdErr != nil {
		t.Fatalf("first uninstall error: %#v", cmdErr)
	}
	waitForSkillsCall(t, runner, "", "skills", "remove", "-g", "--skill", "tdd", "--agent", "codex", "claude-code", "opencode", "github-copilot", "-y")
	_, cmdErr = cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action": "update",
		"hubId":  "hub-a",
		"scope":  "hub",
	}))
	if cmdErr == nil || cmdErr.Code != rp.CodeConflict {
		t.Fatalf("cmdErr=%#v, want CONFLICT", cmdErr)
	}
	close(block)
	waitForSkillsOperationDone(t, cmd)
}

func TestSkillsCommandUpdateCanIncludeOnlineProjectsInOneOperation(t *testing.T) {
	baseDir := t.TempDir()
	projectRoot := filepath.Join(baseDir, "project")
	globalLock := filepath.Join(baseDir, "global-lock.json")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	writeSkillsLockSourcesForTest(t, globalLock, map[string]string{"tdd": "mattpocock/skills"})
	writeSkillsLockSourcesForTest(t, filepath.Join(projectRoot, "skills-lock.json"), map[string]string{"diagnose": "mattpocock/skills"})
	runner := newFakeSkillsRunner()
	cmd := newSkillsCommandWithRunner(runner, skillsCommandConfig{
		HubID:          "hub-a",
		HomeDir:        filepath.Join(baseDir, "home"),
		GlobalLockPath: globalLock,
		Projects: []ProjectInfo{
			{Name: "WheelMaker", Path: projectRoot, Online: true},
			{Name: "Offline", Path: t.TempDir(), Online: false},
		},
	})

	resp, cmdErr := cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action":          "update",
		"hubId":           "hub-a",
		"scope":           "hub",
		"includeProjects": true,
	}))
	if cmdErr != nil {
		t.Fatalf("update all error: %#v", cmdErr)
	}
	body := resp.(skillsCommandResponse)
	if !body.OK || !body.Accepted || body.Operation == nil || !body.Operation.IncludeProjects {
		t.Fatalf("response=%#v, want accepted includeProjects operation", body)
	}
	waitForSkillsCall(t, runner, "", "skills", "add", "mattpocock/skills", "-g", "--agent", "codex", "claude-code", "opencode", "github-copilot", "--skill", "tdd", "-y")
	waitForSkillsCall(t, runner, projectRoot, "skills", "add", "mattpocock/skills", "--agent", "codex", "claude-code", "opencode", "github-copilot", "--skill", "diagnose", "--copy", "-y")
	operation := waitForSkillsOperationDone(t, cmd)
	if operation.Status != "succeeded" || !strings.Contains(operation.Message, "Updated skills") {
		t.Fatalf("operation=%#v, want succeeded update operation", operation)
	}
}

func TestSkillsCommandRejectsUnsupportedSources(t *testing.T) {
	cmd := newSkillsCommandWithRunner(newFakeSkillsRunner(), skillsCommandConfig{HubID: "hub-a"})
	for _, source := range []string{"../local", "git@github.com:a/b.git", "https://example.com/repo.git"} {
		t.Run(source, func(t *testing.T) {
			_, cmdErr := cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
				"action": "list",
				"hubId":  "hub-a",
				"source": source,
			}))
			if cmdErr == nil || cmdErr.Code != rp.CodeForbidden {
				t.Fatalf("cmdErr=%#v, want FORBIDDEN", cmdErr)
			}
		})
	}
}

func TestSkillsCommandFailureReturnsStructuredSummary(t *testing.T) {
	runner := newFakeSkillsRunner()
	runner.set("", "skills", []string{"add", "mattpocock/skills", "--list"}, skillsCommandResult{
		Stderr:   "first problem\n\n" + strings.Repeat("x", 650),
		ExitCode: 7,
		Err:      errors.New("exit status 7"),
	})
	cmd := newSkillsCommandWithRunner(runner, skillsCommandConfig{HubID: "hub-a"})

	resp, cmdErr := cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action": "list",
		"hubId":  "hub-a",
		"source": "mattpocock/skills",
	}))
	if cmdErr != nil {
		t.Fatalf("list error: %#v", cmdErr)
	}
	body := resp.(skillsCommandResponse)
	if body.OK || !strings.Contains(body.ErrorSummary, "exit code 7") {
		t.Fatalf("response=%#v, want failed summary", body)
	}
	if strings.Contains(body.ErrorSummary, "first problem") {
		t.Fatalf("summary=%q should use last non-empty stderr segment", body.ErrorSummary)
	}
	if len(body.ErrorSummary) > len("exit code 7: ")+500 {
		t.Fatalf("summary length=%d, want truncated segment", len(body.ErrorSummary))
	}
}

func TestSkillsCommandUnknownProjectReturnsNotFound(t *testing.T) {
	cmd := newSkillsCommandWithRunner(newFakeSkillsRunner(), skillsCommandConfig{HubID: "hub-a"})

	_, cmdErr := cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action":      "update",
		"hubId":       "hub-a",
		"scope":       "project",
		"projectName": "Missing",
	}))
	if cmdErr == nil || cmdErr.Code != rp.CodeNotFound {
		t.Fatalf("cmdErr=%#v, want NOT_FOUND", cmdErr)
	}
}

func rawSkillsCommandPayload(t *testing.T, payload map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return raw
}

func writeSkillsLockForTest(t *testing.T, path string, plugins map[string]string) {
	t.Helper()
	type lockSkill struct {
		PluginName string `json:"pluginName"`
	}
	body := struct {
		Version int                  `json:"version"`
		Skills  map[string]lockSkill `json:"skills"`
	}{
		Version: 1,
		Skills:  map[string]lockSkill{},
	}
	for name, pluginName := range plugins {
		body.Skills[name] = lockSkill{PluginName: pluginName}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal lock: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
}

func skillManagedFlagsForTest(t *testing.T, skills []skillsSkillSnapshot) map[string]bool {
	t.Helper()
	raw, err := json.Marshal(skills)
	if err != nil {
		t.Fatalf("marshal skills: %v", err)
	}
	var items []struct {
		Name    string `json:"name"`
		Managed *bool  `json:"managed"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatalf("unmarshal skills: %v", err)
	}
	out := make(map[string]bool, len(items))
	for _, item := range items {
		if item.Managed == nil {
			t.Fatalf("skill %q is missing managed flag in %s", item.Name, string(raw))
		}
		out[item.Name] = *item.Managed
	}
	return out
}

func writeSkillsLockSourcesForTest(t *testing.T, path string, sources map[string]string) {
	t.Helper()
	type lockSkill struct {
		Source     string `json:"source"`
		SourceType string `json:"sourceType"`
	}
	body := struct {
		Version int                  `json:"version"`
		Skills  map[string]lockSkill `json:"skills"`
	}{
		Version: 1,
		Skills:  map[string]lockSkill{},
	}
	for name, source := range sources {
		body.Skills[name] = lockSkill{Source: source, SourceType: "github"}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal lock: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
}
