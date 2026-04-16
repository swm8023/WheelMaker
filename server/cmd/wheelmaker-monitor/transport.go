package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	hubpkg "github.com/swm8023/wheelmaker/internal/hub"
	rp "github.com/swm8023/wheelmaker/internal/protocol"
	"github.com/swm8023/wheelmaker/internal/shared"
)

type HubInfo struct {
	HubID  string `json:"hubId"`
	Online bool   `json:"online"`
}

type MonitorLogRequest struct {
	HubID string `json:"hubId"`
	File  string `json:"file,omitempty"`
	Level string `json:"level,omitempty"`
	Tail  int    `json:"tail,omitempty"`
}

type HubTransport interface {
	ListHub(ctx context.Context) ([]HubInfo, error)
	MonitorStatus(ctx context.Context, hubID string) (*ServiceStatus, error)
	MonitorLog(ctx context.Context, req MonitorLogRequest) (*LogResult, error)
	MonitorDB(ctx context.Context, hubID string) (*DBTablesResult, error)
	MonitorAction(ctx context.Context, hubID string, action string) error
	ProjectList(ctx context.Context, hubID string) ([]RegistryProject, error)
}

type monitorTransportConfig struct {
	BaseDir      string
	Registry     shared.RegistryConfig
	Projects     []shared.ProjectConfig
	DefaultHubID string
}

func newHubTransport(cfg monitorTransportConfig) HubTransport {
	if shouldUseRegistryTransport(cfg.Registry) {
		server := strings.TrimSpace(cfg.Registry.Server)
		if server == "" {
			server = "127.0.0.1"
		}
		port := cfg.Registry.Port
		if port == 0 {
			port = 9630
		}
		return &registryHubTransport{
			server: server,
			port:   port,
			token:  strings.TrimSpace(cfg.Registry.Token),
		}
	}
	hubID := strings.TrimSpace(cfg.DefaultHubID)
	if hubID == "" {
		hubID = strings.TrimSpace(cfg.Registry.HubID)
	}
	if hubID == "" {
		hubID = "local"
	}
	return &directHubTransport{
		hubID:    hubID,
		core:     hubpkg.NewMonitorCore(cfg.BaseDir),
		projects: cfg.Projects,
	}
}

func shouldUseRegistryTransport(cfg shared.RegistryConfig) bool {
	return cfg.Listen || strings.TrimSpace(cfg.Server) != "" || cfg.Port > 0
}

type directHubTransport struct {
	hubID    string
	core     *hubpkg.MonitorCore
	projects []shared.ProjectConfig
}

func (d *directHubTransport) ensureHub(hubID string) error {
	want := strings.TrimSpace(hubID)
	if want == "" || want == d.hubID {
		return nil
	}
	return fmt.Errorf("hub not found: %s", want)
}

func (d *directHubTransport) ListHub(context.Context) ([]HubInfo, error) {
	return []HubInfo{{HubID: d.hubID, Online: true}}, nil
}

func (d *directHubTransport) MonitorStatus(_ context.Context, hubID string) (*ServiceStatus, error) {
	if err := d.ensureHub(hubID); err != nil {
		return nil, err
	}
	status, err := d.core.GetServiceStatus()
	if err != nil {
		return nil, err
	}
	return &ServiceStatus{
		Running:   status.Running,
		Processes: mapProcessInfo(status.Processes),
		Timestamp: status.Timestamp,
	}, nil
}

func mapProcessInfo(in []hubpkg.ProcessInfo) []ProcessInfo {
	if len(in) == 0 {
		return nil
	}
	out := make([]ProcessInfo, 0, len(in))
	for _, item := range in {
		out = append(out, ProcessInfo{
			PID:       item.PID,
			Role:      item.Role,
			StartedAt: item.StartedAt,
		})
	}
	return out
}

func (d *directHubTransport) MonitorLog(_ context.Context, req MonitorLogRequest) (*LogResult, error) {
	if err := d.ensureHub(req.HubID); err != nil {
		return nil, err
	}
	result, err := d.core.GetLogs(req.File, req.Level, req.Tail)
	if err != nil {
		return nil, err
	}
	return &LogResult{
		File:    result.File,
		Entries: mapLogEntries(result.Entries),
		Total:   result.Total,
	}, nil
}

func mapLogEntries(in []hubpkg.LogEntry) []LogEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]LogEntry, 0, len(in))
	for _, item := range in {
		out = append(out, LogEntry{
			Time:    item.Time,
			Level:   item.Level,
			Message: item.Message,
			Raw:     item.Raw,
		})
	}
	return out
}

func (d *directHubTransport) MonitorDB(_ context.Context, hubID string) (*DBTablesResult, error) {
	if err := d.ensureHub(hubID); err != nil {
		return nil, err
	}
	result := d.core.GetDBTables()
	return &DBTablesResult{
		Tables: mapDBTables(result.Tables),
		Error:  result.Error,
	}, nil
}

func mapDBTables(in []hubpkg.DBTable) []DBTable {
	if len(in) == 0 {
		return nil
	}
	out := make([]DBTable, 0, len(in))
	for _, item := range in {
		out = append(out, DBTable{
			Name:    item.Name,
			Columns: item.Columns,
			Rows:    item.Rows,
		})
	}
	return out
}

func (d *directHubTransport) MonitorAction(_ context.Context, hubID string, action string) error {
	if err := d.ensureHub(hubID); err != nil {
		return err
	}
	return d.core.ExecuteAction(action)
}

func (d *directHubTransport) ProjectList(_ context.Context, hubID string) ([]RegistryProject, error) {
	if err := d.ensureHub(hubID); err != nil {
		return nil, err
	}
	items := make([]RegistryProject, 0, len(d.projects))
	for _, p := range d.projects {
		items = append(items, RegistryProject{
			ProjectID: rp.ProjectID(d.hubID, p.Name),
			Name:      p.Name,
			Path:      p.Path,
			Online:    true,
			Agent:     "unknown",
			IMType:    p.IMType(),
		})
	}
	return items, nil
}

type registryHubTransport struct {
	server string
	port   int
	token  string
}

func (r *registryHubTransport) ListHub(ctx context.Context) ([]HubInfo, error) {
	var payload struct {
		Hubs []HubInfo `json:"hubs"`
	}
	if err := r.call(ctx, "monitor.listHub", nil, &payload); err != nil {
		return nil, err
	}
	return payload.Hubs, nil
}

func (r *registryHubTransport) MonitorStatus(ctx context.Context, hubID string) (*ServiceStatus, error) {
	var out ServiceStatus
	if err := r.call(ctx, "monitor.status", rp.MonitorHubRefPayload{HubID: strings.TrimSpace(hubID)}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *registryHubTransport) MonitorLog(ctx context.Context, req MonitorLogRequest) (*LogResult, error) {
	var out LogResult
	payload := rp.MonitorLogPayload{
		HubID: strings.TrimSpace(req.HubID),
		File:  req.File,
		Level: req.Level,
		Tail:  req.Tail,
	}
	if err := r.call(ctx, "monitor.log", payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *registryHubTransport) MonitorDB(ctx context.Context, hubID string) (*DBTablesResult, error) {
	var out DBTablesResult
	if err := r.call(ctx, "monitor.db", rp.MonitorHubRefPayload{HubID: strings.TrimSpace(hubID)}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *registryHubTransport) MonitorAction(ctx context.Context, hubID string, action string) error {
	var out struct {
		OK bool `json:"ok"`
	}
	payload := rp.MonitorActionPayload{
		HubID:  strings.TrimSpace(hubID),
		Action: strings.TrimSpace(action),
	}
	return r.call(ctx, "monitor.action", payload, &out)
}

func (r *registryHubTransport) ProjectList(ctx context.Context, hubID string) ([]RegistryProject, error) {
	var payload struct {
		Projects []RegistryProject `json:"projects"`
	}
	if err := r.call(ctx, "project.list", nil, &payload); err != nil {
		return nil, err
	}
	filtered := make([]RegistryProject, 0, len(payload.Projects))
	for _, item := range payload.Projects {
		if strings.TrimSpace(hubID) == "" {
			filtered = append(filtered, item)
			continue
		}
		if projectHubID(item.ProjectID) == strings.TrimSpace(hubID) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func projectHubID(projectID string) string {
	parts := strings.SplitN(strings.TrimSpace(projectID), ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

func (r *registryHubTransport) call(ctx context.Context, method string, payload any, out any) error {
	conn, err := r.connect(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := writeRequest(conn, 2, method, payload); err != nil {
		return err
	}
	resp, err := readEnvelope(conn)
	if err != nil {
		return err
	}
	if resp.Type == "error" {
		var ep rp.ErrorPayload
		_ = json.Unmarshal(resp.Payload, &ep)
		if strings.TrimSpace(ep.Message) == "" {
			ep.Message = "registry request failed"
		}
		return fmt.Errorf("%s: %s", method, ep.Message)
	}
	if out == nil {
		return nil
	}
	if len(resp.Payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp.Payload, out); err != nil {
		return fmt.Errorf("%s decode payload: %w", method, err)
	}
	return nil
}

func (r *registryHubTransport) connect(ctx context.Context) (*websocket.Conn, error) {
	timeout := 5 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d > 0 {
			timeout = d
		}
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	u := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", r.server, r.port), Path: "/ws"}
	dialer := websocket.Dialer{HandshakeTimeout: timeout}
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("dial registry: %w", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	if err := writeRequest(conn, 1, "connect.init", rp.ConnectInitPayload{
		ClientName:      "wheelmaker-monitor",
		ClientVersion:   "0.1.0",
		ProtocolVersion: rp.DefaultProtocolVersion,
		Role:            "monitor",
		Token:           r.token,
	}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	initResp, err := readEnvelope(conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if initResp.Type == "error" {
		var ep rp.ErrorPayload
		_ = json.Unmarshal(initResp.Payload, &ep)
		_ = conn.Close()
		return nil, fmt.Errorf("connect.init: %s", ep.Message)
	}
	return conn, nil
}

func writeRequest(conn *websocket.Conn, reqID int64, method string, payload any) error {
	msg := map[string]any{
		"requestId": reqID,
		"type":      "request",
		"method":    method,
	}
	if payload != nil {
		msg["payload"] = payload
	}
	if err := conn.WriteJSON(msg); err != nil {
		return fmt.Errorf("write %s: %w", method, err)
	}
	return nil
}

func readEnvelope(conn *websocket.Conn) (rp.Envelope, error) {
	var env rp.Envelope
	if err := conn.ReadJSON(&env); err != nil {
		return rp.Envelope{}, fmt.Errorf("read response: %w", err)
	}
	return env, nil
}
