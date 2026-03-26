package client

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/swm8023/wheelmaker/internal/hub/acp"
)

// managedTerminal holds a running subprocess created by terminal/create.
type managedTerminal struct {
	mu       sync.Mutex
	output   syncOutputBuffer
	cmd      *exec.Cmd
	done     chan struct{}
	exitCode *int    // set when process exits
	signal   *string // set when process is killed by signal
	limit    int     // output byte limit; 0 = no limit
}

// syncOutputBuffer serializes process output writes and snapshot reads.
type syncOutputBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncOutputBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncOutputBuffer) Snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	raw := b.buf.Bytes()
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}

// terminalManager tracks subprocesses spawned by terminal/create callbacks.
type terminalManager struct {
	mu      sync.Mutex
	terms   map[string]*managedTerminal
	counter atomic.Int64
}

func newTerminalManager() *terminalManager {
	return &terminalManager{
		terms: make(map[string]*managedTerminal),
	}
}

// Create spawns a subprocess and starts buffering its combined stdout+stderr.
func (tm *terminalManager) Create(params acp.TerminalCreateParams) (acp.TerminalCreateResult, error) {
	command := params.Command
	args := params.Args

	if runtime.GOOS == "windows" {
		// Wrap with cmd.exe /C so built-in shell commands work on Windows.
		args = append([]string{"/C", command}, args...)
		command = "cmd.exe"
	}

	cmd := exec.Command(command, args...)
	if params.CWD != "" {
		cmd.Dir = params.CWD
	}
	if len(params.Env) > 0 {
		env := os.Environ()
		// B5 fix: Env is now []EnvVariable (was map[string]string).
		for _, e := range params.Env {
			env = append(env, e.Name+"="+e.Value)
		}
		cmd.Env = env
	}

	limit := 0
	if params.OutputByteLimit != nil {
		limit = *params.OutputByteLimit
	}

	t := &managedTerminal{
		done:  make(chan struct{}),
		limit: limit,
	}
	cmd.Stdout = &t.output
	cmd.Stderr = &t.output
	t.cmd = cmd

	if err := cmd.Start(); err != nil {
		return acp.TerminalCreateResult{}, fmt.Errorf("terminal/create: start %q: %w", params.Command, err)
	}

	id := fmt.Sprintf("term-%d", tm.counter.Add(1))
	tm.mu.Lock()
	tm.terms[id] = t
	tm.mu.Unlock()

	// Wait for exit and record result.
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

// Output returns the accumulated buffered output (non-blocking).
func (tm *terminalManager) Output(terminalID string) (acp.TerminalOutputResult, error) {
	t := tm.get(terminalID)
	if t == nil {
		return acp.TerminalOutputResult{}, fmt.Errorf("terminal/output: unknown terminalId %q", terminalID)
	}

	t.mu.Lock()
	raw := t.output.Snapshot()
	truncated := false
	if t.limit > 0 && len(raw) > t.limit {
		raw = raw[len(raw)-t.limit:]
		truncated = true
	}
	output := string(raw)
	exitCode := t.exitCode
	sig := t.signal
	t.mu.Unlock()

	result := acp.TerminalOutputResult{
		Output:    output,
		Truncated: truncated,
	}
	// B6 fix: exitStatus is an object {exitCode, signal}, not a plain integer.
	if exitCode != nil || sig != nil {
		result.ExitStatus = &acp.TerminalExitStatus{
			ExitCode: exitCode,
			Signal:   sig,
		}
	}
	return result, nil
}

// WaitForExit blocks until the subprocess exits and returns its exit info.
func (tm *terminalManager) WaitForExit(terminalID string) (acp.TerminalWaitForExitResult, error) {
	t := tm.get(terminalID)
	if t == nil {
		return acp.TerminalWaitForExitResult{}, fmt.Errorf("terminal/wait_for_exit: unknown terminalId %q", terminalID)
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

// Kill sends SIGKILL to the subprocess.
func (tm *terminalManager) Kill(terminalID string) error {
	t := tm.get(terminalID)
	if t == nil {
		return fmt.Errorf("terminal/kill: unknown terminalId %q", terminalID)
	}
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	return nil
}

// Release kills the subprocess (if running) and removes it from the map.
func (tm *terminalManager) Release(terminalID string) error {
	tm.mu.Lock()
	t := tm.terms[terminalID]
	delete(tm.terms, terminalID)
	tm.mu.Unlock()

	if t != nil && t.cmd.Process != nil {
		select {
		case <-t.done:
			// already exited
		default:
			_ = t.cmd.Process.Kill()
		}
	}
	return nil
}

// KillAll kills all running terminals and clears the map.
// Called when cleaning up before replacing the connection.
func (tm *terminalManager) KillAll() {
	tm.mu.Lock()
	terms := make(map[string]*managedTerminal, len(tm.terms))
	for k, v := range tm.terms {
		terms[k] = v
	}
	tm.terms = make(map[string]*managedTerminal)
	tm.mu.Unlock()

	for _, t := range terms {
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
	}
}

// get looks up a terminal by ID (thread-safe).
func (tm *terminalManager) get(id string) *managedTerminal {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.terms[id]
}
