package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/swm8023/wheelmaker/internal/shared"
)

// Monitor holds the base directory and provides all monitoring operations.
type Monitor struct {
	baseDir string
	mu      sync.RWMutex
}

// NewMonitor creates a Monitor for the given WheelMaker home directory.
func NewMonitor(baseDir string) *Monitor {
	return &Monitor{baseDir: baseDir}
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
	Timestamp string        `json:"timestamp"`
}

// GetServiceStatus returns the current wheelmaker process status.
func (m *Monitor) GetServiceStatus() (*ServiceStatus, error) {
	procs, err := listWheelmakerProcesses()
	if err != nil {
		return nil, err
	}
	return &ServiceStatus{
		Running:   len(procs) > 0,
		Processes: procs,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}, nil
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
	switch {
	case strings.Contains(cmdline, "--hub-worker"):
		return "hub-worker"
	case strings.Contains(cmdline, "--registry-worker"):
		return "registry-worker"
	case strings.Contains(cmdline, " -d"):
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

// GetState reads and returns the parsed state.json.
func (m *Monitor) GetState() (json.RawMessage, error) {
	return readJSONFile(filepath.Join(m.baseDir, "state.json"))
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
// file: "hub" or "debug". level: "" (all), "warn", "error".
// tail: number of lines from the end (0 = all, max 5000).
func (m *Monitor) GetLogs(file string, level string, tail int) (*LogResult, error) {
	var logPath string
	switch file {
	case "debug":
		logPath = filepath.Join(m.baseDir, "hub.debug.log")
	default:
		logPath = filepath.Join(m.baseDir, "hub.log")
		file = "hub"
	}

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
		entries = append(entries, entry)
	}

	return &LogResult{
		File:    file,
		Entries: entries,
		Total:   len(entries),
	}, nil
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

// RestartMonitor restarts the monitor process itself.
func (m *Monitor) RestartMonitor() error {
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

// Overview returns a combined snapshot of status, config, and state.
type Overview struct {
	Service *ServiceStatus  `json:"service"`
	Config  json.RawMessage `json:"config"`
	State   json.RawMessage `json:"state"`
}

func (m *Monitor) GetOverview() (*Overview, error) {
	svc, err := m.GetServiceStatus()
	if err != nil {
		return nil, err
	}
	cfg, _ := m.GetConfig()
	state, _ := m.GetState()
	return &Overview{
		Service: svc,
		Config:  cfg,
		State:   state,
	}, nil
}

// ---------- registry status ----------

// RegistryProject is one project reported by the registry.
type RegistryProject struct {
	ProjectID string                 `json:"projectId"`
	Name      string                 `json:"name"`
	Path      string                 `json:"path"`
	Online    bool                   `json:"online"`
	Agent     string                 `json:"agent"`
	IMType    string                 `json:"imType"`
	Git       shared.ProjectGitState `json:"git"`
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
		"payload": shared.ConnectInitPayload{
			ClientName:      "wheelmaker-monitor",
			ClientVersion:   "0.1.0",
			ProtocolVersion: shared.DefaultProtocolVersion,
			Role:            "client",
			Token:           cfg.Registry.Token,
		},
	}
	if err := conn.WriteJSON(initReq); err != nil {
		return &RegistryStatus{Timestamp: ts, Error: "ws write: " + err.Error()}
	}
	var initResp shared.Envelope
	if err := conn.ReadJSON(&initResp); err != nil {
		return &RegistryStatus{Timestamp: ts, Error: "ws read: " + err.Error()}
	}
	if initResp.Type == "error" {
		var ep shared.ErrorPayload
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
	var listResp shared.Envelope
	if err := conn.ReadJSON(&listResp); err != nil {
		return &RegistryStatus{Timestamp: ts, Error: "ws read: " + err.Error()}
	}
	if listResp.Type == "error" {
		var ep shared.ErrorPayload
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
