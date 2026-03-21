package acp

import (
	"encoding/json"
	"testing"
)

func TestNormalizeNotificationParams_CurrentModeUpdate(t *testing.T) {
	in := json.RawMessage(`{
		"sessionId":"sess-1",
		"update":{"sessionUpdate":"current_mode_update","modeId":"code"}
	}`)

	out := NormalizeNotificationParams("session/update", in)

	var p SessionUpdateParams
	if err := json.Unmarshal(out, &p); err != nil {
		t.Fatalf("unmarshal normalized payload: %v", err)
	}
	if p.Update.SessionUpdate != "config_option_update" {
		t.Fatalf("sessionUpdate=%q, want config_option_update", p.Update.SessionUpdate)
	}
	if len(p.Update.ConfigOptions) != 1 {
		t.Fatalf("configOptions len=%d, want 1", len(p.Update.ConfigOptions))
	}
	if p.Update.ConfigOptions[0].ID != "mode" {
		t.Fatalf("configOptions[0].id=%q, want mode", p.Update.ConfigOptions[0].ID)
	}
	if p.Update.ConfigOptions[0].CurrentValue != "code" {
		t.Fatalf("configOptions[0].currentValue=%q, want code", p.Update.ConfigOptions[0].CurrentValue)
	}
}

func TestNormalizeNotificationParams_PassThrough(t *testing.T) {
	in := json.RawMessage(`{"sessionId":"sess-1","update":{"sessionUpdate":"agent_message_chunk"}}`)
	out := NormalizeNotificationParams("session/update", in)
	if string(out) != string(in) {
		t.Fatalf("unexpected payload rewrite: got %s want %s", out, in)
	}
}
