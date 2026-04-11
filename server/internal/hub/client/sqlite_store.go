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
	title TEXT NOT NULL DEFAULT '',
	last_message_preview TEXT NOT NULL DEFAULT '',
	last_message_at TEXT NOT NULL DEFAULT '',
	message_count INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	last_active_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS session_messages (
	message_id TEXT PRIMARY KEY,
	project_name TEXT NOT NULL,
	session_id TEXT NOT NULL,
	role TEXT NOT NULL,
	kind TEXT NOT NULL,
	body TEXT NOT NULL DEFAULT '',
	blocks_json TEXT NOT NULL DEFAULT '[]',
	options_json TEXT NOT NULL DEFAULT '[]',
	status TEXT NOT NULL DEFAULT 'done',
	source_channel TEXT NOT NULL DEFAULT '',
	source_chat_id TEXT NOT NULL DEFAULT '',
	request_id INTEGER NOT NULL DEFAULT 0,
	aggregate_key TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_route_bindings_project ON route_bindings(project_name);
CREATE INDEX IF NOT EXISTS idx_sessions_project_last_active ON sessions(project_name, last_active_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_messages_project_session ON session_messages(project_name, session_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_session_messages_aggregate_key ON session_messages(project_name, session_id, aggregate_key);
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
	ID                 string
	ProjectName        string
	Status             SessionStatus
	LastReply          string
	ACPSessionID       string
	AgentsJSON         string
	Title              string
	LastMessagePreview string
	LastMessageAt      time.Time
	MessageCount       int
	CreatedAt          time.Time
	LastActiveAt       time.Time
}

type SessionListEntry struct {
	ID            string
	Agent         string
	Title         string
	Preview       string
	Status        SessionStatus
	MessageCount  int
	CreatedAt     time.Time
	LastActiveAt  time.Time
	LastMessageAt time.Time
}

type SessionMessageRecord struct {
	MessageID     string
	SessionID     string
	ProjectName   string
	Role          string
	Kind          string
	Body          string
	Blocks        []acp.ContentBlock
	Options       []acp.PermissionOption
	Status        string
	SourceChannel string
	SourceChatID  string
	RequestID     int64
	AggregateKey  string
	CreatedAt     time.Time
	UpdatedAt     time.Time
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
	AppendSessionMessage(ctx context.Context, rec SessionMessageRecord) error
	UpsertSessionMessage(ctx context.Context, rec SessionMessageRecord) error
	ListSessionMessages(ctx context.Context, projectName, sessionID string) ([]SessionMessageRecord, error)
	HasSessionMessage(ctx context.Context, projectName, sessionID, messageID string) (bool, error)
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
	if err := migrateProjectsTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateSessionsTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &sqliteStore{db: db}, nil
}

func migrateProjectsTable(db *sql.DB) error {
	columns := []struct {
		name string
		ddl  string
	}{
		{
			name: "yolo",
			ddl:  `ALTER TABLE projects ADD COLUMN yolo INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "agent_state_json",
			ddl:  `ALTER TABLE projects ADD COLUMN agent_state_json TEXT NOT NULL DEFAULT '{}'`,
		},
	}
	for _, column := range columns {
		exists, err := sqliteColumnExists(db, "projects", column.name)
		if err != nil {
			return fmt.Errorf("check projects.%s column: %w", column.name, err)
		}
		if exists {
			continue
		}
		if _, err := db.Exec(column.ddl); err != nil {
			return fmt.Errorf("migrate projects.%s column: %w", column.name, err)
		}
	}
	return nil
}

func migrateSessionsTable(db *sql.DB) error {
	messageColumns := []struct {
		name string
		ddl  string
	}{
		{
			name: "blocks_json",
			ddl:  `ALTER TABLE session_messages ADD COLUMN blocks_json TEXT NOT NULL DEFAULT '[]'`,
		},
		{
			name: "options_json",
			ddl:  `ALTER TABLE session_messages ADD COLUMN options_json TEXT NOT NULL DEFAULT '[]'`,
		},
	}
	for _, column := range messageColumns {
		exists, err := sqliteColumnExists(db, "session_messages", column.name)
		if err != nil {
			return fmt.Errorf("check session_messages.%s column: %w", column.name, err)
		}
		if exists {
			continue
		}
		if _, err := db.Exec(column.ddl); err != nil {
			return fmt.Errorf("migrate session_messages.%s column: %w", column.name, err)
		}
	}

	columns := []struct {
		name string
		ddl  string
	}{
		{
			name: "title",
			ddl:  `ALTER TABLE sessions ADD COLUMN title TEXT NOT NULL DEFAULT ''`,
		},
		{
			name: "last_message_preview",
			ddl:  `ALTER TABLE sessions ADD COLUMN last_message_preview TEXT NOT NULL DEFAULT ''`,
		},
		{
			name: "last_message_at",
			ddl:  `ALTER TABLE sessions ADD COLUMN last_message_at TEXT NOT NULL DEFAULT ''`,
		},
		{
			name: "message_count",
			ddl:  `ALTER TABLE sessions ADD COLUMN message_count INTEGER NOT NULL DEFAULT 0`,
		},
	}
	for _, column := range columns {
		exists, err := sqliteColumnExists(db, "sessions", column.name)
		if err != nil {
			return fmt.Errorf("check sessions.%s column: %w", column.name, err)
		}
		if exists {
			continue
		}
		if _, err := db.Exec(column.ddl); err != nil {
			return fmt.Errorf("migrate sessions.%s column: %w", column.name, err)
		}
	}
	return nil
}

func sqliteColumnExists(db *sql.DB, tableName, columnName string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(columnName)) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
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
		SELECT id, status, last_reply, acp_session_id, agents_json, title, last_message_preview, last_message_at, message_count, created_at, last_active_at
		FROM sessions
		WHERE project_name = ? AND id = ?
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID))

	rec := &SessionRecord{}
	var status int
	var lastMessageAt string
	var createdAt string
	var lastActiveAt string
	if err := row.Scan(
		&rec.ID,
		&status,
		&rec.LastReply,
		&rec.ACPSessionID,
		&rec.AgentsJSON,
		&rec.Title,
		&rec.LastMessagePreview,
		&lastMessageAt,
		&rec.MessageCount,
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
	rec.LastMessageAt = parseStoreTime(lastMessageAt)
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
	if rec.LastMessageAt.IsZero() && strings.TrimSpace(rec.LastMessagePreview) != "" {
		rec.LastMessageAt = rec.LastActiveAt
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (
			id, project_name, status, last_reply, acp_session_id, agents_json, title, last_message_preview, last_message_at, message_count, created_at, last_active_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_name=excluded.project_name,
			status=excluded.status,
			last_reply=excluded.last_reply,
			acp_session_id=excluded.acp_session_id,
			agents_json=excluded.agents_json,
			title=CASE WHEN excluded.title != '' THEN excluded.title ELSE sessions.title END,
			last_message_preview=CASE WHEN excluded.last_message_preview != '' THEN excluded.last_message_preview ELSE sessions.last_message_preview END,
			last_message_at=CASE WHEN excluded.last_message_at != '' THEN excluded.last_message_at ELSE sessions.last_message_at END,
			message_count=CASE WHEN excluded.message_count > 0 THEN excluded.message_count ELSE sessions.message_count END,
			last_active_at=excluded.last_active_at
	`, rec.ID, rec.ProjectName, int(rec.Status), rec.LastReply, rec.ACPSessionID, rec.AgentsJSON, rec.Title, rec.LastMessagePreview,
		rec.LastMessageAt.UTC().Format(time.RFC3339Nano), rec.MessageCount,
		rec.CreatedAt.UTC().Format(time.RFC3339Nano), rec.LastActiveAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (s *sqliteStore) ListSessions(ctx context.Context, projectName string) ([]SessionListEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, status, acp_session_id, agents_json, title, last_message_preview, message_count, created_at, last_active_at, last_message_at
		FROM sessions
		WHERE project_name = ?
		ORDER BY CASE WHEN last_message_at = '' THEN last_active_at ELSE last_message_at END DESC, last_active_at DESC
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
		var storedTitle string
		var preview string
		var messageCount int
		var createdAt string
		var lastActiveAt string
		var lastMessageAt string
		if err := rows.Scan(&entry.ID, &status, &acpSessionID, &agentsJSON, &storedTitle, &preview, &messageCount, &createdAt, &lastActiveAt, &lastMessageAt); err != nil {
			return nil, fmt.Errorf("scan session list entry: %w", err)
		}
		entry.Status = SessionStatus(status)
		entry.CreatedAt = parseStoreTime(createdAt)
		entry.LastActiveAt = parseStoreTime(lastActiveAt)
		entry.LastMessageAt = parseStoreTime(lastMessageAt)
		entry.MessageCount = messageCount
		entry.Agent, entry.Title = inferSessionListMetadata(acpSessionID, agentsJSON)
		if strings.TrimSpace(storedTitle) != "" {
			entry.Title = strings.TrimSpace(storedTitle)
		}
		entry.Preview = strings.TrimSpace(preview)
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *sqliteStore) AppendSessionMessage(ctx context.Context, rec SessionMessageRecord) error {
	return s.insertOrUpdateSessionMessage(ctx, rec, false)
}

func (s *sqliteStore) UpsertSessionMessage(ctx context.Context, rec SessionMessageRecord) error {
	return s.insertOrUpdateSessionMessage(ctx, rec, true)
}

func (s *sqliteStore) insertOrUpdateSessionMessage(ctx context.Context, rec SessionMessageRecord, update bool) error {
	rec.MessageID = strings.TrimSpace(rec.MessageID)
	rec.ProjectName = strings.TrimSpace(rec.ProjectName)
	rec.SessionID = strings.TrimSpace(rec.SessionID)
	rec.Role = strings.TrimSpace(rec.Role)
	rec.Kind = strings.TrimSpace(rec.Kind)
	rec.Status = strings.TrimSpace(rec.Status)
	rec.AggregateKey = strings.TrimSpace(rec.AggregateKey)
	if rec.MessageID == "" {
		return fmt.Errorf("message id is required")
	}
	if rec.ProjectName == "" {
		return fmt.Errorf("project name is required")
	}
	if rec.SessionID == "" {
		return fmt.Errorf("session id is required")
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = rec.CreatedAt
	}
	blocksJSON, err := marshalSessionContentBlocks(rec.Blocks)
	if err != nil {
		return err
	}
	optionsJSON, err := marshalSessionPermissionOptions(rec.Options)
	if err != nil {
		return err
	}
	query := `
		INSERT INTO session_messages (
			message_id, project_name, session_id, role, kind, body, blocks_json, options_json, status, source_channel, source_chat_id, request_id, aggregate_key, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if update {
		query += `
		ON CONFLICT(message_id) DO UPDATE SET
			project_name=excluded.project_name,
			session_id=excluded.session_id,
			role=excluded.role,
			kind=excluded.kind,
			body=excluded.body,
			blocks_json=excluded.blocks_json,
			options_json=excluded.options_json,
			status=excluded.status,
			source_channel=excluded.source_channel,
			source_chat_id=excluded.source_chat_id,
			request_id=excluded.request_id,
			aggregate_key=excluded.aggregate_key,
			updated_at=excluded.updated_at`
	}
	_, err = s.db.ExecContext(ctx, query,
		rec.MessageID, rec.ProjectName, rec.SessionID, rec.Role, rec.Kind, rec.Body, blocksJSON, optionsJSON, rec.Status, rec.SourceChannel, rec.SourceChatID, rec.RequestID, rec.AggregateKey,
		rec.CreatedAt.UTC().Format(time.RFC3339Nano), rec.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save session message: %w", err)
	}
	return nil
}

func (s *sqliteStore) ListSessionMessages(ctx context.Context, projectName, sessionID string) ([]SessionMessageRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT message_id, project_name, session_id, role, kind, body, blocks_json, options_json, status, source_channel, source_chat_id, request_id, aggregate_key, created_at, updated_at
		FROM session_messages
		WHERE project_name = ? AND session_id = ?
		ORDER BY created_at ASC, updated_at ASC
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID))
	if err != nil {
		return nil, fmt.Errorf("list session messages: %w", err)
	}
	defer rows.Close()

	var out []SessionMessageRecord
	for rows.Next() {
		var rec SessionMessageRecord
		var blocksJSON string
		var optionsJSON string
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&rec.MessageID, &rec.ProjectName, &rec.SessionID, &rec.Role, &rec.Kind, &rec.Body, &blocksJSON, &optionsJSON, &rec.Status, &rec.SourceChannel, &rec.SourceChatID, &rec.RequestID, &rec.AggregateKey, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan session message: %w", err)
		}
		rec.Blocks = unmarshalSessionContentBlocks(blocksJSON)
		rec.Options = unmarshalSessionPermissionOptions(optionsJSON)
		rec.CreatedAt = parseStoreTime(createdAt)
		rec.UpdatedAt = parseStoreTime(updatedAt)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *sqliteStore) HasSessionMessage(ctx context.Context, projectName, sessionID, messageID string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1 FROM session_messages
		WHERE project_name = ? AND session_id = ? AND message_id = ?
		LIMIT 1
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID), strings.TrimSpace(messageID)).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("has session message: %w", err)
	}
	return true, nil
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

func marshalSessionContentBlocks(blocks []acp.ContentBlock) (string, error) {
	if len(blocks) == 0 {
		return "[]", nil
	}
	raw, err := json.Marshal(blocks)
	if err != nil {
		return "", fmt.Errorf("marshal session content blocks: %w", err)
	}
	return string(raw), nil
}

func marshalSessionPermissionOptions(options []acp.PermissionOption) (string, error) {
	if len(options) == 0 {
		return "[]", nil
	}
	raw, err := json.Marshal(options)
	if err != nil {
		return "", fmt.Errorf("marshal session permission options: %w", err)
	}
	return string(raw), nil
}

func unmarshalSessionContentBlocks(raw string) []acp.ContentBlock {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var blocks []acp.ContentBlock
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		return nil
	}
	return blocks
}

func unmarshalSessionPermissionOptions(raw string) []acp.PermissionOption {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var options []acp.PermissionOption
	if err := json.Unmarshal([]byte(raw), &options); err != nil {
		return nil
	}
	return options
}
