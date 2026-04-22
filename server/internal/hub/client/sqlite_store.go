package client

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

type SessionTurnMessageRecord struct {
	MessageID     string
	SessionID     string
	ProjectName   string
	Method        string
	ContentJSON   string
	MetaJSON      string
	Source        string
	Time          time.Time
	EventTime     time.Time
	Body          string
	Blocks        []acp.ContentBlock
	Options       []acp.PermissionOption
	SourceChannel string
	SourceChatID  string
	RequestID     int64
	SyncIndex     int64
	SyncSubIndex  int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
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
	AppendSessionTurnMessage(ctx context.Context, rec SessionTurnMessageRecord) error
	UpsertSessionTurnMessage(ctx context.Context, rec SessionTurnMessageRecord) error
	LoadSessionTurnMessage(ctx context.Context, projectName, sessionID, messageID string) (*SessionTurnMessageRecord, error)
	ListSessionTurnMessages(ctx context.Context, projectName, sessionID string) ([]SessionTurnMessageRecord, error)
	ListSessionTurnMessagesAfterIndex(ctx context.Context, projectName, sessionID string, afterIndex int64) ([]SessionTurnMessageRecord, error)
	ListSessionTurnMessagesAfterCursor(ctx context.Context, projectName, sessionID string, afterIndex, afterSubIndex int64) ([]SessionTurnMessageRecord, error)
	HasSessionTurnMessage(ctx context.Context, projectName, sessionID, messageID string) (bool, error)

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
	if err := migratePromptTurnTables(db); err != nil {
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
	titleExists, err := sqliteColumnExists(db, "sessions", "title")
	if err != nil {
		return fmt.Errorf("check sessions.title column: %w", err)
	}
	if !titleExists {
		if _, err := db.Exec(`ALTER TABLE sessions ADD COLUMN title TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("migrate sessions.title column: %w", err)
		}
	}

	legacyColumns := []string{"last_reply", "last_message_at", "last_sync_index", "last_sync_subindex"}
	hasLegacy := false
	for _, column := range legacyColumns {
		exists, err := sqliteColumnExists(db, "sessions", column)
		if err != nil {
			return fmt.Errorf("check sessions.%s column: %w", column, err)
		}
		if exists {
			hasLegacy = true
		}
	}
	if !hasLegacy {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin sessions migration tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS sessions_new (
			id TEXT PRIMARY KEY,
			project_name TEXT NOT NULL,
			status INTEGER NOT NULL,
			acp_session_id TEXT NOT NULL DEFAULT '',
			agents_json TEXT NOT NULL DEFAULT '{}',
			title TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			last_active_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create sessions_new: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO sessions_new (id, project_name, status, acp_session_id, agents_json, title, created_at, last_active_at)
		SELECT id, project_name, status, acp_session_id, agents_json, COALESCE(title, ''), created_at, last_active_at
		FROM sessions
	`); err != nil {
		return fmt.Errorf("copy sessions to sessions_new: %w", err)
	}

	if _, err := tx.Exec(`DROP TABLE sessions`); err != nil {
		return fmt.Errorf("drop legacy sessions table: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE sessions_new RENAME TO sessions`); err != nil {
		return fmt.Errorf("rename sessions_new: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_project_last_active ON sessions(project_name, last_active_at DESC)`); err != nil {
		return fmt.Errorf("create sessions index after migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sessions migration tx: %w", err)
	}
	return nil
}

func migrateSessionRecordsTable(db *sql.DB) error {
	if _, err := db.Exec(`DROP TABLE IF EXISTS session_messages`); err != nil {
		return fmt.Errorf("drop legacy session_messages: %w", err)
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_session_records_session_cursor`); err != nil {
		return fmt.Errorf("drop legacy session_records cursor index: %w", err)
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_session_records_message_id`); err != nil {
		return fmt.Errorf("drop legacy session_records message index: %w", err)
	}
	if _, err := db.Exec(`DROP TABLE IF EXISTS session_records`); err != nil {
		return fmt.Errorf("drop legacy session_records table: %w", err)
	}
	return nil
}

func migratePromptTurnTables(db *sql.DB) error {
	promptsExists, err := sqliteTableExists(db, "session_prompts")
	if err != nil {
		return fmt.Errorf("check session_prompts table: %w", err)
	}
	if !promptsExists {
		if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS session_prompts (
			session_id TEXT NOT NULL,
			prompt_index INTEGER NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			stop_reason TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (session_id, prompt_index)
		)`); err != nil {
			return fmt.Errorf("create session_prompts: %w", err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_session_prompts_session_prompt ON session_prompts(session_id, prompt_index)`); err != nil {
		return fmt.Errorf("create session_prompts index: %w", err)
	}
	promptUpdateIndexExists, err := sqliteColumnExists(db, "session_prompts", "update_index")
	if err != nil {
		return fmt.Errorf("check session_prompts.update_index column: %w", err)
	}
	if promptUpdateIndexExists {
		if err := rebuildSessionPromptsWithoutUpdateIndex(db); err != nil {
			return fmt.Errorf("drop legacy session_prompts.update_index: %w", err)
		}
	}

	turnsExists, err := sqliteTableExists(db, "session_turns")
	if err != nil {
		return fmt.Errorf("check session_turns table: %w", err)
	}
	if !turnsExists {
		if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS session_turns (
			session_id TEXT NOT NULL,
			prompt_index INTEGER NOT NULL,
			turn_index INTEGER NOT NULL,
			update_index INTEGER NOT NULL DEFAULT 0,
			update_json TEXT NOT NULL DEFAULT '{}',
			extra_json TEXT NOT NULL DEFAULT '{}',
			PRIMARY KEY (session_id, prompt_index, turn_index)
		)`); err != nil {
			return fmt.Errorf("create session_turns: %w", err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_session_turns_session_prompt_turn ON session_turns(session_id, prompt_index, turn_index)`); err != nil {
		return fmt.Errorf("create session_turns index: %w", err)
	}
	return nil
}

func rebuildSessionPromptsWithoutUpdateIndex(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin session_prompts rebuild tx: %w", err)
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.Exec(`
		CREATE TABLE session_prompts_new (
			session_id TEXT NOT NULL,
			prompt_index INTEGER NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			stop_reason TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (session_id, prompt_index)
		)
	`); err != nil {
		return fmt.Errorf("create session_prompts_new: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO session_prompts_new (session_id, prompt_index, title, stop_reason, updated_at)
		SELECT session_id, prompt_index, title, stop_reason, updated_at
		FROM session_prompts
	`); err != nil {
		return fmt.Errorf("copy session_prompts rows: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE session_prompts`); err != nil {
		return fmt.Errorf("drop legacy session_prompts: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE session_prompts_new RENAME TO session_prompts`); err != nil {
		return fmt.Errorf("rename session_prompts_new: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_session_prompts_session_prompt ON session_prompts(session_id, prompt_index)`); err != nil {
		return fmt.Errorf("recreate session_prompts index: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session_prompts rebuild tx: %w", err)
	}
	rollback = false
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

func (s *sqliteStore) AppendSessionTurnMessage(ctx context.Context, rec SessionTurnMessageRecord) error {
	return s.insertOrUpdateSessionTurnMessage(ctx, rec, false)
}

func (s *sqliteStore) UpsertSessionTurnMessage(ctx context.Context, rec SessionTurnMessageRecord) error {
	return s.insertOrUpdateSessionTurnMessage(ctx, rec, true)
}

func (s *sqliteStore) LoadSessionTurnMessage(ctx context.Context, projectName, sessionID, messageID string) (*SessionTurnMessageRecord, error) {
	return s.loadSessionTurnMessage(ctx, projectName, sessionID, messageID)
}

func (s *sqliteStore) ListSessionTurnMessages(ctx context.Context, projectName, sessionID string) ([]SessionTurnMessageRecord, error) {
	return s.listSessionTurnMessages(ctx, projectName, sessionID)
}

func (s *sqliteStore) ListSessionTurnMessagesAfterIndex(ctx context.Context, projectName, sessionID string, afterIndex int64) ([]SessionTurnMessageRecord, error) {
	return s.listSessionTurnMessagesAfterIndex(ctx, projectName, sessionID, afterIndex)
}

func (s *sqliteStore) ListSessionTurnMessagesAfterCursor(ctx context.Context, projectName, sessionID string, afterIndex, afterSubIndex int64) ([]SessionTurnMessageRecord, error) {
	return s.listSessionTurnMessagesAfterCursor(ctx, projectName, sessionID, afterIndex, afterSubIndex)
}

func (s *sqliteStore) HasSessionTurnMessage(ctx context.Context, projectName, sessionID, messageID string) (bool, error) {
	return s.hasSessionTurnMessage(ctx, projectName, sessionID, messageID)
}
func normalizeSessionTurnSource(rec SessionTurnMessageRecord) string {
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

func isSessionPromptMethod(method string) bool {
	method = strings.TrimSpace(method)
	return method == acp.MethodSessionPrompt || method == "session.prompt"
}

func isSessionPermissionMethod(method string) bool {
	method = strings.TrimSpace(method)
	return method == acp.MethodRequestPermission || method == "session.permission"
}

func inferSessionTurnMethod(rec SessionTurnMessageRecord) string {
	if strings.TrimSpace(rec.Method) != "" {
		return strings.TrimSpace(rec.Method)
	}
	if strings.TrimSpace(rec.ContentJSON) != "" {
		var doc struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal([]byte(rec.ContentJSON), &doc); err == nil {
			if strings.TrimSpace(doc.Method) != "" {
				return strings.TrimSpace(doc.Method)
			}
		}
	}
	if rec.RequestID != 0 || len(rec.Options) > 0 {
		return acp.MethodRequestPermission
	}
	return acp.MethodSessionUpdate
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

func buildSessionTurnMetaJSON(rec SessionTurnMessageRecord) (string, error) {
	if strings.TrimSpace(rec.MetaJSON) != "" {
		return normalizeJSONDoc(rec.MetaJSON, "{}"), nil
	}
	return "{}", nil
}

func deriveSessionTurnBodyFromUpdate(update acp.SessionUpdate) string {
	switch strings.TrimSpace(update.SessionUpdate) {
	case acp.SessionUpdateAgentMessageChunk, acp.SessionUpdateUserMessageChunk, acp.SessionUpdateAgentThoughtChunk:
		return extractTextChunk(update.Content)
	case acp.SessionUpdateToolCall, acp.SessionUpdateToolCallUpdate:
		return renderSessionToolStatus(update)
	default:
		return extractTextChunk(update.Content)
	}
}

func hydrateSessionTurnFromUpdate(rec *SessionTurnMessageRecord, update acp.SessionUpdate) {
	if rec == nil {
		return
	}
	body := deriveSessionTurnBodyFromUpdate(update)
	if strings.TrimSpace(body) != "" {
		rec.Body = body
	}
}

func hydrateSessionTurnLegacyFields(rec *SessionTurnMessageRecord) {
	if rec == nil {
		return
	}
	rec.ContentJSON = normalizeJSONDoc(rec.ContentJSON, `{"method":"`+acp.MethodSessionUpdate+`"}`)
	rec.MetaJSON = normalizeJSONDoc(rec.MetaJSON, "{}")

	var content struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
		Params struct {
			Prompt   []acp.ContentBlock     `json:"prompt"`
			Update   acp.SessionUpdate      `json:"update"`
			ToolCall acp.ToolCallRef        `json:"toolCall"`
			Options  []acp.PermissionOption `json:"options"`
		} `json:"params"`
		Result struct {
			StopReason string               `json:"stopReason"`
			Outcome    acp.PermissionResult `json:"outcome"`
		} `json:"result"`
		Payload struct {
			Role         string                 `json:"role"`
			Kind         string                 `json:"kind"`
			UpdateMethod string                 `json:"updateMethod"`
			Text         string                 `json:"text"`
			Blocks       []acp.ContentBlock     `json:"blocks"`
			Options      []acp.PermissionOption `json:"options"`
			Status       string                 `json:"status"`
			RequestID    int64                  `json:"requestId"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(rec.ContentJSON), &content); err == nil {
		rec.Method = strings.TrimSpace(content.Method)
		rec.RequestID = firstNonZeroInt64(rec.RequestID, content.ID)

		switch strings.TrimSpace(content.Method) {
		case acp.MethodSessionPrompt, "session.prompt":
			if len(content.Params.Prompt) > 0 {
				rec.Blocks = cloneSessionContentBlocks(content.Params.Prompt)
				rec.Body = firstNonEmpty(rec.Body, PromptPreview(content.Params.Prompt))
			}
			if strings.TrimSpace(content.Result.StopReason) != "" {
				rec.Body = firstNonEmpty(rec.Body, strings.TrimSpace(content.Result.StopReason))
			}
		case acp.MethodRequestPermission, "session.permission":
			rec.Body = firstNonEmpty(rec.Body, strings.TrimSpace(content.Params.ToolCall.Title))
			if len(rec.Options) == 0 {
				rec.Options = cloneSessionPermissionOptions(content.Params.Options)
			}
		}

		if strings.TrimSpace(content.Params.Update.SessionUpdate) != "" {
			hydrateSessionTurnFromUpdate(rec, content.Params.Update)
		}

		// Legacy payload fallback for pre-ACP wrapped content.
		if strings.TrimSpace(content.Payload.Text) != "" {
			rec.Body = content.Payload.Text
		}
		if len(rec.Blocks) == 0 {
			rec.Blocks = cloneSessionContentBlocks(content.Payload.Blocks)
		}
		if len(rec.Options) == 0 {
			rec.Options = cloneSessionPermissionOptions(content.Payload.Options)
		}
		rec.RequestID = firstNonZeroInt64(rec.RequestID, content.Payload.RequestID)
	}
	if rec.Method == "" {
		rec.Method = inferSessionTurnMethod(*rec)
	}
	parts := strings.SplitN(strings.TrimSpace(rec.Source), ":", 2)
	if len(parts) > 0 {
		rec.SourceChannel = strings.TrimSpace(parts[0])
	}
	if len(parts) > 1 {
		rec.SourceChatID = strings.TrimSpace(parts[1])
	}
}
func (s *sqliteStore) insertOrUpdateSessionTurnMessage(ctx context.Context, rec SessionTurnMessageRecord, update bool) error {
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

	rec.Method = inferSessionTurnMethod(rec)
	contentJSON := strings.TrimSpace(rec.ContentJSON)
	if contentJSON == "" {
		contentJSON = "{}"
	}
	storedTime := time.Now().UTC().Format(time.RFC3339Nano)
	if !rec.UpdatedAt.IsZero() {
		storedTime = rec.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}

	metaJSON, err := buildSessionTurnMetaJSON(rec)
	if err != nil {
		return err
	}
	rec.MetaJSON = metaJSON

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session record tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var sessionCount int64
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM sessions
		WHERE project_name = ? AND id = ?
	`, rec.ProjectName, rec.SessionID).Scan(&sessionCount)
	if err != nil {
		return fmt.Errorf("check session exists: %w", err)
	}
	if sessionCount == 0 {
		return fmt.Errorf("session %q not found", rec.SessionID)
	}

	var existingTurn *sessionTurnLookup
	if update && rec.SyncIndex > 0 {
		existingTurn, err = findSessionTurnBySyncIndexTx(ctx, tx, rec.SessionID, rec.SyncIndex)
		if err != nil {
			return err
		}
	}

	if existingTurn != nil {
		rec.SyncSubIndex = maxInt64(existingTurn.UpdateIndex, 0)
	} else {
		var turnCount int64
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM session_turns WHERE session_id = ?`, rec.SessionID).Scan(&turnCount); err != nil {
			return fmt.Errorf("load session turn count: %w", err)
		}
		rec.SyncIndex = turnCount + 1
		rec.SyncSubIndex = 0
	}
	rec.MessageID = formatSessionTurnSeq(rec.SyncIndex, rec.SyncSubIndex)

	if err := upsertPromptTurnTx(ctx, tx, rec, existingTurn, contentJSON, storedTime); err != nil {
		return fmt.Errorf("upsert prompt/turn projection: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session record tx: %w", err)
	}
	return nil
}

type sessionTurnLookup struct {
	PromptIndex int64
	TurnIndex   int64
	UpdateIndex int64
}

func findSessionTurnBySyncIndexTx(ctx context.Context, tx *sql.Tx, sessionID string, syncIndex int64) (*sessionTurnLookup, error) {
	if syncIndex <= 0 {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT prompt_index, turn_index, update_index
		FROM session_turns
		WHERE session_id = ?
		ORDER BY prompt_index ASC, turn_index ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session_turns by sync index: %w", err)
	}
	defer rows.Close()

	var current int64
	for rows.Next() {
		current++
		var lookup sessionTurnLookup
		if err := rows.Scan(&lookup.PromptIndex, &lookup.TurnIndex, &lookup.UpdateIndex); err != nil {
			return nil, fmt.Errorf("scan session_turn by sync index: %w", err)
		}
		if current != syncIndex {
			continue
		}
		return &lookup, nil
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session_turn by sync index: %w", err)
	}
	return nil, nil
}

func parsePromptStopReasonFromContentJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var doc struct {
		Method string `json:"method"`
		Result struct {
			StopReason string `json:"stopReason"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return ""
	}
	if !isSessionPromptMethod(doc.Method) {
		return ""
	}
	return strings.TrimSpace(doc.Result.StopReason)
}

func upsertPromptTurnTx(ctx context.Context, tx *sql.Tx, rec SessionTurnMessageRecord, existingTurn *sessionTurnLookup, contentJSON, storedTime string) error {

	var latestPromptIndex int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(prompt_index), 0) FROM session_prompts WHERE session_id = ?`, rec.SessionID).Scan(&latestPromptIndex); err != nil {
		return fmt.Errorf("load latest prompt index: %w", err)
	}

	promptIndex := latestPromptIndex
	if existingTurn != nil {
		promptIndex = existingTurn.PromptIndex
	} else if isSessionPromptMethod(rec.Method) {
		promptIndex = latestPromptIndex + 1
	}
	if promptIndex <= 0 {
		promptIndex = 1
	}

	var existingTitle string
	var existingStopReason string
	err := tx.QueryRowContext(ctx, `
		SELECT title, stop_reason
		FROM session_prompts
		WHERE session_id = ? AND prompt_index = ?
	`, rec.SessionID, promptIndex).Scan(&existingTitle, &existingStopReason)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load session prompt projection: %w", err)
	}
	title := strings.TrimSpace(existingTitle)
	if isSessionPromptMethod(rec.Method) {
		if strings.TrimSpace(rec.Body) != "" {
			title = strings.TrimSpace(rec.Body)
			if len([]rune(title)) > 64 {
				title = string([]rune(title)[:64])
			}
		}
	}
	stopReason := strings.TrimSpace(existingStopReason)
	if parsed := parsePromptStopReasonFromContentJSON(contentJSON); parsed != "" {
		stopReason = parsed
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO session_prompts (session_id, prompt_index, title, stop_reason, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(session_id, prompt_index) DO UPDATE SET
			title = excluded.title,
			stop_reason = excluded.stop_reason,
			updated_at = excluded.updated_at
	`, rec.SessionID, promptIndex, title, stopReason, storedTime); err != nil {
		return fmt.Errorf("upsert session_prompts projection: %w", err)
	}

	turnIndex := int64(0)
	turnUpdateIndex := int64(0)
	if existingTurn != nil {
		turnIndex = existingTurn.TurnIndex
		turnUpdateIndex = existingTurn.UpdateIndex + 1
	} else {
		if isSessionPromptMethod(rec.Method) {
			turnIndex = 1
		} else {
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(turn_index), 0) FROM session_turns WHERE session_id = ? AND prompt_index = ?`, rec.SessionID, promptIndex).Scan(&turnIndex); err != nil {
				return fmt.Errorf("load next turn index: %w", err)
			}
			turnIndex++
		}
		turnUpdateIndex = 1
	}

	turnJSON := normalizeJSONDoc(contentJSON, `{"method":"`+acp.MethodSessionUpdate+`"}`)
	extraJSONRaw := []byte("{}")

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO session_turns (session_id, prompt_index, turn_index, update_index, update_json, extra_json)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id, prompt_index, turn_index) DO UPDATE SET
			update_index = excluded.update_index,
			update_json = excluded.update_json,
			extra_json = excluded.extra_json
	`, rec.SessionID, promptIndex, turnIndex, turnUpdateIndex, turnJSON, string(extraJSONRaw)); err != nil {
		return fmt.Errorf("upsert session_turns projection: %w", err)
	}
	return nil
}

type sessionTurnRow struct {
	SessionID       string
	PromptIndex     int64
	TurnIndex       int64
	UpdateIndex     int64
	PromptUpdatedAt string
	UpdateJSON      string
}

func (s *sqliteStore) listSessionTurnRows(ctx context.Context, projectName, sessionID string) ([]sessionTurnRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.session_id, t.prompt_index, t.turn_index, t.update_index, p.updated_at, t.update_json
		FROM session_turns t
		JOIN sessions s ON s.id = t.session_id
		LEFT JOIN session_prompts p ON p.session_id = t.session_id AND p.prompt_index = t.prompt_index
		WHERE s.project_name = ? AND t.session_id = ?
		ORDER BY t.prompt_index ASC, t.turn_index ASC
	`, strings.TrimSpace(projectName), strings.TrimSpace(sessionID))
	if err != nil {
		return nil, fmt.Errorf("list session turns: %w", err)
	}
	defer rows.Close()

	out := make([]sessionTurnRow, 0)
	for rows.Next() {
		var row sessionTurnRow
		if err := rows.Scan(&row.SessionID, &row.PromptIndex, &row.TurnIndex, &row.UpdateIndex, &row.PromptUpdatedAt, &row.UpdateJSON); err != nil {
			return nil, fmt.Errorf("scan session turn: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session turns: %w", err)
	}
	return out, nil
}

type hydratedSessionTurn struct {
	Record      SessionTurnMessageRecord
	PromptIndex int64
	TurnIndex   int64
}

func hydrateSessionTurnFromRow(projectName string, fallbackSyncIndex int64, row sessionTurnRow) hydratedSessionTurn {
	rec := SessionTurnMessageRecord{
		SessionID:    strings.TrimSpace(row.SessionID),
		ProjectName:  strings.TrimSpace(projectName),
		ContentJSON:  normalizeJSONDoc(row.UpdateJSON, `{"method":"`+acp.MethodSessionUpdate+`"}`),
		MetaJSON:     "{}",
		SyncIndex:    fallbackSyncIndex,
		SyncSubIndex: maxInt64(row.UpdateIndex-1, 0),
	}
	hydrateSessionTurnLegacyFields(&rec)
	rec.Time = parseStoreTime(strings.TrimSpace(row.PromptUpdatedAt))
	rec.EventTime = rec.Time
	rec.CreatedAt = rec.Time
	rec.UpdatedAt = rec.Time
	rec.MessageID = formatSessionTurnSeq(rec.SyncIndex, rec.SyncSubIndex)
	return hydratedSessionTurn{Record: rec, PromptIndex: row.PromptIndex, TurnIndex: row.TurnIndex}
}

func filterAndSortSessionTurns(rows []sessionTurnRow, projectName string, afterIndex, afterSubIndex int64) []SessionTurnMessageRecord {
	hydrated := make([]hydratedSessionTurn, 0, len(rows))
	fallback := int64(0)
	for _, row := range rows {
		fallback++
		hydrated = append(hydrated, hydrateSessionTurnFromRow(projectName, fallback, row))
	}
	sort.Slice(hydrated, func(i, j int) bool {
		left := hydrated[i]
		right := hydrated[j]
		if left.Record.SyncIndex != right.Record.SyncIndex {
			return left.Record.SyncIndex < right.Record.SyncIndex
		}
		if left.Record.SyncSubIndex != right.Record.SyncSubIndex {
			return left.Record.SyncSubIndex < right.Record.SyncSubIndex
		}
		if left.PromptIndex != right.PromptIndex {
			return left.PromptIndex < right.PromptIndex
		}
		return left.TurnIndex < right.TurnIndex
	})

	out := make([]SessionTurnMessageRecord, 0, len(hydrated))
	for _, item := range hydrated {
		rec := item.Record
		if rec.SyncIndex > afterIndex || (rec.SyncIndex == afterIndex && rec.SyncSubIndex > afterSubIndex) {
			out = append(out, rec)
		}
	}
	return out
}

func (s *sqliteStore) listSessionTurnMessages(ctx context.Context, projectName, sessionID string) ([]SessionTurnMessageRecord, error) {
	rows, err := s.listSessionTurnRows(ctx, projectName, sessionID)
	if err != nil {
		return nil, err
	}
	return filterAndSortSessionTurns(rows, projectName, 0, -1), nil
}

func (s *sqliteStore) loadSessionTurnMessage(ctx context.Context, projectName, sessionID, messageID string) (*SessionTurnMessageRecord, error) {
	idx, _, ok := parseSessionTurnSeq(messageID)
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
	rows, err := s.listSessionTurnMessagesAfterCursor(ctx, projectName, sessionID, idx-1, -1)
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

func (s *sqliteStore) listSessionTurnMessagesAfterIndex(ctx context.Context, projectName, sessionID string, afterIndex int64) ([]SessionTurnMessageRecord, error) {
	return s.listSessionTurnMessagesAfterCursor(ctx, projectName, sessionID, afterIndex, 0)
}

func (s *sqliteStore) listSessionTurnMessagesAfterCursor(ctx context.Context, projectName, sessionID string, afterIndex, afterSubIndex int64) ([]SessionTurnMessageRecord, error) {
	rows, err := s.listSessionTurnRows(ctx, projectName, sessionID)
	if err != nil {
		return nil, err
	}
	return filterAndSortSessionTurns(rows, projectName, afterIndex, afterSubIndex), nil
}

func (s *sqliteStore) hasSessionTurnMessage(ctx context.Context, projectName, sessionID, messageID string) (bool, error) {
	rec, err := s.loadSessionTurnMessage(ctx, projectName, sessionID, messageID)
	if err != nil {
		return false, err
	}
	return rec != nil, nil
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
func formatSessionTurnSeq(index, subIndex int64) string {
	return fmt.Sprintf("%d.%d", index, subIndex)
}

func parseSessionTurnSeq(seq string) (int64, int64, bool) {
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
func maxInt64(v int64, fallback int64) int64 {
	if v > fallback {
		return v
	}
	return fallback
}

func firstNonZeroInt64(v int64, fallback int64) int64 {
	if v != 0 {
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
