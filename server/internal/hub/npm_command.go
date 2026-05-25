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

	mu          sync.Mutex
	operation   *npmOperationSnapshot
	latestCache map[string]npmLatestCacheEntry
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
		latestCache: map[string]npmLatestCacheEntry{},
	}
}

type npmCommandPayload struct {
	Action       string   `json:"action"`
	HubID        string   `json:"hubId"`
	PackageName  string   `json:"packageName,omitempty"`
	PackageNames []string `json:"packageNames,omitempty"`
	Version      string   `json:"version,omitempty"`
}

type npmCommandResponse struct {
	OK        bool                  `json:"ok"`
	Accepted  bool                  `json:"accepted,omitempty"`
	UpdatedAt string                `json:"updatedAt,omitempty"`
	Hub       npmHubSnapshot        `json:"hub,omitempty"`
	Operation *npmOperationSnapshot `json:"operation"`
}

type npmHubSnapshot struct {
	HubID       string             `json:"hubId"`
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

type npmOperationSnapshot struct {
	Running      bool     `json:"running"`
	Action       string   `json:"action"`
	PackageName  string   `json:"packageName"`
	PackageNames []string `json:"packageNames,omitempty"`
	Version      string   `json:"version"`
	Status       string   `json:"status"`
	StartedAt    string   `json:"startedAt"`
	FinishedAt   string   `json:"finishedAt"`
	ExitCode     *int     `json:"exitCode"`
	ErrorSummary string   `json:"errorSummary"`
	Message      string   `json:"message,omitempty"`
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
	{PackageName: "@agentclientprotocol/claude-agent-acp", DisplayName: "Claude ACP", AgentTypes: []string{"claude"}, Kind: "runtime"},
	{PackageName: "@anthropic-ai/claude-code", DisplayName: "Claude CLI", AgentTypes: []string{"claude"}, Kind: "runtime"},
	{PackageName: "@openai/codex", DisplayName: "Codex CLI", AgentTypes: []string{"codex"}, Kind: "runtime"},
	{PackageName: "@github/copilot", DisplayName: "Copilot CLI", AgentTypes: []string{"copilot"}, Kind: "runtime"},
	{PackageName: "opencode-ai", DisplayName: "OpenCode CLI", AgentTypes: []string{"opencode"}, Kind: "runtime"},
}

var deprecatedNPMPackages = []npmPackagePolicy{
	{PackageName: "@zed-industries/codex-acp", DisplayName: "Deprecated Codex ACP", Kind: "deprecated"},
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
	payload.PackageNames = normalizeNPMPackageNames(payload.PackageNames)
	payload.Version = strings.TrimSpace(payload.Version)
	if payload.HubID == "" {
		return nil, &npmCommandError{Code: rp.CodeInvalidArgument, Message: "hubId is required"}
	}

	switch payload.Action {
	case "scan":
		return c.scan(ctx, payload.HubID), nil
	case "install":
		return c.startInstall(payload)
	case "install_many":
		return c.startInstallMany(payload)
	case "uninstall":
		return c.startUninstall(payload)
	default:
		return nil, &npmCommandError{Code: rp.CodeInvalidArgument, Message: "unsupported cmd.npm action"}
	}
}

func (c *NPMCommand) scan(ctx context.Context, hubID string) npmCommandResponse {
	now := c.now()
	updatedAt := now.Format(time.RFC3339)
	hub := npmHubSnapshot{
		HubID:    hubID,
		Packages: []npmPackageStatus{},
	}

	listResult := c.runner.Run(ctx, "npm", "list", "-g", "--depth=0", "--json")
	if commandFailed(listResult) {
		hub.Error = "npm list failed: " + npmResultSummary(listResult)
		return npmCommandResponse{OK: false, UpdatedAt: updatedAt, Hub: hub, Operation: c.currentOperationSnapshot()}
	}
	installed, err := parseNPMListDependencies(listResult.Stdout)
	if err != nil {
		hub.Error = "npm list failed: " + err.Error()
		return npmCommandResponse{OK: false, UpdatedAt: updatedAt, Hub: hub, Operation: c.currentOperationSnapshot()}
	}

	latest, missingLatest := c.latestResultsForScan(now)
	operation := c.currentOperationSnapshot()
	if len(missingLatest) > 0 && (operation == nil || !operation.Running) {
		started, cmdErr := c.acceptOperation("scan_latest", "", "", nil)
		if cmdErr == nil {
			operation = cloneNPMOperation(started)
			go c.runLatestOperation(started, missingLatest)
		}
	}
	for _, policy := range runtimeNPMPackages {
		installedVersion := installed[policy.PackageName]
		row := npmPackageStatus{
			PackageName:      policy.PackageName,
			DisplayName:      policy.DisplayName,
			AgentTypes:       cloneNPMStringSlice(policy.AgentTypes),
			Kind:             policy.Kind,
			Installed:        installedVersion != "",
			InstalledVersion: installedVersion,
			CanUninstall:     false,
		}
		latestResult, hasLatest := latest[policy.PackageName]
		if hasLatest {
			row.LatestVersion = latestResult.version
			row.Error = latestResult.errorSummary
			row.Status, row.CanInstall, row.CanUpdate = runtimePackageStatus(row.Installed, row.InstalledVersion, row.LatestVersion, row.Error)
		} else {
			row.Status = "checking_latest"
		}
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
			AgentTypes:       cloneNPMStringSlice(policy.AgentTypes),
			Kind:             policy.Kind,
			Installed:        true,
			InstalledVersion: installedVersion,
			Status:           "deprecated",
			CanUninstall:     true,
		})
	}
	return npmCommandResponse{OK: true, UpdatedAt: updatedAt, Hub: hub, Operation: operation}
}

func cloneNPMStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}

func normalizeNPMPackageNames(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		packageName := strings.TrimSpace(value)
		if packageName == "" {
			continue
		}
		if _, ok := seen[packageName]; ok {
			continue
		}
		seen[packageName] = struct{}{}
		out = append(out, packageName)
	}
	return out
}

type npmLatestResult struct {
	version      string
	errorSummary string
}

type npmLatestCacheEntry struct {
	result    npmLatestResult
	fetchedAt time.Time
}

const (
	npmLatestSuccessTTL = time.Hour
	npmLatestFailureTTL = 5 * time.Minute
)

func (c *NPMCommand) lookupLatestVersions(ctx context.Context, packageNames []string) map[string]npmLatestResult {
	out := make(map[string]npmLatestResult, len(packageNames))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, packageName := range packageNames {
		packageName := packageName
		wg.Add(1)
		go func() {
			defer wg.Done()
			result := c.runner.Run(ctx, "npm", "view", packageName, "version")
			next := npmLatestResult{version: firstNonEmptyLine(result.Stdout)}
			if commandFailed(result) || next.version == "" {
				next.version = ""
				next.errorSummary = npmResultSummary(result)
			}
			mu.Lock()
			out[packageName] = next
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

func (c *NPMCommand) latestResultsForScan(now time.Time) (map[string]npmLatestResult, []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make(map[string]npmLatestResult, len(runtimeNPMPackages))
	var missing []string
	for _, policy := range runtimeNPMPackages {
		entry, ok := c.latestCache[policy.PackageName]
		if !ok || !npmLatestCacheValid(entry, now) {
			missing = append(missing, policy.PackageName)
			continue
		}
		out[policy.PackageName] = entry.result
	}
	return out, missing
}

func npmLatestCacheValid(entry npmLatestCacheEntry, now time.Time) bool {
	if entry.fetchedAt.IsZero() {
		return false
	}
	ttl := npmLatestFailureTTL
	if entry.result.version != "" && entry.result.errorSummary == "" {
		ttl = npmLatestSuccessTTL
	}
	return now.Sub(entry.fetchedAt) < ttl
}

func (c *NPMCommand) runLatestOperation(operation *npmOperationSnapshot, packageNames []string) {
	results := c.lookupLatestVersions(context.Background(), packageNames)
	finished := c.now()
	finishedAt := finished.Format(time.RFC3339)
	var failed []string

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, packageName := range packageNames {
		result := results[packageName]
		c.latestCache[packageName] = npmLatestCacheEntry{
			result:    result,
			fetchedAt: finished,
		}
		if result.errorSummary != "" {
			failed = append(failed, packageName)
		}
	}
	if c.operation != operation {
		return
	}
	operation.Running = false
	operation.FinishedAt = finishedAt
	if len(failed) > 0 {
		operation.Status = "failed"
		operation.ErrorSummary = fmt.Sprintf("latest version check failed for %s", strings.Join(failed, ", "))
		return
	}
	operation.Status = "succeeded"
	operation.Message = "Latest package versions refreshed."
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
	operation, cmdErr := c.acceptOperation("install", payload.PackageName, version, nil)
	if cmdErr != nil {
		return nil, cmdErr
	}
	go c.runCommandOperation(operation, "npm", "install", "-g", payload.PackageName+"@"+version)
	return npmCommandResponse{OK: true, Accepted: true, Operation: cloneNPMOperation(operation)}, nil
}

func (c *NPMCommand) startInstallMany(payload npmCommandPayload) (any, *npmCommandError) {
	if len(payload.PackageNames) == 0 {
		return nil, &npmCommandError{Code: rp.CodeInvalidArgument, Message: "packageNames is required"}
	}
	version := payload.Version
	if version == "" {
		version = "latest"
	}
	if version != "latest" {
		return nil, &npmCommandError{Code: rp.CodeInvalidArgument, Message: "version must be latest"}
	}
	for _, packageName := range payload.PackageNames {
		if !runtimePackageAllowed(packageName) {
			return nil, &npmCommandError{Code: rp.CodeForbidden, Message: "package is not installable"}
		}
	}
	operation, cmdErr := c.acceptOperation("install_many", "", version, payload.PackageNames)
	if cmdErr != nil {
		return nil, cmdErr
	}
	go c.runInstallManyOperation(operation, payload.PackageNames, version)
	return npmCommandResponse{OK: true, Accepted: true, Operation: cloneNPMOperation(operation)}, nil
}

func (c *NPMCommand) startUninstall(payload npmCommandPayload) (any, *npmCommandError) {
	if payload.PackageName == "" {
		return nil, &npmCommandError{Code: rp.CodeInvalidArgument, Message: "packageName is required"}
	}
	if !deprecatedPackageAllowed(payload.PackageName) {
		return nil, &npmCommandError{Code: rp.CodeForbidden, Message: "package is not uninstallable"}
	}
	operation, cmdErr := c.acceptOperation("uninstall", payload.PackageName, "", nil)
	if cmdErr != nil {
		return nil, cmdErr
	}
	go c.runCommandOperation(operation, "npm", "uninstall", "-g", payload.PackageName)
	return npmCommandResponse{OK: true, Accepted: true, Operation: cloneNPMOperation(operation)}, nil
}

func (c *NPMCommand) acceptOperation(action string, packageName string, version string, packageNames []string) (*npmOperationSnapshot, *npmCommandError) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.operation != nil && c.operation.Running {
		return nil, &npmCommandError{Code: rp.CodeConflict, Message: "npm operation already running"}
	}
	status := "running"
	if action == "scan_latest" {
		status = "checking_latest"
	}
	operation := &npmOperationSnapshot{
		Running:      true,
		Action:       action,
		PackageName:  packageName,
		PackageNames: cloneNPMStringSlice(packageNames),
		Version:      version,
		Status:       status,
		StartedAt:    c.now().Format(time.RFC3339),
	}
	c.operation = operation
	return operation, nil
}

func (c *NPMCommand) runCommandOperation(operation *npmOperationSnapshot, name string, args ...string) {
	result := c.runner.Run(context.Background(), name, args...)
	exitCode := result.ExitCode
	finishedAt := c.now().Format(time.RFC3339)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.operation != operation {
		return
	}
	operation.Running = false
	operation.FinishedAt = finishedAt
	operation.ExitCode = &exitCode
	if commandFailed(result) {
		operation.Status = "failed"
		operation.ErrorSummary = formatNPMTaskErrorSummary(exitCode, result.Stdout, result.Stderr)
		return
	}
	operation.Status = "succeeded"
	if operation.Action == "uninstall" {
		operation.Message = fmt.Sprintf("Uninstalled %s. Restart WheelMaker or start a new agent session for the change to take effect.", operation.PackageName)
	} else {
		operation.Message = fmt.Sprintf("Installed %s@%s. Restart WheelMaker or start a new agent session for the change to take effect.", operation.PackageName, operation.Version)
	}
}

func (c *NPMCommand) runInstallManyOperation(operation *npmOperationSnapshot, packageNames []string, version string) {
	var failed []string
	var exitCode *int
	for _, packageName := range packageNames {
		result := c.runner.Run(context.Background(), "npm", "install", "-g", packageName+"@"+version)
		if commandFailed(result) {
			code := result.ExitCode
			exitCode = &code
			failed = append(failed, packageName+": "+formatNPMTaskErrorSummary(result.ExitCode, result.Stdout, result.Stderr))
		}
	}
	finishedAt := c.now().Format(time.RFC3339)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.operation != operation {
		return
	}
	operation.Running = false
	operation.FinishedAt = finishedAt
	if exitCode != nil {
		code := *exitCode
		operation.ExitCode = &code
	}
	if len(failed) > 0 {
		operation.Status = "failed"
		operation.ErrorSummary = "Failed npm package installs: " + strings.Join(failed, "; ")
		return
	}
	operation.Status = "succeeded"
	operation.Message = fmt.Sprintf("Installed %d npm %s. Restart WheelMaker or start a new agent session for the change to take effect.", len(packageNames), pluralNoun(len(packageNames), "package", "packages"))
}

func (c *NPMCommand) currentOperationSnapshot() *npmOperationSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneNPMOperation(c.operation)
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
		return "latest_unknown", false, false
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

func cloneNPMOperation(operation *npmOperationSnapshot) *npmOperationSnapshot {
	if operation == nil {
		return nil
	}
	cp := *operation
	cp.PackageNames = cloneNPMStringSlice(operation.PackageNames)
	if operation.ExitCode != nil {
		exitCode := *operation.ExitCode
		cp.ExitCode = &exitCode
	}
	return &cp
}

func pluralNoun(count int, singular string, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}
