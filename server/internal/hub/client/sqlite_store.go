package client

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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
	status INTEGER NOT NULL,	acp_session_id TEXT NOT NULL DEFAULT '',
	agents_json TEXT NOT NULL DEFAULT '{}',
	title TEXT NOT NULL DEFAULT '',	created_at TEXT NOT NULL,
	last_active_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS session_prompts (
	session_id TEXT NOT NULL,
	prompt_index INTEGER NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	stop_reason TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (session_id, prompt_index)
);
CREATE TABLE IF NOT EXISTS session_turns (
	session_id TEXT NOT NULL,
	prompt_index INTEGER NOT NULL,
	turn_index INTEGER NOT NULL,
	update_index INTEGER NOT NULL DEFAULT 0,
	update_json TEXT NOT NULL DEFAULT '{}',
	extra_json TEXT NOT NULL DEFAULT '{}',
	PRIMARY KEY (session_id, prompt_index, turn_index)
);
CREATE INDEX IF NOT EXISTS idx_route_bindings_project ON route_bindings(project_name);
CREATE INDEX IF NOT EXISTS idx_sessions_project_last_active ON sessions(project_name, last_active_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_prompts_session_prompt ON session_prompts(session_id, prompt_index);
CREATE INDEX IF NOT EXISTS idx_session_turns_session_prompt_turn ON session_turns(session_id, prompt_index, turn_index);
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
	ACPSessionID string
	AgentsJSON   string
	Title        string
	CreatedAt    time.Time
	LastActiveAt time.Time
}

type SessionListEntry struct {
	ID           string
	ProjectName  string
	Agent        string
	Title        string
	Status       SessionStatus
	CreatedAt    time.Time
	LastActiveAt time.Time
	InMemory     bool
}

type SessionPromptRecord struct {
	PromptID    string
	SessionID   string
	PromptIndex int64
	Title       string
	StopReason  string
	UpdatedAt   time.Time
}

type SessionTurnRecord struct {
	TurnID      string
	SessionID   string
	PromptIndex int64
	TurnIndex   int64
	UpdateIndex int64
	UpdateJSON  string
	ExtraJSON   string
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
	UpsertSessionPrompt(ctx context.Context, rec SessionPromptRecord) error
	LoadSessionPrompt(ctx context.Context, projectName, sessionID string, promptIndex int64) (*SessionPromptRecord, error)
	ListSessionPrompts(ctx context.Context, projectName, sessionID string) ([]SessionPromptRecord, error)
	ListSessionPromptsAfterIndex(ctx context.Context, projectName, sessionID string, afterPromptIndex int64) ([]SessionPromptRecord, error)
	UpsertSessionTurn(ctx context.Context, rec SessionTurnRecord) error
	LoadSessionTurn(ctx context.Context, projectName, sessionID string, promptIndex, turnIndex int64) (*SessionTurnRecord, error)
	ListSessionTurns(ctx context.Context, projectName, sessionID string, promptIndex int64) ([]SessionTurnRecord, error)
	Close() error
}

type sqliteStore struct {
	db      *sql.DB
	writeMu sync.Mutex
}

func NewStore(dbPath string) (Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dbPath, err)
	}
	if _, err := db.Exec(sqliteSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &sqliteStore{db: db}, nil
}

var expectedStoreSchemaColumns = map[string][]string{
	"projects":        {"project_name", "yolo", "agent_state_json", "created_at", "updated_at"},
	"route_bindings":  {"project_name", "route_key", "session_id", "created_at", "updated_at"},
	"sessions":        {"id", "project_name", "status", "acp_session_id", "agents_json", "title", "created_at", "last_active_at"},
	"session_prompts": {"session_id", "prompt_index", "title", "stop_reason", "updated_at"},
	"session_turns":   {"session_id", "prompt_index", "turn_index", "update_index", "update_json", "extra_json"},
}

type StoreSchemaMismatchError struct {
	Path   string
	Issues []string
}

func (e *StoreSchemaMismatchError) Error() string {
	return fmt.Sprintf("store schema mismatch for %q: %s", e.Path, strings.Join(e.Issues, "; "))
}

func IsStoreSchemaMismatch(err error) bool {
	var mismatchErr *StoreSchemaMismatchError
	return errors.As(err, &mismatchErr)
}

func CheckStoreSchema(dbPath string) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("mkdir db dir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open sqlite %q: %w", dbPath, err)
	}
	defer db.Close()

	existingTables, err := sqliteUserTables(db)
	if err != nil {
		return fmt.Errorf("list sqlite tables: %w", err)
	}
	if len(existingTables) == 0 {
		return nil
	}

	issues := make([]string, 0)
	for tableName := range expectedStoreSchemaColumns {
		if _, ok := existingTables[tableName]; !ok {
			issues = append(issues, fmt.Sprintf("missing table %q", tableName))
		}
	}
	for tableName := range existingTables {
		if _, ok := expectedStoreSchemaColumns[tableName]; !ok {
			issues = append(issues, fmt.Sprintf("unexpected table %q", tableName))
		}
	}

	for tableName, expectedColumns := range expectedStoreSchemaColumns {
		if _, ok := existingTables[tableName]; !ok {
			continue
		}
		actualColumns, err := sqliteTableColumns(db, tableName)
		if err != nil {
			return fmt.Errorf("read columns for %s: %w", tableName, err)
		}
		if !sameColumnSet(actualColumns, expectedColumns) {
			issues = append(issues, fmt.Sprintf("table %q columns mismatch (expected=%s actual=%s)", tableName, joinSortedColumns(expectedColumns), joinSortedColumns(actualColumns)))
		}
	}

	if len(issues) > 0 {
		sort.Strings(issues)
		return &StoreSchemaMismatchError{Path: dbPath, Issues: issues}
	}
	return nil
}

func sqliteUserTables(db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables[normalizeStoreSchemaName(name)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tables, nil
}

func sqliteTableColumns(db *sql.DB, tableName string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make([]string, 0)
	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns = append(columns, normalizeStoreSchemaName(name))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func sameColumnSet(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	actualSet := make(map[string]struct{}, len(actual))
	for _, column := range actual {
		actualSet[normalizeStoreSchemaName(column)] = struct{}{}
	}
	for _, column := range expected {
		if _, ok := actualSet[normalizeStoreSchemaName(column)]; !ok {
			return false
		}
	}
	return true
}

func joinSortedColumns(columns []string) string {
	out := make([]string, 0, len(columns))
	for _, column := range columns {
		out = append(out, normalizeStoreSchemaName(column))
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func normalizeStoreSchemaName(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
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
		SELECT id, status, acp_session_id, agents_json, title, created_at, last_active_at
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
		&rec.ACPSessionID,
		&rec.AgentsJSON,
		&rec.Title,
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
			id, project_name, status, acp_session_id, agents_json, title, created_at, last_active_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_name=excluded.project_name,
			status=excluded.status,
			acp_session_id=excluded.acp_session_id,
			agents_json=excluded.agents_json,
			title=CASE WHEN excluded.title != '' THEN excluded.title ELSE sessions.title END,
			last_active_at=excluded.last_active_at
	`, rec.ID, rec.ProjectName, int(rec.Status), rec.ACPSessionID, rec.AgentsJSON, rec.Title,
		rec.CreatedAt.UTC().Format(time.RFC3339Nano), rec.LastActiveAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (s *sqliteStore) ListSessions(ctx context.Context, projectName string) ([]SessionListEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_name, status, acp_session_id, agents_json, title, created_at, last_active_at
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
		var entryProjectName string
		var status int
		var acpSessionID string
		var agentsJSON string
		var storedTitle string
		var createdAt string
		var lastActiveAt string
		if err := rows.Scan(&entry.ID, &entryProjectName, &status, &acpSessionID, &agentsJSON, &storedTitle, &createdAt, &lastActiveAt); err != nil {
			return nil, fmt.Errorf("scan session list entry: %w", err)
		}
		entry.ProjectName = strings.TrimSpace(entryProjectName)
		entry.Status = SessionStatus(status)
		entry.CreatedAt = parseStoreTime(createdAt)
		entry.LastActiveAt = parseStoreTime(lastActiveAt)
		entry.Agent, entry.Title = inferSessionListMetadata(acpSessionID, agentsJSON)
		if strings.TrimSpace(storedTitle) != "" {
			entry.Title = strings.TrimSpace(storedTitle)
		}
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

func (s *sqliteStore) UpsertSessionPrompt(ctx context.Context, rec SessionPromptRecord) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	rec.SessionID = strings.TrimSpace(rec.SessionID)
	if rec.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if rec.PromptIndex <= 0 {
		return fmt.Errorf("prompt index is required")
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = time.Now().UTC()
	}
	updatedAt := rec.UpdatedAt.UTC().Format(time.RFC3339Nano)

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO session_prompts (session_id, prompt_index, title, stop_reason, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(session_id, prompt_index) DO UPDATE SET
			title = CASE WHEN excluded.title != '' THEN excluded.title ELSE session_prompts.title END,
			stop_reason = CASE WHEN excluded.stop_reason != '' THEN excluded.stop_reason ELSE session_prompts.stop_reason END,
			updated_at = CASE WHEN excluded.updated_at > session_prompts.updated_at THEN excluded.updated_at ELSE session_prompts.updated_at END
	`, rec.SessionID, rec.PromptIndex, strings.TrimSpace(rec.Title), strings.TrimSpace(rec.StopReason), updatedAt); err != nil {
		return fmt.Errorf("upsert session prompt: %w", err)
	}
	return nil
}

func (s *sqliteStore) LoadSessionPrompt(ctx context.Context, projectName, sessionID string, promptIndex int64) (*SessionPromptRecord, error) {
	if promptIndex <= 0 {
		return nil, nil
	}
	var rec SessionPromptRecord
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT p.session_id, p.prompt_index, p.title, p.stop_reason, p.updated_at
		FROM session_prompts p
		JOIN sessions s ON s.id = p.session_id
		WHERE s.project_name = ? AND p.session_id = ? AND p.prompt_index = ?
		LIMIT 1
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID), promptIndex).Scan(&rec.SessionID, &rec.PromptIndex, &rec.Title, &rec.StopReason, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load session prompt: %w", err)
	}
	rec.UpdatedAt = parseStoreTime(updatedAt)
	rec.PromptID = formatPromptSeq(rec.PromptIndex)
	return &rec, nil
}

func (s *sqliteStore) ListSessionPrompts(ctx context.Context, projectName, sessionID string) ([]SessionPromptRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.session_id, p.prompt_index, p.title, p.stop_reason, p.updated_at
		FROM session_prompts p
		JOIN sessions s ON s.id = p.session_id
		WHERE s.project_name = ? AND p.session_id = ?
		ORDER BY p.prompt_index ASC
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID))
	if err != nil {
		return nil, fmt.Errorf("list session prompts: %w", err)
	}
	defer rows.Close()

	out := []SessionPromptRecord{}
	for rows.Next() {
		var rec SessionPromptRecord
		var updatedAt string
		if err := rows.Scan(&rec.SessionID, &rec.PromptIndex, &rec.Title, &rec.StopReason, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan session prompt: %w", err)
		}
		rec.UpdatedAt = parseStoreTime(updatedAt)
		rec.PromptID = formatPromptSeq(rec.PromptIndex)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *sqliteStore) ListSessionPromptsAfterIndex(ctx context.Context, projectName, sessionID string, afterPromptIndex int64) ([]SessionPromptRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.session_id, p.prompt_index, p.title, p.stop_reason, p.updated_at
		FROM session_prompts p
		JOIN sessions s ON s.id = p.session_id
		WHERE s.project_name = ? AND p.session_id = ? AND p.prompt_index > ?
		ORDER BY p.prompt_index ASC
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID), afterPromptIndex)
	if err != nil {
		return nil, fmt.Errorf("list session prompts after index: %w", err)
	}
	defer rows.Close()

	out := []SessionPromptRecord{}
	for rows.Next() {
		var rec SessionPromptRecord
		var updatedAt string
		if err := rows.Scan(&rec.SessionID, &rec.PromptIndex, &rec.Title, &rec.StopReason, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan session prompt after index: %w", err)
		}
		rec.UpdatedAt = parseStoreTime(updatedAt)
		rec.PromptID = formatPromptSeq(rec.PromptIndex)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *sqliteStore) UpsertSessionTurn(ctx context.Context, rec SessionTurnRecord) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	rec.SessionID = strings.TrimSpace(rec.SessionID)
	if rec.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if rec.PromptIndex <= 0 {
		return fmt.Errorf("prompt index is required")
	}
	if rec.TurnIndex <= 0 {
		return fmt.Errorf("turn index is required")
	}
	rec.UpdateJSON = normalizeJSONDoc(rec.UpdateJSON, `{}`)
	rec.ExtraJSON = normalizeJSONDoc(rec.ExtraJSON, `{}`)

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO session_turns (session_id, prompt_index, turn_index, update_index, update_json, extra_json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, prompt_index, turn_index) DO UPDATE SET
			update_index = CASE WHEN excluded.update_index > session_turns.update_index THEN excluded.update_index ELSE session_turns.update_index END,
			update_json = excluded.update_json,
			extra_json = excluded.extra_json
	`, rec.SessionID, rec.PromptIndex, rec.TurnIndex, rec.UpdateIndex, rec.UpdateJSON, rec.ExtraJSON); err != nil {
		return fmt.Errorf("upsert session turn: %w", err)
	}
	return nil
}

func (s *sqliteStore) LoadSessionTurn(ctx context.Context, projectName, sessionID string, promptIndex, turnIndex int64) (*SessionTurnRecord, error) {
	if promptIndex <= 0 || turnIndex <= 0 {
		return nil, nil
	}
	var rec SessionTurnRecord
	err := s.db.QueryRowContext(ctx, `
		SELECT t.session_id, t.prompt_index, t.turn_index, t.update_index, t.update_json, t.extra_json
		FROM session_turns t
		JOIN sessions s ON s.id = t.session_id
		WHERE s.project_name = ? AND t.session_id = ? AND t.prompt_index = ? AND t.turn_index = ?
		LIMIT 1
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID), promptIndex, turnIndex).Scan(&rec.SessionID, &rec.PromptIndex, &rec.TurnIndex, &rec.UpdateIndex, &rec.UpdateJSON, &rec.ExtraJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load session turn: %w", err)
	}
	rec.TurnID = formatPromptTurnSeq(rec.PromptIndex, rec.TurnIndex)
	rec.UpdateJSON = normalizeJSONDoc(rec.UpdateJSON, `{}`)
	rec.ExtraJSON = normalizeJSONDoc(rec.ExtraJSON, `{}`)
	return &rec, nil
}

func (s *sqliteStore) ListSessionTurns(ctx context.Context, projectName, sessionID string, promptIndex int64) ([]SessionTurnRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.session_id, t.prompt_index, t.turn_index, t.update_index, t.update_json, t.extra_json
		FROM session_turns t
		JOIN sessions s ON s.id = t.session_id
		WHERE s.project_name = ? AND t.session_id = ? AND t.prompt_index = ?
		ORDER BY t.turn_index ASC
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID), promptIndex)
	if err != nil {
		return nil, fmt.Errorf("list session turns: %w", err)
	}
	defer rows.Close()

	out := []SessionTurnRecord{}
	for rows.Next() {
		var rec SessionTurnRecord
		if err := rows.Scan(&rec.SessionID, &rec.PromptIndex, &rec.TurnIndex, &rec.UpdateIndex, &rec.UpdateJSON, &rec.ExtraJSON); err != nil {
			return nil, fmt.Errorf("scan session turn: %w", err)
		}
		rec.TurnID = formatPromptTurnSeq(rec.PromptIndex, rec.TurnIndex)
		rec.UpdateJSON = normalizeJSONDoc(rec.UpdateJSON, `{}`)
		rec.ExtraJSON = normalizeJSONDoc(rec.ExtraJSON, `{}`)
		out = append(out, rec)
	}
	return out, rows.Err()
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

func formatPromptSeq(promptIndex int64) string {
	return fmt.Sprintf("p%d", promptIndex)
}

func formatPromptTurnSeq(promptIndex, turnIndex int64) string {
	return fmt.Sprintf("p%d.t%d", promptIndex, turnIndex)
}

func maxInt64(v int64, fallback int64) int64 {
	if v > fallback {
		return v
	}
	return fallback
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
