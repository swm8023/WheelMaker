package hub

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/agent"
	"github.com/swm8023/wheelmaker/internal/hub/agent/claude"
	"github.com/swm8023/wheelmaker/internal/hub/agent/codex"
	"github.com/swm8023/wheelmaker/internal/hub/agent/copilot"
	"github.com/swm8023/wheelmaker/internal/hub/client"
	"github.com/swm8023/wheelmaker/internal/hub/im"
	"github.com/swm8023/wheelmaker/internal/hub/im/console"
	"github.com/swm8023/wheelmaker/internal/hub/im/feishu"
	shared "github.com/swm8023/wheelmaker/internal/shared"
)

const (
	feishuVerificationToken = ""
	feishuEncryptKey        = ""
)

// Hub orchestrates one or more WheelMaker project clients.
// Each project has its own IM channel, agent session, and state partition.
type Hub struct {
	cfg       *shared.AppConfig
	statePath string
	clients   []*client.Client
	regSync   *Reporter
}

// New creates a Hub from the given config and state file path.
// hub.Start() must be called before hub.Run().
func New(cfg *shared.AppConfig, statePath string) *Hub {
	return &Hub{cfg: cfg, statePath: statePath}
}

// Start validates config, creates one client.Client per project, and starts each client.
// Returns an error if more than one project uses type "console", or if any project
// has an unsupported IM type.
func (h *Hub) Start(ctx context.Context) error {
	// Validate: at most one console IM project.
	consoleCount := 0
	for _, pc := range h.cfg.Projects {
		if pc.IM.Type == "console" {
			consoleCount++
		}
	}
	if consoleCount > 1 {
		return errors.New("hub: at most one project may use im.type = \"console\"")
	}

	for _, pc := range h.cfg.Projects {
		c, err := h.buildClient(ctx, pc)
		if err != nil {
			return fmt.Errorf("hub: project %q: %w", pc.Name, err)
		}
		h.clients = append(h.clients, c)
	}
	h.setupRegistrySync()
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

	// Create IM channel.
	imProvider, err := h.buildIM(pc)
	if err != nil {
		return nil, err
	}

	// Create per-project state store.
	store := client.NewProjectJSONStore(h.statePath, pc.Name)

	// Create the client.
	c := client.New(store, imProvider, pc.Name, cwd)
	c.SetYOLO(pc.YOLO)

	// Enable ACP JSON debug logging for projects with debug=true.
	if pc.Debug {
		if dw := shared.DebugWriter(); dw != nil {
			c.SetDebugLogger(dw)
			imProvider.SetDebugLogger(dw)
		}
	}

	// Register all known agent factories so users can switch between them at runtime.
	c.RegisterAgent("codex", func(_ string, _ map[string]string) agent.Agent {
		return codex.New(codex.Config{})
	})
	c.RegisterAgent("claude", func(_ string, _ map[string]string) agent.Agent {
		return claude.New(claude.Config{})
	})
	c.RegisterAgent("copilot", func(_ string, _ map[string]string) agent.Agent {
		return copilot.New(copilot.Config{})
	})
	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	return c, nil
}

// buildIM creates the im.ImAdapter for a project's IM config.
func (h *Hub) buildIM(pc shared.ProjectConfig) (*im.ImAdapter, error) {
	switch pc.IM.Type {
	case "console":
		return im.New(console.New(pc.Name, pc.Debug)), nil
	case "feishu":
		return im.New(feishu.New(feishu.Config{
			AppID:             pc.IM.AppID,
			AppSecret:         pc.IM.AppSecret,
			VerificationToken: feishuVerificationToken,
			EncryptKey:        feishuEncryptKey,
			Debug:             pc.Debug,
			YOLO:              pc.YOLO,
		})), nil
	default:
		return nil, fmt.Errorf("unknown im.type %q (supported: console, feishu)", pc.IM.Type)
	}
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
		projects = append(projects, ProjectInfo{
			ID:     p.Name,
			Name:   p.Name,
			Path:   p.Path,
			Agent:  p.Client.Agent,
			IMType: p.IM.Type,
		})
	}

	h.regSync = NewReporter(ReporterConfig{
		Server:            host,
		Port:              port,
		Token:             cfg.Token,
		HubID:             hubID,
		ReconnectInterval: 2 * time.Second,
	}, projects)
}
