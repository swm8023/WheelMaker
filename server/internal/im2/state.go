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
CREATE TABLE IF NOT EXISTS im_route_bindings (
	project_name      TEXT NOT NULL,
	route_key         TEXT NOT NULL,
	im_type           TEXT NOT NULL,
	chat_id           TEXT NOT NULL,
	client_session_id TEXT NOT NULL,
	online            INTEGER NOT NULL DEFAULT 0,
	last_seen_at      TEXT NOT NULL,
	updated_at        TEXT NOT NULL,
	PRIMARY KEY (project_name, route_key)
);
CREATE INDEX IF NOT EXISTS idx_im_route_bindings_session ON im_route_bindings(project_name, client_session_id);
CREATE INDEX IF NOT EXISTS idx_im_route_bindings_online ON im_route_bindings(project_name, online);
`

// State is the only external state boundary for IM 2.0.
// IM 2.0 persists chat binding/runtime only (no client session table).
type State interface {
	Load(ctx context.Context) error
	ResolveClientSessionID(ctx context.Context, routeKey string) (string, bool, error)
	BindRouteKey(ctx context.Context, routeKey, imType, chatID, clientSessionID string, critical bool) error
	RebindRouteKey(ctx context.Context, routeKey, clientSessionID string) error
	SetRouteKeyOnline(ctx context.Context, routeKey string, online bool, critical bool) error
	ListSessionRouteBindings(ctx context.Context, clientSessionID string, onlineOnly bool) ([]IMRouteBinding, error)
	Close() error
}

type sqliteState struct {
	db          *sql.DB
	projectName string
	flushDelay  time.Duration

	mu            sync.Mutex
	routeBindings map[string]IMRouteBinding
	dirtyRoutes   map[string]struct{}
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
		routeBindings: map[string]IMRouteBinding{},
		dirtyRoutes:   map[string]struct{}{},
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

	bindings := map[string]IMRouteBinding{}
	rows, err := s.db.QueryContext(ctx, `
		SELECT route_key, im_type, chat_id, client_session_id, online, last_seen_at, updated_at
		FROM im_route_bindings
		WHERE project_name = ?`, s.projectName)
	if err != nil {
		return fmt.Errorf("im2 state: load im_route_bindings: %w", err)
	}
	for rows.Next() {
		var (
			routeKey, imType, chatID, clientSessionID string
			onlineInt                                 int
			lastSeenRaw, updatedAtRaw                 string
		)
		if err := rows.Scan(&routeKey, &imType, &chatID, &clientSessionID, &onlineInt, &lastSeenRaw, &updatedAtRaw); err != nil {
			rows.Close()
			return fmt.Errorf("im2 state: scan im_route_bindings: %w", err)
		}
		bindings[routeKey] = IMRouteBinding{
			ProjectName:     s.projectName,
			RouteKey:        routeKey,
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
		return fmt.Errorf("im2 state: iterate im_route_bindings: %w", err)
	}
	rows.Close()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("im2 state: closed")
	}
	s.routeBindings = bindings
	s.dirtyRoutes = map[string]struct{}{}
	s.mu.Unlock()
	return nil
}

func (s *sqliteState) ResolveClientSessionID(_ context.Context, routeKey string) (string, bool, error) {
	routeKey = strings.TrimSpace(routeKey)
	if routeKey == "" {
		return "", false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", false, fmt.Errorf("im2 state: closed")
	}
	binding, ok := s.routeBindings[routeKey]
	if !ok || strings.TrimSpace(binding.ClientSessionID) == "" {
		return "", false, nil
	}
	return binding.ClientSessionID, true, nil
}

func (s *sqliteState) BindRouteKey(ctx context.Context, routeKey, imType, chatID, clientSessionID string, critical bool) error {
	clientSessionID = strings.TrimSpace(clientSessionID)
	if clientSessionID == "" {
		return fmt.Errorf("im2 state: empty clientSessionID")
	}

	resolvedRouteKey, resolvedIMType, resolvedChatID, err := normalizeRouteParts(routeKey, imType, chatID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("im2 state: closed")
	}
	s.routeBindings[resolvedRouteKey] = IMRouteBinding{
		ProjectName:     s.projectName,
		RouteKey:        resolvedRouteKey,
		IMType:          resolvedIMType,
		ChatID:          resolvedChatID,
		ClientSessionID: clientSessionID,
		Online:          true,
		LastSeenAt:      now,
		UpdatedAt:       now,
	}
	s.dirtyRoutes[resolvedRouteKey] = struct{}{}
	s.mu.Unlock()
	return s.persistWithPolicy(ctx, critical)
}

func (s *sqliteState) RebindRouteKey(ctx context.Context, routeKey, clientSessionID string) error {
	imType, chatID, ok := ParseRouteKey(routeKey)
	if !ok {
		return fmt.Errorf("im2 state: invalid routeKey %q", routeKey)
	}
	return s.BindRouteKey(ctx, routeKey, imType, chatID, clientSessionID, true)
}

func (s *sqliteState) SetRouteKeyOnline(ctx context.Context, routeKey string, online bool, critical bool) error {
	routeKey = strings.TrimSpace(routeKey)
	if routeKey == "" {
		return nil
	}

	now := time.Now().UTC()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("im2 state: closed")
	}
	binding, ok := s.routeBindings[routeKey]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	binding.Online = online
	if online {
		binding.LastSeenAt = now
	}
	binding.UpdatedAt = now
	s.routeBindings[routeKey] = binding
	s.dirtyRoutes[routeKey] = struct{}{}
	s.mu.Unlock()
	return s.persistWithPolicy(ctx, critical)
}

func (s *sqliteState) ListSessionRouteBindings(_ context.Context, clientSessionID string, onlineOnly bool) ([]IMRouteBinding, error) {
	clientSessionID = strings.TrimSpace(clientSessionID)
	if clientSessionID == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("im2 state: closed")
	}
	bindings := make([]IMRouteBinding, 0, 8)
	for _, binding := range s.routeBindings {
		if binding.ClientSessionID != clientSessionID {
			continue
		}
		if onlineOnly && !binding.Online {
			continue
		}
		bindings = append(bindings, binding)
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].UpdatedAt.Equal(bindings[j].UpdatedAt) {
			return bindings[i].RouteKey < bindings[j].RouteKey
		}
		return bindings[i].UpdatedAt.After(bindings[j].UpdatedAt)
	})
	return bindings, nil
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
	routeKeys := mapKeys(s.dirtyRoutes)
	if len(routeKeys) == 0 {
		s.mu.Unlock()
		return nil
	}
	records := make([]IMRouteBinding, 0, len(routeKeys))
	for _, routeKey := range routeKeys {
		if rec, ok := s.routeBindings[routeKey]; ok {
			records = append(records, rec)
		}
	}
	s.dirtyRoutes = map[string]struct{}{}
	s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.requeueDirty(routeKeys)
		return fmt.Errorf("im2 state: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, rec := range records {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO im_route_bindings (
				project_name, route_key, im_type, chat_id, client_session_id, online, last_seen_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_name, route_key) DO UPDATE SET
				im_type = excluded.im_type,
				chat_id = excluded.chat_id,
				client_session_id = excluded.client_session_id,
				online = excluded.online,
				last_seen_at = excluded.last_seen_at,
				updated_at = excluded.updated_at`,
			s.projectName,
			rec.RouteKey,
			rec.IMType,
			rec.ChatID,
			rec.ClientSessionID,
			boolToInt(rec.Online),
			rec.LastSeenAt.UTC().Format(time.RFC3339Nano),
			rec.UpdatedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			s.requeueDirty(routeKeys)
			return fmt.Errorf("im2 state: upsert route_binding %q: %w", rec.RouteKey, err)
		}
	}

	if err := tx.Commit(); err != nil {
		s.requeueDirty(routeKeys)
		return fmt.Errorf("im2 state: commit tx: %w", err)
	}
	return nil
}

func (s *sqliteState) requeueDirty(routeKeys []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	for _, routeKey := range routeKeys {
		s.dirtyRoutes[routeKey] = struct{}{}
	}
	s.scheduleFlushLocked()
}

func normalizeRouteParts(routeKey, imType, chatID string) (string, string, string, error) {
	routeKey = strings.TrimSpace(routeKey)
	if routeKey != "" {
		t, c, ok := ParseRouteKey(routeKey)
		if !ok {
			return "", "", "", fmt.Errorf("im2 state: invalid routeKey %q", routeKey)
		}
		return routeKey, t, c, nil
	}

	routeKey, err := BuildRouteKey(imType, chatID)
	if err != nil {
		return "", "", "", fmt.Errorf("im2 state: build routeKey: %w", err)
	}
	t, c, _ := ParseRouteKey(routeKey)
	return routeKey, t, c, nil
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
