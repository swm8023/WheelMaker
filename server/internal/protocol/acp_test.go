package protocol

import (
	"encoding/json"
	"testing"
)

func TestSessionUpdateParams_JSONParity(t *testing.T) {
	in := SessionUpdateParams{SessionID: "s1"}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out SessionUpdateParams
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.SessionID != "s1" {
		t.Fatalf("session id = %q", out.SessionID)
	}
}

func TestSessionUpdate_UsageUpdateFields(t *testing.T) {
	raw := []byte(`{
		"sessionId":"s1",
		"update":{
			"sessionUpdate":"usage_update",
			"size":258400,
			"used":183223
		}
	}`)

	var out SessionUpdateParams
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Update.SessionUpdate != SessionUpdateUsageUpdate {
		t.Fatalf("sessionUpdate=%q, want %q", out.Update.SessionUpdate, SessionUpdateUsageUpdate)
	}
	if out.Update.Size == nil || *out.Update.Size != 258400 {
		t.Fatalf("size=%v, want 258400", out.Update.Size)
	}
	if out.Update.Used == nil || *out.Update.Used != 183223 {
		t.Fatalf("used=%v, want 183223", out.Update.Used)
	}
}
