package monitorcore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Core struct {
	BaseDir string
}

func New(baseDir string) *Core {
	return &Core{BaseDir: strings.TrimSpace(baseDir)}
}

type ProcessInfo struct {
	PID       int    `json:"pid"`
	Role      string `json:"role"`
	StartedAt string `json:"startedAt,omitempty"`
}

type ServiceStatus struct {
	Running   bool          `json:"running"`
	Processes []ProcessInfo `json:"processes"`
	Timestamp string        `json:"timestamp"`
}

func (c *Core) GetServiceStatus() (*ServiceStatus, error) {
	procs, err := listWheelmakerProcesses()
	if err != nil {
		return nil, err
	}
	return &ServiceStatus{Running: len(procs) > 0, Processes: procs, Timestamp: time.Now().UTC().Format(time.RFC3339)}, nil
}

func listWheelmakerProcesses() ([]ProcessInfo, error) {
	if runtime.GOOS == "windows" {
		script := `Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" | Select-Object ProcessId,CommandLine,@{Name='StartedAt';Expression={[Management.ManagementDateTimeConverter]::ToDateTime($_.CreationDate).ToString('MM-dd HH:mm')}} | ConvertTo-Json -Compress`
		out, err := exec.Command("powershell", "-NoProfile", "-Command", script).Output()
		if err != nil {
			return nil, err
		}
		raw := strings.TrimSpace(string(out))
		if raw == "" || raw == "null" || raw == "[]" {
			return nil, nil
		}
		type entry struct {
			ProcessID   int    `json:"ProcessId"`
			CommandLine string `json:"CommandLine"`
			StartedAt   string `json:"StartedAt"`
		}
		entries := []entry{}
		if strings.HasPrefix(raw, "{") {
			var one entry
			if err := json.Unmarshal([]byte(raw), &one); err != nil {
				return nil, err
			}
			entries = append(entries, one)
		} else if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return nil, err
		}
		outProcs := make([]ProcessInfo, 0, len(entries))
		for _, e := range entries {
			outProcs = append(outProcs, ProcessInfo{
				PID:       e.ProcessID,
				Role:      classifyRole(e.CommandLine),
				StartedAt: strings.TrimSpace(e.StartedAt),
			})
		}
		return outProcs, nil
	}
	out, err := exec.Command("ps", "-eo", "pid=,lstart=,args=").Output()
	if err != nil {
		return nil, err
	}
	procs := []ProcessInfo{}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "wheelmaker") || strings.Contains(line, "wheelmaker-monitor") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(fields[0], "%d", &pid); err != nil {
			continue
		}
		startedAt := formatUnixStartedAt(fields[1:6])
		cmdline := strings.Join(fields[6:], " ")
		procs = append(procs, ProcessInfo{PID: pid, Role: classifyRole(cmdline), StartedAt: startedAt})
	}
	return procs, nil
}

func formatUnixStartedAt(parts []string) string {
	if len(parts) != 5 {
		return ""
	}
	startedAt, err := time.Parse("Mon Jan 2 15:04:05 2006", strings.Join(parts, " "))
	if err != nil {
		return ""
	}
	return startedAt.Format("01-02 15:04")
}

func classifyRole(cmdline string) string {
	lower := strings.ToLower(cmdline)
	switch {
	case strings.Contains(lower, "--hub-worker"):
		return "hub-worker"
	case strings.Contains(lower, "--registry-worker"):
		return "registry-worker"
	case strings.Contains(lower, " -d"):
		return "guardian"
	default:
		return "unknown"
	}
}

type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Raw     string `json:"raw"`
}

type LogResult struct {
	File    string     `json:"file"`
	Entries []LogEntry `json:"entries"`
	Total   int        `json:"total"`
}

func (c *Core) GetLogs(file string, level string, tail int) (*LogResult, error) {
	logPath := c.resolveLogFilePath(file)
	key := normalizeLogFileKey(file)
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &LogResult{File: key, Entries: []LogEntry{}}, nil
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
	want := strings.ToUpper(strings.TrimSpace(level))
	min := levelRank(want)
	entries := make([]LogEntry, 0, len(lines))
	for _, line := range lines {
		e := parseLine(line)
		if want != "" && levelRank(e.Level) < min {
			continue
		}
		entries = append(entries, e)
	}
	return &LogResult{File: key, Entries: entries, Total: len(entries)}, nil
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

func (c *Core) resolveLogFilePath(file string) string {
	name := logFileNameForKey(file)
	preferred := filepath.Join(c.BaseDir, "log", name)
	if fileExists(preferred) {
		return preferred
	}
	legacy := filepath.Join(c.BaseDir, name)
	if fileExists(legacy) {
		return legacy
	}
	return preferred
}

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
	e := LogEntry{Raw: line, Message: line}
	if len(line) < 26 {
		return e
	}
	if line[4] == '/' && line[7] == '/' && line[10] == ' ' {
		e.Time = line[:19]
		rest := line[20:]
		if len(rest) >= 5 {
			lv := strings.TrimSpace(rest[:5])
			switch lv {
			case "DEBUG", "INFO", "WARN", "ERROR":
				e.Level = lv
				e.Message = strings.TrimSpace(rest[5:])
				return e
			}
		}
		e.Message = rest
	}
	return e
}

type DBTablesResult struct {
	Tables []DBTable `json:"tables"`
	Error  string    `json:"error,omitempty"`
}

type DBTable struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

func (c *Core) GetDBTables() *DBTablesResult {
	dbPath := filepath.Join(c.BaseDir, "db", "client.sqlite3")
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
		rows, err := db.Query("SELECT * FROM " + name)
		if err != nil {
			tables = append(tables, DBTable{Name: name, Columns: []string{"error"}, Rows: [][]any{{err.Error()}}})
			continue
		}
		cols, _ := rows.Columns()
		data := [][]any{}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			_ = rows.Scan(ptrs...)
			for i, v := range vals {
				if b, ok := v.([]byte); ok {
					vals[i] = string(b)
				}
			}
			data = append(data, vals)
		}
		_ = rows.Close()
		tables = append(tables, DBTable{Name: name, Columns: cols, Rows: data})
	}
	return &DBTablesResult{Tables: tables}
}

func (c *Core) ExecuteAction(action string) error {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "update-publish":
		signalPath := filepath.Join(c.BaseDir, "update-now.signal")
		payload := "full-update\n" + time.Now().UTC().Format(time.RFC3339)
		if err := os.WriteFile(signalPath, []byte(payload), 0o644); err != nil {
			return err
		}
		return nil
	case "start", "stop", "restart", "restart-monitor":
		return nil
	default:
		return fmt.Errorf("unsupported action: %s", action)
	}
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
