// Package copilot implements agent.Agent for GitHub Copilot CLI via its native ACP server.
//
// GitHub Copilot CLI supports two ACP modes:
//   - stdio (default): copilot --acp --stdio
//   - tcp:             copilot --acp --port <N>
package copilot

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/swm8023/wheelmaker/internal/acp"
)

const agentName = "copilot"

// Mode selects the ACP transport for the Copilot CLI process.
type Mode string

const (
	// ModeStdio uses NDJSON over the process's stdin/stdout (recommended for IDE integration).
	ModeStdio Mode = "stdio"
	// ModeTCP starts the Copilot CLI as a TCP ACP server and connects to it.
	ModeTCP Mode = "tcp"
)

// Config holds configuration for the Copilot CLI agent.
type Config struct {
	// Mode selects the ACP transport: "stdio" (default) or "tcp".
	Mode Mode

	// Port is the TCP port used in tcp mode. 0 = auto-select a free port.
	Port int

	// ExePath is the path to the copilot binary. Empty = search PATH for "copilot".
	ExePath string

	// Env is extra environment variables passed to the subprocess.
	Env map[string]string
}

// Agent is a stateless connection factory for GitHub Copilot CLI.
// Each call to Connect() spawns a new copilot subprocess.
type Agent struct {
	cfg Config
}

// New creates an Agent with the given config.
func New(cfg Config) *Agent {
	return &Agent{cfg: cfg}
}

// Name returns the agent identifier.
func (a *Agent) Name() string { return agentName }

// Close is a no-op since Connect() transfers subprocess ownership to the Conn.
func (a *Agent) Close() error { return nil }

// Connect starts a new copilot subprocess in the configured mode and returns
// an initialized *acp.Conn.  Conn.Start() is called internally.
func (a *Agent) Connect(ctx context.Context) (*acp.Conn, error) {
	if a.cfg.Mode == ModeTCP {
		return a.connectTCP(ctx)
	}
	return a.connectStdio()
}

// connectStdio starts copilot with --acp --stdio and returns a subprocess-based Conn.
func (a *Agent) connectStdio() (*acp.Conn, error) {
	exePath, err := resolveExe(a.cfg.ExePath)
	if err != nil {
		return nil, err
	}
	env := buildEnv(a.cfg.Env)
	conn := acp.NewConn(exePath, env, "--acp", "--stdio")
	if err := conn.Start(); err != nil {
		return nil, fmt.Errorf("copilot: start stdio process: %w", err)
	}
	return conn, nil
}

// connectTCP starts copilot with --acp --port <N>, waits for the TCP server to
// be ready, and returns a transport-based Conn backed by the TCP connection.
func (a *Agent) connectTCP(ctx context.Context) (*acp.Conn, error) {
	port := a.cfg.Port
	if port == 0 {
		var err error
		port, err = freePort()
		if err != nil {
			return nil, fmt.Errorf("copilot: find free port: %w", err)
		}
	}

	exePath, err := resolveExe(a.cfg.ExePath)
	if err != nil {
		return nil, err
	}

	portStr := strconv.Itoa(port)
	cmd := exec.Command(exePath, "--acp", "--port", portStr) //nolint:gosec
	cmd.Env = append(os.Environ(), buildEnv(a.cfg.Env)...)
	cmd.Stderr = log.Writer()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("copilot: start tcp process: %w", err)
	}

	// Retry connecting until the TCP server is ready or the deadline passes.
	addr := "127.0.0.1:" + portStr
	tcpConn, err := dialWithRetry(ctx, addr, 10*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("copilot: connect to tcp server at %s: %w", addr, err)
	}

	conn := acp.NewConnWithTransport(cmd, tcpConn)
	if err := conn.Start(); err != nil {
		_ = tcpConn.Close()
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("copilot: start transport conn: %w", err)
	}
	return conn, nil
}

// dialWithRetry attempts to TCP-connect to addr, retrying every 200 ms until
// timeout elapses or ctx is cancelled.
func dialWithRetry(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout after %s", timeout)
		}
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			return c, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// resolveExe returns the path to the copilot binary.
// If configPath is non-empty it is used directly; otherwise PATH is searched.
func resolveExe(configPath string) (string, error) {
	if configPath != "" {
		return configPath, nil
	}
	exePath, err := exec.LookPath("copilot")
	if err != nil {
		return "", fmt.Errorf("copilot: binary not found in PATH (install GitHub Copilot CLI): %w", err)
	}
	return exePath, nil
}

// buildEnv converts a map of environment variables to "KEY=VALUE" strings.
func buildEnv(m map[string]string) []string {
	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, k+"="+v)
	}
	return env
}

// freePort asks the OS for a free TCP port on localhost.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port, nil
}
