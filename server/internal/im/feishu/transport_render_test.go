package feishu

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
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

func TestBuildTextStreamCard_NoHeader(t *testing.T) {
	card := buildTextStreamCard("hello", false)
	if _, ok := card["header"]; ok {
		t.Fatalf("acp text stream card should not have header: %+v", card)
	}
	if schema, _ := card["schema"].(string); schema != "2.0" {
		t.Fatalf("text stream card should use schema 2.0, got %q", schema)
	}
	body, ok := card["body"].(map[string]any)
	if !ok {
		t.Fatalf("body missing in card: %+v", card)
	}
	elements, ok := body["elements"].([]map[string]any)
	if !ok || len(elements) == 0 {
		t.Fatalf("body elements missing in card: %+v", card)
	}
	content, _ := elements[0]["content"].(string)
	if content != "hello" {
		t.Fatalf("content=%q, want %q", content, "hello")
	}
}

func TestSendText_FirstChunkWaitsBeforePosting(t *testing.T) {
	f := newTransport(Config{})

	if err := f.sendText("chat-1", "hello"); err != nil {
		t.Fatalf("sendText() first chunk should be buffered, got error: %v", err)
	}

	ts := f.textStreams["chat-1"]
	if ts == nil {
		t.Fatal("text stream should be created")
	}
	if got := ts.content.String(); got != "hello" {
		t.Fatalf("buffered content=%q, want %q", got, "hello")
	}
	if ts.pushedLen != 0 {
		t.Fatalf("pushedLen=%d, want 0 before first flush", ts.pushedLen)
	}
	if ts.timer == nil {
		t.Fatal("first chunk should arm a delayed flush timer")
	}
	ts.timer.Stop()
}

func TestBuildThoughtStreamCard_HasHeader(t *testing.T) {
	card := buildThoughtStreamCard("thinking", false)
	if schema, _ := card["schema"].(string); schema != "2.0" {
		t.Fatalf("streaming thought card should use schema 2.0, got %q", schema)
	}
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

func TestBuildThoughtStreamCard_CollapsedUsesPanel(t *testing.T) {
	card := buildThoughtStreamCard("deep thought", true)
	if schema, _ := card["schema"].(string); schema != "2.0" {
		t.Fatalf("collapsed thought card should use schema 2.0, got %q", schema)
	}
	// Collapsed card should not have a top-level header (content is in panel).
	if _, ok := card["header"]; ok {
		t.Fatalf("collapsed thought card should not have top-level header: %+v", card)
	}
	body, ok := card["body"].(map[string]any)
	if !ok {
		t.Fatalf("body missing in collapsed thought card: %+v", card)
	}
	elements, ok := body["elements"].([]map[string]any)
	if !ok || len(elements) == 0 {
		t.Fatalf("body elements missing in collapsed thought card: %+v", card)
	}
	panel := elements[0]
	if tag, _ := panel["tag"].(string); tag != "collapsible_panel" {
		t.Fatalf("expected collapsible_panel tag, got %q", tag)
	}
	if expanded, _ := panel["expanded"].(bool); expanded {
		t.Fatalf("collapsed thought card panel should not be expanded")
	}
	// Panel header should use the first line of content as title.
	panelHeader, ok := panel["header"].(map[string]any)
	if !ok {
		t.Fatalf("panel header missing: %+v", panel)
	}
	panelTitle, ok := panelHeader["title"].(map[string]any)
	if !ok {
		t.Fatalf("panel header title missing: %+v", panelHeader)
	}
	titleContent, _ := panelTitle["content"].(string)
	if !strings.Contains(titleContent, "deep thought") {
		t.Fatalf("panel title should contain first line of content, got %q", titleContent)
	}
	inner, ok := panel["elements"].([]map[string]any)
	if !ok || len(inner) == 0 {
		t.Fatalf("panel elements missing: %+v", panel)
	}
	content, _ := inner[0]["content"].(string)
	if !strings.Contains(content, "deep thought") {
		t.Fatalf("panel content mismatch, got %q", content)
	}
}

func TestBuildSystemStreamCard_HasEmojiHeader(t *testing.T) {
	card := buildSystemStreamCard("status ok")
	if schema, _ := card["schema"].(string); schema != "2.0" {
		t.Fatalf("system stream card should use schema 2.0, got %q", schema)
	}
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

func TestResetSystemStream(t *testing.T) {
	f := newTransport(Config{})
	f.systemStreams["chat-1"] = &textStream{messageID: "m1"}
	f.resetSystemStream("chat-1")
	if _, ok := f.systemStreams["chat-1"]; ok {
		t.Fatalf("system stream should be removed")
	}
}

func TestResetThoughtStream(t *testing.T) {
	f := newTransport(Config{})
	f.thoughtStreams["chat-1"] = &textStream{messageID: "m1"}
	f.resetThoughtStream("chat-1")
	if _, ok := f.thoughtStreams["chat-1"]; ok {
		t.Fatalf("thought stream should be removed")
	}
}

func TestShouldHandleMessage_DeduplicatesByMessageID(t *testing.T) {
	f := newTransport(Config{})
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
	f := newTransport(Config{})
	f.seenMessageID["old"] = time.Now().Add(-3 * time.Hour)
	if !f.shouldHandleMessage("old") {
		t.Fatalf("expired message id should be accepted again")
	}
}

func TestBuildTextStreamCard_NoStreamingMarker(t *testing.T) {
	card := buildTextStreamCard("hello", true)
	body, ok := card["body"].(map[string]any)
	if !ok {
		t.Fatalf("body missing in streaming card: %+v", card)
	}
	elements, ok := body["elements"].([]map[string]any)
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
	card := buildToolCallCard("chat-1", ToolCallUpdate{
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
	card := buildToolCallCard("chat-1", ToolCallUpdate{
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
		"$ go test ./...\nPASS\n\n$ rg -n tool\ninternal/im/feishu/transport_impl.go",

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
	ch := newTransport(Config{})
	ch.setLastOutbound("chat-1", "msg-1")
	if got := ch.getLastOutbound("chat-1"); got != "msg-1" {
		t.Fatalf("last outbound=%q, want %q", got, "msg-1")
	}
}

func TestWSRunExitClassification(t *testing.T) {
	if got := classifyWSRunExit(nil, nil); got != wsExitUnexpected {
		t.Fatalf("classifyWSRunExit(nil,nil)=%q, want %q", got, wsExitUnexpected)
	}
	if got := classifyWSRunExit(context.Canceled, nil); got != wsExitContextDone {
		t.Fatalf("classifyWSRunExit(context.Canceled,nil)=%q, want %q", got, wsExitContextDone)
	}
	if got := classifyWSRunExit(nil, errors.New("dial failed")); got != wsExitStartFailed {
		t.Fatalf("classifyWSRunExit(nil,err)=%q, want %q", got, wsExitStartFailed)
	}
}

func TestFinalizeWSRunError_WrapsStartError(t *testing.T) {
	startErr := errors.New("dial failed")
	err := finalizeWSRunError(nil, startErr)
	if !errors.Is(err, startErr) {
		t.Fatalf("finalizeWSRunError() should wrap startErr, got %v", err)
	}
}

func TestHandleP2MessageReceive_DropsEmptyChatID(t *testing.T) {
	f := newTransport(Config{})
	called := false
	f.OnMessage(func(Message) {
		called = true
	})

	event := &larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Message: &larkim.EventMessage{
				ChatId:      strPtr("   "),
				MessageId:   strPtr("msg-1"),
				CreateTime:  strPtr(strconv.FormatInt(time.Now().UnixMilli(), 10)),
				MessageType: strPtr("text"),
				Content:     strPtr(`{"text":"hello"}`),
			},
		},
	}

	if err := f.handleP2MessageReceive(context.Background(), event); err != nil {
		t.Fatalf("handleP2MessageReceive() error = %v", err)
	}
	if called {
		t.Fatal("handleP2MessageReceive() should drop blank chat IDs")
	}
}

func strPtr(value string) *string {
	return &value
}
