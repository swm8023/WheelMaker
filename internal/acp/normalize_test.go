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

	out := NormalizeNotificationParams(MethodSessionUpdate, in)

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
	out := NormalizeNotificationParams(MethodSessionUpdate, in)
	if string(out) != string(in) {
		t.Fatalf("unexpected payload rewrite: got %s want %s", out, in)
	}
}

func TestNormalizeNotificationParams_NonUpdateMethod(t *testing.T) {
	in := json.RawMessage(`{"foo":"bar"}`)
	out := NormalizeNotificationParams("session/cancel", in)
	if string(out) != string(in) {
		t.Fatalf("non-update method should pass through: got %s", out)
	}
}

func TestNormalizeNotificationParams_EmptyParams(t *testing.T) {
	out := NormalizeNotificationParams(MethodSessionUpdate, nil)
	if out != nil {
		t.Fatalf("nil params should return nil, got %s", out)
	}
	out = NormalizeNotificationParams(MethodSessionUpdate, json.RawMessage{})
	if len(out) != 0 {
		t.Fatalf("empty params should return empty, got %s", out)
	}
}

func TestNormalizeNotificationParams_MalformedJSON(t *testing.T) {
	in := json.RawMessage(`{not valid json`)
	out := NormalizeNotificationParams(MethodSessionUpdate, in)
	if string(out) != string(in) {
		t.Fatalf("malformed JSON should pass through: got %s", out)
	}
}

func TestNormalizeNotificationParams_MissingModeID(t *testing.T) {
	// current_mode_update with empty modeId should pass through unchanged.
	in := json.RawMessage(`{
		"sessionId":"sess-1",
		"update":{"sessionUpdate":"current_mode_update","modeId":""}
	}`)
	out := NormalizeNotificationParams(MethodSessionUpdate, in)
	if string(out) != string(in) {
		t.Fatalf("empty modeId should pass through: got %s", out)
	}
}

func TestNormalizeNotificationParams_ExistingConfigOptions(t *testing.T) {
	// If configOptions already present, normalize should keep them unchanged.
	in := json.RawMessage(`{
		"sessionId":"sess-1",
		"update":{
			"sessionUpdate":"current_mode_update",
			"modeId":"code",
			"configOptions":[{"id":"custom","currentValue":"x"}]
		}
	}`)
	out := NormalizeNotificationParams(MethodSessionUpdate, in)

	var p SessionUpdateParams
	if err := json.Unmarshal(out, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Update.SessionUpdate != "config_option_update" {
		t.Fatalf("sessionUpdate=%q, want config_option_update", p.Update.SessionUpdate)
	}
	// Should keep original configOptions, not overwrite.
	if len(p.Update.ConfigOptions) != 1 || p.Update.ConfigOptions[0].ID != "custom" {
		t.Fatalf("existing configOptions should be preserved, got %+v", p.Update.ConfigOptions)
	}
}

func TestNormalizeNotificationParams_NoUpdateField(t *testing.T) {
	// Params with no "update" key should pass through.
	in := json.RawMessage(`{"sessionId":"sess-1","other":"value"}`)
	out := NormalizeNotificationParams(MethodSessionUpdate, in)
	if string(out) != string(in) {
		t.Fatalf("no update field should pass through: got %s", out)
	}
}
