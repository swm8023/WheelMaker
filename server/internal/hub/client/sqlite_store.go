package client

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	status INTEGER NOT NULL,
	last_reply TEXT NOT NULL DEFAULT '',
	acp_session_id TEXT NOT NULL DEFAULT '',
	agents_json TEXT NOT NULL DEFAULT '{}',
	title TEXT NOT NULL DEFAULT '',
	last_message_at TEXT NOT NULL DEFAULT '',
	last_sync_index INTEGER NOT NULL DEFAULT 0,
	last_sync_subindex INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL,
	last_active_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS session_records (
	session_id TEXT NOT NULL,
	sync_index INTEGER NOT NULL,
	sync_subindex INTEGER NOT NULL DEFAULT 0,
	time TEXT NOT NULL,
	source TEXT NOT NULL DEFAULT '',
	content_json TEXT NOT NULL,
	meta_json TEXT NOT NULL DEFAULT '{}',
	PRIMARY KEY (session_id, sync_index)
);
CREATE INDEX IF NOT EXISTS idx_route_bindings_project ON route_bindings(project_name);
CREATE INDEX IF NOT EXISTS idx_sessions_project_last_active ON sessions(project_name, last_active_at DESC);
CREATE INDEX IF NOT EXISTS idx_session_records_session_cursor ON session_records(session_id, sync_index, sync_subindex);
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
	ID               string
	ProjectName      string
	Status           SessionStatus
	LastReply        string
	ACPSessionID     string
	AgentsJSON       string
	Title            string
	LastMessageAt    time.Time
	LastSyncIndex    int64
	LastSyncSubIndex int64
	CreatedAt        time.Time
	LastActiveAt     time.Time
}

type SessionListEntry struct {
	ID            string
	ProjectName   string
	Agent         string
	Title         string
	Status        SessionStatus
	CreatedAt     time.Time
	LastActiveAt  time.Time
	LastMessageAt time.Time
	InMemory      bool
}

type SessionMessageRecord struct {
	MessageID     string
	SessionID     string
	ProjectName   string
	Method        string
	ContentJSON   string
	MetaJSON      string
	Source        string
	Time          time.Time
	EventTime     time.Time
	Role          string
	Kind          string
	Body          string
	Blocks        []acp.ContentBlock
	Options       []acp.PermissionOption
	Status        string
	SourceChannel string
	SourceChatID  string
	RequestID     int64
	SyncIndex     int64
	SyncSubIndex  int64
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
	LoadSessionMessage(ctx context.Context, projectName, sessionID, messageID string) (*SessionMessageRecord, error)
	ListSessionMessages(ctx context.Context, projectName, sessionID string) ([]SessionMessageRecord, error)
	ListSessionMessagesAfterIndex(ctx context.Context, projectName, sessionID string, afterIndex int64) ([]SessionMessageRecord, error)
	ListSessionMessagesAfterCursor(ctx context.Context, projectName, sessionID string, afterIndex, afterSubIndex int64) ([]SessionMessageRecord, error)
	HasSessionMessage(ctx context.Context, projectName, sessionID, messageID string) (bool, error)
	DeleteSession(ctx context.Context, projectName, sessionID string) error

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
	if err := migrateProjectsTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateSessionsTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateSessionRecordsTable(db); err != nil {
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
	columns := []struct {
		name string
		ddl  string
	}{
		{
			name: "title",
			ddl:  `ALTER TABLE sessions ADD COLUMN title TEXT NOT NULL DEFAULT ''`,
		},
		{
			name: "last_message_at",
			ddl:  `ALTER TABLE sessions ADD COLUMN last_message_at TEXT NOT NULL DEFAULT ''`,
		},
		{
			name: "last_sync_index",
			ddl:  `ALTER TABLE sessions ADD COLUMN last_sync_index INTEGER NOT NULL DEFAULT 0`,
		},
		{
			name: "last_sync_subindex",
			ddl:  `ALTER TABLE sessions ADD COLUMN last_sync_subindex INTEGER NOT NULL DEFAULT 0`,
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

func migrateSessionRecordsTable(db *sql.DB) error {
	recordsExists, err := sqliteTableExists(db, "session_records")
	if err != nil {
		return fmt.Errorf("check session_records table: %w", err)
	}
	legacyExists, err := sqliteTableExists(db, "session_messages")
	if err != nil {
		return fmt.Errorf("check session_messages table: %w", err)
	}

	shouldRecreate := legacyExists
	if recordsExists {
		requiredColumns := []string{"session_id", "sync_index", "sync_subindex", "time", "source", "content_json", "meta_json"}
		for _, column := range requiredColumns {
			exists, colErr := sqliteColumnExists(db, "session_records", column)
			if colErr != nil {
				return fmt.Errorf("check session_records.%s column: %w", column, colErr)
			}
			if !exists {
				shouldRecreate = true
				break
			}
		}
		legacyColumns := []string{"message_id", "event_time", "created_at", "updated_at"}
		for _, column := range legacyColumns {
			exists, colErr := sqliteColumnExists(db, "session_records", column)
			if colErr != nil {
				return fmt.Errorf("check session_records.%s column: %w", column, colErr)
			}
			if exists {
				shouldRecreate = true
				break
			}
		}
	}

	if shouldRecreate {
		if _, err := db.Exec(`DROP TABLE IF EXISTS session_messages`); err != nil {
			return fmt.Errorf("drop legacy session_messages: %w", err)
		}
		if _, err := db.Exec(`DROP INDEX IF EXISTS idx_session_records_message_id`); err != nil {
			return fmt.Errorf("drop legacy session_records message index: %w", err)
		}
		if _, err := db.Exec(`DROP TABLE IF EXISTS session_records`); err != nil {
			return fmt.Errorf("drop legacy session_records table: %w", err)
		}
		if _, err := db.Exec(`UPDATE sessions SET last_sync_index = 0, last_sync_subindex = 0`); err != nil {
			return fmt.Errorf("reset session cursor after destructive message migration: %w", err)
		}
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS session_records (
			session_id TEXT NOT NULL,
			sync_index INTEGER NOT NULL,
			sync_subindex INTEGER NOT NULL DEFAULT 0,
			time TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			content_json TEXT NOT NULL,
			meta_json TEXT NOT NULL DEFAULT '{}',
			PRIMARY KEY (session_id, sync_index)
		)
	`); err != nil {
		return fmt.Errorf("create session_records: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_session_records_session_cursor ON session_records(session_id, sync_index, sync_subindex)`); err != nil {
		return fmt.Errorf("create session_records cursor index: %w", err)
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_session_records_message_id`); err != nil {
		return fmt.Errorf("drop legacy session_records message index: %w", err)
	}
	return nil
}

func sqliteTableExists(db *sql.DB, tableName string) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, strings.TrimSpace(tableName)).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
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
		SELECT id, status, last_reply, acp_session_id, agents_json, title, last_message_at, last_sync_index, last_sync_subindex, created_at, last_active_at
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
		&lastMessageAt,
		&rec.LastSyncIndex,
		&rec.LastSyncSubIndex,
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
	if rec.LastMessageAt.IsZero() {
		rec.LastMessageAt = rec.LastActiveAt
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (
			id, project_name, status, last_reply, acp_session_id, agents_json, title, last_message_at, last_sync_index, last_sync_subindex, created_at, last_active_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_name=excluded.project_name,
			status=excluded.status,
			last_reply=excluded.last_reply,
			acp_session_id=excluded.acp_session_id,
			agents_json=excluded.agents_json,
			title=CASE WHEN excluded.title != '' THEN excluded.title ELSE sessions.title END,
			last_message_at=CASE WHEN excluded.last_message_at != '' THEN excluded.last_message_at ELSE sessions.last_message_at END,
			last_sync_index=CASE
				WHEN excluded.last_sync_index > sessions.last_sync_index THEN excluded.last_sync_index
				WHEN excluded.last_sync_index = sessions.last_sync_index AND excluded.last_sync_subindex > sessions.last_sync_subindex THEN excluded.last_sync_index
				ELSE sessions.last_sync_index
			END,
			last_sync_subindex=CASE
				WHEN excluded.last_sync_index > sessions.last_sync_index THEN excluded.last_sync_subindex
				WHEN excluded.last_sync_index = sessions.last_sync_index AND excluded.last_sync_subindex > sessions.last_sync_subindex THEN excluded.last_sync_subindex
				ELSE sessions.last_sync_subindex
			END,
			last_active_at=excluded.last_active_at
	`, rec.ID, rec.ProjectName, int(rec.Status), rec.LastReply, rec.ACPSessionID, rec.AgentsJSON, rec.Title,
		rec.LastMessageAt.UTC().Format(time.RFC3339Nano), rec.LastSyncIndex, rec.LastSyncSubIndex,
		rec.CreatedAt.UTC().Format(time.RFC3339Nano), rec.LastActiveAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (s *sqliteStore) ListSessions(ctx context.Context, projectName string) ([]SessionListEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_name, status, acp_session_id, agents_json, title, created_at, last_active_at, last_message_at
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
		var entryProjectName string
		var status int
		var acpSessionID string
		var agentsJSON string
		var storedTitle string
		var createdAt string
		var lastActiveAt string
		var lastMessageAt string
		if err := rows.Scan(&entry.ID, &entryProjectName, &status, &acpSessionID, &agentsJSON, &storedTitle, &createdAt, &lastActiveAt, &lastMessageAt); err != nil {
			return nil, fmt.Errorf("scan session list entry: %w", err)
		}
		entry.ProjectName = strings.TrimSpace(entryProjectName)
		entry.Status = SessionStatus(status)
		entry.CreatedAt = parseStoreTime(createdAt)
		entry.LastActiveAt = parseStoreTime(lastActiveAt)
		entry.LastMessageAt = parseStoreTime(lastMessageAt)
		entry.Agent, entry.Title = inferSessionListMetadata(acpSessionID, agentsJSON)
		if strings.TrimSpace(storedTitle) != "" {
			entry.Title = strings.TrimSpace(storedTitle)
		}
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

func normalizeSessionRecordSource(rec SessionMessageRecord) string {
	if strings.TrimSpace(rec.Source) != "" {
		return strings.TrimSpace(rec.Source)
	}
	channel := strings.TrimSpace(rec.SourceChannel)
	chatID := strings.TrimSpace(rec.SourceChatID)
	if channel == "" && chatID == "" {
		return ""
	}
	if channel == "" {
		return chatID
	}
	if chatID == "" {
		return channel
	}
	return channel + ":" + chatID
}

func inferSessionRecordMethod(rec SessionMessageRecord) string {
	if strings.TrimSpace(rec.Method) != "" {
		return strings.TrimSpace(rec.Method)
	}
	if rec.RequestID != 0 || len(rec.Options) > 0 || strings.EqualFold(strings.TrimSpace(rec.Kind), "permission") {
		return "session.permission"
	}
	if strings.EqualFold(strings.TrimSpace(rec.Role), "user") {
		return "session.prompt"
	}
	return "session.update"
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

func buildSessionRecordContentJSON(rec SessionMessageRecord) (string, string, error) {
	method := inferSessionRecordMethod(rec)
	if strings.TrimSpace(rec.ContentJSON) != "" {
		normalized := normalizeJSONDoc(rec.ContentJSON, `{"method":"`+method+`"}`)
		var doc map[string]any
		if err := json.Unmarshal([]byte(normalized), &doc); err == nil {
			if current, ok := doc["method"].(string); ok && strings.TrimSpace(current) != "" {
				method = strings.TrimSpace(current)
			} else {
				doc["method"] = method
				raw, err := json.Marshal(doc)
				if err == nil {
					normalized = string(raw)
				}
			}
		}
		return normalized, method, nil
	}
	payload := map[string]any{}
	if strings.TrimSpace(rec.Role) != "" {
		payload["role"] = strings.TrimSpace(rec.Role)
	}
	if strings.TrimSpace(rec.Kind) != "" {
		if method == "session.update" {
			payload["updateMethod"] = strings.TrimSpace(rec.Kind)
		} else {
			payload["kind"] = strings.TrimSpace(rec.Kind)
		}
	}
	if strings.TrimSpace(rec.Body) != "" {
		payload["text"] = rec.Body
	}
	if len(rec.Blocks) > 0 {
		payload["blocks"] = rec.Blocks
	}
	if len(rec.Options) > 0 {
		payload["options"] = rec.Options
	}
	if strings.TrimSpace(rec.Status) != "" {
		payload["status"] = strings.TrimSpace(rec.Status)
	}
	if rec.RequestID != 0 {
		payload["requestId"] = rec.RequestID
	}
	doc := map[string]any{"method": method}
	if len(payload) > 0 {
		doc["payload"] = payload
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return "", "", fmt.Errorf("marshal session record content: %w", err)
	}
	return string(raw), method, nil
}

func buildSessionRecordMetaJSON(rec SessionMessageRecord) (string, error) {
	if strings.TrimSpace(rec.MetaJSON) != "" {
		return normalizeJSONDoc(rec.MetaJSON, "{}"), nil
	}
	return "{}", nil
}

func deriveSessionRecordBodyFromUpdate(update acp.SessionUpdate) string {
	switch strings.TrimSpace(update.SessionUpdate) {
	case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateUserMessageChunk, acp.SessionUpdateAgentThoughtChunk:
		return extractTextChunk(update.Content)
	case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
		return renderSessionToolStatus(update)
	default:
		return extractTextChunk(update.Content)
	}
}

func hydrateSessionRecordFromUpdate(rec *SessionMessageRecord, update acp.SessionUpdate) {
	if rec == nil {
		return
	}
	rec.Kind = firstNonEmpty(strings.TrimSpace(rec.Kind), strings.TrimSpace(update.SessionUpdate))
	body := deriveSessionRecordBodyFromUpdate(update)
	if strings.TrimSpace(body) != "" {
		rec.Body = body
	}
	rec.Status = firstNonEmpty(strings.TrimSpace(rec.Status), strings.TrimSpace(update.Status))
	switch strings.TrimSpace(update.SessionUpdate) {
	case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateAgentThoughtChunk:
		rec.Role = firstNonEmpty(strings.TrimSpace(rec.Role), "assistant")
	case acp.SessionUpdateUserMessageChunk:
		rec.Role = firstNonEmpty(strings.TrimSpace(rec.Role), "user")
	case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
		rec.Role = firstNonEmpty(strings.TrimSpace(rec.Role), "system")
		rec.AggregateKey = firstNonEmpty(strings.TrimSpace(rec.AggregateKey), strings.TrimSpace(update.ToolCallID))
	default:
		rec.Role = firstNonEmpty(strings.TrimSpace(rec.Role), "system")
	}
}

func hydrateSessionRecordLegacyFields(rec *SessionMessageRecord) {
	if rec == nil {
		return
	}
	rec.ContentJSON = normalizeJSONDoc(rec.ContentJSON, `{"method":"session.update"}`)
	rec.MetaJSON = normalizeJSONDoc(rec.MetaJSON, "{}")

	var content struct {
		Method string `json:"method"`
		Params struct {
			Update acp.SessionUpdate `json:"update"`
		} `json:"params"`
		Payload struct {
			Role         string                 `json:"role"`
			Kind         string                 `json:"kind"`
			UpdateMethod string                 `json:"updateMethod"`
			Text         string                 `json:"text"`
			Blocks       []acp.ContentBlock     `json:"blocks"`
			Options      []acp.PermissionOption `json:"options"`
			Status       string                 `json:"status"`
			RequestID    int64                  `json:"requestId"`
			AggregateKey string                 `json:"aggregateKey"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(rec.ContentJSON), &content); err == nil {
		rec.Method = strings.TrimSpace(content.Method)
		if strings.TrimSpace(content.Params.Update.SessionUpdate) != "" {
			hydrateSessionRecordFromUpdate(rec, content.Params.Update)
		} else {
			rec.Role = firstNonEmpty(strings.TrimSpace(rec.Role), strings.TrimSpace(content.Payload.Role))
			if strings.TrimSpace(content.Payload.UpdateMethod) != "" {
				rec.Kind = firstNonEmpty(strings.TrimSpace(rec.Kind), strings.TrimSpace(content.Payload.UpdateMethod))
			} else {
				rec.Kind = firstNonEmpty(strings.TrimSpace(rec.Kind), strings.TrimSpace(content.Payload.Kind))
			}
			if strings.TrimSpace(content.Payload.Text) != "" {
				rec.Body = content.Payload.Text
			}
			rec.Blocks = content.Payload.Blocks
			rec.Options = content.Payload.Options
			rec.Status = firstNonEmpty(strings.TrimSpace(rec.Status), strings.TrimSpace(content.Payload.Status))
			rec.RequestID = firstNonZeroInt64(rec.RequestID, content.Payload.RequestID)
			rec.AggregateKey = firstNonEmpty(strings.TrimSpace(rec.AggregateKey), strings.TrimSpace(content.Payload.AggregateKey))
		}
	}
	if rec.Method == "" {
		rec.Method = inferSessionRecordMethod(*rec)
	}
	parts := strings.SplitN(strings.TrimSpace(rec.Source), ":", 2)
	if len(parts) > 0 {
		rec.SourceChannel = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		rec.SourceChatID = strings.TrimSpace(parts[1])
	}
}
func (s *sqliteStore) insertOrUpdateSessionMessage(ctx context.Context, rec SessionMessageRecord, update bool) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	rec.ProjectName = strings.TrimSpace(rec.ProjectName)
	rec.SessionID = strings.TrimSpace(rec.SessionID)
	if rec.ProjectName == "" {
		return fmt.Errorf("project name is required")
	}
	if rec.SessionID == "" {
		return fmt.Errorf("session id is required")
	}

	now := time.Now().UTC()
	if rec.Time.IsZero() {
		switch {
		case !rec.EventTime.IsZero():
			rec.Time = rec.EventTime
		case !rec.UpdatedAt.IsZero():
			rec.Time = rec.UpdatedAt
		case !rec.CreatedAt.IsZero():
			rec.Time = rec.CreatedAt
		default:
			rec.Time = now
		}
	}
	rec.Time = rec.Time.UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = rec.Time
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = rec.Time
	}
	rec.EventTime = rec.Time

	contentJSON, method, err := buildSessionRecordContentJSON(rec)
	if err != nil {
		return err
	}
	rec.Method = method
	metaJSON, err := buildSessionRecordMetaJSON(rec)
	if err != nil {
		return err
	}
	source := normalizeSessionRecordSource(rec)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session record tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var lastSyncIndex int64
	var lastSyncSubIndex int64
	err = tx.QueryRowContext(ctx, `
		SELECT last_sync_index, last_sync_subindex
		FROM sessions
		WHERE project_name = ? AND id = ?
	`, rec.ProjectName, rec.SessionID).Scan(&lastSyncIndex, &lastSyncSubIndex)
	if err == sql.ErrNoRows {
		return fmt.Errorf("session %q not found", rec.SessionID)
	}
	if err != nil {
		return fmt.Errorf("load session cursor: %w", err)
	}

	var rowSubIndex int64
	hasExisting := false
	if update && rec.SyncIndex > 0 {
		err = tx.QueryRowContext(ctx, `
			SELECT sync_subindex
			FROM session_records
			WHERE session_id = ? AND sync_index = ?
		`, rec.SessionID, rec.SyncIndex).Scan(&rowSubIndex)
		hasExisting = err == nil
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("load session record by index: %w", err)
		}
	}

	if hasExisting {
		rec.SyncSubIndex = rowSubIndex + 1
	} else {
		rec.SyncIndex = lastSyncIndex + 1
		rec.SyncSubIndex = 0
	}
	rec.MessageID = formatSessionRecordSeq(rec.SyncIndex, rec.SyncSubIndex)
	storedTime := rec.Time.UTC().Format(time.RFC3339Nano)

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO session_records (
			session_id, sync_index, sync_subindex, time, source, content_json, meta_json
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, sync_index) DO UPDATE SET
			sync_subindex=excluded.sync_subindex,
			time=excluded.time,
			source=excluded.source,
			content_json=excluded.content_json,
			meta_json=excluded.meta_json
	`, rec.SessionID, rec.SyncIndex, rec.SyncSubIndex, storedTime, source, contentJSON, metaJSON); err != nil {
		return fmt.Errorf("save session record: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE sessions
		SET last_sync_index = ?, last_sync_subindex = ?, last_active_at = ?
		WHERE project_name = ? AND id = ?
	`, rec.SyncIndex, rec.SyncSubIndex, storedTime, rec.ProjectName, rec.SessionID); err != nil {
		return fmt.Errorf("update session cursor: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session record tx: %w", err)
	}
	return nil
}

func (s *sqliteStore) ListSessionMessages(ctx context.Context, projectName, sessionID string) ([]SessionMessageRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.session_id, r.sync_index, r.sync_subindex, r.time, r.source, r.content_json, r.meta_json
		FROM session_records r
		JOIN sessions s ON s.id = r.session_id
		WHERE s.project_name = ? AND r.session_id = ?
		ORDER BY r.sync_index ASC, r.sync_subindex ASC
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID))
	if err != nil {
		return nil, fmt.Errorf("list session records: %w", err)
	}
	defer rows.Close()

	var out []SessionMessageRecord
	for rows.Next() {
		var rec SessionMessageRecord
		var storedTime string
		if err := rows.Scan(&rec.SessionID, &rec.SyncIndex, &rec.SyncSubIndex, &storedTime, &rec.Source, &rec.ContentJSON, &rec.MetaJSON); err != nil {
			return nil, fmt.Errorf("scan session record: %w", err)
		}
		rec.ProjectName = strings.TrimSpace(projectName)
		rec.Time = parseStoreTime(storedTime)
		rec.EventTime = rec.Time
		rec.CreatedAt = rec.Time
		rec.UpdatedAt = rec.Time
		rec.MessageID = formatSessionRecordSeq(rec.SyncIndex, rec.SyncSubIndex)
		hydrateSessionRecordLegacyFields(&rec)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *sqliteStore) LoadSessionMessage(ctx context.Context, projectName, sessionID, messageID string) (*SessionMessageRecord, error) {
	idx, _, ok := parseSessionRecordSeq(messageID)
	if !ok {
		parsed, err := strconv.ParseInt(strings.TrimSpace(messageID), 10, 64)
		if err != nil {
			return nil, nil
		}
		idx = parsed
	}
	if idx <= 0 {
		return nil, nil
	}
	rows, err := s.ListSessionMessagesAfterCursor(ctx, projectName, sessionID, idx-1, -1)
	if err != nil {
		return nil, err
	}
	for _, rec := range rows {
		if rec.SyncIndex == idx {
			copyRec := rec
			return &copyRec, nil
		}
	}
	return nil, nil
}

func (s *sqliteStore) ListSessionMessagesAfterIndex(ctx context.Context, projectName, sessionID string, afterIndex int64) ([]SessionMessageRecord, error) {
	return s.ListSessionMessagesAfterCursor(ctx, projectName, sessionID, afterIndex, 0)
}

func (s *sqliteStore) ListSessionMessagesAfterCursor(ctx context.Context, projectName, sessionID string, afterIndex, afterSubIndex int64) ([]SessionMessageRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.session_id, r.sync_index, r.sync_subindex, r.time, r.source, r.content_json, r.meta_json
		FROM session_records r
		JOIN sessions s ON s.id = r.session_id
		WHERE s.project_name = ? AND r.session_id = ? AND (r.sync_index > ? OR (r.sync_index = ? AND r.sync_subindex > ?))
		ORDER BY r.sync_index ASC, r.sync_subindex ASC
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID), afterIndex, afterIndex, afterSubIndex)
	if err != nil {
		return nil, fmt.Errorf("list session records after cursor: %w", err)
	}
	defer rows.Close()

	var out []SessionMessageRecord
	for rows.Next() {
		var rec SessionMessageRecord
		var storedTime string
		if err := rows.Scan(&rec.SessionID, &rec.SyncIndex, &rec.SyncSubIndex, &storedTime, &rec.Source, &rec.ContentJSON, &rec.MetaJSON); err != nil {
			return nil, fmt.Errorf("scan session record after cursor: %w", err)
		}
		rec.ProjectName = strings.TrimSpace(projectName)
		rec.Time = parseStoreTime(storedTime)
		rec.EventTime = rec.Time
		rec.CreatedAt = rec.Time
		rec.UpdatedAt = rec.Time
		rec.MessageID = formatSessionRecordSeq(rec.SyncIndex, rec.SyncSubIndex)
		hydrateSessionRecordLegacyFields(&rec)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *sqliteStore) HasSessionMessage(ctx context.Context, projectName, sessionID, messageID string) (bool, error) {
	idx, _, ok := parseSessionRecordSeq(messageID)
	if !ok {
		parsed, err := strconv.ParseInt(strings.TrimSpace(messageID), 10, 64)
		if err != nil {
			return false, nil
		}
		idx = parsed
	}
	if idx <= 0 {
		return false, nil
	}
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1
		FROM session_records r
		JOIN sessions s ON s.id = r.session_id
		WHERE s.project_name = ? AND r.session_id = ? AND r.sync_index = ?
		LIMIT 1
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID), idx).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("has session record: %w", err)
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

func formatSessionRecordSeq(index, subIndex int64) string {
	return fmt.Sprintf("%d.%d", index, subIndex)
}

func parseSessionRecordSeq(seq string) (int64, int64, bool) {
	seq = strings.TrimSpace(seq)
	parts := strings.Split(seq, ".")
	if len(parts) != 2 {
		return 0, 0, false
	}
	idx, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return 0, 0, false
	}
	sub, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return idx, sub, true
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
