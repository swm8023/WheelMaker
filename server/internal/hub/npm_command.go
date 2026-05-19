package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

type npmCommandCall struct {
	Name string
	Args []string
}

type npmCommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

type npmCommandRunner interface {
	Run(ctx context.Context, name string, args ...string) npmCommandResult
}

type execNPMCommandRunner struct{}

func (execNPMCommandRunner) Run(ctx context.Context, name string, args ...string) npmCommandResult {
	cmd := exec.CommandContext(ctx, name, args...)
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
	return npmCommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Err:      err,
	}
}

type NPMCommand struct {
	runner npmCommandRunner
	now    func() time.Time

	mu   sync.Mutex
	task *npmTaskSnapshot
}

func NewNPMCommand() *NPMCommand {
	return newNPMCommandWithRunner(execNPMCommandRunner{})
}

func newNPMCommandWithRunner(runner npmCommandRunner) *NPMCommand {
	if runner == nil {
		runner = execNPMCommandRunner{}
	}
	return &NPMCommand{
		runner: runner,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

type npmCommandPayload struct {
	Action      string `json:"action"`
	HubID       string `json:"hubId"`
	PackageName string `json:"packageName,omitempty"`
	Version     string `json:"version,omitempty"`
}

type npmCommandResponse struct {
	OK        bool             `json:"ok"`
	Accepted  bool             `json:"accepted,omitempty"`
	UpdatedAt string           `json:"updatedAt,omitempty"`
	Hub       npmHubSnapshot   `json:"hub,omitempty"`
	Task      *npmTaskSnapshot `json:"task"`
}

type npmHubSnapshot struct {
	HubID       string             `json:"hubId"`
	Online      bool               `json:"online"`
	NodeVersion string             `json:"nodeVersion"`
	NPMVersion  string             `json:"npmVersion"`
	NPMPrefix   string             `json:"npmPrefix"`
	Warning     string             `json:"warning"`
	Error       string             `json:"error"`
	Packages    []npmPackageStatus `json:"packages"`
}

type npmPackageStatus struct {
	PackageName      string   `json:"packageName"`
	DisplayName      string   `json:"displayName"`
	AgentTypes       []string `json:"agentTypes"`
	Kind             string   `json:"kind"`
	Installed        bool     `json:"installed"`
	InstalledVersion string   `json:"installedVersion"`
	LatestVersion    string   `json:"latestVersion"`
	Status           string   `json:"status"`
	Error            string   `json:"error"`
	CanInstall       bool     `json:"canInstall"`
	CanUpdate        bool     `json:"canUpdate"`
	CanUninstall     bool     `json:"canUninstall"`
}

type npmTaskSnapshot struct {
	Running      bool   `json:"running"`
	Action       string `json:"action"`
	PackageName  string `json:"packageName"`
	Version      string `json:"version"`
	Status       string `json:"status"`
	StartedAt    string `json:"startedAt"`
	FinishedAt   string `json:"finishedAt"`
	ExitCode     *int   `json:"exitCode"`
	ErrorSummary string `json:"errorSummary"`
	Message      string `json:"message,omitempty"`
}

type npmCommandError struct {
	Code    string
	Message string
}

func (e *npmCommandError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type npmPackagePolicy struct {
	PackageName string
	DisplayName string
	AgentTypes  []string
	Kind        string
}

var runtimeNPMPackages = []npmPackagePolicy{
	{PackageName: "@zed-industries/codex-acp", DisplayName: "Codex ACP", AgentTypes: []string{"codex"}, Kind: "runtime"},
	{PackageName: "@agentclientprotocol/claude-agent-acp", DisplayName: "Claude ACP", AgentTypes: []string{"claude"}, Kind: "runtime"},
	{PackageName: "@anthropic-ai/claude-code", DisplayName: "Claude CLI", AgentTypes: []string{"claude"}, Kind: "runtime"},
	{PackageName: "@openai/codex", DisplayName: "Codex CLI", AgentTypes: []string{"codexapp"}, Kind: "runtime"},
	{PackageName: "@github/copilot", DisplayName: "Copilot CLI", AgentTypes: []string{"copilot"}, Kind: "runtime"},
	{PackageName: "opencode-ai", DisplayName: "OpenCode CLI", AgentTypes: []string{"opencode"}, Kind: "runtime"},
}

var deprecatedNPMPackages = []npmPackagePolicy{
	{PackageName: "@zed-industries/claude-agent-acp", DisplayName: "Deprecated Claude ACP", AgentTypes: []string{"claude"}, Kind: "deprecated"},
}

func (c *NPMCommand) Handle(ctx context.Context, raw json.RawMessage) (any, *npmCommandError) {
	var payload npmCommandPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, &npmCommandError{Code: rp.CodeInvalidArgument, Message: "invalid cmd.npm payload"}
	}
	payload.Action = strings.TrimSpace(payload.Action)
	payload.HubID = strings.TrimSpace(payload.HubID)
	payload.PackageName = strings.TrimSpace(payload.PackageName)
	payload.Version = strings.TrimSpace(payload.Version)
	if payload.HubID == "" {
		return nil, &npmCommandError{Code: rp.CodeInvalidArgument, Message: "hubId is required"}
	}

	switch payload.Action {
	case "scan":
		return c.scan(ctx, payload.HubID), nil
	case "query":
		return npmCommandResponse{OK: true, Task: c.currentTaskSnapshot()}, nil
	case "install":
		return c.startInstall(payload)
	case "uninstall":
		return c.startUninstall(payload)
	default:
		return nil, &npmCommandError{Code: rp.CodeInvalidArgument, Message: "unsupported cmd.npm action"}
	}
}

func (c *NPMCommand) scan(ctx context.Context, hubID string) npmCommandResponse {
	updatedAt := c.now().Format(time.RFC3339)
	hub := npmHubSnapshot{
		HubID:    hubID,
		Online:   true,
		Packages: []npmPackageStatus{},
	}
	var warnings []string

	if version, warning := c.readCommandLine(ctx, "node", "--version"); warning != "" {
		warnings = append(warnings, "node --version: "+warning)
	} else {
		hub.NodeVersion = version
	}
	if version, warning := c.readCommandLine(ctx, "npm", "--version"); warning != "" {
		warnings = append(warnings, "npm --version: "+warning)
	} else {
		hub.NPMVersion = version
	}
	if prefix, warning := c.readCommandLine(ctx, "npm", "prefix", "-g"); warning != "" {
		warnings = append(warnings, "npm prefix -g: "+warning)
	} else {
		hub.NPMPrefix = prefix
	}

	listResult := c.runner.Run(ctx, "npm", "list", "-g", "--depth=0", "--json")
	if commandFailed(listResult) {
		hub.Warning = strings.Join(warnings, "; ")
		hub.Error = "npm list failed: " + npmResultSummary(listResult)
		return npmCommandResponse{OK: false, UpdatedAt: updatedAt, Hub: hub, Task: c.currentTaskSnapshot()}
	}
	installed, err := parseNPMListDependencies(listResult.Stdout)
	if err != nil {
		hub.Warning = strings.Join(warnings, "; ")
		hub.Error = "npm list failed: " + err.Error()
		return npmCommandResponse{OK: false, UpdatedAt: updatedAt, Hub: hub, Task: c.currentTaskSnapshot()}
	}

	latest := c.lookupLatestVersions(ctx)
	for _, policy := range runtimeNPMPackages {
		installedVersion := installed[policy.PackageName]
		latestResult := latest[policy.PackageName]
		row := npmPackageStatus{
			PackageName:      policy.PackageName,
			DisplayName:      policy.DisplayName,
			AgentTypes:       append([]string(nil), policy.AgentTypes...),
			Kind:             policy.Kind,
			Installed:        installedVersion != "",
			InstalledVersion: installedVersion,
			LatestVersion:    latestResult.version,
			Error:            latestResult.errorSummary,
			CanUninstall:     false,
		}
		row.Status, row.CanInstall, row.CanUpdate = runtimePackageStatus(row.Installed, row.InstalledVersion, row.LatestVersion, row.Error)
		hub.Packages = append(hub.Packages, row)
	}
	for _, policy := range deprecatedNPMPackages {
		installedVersion := installed[policy.PackageName]
		if installedVersion == "" {
			continue
		}
		hub.Packages = append(hub.Packages, npmPackageStatus{
			PackageName:      policy.PackageName,
			DisplayName:      policy.DisplayName,
			AgentTypes:       append([]string(nil), policy.AgentTypes...),
			Kind:             policy.Kind,
			Installed:        true,
			InstalledVersion: installedVersion,
			Status:           "deprecated",
			CanUninstall:     true,
		})
	}
	hub.Warning = strings.Join(warnings, "; ")
	return npmCommandResponse{OK: true, UpdatedAt: updatedAt, Hub: hub, Task: c.currentTaskSnapshot()}
}

func (c *NPMCommand) readCommandLine(ctx context.Context, name string, args ...string) (string, string) {
	result := c.runner.Run(ctx, name, args...)
	if commandFailed(result) {
		return "", npmResultSummary(result)
	}
	return firstNonEmptyLine(result.Stdout), ""
}

type npmLatestResult struct {
	version      string
	errorSummary string
}

func (c *NPMCommand) lookupLatestVersions(ctx context.Context) map[string]npmLatestResult {
	out := make(map[string]npmLatestResult, len(runtimeNPMPackages))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, policy := range runtimeNPMPackages {
		policy := policy
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := c.runner.Run(ctx, "npm", "view", policy.PackageName, "version")
			next := npmLatestResult{version: firstNonEmptyLine(result.Stdout)}
			if commandFailed(result) || next.version == "" {
				next.version = ""
				next.errorSummary = npmResultSummary(result)
			}
			mu.Lock()
			out[policy.PackageName] = next
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

func (c *NPMCommand) startInstall(payload npmCommandPayload) (any, *npmCommandError) {
	if payload.PackageName == "" {
		return nil, &npmCommandError{Code: rp.CodeInvalidArgument, Message: "packageName is required"}
	}
	version := payload.Version
	if version == "" {
		version = "latest"
	}
	if version != "latest" {
		return nil, &npmCommandError{Code: rp.CodeInvalidArgument, Message: "version must be latest"}
	}
	if !runtimePackageAllowed(payload.PackageName) {
		return nil, &npmCommandError{Code: rp.CodeForbidden, Message: "package is not installable"}
	}
	task, cmdErr := c.acceptTask("install", payload.PackageName, version)
	if cmdErr != nil {
		return nil, cmdErr
	}
	go c.runTask(task, "npm", "install", "-g", payload.PackageName+"@"+version)
	return npmCommandResponse{OK: true, Accepted: true, Task: cloneNPMTask(task)}, nil
}

func (c *NPMCommand) startUninstall(payload npmCommandPayload) (any, *npmCommandError) {
	if payload.PackageName == "" {
		return nil, &npmCommandError{Code: rp.CodeInvalidArgument, Message: "packageName is required"}
	}
	if !deprecatedPackageAllowed(payload.PackageName) {
		return nil, &npmCommandError{Code: rp.CodeForbidden, Message: "package is not uninstallable"}
	}
	task, cmdErr := c.acceptTask("uninstall", payload.PackageName, "")
	if cmdErr != nil {
		return nil, cmdErr
	}
	go c.runTask(task, "npm", "uninstall", "-g", payload.PackageName)
	return npmCommandResponse{OK: true, Accepted: true, Task: cloneNPMTask(task)}, nil
}

func (c *NPMCommand) acceptTask(action string, packageName string, version string) (*npmTaskSnapshot, *npmCommandError) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.task != nil && c.task.Running {
		return nil, &npmCommandError{Code: rp.CodeConflict, Message: "npm task already running"}
	}
	task := &npmTaskSnapshot{
		Running:     true,
		Action:      action,
		PackageName: packageName,
		Version:     version,
		Status:      "running",
		StartedAt:   c.now().Format(time.RFC3339),
	}
	c.task = task
	return task, nil
}

func (c *NPMCommand) runTask(task *npmTaskSnapshot, name string, args ...string) {
	result := c.runner.Run(context.Background(), name, args...)
	exitCode := result.ExitCode
	finishedAt := c.now().Format(time.RFC3339)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.task != task {
		return
	}
	task.Running = false
	task.FinishedAt = finishedAt
	task.ExitCode = &exitCode
	if commandFailed(result) {
		task.Status = "failed"
		task.ErrorSummary = formatNPMTaskErrorSummary(exitCode, result.Stdout, result.Stderr)
		return
	}
	task.Status = "succeeded"
	if task.Action == "uninstall" {
		task.Message = fmt.Sprintf("Uninstalled %s. Restart WheelMaker or start a new agent session for the change to take effect.", task.PackageName)
	} else {
		task.Message = fmt.Sprintf("Installed %s@%s. Restart WheelMaker or start a new agent session for the change to take effect.", task.PackageName, task.Version)
	}
}

func (c *NPMCommand) currentTaskSnapshot() *npmTaskSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneNPMTask(c.task)
}

func parseNPMListDependencies(raw string) (map[string]string, error) {
	var body struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, fmt.Errorf("invalid npm list json: %w", err)
	}
	out := make(map[string]string, len(body.Dependencies))
	for name, dep := range body.Dependencies {
		version := strings.TrimSpace(dep.Version)
		if version != "" {
			out[name] = version
		}
	}
	return out, nil
}

func runtimePackageStatus(installed bool, installedVersion string, latestVersion string, latestError string) (string, bool, bool) {
	if latestError != "" {
		if installed {
			return "latest_unknown", false, false
		}
		return "checking_failed", true, false
	}
	if !installed {
		return "not_installed", true, false
	}
	if latestVersion == "" {
		return "checking_failed", false, false
	}
	if installedVersion == latestVersion {
		return "up_to_date", false, false
	}
	return "update_available", false, true
}

func runtimePackageAllowed(packageName string) bool {
	for _, policy := range runtimeNPMPackages {
		if policy.PackageName == packageName {
			return true
		}
	}
	return false
}

func deprecatedPackageAllowed(packageName string) bool {
	for _, policy := range deprecatedNPMPackages {
		if policy.PackageName == packageName {
			return true
		}
	}
	return false
}

func commandFailed(result npmCommandResult) bool {
	return result.Err != nil || result.ExitCode != 0
}

func npmResultSummary(result npmCommandResult) string {
	return formatNPMTaskErrorSummary(result.ExitCode, result.Stdout, result.Stderr)
}

func formatNPMTaskErrorSummary(exitCode int, stdout string, stderr string) string {
	segment := lastNonEmptySegment(stderr)
	if segment == "" {
		segment = lastNonEmptySegment(stdout)
	}
	if segment == "" {
		return fmt.Sprintf("npm command failed with exit code %d", exitCode)
	}
	return fmt.Sprintf("exit code %d: %s", exitCode, truncateRunes(segment, 500))
}

func lastNonEmptySegment(raw string) string {
	clean := strings.ReplaceAll(raw, "\r\n", "\n")
	paragraphs := strings.Split(clean, "\n\n")
	for i := len(paragraphs) - 1; i >= 0; i-- {
		segment := strings.TrimSpace(paragraphs[i])
		if segment != "" {
			return segment
		}
	}
	lines := strings.Split(clean, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func firstNonEmptyLine(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func cloneNPMTask(task *npmTaskSnapshot) *npmTaskSnapshot {
	if task == nil {
		return nil
	}
	cp := *task
	if task.ExitCode != nil {
		exitCode := *task.ExitCode
		cp.ExitCode = &exitCode
	}
	return &cp
}
