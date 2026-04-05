package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

// ACPProcess is a transport-only subprocess channel for newline-delimited ACP JSON messages.
//
// It is intentionally unaware of JSON-RPC request IDs, response matching, and method dispatch.
// Those are handled by Conn implementations (for example ownedConn).
type ACPProcess struct {
	exePath string
	exeArgs []string
	env     []string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	encMu sync.Mutex
	enc   *json.Encoder

	hMu       sync.RWMutex
	onMessage func(json.RawMessage)

	doneOnce sync.Once
	done     chan struct{}
}

// NewACPProcess creates a subprocess-backed ACP transport.
func NewACPProcess(exePath string, env []string, args ...string) *ACPProcess {
	return &ACPProcess{
		exePath: exePath,
		exeArgs: append([]string(nil), args...),
		env:     env,
		done:    make(chan struct{}),
	}
}

// Start starts the subprocess transport.
func (p *ACPProcess) Start() error {
	cmd := exec.Command(p.exePath, p.exeArgs...)
	cmd.Env = append(cmd.Environ(), p.env...)
	cmd.Stderr = log.Writer()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("agent acp process: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("agent acp process: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("agent acp process: start process: %w", err)
	}

	p.cmd = cmd
	p.stdin = stdin
	p.stdout = stdout
	p.enc = json.NewEncoder(stdin)
	go p.readLoop(stdout)
	return nil
}

// SendMessage writes one JSON message to the process stdin.
func (p *ACPProcess) SendMessage(v any) error {
	p.encMu.Lock()
	defer p.encMu.Unlock()
	if p.enc == nil {
		return fmt.Errorf("agent acp process: encoder is not ready")
	}
	if err := p.enc.Encode(v); err != nil {
		return fmt.Errorf("agent acp process: encode message: %w", err)
	}
	return nil
}

// OnMessage sets the raw-message callback for process stdout frames.
func (p *ACPProcess) OnMessage(h func(json.RawMessage)) {
	p.hMu.Lock()
	p.onMessage = h
	p.hMu.Unlock()
}

// Done is closed when the process transport stops.
func (p *ACPProcess) Done() <-chan struct{} {
	if p == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return p.done
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

		p.hMu.RLock()
		h := p.onMessage
		p.hMu.RUnlock()
		if h != nil {
			h(raw)
		}
	}
}

// Close stops the transport and kills the subprocess if still running.
func (p *ACPProcess) Close() error {
	if p == nil {
		return nil
	}
	p.markDone()

	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	if p.cmd != nil {
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		_ = p.cmd.Wait()
	}
	return nil
}

func (p *ACPProcess) markDone() {
	p.doneOnce.Do(func() {
		close(p.done)
	})
}
