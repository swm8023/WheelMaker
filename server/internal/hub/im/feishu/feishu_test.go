package feishu

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/swm8023/wheelmaker/internal/hub/im"
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

func TestBuildTextStreamCard_NoHeader(t *testing.T) {
	card := buildTextStreamCard("hello", false)
	if _, ok := card["header"]; ok {
		t.Fatalf("acp text stream card should not have header: %+v", card)
	}
	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) == 0 {
		t.Fatalf("elements missing in card: %+v", card)
	}
	content, _ := elements[0]["content"].(string)
	if content != "hello" {
		t.Fatalf("content=%q, want %q", content, "hello")
	}
}

func TestBuildThoughtStreamCard_HasHeader(t *testing.T) {
	card := buildThoughtStreamCard("thinking", false)
	header, ok := card["header"].(map[string]any)
	if !ok {
		t.Fatalf("header missing in thought card: %+v", card)
	}
	titleMap, ok := header["title"].(map[string]any)
	if !ok {
		t.Fatalf("header title missing in thought card: %+v", card)
	}
	title, _ := titleMap["content"].(string)
	if !strings.Contains(title, "Thinking") {
		t.Fatalf("thought title mismatch, got %q", title)
	}
}

func TestBuildSystemStreamCard_HasEmojiHeader(t *testing.T) {
	card := buildSystemStreamCard("status ok")
	header, ok := card["header"].(map[string]any)
	if !ok {
		t.Fatalf("header missing in system card: %+v", card)
	}
	titleMap, ok := header["title"].(map[string]any)
	if !ok {
		t.Fatalf("header title missing in system card: %+v", card)
	}
	title, _ := titleMap["content"].(string)
	if !strings.Contains(title, "📣") {
		t.Fatalf("system title should contain emoji, got %q", title)
	}
	if !strings.Contains(title, "System Message") {
		t.Fatalf("system title mismatch, got %q", title)
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

func TestResetSystemStream(t *testing.T) {
	f := New(Config{})
	f.systemStreams["chat-1"] = &textStream{messageID: "m1"}
	f.resetSystemStream("chat-1")
	if _, ok := f.systemStreams["chat-1"]; ok {
		t.Fatalf("system stream should be removed")
	}
}

func TestResetThoughtStream(t *testing.T) {
	f := New(Config{})
	f.thoughtStreams["chat-1"] = &textStream{messageID: "m1"}
	f.resetThoughtStream("chat-1")
	if _, ok := f.thoughtStreams["chat-1"]; ok {
		t.Fatalf("thought stream should be removed")
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
	f.seenMessageID["old"] = time.Now().Add(-3 * time.Hour)
	if !f.shouldHandleMessage("old") {
		t.Fatalf("expired message id should be accepted again")
	}
}

func TestBuildTextStreamCard_NoStreamingMarker(t *testing.T) {
	card := buildTextStreamCard("hello", true)
	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) != 1 {
		t.Fatalf("elements mismatch in streaming card: %+v", card)
	}
}

func TestNormalizeStreamMarkdown_InsertsBlankLineBeforeHeader(t *testing.T) {
	content := normalizeStreamMarkdown("first line\n## Next")
	if content != "first line\n\n## Next" {
		t.Fatalf("normalized content=%q", content)
	}
}

func TestIsFeishuMessageStale(t *testing.T) {
	old := strconv.FormatInt(time.Now().Add(-16*time.Minute).UnixMilli(), 10)
	if !isFeishuMessageStale(&old) {
		t.Fatalf("expected stale message to be dropped")
	}
	fresh := strconv.FormatInt(time.Now().Add(-2*time.Minute).UnixMilli(), 10)
	if isFeishuMessageStale(&fresh) {
		t.Fatalf("fresh message should be accepted")
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
	}, nil, false)
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

func TestBuildToolCallCard_DoesNotFormatInlineBullets(t *testing.T) {
	card := buildToolCallCard("chat-1", im.ToolCallUpdate{
		ToolCallID: "call-1",
		Title:      "Run tests",
		Status:     "completed",
		RawOutput:  []byte(`"- step one - step two - step three"`),
	}, nil, false)
	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) == 0 {
		t.Fatalf("elements missing in card: %+v", card)
	}
	content, _ := elements[0]["content"].(string)
	if !strings.Contains(content, "```text\n") {
		t.Fatalf("tool card should use code block: %q", content)
	}
	if !strings.Contains(content, "- step one - step two - step three") {
		t.Fatalf("tool card output should keep original bullet text: %q", content)
	}
}

func TestSanitizeDebugStreamLine_StripsPrefixes(t *testing.T) {
	in := "[debug][codex] <-[acp] {\"jsonrpc\":\"2.0\"}"
	got := sanitizeDebugStreamLine(in)
	if got != "<-[acp] {\"jsonrpc\":\"2.0\"}" {
		t.Fatalf("sanitizeDebugStreamLine()=%q", got)
	}
}

func TestCompactStatusEmoji(t *testing.T) {
	if got := compactStatusEmoji("completed"); got != "✅" {
		t.Fatalf("completed emoji=%q", got)
	}
	if got := compactStatusEmoji("failed"); got != "❌" {
		t.Fatalf("failed emoji=%q", got)
	}
	if got := compactStatusEmoji("in_progress"); got != "⏳" {
		t.Fatalf("in_progress emoji=%q", got)
	}
}

func TestBuildCompactToolCard(t *testing.T) {
	card := buildCompactToolCard(
		[]string{"✅ go test ./...", "⏳ rg -n tool", "❌ go vet ./..."},
		"$ go test ./...\nPASS\n\n$ rg -n tool\ninternal/hub/im/feishu/feishu.go",

		false,
	)
	header, ok := card["header"].(map[string]any)
	if !ok {
		t.Fatalf("header missing in compact tool card: %+v", card)
	}
	titleMap, ok := header["title"].(map[string]any)
	if !ok {
		t.Fatalf("title missing in compact tool card: %+v", card)
	}
	title, _ := titleMap["content"].(string)
	if !strings.Contains(title, "Tools ✅ ⏳ ❌") {
		t.Fatalf("unexpected compact card title: %q", title)
	}
	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) == 0 {
		t.Fatalf("elements missing in compact tool card: %+v", card)
	}
	if len(elements) != 1 {
		t.Fatalf("expected single compact section, got: %+v", elements)
	}
	content, _ := elements[0]["content"].(string)
	if strings.Contains(content, "### Summary") || strings.Contains(content, "### Terminal") {
		t.Fatalf("compact headings should be removed: %q", content)
	}
	if strings.Contains(content, "go test ./...") && !strings.Contains(content, "$ go test ./...") {
		t.Fatalf("summary text should not be rendered above transcript: %q", content)
	}
	if !strings.Contains(content, "```text") ||
		!strings.Contains(content, "$ go test ./...") ||
		!strings.Contains(content, "PASS") {
		t.Fatalf("compact transcript mismatch: %q", content)
	}
}

func TestBuildCompactToolCard_DoesNotFormatInlineNumberedTranscript(t *testing.T) {
	card := buildCompactToolCard(
		[]string{"RUN plan"},
		"1. collect context 2. update tests 3. ship",

		false,
	)
	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) != 1 {
		t.Fatalf("elements mismatch: %+v", card)
	}
	content, _ := elements[0]["content"].(string)
	if !strings.Contains(content, "```text\n1. collect context 2. update tests 3. ship\n```") {
		t.Fatalf("compact transcript should keep original numbered text, got: %q", content)
	}
}

func TestBuildCompactToolCard_DoesNotFormatNumberedVariants(t *testing.T) {
	card := buildCompactToolCard(
		[]string{"RUN plan"},
		"1)collect context 2)update tests 3)ship",
		false,
	)
	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) != 1 {
		t.Fatalf("elements mismatch: %+v", card)
	}
	content, _ := elements[0]["content"].(string)
	if !strings.Contains(content, "```text\n1)collect context 2)update tests 3)ship\n```") {
		t.Fatalf("compact transcript should keep numbered variant with ), got: %q", content)
	}

	card2 := buildCompactToolCard(
		[]string{"RUN plan"},
		"1、准备\u30002、验证\u30003、发布",

		false,
	)
	elements2, ok := card2["elements"].([]map[string]any)
	if !ok || len(elements2) != 1 {
		t.Fatalf("elements mismatch: %+v", card2)
	}
	content2, _ := elements2[0]["content"].(string)
	if !strings.Contains(content2, "```text\n1、准备\u30002、验证\u30003、发布\n```") {
		t.Fatalf("compact transcript should keep full-width numbered variant, got: %q", content2)
	}
}

func TestBuildCompactToolCard_DoesNotFormatInlineHyphenBullets(t *testing.T) {
	card := buildCompactToolCard(
		[]string{"RUN plan"},
		"- collect context - update tests - ship",

		false,
	)
	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) != 1 {
		t.Fatalf("elements mismatch: %+v", card)
	}
	content, _ := elements[0]["content"].(string)
	if !strings.Contains(content, "```text\n- collect context - update tests - ship\n```") {
		t.Fatalf("compact transcript should keep original hyphen bullets, got: %q", content)
	}
}

func TestBuildCompactToolCard_TitleIconsLimited(t *testing.T) {
	card := buildCompactToolCard(
		[]string{
			"✅ a", "⏳ b", "❌ c", "✅ d", "⛔ e", "⏳ f", "✅ g", "❌ h", "✅ i",
		},
		"ok",

		false,
	)
	header := card["header"].(map[string]any)
	title := header["title"].(map[string]any)["content"].(string)
	const want = "Tools ✅ ⏳ ❌ ✅ ⛔ ⏳ ✅ ❌"
	if title != want {
		t.Fatalf("title=%q, want %q", title, want)
	}
}

func TestBuildCompactToolCard_TitleFallsBackToTextOnly(t *testing.T) {
	card := buildCompactToolCard(
		[]string{"RUN go test ./..."},
		"ok",

		false,
	)
	header := card["header"].(map[string]any)
	title := header["title"].(map[string]any)["content"].(string)
	if title != "Tools" {
		t.Fatalf("title=%q, want %q", title, "Tools")
	}
}

func TestBuildCompactToolCard_NoStreamingMarker(t *testing.T) {
	card := buildCompactToolCard(
		[]string{"⏳ go test ./..."},
		"$ go test ./...",
		true,
	)
	elements, ok := card["elements"].([]map[string]any)
	if !ok || len(elements) != 1 {
		t.Fatalf("elements mismatch in compact streaming card: %+v", card)
	}
}

func TestLastOutboundState(t *testing.T) {
	ch := New(Config{})
	ch.setLastOutbound("chat-1", "msg-1")
	if got := ch.getLastOutbound("chat-1"); got != "msg-1" {
		t.Fatalf("last outbound=%q, want %q", got, "msg-1")
	}
}
