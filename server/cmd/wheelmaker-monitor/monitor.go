package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	rp "github.com/swm8023/wheelmaker/internal/protocol"
	"github.com/swm8023/wheelmaker/internal/shared"
	_ "modernc.org/sqlite"
)

// Monitor holds the base directory and provides all monitoring operations.
type Monitor struct {
	baseDir      string
	mu           sync.RWMutex
	transport    HubTransport
	defaultHubID string
}

const (
	wheelmakerServiceName = "WheelMaker"
	monitorServiceName    = "WheelMakerMonitor"
	updaterServiceName    = "WheelMakerUpdater"
	manualSignalFileName  = "update-now.signal"
	fullUpdateSignalToken = "full-update"
)

var errServiceNotInstalled = errors.New("service not installed")
var debugSessionPrefixRe = regexp.MustCompile(`\{([0-9a-fA-F-]{36})\s+([^}]*)\}`)

type ActionError struct {
	Code    string
	Message string
	Hint    string
}

func (e *ActionError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return "action failed"
}

// NewMonitor creates a Monitor for the given WheelMaker home directory.
func NewMonitor(baseDir string) *Monitor {
	baseDir = strings.TrimSpace(baseDir)
	cfgPath := filepath.Join(baseDir, "config.json")
	cfg := shared.AppConfig{}
	if loaded, err := shared.LoadConfig(cfgPath); err == nil && loaded != nil {
		cfg = *loaded
	}
	defaultHubID := strings.TrimSpace(cfg.Registry.HubID)
	if defaultHubID == "" {
		defaultHubID = "local"
	}
	m := &Monitor{
		baseDir:      baseDir,
		defaultHubID: defaultHubID,
	}
	m.transport = newHubTransport(monitorTransportConfig{
		BaseDir:      baseDir,
		Registry:     cfg.Registry,
		Projects:     cfg.Projects,
		DefaultHubID: defaultHubID,
	})
	return m
}

func (m *Monitor) resolveHubID(ctx context.Context, hubID string) (string, error) {
	hubID = strings.TrimSpace(hubID)
	if hubID != "" {
		return hubID, nil
	}
	if m.transport == nil {
		return m.defaultHubID, nil
	}
	hubs, err := m.transport.ListHub(ctx)
	if err == nil && len(hubs) > 0 {
		return strings.TrimSpace(hubs[0].HubID), nil
	}
	if m.defaultHubID != "" {
		return m.defaultHubID, nil
	}
	return "", errors.New("hubId is required")
}

func (m *Monitor) isLocalHub(hubID string) bool {
	h := strings.TrimSpace(hubID)
	if h == "" {
		return true
	}
	if strings.EqualFold(h, "local") {
		return true
	}
	return h == strings.TrimSpace(m.defaultHubID)
}

func (m *Monitor) ListHubs(ctx context.Context) ([]HubInfo, error) {
	if m.transport == nil {
		return []HubInfo{{HubID: m.defaultHubID, Online: true}}, nil
	}
	hubs, err := m.transport.ListHub(ctx)
	if err == nil {
		return hubs, nil
	}
	return []HubInfo{{HubID: m.defaultHubID, Online: true}}, nil
}

func (m *Monitor) GetServiceStatusByHub(ctx context.Context, hubID string) (*ServiceStatus, error) {
	resolved, err := m.resolveHubID(ctx, hubID)
	if err != nil {
		return nil, err
	}
	if m.transport == nil || m.isLocalHub(resolved) {
		return m.GetServiceStatus()
	}
	return m.transport.MonitorStatus(ctx, resolved)
}

func (m *Monitor) GetLogsByHub(ctx context.Context, hubID string, file string, level string, tail int) (*LogResult, error) {
	resolved, err := m.resolveHubID(ctx, hubID)
	if err != nil {
		return nil, err
	}
	if m.transport == nil || m.isLocalHub(resolved) {
		return m.GetLogs(file, level, tail)
	}
	return m.transport.MonitorLog(ctx, MonitorLogRequest{HubID: resolved, File: file, Level: level, Tail: tail})
}

func (m *Monitor) GetDBTablesByHub(ctx context.Context, hubID string) (*DBTablesResult, error) {
	resolved, err := m.resolveHubID(ctx, hubID)
	if err != nil {
		return nil, err
	}
	if m.transport == nil || m.isLocalHub(resolved) {
		return m.GetDBTables(), nil
	}
	return m.transport.MonitorDB(ctx, resolved)
}

func (m *Monitor) executeLocalAction(action string) error {
	switch strings.TrimSpace(action) {
	case "restart":
		return m.RestartService()
	case "stop":
		return m.StopService()
	case "start":
		return m.StartService()
	case "update-publish":
		return m.TriggerUpdatePublish()
	default:
		return fmt.Errorf("unsupported local action: %s", action)
	}
}

func (m *Monitor) ExecuteActionByHub(ctx context.Context, hubID string, action string) error {
	resolved, err := m.resolveHubID(ctx, hubID)
	if err != nil {
		return err
	}
	if m.transport == nil || m.isLocalHub(resolved) {
		return m.executeLocalAction(action)
	}

	action = strings.TrimSpace(action)
	if action == "start" {
		return &ActionError{
			Code:    "REMOTE_START_UNSUPPORTED",
			Message: "remote hub start is not supported from this monitor",
			Hint:    "请在远端机器启动 hub，或切换到本地 hub 使用 Start。",
		}
	}

	if err := m.transport.MonitorAction(ctx, resolved, action); err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "hub offline") || strings.Contains(msg, "unavailable") {
			return &ActionError{
				Code:    "HUB_OFFLINE",
				Message: "target hub is offline",
				Hint:    "远端 hub 离线时无法执行该操作；Start 仅支持本地 hub 直启。",
			}
		}
		return err
	}
	return nil
}

func (m *Monitor) localProjectsByHub(hubID string) ([]RegistryProject, error) {
	cfgRaw, err := m.GetConfig()
	if err != nil || string(cfgRaw) == "null" {
		return nil, err
	}
	var cfg shared.AppConfig
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		return nil, err
	}
	items := make([]RegistryProject, 0, len(cfg.Projects))
	for _, p := range cfg.Projects {
		items = append(items, RegistryProject{
			ProjectID: rp.ProjectID(hubID, p.Name),
			Name:      p.Name,
			Path:      p.Path,
			Online:    true,
			Agent:     "unknown",
			IMType:    p.IMType(),
		})
	}
	return items, nil
}

func (m *Monitor) GetProjectsByHub(ctx context.Context, hubID string) ([]RegistryProject, error) {
	resolved, err := m.resolveHubID(ctx, hubID)
	if err != nil {
		return nil, err
	}
	if m.transport == nil || m.isLocalHub(resolved) {
		return m.localProjectsByHub(resolved)
	}
	return m.transport.ProjectList(ctx, resolved)
}

// ---------- process monitoring ----------

// ProcessInfo describes one running wheelmaker process.
type ProcessInfo struct {
	PID  int    `json:"pid"`
	Role string `json:"role"` // "guardian", "hub-worker", "registry-worker", "unknown"
}

// ServiceStatus is an aggregated view of all wheelmaker processes.
type ServiceStatus struct {
	Running   bool          `json:"running"`
	Processes []ProcessInfo `json:"processes"`
	Services  []ServiceInfo `json:"services,omitempty"`
	Timestamp string        `json:"timestamp"`
}

type ServiceInfo struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Status    string `json:"status"`
	StartType string `json:"startType"`
}

// GetServiceStatus returns the current wheelmaker process status.
func (m *Monitor) GetServiceStatus() (*ServiceStatus, error) {
	svcInfos, err := listManagedServices()
	if err != nil {
		return nil, err
	}

	procs, err := listWheelmakerProcesses()
	if err != nil {
		return nil, err
	}
	running := len(procs) > 0
	if runtime.GOOS == "windows" && len(svcInfos) > 0 {
		for _, svc := range svcInfos {
			if strings.EqualFold(svc.Name, wheelmakerServiceName) && strings.EqualFold(svc.Status, "Running") {
				running = true
				break
			}
		}
	}
	return &ServiceStatus{
		Running:   running,
		Processes: procs,
		Services:  svcInfos,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func listManagedServices() ([]ServiceInfo, error) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}
	names := []string{wheelmakerServiceName, monitorServiceName, updaterServiceName}
	out := make([]ServiceInfo, 0, len(names))
	for _, name := range names {
		info, err := getWindowsServiceInfo(name)
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	return out, nil
}

func getWindowsServiceInfo(name string) (ServiceInfo, error) {
	script := fmt.Sprintf(`$svc = Get-CimInstance Win32_Service -Filter "Name='%s'" -ErrorAction SilentlyContinue
if ($null -eq $svc) {
  '{"name":"%s","installed":false,"status":"NotInstalled","startType":"-"}'
  exit 0
}
$obj = @{
  name = $svc.Name
  installed = $true
  status = [string]$svc.State
  startType = [string]$svc.StartMode
}
$obj | ConvertTo-Json -Compress`, name, name)
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	out, err := cmd.Output()
	if err != nil {
		return ServiceInfo{}, fmt.Errorf("query service %s: %w", name, err)
	}
	var info ServiceInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return ServiceInfo{}, fmt.Errorf("parse service info %s: %w", name, err)
	}
	return info, nil
}

func listWheelmakerProcesses() ([]ProcessInfo, error) {
	if runtime.GOOS != "windows" {
		return listProcessesUnix()
	}
	return listProcessesWindows()
}

func listProcessesWindows() ([]ProcessInfo, error) {
	script := `Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" | Select-Object ProcessId,CommandLine | ConvertTo-Json -Compress`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	if raw == "" || raw == "null" || raw == "[]" {
		return nil, nil
	}

	type psEntry struct {
		ProcessId   int    `json:"ProcessId"`
		CommandLine string `json:"CommandLine"`
	}

	var entries []psEntry
	if raw[0] == '{' {
		var single psEntry
		if err := json.Unmarshal([]byte(raw), &single); err != nil {
			return nil, err
		}
		entries = []psEntry{single}
	} else {
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return nil, err
		}
	}

	var procs []ProcessInfo
	for _, e := range entries {
		role := classifyRole(e.CommandLine)
		procs = append(procs, ProcessInfo{PID: e.ProcessId, Role: role})
	}
	return procs, nil
}

func listProcessesUnix() ([]ProcessInfo, error) {
	out, err := exec.Command("ps", "aux").Output()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	var procs []ProcessInfo
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "wheelmaker") || strings.Contains(line, "wheelmaker-monitor") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(fields[1], "%d", &pid); err != nil {
			continue
		}
		role := classifyRole(line)
		procs = append(procs, ProcessInfo{PID: pid, Role: role})
	}
	return procs, nil
}

func classifyRole(cmdline string) string {
	lower := strings.ToLower(cmdline)
	switch {
	case strings.Contains(lower, "--hub-worker"):
		return "hub-worker"
	case strings.Contains(lower, "--registry-worker"):
		return "registry-worker"
	case strings.Contains(lower, "--daemon-worker"):
		return "hub-worker"
	case strings.Contains(lower, " -d"):
		return "guardian"
	case strings.Contains(lower, "wheelmaker.exe"):
		// Windows service-mode guardian typically runs without "-d" flag.
		return "guardian"
	default:
		return "unknown"
	}
}

// ---------- config / state ----------

// GetConfig reads and returns the parsed config.json.
func (m *Monitor) GetConfig() (json.RawMessage, error) {
	return readJSONFile(filepath.Join(m.baseDir, "config.json"))
}

// ---------- database tables ----------

// DBTablesResult holds the query results for all database tables.
type DBTablesResult struct {
	Tables []DBTable `json:"tables"`
	Error  string    `json:"error,omitempty"`
}

// DBTable holds one table's column names and row data.
type DBTable struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// GetDBTables opens the client SQLite database read-only and returns all table contents.
func (m *Monitor) GetDBTables() *DBTablesResult {
	dbPath := filepath.Join(m.baseDir, "db", "client.sqlite3")
	if !fileExists(dbPath) {
		return &DBTablesResult{Error: "database not found"}
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return &DBTablesResult{Error: "open database: " + err.Error()}
	}
	defer db.Close()

	tableNames := []string{"projects", "route_bindings", "sessions", "session_records"}
	tables := make([]DBTable, 0, len(tableNames))

	for _, name := range tableNames {
		t, err := queryTable(db, name)
		if err != nil {
			tables = append(tables, DBTable{
				Name:    name,
				Columns: []string{"error"},
				Rows:    [][]any{{err.Error()}},
			})
			continue
		}
		tables = append(tables, *t)
	}

	return &DBTablesResult{Tables: tables}
}

type MonitorSessionSummary struct {
	SessionID     string `json:"sessionId"`
	ProjectName   string `json:"projectName"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	LastIndex     int64  `json:"lastIndex"`
	LastSubIndex  int64  `json:"lastSubIndex"`
	CreatedAt     string `json:"createdAt"`
	LastActiveAt  string `json:"lastActiveAt"`
	LastMessageAt string `json:"lastMessageAt,omitempty"`
	EffectiveAt   string `json:"updatedAt"`
}

type MonitorSessionMessage struct {
	ProjectName string `json:"projectName"`
	SessionID   string `json:"sessionId"`
	Index       int64  `json:"index"`
	SubIndex    int64  `json:"subIndex"`
	Method      string `json:"method"`
	Role        string `json:"role"`
	Kind        string `json:"kind"`
	Body        string `json:"body"`
	Status      string `json:"status"`
	RequestID   int64  `json:"requestId,omitempty"`
	Source      string `json:"source,omitempty"`
	Time        string `json:"time"`
}

func (m *Monitor) openClientDB() (*sql.DB, error) {
	dbPath := filepath.Join(m.baseDir, "db", "client.sqlite3")
	if !fileExists(dbPath) {
		return nil, fmt.Errorf("database not found")
	}
	return sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
}

func (m *Monitor) ListSessions(projectName string, limit int) ([]MonitorSessionSummary, error) {
	db, err := m.openClientDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `
		SELECT id, project_name, status, title, last_message_at, last_sync_index, last_sync_subindex, created_at, last_active_at
		FROM sessions`
	args := []any{}
	projectName = strings.TrimSpace(projectName)
	if projectName != "" {
		query += ` WHERE project_name = ?`
		args = append(args, projectName)
	}
	query += ` ORDER BY CASE WHEN last_message_at = '' THEN last_active_at ELSE last_message_at END DESC, last_active_at DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []MonitorSessionSummary{}
	for rows.Next() {
		var item MonitorSessionSummary
		var status int
		if err := rows.Scan(&item.SessionID, &item.ProjectName, &status, &item.Title, &item.LastMessageAt, &item.LastIndex, &item.LastSubIndex, &item.CreatedAt, &item.LastActiveAt); err != nil {
			return nil, err
		}
		item.Status = monitorSessionStatusLabel(status)
		item.Title = firstNonEmpty(strings.TrimSpace(item.Title), item.SessionID)
		item.EffectiveAt = strings.TrimSpace(item.LastMessageAt)
		if item.EffectiveAt == "" {
			item.EffectiveAt = strings.TrimSpace(item.LastActiveAt)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (m *Monitor) GetSessionMessages(sessionID, projectName string, afterIndex, afterSubIndex int64, limit int) ([]MonitorSessionMessage, error) {
	db, err := m.openClientDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `
		SELECT s.project_name, r.session_id, r.sync_index, r.sync_subindex, r.time, r.source, r.content_json, r.meta_json
		FROM session_records r
		JOIN sessions s ON s.id = r.session_id
		WHERE r.session_id = ?`
	args := []any{strings.TrimSpace(sessionID)}
	projectName = strings.TrimSpace(projectName)
	if projectName != "" {
		query += ` AND s.project_name = ?`
		args = append(args, projectName)
	}
	if afterIndex > 0 || afterSubIndex > 0 {
		query += ` AND (r.sync_index > ? OR (r.sync_index = ? AND r.sync_subindex > ?))`
		args = append(args, afterIndex, afterIndex, afterSubIndex)
	}
	query += ` ORDER BY r.sync_index ASC, r.sync_subindex ASC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []MonitorSessionMessage{}
	for rows.Next() {
		var item MonitorSessionMessage
		var contentJSON string
		var metaJSON string
		if err := rows.Scan(&item.ProjectName, &item.SessionID, &item.Index, &item.SubIndex, &item.Time, &item.Source, &contentJSON, &metaJSON); err != nil {
			return nil, err
		}
		item.Method, item.Role, item.Kind, item.Body, item.Status, item.RequestID = parseMonitorSessionRecord(contentJSON, metaJSON)
		out = append(out, item)
	}
	return out, rows.Err()
}

func parseMonitorSessionRecord(contentJSON, metaJSON string) (method, role, kind, body, status string, requestID int64) {
	contentJSON = strings.TrimSpace(contentJSON)
	if contentJSON != "" {
		var content struct {
			Method  string `json:"method"`
			Payload struct {
				Role         string `json:"role"`
				Kind         string `json:"kind"`
				UpdateMethod string `json:"updateMethod"`
				Text         string `json:"text"`
				Status       string `json:"status"`
				RequestID    int64  `json:"requestId"`
			} `json:"payload"`
		}
		if err := json.Unmarshal([]byte(contentJSON), &content); err == nil {
			method = strings.TrimSpace(content.Method)
			role = strings.TrimSpace(content.Payload.Role)
			if method == "session.update" && strings.TrimSpace(content.Payload.UpdateMethod) != "" {
				kind = strings.TrimSpace(content.Payload.UpdateMethod)
			} else {
				kind = strings.TrimSpace(content.Payload.Kind)
			}
			body = content.Payload.Text
			status = strings.TrimSpace(content.Payload.Status)
			requestID = content.Payload.RequestID
		}
	}

	metaJSON = strings.TrimSpace(metaJSON)
	if metaJSON != "" && metaJSON != "{}" {
		var meta struct {
			Role      string `json:"role"`
			Kind      string `json:"kind"`
			Text      string `json:"text"`
			Status    string `json:"status"`
			RequestID int64  `json:"requestId"`
		}
		if err := json.Unmarshal([]byte(metaJSON), &meta); err == nil {
			if role == "" {
				role = strings.TrimSpace(meta.Role)
			}
			if kind == "" {
				kind = strings.TrimSpace(meta.Kind)
			}
			if body == "" {
				body = meta.Text
			}
			if status == "" {
				status = strings.TrimSpace(meta.Status)
			}
			if requestID == 0 {
				requestID = meta.RequestID
			}
		}
	}
	return method, role, kind, body, status, requestID
}
func monitorSessionStatusLabel(status int) string {
	switch status {
	case 0:
		return "active"
	case 1:
		return "suspended"
	case 2:
		return "persisted"
	default:
		return "unknown"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func queryTable(db *sql.DB, name string) (*DBTable, error) {
	rows, err := db.Query("SELECT * FROM " + name) //nolint:gosec // table names are hard-coded
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var result [][]any
	for rows.Next() {
		values := make([]any, len(cols))
		scanPtrs := make([]any, len(cols))
		for i := range values {
			scanPtrs[i] = &values[i]
		}
		if err := rows.Scan(scanPtrs...); err != nil {
			return nil, err
		}
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				values[i] = string(b)
			}
		}
		result = append(result, values)
	}
	if result == nil {
		result = [][]any{}
	}

	return &DBTable{Name: name, Columns: cols, Rows: result}, nil
}

func readJSONFile(path string) (json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return json.RawMessage("null"), nil
		}
		return nil, err
	}
	// Validate JSON
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", path, err)
	}
	return raw, nil
}

// ---------- log reading ----------

// LogEntry represents one parsed log line.
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Raw     string `json:"raw"`
}

// LogResult is the response for log queries.
type LogResult struct {
	File    string     `json:"file"`
	Entries []LogEntry `json:"entries"`
	Total   int        `json:"total"`
}

// GetLogs reads the specified log file with optional level filter.
// file: "hub" | "debug" | "registry" | "registry-debug" | "updater".
// level: "" (all), "warn", "error".
// tail: number of lines from the end (0 = all, max 5000).
func (m *Monitor) GetLogs(file string, level string, tail int) (*LogResult, error) {
	logPath := m.resolveLogFilePath(file)
	file = normalizeLogFileKey(file)
	isDebugLog := isDebugLogKey(file)

	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &LogResult{File: file, Entries: []LogEntry{}}, nil
		}
		return nil, err
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if tail <= 0 || tail > 5000 {
		tail = 5000
	}
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}

	level = strings.ToUpper(strings.TrimSpace(level))
	minLevel := levelRank(level)
	var entries []LogEntry
	for _, line := range lines {
		entry := parseLine(line)
		if level != "" && levelRank(entry.Level) < minLevel {
			continue
		}
		if isDebugLog {
			normalizeDebugLogEntry(&entry)
		}
		entries = append(entries, entry)
	}

	return &LogResult{
		File:    file,
		Entries: entries,
		Total:   len(entries),
	}, nil
}

func normalizeLogFileKey(file string) string {
	switch strings.TrimSpace(strings.ToLower(file)) {
	case "debug":
		return "debug"
	case "registry":
		return "registry"
	case "registry-debug":
		return "registry-debug"
	case "updater":
		return "updater"
	default:
		return "hub"
	}
}

func logFileNameForKey(file string) string {
	switch normalizeLogFileKey(file) {
	case "debug":
		return "hub.debug.log"
	case "registry":
		return "registry.log"
	case "registry-debug":
		return "registry.debug.log"
	case "updater":
		return "updater.log"
	default:
		return "hub.log"
	}
}

func (m *Monitor) resolveLogFilePath(file string) string {
	name := logFileNameForKey(file)
	preferred := filepath.Join(m.baseDir, "log", name)
	if fileExists(preferred) {
		return preferred
	}
	legacy := filepath.Join(m.baseDir, name)
	if fileExists(legacy) {
		return legacy
	}
	return preferred
}

func isDebugLogKey(file string) bool {
	key := normalizeLogFileKey(file)
	return key == "debug" || key == "registry-debug"
}

func normalizeDebugLogEntry(entry *LogEntry) {
	entry.Time = ""
	entry.Level = ""
	entry.Message = normalizeDebugLogMessage(entry.Message)
}

func normalizeDebugLogMessage(message string) string {
	matches := debugSessionPrefixRe.FindStringSubmatchIndex(message)
	if matches == nil {
		return message
	}
	fullSessionID := message[matches[2]:matches[3]]
	eventName := strings.TrimSpace(message[matches[4]:matches[5]])
	shortenedPrefix := "{" + shortSessionID(fullSessionID)
	if eventName != "" {
		shortenedPrefix += " " + eventName
	}
	shortenedPrefix += "}"

	normalized := message[:matches[0]] + shortenedPrefix + message[matches[1]:]
	return removeDuplicateSessionID(normalized, fullSessionID)
}

func removeDuplicateSessionID(message string, sessionID string) string {
	fieldWithComma := "\"sessionId\":\"" + sessionID + "\","
	if strings.Contains(message, fieldWithComma) {
		return strings.Replace(message, fieldWithComma, "", 1)
	}

	fieldWithLeadingComma := ",\"sessionId\":\"" + sessionID + "\""
	if strings.Contains(message, fieldWithLeadingComma) {
		return strings.Replace(message, fieldWithLeadingComma, "", 1)
	}

	fieldOnlyObject := "{\"sessionId\":\"" + sessionID + "\"}"
	if strings.Contains(message, fieldOnlyObject) {
		return strings.Replace(message, fieldOnlyObject, "{}", 1)
	}

	return message
}

func shortSessionID(sessionID string) string {
	if len(sessionID) <= 12 {
		return sessionID
	}
	return sessionID[:8] + ".." + sessionID[len(sessionID)-4:]
}

// levelRank returns numeric severity: higher = more severe.
func levelRank(level string) int {
	switch strings.ToUpper(level) {
	case "ERROR":
		return 4
	case "WARN":
		return 3
	case "INFO":
		return 2
	case "DEBUG":
		return 1
	default:
		return 0
	}
}

func parseLine(line string) LogEntry {
	// Format: "2006/01/02 15:04:05 LEVEL message..."
	entry := LogEntry{Raw: line}
	if len(line) < 26 {
		entry.Message = line
		return entry
	}
	// Try to parse date (19 chars) + space + level (5 chars)
	if line[4] == '/' && line[7] == '/' && line[10] == ' ' {
		entry.Time = line[:19]
		rest := line[20:]
		// Level tag is 5 chars like "INFO ", "WARN ", "ERROR", "DEBUG"
		if len(rest) >= 5 {
			levelStr := strings.TrimSpace(rest[:5])
			switch levelStr {
			case "DEBUG", "INFO", "WARN", "ERROR":
				entry.Level = levelStr
				entry.Message = strings.TrimSpace(rest[5:])
				return entry
			}
		}
		entry.Message = rest
		return entry
	}
	entry.Message = line
	return entry
}

// ---------- actions ----------

// RestartService restarts the wheelmaker service using internal process control.
func (m *Monitor) RestartService() error {
	if runtime.GOOS == "windows" {
		err := restartManagedRuntimeServices()
		if err == nil {
			return nil
		}
		if !errors.Is(err, errServiceNotInstalled) {
			return err
		}
	}
	if err := m.StopService(); err != nil {
		return err
	}
	time.Sleep(500 * time.Millisecond)
	return m.StartService()
}

// StopService stops the wheelmaker service using internal process control.
func (m *Monitor) StopService() error {
	if runtime.GOOS != "windows" {
		procs, err := listProcessesUnix()
		if err != nil {
			return err
		}
		for _, proc := range procs {
			_ = exec.Command("kill", "-TERM", fmt.Sprintf("%d", proc.PID)).Run()
		}
		time.Sleep(800 * time.Millisecond)
		remain, _ := listProcessesUnix()
		for _, proc := range remain {
			_ = exec.Command("kill", "-KILL", fmt.Sprintf("%d", proc.PID)).Run()
		}
		return nil
	}
	err := stopManagedRuntimeServices()
	if err == nil {
		return nil
	}
	if !errors.Is(err, errServiceNotInstalled) {
		return err
	}

	// Keep monitor alive: only stop wheelmaker.exe processes.
	script := `$ErrorActionPreference = 'SilentlyContinue'
$procs = @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'")
if ($procs.Count -eq 0) { exit 0 }
foreach ($p in $procs) { Stop-Process -Id $p.ProcessId -ErrorAction SilentlyContinue }
Start-Sleep -Milliseconds 800
$left = @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'")
foreach ($p in $left) { Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue }
$deadline = (Get-Date).AddSeconds(6)
while ((Get-Date) -lt $deadline) {
  $remain = @(Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'")
  if ($remain.Count -eq 0) { exit 0 }
  Start-Sleep -Milliseconds 300
}
Write-Error "wheelmaker.exe still running after stop timeout"
exit 1`
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("stop wheelmaker failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// StartService starts the wheelmaker service using internal process control.
func (m *Monitor) StartService() error {
	if runtime.GOOS == "windows" {
		err := startManagedRuntimeServices()
		if err == nil {
			return nil
		}
		if !errors.Is(err, errServiceNotInstalled) {
			return err
		}
	}
	wheelmakerExe, err := m.resolveWheelmakerExecutable()
	if err != nil {
		return err
	}
	cmd := exec.Command(wheelmakerExe, "-d")
	cmd.Dir = filepath.Dir(wheelmakerExe)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start wheelmaker failed: %w", err)
	}
	return nil
}

// TriggerUpdatePublish requests updater to run a full update/build/publish round.
// It only writes a signal file and does not start/stop updater process or service.
func (m *Monitor) TriggerUpdatePublish() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	signalPath := filepath.Join(m.baseDir, manualSignalFileName)
	if err := os.MkdirAll(filepath.Dir(signalPath), 0o755); err != nil {
		return fmt.Errorf("create signal directory: %w", err)
	}
	payload := fullUpdateSignalToken + "\n" + time.Now().UTC().Format(time.RFC3339)
	if err := os.WriteFile(signalPath, []byte(payload), 0o644); err != nil {
		return fmt.Errorf("write update signal: %w", err)
	}
	return nil
}

// RestartMonitor restarts the monitor process itself.
func (m *Monitor) RestartMonitor() error {
	if runtime.GOOS == "windows" {
		exists, err := windowsServiceExists(monitorServiceName)
		if err != nil {
			return err
		}
		if exists {
			// Run restart in a detached process so this HTTP handler can return before service stop.
			script := fmt.Sprintf("Start-Sleep -Milliseconds 300; Restart-Service -Name '%s' -Force -ErrorAction Stop", monitorServiceName)
			cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("schedule monitor service restart: %w", err)
			}
			go func() {
				time.Sleep(120 * time.Millisecond)
				os.Exit(0)
			}()
			return nil
		}
	}

	monitorExe, err := m.resolveMonitorExecutable()
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		script := fmt.Sprintf(
			"Start-Sleep -Milliseconds 900; Start-Process -FilePath %q -ArgumentList @('-dir', %q) -WindowStyle Hidden",
			monitorExe,
			m.baseDir,
		)
		cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("schedule monitor relaunch: %w", err)
		}
	} else {
		cmd := exec.Command(monitorExe, "-dir", m.baseDir)
		cmd.Dir = filepath.Dir(monitorExe)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start new monitor process: %w", err)
		}
	}

	go func() {
		time.Sleep(120 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}

func (m *Monitor) resolveWheelmakerExecutable() (string, error) {
	name := "wheelmaker"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := []string{
		filepath.Join(m.baseDir, "bin", name),
	}
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), name))
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("wheelmaker executable not found in candidates: %s", strings.Join(candidates, ", "))
}

func (m *Monitor) resolveMonitorExecutable() (string, error) {
	name := "wheelmaker-monitor"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := make([]string, 0, 3)
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates, exePath)
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), name))
	}
	candidates = append(candidates, filepath.Join(m.baseDir, "bin", name))
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("monitor executable not found in candidates: %s", strings.Join(candidates, ", "))
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func windowsServiceExists(serviceName string) (bool, error) {
	script := fmt.Sprintf("$svc = Get-Service -Name '%s' -ErrorAction SilentlyContinue; if ($null -eq $svc) { exit 3 }; exit 0", serviceName)
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if ee.ExitCode() == 3 {
				return false, nil
			}
		}
		return false, fmt.Errorf("check service %s: %w", serviceName, err)
	}
	return true, nil
}

func windowsServiceStart(serviceName string, timeout time.Duration) error {
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$svc = Get-Service -Name '%s' -ErrorAction SilentlyContinue
if ($null -eq $svc) { exit 3 }
if ($svc.Status -ne 'Running') { Start-Service -Name '%s' -ErrorAction Stop }
$deadline = (Get-Date).AddMilliseconds(%d)
while ((Get-Date) -lt $deadline) {
  $s = (Get-Service -Name '%s').Status
  if ($s -eq 'Running') { exit 0 }
  Start-Sleep -Milliseconds 200
}
Write-Error 'service start timeout'
exit 1`, serviceName, serviceName, timeout.Milliseconds(), serviceName)
	return runWindowsServiceScript(serviceName, script, "start")
}

func windowsServiceStop(serviceName string, timeout time.Duration) error {
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$svc = Get-Service -Name '%s' -ErrorAction SilentlyContinue
if ($null -eq $svc) { exit 3 }
if ($svc.Status -ne 'Stopped') { Stop-Service -Name '%s' -Force -ErrorAction Stop }
$deadline = (Get-Date).AddMilliseconds(%d)
while ((Get-Date) -lt $deadline) {
  $s = (Get-Service -Name '%s').Status
  if ($s -eq 'Stopped') { exit 0 }
  Start-Sleep -Milliseconds 200
}
Write-Error 'service stop timeout'
exit 1`, serviceName, serviceName, timeout.Milliseconds(), serviceName)
	return runWindowsServiceScript(serviceName, script, "stop")
}

func windowsServiceRestart(serviceName string, timeout time.Duration) error {
	if err := windowsServiceStop(serviceName, timeout); err != nil {
		if !errors.Is(err, errServiceNotInstalled) {
			return err
		}
	}
	return windowsServiceStart(serviceName, timeout)
}

func runWindowsServiceScript(serviceName string, script string, action string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 3 {
			return errServiceNotInstalled
		}
		return fmt.Errorf("%s service %s failed: %v (%s)", action, serviceName, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func managedRuntimeServiceNames() []string {
	return []string{wheelmakerServiceName, updaterServiceName}
}

func restartManagedRuntimeServices() error {
	return runManagedRuntimeServiceAction("restart")
}

func stopManagedRuntimeServices() error {
	return runManagedRuntimeServiceAction("stop")
}

func startManagedRuntimeServices() error {
	return runManagedRuntimeServiceAction("start")
}

func runManagedRuntimeServiceAction(action string) error {
	names := managedRuntimeServiceNames()
	performed := false
	errs := make([]string, 0)
	for _, name := range names {
		var err error
		switch action {
		case "start":
			err = windowsServiceStart(name, 10*time.Second)
		case "stop":
			err = windowsServiceStop(name, 10*time.Second)
		case "restart":
			err = windowsServiceRestart(name, 12*time.Second)
		default:
			return fmt.Errorf("unsupported service action: %s", action)
		}
		if err == nil {
			performed = true
			continue
		}
		if errors.Is(err, errServiceNotInstalled) {
			continue
		}
		errs = append(errs, fmt.Sprintf("%s: %v", name, err))
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	if !performed {
		return errServiceNotInstalled
	}
	return nil
}

// Overview returns a combined snapshot of status, config, and database tables.
type Overview struct {
	Service *ServiceStatus  `json:"service"`
	Config  json.RawMessage `json:"config"`
	DB      *DBTablesResult `json:"db"`
}

func (m *Monitor) GetOverview() (*Overview, error) {
	svc, err := m.GetServiceStatus()
	if err != nil {
		return nil, err
	}
	cfg, _ := m.GetConfig()
	db := m.GetDBTables()
	return &Overview{
		Service: svc,
		Config:  cfg,
		DB:      db,
	}, nil
}

// ---------- registry status ----------

// RegistryProject is one project reported by the registry.
type RegistryProject struct {
	ProjectID string             `json:"projectId"`
	Name      string             `json:"name"`
	Path      string             `json:"path"`
	Online    bool               `json:"online"`
	Agent     string             `json:"agent"`
	IMType    string             `json:"imType"`
	Git       rp.ProjectGitState `json:"git"`
}

// RegistryStatus is the live state retrieved from the registry server.
type RegistryStatus struct {
	Connected bool              `json:"connected"`
	Error     string            `json:"error,omitempty"`
	Projects  []RegistryProject `json:"projects"`
	Timestamp string            `json:"timestamp"`
}

// GetRegistryStatus connects to the registry via WebSocket and queries project.list.
func (m *Monitor) GetRegistryStatus() *RegistryStatus {
	ts := time.Now().UTC().Format(time.RFC3339)

	cfgRaw, err := m.GetConfig()
	if err != nil || string(cfgRaw) == "null" {
		return &RegistryStatus{Timestamp: ts, Error: "config not available"}
	}
	var cfg shared.AppConfig
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		return &RegistryStatus{Timestamp: ts, Error: "invalid config"}
	}

	// Monitor is intentionally local-only: always query local registry.
	server := "127.0.0.1"
	port := cfg.Registry.Port
	if port == 0 {
		port = 9630
	}

	u := url.URL{Scheme: "ws", Host: fmt.Sprintf("%s:%d", server, port), Path: "/ws"}
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return &RegistryStatus{Timestamp: ts, Error: "registry unreachable: " + err.Error()}
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// connect.init
	initReq := map[string]any{
		"requestId": 1,
		"type":      "request",
		"method":    "connect.init",
		"payload": rp.ConnectInitPayload{
			ClientName:      "wheelmaker-monitor",
			ClientVersion:   "0.1.0",
			ProtocolVersion: rp.DefaultProtocolVersion,
			Role:            "client",
			Token:           cfg.Registry.Token,
		},
	}
	if err := conn.WriteJSON(initReq); err != nil {
		return &RegistryStatus{Timestamp: ts, Error: "ws write: " + err.Error()}
	}
	var initResp rp.Envelope
	if err := conn.ReadJSON(&initResp); err != nil {
		return &RegistryStatus{Timestamp: ts, Error: "ws read: " + err.Error()}
	}
	if initResp.Type == "error" {
		var ep rp.ErrorPayload
		json.Unmarshal(initResp.Payload, &ep)
		return &RegistryStatus{Timestamp: ts, Error: "connect: " + ep.Message}
	}

	// project.list
	listReq := map[string]any{
		"requestId": 2,
		"type":      "request",
		"method":    "project.list",
	}
	if err := conn.WriteJSON(listReq); err != nil {
		return &RegistryStatus{Timestamp: ts, Error: "ws write: " + err.Error()}
	}
	var listResp rp.Envelope
	if err := conn.ReadJSON(&listResp); err != nil {
		return &RegistryStatus{Timestamp: ts, Error: "ws read: " + err.Error()}
	}
	if listResp.Type == "error" {
		var ep rp.ErrorPayload
		json.Unmarshal(listResp.Payload, &ep)
		return &RegistryStatus{Timestamp: ts, Error: "project.list: " + ep.Message}
	}

	var payload struct {
		Projects []RegistryProject `json:"projects"`
	}
	if err := json.Unmarshal(listResp.Payload, &payload); err != nil {
		return &RegistryStatus{Timestamp: ts, Error: "parse projects: " + err.Error()}
	}

	return &RegistryStatus{
		Connected: true,
		Projects:  payload.Projects,
		Timestamp: ts,
	}
}
