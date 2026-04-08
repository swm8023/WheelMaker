# Server Observability And Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add startup/runtime observability, ACP debug logging with redaction/rotation, and long-wait error reporting (observation-only, no new auto-recovery behavior).

**Architecture:** Introduce a small ACP log formatting/redaction layer in `internal/hub/agent`, keep logger plumbing centralized in `internal/shared`, and add session wait-observer helpers in `internal/hub/client` so prompt path behavior stays unchanged while timeout warnings/errors and IM ERROR notifications become deterministic and testable.

**Tech Stack:** Go 1.26, standard library (`bufio`, `encoding/json`, `time`, `filepath`), existing `shared` logger API, existing client/hub test harness.

---

## File Structure

- Create `server/internal/hub/agent/acp_log.go`
- Create `server/internal/hub/agent/acp_log_test.go`
- Modify `server/internal/hub/agent/acp_process.go`
- Create `server/internal/hub/agent/acp_process_test.go`

- Create `server/internal/shared/debug_rotator.go`
- Create `server/internal/shared/debug_rotator_test.go`
- Modify `server/internal/shared/logger.go`

- Create `server/internal/hub/client/session_observe.go`
- Create `server/internal/hub/client/session_observe_test.go`
- Modify `server/internal/hub/client/session.go`

- Modify `server/internal/hub/client/permission.go`
- Modify `server/internal/hub/client/client.go`
- Modify `server/internal/hub/hub.go`
- Modify `server/cmd/wheelmaker/main.go`
- Modify `server/internal/hub/hub_test.go`

---

### Task 1: Build ACP Log Format/Redaction Utilities

**Files:**
- Create: `server/internal/hub/agent/acp_log.go`
- Test: `server/internal/hub/agent/acp_log_test.go`

- [ ] **Step 1: Write the failing tests for ACP log shape, redaction, truncation**

```go
package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatACPLogLine_MinimalShape(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","method":"session/prompt","params":{"sessionId":"sess-1","token":"abc"}}`)
	line := formatACPLogLine('>', payload)
	if !strings.HasPrefix(line, "[acp] > {sess-1 session/prompt} ") {
		t.Fatalf("line=%q", line)
	}
	if strings.Contains(line, "jsonrpc") {
		t.Fatalf("line contains verbose metadata: %q", line)
	}
}

func TestRedactACPPayload_JSONKeys(t *testing.T) {
	raw := []byte(`{"authorization":"Bearer X","nested":{"token":"abc","password":"p"}}`)
	redacted := redactACPPayload(raw)
	s := string(redacted)
	if strings.Contains(s, "Bearer X") || strings.Contains(s, "abc") || strings.Contains(s, "\"p\"") {
		t.Fatalf("redaction failed: %s", s)
	}
	if !strings.Contains(s, "***") {
		t.Fatalf("expected masked marker: %s", s)
	}
	var obj map[string]any
	if err := json.Unmarshal(redacted, &obj); err != nil {
		t.Fatalf("redacted json invalid: %v", err)
	}
}

func TestRedactACPPayload_Truncate64KB(t *testing.T) {
	base := strings.Repeat("x", acpDebugPayloadMaxBytes+1024)
	raw := []byte(`{"method":"session/prompt","params":{"sessionId":"s","content":"` + base + `"}}`)
	out := redactAndTrimACPPayload(raw)
	if len(out) > acpDebugPayloadMaxBytes {
		t.Fatalf("len=%d, want <=%d", len(out), acpDebugPayloadMaxBytes)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hub/agent -run "TestFormatACPLogLine_MinimalShape|TestRedactACPPayload_JSONKeys|TestRedactACPPayload_Truncate64KB" -count=1`
Expected: FAIL (undefined `formatACPLogLine`/`redactACPPayload`/`redactAndTrimACPPayload`).

- [ ] **Step 3: Write minimal implementation**

```go
package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const acpDebugPayloadMaxBytes = 64 * 1024

var acpSensitiveKeys = map[string]struct{}{
	"token": {}, "authorization": {}, "cookie": {}, "secret": {},
	"api_key": {}, "access_token": {}, "refresh_token": {}, "password": {},
}

func formatACPLogLine(dir rune, raw []byte) string {
	sessionID, method := extractACPLogSessionMethod(raw)
	payload := string(redactAndTrimACPPayload(raw))
	return fmt.Sprintf("[acp] %c {%s %s} %s", dir, sessionID, method, payload)
}

func extractACPLogSessionMethod(raw []byte) (string, string) {
	sessionID := "-"
	method := "-"
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return sessionID, method
	}
	if v, ok := m["method"].(string); ok && strings.TrimSpace(v) != "" {
		method = strings.TrimSpace(v)
	}
	if params, ok := m["params"].(map[string]any); ok {
		if sid, ok := params["sessionId"].(string); ok && strings.TrimSpace(sid) != "" {
			sessionID = strings.TrimSpace(sid)
		}
	}
	if sessionID == "-" {
		if sid, ok := m["sessionId"].(string); ok && strings.TrimSpace(sid) != "" {
			sessionID = strings.TrimSpace(sid)
		}
	}
	return sessionID, method
}

func redactAndTrimACPPayload(raw []byte) []byte {
	redacted := redactACPPayload(raw)
	if len(redacted) <= acpDebugPayloadMaxBytes {
		return redacted
	}
	return redacted[:acpDebugPayloadMaxBytes]
}

func redactACPPayload(raw []byte) []byte {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return []byte(redactPlainText(string(raw)))
	}
	sanitizeJSONValue(v)
	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return []byte(redactPlainText(string(raw)))
	}
	return bytes.TrimSpace(buf.Bytes())
}

func sanitizeJSONValue(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, vv := range x {
			if isSensitiveKey(k) {
				x[k] = "***"
				continue
			}
			sanitizeJSONValue(vv)
		}
	case []any:
		for i := range x {
			sanitizeJSONValue(x[i])
		}
	}
}

func isSensitiveKey(k string) bool {
	_, ok := acpSensitiveKeys[strings.ToLower(strings.TrimSpace(k))]
	return ok
}

func redactPlainText(s string) string {
	repls := []string{"token", "authorization", "cookie", "secret", "api_key", "password"}
	out := s
	for _, key := range repls {
		out = strings.ReplaceAll(out, key+":", key+":***")
		out = strings.ReplaceAll(out, key+"=", key+"=***")
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/hub/agent -run "TestFormatACPLogLine_MinimalShape|TestRedactACPPayload_JSONKeys|TestRedactACPPayload_Truncate64KB" -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/hub/agent/acp_log.go internal/hub/agent/acp_log_test.go
git commit -m "test+feat(agent): add ACP log format redaction helpers"
```

---

### Task 2: Integrate ACP Process Stream Logging (`>`, `<`, `!`)

**Files:**
- Modify: `server/internal/hub/agent/acp_process.go`
- Test: `server/internal/hub/agent/acp_process_test.go`

- [ ] **Step 1: Write failing tests for outbound/inbound/stderr log routing**

```go
package agent

import (
	"bytes"
	"testing"

	shared "github.com/swm8023/wheelmaker/internal/shared"
)

func TestLogOutboundACPDebugLine(t *testing.T) {
	var buf bytes.Buffer
	if err := shared.Setup(shared.LoggerConfig{Level: shared.LevelDebug}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer shared.Close()
	shared.SetOutput(&buf)

	raw := []byte(`{"method":"session/prompt","params":{"sessionId":"sess-1"}}`)
	logACPDebugRaw('>', raw)

	if got := buf.String(); got == "" || !bytes.Contains([]byte(got), []byte("[acp] > {sess-1 session/prompt}")) {
		t.Fatalf("unexpected outbound log: %q", got)
	}
}

func TestLogACPStderrLineAsError(t *testing.T) {
	var buf bytes.Buffer
	if err := shared.Setup(shared.LoggerConfig{Level: shared.LevelWarn}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer shared.Close()
	shared.SetOutput(&buf)

	logACPStderrLine("panic: worker crashed")
	if got := buf.String(); !bytes.Contains([]byte(got), []byte("[acp] ! {- -} panic: worker crashed")) {
		t.Fatalf("unexpected stderr log: %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hub/agent -run "TestLogOutboundACPDebugLine|TestLogACPStderrLineAsError" -count=1`
Expected: FAIL (undefined `logACPDebugRaw`/`logACPStderrLine`).

- [ ] **Step 3: Write minimal implementation in `acp_process.go`**

```go
// imports
import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/swm8023/wheelmaker/internal/protocol"
	shared "github.com/swm8023/wheelmaker/internal/shared"
)

// struct fields
stderr io.ReadCloser

func logACPDebugRaw(dir rune, raw []byte) {
	shared.Debug("%s", formatACPLogLine(dir, raw))
}

func logACPStderrLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	shared.Error("[acp] ! {- -} %s", line)
}

func (p *ACPProcess) Start() error {
	cmd := exec.Command(p.exePath, p.exeArgs...)
	cmd.Env = append(cmd.Environ(), p.env...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("agent acp process: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("agent acp process: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("agent acp process: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("agent acp process: start process: %w", err)
	}

	p.cmd = cmd
	p.stdin = stdin
	p.stdout = stdout
	p.stderr = stderr
	p.enc = json.NewEncoder(stdin)
	go p.readLoop(stdout)
	go p.readStderrLoop(stderr)
	return nil
}

func (p *ACPProcess) SendMessage(v any) error {
	p.encMu.Lock()
	defer p.encMu.Unlock()
	if p.enc == nil {
		return fmt.Errorf("agent acp process: encoder is not ready")
	}
	if raw, err := json.Marshal(v); err == nil {
		logACPDebugRaw('>', raw)
	}
	if err := p.enc.Encode(v); err != nil {
		return fmt.Errorf("agent acp process: encode message: %w", err)
	}
	return nil
}

func (p *ACPProcess) readLoop(r io.Reader) {
	defer p.markDone()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, protocol.ACPRPCMaxScannerBuf), protocol.ACPRPCMaxScannerBuf)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		raw := make([]byte, len(line))
		copy(raw, line)
		logACPDebugRaw('<', raw)
		p.hMu.RLock()
		h := p.onMessage
		p.hMu.RUnlock()
		if h != nil {
			h(raw)
		}
	}
}

func (p *ACPProcess) readStderrLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, protocol.ACPRPCMaxScannerBuf), protocol.ACPRPCMaxScannerBuf)
	for scanner.Scan() {
		logACPStderrLine(scanner.Text())
	}
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/hub/agent -run "TestLogOutboundACPDebugLine|TestLogACPStderrLineAsError|TestFormatACPLogLine_MinimalShape" -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/hub/agent/acp_process.go internal/hub/agent/acp_process_test.go
git commit -m "test+feat(agent): log ACP streams with minimal > < ! format"
```

---

### Task 3: Add Daily Debug Rotation (Keep 7 Days)

**Files:**
- Create: `server/internal/shared/debug_rotator.go`
- Create: `server/internal/shared/debug_rotator_test.go`
- Modify: `server/internal/shared/logger.go`

- [ ] **Step 1: Write failing tests for same-day append, day rollover, retention cleanup**

```go
package shared

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDebugRotator_RotateOnDayChange(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "hub.debug.log")

	now := time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC)
	r := newDebugDailyRotator(path, 7, func() time.Time { return now })
	defer r.Close()

	if _, err := r.Write([]byte("day1\n")); err != nil {
		t.Fatalf("write day1: %v", err)
	}
	now = now.Add(24 * time.Hour)
	if _, err := r.Write([]byte("day2\n")); err != nil {
		t.Fatalf("write day2: %v", err)
	}

	archived := filepath.Join(base, "hub.debug.2026-04-08.log")
	if _, err := os.Stat(archived); err != nil {
		t.Fatalf("expected archived file %s: %v", archived, err)
	}
}

func TestDebugRotator_KeepSevenDays(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "hub.debug.log")
	now := time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC)
	r := newDebugDailyRotator(path, 7, func() time.Time { return now })
	defer r.Close()

	for i := 1; i <= 10; i++ {
		d := now.AddDate(0, 0, -i)
		p := filepath.Join(base, "hub.debug."+d.Format("2006-01-02")+".log")
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write archive: %v", err)
		}
	}
	if err := r.cleanupOldArchives(); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	for i := 8; i <= 10; i++ {
		d := now.AddDate(0, 0, -i)
		p := filepath.Join(base, "hub.debug."+d.Format("2006-01-02")+".log")
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("expected removed: %s", p)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/shared -run "TestDebugRotator_RotateOnDayChange|TestDebugRotator_KeepSevenDays" -count=1`
Expected: FAIL (undefined `newDebugDailyRotator`).

- [ ] **Step 3: Implement rotator and wire logger setup**

```go
// debug_rotator.go
package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type debugDailyRotator struct {
	mu        sync.Mutex
	basePath  string
	keepDays  int
	now       func() time.Time
	file      *os.File
	dayString string
}

func newDebugDailyRotator(path string, keepDays int, now func() time.Time) *debugDailyRotator {
	if now == nil {
		now = time.Now
	}
	if keepDays <= 0 {
		keepDays = 7
	}
	return &debugDailyRotator{basePath: path, keepDays: keepDays, now: now}
}

func (r *debugDailyRotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.rotateIfNeededLocked(); err != nil {
		return 0, err
	}
	return r.file.Write(p)
}

func (r *debugDailyRotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file != nil {
		err := r.file.Close()
		r.file = nil
		return err
	}
	return nil
}

func (r *debugDailyRotator) rotateIfNeededLocked() error {
	day := r.now().Format("2006-01-02")
	if r.file != nil && r.dayString == day {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.basePath), 0o755); err != nil {
		return err
	}
	if r.file != nil {
		_ = r.file.Close()
		archive := r.archivePath(r.dayString)
		_ = os.Remove(archive)
		if err := os.Rename(r.basePath, archive); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	f, err := os.OpenFile(r.basePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	r.file = f
	r.dayString = day
	return r.cleanupOldArchives()
}

func (r *debugDailyRotator) archivePath(day string) string {
	ext := filepath.Ext(r.basePath)
	base := strings.TrimSuffix(r.basePath, ext)
	return fmt.Sprintf("%s.%s%s", base, day, ext)
}

func (r *debugDailyRotator) cleanupOldArchives() error {
	if r.keepDays <= 0 {
		return nil
	}
	ext := filepath.Ext(r.basePath)
	base := strings.TrimSuffix(filepath.Base(r.basePath), ext)
	glob := filepath.Join(filepath.Dir(r.basePath), base+".*"+ext)
	matches, err := filepath.Glob(glob)
	if err != nil {
		return err
	}
	cutoff := r.now().AddDate(0, 0, -r.keepDays)
	for _, m := range matches {
		name := filepath.Base(m)
		prefix := base + "."
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
			continue
		}
		day := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ext)
		d, err := time.Parse("2006-01-02", day)
		if err != nil {
			continue
		}
		if d.Before(cutoff) {
			_ = os.Remove(m)
		}
	}
	return nil
}
```

```go
// logger.go setup branch (debug path)
rotator := newDebugDailyRotator(dbgPath, 7, time.Now)
l.debugOut = rotator
l.debugFile = nil
```

```go
// logger.go close branch
if c, ok := l.debugOut.(io.Closer); ok {
	_ = c.Close()
}
l.debugOut = nil
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/shared -run "TestDebugRotator_RotateOnDayChange|TestDebugRotator_KeepSevenDays" -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/shared/debug_rotator.go internal/shared/debug_rotator_test.go internal/shared/logger.go
git commit -m "test+feat(shared): rotate debug logs daily and keep 7 days"
```

---

### Task 4: Add Prompt Wait Observability and Timeout IM ERROR Reporting

**Files:**
- Create: `server/internal/hub/client/session_observe.go`
- Create: `server/internal/hub/client/session_observe_test.go`
- Modify: `server/internal/hub/client/session.go`

- [ ] **Step 1: Write failing tests for threshold transitions and timeout report cooldown**

```go
package client

import (
	"testing"
	"time"
)

func TestPromptObserve_FirstWaitTransitions(t *testing.T) {
	st := newPromptObserveState(time.Unix(0, 0))
	e := st.Eval(time.Unix(60, 0), false)
	if !e.WarnFirstWait || e.ErrorFirstWait {
		t.Fatalf("unexpected first wait events: %+v", e)
	}
	e = st.Eval(time.Unix(180, 0), false)
	if !e.ErrorFirstWait {
		t.Fatalf("expected first wait error at 180s: %+v", e)
	}
}

func TestPromptObserve_SilenceTransitions(t *testing.T) {
	st := newPromptObserveState(time.Unix(0, 0))
	st.MarkActivity(time.Unix(5, 0), true)
	e := st.Eval(time.Unix(65, 0), true)
	if !e.WarnSilence || e.ErrorSilence {
		t.Fatalf("unexpected silence events: %+v", e)
	}
	e = st.Eval(time.Unix(185, 0), true)
	if !e.ErrorSilence {
		t.Fatalf("expected silence error at 180s: %+v", e)
	}
}

func TestTimeoutNotifyLimiter_Cooldown(t *testing.T) {
	n := newTimeoutNotifyLimiter(60 * time.Second)
	now := time.Unix(100, 0)
	if !n.Allow("sess-1:first-wait", now) {
		t.Fatal("first report should be allowed")
	}
	if n.Allow("sess-1:first-wait", now.Add(30*time.Second)) {
		t.Fatal("report inside cooldown should be blocked")
	}
	if !n.Allow("sess-1:first-wait", now.Add(61*time.Second)) {
		t.Fatal("report after cooldown should be allowed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hub/client -run "TestPromptObserve_FirstWaitTransitions|TestPromptObserve_SilenceTransitions|TestTimeoutNotifyLimiter_Cooldown" -count=1`
Expected: FAIL (undefined observe helpers).

- [ ] **Step 3: Implement observe helpers and integrate `handlePrompt` timer select-loop**

```go
// session_observe.go
package client

import "time"

var (
	promptWarnAfter      = 60 * time.Second
	promptErrorAfter     = 180 * time.Second
	promptObserveTick    = 1 * time.Second
	timeoutNotifyCooldown = 60 * time.Second
)

type promptObserveEvents struct {
	WarnFirstWait  bool
	ErrorFirstWait bool
	WarnSilence    bool
	ErrorSilence   bool
}

type promptObserveState struct {
	startAt          time.Time
	lastActivityAt   time.Time
	sawFirstText     bool
	warnedFirstWait  bool
	erroredFirstWait bool
	warnedSilence    bool
	erroredSilence   bool
}

func newPromptObserveState(now time.Time) *promptObserveState {
	return &promptObserveState{startAt: now, lastActivityAt: now}
}

func (s *promptObserveState) MarkActivity(now time.Time, hasText bool) {
	s.lastActivityAt = now
	if hasText {
		s.sawFirstText = true
		s.warnedSilence = false
		s.erroredSilence = false
	}
}

func (s *promptObserveState) Eval(now time.Time, streamStarted bool) promptObserveEvents {
	e := promptObserveEvents{}
	if !streamStarted {
		elapsed := now.Sub(s.startAt)
		if elapsed >= promptWarnAfter && !s.warnedFirstWait {
			s.warnedFirstWait = true
			e.WarnFirstWait = true
		}
		if elapsed >= promptErrorAfter && !s.erroredFirstWait {
			s.erroredFirstWait = true
			e.ErrorFirstWait = true
		}
		return e
	}
	idle := now.Sub(s.lastActivityAt)
	if idle >= promptWarnAfter && !s.warnedSilence {
		s.warnedSilence = true
		e.WarnSilence = true
	}
	if idle >= promptErrorAfter && !s.erroredSilence {
		s.erroredSilence = true
		e.ErrorSilence = true
	}
	return e
}

type timeoutNotifyLimiter struct {
	cooldown time.Duration
	lastByKey map[string]time.Time
}

func newTimeoutNotifyLimiter(cooldown time.Duration) *timeoutNotifyLimiter {
	return &timeoutNotifyLimiter{cooldown: cooldown, lastByKey: map[string]time.Time{}}
}

func (l *timeoutNotifyLimiter) Allow(key string, now time.Time) bool {
	last, ok := l.lastByKey[key]
	if ok && now.Sub(last) < l.cooldown {
		return false
	}
	l.lastByKey[key] = now
	return true
}
```

```go
// session.go additions inside Session struct
timeoutLimiter *timeoutNotifyLimiter
```

```go
// newSession initialization
timeoutLimiter: newTimeoutNotifyLimiter(timeoutNotifyCooldown),
```

```go
// session.go handlePrompt loop
obs := newPromptObserveState(time.Now())
watch := time.NewTicker(promptObserveTick)
defer watch.Stop()

for {
	select {
	case u, ok := <-updates:
		if !ok {
			goto done
		}
		hasText := u.Type == acp.UpdateText && strings.TrimSpace(u.Content) != ""
		obs.MarkActivity(time.Now(), hasText)
		// keep current update processing and retry behavior unchanged here
	case <-watch.C:
		events := obs.Eval(time.Now(), obs.sawFirstText)
		if events.WarnFirstWait {
			logger.Warn("client: prompt wait warn stage=first_wait session=%s", s.ID)
		}
		if events.ErrorFirstWait {
			s.reportTimeoutError("first_wait")
		}
		if events.WarnSilence {
			logger.Warn("client: prompt wait warn stage=silence session=%s", s.ID)
		}
		if events.ErrorSilence {
			s.reportTimeoutError("silence")
		}
	}
}
```

```go
// session.go IM report helper
func (s *Session) reportTimeoutError(kind string) {
	now := time.Now()
	s.mu.Lock()
	agent := s.currentAgentNameLocked()
	sid := s.acpSessionID
	allow := true
	if s.timeoutLimiter != nil {
		allow = s.timeoutLimiter.Allow(s.ID+":"+kind, now)
	}
	s.mu.Unlock()

	logger.Error("client: prompt wait error stage=%s agent=%s session=%s", kind, agent, sid)
	if !allow {
		return
	}
	router, source, ok := s.imContext()
	if !ok {
		return
	}
	msg := "category=timeout stage=" + kind + " agent=" + renderUnknown(agent) + " sessionID=" + renderUnknown(sid) + " action=/status then retry"
	_ = router.SystemNotify(context.Background(), im.SendTarget{SessionID: s.ID, Source: &source}, im.SystemPayload{Kind: "message", Body: msg})
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/hub/client -run "TestPromptObserve_FirstWaitTransitions|TestPromptObserve_SilenceTransitions|TestTimeoutNotifyLimiter_Cooldown" -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/hub/client/session_observe.go internal/hub/client/session_observe_test.go internal/hub/client/session.go
git commit -m "test+feat(client): add prompt wait observability and timeout reporting"
```

---

### Task 5: Expand Runtime/Startup Exception Reporting (No Behavior Change)

**Files:**
- Modify: `server/internal/hub/client/permission.go`
- Modify: `server/internal/hub/client/client.go`
- Modify: `server/internal/hub/hub.go`
- Modify: `server/cmd/wheelmaker/main.go`
- Test: `server/internal/hub/hub_test.go`

- [ ] **Step 1: Write failing tests for startup lifecycle log points and permission publish error logging**

```go
package hub

import (
	"bytes"
	"context"
	"strings"
	"testing"

	shared "github.com/swm8023/wheelmaker/internal/shared"
)

func TestBuildClient_UnsupportedTypeLogsError(t *testing.T) {
	var buf bytes.Buffer
	if err := shared.Setup(shared.LoggerConfig{Level: shared.LevelInfo}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer shared.Close()
	shared.SetOutput(&buf)

	h := New(&shared.AppConfig{}, t.TempDir()+"/db/client.sqlite3")
	_, _ = h.buildClient(context.Background(), shared.ProjectConfig{Name: "p", IM: shared.IMConfig{Type: "console"}})
	if !strings.Contains(buf.String(), "hub: build client failed") {
		t.Fatalf("missing startup error log: %s", buf.String())
	}
}
```

```go
package client

import (
	"bytes"
	"context"
	"errors"
	"testing"

	shared "github.com/swm8023/wheelmaker/internal/shared"
	"github.com/swm8023/wheelmaker/internal/im"
	acp "github.com/swm8023/wheelmaker/internal/protocol"
)

type failingPermRouter struct{}

func (f *failingPermRouter) Bind(context.Context, im.ChatRef, string, im.BindOptions) error { return nil }
func (f *failingPermRouter) PublishSessionUpdate(context.Context, im.SendTarget, acp.SessionUpdateParams) error {
	return nil
}
func (f *failingPermRouter) PublishPromptResult(context.Context, im.SendTarget, acp.SessionPromptResult) error {
	return nil
}
func (f *failingPermRouter) PublishPermissionRequest(context.Context, im.SendTarget, int64, acp.PermissionRequestParams) error {
	return errors.New("publish fail")
}
func (f *failingPermRouter) SystemNotify(context.Context, im.SendTarget, im.SystemPayload) error { return nil }
func (f *failingPermRouter) Run(context.Context) error { return nil }

func TestPermissionRouter_PublishFailureLogged(t *testing.T) {
	var buf bytes.Buffer
	if err := shared.Setup(shared.LoggerConfig{Level: shared.LevelWarn}); err != nil {
		t.Fatalf("setup logger: %v", err)
	}
	defer shared.Close()
	shared.SetOutput(&buf)

	s := newSession("sess-1", "/tmp")
	s.imRouter = &failingPermRouter{}
	s.setIMSource(im.ChatRef{ChannelID: "app", ChatID: "chat-1"})
	r := newPermissionRouter(s)
	_, _ = r.decide(context.Background(), 1, acp.PermissionRequestParams{}, "")
	if got := buf.String(); got == "" {
		t.Fatal("expected warn/error log for permission publish failure")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/hub ./internal/hub/client -run "TestBuildClient_UnsupportedTypeLogsError|TestPermissionRouter_PublishFailureLogged" -count=1`
Expected: FAIL (missing expected log content).

- [ ] **Step 3: Implement lifecycle/error logs**

```go
// hub.go Start with explicit lifecycle logs
func (h *Hub) Start(ctx context.Context) error {
	shared.Info("hub: start projects=%d", len(h.cfg.Projects))
	for _, pc := range h.cfg.Projects {
		shared.Info("hub: build client project=%s im=%s", pc.Name, pc.IM.Type)
		c, err := h.buildClient(ctx, pc)
		if err != nil {
			shared.Error("hub: build client failed project=%s err=%v", pc.Name, err)
			return fmt.Errorf("hub: project %q: %w", pc.Name, err)
		}
		h.clients = append(h.clients, c)
		shared.Info("hub: client ready project=%s", pc.Name)
	}
	h.setupRegistrySync()
	return nil
}
```

```go
// main.go runHubWorker checkpoints
if err := shared.Setup(shared.LoggerConfig{
	Level:        shared.ParseLevel(cfg.Log.Level),
	LogFile:      hubLog,
	DebugLogFile: hubDebugLog,
}); err != nil {
	return fmt.Errorf("logger setup: %w", err)
}
defer shared.Close()

shared.Info("wheelmaker: hub worker start cfg=%s db=%s", cfgPath, dbPath)
h := hub.New(cfg, dbPath)
if err := h.Start(ctx); err != nil {
	shared.Error("wheelmaker: hub start failed err=%v", err)
	return err
}
shared.Info("wheelmaker: hub started")
if err := h.Run(ctx); err != nil {
	shared.Error("wheelmaker: hub run failed err=%v", err)
	return err
}
shared.Info("wheelmaker: hub run exited")
return nil
```

```go
// permission.go
if err := router.PublishPermissionRequest(
	ctx,
	im.SendTarget{SessionID: r.session.ID, Source: &source},
	requestID,
	params,
); err != nil {
	logger.Error("client: permission publish failed session=%s request=%d err=%v", r.session.ID, requestID, err)
	return acp.PermissionResult{Outcome: "cancelled"}, nil
}
select {
case <-ctx.Done():
	logger.Warn("client: permission request timeout/cancelled session=%s request=%d", r.session.ID, requestID)
	return acp.PermissionResult{Outcome: "cancelled"}, nil
case result := <-waitCh:
	if result.Outcome == "selected" && strings.TrimSpace(result.OptionID) != "" {
		return result, nil
	}
	return acp.PermissionResult{Outcome: "cancelled"}, nil
}
```

```go
// client.go persistBoundSession with explicit error log
func (c *Client) persistBoundSession(routeKey string, sess *Session) error {
	if err := sess.persistSession(context.Background()); err != nil {
		logger.Error("client: save session failed project=%s route=%s session=%s err=%v", c.projectName, routeKey, sess.ID, err)
		return fmt.Errorf("save session: %w", err)
	}
	if err := c.store.SaveRouteBinding(context.Background(), c.projectName, routeKey, sess.ID); err != nil {
		logger.Error("client: save route binding failed project=%s route=%s session=%s err=%v", c.projectName, routeKey, sess.ID, err)
		return fmt.Errorf("save route binding: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/hub ./internal/hub/client -run "TestBuildClient_UnsupportedTypeLogsError|TestPermissionRouter_PublishFailureLogged" -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/hub/client/permission.go internal/hub/client/client.go internal/hub/hub.go cmd/wheelmaker/main.go internal/hub/hub_test.go
git commit -m "test+feat(hub/client): add startup and runtime exception reporting"
```

---

### Task 6: End-to-End Verification

**Files:**
- Test: existing suites in `internal/hub/agent`, `internal/shared`, `internal/hub/client`, `internal/hub`, `cmd/wheelmaker`

- [ ] **Step 1: Run targeted reliability suites**

Run:
```bash
go test ./internal/hub/agent -count=1
go test ./internal/shared -count=1
go test ./internal/hub/client -count=1
go test ./internal/hub -count=1
go test ./cmd/wheelmaker -count=1
```
Expected: All PASS.

- [ ] **Step 2: Run full server test suite**

Run:
```bash
go test ./... -count=1
```
Expected: PASS with no new failures.

- [ ] **Step 3: Final integration commit**

```bash
git add -A
git commit -m "feat: add server observability-first logging and timeout reporting"
```

- [ ] **Step 4: Push branch**

```bash
git push origin HEAD
```
Expected: push succeeds.

- [ ] **Step 5: Deployment hook (server changed)**

```bash
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/signal_update_now.ps1 -DelaySeconds 30
```
Expected: script exits successfully and updater signal is sent.

---

## Plan Self-Review

- Spec coverage check:
  - startup detailed logs: covered in Task 5.
  - runtime exception classes: covered in Tasks 4 and 5.
  - wait thresholds 60/180 and timeout class cooldown: covered in Task 4.
  - IM ERROR-only timeout reporting: covered in Task 4 helper.
  - ACP log minimal `> < !`, redaction, truncation: covered in Tasks 1 and 2.
  - debug daily rotation, keep 7 days: covered in Task 3.

- Placeholder scan:
  - no `TODO` / `TBD` / deferred placeholders in tasks.

- Type/signature consistency:
  - helper names are consistent across tasks (`formatACPLogLine`, `newDebugDailyRotator`, `newPromptObserveState`, `reportTimeoutError`).

