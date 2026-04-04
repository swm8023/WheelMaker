package agentv2

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

func TestProcessConn_InMemorySendAndNotification(t *testing.T) {
	server := func(r io.Reader, w io.Writer) {
		s := bufio.NewScanner(r)
		for s.Scan() {
			var req struct {
				ID     *int64          `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if err := json.Unmarshal(s.Bytes(), &req); err != nil {
				continue
			}
			if req.Method == "ping" && req.ID != nil {
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":`))
				id, _ := json.Marshal(*req.ID)
				_, _ = w.Write(id)
				_, _ = w.Write([]byte(`,"result":{"ok":true}}` + "\n"))
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","method":"notify/test","params":{"msg":"hello"}}` + "\n"))
			}
		}
	}

	conn := NewInMemoryProcessConn(server)
	if err := conn.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var mu sync.Mutex
	notified := false
	conn.OnRequest(func(_ context.Context, method string, params json.RawMessage, noResponse bool) (any, error) {
		if noResponse && method == "notify/test" {
			mu.Lock()
			notified = true
			mu.Unlock()
		}
		return nil, nil
	})

	var out struct {
		OK bool `json:"ok"`
	}
	if err := conn.Send(context.Background(), "ping", map[string]any{"x": 1}, &out); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !out.OK {
		t.Fatal("expected OK=true")
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := notified
		mu.Unlock()
		if ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("expected notification callback")
}
