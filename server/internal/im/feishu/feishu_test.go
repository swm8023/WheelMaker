package feishu

import (
	"strconv"
	"strings"
	"testing"
	"time"
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
	if len(parts) != 0 {
		t.Fatalf("expected empty chunks, got %#v", parts)
	}
}
