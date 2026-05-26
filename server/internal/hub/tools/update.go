package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

const (
	updateSignalFileName  = "update-now.signal"
	releaseManifestName   = "release.json"
	fullUpdateSignalToken = "full-update"
)

type updateCommandCall struct {
	Dir  string
	Name string
	Args []string
}

type updateCommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

type updateCommandRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) updateCommandResult
}

type execUpdateCommandRunner struct{}

func (execUpdateCommandRunner) Run(ctx context.Context, dir string, name string, args ...string) updateCommandResult {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	return updateCommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Err:      err,
	}
}

type UpdateCommand struct {
	baseDir string
	runner  updateCommandRunner
	now     func() time.Time
}

func NewUpdateCommand(baseDir string) *UpdateCommand {
	return newUpdateCommandWithRunner(baseDir, execUpdateCommandRunner{})
}

func newUpdateCommandWithRunner(baseDir string, runner updateCommandRunner) *UpdateCommand {
	if runner == nil {
		runner = execUpdateCommandRunner{}
	}
	return &UpdateCommand{
		baseDir: strings.TrimSpace(baseDir),
		runner:  runner,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

type updateCommandPayload struct {
	Action string `json:"action"`
	HubID  string `json:"hubId"`
	Force  bool   `json:"force,omitempty"`
}

type updateReleaseManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	Repo          string `json:"repo"`
	Branch        string `json:"branch"`
	Remote        string `json:"remote"`
	SHA           string `json:"sha"`
	PublishedAt   string `json:"publishedAt"`
}

type updateGitSnapshot struct {
	Branch             string `json:"branch"`
	Remote             string `json:"remote"`
	CurrentSHA         string `json:"currentSha"`
	LatestSHA          string `json:"latestSha"`
	CurrentCommittedAt string `json:"currentCommittedAt,omitempty"`
	LatestCommittedAt  string `json:"latestCommittedAt,omitempty"`
	BehindCount        int    `json:"behindCount"`
	AheadCount         int    `json:"aheadCount"`
	Dirty              bool   `json:"dirty"`
}

type updateCommandResponse struct {
	OK               bool                   `json:"ok"`
	Accepted         bool                   `json:"accepted,omitempty"`
	RequestedAt      string                 `json:"requestedAt,omitempty"`
	Status           string                 `json:"status"`
	HubID            string                 `json:"hubId"`
	Release          *updateReleaseManifest `json:"release,omitempty"`
	Git              *updateGitSnapshot     `json:"git,omitempty"`
	PendingSignal    bool                   `json:"pendingSignal"`
	CanUpdatePublish bool                   `json:"canUpdatePublish"`
	Error            string                 `json:"error,omitempty"`
}

type updateCommandError struct {
	Code    string
	Message string
}

func (e *updateCommandError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

func (e *updateCommandError) commandCode() string {
	if e == nil {
		return ""
	}
	return e.Code
}

func (e *updateCommandError) commandMessage() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (c *UpdateCommand) Handle(ctx context.Context, raw json.RawMessage) (any, *updateCommandError) {
	var payload updateCommandPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, &updateCommandError{Code: rp.CodeInvalidArgument, Message: "invalid cmd.update payload"}
	}
	payload.Action = strings.TrimSpace(payload.Action)
	payload.HubID = strings.TrimSpace(payload.HubID)
	if payload.HubID == "" {
		return nil, &updateCommandError{Code: rp.CodeInvalidArgument, Message: "hubId is required"}
	}
	switch payload.Action {
	case "query":
		return c.query(ctx, payload.HubID, payload.Force), nil
	case "update-publish":
		resp, err := c.requestUpdatePublish(payload.HubID)
		if err != nil {
			return nil, err
		}
		return resp, nil
	default:
		return nil, &updateCommandError{Code: rp.CodeInvalidArgument, Message: "unsupported cmd.update action"}
	}
}

func (c *UpdateCommand) query(ctx context.Context, hubID string, force bool) updateCommandResponse {
	pendingSignal := fileExists(filepath.Join(c.baseDir, updateSignalFileName))
	release, err := c.readReleaseManifest()
	if pendingSignal {
		return updateCommandResponse{
			OK:               true,
			Status:           "update_pending",
			HubID:            hubID,
			Release:          release,
			PendingSignal:    true,
			CanUpdatePublish: true,
		}
	}
	if err != nil {
		status := "checking_failed"
		if os.IsNotExist(err) {
			status = "not_published"
			err = nil
		}
		return updateCommandResponse{
			OK:               err == nil,
			Status:           status,
			HubID:            hubID,
			Release:          release,
			PendingSignal:    pendingSignal,
			CanUpdatePublish: true,
			Error:            errorString(err),
		}
	}
	resp := c.queryGit(ctx, release, force)
	resp.HubID = hubID
	resp.Release = release
	resp.PendingSignal = false
	resp.CanUpdatePublish = true
	return resp
}

func (c *UpdateCommand) requestUpdatePublish(hubID string) (updateCommandResponse, *updateCommandError) {
	signalPath := filepath.Join(c.baseDir, updateSignalFileName)
	if err := os.MkdirAll(filepath.Dir(signalPath), 0o755); err != nil {
		return updateCommandResponse{}, &updateCommandError{Code: rp.CodeInternal, Message: err.Error()}
	}
	requestedAt := c.now().Format(time.RFC3339)
	payload := fullUpdateSignalToken + "\n" + requestedAt
	if err := os.WriteFile(signalPath, []byte(payload), 0o644); err != nil {
		return updateCommandResponse{}, &updateCommandError{Code: rp.CodeInternal, Message: err.Error()}
	}
	return updateCommandResponse{
		OK:               true,
		Accepted:         true,
		RequestedAt:      requestedAt,
		Status:           "update_pending",
		HubID:            hubID,
		PendingSignal:    true,
		CanUpdatePublish: true,
	}, nil
}

func (c *UpdateCommand) readReleaseManifest() (*updateReleaseManifest, error) {
	raw, err := os.ReadFile(filepath.Join(c.baseDir, releaseManifestName))
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	var manifest updateReleaseManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("release manifest is invalid: %w", err)
	}
	manifest.Repo = strings.TrimSpace(manifest.Repo)
	manifest.Branch = strings.TrimSpace(manifest.Branch)
	manifest.Remote = strings.TrimSpace(manifest.Remote)
	manifest.SHA = strings.TrimSpace(manifest.SHA)
	if manifest.Remote == "" {
		manifest.Remote = "origin"
	}
	if manifest.Repo == "" || manifest.SHA == "" || manifest.Branch == "" {
		return &manifest, fmt.Errorf("release manifest is missing repo, branch, or sha")
	}
	if info, err := os.Stat(filepath.Join(manifest.Repo, ".git")); err != nil || !info.IsDir() {
		return &manifest, fmt.Errorf("release repo path is invalid")
	}
	return &manifest, nil
}

func (c *UpdateCommand) queryGit(ctx context.Context, release *updateReleaseManifest, force bool) updateCommandResponse {
	ref := release.Remote + "/" + release.Branch
	git := &updateGitSnapshot{
		Branch:     release.Branch,
		Remote:     release.Remote,
		CurrentSHA: release.SHA,
	}
	if force {
		if result := c.runGit(ctx, release.Repo, "fetch", "--prune", release.Remote, release.Branch); updateCommandFailed(result) {
			return updateCommandResponse{OK: false, Status: "checking_failed", Git: git, Error: updateResultSummary(result), CanUpdatePublish: true}
		}
	}
	latest, err := c.gitOutput(ctx, release.Repo, "rev-parse", ref)
	if err != nil {
		return updateCommandResponse{OK: false, Status: "checking_failed", Git: git, Error: err.Error(), CanUpdatePublish: true}
	}
	git.LatestSHA = latest
	if currentCommittedAt, err := c.gitOutput(ctx, release.Repo, "show", "-s", "--format=%cI", release.SHA); err == nil {
		git.CurrentCommittedAt = currentCommittedAt
	}
	if latestCommittedAt, err := c.gitOutput(ctx, release.Repo, "show", "-s", "--format=%cI", ref); err == nil {
		git.LatestCommittedAt = latestCommittedAt
	}
	behind, err := c.gitCount(ctx, release.Repo, release.SHA+".."+ref)
	if err != nil {
		return updateCommandResponse{OK: false, Status: "checking_failed", Git: git, Error: err.Error(), CanUpdatePublish: true}
	}
	ahead, err := c.gitCount(ctx, release.Repo, ref+".."+release.SHA)
	if err != nil {
		return updateCommandResponse{OK: false, Status: "checking_failed", Git: git, Error: err.Error(), CanUpdatePublish: true}
	}
	git.BehindCount = behind
	git.AheadCount = ahead
	if status, err := c.gitOutput(ctx, release.Repo, "status", "--porcelain"); err == nil {
		git.Dirty = strings.TrimSpace(status) != ""
	}
	return updateCommandResponse{
		OK:               true,
		Status:           updateStatusFromCounts(ahead, behind),
		Git:              git,
		CanUpdatePublish: true,
	}
}

func (c *UpdateCommand) runGit(ctx context.Context, repo string, args ...string) updateCommandResult {
	return c.runner.Run(ctx, repo, "git", args...)
}

func (c *UpdateCommand) gitOutput(ctx context.Context, repo string, args ...string) (string, error) {
	result := c.runGit(ctx, repo, args...)
	if result.Err != nil || result.ExitCode != 0 {
		return "", errors.New(updateResultSummary(result))
	}
	return firstNonEmptyLine(result.Stdout), nil
}

func (c *UpdateCommand) gitCount(ctx context.Context, repo string, revRange string) (int, error) {
	raw, err := c.gitOutput(ctx, repo, "rev-list", "--count", revRange)
	if err != nil {
		return 0, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid git count: %s", raw)
	}
	return count, nil
}

func updateStatusFromCounts(ahead int, behind int) string {
	switch {
	case ahead > 0 && behind > 0:
		return "diverged"
	case ahead > 0:
		return "ahead_of_remote"
	case behind > 0:
		return "update_available"
	default:
		return "up_to_date"
	}
}

func updateResultSummary(result updateCommandResult) string {
	segment := lastNonEmptySegment(result.Stderr)
	if segment == "" {
		segment = lastNonEmptySegment(result.Stdout)
	}
	if segment == "" {
		return fmt.Sprintf("git command failed with exit code %d", result.ExitCode)
	}
	return fmt.Sprintf("exit code %d: %s", result.ExitCode, truncateRunes(segment, 500))
}

func updateCommandFailed(result updateCommandResult) bool {
	return result.Err != nil || result.ExitCode != 0
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
