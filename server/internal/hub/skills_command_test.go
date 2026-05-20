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

	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

type fakeSkillsRunner struct {
	mu      sync.Mutex
	calls   []skillsCommandCall
	results map[string]skillsCommandResult
}

func newFakeSkillsRunner() *fakeSkillsRunner {
	return &fakeSkillsRunner{results: map[string]skillsCommandResult{}}
}

func (f *fakeSkillsRunner) Run(_ context.Context, dir string, name string, args ...string) skillsCommandResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := skillsCommandCall{Dir: dir, Name: name, Args: append([]string(nil), args...)}
	f.calls = append(f.calls, call)
	if result, ok := f.results[skillsCallKey(dir, name, args...)]; ok {
		return result
	}
	return skillsCommandResult{ExitCode: 0, Stdout: "[]"}
}

func (f *fakeSkillsRunner) set(dir string, name string, args []string, result skillsCommandResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[skillsCallKey(dir, name, args...)] = result
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
		Stdout:   `[{"name":"tdd","path":"C:/skills/tdd","scope":"global","agents":["Codex"]}]`,
		ExitCode: 0,
	})
	runner.set(projectRoot, "skills", []string{"list", "--json"}, skillsCommandResult{
		Stdout:   `[{"name":"diagnose","path":"C:/skills/diagnose","scope":"project","agents":["Codex","Claude Code"]}]`,
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
	if len(body.HubSkills.Skills) != 1 || body.HubSkills.Skills[0].Name != "tdd" {
		t.Fatalf("hub skills=%#v", body.HubSkills.Skills)
	}
	if body.HubSkills.Skills[0].Category != "Mattpocock Skills" || body.HubSkills.Skills[0].CategoryKey != "mattpocock-skills" {
		t.Fatalf("hub skill category=%#v", body.HubSkills.Skills[0])
	}
	if len(body.Projects) != 1 || body.Projects[0].ProjectName != "WheelMaker" || len(body.Projects[0].Skills) != 1 {
		t.Fatalf("projects=%#v", body.Projects)
	}
	if body.Projects[0].Skills[0].Name != "diagnose" || body.Projects[0].Skills[0].Category != "Mattpocock Skills" {
		t.Fatalf("project skill=%#v", body.Projects[0].Skills[0])
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

func TestSkillsCommandInstallUsesFixedAgentsAndSymlinkInstall(t *testing.T) {
	projectRoot := t.TempDir()
	runner := newFakeSkillsRunner()
	runner.set("", "skills", []string{"list", "-g", "--json"}, skillsCommandResult{Stdout: "[]", ExitCode: 0})
	runner.set(projectRoot, "skills", []string{"list", "--json"}, skillsCommandResult{Stdout: "[]", ExitCode: 0})
	cmd := newSkillsCommandWithRunner(runner, skillsCommandConfig{
		HubID:    "hub-a",
		Projects: []ProjectInfo{{Name: "WheelMaker", Path: projectRoot, Online: true}},
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
	if !runner.hasCall("", "skills", "add", "mattpocock/skills", "-g", "--agent", "codex", "claude-code", "opencode", "github-copilot", "--skill", "tdd", "-y") {
		t.Fatalf("hub install call not found: %#v", runner.calls)
	}
	if runner.hasCall("", "skills", "add", "mattpocock/skills", "-g", "--copy") {
		t.Fatalf("install should not request copy mode: %#v", runner.calls)
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
	if !runner.hasCall(projectRoot, "skills", "add", "mattpocock/skills", "--agent", "codex", "claude-code", "opencode", "github-copilot", "--skill", "diagnose", "-y") {
		t.Fatalf("project install call not found: %#v", runner.calls)
	}
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
	if !runner.hasCall("", "skills", "remove", "-g", "--skill", "tdd", "--agent", "*", "-y") {
		t.Fatalf("uninstall call not found: %#v", runner.calls)
	}
}

func TestSkillsCommandUpdateUsesHubAndProjectScopes(t *testing.T) {
	projectRoot := t.TempDir()
	runner := newFakeSkillsRunner()
	runner.set("", "skills", []string{"list", "-g", "--json"}, skillsCommandResult{Stdout: "[]", ExitCode: 0})
	runner.set(projectRoot, "skills", []string{"list", "--json"}, skillsCommandResult{Stdout: "[]", ExitCode: 0})
	cmd := newSkillsCommandWithRunner(runner, skillsCommandConfig{
		HubID:    "hub-a",
		Projects: []ProjectInfo{{Name: "WheelMaker", Path: projectRoot, Online: true}},
	})

	_, cmdErr := cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action": "update",
		"hubId":  "hub-a",
		"scope":  "hub",
	}))
	if cmdErr != nil {
		t.Fatalf("hub update error: %#v", cmdErr)
	}
	_, cmdErr = cmd.Handle(context.Background(), rawSkillsCommandPayload(t, map[string]any{
		"action":      "update",
		"hubId":       "hub-a",
		"scope":       "project",
		"projectName": "WheelMaker",
	}))
	if cmdErr != nil {
		t.Fatalf("project update error: %#v", cmdErr)
	}
	if !runner.hasCall("", "skills", "update", "-g", "-y") {
		t.Fatalf("hub update call not found: %#v", runner.calls)
	}
	if !runner.hasCall(projectRoot, "skills", "update", "-p", "-y") {
		t.Fatalf("project update call not found: %#v", runner.calls)
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
