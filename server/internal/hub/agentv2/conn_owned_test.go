package agentv2

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

func TestOwnedConn_SendMatchesResponse(t *testing.T) {
	tr := newFakeOwnedTransport()
	tr.onSend = func(v any) {
		req, ok := v.(protocol.ACPRPCRequest)
		if !ok {
			return
		}
		_ = tr.emit(protocol.ACPRPCResponse{
			JSONRPC: protocol.ACPRPCVersion,
			ID:      req.ID,
			Result:  json.RawMessage(`{"ok":true}`),
		})
	}

	conn := NewOwnedConn(tr)
	t.Cleanup(func() { _ = conn.Close() })

	var out struct {
		OK bool `json:"ok"`
	}
	if err := conn.Send(context.Background(), "test/method", map[string]any{"x": 1}, &out); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !out.OK {
		t.Fatalf("result decode failed: %+v", out)
	}
}

func TestOwnedConn_IncomingRequestDispatchesAndReplies(t *testing.T) {
	tr := newFakeOwnedTransport()
	conn := NewOwnedConn(tr)
	t.Cleanup(func() { _ = conn.Close() })

	conn.OnRequest(func(_ context.Context, method string, _ json.RawMessage, noResponse bool) (any, error) {
		if noResponse {
			t.Fatalf("expected request, got notification")
		}
		if method != "session/request_permission" {
			t.Fatalf("method=%q", method)
		}
		return map[string]any{"ok": true}, nil
	})

	if err := tr.emit(protocol.ACPRPCRequest{
		JSONRPC: protocol.ACPRPCVersion,
		ID:      42,
		Method:  "session/request_permission",
		Params:  map[string]any{"sessionId": "s1"},
	}); err != nil {
		t.Fatalf("emit request: %v", err)
	}

	select {
	case sent := <-tr.sent:
		raw, err := json.Marshal(sent)
		if err != nil {
			t.Fatalf("marshal sent response: %v", err)
		}
		var resp struct {
			ID     int64                 `json:"id"`
			Result map[string]any        `json:"result"`
			Error  *protocol.ACPRPCError `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("unmarshal sent response: %v", err)
		}
		if resp.ID != 42 {
			t.Fatalf("response id=%d, want 42", resp.ID)
		}
		if resp.Error != nil {
			t.Fatalf("unexpected response error: %v", resp.Error)
		}
		if v, ok := resp.Result["ok"].(bool); !ok || !v {
			t.Fatalf("response result=%v", resp.Result)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func TestOwnedConn_NotificationDispatchesNoResponse(t *testing.T) {
	tr := newFakeOwnedTransport()
	conn := NewOwnedConn(tr)
	t.Cleanup(func() { _ = conn.Close() })

	notified := make(chan struct{}, 1)
	conn.OnRequest(func(_ context.Context, method string, _ json.RawMessage, noResponse bool) (any, error) {
		if method == protocol.MethodSessionUpdate && noResponse {
			notified <- struct{}{}
		}
		return nil, nil
	})

	if err := tr.emit(protocol.ACPRPCNotification{
		JSONRPC: protocol.ACPRPCVersion,
		Method:  protocol.MethodSessionUpdate,
		Params:  map[string]any{"sessionId": "s1"},
	}); err != nil {
		t.Fatalf("emit notification: %v", err)
	}

	select {
	case <-notified:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification dispatch")
	}
}

type fakeOwnedTransport struct {
	mu sync.RWMutex

	h      func(json.RawMessage)
	onSend func(v any)

	sent chan any
	done chan struct{}
}

func newFakeOwnedTransport() *fakeOwnedTransport {
	return &fakeOwnedTransport{
		sent: make(chan any, 16),
		done: make(chan struct{}),
	}
}

func (f *fakeOwnedTransport) SendMessage(v any) error {
	f.sent <- v
	f.mu.RLock()
	hook := f.onSend
	f.mu.RUnlock()
	if hook != nil {
		hook(v)
	}
	return nil
}

func (f *fakeOwnedTransport) OnMessage(h func(json.RawMessage)) {
	f.mu.Lock()
	f.h = h
	f.mu.Unlock()
}

func (f *fakeOwnedTransport) Done() <-chan struct{} { return f.done }

func (f *fakeOwnedTransport) Close() error {
	select {
	case <-f.done:
	default:
		close(f.done)
	}
	return nil
}

func (f *fakeOwnedTransport) emit(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f.mu.RLock()
	h := f.h
	f.mu.RUnlock()
	if h != nil {
		h(raw)
	}
	return nil
}
