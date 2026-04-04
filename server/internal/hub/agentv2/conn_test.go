package agentv2

import (
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"testing"

	"github.com/swm8023/wheelmaker/internal/hub/acp"
)

func TestConn_SendAndNotify(t *testing.T) {
	var notifyCount atomic.Int32
	c := newFakeConn(t, &notifyCount)
	t.Cleanup(func() { _ = c.Close() })

	var out map[string]any
	if err := c.Send(context.Background(), "initialize", map[string]any{"x": 1}, &out); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got, _ := out["ok"].(bool); !got {
		t.Fatalf("initialize result = %#v", out)
	}

	if err := c.Notify("session/cancel", map[string]any{"sessionId": "s1"}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if notifyCount.Load() != 1 {
		t.Fatalf("notifyCount=%d, want 1", notifyCount.Load())
	}
}

func newFakeConn(t *testing.T, notifyCount *atomic.Int32) Conn {
	t.Helper()
	c, err := NewInMemoryConn(func(r io.Reader, w io.Writer) {
		dec := json.NewDecoder(r)
		enc := json.NewEncoder(w)
		for {
			var req map[string]any
			if err := dec.Decode(&req); err != nil {
				return
			}
			method, _ := req["method"].(string)
			id, hasID := req["id"]
			if !hasID {
				if method == "session/cancel" {
					notifyCount.Add(1)
				}
				continue
			}
			if method == "initialize" {
				_ = enc.Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"result":  map[string]any{"ok": true},
				})
				continue
			}
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result":  map[string]any{},
			})
		}
	})
	if err != nil {
		t.Fatalf("new fake conn: %v", err)
	}
	return c
}

var _ acp.InMemoryServer = func(io.Reader, io.Writer) {}
