package hub

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/swm8023/wheelmaker/internal/agent"
	"github.com/swm8023/wheelmaker/internal/agent/claude"
	"github.com/swm8023/wheelmaker/internal/agent/codex"
	"github.com/swm8023/wheelmaker/internal/agent/mock"
	"github.com/swm8023/wheelmaker/internal/client"
	"github.com/swm8023/wheelmaker/internal/im"
	"github.com/swm8023/wheelmaker/internal/im/console"
	"github.com/swm8023/wheelmaker/internal/im/feishu"
	"github.com/swm8023/wheelmaker/internal/im/mobile"
)

// Hub orchestrates one or more WheelMaker project clients.
// Each project has its own IM channel, agent session, and state partition.
type Hub struct {
	cfg       *Config
	statePath string
	clients   []*client.Client

	debugMu     sync.Mutex
	debugFile   *os.File
	debugWriter io.Writer
}

// New creates a Hub from the given config and state file path.
// hub.Start() must be called before hub.Run().
func New(cfg *Config, statePath string) *Hub {
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
	return nil
}

// buildClient creates, configures, and starts a client.Client for one project.
func (h *Hub) buildClient(ctx context.Context, pc ProjectConfig) (*client.Client, error) {
	// Resolve working directory.
	cwd := pc.Client.Path
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

	// Enable ACP JSON debug logging for projects with debug=true.
	if pc.Debug {
		c.SetDebugLogger(h.getDebugWriter())
	}

	// Register all known agent factories so users can switch between them at runtime.
	c.RegisterAgent("codex", func(_ string, _ map[string]string) agent.Agent {
		return codex.New(codex.Config{})
	})
	c.RegisterAgent("claude", func(_ string, _ map[string]string) agent.Agent {
		return claude.New(claude.Config{})
	})
	c.RegisterAgent("mock", func(_ string, _ map[string]string) agent.Agent {
		return mock.New()
	})

	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	return c, nil
}

// buildIM creates the im.Bridge for a project's IM config.
func (h *Hub) buildIM(pc ProjectConfig) (*im.Bridge, error) {
	switch pc.IM.Type {
	case "console":
		return im.New(console.New(pc.Name, pc.Debug)), nil
	case "feishu":
		return im.New(feishu.New(feishu.Config{
			AppID:             pc.IM.AppID,
			AppSecret:         pc.IM.AppSecret,
			VerificationToken: h.cfg.Feishu.VerificationToken,
			EncryptKey:        h.cfg.Feishu.EncryptKey,
			Debug:             pc.Debug,
		})), nil
	case "mobile":
		addr := fmt.Sprintf(":%d", pc.IM.Mobile.Port)
		if pc.IM.Mobile.Port == 0 {
			addr = ":9527"
		}
		return im.New(mobile.New(mobile.Config{
			Addr:  addr,
			Token: pc.IM.Mobile.Token,
		})), nil
	default:
		return nil, fmt.Errorf("unknown im.type %q (supported: console, feishu, mobile)", pc.IM.Type)
	}
}

// Run starts each project client in a goroutine and blocks until ctx is done
// or all goroutines have exited. Individual project errors are logged to stderr;
// only ctx cancellation terminates the run.
func (h *Hub) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, c := range h.clients {
		wg.Add(1)
		go func(c *client.Client) {
			defer wg.Done()
			if err := c.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("wheelmaker: project run error: %v", err)
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
	h.debugMu.Lock()
	if h.debugFile != nil {
		if err := h.debugFile.Close(); err != nil {
			errs = append(errs, err)
		}
		h.debugFile = nil
		h.debugWriter = nil
	}
	h.debugMu.Unlock()
	if len(errs) > 0 {
		return fmt.Errorf("hub close errors: %v", errs)
	}
	return nil
}

func (h *Hub) getDebugWriter() io.Writer {
	h.debugMu.Lock()
	defer h.debugMu.Unlock()
	if h.debugWriter != nil {
		return h.debugWriter
	}

	f, err := os.OpenFile("debug.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("hub: open debug.log failed: %v", err)
		h.debugWriter = log.Writer()
		return h.debugWriter
	}
	h.debugFile = f
	h.debugWriter = io.MultiWriter(log.Writer(), f)
	log.Printf("hub: debug log enabled at debug.log")
	return h.debugWriter
}
