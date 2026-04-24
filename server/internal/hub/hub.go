package hub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/agent"
	"github.com/swm8023/wheelmaker/internal/hub/client"
	im "github.com/swm8023/wheelmaker/internal/im"
	imapp "github.com/swm8023/wheelmaker/internal/im/app"
	imfeishu "github.com/swm8023/wheelmaker/internal/im/feishu"
	rp "github.com/swm8023/wheelmaker/internal/protocol"
	logger "github.com/swm8023/wheelmaker/internal/shared"
)

const (
	feishuVerificationToken = ""
	feishuEncryptKey        = ""
)

// Hub orchestrates one or more WheelMaker project clients.
// Each project has its own IM channel, agent session, and state partition.
type Hub struct {
	cfg           *logger.AppConfig
	dbPath        string
	clients       []*client.Client
	regSync       *Reporter
	appIM         map[string]*imapp.Channel
	clientsByName map[string]*client.Client
}

// New creates a Hub from the given config and client DB path.
// hub.Start() must be called before hub.Run().
func New(cfg *logger.AppConfig, dbPath string) *Hub {
	return &Hub{
		cfg:           cfg,
		dbPath:        dbPath,
		appIM:         map[string]*imapp.Channel{},
		clientsByName: map[string]*client.Client{},
	}
}

// Start validates config, creates one client.Client per project, and starts each client.
// Returns an error if any project has an unsupported IM type.
func (h *Hub) Start(ctx context.Context) error {
	hubLogger("").Info("start projects=%d", len(h.cfg.Projects))
	if err := client.CheckStoreSchema(h.dbPath); err != nil {
		if client.IsStoreSchemaMismatch(err) {
			dbDir := filepath.Dir(h.dbPath)
			return fmt.Errorf("hub db schema mismatch: %w; delete local db directory %q and restart server", err, dbDir)
		}
		return fmt.Errorf("hub db schema check: %w", err)
	}
	for _, pc := range h.cfg.Projects {
		hubLogger(pc.Name).Info("build client im=%s", pc.IMType())
		c, err := h.buildClient(ctx, pc)
		if err != nil {
			hubLogger(pc.Name).Error("build client failed err=%v", err)
			return fmt.Errorf("hub: project %q: %w", pc.Name, err)
		}
		h.clients = append(h.clients, c)
		hubLogger(pc.Name).Info("client ready")
	}
	h.setupRegistrySync()
	hubLogger("").Info("start completed projects=%d", len(h.clients))
	return nil
}

// buildClient creates, configures, and starts a client.Client for one project.
func (h *Hub) buildClient(ctx context.Context, pc logger.ProjectConfig) (*client.Client, error) {
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

func (h *Hub) buildIMClient(ctx context.Context, pc logger.ProjectConfig, cwd string) (*client.Client, error) {
	hubLogger(pc.Name).Info("opening store db=%s", h.dbPath)
	store, err := client.NewStore(h.dbPath)
	if err != nil {
		hubLogger(pc.Name).Error("open store failed err=%v", err)
		return nil, fmt.Errorf("new store: %w", err)
	}
	c := client.New(store, pc.Name, cwd)
	c.SetSessionViewSink(c)
	h.clientsByName[pc.Name] = c

	router := im.NewRouter(c, im.NewMemoryHistoryStore())
	if pc.Feishu != nil && !pc.HasFeishu() {
		hubLogger(pc.Name).Error("build client failed err=invalid feishu config")
		_ = c.Close()
		return nil, fmt.Errorf("invalid feishu config: both app_id and app_secret are required")
	}
	if pc.HasFeishu() {
		hubLogger(pc.Name).Info("register channel type=feishu")
		if err := router.RegisterChannel(imfeishu.New(imfeishu.Config{
			AppID:             pc.Feishu.AppID,
			AppSecret:         pc.Feishu.AppSecret,
			VerificationToken: feishuVerificationToken,
			EncryptKey:        feishuEncryptKey,
			BlockedUpdates:    pc.IMFilter.Block,
		})); err != nil {
			hubLogger(pc.Name).Error("register channel failed type=feishu err=%v", err)
			_ = c.Close()
			return nil, err
		}
	}
	hubLogger(pc.Name).Info("register channel type=app")
	appChannel := imapp.New()
	if err := router.RegisterChannel(appChannel); err != nil {
		hubLogger(pc.Name).Error("register channel failed type=app err=%v", err)
		_ = c.Close()
		return nil, err
	}
	h.appIM[pc.Name] = appChannel
	c.SetIMRouter(router)
	hubLogger(pc.Name).Info("starting client")
	if err := c.Start(ctx); err != nil {
		hubLogger(pc.Name).Error("start client failed err=%v", err)
		_ = c.Close()
		return nil, fmt.Errorf("start: %w", err)
	}
	hubLogger(pc.Name).Info("client started")
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
			if err := h.regSync.Run(ctx); err != nil && ctx.Err() == nil {
				registryLogger("").Error("sync error: %v", err)
			}
		}()
	}
	for _, c := range h.clients {
		wg.Add(1)
		go func(c *client.Client) {
			defer wg.Done()
			if err := c.Run(ctx); err != nil && ctx.Err() == nil {
				hubLogger("").Error("project run error: %v", err)
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
		MonitorBaseDir:    filepath.Dir(filepath.Dir(h.dbPath)),
	}, projects)
	rep.SetMonitorResetSessionPromptState(func() {
		for _, projectClient := range h.clientsByName {
			if projectClient != nil {
				projectClient.ResetSessionPromptState()
			}
		}
	})
	for _, project := range projects {
		projectClient := h.clientsByName[project.Name]
		appChannel := h.appIM[project.Name]
		projectID := rp.ProjectID(hubID, project.Name)
		if projectClient != nil {
			projectClient.SetSessionEventPublisher(func(method string, payload any) error {
				return rep.PublishProjectEvent(projectID, method, payload)
			})
			rep.RegisterSessionHandler(projectID, projectClient)
		}
		if appChannel != nil {
			rep.RegisterChatHandler(projectID, appChannel)
		}
	}
	h.regSync = rep
}

func (h *Hub) collectProjectInfo(cfgProject logger.ProjectConfig) ProjectInfo {
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
		Agent:  "auto",
		IMType: cfgProject.IMType(),
	}
	if preferred := strings.TrimSpace(agent.DefaultACPFactory().PreferredName()); preferred != "" {
		info.Agent = preferred
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
