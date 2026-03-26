package registry

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/swm8023/wheelmaker/internal/logger"
)

// ReporterConfig controls hub->registry reporting behavior.
type ReporterConfig struct {
	Server   string
	Port     int
	Token    string
	HubID    string
	Interval time.Duration
}

// Reporter periodically reports project snapshots to a registry server.
type Reporter struct {
	cfg      ReporterConfig
	projects []ProjectInfo
}

// NewReporter creates a Reporter.
func NewReporter(cfg ReporterConfig, projects []ProjectInfo) *Reporter {
	if cfg.Port == 0 {
		cfg.Port = 9630
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 15 * time.Second
	}
	if strings.TrimSpace(cfg.HubID) == "" {
		cfg.HubID = "wheelmaker-hub"
	}
	cp := make([]ProjectInfo, len(projects))
	copy(cp, projects)
	return &Reporter{cfg: cfg, projects: cp}
}

// Run reports once immediately, then repeats at interval until ctx is cancelled.
func (r *Reporter) Run(ctx context.Context) error {
	if err := r.ReportOnce(ctx); err != nil {
		logger.Warn("registry reporter: initial report failed: %v", err)
	}
	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.ReportOnce(ctx); err != nil {
				logger.Warn("registry reporter: periodic report failed: %v", err)
			}
		}
	}
}

// ReportOnce sends hello/auth/reportProjects in one short-lived connection.
func (r *Reporter) ReportOnce(ctx context.Context) error {
	wsURL, err := buildWSURL(r.cfg.Server, r.cfg.Port)
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial registry %s: %w", wsURL, err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(envelope{
		Version: defaultProtocolVersion,
		Type:    "request",
		Method:  "hello",
		Payload: mustRaw(map[string]any{
			"clientName":      "wheelmaker-hub",
			"clientVersion":   "0.1.0",
			"protocolVersion": defaultProtocolVersion,
		}),
	}); err != nil {
		return err
	}
	if _, err := readAck(conn); err != nil {
		return fmt.Errorf("hello failed: %w", err)
	}

	if r.cfg.Token != "" {
		if err := conn.WriteJSON(envelope{
			Version: defaultProtocolVersion,
			Type:    "request",
			Method:  "auth",
			Payload: mustRaw(map[string]any{"token": r.cfg.Token}),
		}); err != nil {
			return err
		}
		if _, err := readAck(conn); err != nil {
			return fmt.Errorf("auth failed: %w", err)
		}
	}

	if err := conn.WriteJSON(envelope{
		Version: defaultProtocolVersion,
		Type:    "request",
		Method:  "registry.reportProjects",
		Payload: mustRaw(map[string]any{
			"hubId":    r.cfg.HubID,
			"projects": r.projects,
		}),
	}); err != nil {
		return err
	}
	if _, err := readAck(conn); err != nil {
		return fmt.Errorf("registry.reportProjects failed: %w", err)
	}
	return nil
}

func readAck(conn *websocket.Conn) (envelope, error) {
	var resp envelope
	if err := conn.ReadJSON(&resp); err != nil {
		return envelope{}, err
	}
	if resp.Type == "error" && resp.Error != nil {
		return resp, fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}
	return resp, nil
}

func buildWSURL(server string, port int) (string, error) {
	base := strings.TrimSpace(server)
	if base == "" {
		base = "127.0.0.1"
	}
	if strings.HasPrefix(base, "ws://") || strings.HasPrefix(base, "wss://") ||
		strings.HasPrefix(base, "http://") || strings.HasPrefix(base, "https://") {
		u, err := url.Parse(base)
		if err != nil {
			return "", fmt.Errorf("invalid registry server %q: %w", base, err)
		}
		switch u.Scheme {
		case "http":
			u.Scheme = "ws"
		case "https":
			u.Scheme = "wss"
		}
		if u.Path == "" || u.Path == "/" {
			u.Path = "/ws"
		}
		return u.String(), nil
	}
	host := base
	if strings.Contains(base, ":") {
		host = base
	} else {
		host = fmt.Sprintf("%s:%d", base, port)
	}
	return "ws://" + host + "/ws", nil
}
