package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swm8023/wheelmaker/internal/hub/client"
)

func main() {
	var dbPath string
	var sessionRoot string
	var noBackup bool
	flag.StringVar(&dbPath, "db", defaultDBPath(), "path to client.sqlite3")
	flag.StringVar(&sessionRoot, "session-root", defaultSessionRoot(), "path to session file root")
	flag.BoolVar(&noBackup, "no-backup", false, "skip sqlite backup")
	flag.Parse()

	result, err := client.MigrateLegacySessionPromptFilesToTurns(context.Background(), client.SessionTurnMigrationOptions{
		DBPath:      dbPath,
		SessionRoot: sessionRoot,
		LogWriter:   os.Stdout,
		Backup:      !noBackup,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "session turn migration failed:", err)
		os.Exit(1)
	}
	fmt.Printf("session turn migration completed: sessionsConverted=%d turnsConverted=%d backup=%s\n", result.SessionsConverted, result.TurnsConverted, result.BackupPath)
}

func defaultDBPath() string {
	return filepath.Join(defaultWheelMakerDir(), "db", "client.sqlite3")
}

func defaultSessionRoot() string {
	return filepath.Join(defaultWheelMakerDir(), "session")
}

func defaultWheelMakerDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".wheelmaker"
	}
	return filepath.Join(home, ".wheelmaker")
}
