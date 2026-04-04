package agentv2

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/swm8023/wheelmaker/internal/protocol"
)

func TestSharedConnPool_RoutesBySessionID(t *testing.T) {
	raw := &fakeRawConn{}
	shared := NewSharedConnPool(func() (Conn, error) {
		return raw, nil
	})

	r1, err := shared.Open()
	if err != nil {
		t.Fatalf("open route1: %v", err)
	}
	r2, err := shared.Open()
	if err != nil {
		t.Fatalf("open route2: %v", err)
	}
	t.Cleanup(func() {
		_ = r1.Close()
		_ = r2.Close()
		_ = shared.Close()
	})

	count1 := 0
	count2 := 0
	r1.OnRequest(func(_ context.Context, _ string, _ json.RawMessage, _ bool) (any, error) {
		count1++
		return nil, nil
	})
	r2.OnRequest(func(_ context.Context, _ string, _ json.RawMessage, _ bool) (any, error) {
		count2++
		return nil, nil
	})

	b1, ok := r1.(sessionBinder)
	if !ok {
		t.Fatal("route1 does not support session binder")
	}
	b2, ok := r2.(sessionBinder)
	if !ok {
		t.Fatal("route2 does not support session binder")
	}
	b1.BindSessionID("sid-1")
	b2.BindSessionID("sid-2")

	params, _ := json.Marshal(map[string]any{"sessionId": "sid-2"})
	if _, err := raw.emit(protocol.MethodSessionUpdate, params, true); err != nil {
		t.Fatalf("emit sid-2: %v", err)
	}
	if count1 != 0 || count2 != 1 {
		t.Fatalf("counts after sid-2 emit: c1=%d c2=%d", count1, count2)
	}

	unknown, _ := json.Marshal(map[string]any{"sessionId": "unknown"})
	if _, err := raw.emit(protocol.MethodSessionUpdate, unknown, true); err != nil {
		t.Fatalf("emit unknown: %v", err)
	}
	if count1 != 1 || count2 != 1 {
		t.Fatalf("counts after unknown emit: c1=%d c2=%d", count1, count2)
	}
}

type fakeRawConn struct {
	h RequestHandler
}

func (f *fakeRawConn) Send(_ context.Context, _ string, _ any, _ any) error { return nil }
func (f *fakeRawConn) Notify(_ string, _ any) error                         { return nil }
func (f *fakeRawConn) OnRequest(h RequestHandler)                           { f.h = h }
func (f *fakeRawConn) Close() error                                         { return nil }

func (f *fakeRawConn) emit(method string, params []byte, noResponse bool) (any, error) {
	if f.h == nil {
		return nil, nil
	}
	return f.h(context.Background(), method, params, noResponse)
}
