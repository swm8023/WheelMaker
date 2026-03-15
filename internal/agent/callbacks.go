package agent

// callbacks.go handles all Agent→Client requests from the ACP subprocess.
// These are requests (with an id) sent by the CLI binary to the client;
// the client must send a JSON-RPC response.
//
// Methods handled:
//   - session/request_permission  → auto allow_once (MVP)
//   - fs/read_text_file           → os.ReadFile
//   - fs/write_text_file          → os.WriteFile
//   - terminal/create             → spawn subprocess, buffer output
//   - terminal/output             → return buffered output
//   - terminal/wait_for_exit      → block until subprocess exits
//   - terminal/kill               → kill subprocess
//   - terminal/release            → clean up resources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/swm8023/wheelmaker/internal/agent/acp"
)

// handleCallback is registered on conn as the RequestHandler in ensureReady.
// It dispatches Agent→Client requests to the appropriate implementation.
func (a *Agent) handleCallback(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "session/request_permission":
		return a.callbackPermission(ctx, params)
	case "fs/read_text_file":
		return a.callbackFSRead(params)
	case "fs/write_text_file":
		return a.callbackFSWrite(params)
	case "terminal/create":
		return a.callbackTerminalCreate(params)
	case "terminal/output":
		return a.callbackTerminalOutput(params)
	case "terminal/wait_for_exit":
		return a.callbackTerminalWaitForExit(params)
	case "terminal/kill":
		return a.callbackTerminalKill(params)
	case "terminal/release":
		return a.callbackTerminalRelease(params)
	default:
		return nil, fmt.Errorf("unsupported method: %s", method)
	}
}

func (a *Agent) callbackPermission(ctx context.Context, params json.RawMessage) (any, error) {
	var p acp.PermissionRequestParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("permission: unmarshal params: %w", err)
	}
	a.mu.Lock()
	h := a.permission
	// FL2: use per-prompt context so Cancel() unblocks pending permission requests.
	pCtx := a.promptCtx
	a.mu.Unlock()
	if pCtx == nil {
		pCtx = ctx
	}

	result, err := h.RequestPermission(pCtx, p)
	if err != nil {
		if pCtx.Err() != nil {
			// Prompt was cancelled — respond with "cancelled" outcome as required.
			return acp.PermissionResponse{
				Outcome: acp.PermissionResult{Outcome: "cancelled"},
			}, nil
		}
		return nil, err
	}
	// B2 fix: wrap in PermissionResponse so result JSON is {"outcome":{...}}.
	return acp.PermissionResponse{Outcome: result}, nil
}

func (a *Agent) callbackFSRead(params json.RawMessage) (any, error) {
	var p acp.FSReadTextFileParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("fs/read: unmarshal params: %w", err)
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return nil, fmt.Errorf("fs/read: %w", err)
	}
	content := string(data)
	// B7 fix: apply line/limit filtering when requested (1-based line per spec).
	if p.Line != nil || p.Limit != nil {
		lines := strings.Split(content, "\n")
		start := 0
		if p.Line != nil {
			start = *p.Line - 1
			if start < 0 {
				start = 0
			}
			if start > len(lines) {
				start = len(lines)
			}
		}
		end := len(lines)
		if p.Limit != nil {
			end = start + *p.Limit
			if end > len(lines) {
				end = len(lines)
			}
		}
		content = strings.Join(lines[start:end], "\n")
	}
	return acp.FSReadTextFileResult{Content: content}, nil
}

func (a *Agent) callbackFSWrite(params json.RawMessage) (any, error) {
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

func (a *Agent) callbackTerminalCreate(params json.RawMessage) (any, error) {
	var p acp.TerminalCreateParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("terminal/create: unmarshal params: %w", err)
	}
	return a.terminals.Create(p)
}

func (a *Agent) callbackTerminalOutput(params json.RawMessage) (any, error) {
	var p acp.TerminalOutputParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("terminal/output: unmarshal params: %w", err)
	}
	return a.terminals.Output(p.TerminalID)
}

func (a *Agent) callbackTerminalWaitForExit(params json.RawMessage) (any, error) {
	var p acp.TerminalWaitForExitParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("terminal/wait_for_exit: unmarshal params: %w", err)
	}
	return a.terminals.WaitForExit(p.TerminalID)
}

func (a *Agent) callbackTerminalKill(params json.RawMessage) (any, error) {
	var p acp.TerminalKillParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("terminal/kill: unmarshal params: %w", err)
	}
	return struct{}{}, a.terminals.Kill(p.TerminalID)
}

func (a *Agent) callbackTerminalRelease(params json.RawMessage) (any, error) {
	var p acp.TerminalReleaseParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("terminal/release: unmarshal params: %w", err)
	}
	return struct{}{}, a.terminals.Release(p.TerminalID)
}
