package feishu

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	acp "github.com/swm8023/wheelmaker/internal/protocol"
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

func TestParseMessageImageKey(t *testing.T) {
	content := `{"image_key":"img_123"}`
	if got := parseMessageImageKey(&content); got != "img_123" {
		t.Fatalf("parseMessageImageKey()=%q, want %q", got, "img_123")
	}
}

func TestParseMessagePromptBlocks_Image(t *testing.T) {
	f := newTransport(Config{})
	f.imageFetcher = func(context.Context, string, string) (acp.ContentBlock, error) {
		return acp.ContentBlock{Type: acp.ContentBlockTypeImage, MimeType: "image/png", Data: "aGVsbG8="}, nil
	}
	msgType := "image"
	content := `{"image_key":"img_123"}`
	blocks := f.parseMessagePromptBlocks(context.Background(), "msg-1", &msgType, &content)
	if len(blocks) != 1 {
		t.Fatalf("blocks=%+v, want one image block", blocks)
	}
	if blocks[0].Type != acp.ContentBlockTypeImage {
		t.Fatalf("block type=%q, want image", blocks[0].Type)
	}
}
func TestBuildUnifiedStreamCard_TextOnly(t *testing.T) {
	seg := streamSegment{kind: segText, content: "hello"}
	card := buildUnifiedStreamCard([]streamSegment{seg}, false)
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	if _, ok := card["header"]; ok {
		t.Fatalf("unified text-only card should not have header: %+v", card)
	}
	if schema, _ := card["schema"].(string); schema != "2.0" {
		t.Fatalf("unified card should use schema 2.0, got %q", schema)
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

	us := f.unifiedStreams["chat-1"]
	if us == nil {
		t.Fatal("unified stream should be created")
	}
	if len(us.segments) != 1 || us.segments[0].kind != segText {
		t.Fatalf("expected one text segment, got %d segments", len(us.segments))
	}
	if got := us.segments[0].content; got != "hello" {
		t.Fatalf("buffered content=%q, want %q", got, "hello")
	}
	if us.pushedRunes != 0 {
		t.Fatalf("pushedRunes=%d, want 0 before first flush", us.pushedRunes)
	}
	if us.timer == nil {
		t.Fatal("first chunk should arm a delayed flush timer")
	}
	us.timer.Stop()
}

func TestSendText_SecondChunkDoesNotPanic(t *testing.T) {
	f := newTransport(Config{})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("sendText second chunk panicked: %v", r)
		}
	}()

	if err := f.sendText("chat-1", "我"); err != nil {
		t.Fatalf("sendText() first chunk error: %v", err)
	}
	if err := f.sendText("chat-1", "好"); err != nil {
		t.Fatalf("sendText() second chunk error: %v", err)
	}

	us := f.unifiedStreams["chat-1"]
	if us == nil {
		t.Fatal("unified stream should exist")
	}
	if len(us.segments) != 1 {
		t.Fatalf("expected one merged segment, got %d", len(us.segments))
	}
	if got := us.segments[0].content; got != "我好" {
		t.Fatalf("merged content=%q, want %q", got, "我好")
	}
	if us.timer != nil {
		us.timer.Stop()
	}
}

func TestBuildUnifiedStreamCard_ThoughtRenderedAsMarkdown(t *testing.T) {
	// Streaming thought (done=false, last segment) → rendered as markdown
	seg := streamSegment{kind: segThought, content: "thinking"}
	card := buildUnifiedStreamCard([]streamSegment{seg}, false)
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	if schema, _ := card["schema"].(string); schema != "2.0" {
		t.Fatalf("unified thought card should use schema 2.0, got %q", schema)
	}
	body, ok := card["body"].(map[string]any)
	if !ok {
		t.Fatalf("body missing in thought card: %+v", card)
	}
	elements, ok := body["elements"].([]map[string]any)
	if !ok || len(elements) == 0 {
		t.Fatalf("body elements missing in thought card: %+v", card)
	}
	first := elements[0]
	if tag, _ := first["tag"].(string); tag != "markdown" {
		t.Fatalf("expected markdown tag, got %q", tag)
	}
	content, _ := first["content"].(string)
	if !strings.Contains(content, "thinking") {
		t.Fatalf("thought content mismatch, got %q", content)
	}
	if !strings.Contains(content, "Thinking") {
		t.Fatalf("thought heading mismatch, got %q", content)
	}
}

func TestBuildUnifiedStreamCard_ThoughtDoneCollapsible(t *testing.T) {
	// Completed thought (done=true) → rendered as collapsible_panel
	seg := streamSegment{kind: segThought, content: "deep thought"}
	card := buildUnifiedStreamCard([]streamSegment{seg}, true)
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	if schema, _ := card["schema"].(string); schema != "2.0" {
		t.Fatalf("collapsed thought card should use schema 2.0, got %q", schema)
	}
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
	first := elements[0]
	if tag, _ := first["tag"].(string); tag != "collapsible_panel" {
		t.Fatalf("expected collapsible_panel tag, got %q", tag)
	}
	if expanded, _ := first["expanded"].(bool); expanded {
		t.Fatal("completed thought should be collapsed by default")
	}
	header, ok := first["header"].(map[string]any)
	if !ok {
		t.Fatalf("collapsible_panel missing header: %+v", first)
	}
	titleMap, ok := header["title"].(map[string]any)
	if !ok {
		t.Fatalf("collapsible_panel header missing title: %+v", header)
	}
	titleContent, _ := titleMap["content"].(string)
	if !strings.Contains(titleContent, "🧠") {
		t.Fatalf("thought title should contain brain emoji, got %q", titleContent)
	}
	innerElements, ok := first["elements"].([]map[string]any)
	if !ok || len(innerElements) == 0 {
		t.Fatalf("collapsible_panel missing inner elements: %+v", first)
	}
	innerContent, _ := innerElements[0]["content"].(string)
	if !strings.Contains(innerContent, "deep thought") {
		t.Fatalf("thought content mismatch, got %q", innerContent)
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

func TestResetUnifiedStream(t *testing.T) {
	f := newTransport(Config{})
	f.unifiedStreams["chat-1"] = &unifiedStream{messageID: "m1"}
	f.resetUnifiedStream("chat-1")
	if _, ok := f.unifiedStreams["chat-1"]; ok {
		t.Fatalf("unified stream should be removed")
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

func TestBuildUnifiedStreamCard_DoneCardHasSingleElement(t *testing.T) {
	seg := streamSegment{kind: segText, content: "hello"}
	card := buildUnifiedStreamCard([]streamSegment{seg}, true)
	body, ok := card["body"].(map[string]any)
	if !ok {
		t.Fatalf("body missing in done card: %+v", card)
	}
	elements, ok := body["elements"].([]map[string]any)
	if !ok || len(elements) != 1 {
		t.Fatalf("elements mismatch in done card: %+v", card)
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

func TestBuildUnifiedStreamCard_MixedSegments(t *testing.T) {
	seg1 := streamSegment{kind: segText, content: "hello world"}
	seg2 := streamSegment{kind: segThought, content: "deep reasoning"}
	seg3 := streamSegment{kind: segText, content: "conclusion"}

	card := buildUnifiedStreamCard([]streamSegment{seg1, seg2, seg3}, true)
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	body, ok := card["body"].(map[string]any)
	if !ok {
		t.Fatalf("body missing: %+v", card)
	}
	elements, ok := body["elements"].([]map[string]any)
	if !ok || len(elements) != 3 {
		t.Fatalf("expected 3 elements (text+collapsible_thought+text), got %d", len(elements))
	}
	if tag, _ := elements[0]["tag"].(string); tag != "markdown" {
		t.Fatalf("first element should be markdown, got %q", tag)
	}
	if tag, _ := elements[1]["tag"].(string); tag != "collapsible_panel" {
		t.Fatalf("second element should be collapsible_panel, got %q", tag)
	}
	if tag, _ := elements[2]["tag"].(string); tag != "markdown" {
		t.Fatalf("third element should be markdown, got %q", tag)
	}
}

func TestBuildUnifiedStreamCard_DividerBetweenText(t *testing.T) {
	// ABA pattern: text + divider + text (same kind) → no visible divider,
	// just two separate markdown elements (paragraph break).
	seg1 := streamSegment{kind: segText, content: "first part"}
	seg2 := streamSegment{kind: segDivider}
	seg3 := streamSegment{kind: segText, content: "second part"}

	card := buildUnifiedStreamCard([]streamSegment{seg1, seg2, seg3}, true)
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	body := card["body"].(map[string]any)
	elements := body["elements"].([]map[string]any)
	if len(elements) != 2 {
		t.Fatalf("expected 2 elements (text+text, no divider), got %d", len(elements))
	}
	c1, _ := elements[0]["content"].(string)
	c2, _ := elements[1]["content"].(string)
	if c1 != "first part" || c2 != "second part" {
		t.Fatalf("content mismatch: %q, %q", c1, c2)
	}
}

func TestBuildUnifiedStreamCard_DividerBetweenDifferentKinds(t *testing.T) {
	// ABC pattern: text + divider + thought (different kinds) → visible divider.
	seg1 := streamSegment{kind: segText, content: "reply text"}
	seg2 := streamSegment{kind: segDivider}
	seg3 := streamSegment{kind: segThought, content: "inner thought"}

	card := buildUnifiedStreamCard([]streamSegment{seg1, seg2, seg3}, true)
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	body := card["body"].(map[string]any)
	elements := body["elements"].([]map[string]any)
	if len(elements) != 3 {
		t.Fatalf("expected 3 elements (text+divider+thought), got %d", len(elements))
	}
	if tag, _ := elements[1]["tag"].(string); tag != "markdown" {
		t.Fatalf("middle element should be markdown divider, got %q", tag)
	}
	content, _ := elements[1]["content"].(string)
	if strings.TrimSpace(content) != "---" {
		t.Fatalf("middle element divider content=%q, want %q", content, "---")
	}
}

func TestBuildUnifiedStreamCard_StripEdgeDividers(t *testing.T) {
	seg1 := streamSegment{kind: segDivider}
	seg2 := streamSegment{kind: segText, content: "content"}
	seg3 := streamSegment{kind: segDivider}

	card := buildUnifiedStreamCard([]streamSegment{seg1, seg2, seg3}, true)
	if card == nil {
		t.Fatal("expected non-nil card")
	}
	body := card["body"].(map[string]any)
	elements := body["elements"].([]map[string]any)
	if len(elements) != 1 {
		t.Fatalf("expected 1 element after stripping edge dividers, got %d", len(elements))
	}
	if tag, _ := elements[0]["tag"].(string); tag != "markdown" {
		t.Fatalf("remaining element should be markdown, got %q", tag)
	}
}

func TestInsertDivider_NoConsecutive(t *testing.T) {
	f := newTransport(Config{})
	_ = f.sendText("chat-1", "hello")
	f.insertDivider("chat-1")
	f.insertDivider("chat-1")
	f.insertDivider("chat-1")

	us := f.unifiedStreams["chat-1"]
	if us == nil {
		t.Fatal("unified stream should exist")
	}
	// Should be: text + divider (no consecutive dividers)
	if len(us.segments) != 2 {
		t.Fatalf("expected 2 segments (text+divider), got %d", len(us.segments))
	}
	if us.segments[1].kind != segDivider {
		t.Fatalf("second segment should be divider, got %v", us.segments[1].kind)
	}
	us.timer.Stop()
}

func TestSendText_AutoSplitAt10K(t *testing.T) {
	f := newTransport(Config{})

	// Write content exceeding unifiedStreamMaxRunes
	bigChunk := strings.Repeat("x", unifiedStreamMaxRunes+1)
	_ = f.sendText("chat-1", bigChunk)

	us := f.unifiedStreams["chat-1"]
	if us == nil {
		t.Fatal("unified stream should exist")
	}
	us.timer.Stop()

	// Now send another chunk; since totalRunes > 10K, it should trigger auto-split
	// (which calls pushUnifiedCardLocked and creates a new stream).
	// Because we don't have a bot, push will fail silently.
	// The key check: after sendText, a new stream is created.
	_ = f.sendText("chat-1", "more")

	usNew := f.unifiedStreams["chat-1"]
	if usNew == nil {
		t.Fatal("new unified stream should exist after auto-split")
	}
	if len(usNew.segments) != 1 {
		t.Fatalf("new stream should have 1 segment, got %d", len(usNew.segments))
	}
	if got := usNew.segments[0].content; got != "more" {
		t.Fatalf("new stream content=%q, want %q", got, "more")
	}
	usNew.timer.Stop()
}
