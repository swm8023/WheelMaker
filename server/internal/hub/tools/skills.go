package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

var (
	skillSourceRepoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	skillSourceSlugPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	skillNamePattern       = regexp.MustCompile(`^[@A-Za-z0-9][@A-Za-z0-9_.:-]*$`)
	ansiEscapePattern      = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
)

var fixedSkillAgents = []string{"codex", "claude-code", "opencode", "github-copilot"}

type skillsCommandCall struct {
	Dir  string
	Name string
	Args []string
}

type skillsCommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

type skillsCommandRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) skillsCommandResult
}

type execSkillsCommandRunner struct{}

func (execSkillsCommandRunner) Run(ctx context.Context, dir string, name string, args ...string) skillsCommandResult {
	cmd := exec.CommandContext(ctx, name, args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
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
	return skillsCommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Err:      err,
	}
}

type skillsCommandConfig struct {
	HubID          string
	Projects       []ProjectInfo
	GlobalLockPath string
	HomeDir        string
}

type SkillsCommand struct {
	runner         skillsCommandRunner
	now            func() time.Time
	hubID          string
	globalLockPath string
	homeDir        string

	mu        sync.RWMutex
	projects  []ProjectInfo
	operation *skillsOperationSnapshot
}

func NewSkillsCommand(config skillsCommandConfig) *SkillsCommand {
	return newSkillsCommandWithRunner(execSkillsCommandRunner{}, config)
}

func newSkillsCommandWithRunner(runner skillsCommandRunner, config skillsCommandConfig) *SkillsCommand {
	if runner == nil {
		runner = execSkillsCommandRunner{}
	}
	cmd := &SkillsCommand{
		runner:         runner,
		hubID:          strings.TrimSpace(config.HubID),
		globalLockPath: strings.TrimSpace(config.GlobalLockPath),
		homeDir:        strings.TrimSpace(config.HomeDir),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	cmd.SetProjects(config.Projects)
	return cmd
}

func (c *SkillsCommand) SetProjects(projects []ProjectInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projects = append([]ProjectInfo(nil), projects...)
}

type skillsCommandPayload struct {
	Action          string   `json:"action"`
	HubID           string   `json:"hubId"`
	Scope           string   `json:"scope,omitempty"`
	ProjectName     string   `json:"projectName,omitempty"`
	Source          string   `json:"source,omitempty"`
	Skills          []string `json:"skills,omitempty"`
	IncludeProjects bool     `json:"includeProjects,omitempty"`
}

type skillsCommandResponse struct {
	OK           bool                     `json:"ok"`
	Accepted     bool                     `json:"accepted,omitempty"`
	HubID        string                   `json:"hubId"`
	UpdatedAt    string                   `json:"updatedAt,omitempty"`
	Source       string                   `json:"source,omitempty"`
	Scope        string                   `json:"scope,omitempty"`
	ProjectName  string                   `json:"projectName,omitempty"`
	HubSkills    skillsScopeSnapshot      `json:"hubSkills,omitempty"`
	Projects     []skillsProjectSnapshot  `json:"projects,omitempty"`
	Skills       []skillsSkillSnapshot    `json:"skills,omitempty"`
	Candidates   []skillsSourceCandidate  `json:"candidates,omitempty"`
	Operation    *skillsOperationSnapshot `json:"operation,omitempty"`
	Message      string                   `json:"message,omitempty"`
	ErrorSummary string                   `json:"errorSummary,omitempty"`
}

type skillsOperationSnapshot struct {
	Running         bool     `json:"running"`
	Action          string   `json:"action"`
	Scope           string   `json:"scope,omitempty"`
	ProjectName     string   `json:"projectName,omitempty"`
	Source          string   `json:"source,omitempty"`
	Skills          []string `json:"skills,omitempty"`
	IncludeProjects bool     `json:"includeProjects,omitempty"`
	Status          string   `json:"status"`
	StartedAt       string   `json:"startedAt"`
	FinishedAt      string   `json:"finishedAt,omitempty"`
	ExitCode        *int     `json:"exitCode"`
	ErrorSummary    string   `json:"errorSummary,omitempty"`
	Message         string   `json:"message,omitempty"`
}

type skillsScopeSnapshot struct {
	Scope  string                `json:"scope"`
	Skills []skillsSkillSnapshot `json:"skills"`
}

type skillsProjectSnapshot struct {
	ProjectName string                `json:"projectName"`
	ProjectID   string                `json:"projectId"`
	Online      bool                  `json:"online"`
	Path        string                `json:"path"`
	Skills      []skillsSkillSnapshot `json:"skills"`
	Error       string                `json:"error,omitempty"`
}

type skillsSkillSnapshot struct {
	Name        string   `json:"name"`
	Path        string   `json:"path,omitempty"`
	Category    string   `json:"category"`
	CategoryKey string   `json:"categoryKey"`
	Managed     bool     `json:"managed"`
	Agents      []string `json:"agents,omitempty"`
}

type skillsSourceCandidate struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category"`
	CategoryKey string `json:"categoryKey"`
}

type skillsCommandError struct {
	Code    string
	Message string
}

func (e *skillsCommandError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

func (e *skillsCommandError) commandCode() string {
	if e == nil {
		return ""
	}
	return e.Code
}

func (e *skillsCommandError) commandMessage() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (c *SkillsCommand) Handle(ctx context.Context, raw json.RawMessage) (any, *skillsCommandError) {
	var payload skillsCommandPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, &skillsCommandError{Code: rp.CodeInvalidArgument, Message: "invalid cmd.skills payload"}
	}
	payload.Action = strings.TrimSpace(payload.Action)
	payload.HubID = strings.TrimSpace(payload.HubID)
	payload.Scope = strings.TrimSpace(payload.Scope)
	payload.ProjectName = strings.TrimSpace(payload.ProjectName)
	payload.Source = strings.TrimSpace(payload.Source)
	payload.Skills = normalizeSkillNames(payload.Skills)
	if payload.HubID == "" {
		return nil, &skillsCommandError{Code: rp.CodeInvalidArgument, Message: "hubId is required"}
	}
	if c.hubID != "" && payload.HubID != c.hubID {
		return nil, &skillsCommandError{Code: rp.CodeForbidden, Message: "hubId does not match this hub"}
	}

	switch payload.Action {
	case "scan":
		return c.scan(ctx, payload.HubID), nil
	case "list":
		return c.listSource(ctx, payload)
	case "install":
		return c.startInstall(payload)
	case "uninstall":
		return c.startUninstall(payload)
	case "update":
		return c.startUpdate(payload)
	default:
		return nil, &skillsCommandError{Code: rp.CodeInvalidArgument, Message: "unsupported cmd.skills action"}
	}
}

func (c *SkillsCommand) scan(ctx context.Context, hubID string) skillsCommandResponse {
	updatedAt := c.now().Format(time.RFC3339)
	hubSkills, hubErr := c.scanHubSkills(ctx)
	resp := skillsCommandResponse{
		OK:        hubErr == "",
		HubID:     hubID,
		UpdatedAt: updatedAt,
		HubSkills: skillsScopeSnapshot{
			Scope:  "hub",
			Skills: hubSkills,
		},
		Projects:  []skillsProjectSnapshot{},
		Operation: c.currentOperationSnapshot(),
	}
	if hubErr != "" {
		resp.ErrorSummary = hubErr
	}

	for _, project := range c.projectSnapshot() {
		projectName := strings.TrimSpace(project.Name)
		snapshot := skillsProjectSnapshot{
			ProjectName: projectName,
			ProjectID:   rp.ProjectID(hubID, projectName),
			Online:      project.Online,
			Path:        strings.TrimSpace(project.Path),
			Skills:      []skillsSkillSnapshot{},
		}
		if snapshot.Path == "" {
			snapshot.Error = "project path is empty"
			resp.Projects = append(resp.Projects, snapshot)
			continue
		}
		skills, errSummary := c.scanProjectSkills(ctx, project)
		snapshot.Skills = skills
		snapshot.Error = errSummary
		resp.Projects = append(resp.Projects, snapshot)
	}
	return resp
}

func (c *SkillsCommand) listSource(ctx context.Context, payload skillsCommandPayload) (skillsCommandResponse, *skillsCommandError) {
	if err := validateRemoteSkillSource(payload.Source); err != nil {
		return skillsCommandResponse{}, err
	}
	result := c.runSkills(ctx, "", "add", payload.Source, "--list")
	if skillsCommandFailed(result) {
		return skillsCommandResponse{
			OK:           false,
			HubID:        payload.HubID,
			UpdatedAt:    c.now().Format(time.RFC3339),
			Source:       payload.Source,
			ErrorSummary: skillsResultSummary(result),
		}, nil
	}
	candidates := parseSkillsSourceCandidates(result.Stdout)
	if len(candidates) == 0 {
		return skillsCommandResponse{
			OK:           false,
			HubID:        payload.HubID,
			UpdatedAt:    c.now().Format(time.RFC3339),
			Source:       payload.Source,
			ErrorSummary: "skills list output did not include any installable skills",
		}, nil
	}
	return skillsCommandResponse{
		OK:         true,
		HubID:      payload.HubID,
		UpdatedAt:  c.now().Format(time.RFC3339),
		Source:     payload.Source,
		Candidates: candidates,
	}, nil
}

func (c *SkillsCommand) startInstall(payload skillsCommandPayload) (any, *skillsCommandError) {
	if err := validateRemoteSkillSource(payload.Source); err != nil {
		return nil, err
	}
	if len(payload.Skills) == 0 {
		return nil, &skillsCommandError{Code: rp.CodeInvalidArgument, Message: "skills are required"}
	}
	if err := validateSkillNames(payload.Skills); err != nil {
		return nil, err
	}
	target, cmdErr := c.resolveTarget(payload)
	if cmdErr != nil {
		return nil, cmdErr
	}
	args := skillsAddArgs(target, payload.Source, payload.Skills)
	return c.startOperation(payload, []skillsOperationRun{{
		target:             target,
		args:               args,
		message:            "Installed skills.",
		prepareInstallDirs: true,
	}})
}

func (c *SkillsCommand) startUninstall(payload skillsCommandPayload) (any, *skillsCommandError) {
	if len(payload.Skills) == 0 {
		return nil, &skillsCommandError{Code: rp.CodeInvalidArgument, Message: "skills are required"}
	}
	if err := validateSkillNames(payload.Skills); err != nil {
		return nil, err
	}
	target, cmdErr := c.resolveTarget(payload)
	if cmdErr != nil {
		return nil, cmdErr
	}
	args := skillsRemoveArgs(target, payload.Skills)
	return c.startOperation(payload, []skillsOperationRun{{
		target:  target,
		args:    args,
		message: "Uninstalled skills.",
	}})
}

func (c *SkillsCommand) startUpdate(payload skillsCommandPayload) (any, *skillsCommandError) {
	target, cmdErr := c.resolveTarget(payload)
	if cmdErr != nil {
		return nil, cmdErr
	}
	runs, err := c.updateRuns(target)
	if err != nil {
		return nil, err
	}
	if payload.IncludeProjects {
		if target.scope != "hub" {
			return nil, &skillsCommandError{Code: rp.CodeInvalidArgument, Message: "includeProjects requires hub scope"}
		}
		projectRuns, projectErr := c.projectUpdateRuns()
		if projectErr != nil {
			return nil, projectErr
		}
		runs = append(runs, projectRuns...)
	}
	return c.startOperation(payload, runs)
}

type skillsOperationRun struct {
	target             skillsCommandTarget
	args               []string
	message            string
	prepareInstallDirs bool
}

func (c *SkillsCommand) startOperation(payload skillsCommandPayload, runs []skillsOperationRun) (any, *skillsCommandError) {
	operation, cmdErr := c.acceptOperation(payload)
	if cmdErr != nil {
		return nil, cmdErr
	}
	accepted := cloneSkillsOperation(operation)
	go c.runSkillsOperation(operation, runs)
	return skillsCommandResponse{
		OK:          true,
		Accepted:    true,
		HubID:       payload.HubID,
		UpdatedAt:   operation.StartedAt,
		Source:      payload.Source,
		Scope:       payload.Scope,
		ProjectName: payload.ProjectName,
		Operation:   accepted,
	}, nil
}

func (c *SkillsCommand) acceptOperation(payload skillsCommandPayload) (*skillsOperationSnapshot, *skillsCommandError) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.operation != nil && c.operation.Running {
		return nil, &skillsCommandError{Code: rp.CodeConflict, Message: "skills operation already running"}
	}
	operation := &skillsOperationSnapshot{
		Running:         true,
		Action:          payload.Action,
		Scope:           payload.Scope,
		ProjectName:     payload.ProjectName,
		Source:          payload.Source,
		Skills:          append([]string(nil), payload.Skills...),
		IncludeProjects: payload.IncludeProjects,
		Status:          "running",
		StartedAt:       c.now().Format(time.RFC3339),
	}
	c.operation = operation
	return operation, nil
}

func (c *SkillsCommand) runSkillsOperation(operation *skillsOperationSnapshot, runs []skillsOperationRun) {
	messages := make([]string, 0, len(runs))
	for _, run := range runs {
		if run.prepareInstallDirs {
			if err := c.prepareSkillsInstallDirs(run.target); err != nil {
				exitCode := -1
				c.finishOperation(operation, "failed", &exitCode, err.Error(), "")
				return
			}
		}
		result := c.runSkills(context.Background(), run.target.dir, run.args...)
		exitCode := result.ExitCode
		if skillsCommandFailed(result) {
			c.finishOperation(operation, "failed", &exitCode, skillsResultSummary(result), "")
			return
		}
		if run.message != "" {
			messages = append(messages, run.message)
		}
	}
	message := "Skills operation completed."
	if len(messages) > 0 {
		message = messages[0]
	}
	c.finishOperation(operation, "succeeded", nil, "", message)
}

func (c *SkillsCommand) finishOperation(operation *skillsOperationSnapshot, status string, exitCode *int, errorSummary string, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.operation != operation {
		return
	}
	operation.Running = false
	operation.Status = status
	operation.FinishedAt = c.now().Format(time.RFC3339)
	if exitCode != nil {
		code := *exitCode
		operation.ExitCode = &code
	}
	operation.ErrorSummary = errorSummary
	operation.Message = message
}

func (c *SkillsCommand) currentOperationSnapshot() *skillsOperationSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneSkillsOperation(c.operation)
}

func cloneSkillsOperation(operation *skillsOperationSnapshot) *skillsOperationSnapshot {
	if operation == nil {
		return nil
	}
	clone := *operation
	clone.Skills = append([]string(nil), operation.Skills...)
	if operation.ExitCode != nil {
		code := *operation.ExitCode
		clone.ExitCode = &code
	}
	return &clone
}

func skillsAddArgs(target skillsCommandTarget, source string, skills []string) []string {
	args := []string{"add", source}
	if target.scope == "hub" {
		args = append(args, "-g")
	}
	args = append(args, "--agent")
	args = append(args, fixedSkillAgents...)
	args = append(args, "--skill")
	args = append(args, skills...)
	if target.scope == "project" {
		args = append(args, "--copy")
	}
	return append(args, "-y")
}

func skillsRemoveArgs(target skillsCommandTarget, skills []string) []string {
	args := []string{"remove"}
	if target.scope == "hub" {
		args = append(args, "-g")
	}
	args = append(args, "--skill")
	args = append(args, skills...)
	args = append(args, "--agent")
	args = append(args, fixedSkillAgents...)
	return append(args, "-y")
}

func (c *SkillsCommand) projectUpdateRuns() ([]skillsOperationRun, *skillsCommandError) {
	var runs []skillsOperationRun
	for _, project := range c.projectSnapshot() {
		if !project.Online {
			continue
		}
		dir := strings.TrimSpace(project.Path)
		if dir == "" {
			return nil, &skillsCommandError{Code: rp.CodeInvalidArgument, Message: "project path is empty"}
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, &skillsCommandError{Code: rp.CodeInternal, Message: err.Error()}
		}
		projectName := strings.TrimSpace(project.Name)
		target := skillsCommandTarget{
			scope:       "project",
			projectName: projectName,
			project:     project,
			dir:         abs,
		}
		projectRuns, cmdErr := c.updateRuns(target)
		if cmdErr != nil {
			return nil, cmdErr
		}
		runs = append(runs, projectRuns...)
	}
	return runs, nil
}

func (c *SkillsCommand) updateRuns(target skillsCommandTarget) ([]skillsOperationRun, *skillsCommandError) {
	lockPath := c.skillsLockFile(target)
	groups := readSkillsLockInstallGroups(lockPath)
	if len(groups) == 0 {
		return []skillsOperationRun{{
			target:             target,
			args:               fallbackSkillsUpdateArgs(target),
			message:            "Updated skills.",
			prepareInstallDirs: target.scope == "hub",
		}}, nil
	}
	runs := make([]skillsOperationRun, 0, len(groups))
	for _, group := range groups {
		runs = append(runs, skillsOperationRun{
			target:             target,
			args:               skillsAddArgs(target, group.Source, group.Skills),
			message:            "Updated skills.",
			prepareInstallDirs: target.scope == "hub",
		})
	}
	return runs, nil
}

func fallbackSkillsUpdateArgs(target skillsCommandTarget) []string {
	args := []string{"update"}
	if target.scope == "hub" {
		args = append(args, "-g")
	} else {
		args = append(args, "-p")
	}
	return append(args, "-y")
}

func (c *SkillsCommand) prepareSkillsInstallDirs(target skillsCommandTarget) error {
	dirs, err := c.skillsInstallDirs(target)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("prepare skills directory %s: %w", dir, err)
		}
	}
	return nil
}

func (c *SkillsCommand) skillsInstallDirs(target skillsCommandTarget) ([]string, error) {
	switch target.scope {
	case "hub":
		home, err := c.skillsHomeDir()
		if err != nil {
			return nil, err
		}
		claudeHome := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR"))
		if claudeHome == "" {
			claudeHome = filepath.Join(home, ".claude")
		}
		return []string{
			filepath.Join(home, ".agents", "skills"),
			filepath.Join(claudeHome, "skills"),
		}, nil
	case "project":
		if strings.TrimSpace(target.dir) == "" {
			return nil, fmt.Errorf("project path is empty")
		}
		info, err := os.Stat(target.dir)
		if err != nil {
			return nil, fmt.Errorf("stat project path %s: %w", target.dir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("project path is not a directory: %s", target.dir)
		}
		return []string{
			filepath.Join(target.dir, ".agents", "skills"),
			filepath.Join(target.dir, ".claude", "skills"),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported skills scope: %s", target.scope)
	}
}

func (c *SkillsCommand) skillsLockFile(target skillsCommandTarget) string {
	if target.scope == "hub" {
		return c.globalLockFile()
	}
	if strings.TrimSpace(target.dir) == "" {
		return ""
	}
	return filepath.Join(target.dir, "skills-lock.json")
}

func (c *SkillsCommand) skillsHomeDir() (string, error) {
	if c.homeDir != "" {
		return filepath.Abs(c.homeDir)
	}
	return os.UserHomeDir()
}

type skillsCommandTarget struct {
	scope       string
	projectName string
	project     ProjectInfo
	dir         string
}

func (c *SkillsCommand) resolveTarget(payload skillsCommandPayload) (skillsCommandTarget, *skillsCommandError) {
	scope := strings.TrimSpace(payload.Scope)
	if scope != "hub" && scope != "project" {
		return skillsCommandTarget{}, &skillsCommandError{Code: rp.CodeInvalidArgument, Message: "scope must be hub or project"}
	}
	if scope == "hub" {
		return skillsCommandTarget{scope: "hub"}, nil
	}
	if payload.ProjectName == "" {
		return skillsCommandTarget{}, &skillsCommandError{Code: rp.CodeInvalidArgument, Message: "projectName is required"}
	}
	project, ok := c.findProject(payload.HubID, payload.ProjectName)
	if !ok {
		return skillsCommandTarget{}, &skillsCommandError{Code: rp.CodeNotFound, Message: "project not found"}
	}
	dir := strings.TrimSpace(project.Path)
	if dir == "" {
		return skillsCommandTarget{}, &skillsCommandError{Code: rp.CodeInvalidArgument, Message: "project path is empty"}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return skillsCommandTarget{}, &skillsCommandError{Code: rp.CodeInternal, Message: err.Error()}
	}
	return skillsCommandTarget{scope: "project", projectName: strings.TrimSpace(project.Name), project: project, dir: abs}, nil
}

func (c *SkillsCommand) scanHubSkills(ctx context.Context) ([]skillsSkillSnapshot, string) {
	lockMetadata := readSkillsLockScanMetadata(c.globalLockFile())
	result := c.runSkills(ctx, "", "list", "-g", "--json")
	if skillsCommandFailed(result) {
		return []skillsSkillSnapshot{}, skillsResultSummary(result)
	}
	skills, err := parseSkillsListJSON(result.Stdout, lockMetadata.PluginNames, lockMetadata.Managed)
	if err != nil {
		return []skillsSkillSnapshot{}, err.Error()
	}
	return skills, ""
}

func (c *SkillsCommand) scanProjectSkills(ctx context.Context, project ProjectInfo) ([]skillsSkillSnapshot, string) {
	dir := strings.TrimSpace(project.Path)
	if dir == "" {
		return []skillsSkillSnapshot{}, "project path is empty"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return []skillsSkillSnapshot{}, err.Error()
	}
	lockMetadata := readSkillsLockScanMetadata(filepath.Join(abs, "skills-lock.json"))
	result := c.runSkills(ctx, abs, "list", "--json")
	if skillsCommandFailed(result) {
		return []skillsSkillSnapshot{}, skillsResultSummary(result)
	}
	skills, err := parseSkillsListJSON(result.Stdout, lockMetadata.PluginNames, lockMetadata.Managed)
	if err != nil {
		return []skillsSkillSnapshot{}, err.Error()
	}
	return skills, ""
}

func (c *SkillsCommand) runSkills(ctx context.Context, dir string, args ...string) skillsCommandResult {
	result := c.runner.Run(ctx, dir, "skills", args...)
	if !skillsCommandUnavailable(result) {
		return result
	}
	npxArgs := append([]string{"--yes", "skills"}, args...)
	return c.runner.Run(ctx, dir, "npx", npxArgs...)
}

func (c *SkillsCommand) projectSnapshot() []ProjectInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]ProjectInfo(nil), c.projects...)
}

func (c *SkillsCommand) findProject(hubID string, nameOrID string) (ProjectInfo, bool) {
	nameOrID = strings.TrimSpace(nameOrID)
	for _, project := range c.projectSnapshot() {
		projectName := strings.TrimSpace(project.Name)
		if projectName == nameOrID || rp.ProjectID(hubID, projectName) == nameOrID {
			return project, true
		}
	}
	return ProjectInfo{}, false
}

func (c *SkillsCommand) globalLockFile() string {
	if strings.TrimSpace(c.globalLockPath) != "" {
		return c.globalLockPath
	}
	if stateHome := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); stateHome != "" {
		return filepath.Join(stateHome, "skills", ".skill-lock.json")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".agents", ".skill-lock.json")
	}
	return ""
}

func parseSkillsListJSON(raw string, pluginNames map[string]string, managed map[string]bool) ([]skillsSkillSnapshot, error) {
	var items []struct {
		Name       string   `json:"name"`
		Path       string   `json:"path"`
		Scope      string   `json:"scope"`
		Agents     []string `json:"agents"`
		PluginName string   `json:"pluginName"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		var wrapped struct {
			Skills []struct {
				Name       string   `json:"name"`
				Path       string   `json:"path"`
				Scope      string   `json:"scope"`
				Agents     []string `json:"agents"`
				PluginName string   `json:"pluginName"`
			} `json:"skills"`
		}
		if wrappedErr := json.Unmarshal([]byte(raw), &wrapped); wrappedErr != nil {
			return nil, fmt.Errorf("invalid skills list json: %w", err)
		}
		for _, item := range wrapped.Skills {
			items = append(items, struct {
				Name       string   `json:"name"`
				Path       string   `json:"path"`
				Scope      string   `json:"scope"`
				Agents     []string `json:"agents"`
				PluginName string   `json:"pluginName"`
			}(item))
		}
	}
	out := make([]skillsSkillSnapshot, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		pluginName := strings.TrimSpace(item.PluginName)
		if pluginName == "" {
			pluginName = pluginNames[name]
		}
		categoryKey, category := skillCategory(pluginName)
		out = append(out, skillsSkillSnapshot{
			Name:        name,
			Path:        strings.TrimSpace(item.Path),
			Category:    category,
			CategoryKey: categoryKey,
			Managed:     managed[name],
			Agents:      append([]string(nil), item.Agents...),
		})
	}
	return out, nil
}

type skillsLockScanMetadata struct {
	PluginNames map[string]string
	Managed     map[string]bool
}

func readSkillsLockScanMetadata(path string) skillsLockScanMetadata {
	out := skillsLockScanMetadata{
		PluginNames: map[string]string{},
		Managed:     map[string]bool{},
	}
	if strings.TrimSpace(path) == "" {
		return out
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var body struct {
		Skills map[string]struct {
			PluginName string `json:"pluginName"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return out
	}
	for name, skill := range body.Skills {
		skillName := strings.TrimSpace(name)
		if skillName == "" {
			continue
		}
		out.Managed[skillName] = true
		pluginName := strings.TrimSpace(skill.PluginName)
		if pluginName != "" {
			out.PluginNames[skillName] = pluginName
		}
	}
	return out
}

type skillsLockInstallGroup struct {
	Source string
	Skills []string
}

func readSkillsLockInstallGroups(path string) []skillsLockInstallGroup {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var body struct {
		Skills map[string]struct {
			Source     string `json:"source"`
			SourceURL  string `json:"sourceUrl"`
			SourceType string `json:"sourceType"`
			Ref        string `json:"ref"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil
	}
	names := make([]string, 0, len(body.Skills))
	for name := range body.Skills {
		names = append(names, name)
	}
	sort.Strings(names)

	grouped := map[string][]string{}
	var sources []string
	for _, name := range names {
		skillName := strings.TrimSpace(name)
		if skillName == "" {
			continue
		}
		entry := body.Skills[name]
		sourceType := strings.ToLower(strings.TrimSpace(entry.SourceType))
		if sourceType == "local" || sourceType == "node_modules" {
			continue
		}
		source := strings.TrimSpace(entry.Source)
		if source == "" {
			source = strings.TrimSpace(entry.SourceURL)
		}
		if source == "" {
			continue
		}
		if ref := strings.TrimSpace(entry.Ref); ref != "" {
			source += "#" + ref
		}
		if _, ok := grouped[source]; !ok {
			sources = append(sources, source)
		}
		grouped[source] = append(grouped[source], skillName)
	}
	sort.Strings(sources)

	groups := make([]skillsLockInstallGroup, 0, len(sources))
	for _, source := range sources {
		groups = append(groups, skillsLockInstallGroup{
			Source: source,
			Skills: grouped[source],
		})
	}
	return groups
}

func parseSkillsSourceCandidates(raw string) []skillsSourceCandidate {
	currentCategoryKey, currentCategory := skillCategory("")
	var candidates []skillsSourceCandidate
	var current *skillsSourceCandidate
	for _, rawLine := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		line := cleanSkillsOutputLine(rawLine)
		if line == "" {
			current = nil
			continue
		}
		if shouldSkipSkillsOutputLine(line) {
			continue
		}
		if name, desc, ok := parseSkillsCandidateLine(line); ok {
			categoryKey := currentCategoryKey
			category := currentCategory
			candidates = append(candidates, skillsSourceCandidate{
				Name:        name,
				Description: desc,
				Category:    category,
				CategoryKey: categoryKey,
			})
			current = &candidates[len(candidates)-1]
			continue
		}
		if current != nil {
			if current.Description == "" {
				current.Description = line
			} else {
				current.Description += " " + line
			}
			continue
		}
		currentCategoryKey = categoryKeyFromTitle(line)
		currentCategory = line
	}
	return candidates
}

func cleanSkillsOutputLine(line string) string {
	line = ansiEscapePattern.ReplaceAllString(line, "")
	line = strings.TrimSpace(strings.ReplaceAll(line, "\r", ""))
	for index, r := range line {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '@' || r == '_' || r == '-' || r == '.' || r == ':' {
			return strings.TrimSpace(line[index:])
		}
	}
	return ""
}

func shouldSkipSkillsOutputLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" {
		return true
	}
	switch lower {
	case "available skills", "global skills", "project skills", "skills":
		return true
	}
	return strings.HasPrefix(lower, "source:") ||
		strings.HasPrefix(lower, "fetching ") ||
		strings.HasPrefix(lower, "cloning ") ||
		strings.HasPrefix(lower, "found ") ||
		strings.HasPrefix(lower, "use --skill") ||
		strings.HasPrefix(lower, "run ")
}

func parseSkillsCandidateLine(line string) (string, string, bool) {
	for _, separator := range []string{" - ", ": "} {
		index := strings.Index(line, separator)
		if index <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:index])
		if skillNamePattern.MatchString(name) {
			return name, strings.TrimSpace(line[index+len(separator):]), true
		}
	}
	if skillNamePattern.MatchString(line) {
		return line, "", true
	}
	return "", "", false
}

func skillCategory(pluginName string) (string, string) {
	key := strings.TrimSpace(pluginName)
	if key == "" {
		return "general", "General"
	}
	return key, categoryTitleFromKey(key)
}

func categoryKeyFromTitle(title string) string {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(title)))
	if len(parts) == 0 {
		return "general"
	}
	for i, part := range parts {
		parts[i] = strings.Trim(part, "_-")
	}
	return strings.Join(parts, "-")
}

func categoryTitleFromKey(key string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(key), func(r rune) bool {
		return r == '-' || r == '_' || unicode.IsSpace(r)
	})
	if len(parts) == 0 {
		return "General"
	}
	for i, part := range parts {
		part = strings.ToLower(part)
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func validateRemoteSkillSource(source string) *skillsCommandError {
	source = strings.TrimSpace(source)
	if source == "" {
		return &skillsCommandError{Code: rp.CodeInvalidArgument, Message: "source is required"}
	}
	lower := strings.ToLower(source)
	if strings.Contains(source, `\`) || strings.Contains(source, "..") ||
		strings.HasPrefix(source, "/") || regexp.MustCompile(`^[A-Za-z]:`).MatchString(source) ||
		strings.HasPrefix(lower, "git@") || strings.HasPrefix(lower, "ssh://") || strings.HasPrefix(lower, "file://") {
		return &skillsCommandError{Code: rp.CodeForbidden, Message: "source is not an allowed remote skill source"}
	}
	if skillSourceRepoPattern.MatchString(source) {
		parts := strings.Split(source, "/")
		if skillSourceSlugPattern.MatchString(parts[0]) && skillSourceSlugPattern.MatchString(parts[1]) {
			return nil
		}
	}
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return &skillsCommandError{Code: rp.CodeForbidden, Message: "source is not an allowed remote skill source"}
	}
	host := strings.ToLower(parsed.Host)
	if host == "github.com" || host == "www.github.com" {
		path := strings.Trim(strings.TrimSuffix(parsed.EscapedPath(), ".git"), "/")
		parts := strings.Split(path, "/")
		if len(parts) == 2 && skillSourceSlugPattern.MatchString(parts[0]) && skillSourceSlugPattern.MatchString(parts[1]) {
			return nil
		}
	}
	if strings.Contains(parsed.EscapedPath(), "/.well-known/agent-skills") || strings.Contains(parsed.EscapedPath(), "/.well-known/skills") {
		return nil
	}
	return &skillsCommandError{Code: rp.CodeForbidden, Message: "source is not an allowed remote skill source"}
}

func normalizeSkillNames(skills []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		name := strings.TrimSpace(skill)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func validateSkillNames(skills []string) *skillsCommandError {
	for _, skill := range skills {
		if !skillNamePattern.MatchString(skill) {
			return &skillsCommandError{Code: rp.CodeInvalidArgument, Message: "skill name is invalid"}
		}
	}
	return nil
}

func skillsCommandFailed(result skillsCommandResult) bool {
	return result.Err != nil || result.ExitCode != 0
}

func skillsCommandUnavailable(result skillsCommandResult) bool {
	if result.Err == nil {
		return false
	}
	if errors.Is(result.Err, exec.ErrNotFound) {
		return true
	}
	lower := strings.ToLower(result.Err.Error())
	return strings.Contains(lower, "executable file not found") ||
		strings.Contains(lower, "file not found") ||
		strings.Contains(lower, "not found in %path%")
}

func skillsResultSummary(result skillsCommandResult) string {
	segment := lastNonEmptySegment(result.Stderr)
	if segment == "" {
		segment = lastNonEmptySegment(result.Stdout)
	}
	if segment == "" {
		return fmt.Sprintf("skills command failed with exit code %d", result.ExitCode)
	}
	return fmt.Sprintf("exit code %d: %s", result.ExitCode, truncateRunes(segment, 500))
}
