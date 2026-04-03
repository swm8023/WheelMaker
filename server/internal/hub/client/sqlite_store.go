package client

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS sessions (
    id           TEXT PRIMARY KEY,
    project_name TEXT NOT NULL,
    status       INTEGER NOT NULL DEFAULT 0,
    active_agent TEXT NOT NULL DEFAULT '',
    last_reply   TEXT NOT NULL DEFAULT '',
    acp_session_id TEXT NOT NULL DEFAULT '',
    session_meta TEXT NOT NULL DEFAULT '{}',
    init_meta    TEXT NOT NULL DEFAULT '{}',
    created_at   TEXT NOT NULL,
    last_active  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS session_agents (
    session_id      TEXT NOT NULL,
    agent_name      TEXT NOT NULL,
    acp_session_id  TEXT NOT NULL DEFAULT '',
    config_options  TEXT NOT NULL DEFAULT '[]',
    commands        TEXT NOT NULL DEFAULT '[]',
    title           TEXT NOT NULL DEFAULT '',
    updated_at      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (session_id, agent_name),
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_name);
`

// SQLiteSessionStore implements SessionStore backed by a SQLite database.
type SQLiteSessionStore struct {
	db          *sql.DB
	projectName string
}

// NewSQLiteSessionStore creates a new SQLite-backed SessionStore.
// dbPath is the path to the SQLite database file.
func NewSQLiteSessionStore(dbPath, projectName string) (*SQLiteSessionStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dbPath, err)
	}
	if _, err := db.Exec(sqliteSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &SQLiteSessionStore{db: db, projectName: projectName}, nil
}

func (s *SQLiteSessionStore) Save(ctx context.Context, snap *SessionSnapshot) error {
	sessionMetaJSON, err := json.Marshal(snap.SessionMeta)
	if err != nil {
		return fmt.Errorf("marshal sessionMeta: %w", err)
	}
	initMetaJSON, err := json.Marshal(snap.InitMeta)
	if err != nil {
		return fmt.Errorf("marshal initMeta: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.ExecContext(ctx, `
		INSERT INTO sessions (id, project_name, status, active_agent, last_reply, acp_session_id, session_meta, init_meta, created_at, last_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status=excluded.status, active_agent=excluded.active_agent,
			last_reply=excluded.last_reply, acp_session_id=excluded.acp_session_id,
			session_meta=excluded.session_meta, init_meta=excluded.init_meta,
			last_active=excluded.last_active`,
		snap.ID, s.projectName, int(snap.Status), snap.ActiveAgent,
		snap.LastReply, snap.ACPSessionID,
		string(sessionMetaJSON), string(initMetaJSON),
		snap.CreatedAt.Format(time.RFC3339), snap.LastActiveAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}

	// Delete existing agents then re-insert.
	if _, err := tx.ExecContext(ctx, `DELETE FROM session_agents WHERE session_id = ?`, snap.ID); err != nil {
		return fmt.Errorf("delete agents: %w", err)
	}
	if snap.Agents != nil {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO session_agents (session_id, agent_name, acp_session_id, config_options, commands, title, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare agent insert: %w", err)
		}
		defer stmt.Close()
		for name, as := range snap.Agents {
			optsJSON, _ := json.Marshal(as.ConfigOptions)
			cmdsJSON, _ := json.Marshal(as.Commands)
			if _, err := stmt.ExecContext(ctx, snap.ID, name, as.ACPSessionID, string(optsJSON), string(cmdsJSON), as.Title, as.UpdatedAt); err != nil {
				return fmt.Errorf("insert agent %q: %w", name, err)
			}
		}
	}

	return tx.Commit()
}

func (s *SQLiteSessionStore) Load(ctx context.Context, sessionID string) (*SessionSnapshot, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, status, active_agent, last_reply, acp_session_id, session_meta, init_meta, created_at, last_active
		FROM sessions WHERE id = ? AND project_name = ?`, sessionID, s.projectName)

	snap := &SessionSnapshot{ProjectName: s.projectName}
	var status int
	var sessionMetaJSON, initMetaJSON, createdStr, activeStr string
	err := row.Scan(&snap.ID, &status, &snap.ActiveAgent, &snap.LastReply,
		&snap.ACPSessionID, &sessionMetaJSON, &initMetaJSON, &createdStr, &activeStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan session: %w", err)
	}
	snap.Status = SessionStatus(status)
	snap.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	snap.LastActiveAt, _ = time.Parse(time.RFC3339, activeStr)
	_ = json.Unmarshal([]byte(sessionMetaJSON), &snap.SessionMeta)
	_ = json.Unmarshal([]byte(initMetaJSON), &snap.InitMeta)

	// Load agents.
	rows, err := s.db.QueryContext(ctx, `
		SELECT agent_name, acp_session_id, config_options, commands, title, updated_at
		FROM session_agents WHERE session_id = ?`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query agents: %w", err)
	}
	defer rows.Close()

	snap.Agents = make(map[string]*SessionAgentState)
	for rows.Next() {
		var name, acpSID, optsJSON, cmdsJSON, title, updatedAt string
		if err := rows.Scan(&name, &acpSID, &optsJSON, &cmdsJSON, &title, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		as := &SessionAgentState{ACPSessionID: acpSID, Title: title, UpdatedAt: updatedAt}
		_ = json.Unmarshal([]byte(optsJSON), &as.ConfigOptions)
		_ = json.Unmarshal([]byte(cmdsJSON), &as.Commands)
		snap.Agents[name] = as
	}
	return snap, rows.Err()
}

func (s *SQLiteSessionStore) List(ctx context.Context) ([]SessionSummaryEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, active_agent, created_at, last_active
		FROM sessions WHERE project_name = ?
		ORDER BY last_active DESC`, s.projectName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []SessionSummaryEntry
	for rows.Next() {
		var e SessionSummaryEntry
		var createdStr, activeStr string
		if err := rows.Scan(&e.ID, &e.ActiveAgent, &createdStr, &activeStr); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		e.LastActiveAt, _ = time.Parse(time.RFC3339, activeStr)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *SQLiteSessionStore) Delete(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ? AND project_name = ?`, sessionID, s.projectName)
	return err
}

func (s *SQLiteSessionStore) Close() error {
	return s.db.Close()
}
