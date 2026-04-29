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

	_ "modernc.org/sqlite"
)

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS projects (
	project_name TEXT PRIMARY KEY,
	default_agent_type TEXT NOT NULL DEFAULT '',
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
	agent_type TEXT NOT NULL,
	agent_json TEXT NOT NULL DEFAULT '{}',
	title TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	last_active_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS agent_preferences (
	project_name TEXT NOT NULL,
	agent_type TEXT NOT NULL,
	preference_json TEXT NOT NULL DEFAULT '{}',
	PRIMARY KEY (project_name, agent_type)
);

CREATE TABLE IF NOT EXISTS session_prompts (
	session_id TEXT NOT NULL,
	prompt_index INTEGER NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	stop_reason TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT '',
	turns_json TEXT NOT NULL DEFAULT '',
	turn_index INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (session_id, prompt_index)
);
CREATE INDEX IF NOT EXISTS idx_route_bindings_project ON route_bindings(project_name);
CREATE INDEX IF NOT EXISTS idx_sessions_project_last_active ON sessions(project_name, last_active_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_prompts_session_prompt ON session_prompts(session_id, prompt_index);
`

type PreferenceState struct {
	ConfigOptions []PreferenceConfigOption `json:"configOptions,omitempty"`
	UpdatedAt     string                   `json:"updatedAt,omitempty"`
}

type PreferenceConfigOption struct {
	ID           string `json:"id"`
	CurrentValue string `json:"currentValue,omitempty"`
}

type SessionRecord struct {
	ID           string
	ProjectName  string
	Status       SessionStatus
	AgentType    string
	AgentJSON    string
	Title        string
	Agent        string
	CreatedAt    time.Time
	LastActiveAt time.Time
	InMemory     bool
}

type AgentPreferenceRecord struct {
	ProjectName    string
	AgentType      string
	PreferenceJSON string
}

type SessionPromptRecord struct {
	PromptID    string
	SessionID   string
	PromptIndex int64
	Title       string
	StopReason  string
	UpdatedAt   time.Time
	TurnsJSON   string
	TurnIndex   int64
}

type Store interface {
	LoadRouteBindings(ctx context.Context, projectName string) (map[string]string, error)
	SaveRouteBinding(ctx context.Context, projectName, routeKey, sessionID string) error
	DeleteRouteBinding(ctx context.Context, projectName, routeKey string) error
	LoadProjectDefaultAgent(ctx context.Context, projectName string) (string, error)
	SaveProjectDefaultAgent(ctx context.Context, projectName, agentType string) error

	LoadSession(ctx context.Context, projectName, sessionID string) (*SessionRecord, error)
	SaveSession(ctx context.Context, rec *SessionRecord) error
	ListSessions(ctx context.Context, projectName string) ([]SessionRecord, error)
	DeleteSession(ctx context.Context, projectName, sessionID string) error
	LoadAgentPreference(ctx context.Context, projectName, agentType string) (*AgentPreferenceRecord, error)
	SaveAgentPreference(ctx context.Context, rec AgentPreferenceRecord) error
	UpsertSessionPrompt(ctx context.Context, rec SessionPromptRecord) error
	LoadSessionPrompt(ctx context.Context, projectName, sessionID string, promptIndex int64) (*SessionPromptRecord, error)
	ListSessionPrompts(ctx context.Context, projectName, sessionID string) ([]SessionPromptRecord, error)
	ListSessionPromptsAfterIndex(ctx context.Context, projectName, sessionID string, afterPromptIndex int64) ([]SessionPromptRecord, error)
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
	existingTables, err := sqliteUserTables(db)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("list sqlite tables: %w", err)
	}
	if len(existingTables) == 0 {
		if _, err := db.Exec(sqliteSchema); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("init schema: %w", err)
		}
		if err := checkStoreSchemaDB(db, dbPath); err != nil {
			_ = db.Close()
			return nil, err
		}
	} else if err := validateStoreSchema(db, dbPath, existingTables); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &sqliteStore{db: db}, nil
}

var expectedStoreSchemaColumns = map[string][]string{
	"projects":          {"project_name", "default_agent_type", "updated_at"},
	"route_bindings":    {"project_name", "route_key", "session_id", "created_at", "updated_at"},
	"sessions":          {"id", "project_name", "status", "agent_type", "agent_json", "title", "created_at", "last_active_at"},
	"agent_preferences": {"project_name", "agent_type", "preference_json"},
	"session_prompts":   {"session_id", "prompt_index", "title", "stop_reason", "updated_at", "turns_json", "turn_index"},
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

	return checkStoreSchemaDB(db, dbPath)
}

func checkStoreSchemaDB(db *sql.DB, dbPath string) error {
	existingTables, err := sqliteUserTables(db)
	if err != nil {
		return fmt.Errorf("list sqlite tables: %w", err)
	}
	return validateStoreSchema(db, dbPath, existingTables)
}

func validateStoreSchema(db *sql.DB, dbPath string, existingTables map[string]struct{}) error {
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

func (s *sqliteStore) LoadProjectDefaultAgent(ctx context.Context, projectName string) (string, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT default_agent_type
		FROM projects
		WHERE project_name = ?
	`, strings.TrimSpace(projectName))
	var agentType string
	if err := row.Scan(&agentType); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("load project default agent: %w", err)
	}
	return strings.TrimSpace(agentType), nil
}

func (s *sqliteStore) SaveProjectDefaultAgent(ctx context.Context, projectName, agentType string) error {
	projectName = strings.TrimSpace(projectName)
	agentType = strings.TrimSpace(agentType)
	if projectName == "" {
		return fmt.Errorf("project name is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (project_name, default_agent_type, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(project_name) DO UPDATE SET
			default_agent_type=CASE WHEN excluded.default_agent_type != '' THEN excluded.default_agent_type ELSE projects.default_agent_type END,
			updated_at=excluded.updated_at
	`, projectName, agentType, now)
	if err != nil {
		return fmt.Errorf("save project default agent: %w", err)
	}
	return nil
}

func (s *sqliteStore) LoadAgentPreference(ctx context.Context, projectName, agentType string) (*AgentPreferenceRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT project_name, agent_type, preference_json
		FROM agent_preferences
		WHERE project_name = ? AND agent_type = ?
	`, strings.TrimSpace(projectName), strings.TrimSpace(agentType))

	var rec AgentPreferenceRecord
	if err := row.Scan(&rec.ProjectName, &rec.AgentType, &rec.PreferenceJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("load agent preference: %w", err)
	}
	return &rec, nil
}

func (s *sqliteStore) SaveAgentPreference(ctx context.Context, rec AgentPreferenceRecord) error {
	rec.ProjectName = strings.TrimSpace(rec.ProjectName)
	rec.AgentType = strings.TrimSpace(rec.AgentType)
	if rec.ProjectName == "" || rec.AgentType == "" {
		return fmt.Errorf("project name and agent type are required")
	}
	if strings.TrimSpace(rec.PreferenceJSON) == "" {
		rec.PreferenceJSON = "{}"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_preferences (project_name, agent_type, preference_json)
		VALUES (?, ?, ?)
		ON CONFLICT(project_name, agent_type) DO UPDATE SET
			preference_json=excluded.preference_json
	`, rec.ProjectName, rec.AgentType, rec.PreferenceJSON)
	if err != nil {
		return fmt.Errorf("save agent preference: %w", err)
	}
	return nil
}

func (s *sqliteStore) LoadSession(ctx context.Context, projectName, sessionID string) (*SessionRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, status, agent_type, agent_json, title, created_at, last_active_at
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
		&rec.AgentType,
		&rec.AgentJSON,
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
	rec.AgentType = strings.TrimSpace(rec.AgentType)
	if rec.AgentType == "" {
		return fmt.Errorf("agent type is required")
	}
	if strings.TrimSpace(rec.AgentJSON) == "" {
		rec.AgentJSON = "{}"
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	if rec.LastActiveAt.IsZero() {
		rec.LastActiveAt = rec.CreatedAt
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (
			id, project_name, status, agent_type, agent_json, title, created_at, last_active_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_name=excluded.project_name,
			status=excluded.status,
			agent_type=excluded.agent_type,
			agent_json=excluded.agent_json,
			title=CASE WHEN excluded.title != '' THEN excluded.title ELSE sessions.title END,
			last_active_at=excluded.last_active_at
	`, rec.ID, rec.ProjectName, int(rec.Status), rec.AgentType, rec.AgentJSON, rec.Title,
		rec.CreatedAt.UTC().Format(time.RFC3339Nano), rec.LastActiveAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (s *sqliteStore) ListSessions(ctx context.Context, projectName string) ([]SessionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_name, status, agent_type, agent_json, title, created_at, last_active_at
		FROM sessions
		WHERE project_name = ?
		ORDER BY last_active_at DESC
	`, strings.TrimSpace(projectName))
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	entries := []SessionRecord{}
	for rows.Next() {
		var entry SessionRecord
		var entryProjectName string
		var status int
		var agentType string
		var agentJSON string
		var storedTitle string
		var createdAt string
		var lastActiveAt string
		if err := rows.Scan(&entry.ID, &entryProjectName, &status, &agentType, &agentJSON, &storedTitle, &createdAt, &lastActiveAt); err != nil {
			return nil, fmt.Errorf("scan session list entry: %w", err)
		}
		entry.ProjectName = strings.TrimSpace(entryProjectName)
		entry.Status = SessionStatus(status)
		entry.AgentType = strings.TrimSpace(agentType)
		entry.AgentJSON = firstNonEmpty(strings.TrimSpace(agentJSON), "{}")
		entry.CreatedAt = parseStoreTime(createdAt)
		entry.LastActiveAt = parseStoreTime(lastActiveAt)
		entry.Agent, entry.Title = inferSessionListMetadata(agentType, agentJSON)
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

	turnsJSON := strings.TrimSpace(rec.TurnsJSON)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO session_prompts (session_id, prompt_index, title, stop_reason, updated_at, turns_json, turn_index)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, prompt_index) DO UPDATE SET
			title = CASE WHEN excluded.title != '' THEN excluded.title ELSE session_prompts.title END,
			stop_reason = CASE WHEN excluded.stop_reason != '' THEN excluded.stop_reason ELSE session_prompts.stop_reason END,
			updated_at = CASE WHEN excluded.updated_at > session_prompts.updated_at THEN excluded.updated_at ELSE session_prompts.updated_at END,
			turns_json = CASE WHEN excluded.turns_json != '' THEN excluded.turns_json ELSE session_prompts.turns_json END,
			turn_index = CASE WHEN excluded.turns_json != '' THEN excluded.turn_index ELSE session_prompts.turn_index END
	`, rec.SessionID, rec.PromptIndex, strings.TrimSpace(rec.Title), strings.TrimSpace(rec.StopReason), updatedAt, turnsJSON, rec.TurnIndex); err != nil {
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
		SELECT p.session_id, p.prompt_index, p.title, p.stop_reason, p.updated_at, p.turns_json, p.turn_index
		FROM session_prompts p
		JOIN sessions s ON s.id = p.session_id
		WHERE s.project_name = ? AND p.session_id = ? AND p.prompt_index = ?
		LIMIT 1
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID), promptIndex).Scan(&rec.SessionID, &rec.PromptIndex, &rec.Title, &rec.StopReason, &updatedAt, &rec.TurnsJSON, &rec.TurnIndex)
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
		SELECT p.session_id, p.prompt_index, p.title, p.stop_reason, p.updated_at, p.turns_json, p.turn_index
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
		if err := rows.Scan(&rec.SessionID, &rec.PromptIndex, &rec.Title, &rec.StopReason, &updatedAt, &rec.TurnsJSON, &rec.TurnIndex); err != nil {
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
		SELECT p.session_id, p.prompt_index, p.title, p.stop_reason, p.updated_at, p.turns_json, p.turn_index
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
		if err := rows.Scan(&rec.SessionID, &rec.PromptIndex, &rec.Title, &rec.StopReason, &updatedAt, &rec.TurnsJSON, &rec.TurnIndex); err != nil {
			return nil, fmt.Errorf("scan session prompt after index: %w", err)
		}
		rec.UpdatedAt = parseStoreTime(updatedAt)
		rec.PromptID = formatPromptSeq(rec.PromptIndex)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}

func normalizeJSONDoc(raw string, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return fallback
	}
	return raw
}

// EncodeStoredTurns serialises an ordered turn JSON array for storage in
// session_prompts.turns_json. Returns "" when turns is empty.
func EncodeStoredTurns(turns []string) string {
	if len(turns) == 0 {
		return ""
	}
	entries := make([]string, 0, len(turns))
	for _, updateJSON := range turns {
		entries = append(entries, normalizeJSONDoc(updateJSON, `{}`))
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return ""
	}
	return string(raw)
}

// DecodeStoredTurns parses session_prompts.turns_json back to an ordered turn JSON string slice.
func DecodeStoredTurns(turnsJSON string) ([]string, error) {
	turnsJSON = strings.TrimSpace(turnsJSON)
	if turnsJSON == "" {
		return nil, nil
	}
	entries := []string{}
	if err := json.Unmarshal([]byte(turnsJSON), &entries); err != nil {
		return nil, fmt.Errorf("decode turns_json: %w", err)
	}
	for i := range entries {
		entries[i] = normalizeJSONDoc(entries[i], `{}`)
	}
	return entries, nil
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

func inferSessionListMetadata(agentType, agentJSON string) (string, string) {
	type storedAgentState struct {
		Title string `json:"title,omitempty"`
	}

	state := storedAgentState{}
	if err := json.Unmarshal([]byte(firstNonEmpty(agentJSON, "{}")), &state); err != nil {
		return strings.TrimSpace(agentType), ""
	}
	return strings.TrimSpace(agentType), strings.TrimSpace(state.Title)
}
