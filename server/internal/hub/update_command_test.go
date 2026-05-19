package hub

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUpdateCommandQueryWithoutReleaseAllowsPublish(t *testing.T) {
	cmd := newUpdateCommandWithRunner(t.TempDir(), &fakeUpdateRunner{})

	resp, cmdErr := cmd.Handle(context.Background(), rawUpdateCommandPayload(t, map[string]any{
		"action": "query",
		"hubId":  "hub-a",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle query: %v", cmdErr)
	}
	out := resp.(updateCommandResponse)
	if !out.OK || out.Status != "not_published" || !out.CanUpdatePublish {
		t.Fatalf("response=%+v, want not_published and publish allowed", out)
	}
	if out.PendingSignal {
		t.Fatalf("pendingSignal=true, want false")
	}
	if out.Release != nil {
		t.Fatalf("release=%+v, want nil", out.Release)
	}
}

func TestUpdateCommandUpdatePublishWritesFullUpdateSignal(t *testing.T) {
	baseDir := t.TempDir()
	cmd := newUpdateCommandWithRunner(baseDir, &fakeUpdateRunner{})

	resp, cmdErr := cmd.Handle(context.Background(), rawUpdateCommandPayload(t, map[string]any{
		"action": "update-publish",
		"hubId":  "hub-a",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle update-publish: %v", cmdErr)
	}
	out := resp.(updateCommandResponse)
	if !out.OK || !out.Accepted || !out.PendingSignal || out.RequestedAt == "" {
		t.Fatalf("response=%+v, want accepted pending signal", out)
	}
	raw, err := os.ReadFile(filepath.Join(baseDir, "update-now.signal"))
	if err != nil {
		t.Fatalf("read signal: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(raw)), "full-update") {
		t.Fatalf("signal=%q, want full-update marker", string(raw))
	}
}

func TestUpdateCommandQueryFetchesRemoteAndCountsBehind(t *testing.T) {
	baseDir := t.TempDir()
	repoDir := filepath.Join(baseDir, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	writeReleaseManifestForTest(t, baseDir, updateReleaseManifest{
		SchemaVersion: 1,
		Repo:          repoDir,
		Branch:        "main",
		Remote:        "origin",
		SHA:           "local-sha",
		PublishedAt:   "2026-05-19T10:00:00Z",
	})
	runner := &fakeUpdateRunner{
		results: map[string]updateCommandResult{
			"git fetch --prune origin main": {
				ExitCode: 0,
			},
			"git rev-parse origin/main": {
				Stdout:   "remote-sha\n",
				ExitCode: 0,
			},
			"git show -s --format=%cI local-sha": {
				Stdout:   "2026-05-19T08:00:00Z\n",
				ExitCode: 0,
			},
			"git show -s --format=%cI origin/main": {
				Stdout:   "2026-05-19T09:00:00Z\n",
				ExitCode: 0,
			},
			"git rev-list --count local-sha..origin/main": {
				Stdout:   "3\n",
				ExitCode: 0,
			},
			"git rev-list --count origin/main..local-sha": {
				Stdout:   "0\n",
				ExitCode: 0,
			},
			"git status --porcelain": {
				Stdout:   "",
				ExitCode: 0,
			},
		},
	}
	cmd := newUpdateCommandWithRunner(baseDir, runner)

	resp, cmdErr := cmd.Handle(context.Background(), rawUpdateCommandPayload(t, map[string]any{
		"action": "query",
		"hubId":  "hub-a",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle query: %v", cmdErr)
	}
	out := resp.(updateCommandResponse)
	if !out.OK || out.Status != "update_available" {
		t.Fatalf("response=%+v, want update_available", out)
	}
	if out.Git == nil || out.Git.LatestSHA != "remote-sha" || out.Git.BehindCount != 3 || out.Git.AheadCount != 0 {
		t.Fatalf("git=%+v, want remote-sha behind=3 ahead=0", out.Git)
	}
	if out.Git.CurrentCommittedAt != "2026-05-19T08:00:00Z" || out.Git.LatestCommittedAt != "2026-05-19T09:00:00Z" {
		t.Fatalf("git commit times=%+v, want current/latest commit times", out.Git)
	}
	wantCalls := []updateCommandCall{
		{Dir: repoDir, Name: "git", Args: []string{"fetch", "--prune", "origin", "main"}},
		{Dir: repoDir, Name: "git", Args: []string{"rev-parse", "origin/main"}},
		{Dir: repoDir, Name: "git", Args: []string{"show", "-s", "--format=%cI", "local-sha"}},
		{Dir: repoDir, Name: "git", Args: []string{"show", "-s", "--format=%cI", "origin/main"}},
		{Dir: repoDir, Name: "git", Args: []string{"rev-list", "--count", "local-sha..origin/main"}},
		{Dir: repoDir, Name: "git", Args: []string{"rev-list", "--count", "origin/main..local-sha"}},
		{Dir: repoDir, Name: "git", Args: []string{"status", "--porcelain"}},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("calls mismatch\n got: %#v\nwant: %#v", runner.calls, wantCalls)
	}
}

func TestUpdateCommandQueryPendingSignalDoesNotFetch(t *testing.T) {
	baseDir := t.TempDir()
	repoDir := filepath.Join(baseDir, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	writeReleaseManifestForTest(t, baseDir, updateReleaseManifest{
		SchemaVersion: 1,
		Repo:          repoDir,
		Branch:        "main",
		Remote:        "origin",
		SHA:           "local-sha",
		PublishedAt:   "2026-05-19T10:00:00Z",
	})
	if err := os.WriteFile(filepath.Join(baseDir, "update-now.signal"), []byte("full-update\n"), 0o644); err != nil {
		t.Fatalf("write signal: %v", err)
	}
	runner := &fakeUpdateRunner{}
	cmd := newUpdateCommandWithRunner(baseDir, runner)

	resp, cmdErr := cmd.Handle(context.Background(), rawUpdateCommandPayload(t, map[string]any{
		"action": "query",
		"hubId":  "hub-a",
	}))
	if cmdErr != nil {
		t.Fatalf("Handle query: %v", cmdErr)
	}
	out := resp.(updateCommandResponse)
	if !out.OK || out.Status != "update_pending" || !out.PendingSignal {
		t.Fatalf("response=%+v, want update_pending from signal", out)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("pending query should not run git, calls=%#v", runner.calls)
	}
}

func rawUpdateCommandPayload(t *testing.T, payload map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return raw
}

func writeReleaseManifestForTest(t *testing.T, baseDir string, manifest updateReleaseManifest) {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal release: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "release.json"), raw, 0o644); err != nil {
		t.Fatalf("write release: %v", err)
	}
}

type fakeUpdateRunner struct {
	calls   []updateCommandCall
	results map[string]updateCommandResult
}

func (f *fakeUpdateRunner) Run(_ context.Context, dir string, name string, args ...string) updateCommandResult {
	f.calls = append(f.calls, updateCommandCall{Dir: dir, Name: name, Args: append([]string(nil), args...)})
	if f.results == nil {
		return updateCommandResult{ExitCode: 0}
	}
	key := name + " " + strings.Join(args, " ")
	if result, ok := f.results[key]; ok {
		return result
	}
	return updateCommandResult{ExitCode: 1, Stderr: "unexpected command: " + key}
}
