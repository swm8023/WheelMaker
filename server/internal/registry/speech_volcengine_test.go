package registry

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"testing"

	"github.com/gorilla/websocket"
)

func TestVolcengineFullClientRequestFrame(t *testing.T) {
	frame, err := buildVolcengineFullClientRequest(speechAudioConfig{
		Format:  "pcm",
		Codec:   "raw",
		Rate:    16000,
		Bits:    16,
		Channel: 1,
	})
	if err != nil {
		t.Fatalf("build full request: %v", err)
	}
	if got, want := frame[:4], []byte{0x11, 0x10, 0x11, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("header=%#v, want %#v", got, want)
	}
	payloadSize := int(binary.BigEndian.Uint32(frame[4:8]))
	if payloadSize != len(frame)-8 {
		t.Fatalf("payloadSize=%d, actual=%d", payloadSize, len(frame)-8)
	}
	payload := gunzipBytes(t, frame[8:])
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	audio := body["audio"].(map[string]any)
	if audio["format"] != "pcm" || audio["codec"] != "raw" || audio["rate"] != float64(16000) {
		t.Fatalf("audio payload=%#v", audio)
	}
	request := body["request"].(map[string]any)
	if request["model_name"] != "bigmodel" ||
		request["enable_itn"] != true ||
		request["enable_punc"] != true ||
		request["enable_ddc"] != true ||
		request["show_utterances"] != false ||
		request["result_type"] != "full" ||
		request["enable_nonstream"] != true {
		t.Fatalf("request payload=%#v", request)
	}
}

func TestVolcengineAudioRequestFrames(t *testing.T) {
	frame, err := buildVolcengineAudioRequest([]byte{1, 2, 3}, false)
	if err != nil {
		t.Fatalf("build audio frame: %v", err)
	}
	if got, want := frame[:4], []byte{0x11, 0x20, 0x01, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("audio header=%#v, want %#v", got, want)
	}
	if !bytes.Equal(gunzipBytes(t, frame[8:]), []byte{1, 2, 3}) {
		t.Fatalf("audio payload did not round-trip")
	}

	finalFrame, err := buildVolcengineAudioRequest(nil, true)
	if err != nil {
		t.Fatalf("build final audio frame: %v", err)
	}
	if got, want := finalFrame[:4], []byte{0x11, 0x22, 0x01, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("final audio header=%#v, want %#v", got, want)
	}
	if size := binary.BigEndian.Uint32(finalFrame[4:8]); int(size) != len(finalFrame)-8 {
		t.Fatalf("final payload size=%d actual=%d", size, len(finalFrame)-8)
	}
}

func TestParseVolcengineTranscriptFrame(t *testing.T) {
	body := gzipJSON(t, map[string]any{
		"result": map[string]any{
			"text": "你好世界",
			"utterances": []map[string]any{
				{"text": "你好", "definite": false},
				{"text": "世界", "definite": true},
			},
		},
	})
	frame := append([]byte{0x11, 0x91, 0x11, 0x00}, int32Bytes(7)...)
	frame = append(frame, uint32Bytes(uint32(len(body)))...)
	frame = append(frame, body...)

	parsed, err := parseVolcengineFrame(frame)
	if err != nil {
		t.Fatalf("parse frame: %v", err)
	}
	if parsed.Text != "你好世界" || parsed.Final {
		t.Fatalf("parsed=%#v", parsed)
	}
}

func TestParseVolcengineTranscriptFrameCombinesFullResultList(t *testing.T) {
	body := gzipJSON(t, map[string]any{
		"result": []map[string]any{
			{"text": "前面"},
			{"text": "后面"},
		},
	})
	frame := append([]byte{0x11, 0x91, 0x11, 0x00}, int32Bytes(8)...)
	frame = append(frame, uint32Bytes(uint32(len(body)))...)
	frame = append(frame, body...)

	parsed, err := parseVolcengineFrame(frame)
	if err != nil {
		t.Fatalf("parse frame: %v", err)
	}
	if parsed.Text != "前面后面" {
		t.Fatalf("parsed text=%q, want %q", parsed.Text, "前面后面")
	}
}

func TestParseVolcengineFinalAndErrorFrames(t *testing.T) {
	finalBody := gzipJSON(t, map[string]any{
		"result": map[string]any{"text": "最终"},
	})
	finalFrame := append([]byte{0x11, 0x93, 0x11, 0x00}, int32Bytes(-3)...)
	finalFrame = append(finalFrame, uint32Bytes(uint32(len(finalBody)))...)
	finalFrame = append(finalFrame, finalBody...)
	parsed, err := parseVolcengineFrame(finalFrame)
	if err != nil {
		t.Fatalf("parse final frame: %v", err)
	}
	if parsed.Text != "最终" || !parsed.Final {
		t.Fatalf("final parsed=%#v", parsed)
	}

	errorPayload := []byte(`{"message":"bad auth"}`)
	errorFrame := append([]byte{0x11, 0xf0, 0x10, 0x00}, uint32Bytes(45000001)...)
	errorFrame = append(errorFrame, uint32Bytes(uint32(len(errorPayload)))...)
	errorFrame = append(errorFrame, errorPayload...)
	_, err = parseVolcengineFrame(errorFrame)
	if err == nil {
		t.Fatal("expected error frame to return error")
	}
}

func TestVolcengineReadLoopEmitsFinalEmptyTranscript(t *testing.T) {
	finalBody := gzipJSON(t, map[string]any{})
	finalFrame := append([]byte{0x11, 0x93, 0x11, 0x00}, int32Bytes(-3)...)
	finalFrame = append(finalFrame, uint32Bytes(uint32(len(finalBody)))...)
	finalFrame = append(finalFrame, finalBody...)
	events := &recordingSpeechEvents{done: make(chan struct{})}
	stream := &volcengineSpeechStream{
		ctx:    context.Background(),
		cancel: func() {},
		conn: &scriptedVolcengineConn{
			reads: []scriptedVolcengineRead{{messageType: websocket.BinaryMessage, payload: finalFrame}},
		},
	}

	stream.readLoop(events)

	if len(events.transcripts) != 1 {
		t.Fatalf("transcript count=%d, want 1", len(events.transcripts))
	}
	if events.transcripts[0].text != "" || !events.transcripts[0].final {
		t.Fatalf("transcript=%#v, want final empty", events.transcripts[0])
	}
}

func gzipJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func gunzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	return out
}

func int32Bytes(value int32) []byte {
	var out [4]byte
	binary.BigEndian.PutUint32(out[:], uint32(value))
	return out[:]
}

func uint32Bytes(value uint32) []byte {
	var out [4]byte
	binary.BigEndian.PutUint32(out[:], value)
	return out[:]
}

type scriptedVolcengineRead struct {
	messageType int
	payload     []byte
	err         error
}

type scriptedVolcengineConn struct {
	reads []scriptedVolcengineRead
	index int
}

func (c *scriptedVolcengineConn) WriteMessage(_ int, _ []byte) error {
	return nil
}

func (c *scriptedVolcengineConn) ReadMessage() (int, []byte, error) {
	if c.index >= len(c.reads) {
		return 0, nil, io.EOF
	}
	next := c.reads[c.index]
	c.index++
	return next.messageType, next.payload, next.err
}

func (c *scriptedVolcengineConn) Close() error {
	return nil
}

type recordingSpeechEvents struct {
	transcripts []struct {
		text  string
		final bool
	}
	done chan struct{}
}

func (e *recordingSpeechEvents) Transcript(text string, final bool) {
	e.transcripts = append(e.transcripts, struct {
		text  string
		final bool
	}{text: text, final: final})
	if final {
		close(e.done)
	}
}

func (e *recordingSpeechEvents) Error(_ string, _ string, _ bool) {}
