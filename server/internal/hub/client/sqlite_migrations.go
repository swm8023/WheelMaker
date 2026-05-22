package client

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

const sqliteMigrationVersionCodexAppIdentity = 1

func runSQLiteDataMigrations(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("query user_version: %w", err)
	}
	if version >= sqliteMigrationVersionCodexAppIdentity {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin codexapp identity migration: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	statements := []string{
		`UPDATE projects SET default_agent_type = 'codex' WHERE lower(trim(default_agent_type)) = 'codexapp'`,
		`UPDATE sessions SET agent_type = 'codex' WHERE lower(trim(agent_type)) = 'codexapp'`,
		`DELETE FROM agent_preferences
			WHERE lower(trim(agent_type)) = 'codexapp'
			  AND EXISTS (
				SELECT 1 FROM agent_preferences AS p2
				WHERE p2.project_name = agent_preferences.project_name
				  AND lower(trim(p2.agent_type)) = 'codex'
			  )`,
		`UPDATE agent_preferences SET agent_type = 'codex' WHERE lower(trim(agent_type)) = 'codexapp'`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("apply codexapp identity migration: %w", err)
		}
	}
	if err := migrateCodexAppAgentJSON(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", sqliteMigrationVersionCodexAppIdentity)); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit codexapp identity migration: %w", err)
	}
	committed = true
	return nil
}

func migrateCodexAppAgentJSON(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT id, agent_json FROM sessions WHERE instr(lower(agent_json), 'codexapp') > 0`)
	if err != nil {
		return fmt.Errorf("query codexapp agent_json rows: %w", err)
	}
	type sessionAgentJSONUpdate struct {
		id   string
		json string
	}
	updates := []sessionAgentJSONUpdate{}
	for rows.Next() {
		var id string
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan codexapp agent_json row: %w", err)
		}
		updated, ok := rewriteCodexAppAgentInfoName(raw)
		if !ok {
			continue
		}
		updates = append(updates, sessionAgentJSONUpdate{id: id, json: updated})
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close codexapp agent_json rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate codexapp agent_json rows: %w", err)
	}

	for _, update := range updates {
		if _, err := tx.Exec(`UPDATE sessions SET agent_json = ? WHERE id = ?`, update.json, update.id); err != nil {
			return fmt.Errorf("update codexapp agent_json row %q: %w", update.id, err)
		}
	}
	return nil
}

func rewriteCodexAppAgentInfoName(raw string) (string, bool) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(firstNonEmpty(raw, "{}")), &payload); err != nil {
		return "", false
	}
	agentInfo, ok := payload["agentInfo"].(map[string]any)
	if !ok {
		return "", false
	}
	name, _ := agentInfo["name"].(string)
	if !strings.EqualFold(strings.TrimSpace(name), "codexapp") {
		return "", false
	}
	agentInfo["name"] = "codex"
	updated, err := json.Marshal(payload)
	if err != nil {
		return "", false
	}
	return string(updated), true
}
