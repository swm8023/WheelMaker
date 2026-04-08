package hub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/swm8023/wheelmaker/internal/hub/client"
	im "github.com/swm8023/wheelmaker/internal/im"
	imapp "github.com/swm8023/wheelmaker/internal/im/app"
	imfeishu "github.com/swm8023/wheelmaker/internal/im/feishu"
	rp "github.com/swm8023/wheelmaker/internal/protocol"
	shared "github.com/swm8023/wheelmaker/internal/shared"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	feishuVerificationToken = ""
	feishuEncryptKey        = ""
)

// Hub orchestrates one or more WheelMaker project clients.
// Each project has its own IM channel, agent session, and state partition.
type Hub struct {
	cfg      *shared.AppConfig
	dbPath   string
	clients  []*client.Client
	regSync  *Reporter
	regMu    sync.Mutex
	lastProj map[string]ProjectInfo
}

// New creates a Hub from the given config and client DB path.
// hub.Start() must be called before hub.Run().
func New(cfg *shared.AppConfig, dbPath string) *Hub {
	return &Hub{
		cfg:      cfg,
		dbPath:   dbPath,
		lastProj: map[string]ProjectInfo{},
	}
}

// Start validates config, creates one client.Client per project, and starts each client.
// Returns an error if any project has an unsupported IM type.
func (h *Hub) Start(ctx context.Context) error {
	shared.Info("hub: start projects=%d", len(h.cfg.Projects))
	for _, pc := range h.cfg.Projects {
		shared.Info("hub: build client project=%s im=%s", pc.Name, pc.IM.Type)
		c, err := h.buildClient(ctx, pc)
		if err != nil {
			shared.Error("hub: build client failed project=%s err=%v", pc.Name, err)
			return fmt.Errorf("hub: project %q: %w", pc.Name, err)
		}
		h.clients = append(h.clients, c)
		shared.Info("hub: client ready project=%s", pc.Name)
	}
	h.setupRegistrySync()
	shared.Info("hub: start completed projects=%d", len(h.clients))
	return nil
}

// buildClient creates, configures, and starts a client.Client for one project.
func (h *Hub) buildClient(ctx context.Context, pc shared.ProjectConfig) (*client.Client, error) {
	// Resolve working directory.
	cwd := pc.Path
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			cwd = "."
		}
	}
	return h.buildIMClient(ctx, pc, cwd)
}

func (h *Hub) buildIMClient(ctx context.Context, pc shared.ProjectConfig, cwd string) (*client.Client, error) {
	shared.Info("hub: opening store project=%s db=%s", pc.Name, h.dbPath)
	store, err := client.NewStore(h.dbPath)
	if err != nil {
		shared.Error("hub: open store failed project=%s err=%v", pc.Name, err)
		return nil, fmt.Errorf("new store: %w", err)
	}
	c := client.New(store, pc.Client.Agent, pc.Name, cwd)
	c.SetYOLO(pc.YOLO)

	router := im.NewRouter(c, im.NewMemoryHistoryStore())
	switch pc.IM.Type {
	case "feishu":
		shared.Info("hub: register channel project=%s type=feishu", pc.Name)
		if err := router.RegisterChannel(imfeishu.New(imfeishu.Config{
			AppID:             pc.IM.AppID,
			AppSecret:         pc.IM.AppSecret,
			VerificationToken: feishuVerificationToken,
			EncryptKey:        feishuEncryptKey,
			YOLO:              pc.YOLO,
			BlockedUpdates:    pc.Client.IMFilter.Block,
		})); err != nil {
			shared.Error("hub: register channel failed project=%s type=feishu err=%v", pc.Name, err)
			_ = c.Close()
			return nil, err
		}
	case "app":
		shared.Info("hub: register channel project=%s type=app", pc.Name)
		if err := router.RegisterChannel(imapp.New()); err != nil {
			shared.Error("hub: register channel failed project=%s type=app err=%v", pc.Name, err)
			_ = c.Close()
			return nil, err
		}
	default:
		shared.Error("hub: build client failed project=%s err=unsupported im.type %q", pc.Name, pc.IM.Type)
		_ = c.Close()
		return nil, fmt.Errorf("unsupported im.type %q (supported: feishu, app)", pc.IM.Type)
	}
	c.SetIMRouter(router)
	shared.Info("hub: starting client project=%s", pc.Name)
	if err := c.Start(ctx); err != nil {
		shared.Error("hub: start client failed project=%s err=%v", pc.Name, err)
		_ = c.Close()
		return nil, fmt.Errorf("start: %w", err)
	}
	shared.Info("hub: client started project=%s", pc.Name)
	return c, nil
}

// Run starts each project client in a goroutine and blocks until ctx is done
// or all goroutines have exited. Individual project errors are logged to stderr;
// only ctx cancellation terminates the run.
func (h *Hub) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	if h.regSync != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.monitorRegistryProjectState(ctx)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.regSync.Run(ctx); err != nil && ctx.Err() == nil {
				shared.Error("wheelmaker: registry sync error: %v", err)
			}
		}()
	}
	for _, c := range h.clients {
		wg.Add(1)
		go func(c *client.Client) {
			defer wg.Done()
			if err := c.Run(ctx); err != nil && ctx.Err() == nil {
				shared.Error("wheelmaker: project run error: %v", err)
			}
		}(c)
	}
	wg.Wait()
	return nil
}

// Close calls Close() on all project clients, collecting any errors.
func (h *Hub) Close() error {
	var errs []error
	for _, c := range h.clients {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("hub close errors: %v", errs)
	}
	return nil
}

func (h *Hub) setupRegistrySync() {
	cfg := h.cfg.Registry
	if !cfg.Listen && strings.TrimSpace(cfg.Server) == "" && cfg.Port == 0 {
		return
	}

	port := cfg.Port
	if port == 0 {
		port = 9630
	}
	host := strings.TrimSpace(cfg.Server)
	if host == "" {
		host = "127.0.0.1"
	}

	hubID := strings.TrimSpace(cfg.HubID)
	if hubID == "" {
		if hn, err := os.Hostname(); err == nil && strings.TrimSpace(hn) != "" {
			hubID = hn
		} else {
			hubID = "wheelmaker-hub"
		}
	}

	projects := make([]ProjectInfo, 0, len(h.cfg.Projects))
	for _, p := range h.cfg.Projects {
		projects = append(projects, h.collectProjectInfo(p))
	}

	rep := NewReporter(ReporterConfig{
		Server:            host,
		Port:              port,
		Token:             cfg.Token,
		HubID:             hubID,
		ReconnectInterval: 2 * time.Second,
	}, projects)
	h.regSync = rep
	h.regMu.Lock()
	for _, project := range projects {
		h.lastProj[project.Name] = project
	}
	h.regMu.Unlock()
}

func (h *Hub) monitorRegistryProjectState(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	checkAndReport := func() {
		for _, cfgProject := range h.cfg.Projects {
			project := h.collectProjectInfo(cfgProject)
			h.regMu.Lock()
			previous := h.lastProj[project.Name]
			same := sameProjectInfo(previous, project)
			if !same {
				h.lastProj[project.Name] = project
			}
			h.regMu.Unlock()
			if same {
				continue
			}
			if err := h.regSync.UpdateProject(project); err != nil {
				shared.Warn("hub registry: updateProject failed name=%s err=%v", project.Name, err)
			}
		}
	}

	checkAndReport()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkAndReport()
		}
	}
}

func (h *Hub) collectProjectInfo(cfgProject shared.ProjectConfig) ProjectInfo {
	path := strings.TrimSpace(cfgProject.Path)
	if path == "" {
		if cwd, err := os.Getwd(); err == nil {
			path = cwd
		} else {
			path = "."
		}
	}
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}
	info := ProjectInfo{
		Name:   cfgProject.Name,
		Path:   path,
		Online: true,
		Agent:  cfgProject.Client.Agent,
		IMType: cfgProject.IM.Type,
	}
	gitState := collectGitState(path)
	info.Git = gitState
	info.ProjectRev = hubHashLines(gitState.GitRev, gitState.WorktreeRev)
	return info
}

func collectGitState(projectPath string) rp.ProjectGitState {
	branch, branchErr := runGitLocal(projectPath, "rev-parse", "--abbrev-ref", "HEAD")
	headSHA, shaErr := runGitLocal(projectPath, "rev-parse", "HEAD")
	statusRaw, statusErr := runGitLocal(projectPath, "status", "--porcelain")
	if branchErr != nil || shaErr != nil || statusErr != nil {
		return rp.ProjectGitState{}
	}
	normalizedStatus := normalizeGitStatus(statusRaw)
	dirty := strings.TrimSpace(normalizedStatus) != ""
	gitRev := hubHashLines(strings.TrimSpace(branch), strings.TrimSpace(headSHA), boolString(dirty))
	worktreeRev := hubHashBytes([]byte(normalizedStatus))
	return rp.ProjectGitState{
		Branch:      strings.TrimSpace(branch),
		HeadSHA:     strings.TrimSpace(headSHA),
		Dirty:       dirty,
		GitRev:      gitRev,
		WorktreeRev: worktreeRev,
	}
}

func runGitLocal(projectPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = projectPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func normalizeGitStatus(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := []string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(strings.TrimSpace(line), " ")
		if line == "" {
			continue
		}
		lines = append(lines, strings.ReplaceAll(line, "\\", "/"))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func hubHashLines(parts ...string) string {
	return hubHashBytes([]byte(strings.Join(parts, "\n")))
}

func hubHashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sameProjectInfo(a, b ProjectInfo) bool {
	return a.Name == b.Name &&
		a.Path == b.Path &&
		a.Online == b.Online &&
		a.Agent == b.Agent &&
		a.IMType == b.IMType &&
		a.ProjectRev == b.ProjectRev &&
		a.Git.Branch == b.Git.Branch &&
		a.Git.HeadSHA == b.Git.HeadSHA &&
		a.Git.Dirty == b.Git.Dirty &&
		a.Git.GitRev == b.Git.GitRev &&
		a.Git.WorktreeRev == b.Git.WorktreeRev
}
