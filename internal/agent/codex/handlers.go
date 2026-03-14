package codex

// handlers.go implements the Agent→Client callback methods that codex-acp
// calls when it needs to access the file system, run terminal commands, or
// request user permission for a tool call.
//
// ACP §5.2: these are requests (they have an id) sent from the agent to us.
// We must send back a JSON-RPC response.
//
// Methods handled:
//   - session/request_permission  → auto allow_once in MVP
//   - fs/read_text_file           → os.ReadFile
//   - fs/write_text_file          → os.WriteFile
//   - terminal/create             → spawn subprocess, buffer output
//   - terminal/output             → return buffered output
//   - terminal/wait_for_exit      → block until subprocess exits
//   - terminal/kill               → kill subprocess
//   - terminal/release            → clean up resources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"github.com/swm8023/wheelmaker/internal/acp"
)

// managedTerminal holds a running subprocess created by terminal/create.
type managedTerminal struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	cmd  *exec.Cmd
	done chan struct{}
	// set when process exits
	exitCode *int
	signal   *string
	limit    int // output byte limit; 0 = no limit
}

// handleRequest is registered on the acp.Client as the RequestHandler.
// It dispatches Agent→Client requests to the appropriate implementation.
func (a *Agent) handleRequest(_ context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "session/request_permission":
		return a.handlePermission(params)
	case "fs/read_text_file":
		return a.handleFSRead(params)
	case "fs/write_text_file":
		return a.handleFSWrite(params)
	case "terminal/create":
		return a.handleTerminalCreate(params)
	case "terminal/output":
		return a.handleTerminalOutput(params)
	case "terminal/wait_for_exit":
		return a.handleTerminalWaitForExit(params)
	case "terminal/kill":
		return a.handleTerminalKill(params)
	case "terminal/release":
		return a.handleTerminalRelease(params)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

// handlePermission auto-approves tool calls with allow_once.
// MVP: always allow. Phase 2: route to Feishu for user confirmation.
func (a *Agent) handlePermission(params json.RawMessage) (any, error) {
	var p acp.PermissionRequestParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("permission: unmarshal params: %w", err)
	}

	// Find the allow_once option id.
	optionID := ""
	for _, opt := range p.Options {
		if opt.Kind == "allow_once" {
			optionID = opt.ID
			break
		}
	}
	// Fall back to first option if allow_once not present.
	if optionID == "" && len(p.Options) > 0 {
		optionID = p.Options[0].ID
	}

	return acp.PermissionResult{
		Outcome:  "selected",
		OptionID: optionID,
	}, nil
}

// handleFSRead reads a file and returns its content as a string.
func (a *Agent) handleFSRead(params json.RawMessage) (any, error) {
	var p acp.FSReadTextFileParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("fs/read: unmarshal params: %w", err)
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return nil, fmt.Errorf("fs/read: %w", err)
	}
	return acp.FSReadTextFileResult{Content: string(data)}, nil
}

// handleFSWrite writes content to a file, creating parent directories as needed.
func (a *Agent) handleFSWrite(params json.RawMessage) (any, error) {
	var p acp.FSWriteTextFileParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("fs/write: unmarshal params: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(p.Path), 0o755); err != nil {
		return nil, fmt.Errorf("fs/write: mkdir: %w", err)
	}
	if err := os.WriteFile(p.Path, []byte(p.Content), 0o644); err != nil {
		return nil, fmt.Errorf("fs/write: %w", err)
	}
	return struct{}{}, nil
}

// handleTerminalCreate spawns a subprocess and starts buffering its output.
func (a *Agent) handleTerminalCreate(params json.RawMessage) (any, error) {
	var p acp.TerminalCreateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("terminal/create: unmarshal params: %w", err)
	}

	var args []string
	if runtime.GOOS == "windows" {
		// On Windows, wrap with cmd.exe /C so built-in commands work.
		args = append([]string{"/C", p.Command}, p.Args...)
		p.Command = "cmd.exe"
	} else {
		args = p.Args
	}

	cmd := exec.Command(p.Command, args...)

	if p.CWD != "" {
		cmd.Dir = p.CWD
	}
	if len(p.Env) > 0 {
		env := os.Environ()
		for k, v := range p.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}

	limit := 0
	if p.OutputByteLimit != nil {
		limit = *p.OutputByteLimit
	}

	t := &managedTerminal{
		done:  make(chan struct{}),
		limit: limit,
	}
	cmd.Stdout = &t.buf
	cmd.Stderr = &t.buf
	t.cmd = cmd

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("terminal/create: start %q: %w", p.Command, err)
	}

	id := fmt.Sprintf("term-%d", a.termCounter.Add(1))
	a.termsMu.Lock()
	a.terminals[id] = t
	a.termsMu.Unlock()

	// Goroutine: wait for exit and record result.
	go func() {
		err := cmd.Wait()
		t.mu.Lock()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				code := exitErr.ExitCode()
				t.exitCode = &code
			} else {
				code := -1
				t.exitCode = &code
			}
		} else {
			code := 0
			t.exitCode = &code
		}
		t.mu.Unlock()
		close(t.done)
	}()

	return acp.TerminalCreateResult{TerminalID: id}, nil
}

// handleTerminalOutput returns accumulated output (non-blocking).
func (a *Agent) handleTerminalOutput(params json.RawMessage) (any, error) {
	var p acp.TerminalOutputParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("terminal/output: unmarshal params: %w", err)
	}
	t := a.getTerminal(p.TerminalID)
	if t == nil {
		return nil, fmt.Errorf("terminal/output: unknown terminalId %q", p.TerminalID)
	}

	t.mu.Lock()
	raw := t.buf.Bytes()
	truncated := false
	if t.limit > 0 && len(raw) > t.limit {
		raw = raw[len(raw)-t.limit:]
		truncated = true
	}
	output := string(raw)
	exitCode := t.exitCode
	t.mu.Unlock()

	result := acp.TerminalOutputResult{
		Output:    output,
		Truncated: truncated,
	}
	if exitCode != nil {
		result.ExitStatus = exitCode
	}
	return result, nil
}

// handleTerminalWaitForExit blocks until the subprocess exits.
func (a *Agent) handleTerminalWaitForExit(params json.RawMessage) (any, error) {
	var p acp.TerminalWaitForExitParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("terminal/wait_for_exit: unmarshal params: %w", err)
	}
	t := a.getTerminal(p.TerminalID)
	if t == nil {
		return nil, fmt.Errorf("terminal/wait_for_exit: unknown terminalId %q", p.TerminalID)
	}

	<-t.done

	t.mu.Lock()
	exitCode := t.exitCode
	sig := t.signal
	t.mu.Unlock()

	return acp.TerminalWaitForExitResult{
		ExitCode: exitCode,
		Signal:   sig,
	}, nil
}

// handleTerminalKill kills the subprocess.
func (a *Agent) handleTerminalKill(params json.RawMessage) (any, error) {
	var p acp.TerminalKillParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("terminal/kill: unmarshal params: %w", err)
	}
	t := a.getTerminal(p.TerminalID)
	if t == nil {
		return nil, fmt.Errorf("terminal/kill: unknown terminalId %q", p.TerminalID)
	}
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	return struct{}{}, nil
}

// handleTerminalRelease kills the subprocess (if running) and removes it from the map.
func (a *Agent) handleTerminalRelease(params json.RawMessage) (any, error) {
	var p acp.TerminalReleaseParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("terminal/release: unmarshal params: %w", err)
	}

	a.termsMu.Lock()
	t := a.terminals[p.TerminalID]
	delete(a.terminals, p.TerminalID)
	a.termsMu.Unlock()

	if t != nil && t.cmd.Process != nil {
		select {
		case <-t.done:
			// already exited
		default:
			_ = t.cmd.Process.Kill()
		}
	}
	return struct{}{}, nil
}

// getTerminal looks up a managed terminal by ID.
func (a *Agent) getTerminal(id string) *managedTerminal {
	a.termsMu.Lock()
	defer a.termsMu.Unlock()
	return a.terminals[id]
}

// killAllTerminals kills all running terminals (called on Close).
func (a *Agent) killAllTerminals() {
	a.termsMu.Lock()
	terms := make(map[string]*managedTerminal, len(a.terminals))
	for k, v := range a.terminals {
		terms[k] = v
	}
	a.terminals = make(map[string]*managedTerminal)
	a.termsMu.Unlock()

	for _, t := range terms {
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
	}
}

