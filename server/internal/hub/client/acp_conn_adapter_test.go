package client

import (
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"testing"

	"github.com/swm8023/wheelmaker/internal/hub/acp"
)

func TestACPConnAdapter_SendAndNotify(t *testing.T) {
	var notifyCount atomic.Int32

	raw := acp.NewInMemoryConn(func(r io.Reader, w io.Writer) {
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
	if err := raw.Start(); err != nil {
		t.Fatalf("start in-memory conn: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })

	conn := wrapACPConn(raw)

	var out map[string]any
	if err := conn.Send(context.Background(), "initialize", map[string]any{"x": 1}, &out); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got, _ := out["ok"].(bool); !got {
		t.Fatalf("initialize result = %#v", out)
	}

	if err := conn.Notify("session/cancel", map[string]any{"sessionId": "s1"}); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if notifyCount.Load() != 1 {
		t.Fatalf("notifyCount=%d, want 1", notifyCount.Load())
	}
}
