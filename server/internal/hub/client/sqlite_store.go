package client

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
	_ "modernc.org/sqlite"
)

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS projects (
	project_name TEXT PRIMARY KEY,
	yolo INTEGER NOT NULL DEFAULT 0,
	agent_state_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS route_bindings (
	project_name TEXT NOT NULL,
	route_key TEXT NOT NULL,
	session_id TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (project_name, route_key)
);
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	project_name TEXT NOT NULL,
	status INTEGER NOT NULL,
	last_reply TEXT NOT NULL DEFAULT '',
	acp_session_id TEXT NOT NULL DEFAULT '',
	agents_json TEXT NOT NULL DEFAULT '{}',
	created_at TEXT NOT NULL,
	last_active_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_route_bindings_project ON route_bindings(project_name);
CREATE INDEX IF NOT EXISTS idx_sessions_project_last_active ON sessions(project_name, last_active_at DESC);
`

type ProjectAgentState struct {
	ConfigOptions     []acp.ConfigOption     `json:"configOptions,omitempty"`
	AvailableCommands []acp.AvailableCommand `json:"availableCommands,omitempty"`
	UpdatedAt         string                 `json:"updatedAt,omitempty"`
}

type ProjectConfig struct {
	YOLO       bool
	AgentState map[string]ProjectAgentState
}

type SessionRecord struct {
	ID           string
	ProjectName  string
	Status       SessionStatus
	LastReply    string
	ACPSessionID string
	AgentsJSON   string
	CreatedAt    time.Time
	LastActiveAt time.Time
}

type SessionListEntry struct {
	ID           string
	Agent        string
	Title        string
	Status       SessionStatus
	CreatedAt    time.Time
	LastActiveAt time.Time
}

type Store interface {
	LoadProject(ctx context.Context, projectName string) (*ProjectConfig, error)
	SaveProject(ctx context.Context, projectName string, cfg ProjectConfig) error

	LoadRouteBindings(ctx context.Context, projectName string) (map[string]string, error)
	SaveRouteBinding(ctx context.Context, projectName, routeKey, sessionID string) error
	DeleteRouteBinding(ctx context.Context, projectName, routeKey string) error

	LoadSession(ctx context.Context, projectName, sessionID string) (*SessionRecord, error)
	SaveSession(ctx context.Context, rec *SessionRecord) error
	ListSessions(ctx context.Context, projectName string) ([]SessionListEntry, error)
	DeleteSession(ctx context.Context, projectName, sessionID string) error

	Close() error
}

type sqliteStore struct {
	db *sql.DB
}

func NewStore(dbPath string) (Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dbPath, err)
	}
	if _, err := db.Exec(sqliteSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &sqliteStore{db: db}, nil
}

func (s *sqliteStore) LoadProject(ctx context.Context, projectName string) (*ProjectConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT yolo, agent_state_json FROM projects WHERE project_name = ?`, strings.TrimSpace(projectName))

	var yolo int
	var agentStateJSON string
	if err := row.Scan(&yolo, &agentStateJSON); err != nil {
		if err == sql.ErrNoRows {
			return &ProjectConfig{AgentState: map[string]ProjectAgentState{}}, nil
		}
		return nil, fmt.Errorf("load project: %w", err)
	}

	cfg := &ProjectConfig{YOLO: yolo != 0, AgentState: map[string]ProjectAgentState{}}
	if strings.TrimSpace(agentStateJSON) != "" {
		if err := json.Unmarshal([]byte(agentStateJSON), &cfg.AgentState); err != nil {
			return nil, fmt.Errorf("unmarshal agent_state_json: %w", err)
		}
	}
	return cfg, nil
}

func (s *sqliteStore) SaveProject(ctx context.Context, projectName string, cfg ProjectConfig) error {
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return fmt.Errorf("project name is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	yolo := 0
	if cfg.YOLO {
		yolo = 1
	}
	if cfg.AgentState == nil {
		cfg.AgentState = map[string]ProjectAgentState{}
	}
	raw, err := json.Marshal(cfg.AgentState)
	if err != nil {
		return fmt.Errorf("marshal agent_state_json: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO projects (project_name, yolo, agent_state_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project_name) DO UPDATE SET
			yolo=excluded.yolo,
			agent_state_json=excluded.agent_state_json,
			updated_at=excluded.updated_at
	`, projectName, yolo, string(raw), now, now)
	if err != nil {
		return fmt.Errorf("save project: %w", err)
	}
	return nil
}

func (s *sqliteStore) LoadRouteBindings(ctx context.Context, projectName string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT route_key, session_id
		FROM route_bindings
		WHERE project_name = ?
	`, strings.TrimSpace(projectName))
	if err != nil {
		return nil, fmt.Errorf("load route bindings: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var routeKey string
		var sessionID string
		if err := rows.Scan(&routeKey, &sessionID); err != nil {
			return nil, fmt.Errorf("scan route binding: %w", err)
		}
		out[routeKey] = sessionID
	}
	return out, rows.Err()
}

func (s *sqliteStore) SaveRouteBinding(ctx context.Context, projectName, routeKey, sessionID string) error {
	projectName = strings.TrimSpace(projectName)
	sessionID = strings.TrimSpace(sessionID)
	if projectName == "" {
		return fmt.Errorf("project name is required")
	}
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if err := validateRouteKey(routeKey); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO route_bindings (project_name, route_key, session_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project_name, route_key) DO UPDATE SET
			session_id=excluded.session_id,
			updated_at=excluded.updated_at
	`, projectName, strings.TrimSpace(routeKey), sessionID, now, now)
	if err != nil {
		return fmt.Errorf("save route binding: %w", err)
	}
	return nil
}

func (s *sqliteStore) DeleteRouteBinding(ctx context.Context, projectName, routeKey string) error {
	if err := validateRouteKey(routeKey); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM route_bindings
		WHERE project_name = ? AND route_key = ?
	`, strings.TrimSpace(projectName), strings.TrimSpace(routeKey))
	if err != nil {
		return fmt.Errorf("delete route binding: %w", err)
	}
	return nil
}

func (s *sqliteStore) LoadSession(ctx context.Context, projectName, sessionID string) (*SessionRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, status, last_reply, acp_session_id, agents_json, created_at, last_active_at
		FROM sessions
		WHERE project_name = ? AND id = ?
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID))

	rec := &SessionRecord{}
	var status int
	var createdAt string
	var lastActiveAt string
	if err := row.Scan(
		&rec.ID,
		&status,
		&rec.LastReply,
		&rec.ACPSessionID,
		&rec.AgentsJSON,
		&createdAt,
		&lastActiveAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("load session: %w", err)
	}

	rec.ProjectName = strings.TrimSpace(projectName)
	rec.Status = SessionStatus(status)
	rec.CreatedAt = parseStoreTime(createdAt)
	rec.LastActiveAt = parseStoreTime(lastActiveAt)
	return rec, nil
}

func (s *sqliteStore) SaveSession(ctx context.Context, rec *SessionRecord) error {
	if rec == nil {
		return fmt.Errorf("session record is required")
	}
	rec.ID = strings.TrimSpace(rec.ID)
	rec.ProjectName = strings.TrimSpace(rec.ProjectName)
	if rec.ID == "" {
		return fmt.Errorf("session id is required")
	}
	if rec.ProjectName == "" {
		return fmt.Errorf("project name is required")
	}
	if strings.TrimSpace(rec.AgentsJSON) == "" {
		rec.AgentsJSON = "{}"
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	if rec.LastActiveAt.IsZero() {
		rec.LastActiveAt = rec.CreatedAt
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (
			id, project_name, status, last_reply, acp_session_id, agents_json, created_at, last_active_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_name=excluded.project_name,
			status=excluded.status,
			last_reply=excluded.last_reply,
			acp_session_id=excluded.acp_session_id,
			agents_json=excluded.agents_json,
			last_active_at=excluded.last_active_at
	`, rec.ID, rec.ProjectName, int(rec.Status), rec.LastReply, rec.ACPSessionID, rec.AgentsJSON,
		rec.CreatedAt.UTC().Format(time.RFC3339Nano), rec.LastActiveAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (s *sqliteStore) ListSessions(ctx context.Context, projectName string) ([]SessionListEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, status, acp_session_id, agents_json, created_at, last_active_at
		FROM sessions
		WHERE project_name = ?
		ORDER BY last_active_at DESC
	`, strings.TrimSpace(projectName))
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	entries := []SessionListEntry{}
	for rows.Next() {
		var entry SessionListEntry
		var status int
		var acpSessionID string
		var agentsJSON string
		var createdAt string
		var lastActiveAt string
		if err := rows.Scan(&entry.ID, &status, &acpSessionID, &agentsJSON, &createdAt, &lastActiveAt); err != nil {
			return nil, fmt.Errorf("scan session list entry: %w", err)
		}
		entry.Status = SessionStatus(status)
		entry.CreatedAt = parseStoreTime(createdAt)
		entry.LastActiveAt = parseStoreTime(lastActiveAt)
		entry.Agent, entry.Title = inferSessionListMetadata(acpSessionID, agentsJSON)
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *sqliteStore) DeleteSession(ctx context.Context, projectName, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM sessions
		WHERE project_name = ? AND id = ?
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID))
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}

func validateRouteKey(routeKey string) error {
	if strings.TrimSpace(routeKey) == "" {
		return fmt.Errorf("route key is required")
	}
	return nil
}

func parseStoreTime(raw string) time.Time {
	if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return ts
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts
	}
	return time.Time{}
}

func inferSessionListMetadata(acpSessionID, agentsJSON string) (string, string) {
	type storedAgent struct {
		ACPSessionID string `json:"acpSessionId,omitempty"`
		Title        string `json:"title,omitempty"`
	}

	agents := map[string]storedAgent{}
	if err := json.Unmarshal([]byte(agentsJSON), &agents); err != nil {
		return "", ""
	}
	if strings.TrimSpace(acpSessionID) != "" {
		for name, state := range agents {
			if strings.TrimSpace(state.ACPSessionID) == strings.TrimSpace(acpSessionID) {
				return name, state.Title
			}
		}
	}
	if len(agents) == 1 {
		for name, state := range agents {
			return name, state.Title
		}
	}
	return "", ""
}
