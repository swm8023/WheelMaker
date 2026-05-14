package client

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type SessionTurnMigrationOptions struct {
	DBPath      string
	SessionRoot string
	LogWriter   io.Writer
	Backup      bool
}

type SessionTurnMigrationResult struct {
	SessionsConverted int
	TurnsConverted    int64
	BackupPath        string
}

type legacyPromptFile struct {
	ModelName string             `json:"modelName"`
	StartedAt json.RawMessage    `json:"startedAt"`
	UpdatedAt json.RawMessage    `json:"updatedAt"`
	Turns     []legacyPromptTurn `json:"turns"`
}

type legacyPromptTurn struct {
	Content string `json:"content"`
}

func MigrateLegacySessionPromptFilesToTurns(ctx context.Context, opts SessionTurnMigrationOptions) (SessionTurnMigrationResult, error) {
	if err := ctx.Err(); err != nil {
		return SessionTurnMigrationResult{}, err
	}
	opts.DBPath = strings.TrimSpace(opts.DBPath)
	opts.SessionRoot = strings.TrimSpace(opts.SessionRoot)
	if opts.DBPath == "" {
		return SessionTurnMigrationResult{}, fmt.Errorf("db path is required")
	}
	if opts.SessionRoot == "" {
		return SessionTurnMigrationResult{}, fmt.Errorf("session root is required")
	}
	db, err := sql.Open("sqlite", opts.DBPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return SessionTurnMigrationResult{}, fmt.Errorf("open sqlite: %w", err)
	}
	defer db.Close()

	result := SessionTurnMigrationResult{}
	if opts.Backup {
		backupPath, err := backupSQLiteDB(ctx, db, opts.DBPath)
		if err != nil {
			return SessionTurnMigrationResult{}, err
		}
		result.BackupPath = backupPath
		migrationLog(opts.LogWriter, "backup written: %s", backupPath)
	}

	sessions, err := migrationSessions(ctx, db)
	if err != nil {
		return SessionTurnMigrationResult{}, err
	}
	turnStore := newFileSessionTurnStore(opts.SessionRoot)
	latestBySession := map[string]int64{}
	for _, session := range sessions {
		if err := ctx.Err(); err != nil {
			return SessionTurnMigrationResult{}, err
		}
		latest, converted, err := migrateSessionPromptFiles(ctx, opts.LogWriter, turnStore, opts.SessionRoot, session.projectName, session.sessionID)
		if err != nil {
			return SessionTurnMigrationResult{}, err
		}
		latestBySession[session.sessionID] = latest
		if converted {
			result.SessionsConverted++
			result.TurnsConverted += latest
		}
	}
	if err := applySessionTurnMigrationDB(ctx, db, latestBySession); err != nil {
		return SessionTurnMigrationResult{}, err
	}
	if err := checkStoreSchemaDB(db, opts.DBPath); err != nil {
		return SessionTurnMigrationResult{}, err
	}
	return result, nil
}

type migrationSession struct {
	sessionID   string
	projectName string
}

func migrationSessions(ctx context.Context, db *sql.DB) ([]migrationSession, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, project_name FROM sessions ORDER BY project_name, id`)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var out []migrationSession
	for rows.Next() {
		var session migrationSession
		if err := rows.Scan(&session.sessionID, &session.projectName); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func migrateSessionPromptFiles(ctx context.Context, logWriter io.Writer, turnStore *fileSessionTurnStore, root, projectName, sessionID string) (int64, bool, error) {
	sessionDir := filepath.Join(root, safeHistoryPathPart(projectName), safeHistoryPathPart(sessionID))
	promptsDir := filepath.Join(sessionDir, "prompts")
	turnsDir := filepath.Join(sessionDir, "turns")
	promptsExists := dirExists(promptsDir)
	turnsExists := dirExists(turnsDir)
	if promptsExists && turnsExists {
		return 0, false, fmt.Errorf("session %s has both prompts and turns directories", sessionID)
	}
	if turnsExists {
		latest, err := turnStore.latestTurnIndex(ctx, projectName, sessionID)
		return latest, false, err
	}
	if !promptsExists {
		return 0, false, nil
	}
	files, err := legacyPromptFiles(promptsDir)
	if err != nil {
		return 0, false, err
	}
	contents := []string{}
	expectedPromptIndex := int64(1)
	for _, file := range files {
		if file.promptIndex != expectedPromptIndex {
			migrationLog(logWriter, "warning: session %s prompt file gap: got p%06d want p%06d", sessionID, file.promptIndex, expectedPromptIndex)
			expectedPromptIndex = file.promptIndex
		}
		promptTurns, err := convertLegacyPromptFile(file.path)
		if err != nil {
			return 0, false, err
		}
		contents = append(contents, promptTurns...)
		expectedPromptIndex++
	}
	latest, err := turnStore.WriteTurns(ctx, projectName, sessionID, 1, contents)
	if err != nil {
		return 0, false, err
	}
	legacyDir := filepath.Join(sessionDir, "prompts-legacy")
	if dirExists(legacyDir) {
		return 0, false, fmt.Errorf("legacy prompts directory already exists: %s", legacyDir)
	}
	if err := os.Rename(promptsDir, legacyDir); err != nil {
		return 0, false, fmt.Errorf("rename prompts to prompts-legacy: %w", err)
	}
	return latest, true, nil
}

type legacyPromptPath struct {
	promptIndex int64
	path        string
}

func legacyPromptFiles(dir string) ([]legacyPromptPath, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read prompts dir: %w", err)
	}
	files := []legacyPromptPath{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		var promptIndex int64
		if _, err := fmt.Sscanf(entry.Name(), "p%06d.json", &promptIndex); err != nil {
			continue
		}
		files = append(files, legacyPromptPath{
			promptIndex: promptIndex,
			path:        filepath.Join(dir, entry.Name()),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].promptIndex < files[j].promptIndex
	})
	return files, nil
}

func convertLegacyPromptFile(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read prompt file %s: %w", path, err)
	}
	var prompt legacyPromptFile
	if err := json.Unmarshal(raw, &prompt); err != nil {
		return nil, fmt.Errorf("decode prompt file %s: %w", path, err)
	}
	modelName := strings.TrimSpace(prompt.ModelName)
	startedAt := normalizeLegacyPromptTime(prompt.StartedAt)
	completedAt := normalizeLegacyPromptTime(prompt.UpdatedAt)
	out := make([]string, 0, len(prompt.Turns))
	for i, turn := range prompt.Turns {
		content := strings.TrimSpace(turn.Content)
		if content == "" {
			return nil, fmt.Errorf("prompt file %s turn %d has empty content", path, i+1)
		}
		converted, err := injectLegacyPromptBoundaryMetadata(content, modelName, startedAt, completedAt)
		if err != nil {
			return nil, fmt.Errorf("prompt file %s turn %d: %w", path, i+1, err)
		}
		out = append(out, converted)
	}
	return out, nil
}

func injectLegacyPromptBoundaryMetadata(content, modelName, startedAt, completedAt string) (string, error) {
	doc := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return "", fmt.Errorf("invalid content json: %w", err)
	}
	method := rawJSONString(doc["method"])
	if method != acp.IMMethodPromptRequest && method != acp.IMMethodPromptDone {
		return normalizeJSONDoc(content, "{}"), nil
	}
	param := map[string]json.RawMessage{}
	if rawParam := bytes.TrimSpace(doc["param"]); len(rawParam) > 0 && string(rawParam) != "null" {
		if err := json.Unmarshal(rawParam, &param); err != nil {
			return "", fmt.Errorf("invalid param json: %w", err)
		}
	}
	if method == acp.IMMethodPromptRequest {
		if modelName != "" {
			param["modelName"] = mustJSONString(modelName)
		}
		if startedAt != "" {
			param["createdAt"] = mustJSONString(startedAt)
		}
	}
	if method == acp.IMMethodPromptDone && completedAt != "" {
		param["completedAt"] = mustJSONString(completedAt)
	}
	rawParam, err := json.Marshal(param)
	if err != nil {
		return "", err
	}
	doc["param"] = rawParam
	raw, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func applySessionTurnMigrationDB(ctx context.Context, db *sql.DB, latestBySession map[string]int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if _, err := tx.ExecContext(ctx, `ALTER TABLE sessions ADD COLUMN session_sync_json TEXT NOT NULL DEFAULT '{}'`); err != nil {
		// Column may already exist.
	}
	if err := migrationRenameLastActiveAt(ctx, tx); err != nil {
		return err
	}
	for sessionID, latest := range latestBySession {
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET session_sync_json = ? WHERE id = ?`, sessionSyncJSON(latest), sessionID); err != nil {
			return fmt.Errorf("update session sync json: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS session_prompts`); err != nil {
		return fmt.Errorf("drop session_prompts: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_session_prompts_session_prompt`); err != nil {
		return fmt.Errorf("drop session prompt index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_sessions_project_last_active`); err != nil {
		return fmt.Errorf("drop old session updated index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_sessions_project_updated_at ON sessions(project_name, updated_at DESC)`); err != nil {
		return fmt.Errorf("create session updated index: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration tx: %w", err)
	}
	return nil
}

func migrationRenameLastActiveAt(ctx context.Context, tx *sql.Tx) error {
	cols, err := migrationTableColumns(ctx, tx, "sessions")
	if err != nil {
		return err
	}
	hasUpdatedAt := false
	hasLastActiveAt := false
	for _, col := range cols {
		if col == "updated_at" {
			hasUpdatedAt = true
		}
		if col == "last_active_at" {
			hasLastActiveAt = true
		}
	}
	if !hasUpdatedAt && hasLastActiveAt {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE sessions RENAME COLUMN last_active_at TO updated_at`); err != nil {
			return fmt.Errorf("rename sessions.last_active_at: %w", err)
		}
	}
	return nil
}

func migrationTableColumns(ctx context.Context, tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return nil, fmt.Errorf("table info %s: %w", table, err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, fmt.Errorf("scan table info %s: %w", table, err)
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

func backupSQLiteDB(ctx context.Context, db *sql.DB, dbPath string) (string, error) {
	if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return "", fmt.Errorf("sqlite checkpoint before backup: %w", err)
	}
	timestamp := time.Now().Format("20060102-150405")
	backupPath := dbPath + ".bak-" + timestamp
	if err := copyFile(dbPath, backupPath); err != nil {
		return "", fmt.Errorf("backup sqlite db: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		src := dbPath + suffix
		if _, err := os.Stat(src); err == nil {
			if err := copyFile(src, backupPath+suffix); err != nil {
				return "", fmt.Errorf("backup sqlite %s: %w", suffix, err)
			}
		}
	}
	return backupPath, nil
}

func copyFile(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o644)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func rawJSONString(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func mustJSONString(value string) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

func normalizeLegacyPromptTime(raw json.RawMessage) string {
	value := rawJSONString(raw)
	if value == "" {
		return ""
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts.UTC().Format(time.RFC3339Nano)
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts.UTC().Format(time.RFC3339Nano)
	}
	return value
}

func migrationLog(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}
