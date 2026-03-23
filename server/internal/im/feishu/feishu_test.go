package feishu

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/im"
)

func TestParseMessageText_Text(t *testing.T) {
	mt := "text"
	content := `{"text":"hello"}`
	got := parseMessageText(&mt, &content)
	if got != "hello" {
		t.Fatalf("parseMessageText()=%q, want %q", got, "hello")
	}
}

func TestParseMessageText_NotText(t *testing.T) {
	mt := "image"
	content := `{"image_key":"img_xxx"}`
	got := parseMessageText(&mt, &content)
	if got != "" {
		t.Fatalf("parseMessageText()=%q, want empty", got)
	}
}

func TestParseMessageText_InvalidJSON(t *testing.T) {
	mt := "text"
	content := "{"
	got := parseMessageText(&mt, &content)
	if got != "" {
		t.Fatalf("parseMessageText()=%q, want empty", got)
	}
}

func TestBuildDebugCard_ContainsLines(t *testing.T) {
	card := buildDebugCard([]string{"line-1", "line-2"})
	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) == 0 {
		t.Fatalf("elements missing in card: %+v", card)
	}
	content, _ := elements[0]["content"].(string)
	if !strings.Contains(content, "line-1") || !strings.Contains(content, "line-2") {
		t.Fatalf("debug card content missing lines: %q", content)
	}
}

func TestBuildDebugCard_TruncatesToLast120Lines(t *testing.T) {
	lines := make([]string, 0, 140)
	for i := 0; i < 140; i++ {
		lines = append(lines, "line-"+strconv.Itoa(i))
	}
	card := buildDebugCard(lines)
	elements, _ := card["elements"].([]map[string]any)
	content, _ := elements[0]["content"].(string)
	if strings.Contains(content, "line-0") {
		t.Fatalf("old lines should be truncated, got content=%q", content)
	}
	if !strings.Contains(content, "line-139") {
		t.Fatalf("latest lines should be kept, got content=%q", content)
	}
}

func TestResetDebugStream(t *testing.T) {
	f := New(Config{})
	f.debugStreams["chat-1"] = &debugStream{messageID: "m1", lines: []string{"a"}}
	f.resetDebugStream("chat-1")
	if _, ok := f.debugStreams["chat-1"]; ok {
		t.Fatalf("debug stream should be removed")
	}
}

func TestShouldHandleMessage_DeduplicatesByMessageID(t *testing.T) {
	f := New(Config{})
	if !f.shouldHandleMessage("m-1") {
		t.Fatalf("first message should pass")
	}
	if f.shouldHandleMessage("m-1") {
		t.Fatalf("duplicate message should be dropped")
	}
	if !f.shouldHandleMessage("m-2") {
		t.Fatalf("different message id should pass")
	}
}

func TestShouldHandleMessage_ExpiresTTL(t *testing.T) {
	f := New(Config{})
	f.seenMessageID["old"] = time.Now().Add(-31 * time.Minute)
	if !f.shouldHandleMessage("old") {
		t.Fatalf("expired message id should be accepted again")
	}
}

func TestSplitTextForFeishu(t *testing.T) {
	parts := splitTextForFeishu("abcdef", 2)
	if len(parts) != 3 || parts[0] != "ab" || parts[1] != "cd" || parts[2] != "ef" {
		t.Fatalf("unexpected chunks: %#v", parts)
	}
}

func TestSplitTextForFeishu_Empty(t *testing.T) {
	parts := splitTextForFeishu("   ", 2)
	if len(parts) != 2 || parts[0] != "  " || parts[1] != " " {
		t.Fatalf("expected whitespace-preserving chunks, got %#v", parts)
	}
}

func TestSplitTextForFeishu_PreservesBoundaryWhitespace(t *testing.T) {
	parts := splitTextForFeishu("a b", 2)
	if len(parts) != 2 || parts[0] != "a " || parts[1] != "b" {
		t.Fatalf("unexpected chunks: %#v", parts)
	}
}

func TestBuildToolCallCard(t *testing.T) {
	card := buildToolCallCard("chat-1", im.ToolCallUpdate{
		ToolCallID: "call-1",
		Title:      "Run tests",
		Status:     "failed",
		RawOutput:  []byte(`"permission denied"`),
	}, nil)
	header, ok := card["header"].(map[string]any)
	if !ok {
		t.Fatalf("header missing in card: %+v", card)
	}
	if got, _ := header["template"].(string); got != "red" {
		t.Fatalf("header template=%q, want red", got)
	}
	titleMap, ok := header["title"].(map[string]any)
	if !ok {
		t.Fatalf("header title missing in card: %+v", card)
	}
	title, _ := titleMap["content"].(string)
	if !strings.Contains(title, "Run tests") || !strings.Contains(title, "⚪") {
		t.Fatalf("tool header missing name/permission marker: %q", title)
	}

	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) == 0 {
		t.Fatalf("elements missing in card: %+v", card)
	}
	content, _ := elements[0]["content"].(string)
	if !strings.Contains(content, "permission denied") {
		t.Fatalf("tool card content missing output: %q", content)
	}
}

func TestSanitizeDebugStreamLine_StripsPrefixes(t *testing.T) {
	in := "[debug][codex] <-[acp] {\"jsonrpc\":\"2.0\"}"
	got := sanitizeDebugStreamLine(in)
	if got != "{\"jsonrpc\":\"2.0\"}" {
		t.Fatalf("sanitizeDebugStreamLine()=%q", got)
	}
}
