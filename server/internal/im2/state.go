package im2

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultStateFlushDelay = 350 * time.Millisecond
)

const stateSchema = `
CREATE TABLE IF NOT EXISTS client_sessions (
	project_name      TEXT NOT NULL,
	client_session_id TEXT NOT NULL,
	updated_at        TEXT NOT NULL,
	PRIMARY KEY (project_name, client_session_id)
);
CREATE TABLE IF NOT EXISTS im_active_chats (
	project_name      TEXT NOT NULL,
	active_chat_id    TEXT NOT NULL,
	im_type           TEXT NOT NULL,
	chat_id           TEXT NOT NULL,
	client_session_id TEXT NOT NULL,
	online            INTEGER NOT NULL DEFAULT 0,
	last_seen_at      TEXT NOT NULL,
	updated_at        TEXT NOT NULL,
	PRIMARY KEY (project_name, active_chat_id)
);
CREATE INDEX IF NOT EXISTS idx_im_active_chats_session ON im_active_chats(project_name, client_session_id);
CREATE INDEX IF NOT EXISTS idx_im_active_chats_online ON im_active_chats(project_name, online);
`

// State is the only external state boundary for IM 2.0.
// Implementations keep memory and persistence consistent with auto-flush behavior.
type State interface {
	Load(ctx context.Context) error
	EnsureClientSession(ctx context.Context, clientSessionID string, critical bool) error
	ResolveClientSessionID(ctx context.Context, activeChatID string) (string, bool, error)
	BindActiveChat(ctx context.Context, activeChatID, imType, chatID, clientSessionID string, critical bool) error
	SetActiveChatOnline(ctx context.Context, activeChatID string, online bool, critical bool) error
	ListSessionActiveChats(ctx context.Context, clientSessionID string, onlineOnly bool) ([]IMActiveChat, error)
	Close() error
}

type clientSessionRecord struct {
	ClientSessionID string
	UpdatedAt       time.Time
}

type sqliteState struct {
	db          *sql.DB
	projectName string
	flushDelay  time.Duration

	mu            sync.Mutex
	sessions      map[string]clientSessionRecord
	activeChats   map[string]IMActiveChat
	dirtySessions map[string]struct{}
	dirtyChats    map[string]struct{}
	flushTimer    *time.Timer
	closed        bool
}

// NewState creates sqlite-backed IM state with default flush behavior.
func NewState(dbPath, projectName string) (State, error) {
	return newSQLiteState(dbPath, projectName, defaultStateFlushDelay)
}

func newSQLiteState(dbPath, projectName string, flushDelay time.Duration) (State, error) {
	pn := strings.TrimSpace(projectName)
	if pn == "" {
		return nil, fmt.Errorf("im2 state: empty project name")
	}
	if flushDelay <= 0 {
		flushDelay = defaultStateFlushDelay
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("im2 state: open sqlite: %w", err)
	}
	if _, err := db.Exec(stateSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("im2 state: init schema: %w", err)
	}

	s := &sqliteState{
		db:            db,
		projectName:   pn,
		flushDelay:    flushDelay,
		sessions:      map[string]clientSessionRecord{},
		activeChats:   map[string]IMActiveChat{},
		dirtySessions: map[string]struct{}{},
		dirtyChats:    map[string]struct{}{},
	}
	if err := s.Load(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *sqliteState) Load(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("im2 state: closed")
	}
	s.mu.Unlock()

	sessions := map[string]clientSessionRecord{}
	rows, err := s.db.QueryContext(ctx, `
		SELECT client_session_id, updated_at
		FROM client_sessions
		WHERE project_name = ?`, s.projectName)
	if err != nil {
		return fmt.Errorf("im2 state: load client_sessions: %w", err)
	}
	for rows.Next() {
		var id, updatedAtRaw string
		if err := rows.Scan(&id, &updatedAtRaw); err != nil {
			rows.Close()
			return fmt.Errorf("im2 state: scan client_sessions: %w", err)
		}
		sessions[id] = clientSessionRecord{
			ClientSessionID: id,
			UpdatedAt:       parseTimestamp(updatedAtRaw),
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("im2 state: iterate client_sessions: %w", err)
	}
	rows.Close()

	activeChats := map[string]IMActiveChat{}
	rows, err = s.db.QueryContext(ctx, `
		SELECT active_chat_id, im_type, chat_id, client_session_id, online, last_seen_at, updated_at
		FROM im_active_chats
		WHERE project_name = ?`, s.projectName)
	if err != nil {
		return fmt.Errorf("im2 state: load im_active_chats: %w", err)
	}
	for rows.Next() {
		var (
			activeChatID, imType, chatID, clientSessionID string
			onlineInt                                     int
			lastSeenRaw, updatedAtRaw                     string
		)
		if err := rows.Scan(&activeChatID, &imType, &chatID, &clientSessionID, &onlineInt, &lastSeenRaw, &updatedAtRaw); err != nil {
			rows.Close()
			return fmt.Errorf("im2 state: scan im_active_chats: %w", err)
		}
		activeChats[activeChatID] = IMActiveChat{
			ProjectName:     s.projectName,
			ActiveChatID:    activeChatID,
			IMType:          imType,
			ChatID:          chatID,
			ClientSessionID: clientSessionID,
			Online:          onlineInt == 1,
			LastSeenAt:      parseTimestamp(lastSeenRaw),
			UpdatedAt:       parseTimestamp(updatedAtRaw),
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("im2 state: iterate im_active_chats: %w", err)
	}
	rows.Close()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("im2 state: closed")
	}
	s.sessions = sessions
	s.activeChats = activeChats
	s.dirtySessions = map[string]struct{}{}
	s.dirtyChats = map[string]struct{}{}
	s.mu.Unlock()
	return nil
}

func (s *sqliteState) EnsureClientSession(ctx context.Context, clientSessionID string, critical bool) error {
	id := strings.TrimSpace(clientSessionID)
	if id == "" {
		return fmt.Errorf("im2 state: empty clientSessionID")
	}

	now := time.Now().UTC()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("im2 state: closed")
	}
	s.sessions[id] = clientSessionRecord{ClientSessionID: id, UpdatedAt: now}
	s.dirtySessions[id] = struct{}{}
	s.mu.Unlock()
	return s.persistWithPolicy(ctx, critical)
}

func (s *sqliteState) ResolveClientSessionID(_ context.Context, activeChatID string) (string, bool, error) {
	id := strings.TrimSpace(activeChatID)
	if id == "" {
		return "", false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", false, fmt.Errorf("im2 state: closed")
	}
	chat, ok := s.activeChats[id]
	if !ok || strings.TrimSpace(chat.ClientSessionID) == "" {
		return "", false, nil
	}
	return chat.ClientSessionID, true, nil
}

func (s *sqliteState) BindActiveChat(ctx context.Context, activeChatID, imType, chatID, clientSessionID string, critical bool) error {
	csid := strings.TrimSpace(clientSessionID)
	if csid == "" {
		return fmt.Errorf("im2 state: empty clientSessionID")
	}

	resolvedActiveChatID, resolvedIMType, resolvedChatID, err := normalizeActiveChatParts(activeChatID, imType, chatID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("im2 state: closed")
	}
	s.sessions[csid] = clientSessionRecord{ClientSessionID: csid, UpdatedAt: now}
	s.dirtySessions[csid] = struct{}{}
	s.activeChats[resolvedActiveChatID] = IMActiveChat{
		ProjectName:     s.projectName,
		ActiveChatID:    resolvedActiveChatID,
		IMType:          resolvedIMType,
		ChatID:          resolvedChatID,
		ClientSessionID: csid,
		Online:          true,
		LastSeenAt:      now,
		UpdatedAt:       now,
	}
	s.dirtyChats[resolvedActiveChatID] = struct{}{}
	s.mu.Unlock()
	return s.persistWithPolicy(ctx, critical)
}

func (s *sqliteState) SetActiveChatOnline(ctx context.Context, activeChatID string, online bool, critical bool) error {
	id := strings.TrimSpace(activeChatID)
	if id == "" {
		return nil
	}

	now := time.Now().UTC()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("im2 state: closed")
	}
	chat, ok := s.activeChats[id]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	chat.Online = online
	if online {
		chat.LastSeenAt = now
	}
	chat.UpdatedAt = now
	s.activeChats[id] = chat
	s.dirtyChats[id] = struct{}{}
	s.mu.Unlock()
	return s.persistWithPolicy(ctx, critical)
}

func (s *sqliteState) ListSessionActiveChats(_ context.Context, clientSessionID string, onlineOnly bool) ([]IMActiveChat, error) {
	csid := strings.TrimSpace(clientSessionID)
	if csid == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("im2 state: closed")
	}
	chats := make([]IMActiveChat, 0, 8)
	for _, chat := range s.activeChats {
		if chat.ClientSessionID != csid {
			continue
		}
		if onlineOnly && !chat.Online {
			continue
		}
		chats = append(chats, chat)
	}
	sort.Slice(chats, func(i, j int) bool {
		if chats[i].UpdatedAt.Equal(chats[j].UpdatedAt) {
			return chats[i].ActiveChatID < chats[j].ActiveChatID
		}
		return chats[i].UpdatedAt.After(chats[j].UpdatedAt)
	})
	return chats, nil
}

func (s *sqliteState) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	if s.flushTimer != nil {
		s.flushTimer.Stop()
		s.flushTimer = nil
	}
	s.mu.Unlock()

	if err := s.flush(context.Background()); err != nil {
		_ = s.db.Close()
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		return err
	}

	err := s.db.Close()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return err
}

func (s *sqliteState) persistWithPolicy(ctx context.Context, critical bool) error {
	if critical {
		return s.flush(ctx)
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("im2 state: closed")
	}
	s.scheduleFlushLocked()
	s.mu.Unlock()
	return nil
}

func (s *sqliteState) scheduleFlushLocked() {
	if s.flushTimer == nil {
		s.flushTimer = time.AfterFunc(s.flushDelay, s.flushAsync)
		return
	}
	s.flushTimer.Reset(s.flushDelay)
}

func (s *sqliteState) flushAsync() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.flush(ctx)
}

func (s *sqliteState) flush(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("im2 state: closed")
	}
	sessionIDs := mapKeys(s.dirtySessions)
	chatIDs := mapKeys(s.dirtyChats)
	if len(sessionIDs) == 0 && len(chatIDs) == 0 {
		s.mu.Unlock()
		return nil
	}
	sessions := make([]clientSessionRecord, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		if rec, ok := s.sessions[id]; ok {
			sessions = append(sessions, rec)
		}
	}
	chats := make([]IMActiveChat, 0, len(chatIDs))
	for _, id := range chatIDs {
		if rec, ok := s.activeChats[id]; ok {
			chats = append(chats, rec)
		}
	}
	s.dirtySessions = map[string]struct{}{}
	s.dirtyChats = map[string]struct{}{}
	s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.requeueDirty(sessionIDs, chatIDs)
		return fmt.Errorf("im2 state: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, rec := range sessions {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO client_sessions (project_name, client_session_id, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(project_name, client_session_id) DO UPDATE SET
				updated_at = excluded.updated_at`,
			s.projectName,
			rec.ClientSessionID,
			rec.UpdatedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			s.requeueDirty(sessionIDs, chatIDs)
			return fmt.Errorf("im2 state: upsert client_session %q: %w", rec.ClientSessionID, err)
		}
	}

	for _, rec := range chats {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO im_active_chats (
				project_name, active_chat_id, im_type, chat_id, client_session_id, online, last_seen_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_name, active_chat_id) DO UPDATE SET
				im_type = excluded.im_type,
				chat_id = excluded.chat_id,
				client_session_id = excluded.client_session_id,
				online = excluded.online,
				last_seen_at = excluded.last_seen_at,
				updated_at = excluded.updated_at`,
			s.projectName,
			rec.ActiveChatID,
			rec.IMType,
			rec.ChatID,
			rec.ClientSessionID,
			boolToInt(rec.Online),
			rec.LastSeenAt.UTC().Format(time.RFC3339Nano),
			rec.UpdatedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			s.requeueDirty(sessionIDs, chatIDs)
			return fmt.Errorf("im2 state: upsert active_chat %q: %w", rec.ActiveChatID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		s.requeueDirty(sessionIDs, chatIDs)
		return fmt.Errorf("im2 state: commit tx: %w", err)
	}
	return nil
}

func (s *sqliteState) requeueDirty(sessionIDs, chatIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	for _, id := range sessionIDs {
		s.dirtySessions[id] = struct{}{}
	}
	for _, id := range chatIDs {
		s.dirtyChats[id] = struct{}{}
	}
	s.scheduleFlushLocked()
}

func normalizeActiveChatParts(activeChatID, imType, chatID string) (string, string, string, error) {
	id := strings.TrimSpace(activeChatID)
	if id != "" {
		t, c, ok := ParseActiveChatID(id)
		if !ok {
			return "", "", "", fmt.Errorf("im2 state: invalid activeChatID %q", activeChatID)
		}
		return id, t, c, nil
	}

	id, err := BuildActiveChatID(imType, chatID)
	if err != nil {
		return "", "", "", fmt.Errorf("im2 state: build active chat id: %w", err)
	}
	t, c, _ := ParseActiveChatID(id)
	return id, t, c, nil
}

func mapKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func parseTimestamp(raw string) time.Time {
	v := strings.TrimSpace(raw)
	if v == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

var _ State = (*sqliteState)(nil)
