package registry

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
)

const (
	volcProtocolVersion = 0x1
	volcHeaderSize      = 0x1

	volcMessageFullClientRequest  = 0x1
	volcMessageAudioOnlyRequest   = 0x2
	volcMessageFullServerResponse = 0x9
	volcMessageServerAck          = 0xb
	volcMessageServerError        = 0xf

	volcFlagNoSequence      = 0x0
	volcFlagPositiveSeq     = 0x1
	volcFlagLastNoSeq       = 0x2
	volcFlagNegativeWithSeq = 0x3

	volcSerializationNone = 0x0
	volcSerializationJSON = 0x1

	volcCompressionNone = 0x0
	volcCompressionGzip = 0x1
)

type volcengineSpeechProvider struct {
	endpoint   string
	resourceID string
	dial       volcengineDialFunc
}

type volcengineDialFunc func(ctx context.Context, url string, headers http.Header) (volcengineConn, *http.Response, error)

type volcengineConn interface {
	WriteMessage(messageType int, data []byte) error
	ReadMessage() (messageType int, p []byte, err error)
	Close() error
}

type volcengineSpeechStream struct {
	ctx    context.Context
	cancel context.CancelFunc
	conn   volcengineConn
}

type volcengineParsedFrame struct {
	Text          string
	Final         bool
	HasTranscript bool
}

type volcengineError struct {
	Code    uint32
	Message string
}

func (e volcengineError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("volcengine error code %d", e.Code)
	}
	return fmt.Sprintf("volcengine error code %d: %s", e.Code, e.Message)
}

func newVolcengineSpeechProvider() speechProvider {
	return &volcengineSpeechProvider{
		endpoint:   volcengineSpeechEndpoint,
		resourceID: volcengineSpeechResourceID,
		dial: func(ctx context.Context, url string, headers http.Header) (volcengineConn, *http.Response, error) {
			return websocket.DefaultDialer.DialContext(ctx, url, headers)
		},
	}
}

func (p *volcengineSpeechProvider) Start(ctx context.Context, req speechProviderStartRequest, events speechEventSink) (speechProviderStream, error) {
	headers := http.Header{}
	headers.Set("X-Api-Key", req.APIKey)
	headers.Set("X-Api-Resource-Id", p.resourceID)
	headers.Set("X-Api-Request-Id", newSpeechRequestID())
	headers.Set("X-Api-Sequence", "-1")

	conn, resp, err := p.dial(ctx, p.endpoint, headers)
	if err != nil {
		return nil, err
	}
	if resp != nil {
		if logID := resp.Header.Get("X-Tt-Logid"); logID != "" {
			registryLogger("").Info("volcengine speech connected logid=%s", logID)
		}
	}

	fullRequest, err := buildVolcengineFullClientRequest(req.Audio)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, fullRequest); err != nil {
		_ = conn.Close()
		return nil, err
	}

	streamCtx, cancel := context.WithCancel(ctx)
	stream := &volcengineSpeechStream{
		ctx:    streamCtx,
		cancel: cancel,
		conn:   conn,
	}
	go stream.readLoop(events)
	return stream, nil
}

func (s *volcengineSpeechStream) WriteAudio(ctx context.Context, pcm []byte) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	frame, err := buildVolcengineAudioRequest(pcm, false)
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

func (s *volcengineSpeechStream) Finish(ctx context.Context) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	frame, err := buildVolcengineAudioRequest(nil, true)
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

func (s *volcengineSpeechStream) Cancel() {
	s.cancel()
	_ = s.conn.Close()
}

func (s *volcengineSpeechStream) readLoop(events speechEventSink) {
	defer s.cancel()
	for {
		messageType, frame, err := s.conn.ReadMessage()
		if err != nil {
			if s.ctx.Err() == nil {
				events.Error(codeUnavailable, "speech provider disconnected", true)
			}
			return
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		parsed, err := parseVolcengineFrame(frame)
		if err != nil {
			if s.ctx.Err() == nil {
				events.Error(codeUnavailable, err.Error(), false)
			}
			return
		}
		if parsed.HasTranscript {
			events.Transcript(parsed.Text, parsed.Final)
		}
		if parsed.Final {
			return
		}
	}
}

func buildVolcengineFullClientRequest(audio speechAudioConfig) ([]byte, error) {
	payload := map[string]any{
		"user": map[string]any{
			"uid": "wheelmaker",
		},
		"audio": map[string]any{
			"format":  audio.Format,
			"codec":   audio.Codec,
			"rate":    audio.Rate,
			"bits":    audio.Bits,
			"channel": audio.Channel,
		},
		"request": map[string]any{
			"model_name":      volcengineSpeechModelName,
			"enable_itn":      true,
			"enable_punc":     true,
			"show_utterances": true,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	compressed, err := gzipBytes(raw)
	if err != nil {
		return nil, err
	}
	frame := volcengineHeader(volcMessageFullClientRequest, volcFlagNoSequence, volcSerializationJSON, volcCompressionGzip)
	frame = appendUint32(frame, uint32(len(compressed)))
	frame = append(frame, compressed...)
	return frame, nil
}

func buildVolcengineAudioRequest(pcm []byte, final bool) ([]byte, error) {
	compressed, err := gzipBytes(pcm)
	if err != nil {
		return nil, err
	}
	flag := byte(volcFlagNoSequence)
	if final {
		flag = volcFlagLastNoSeq
	}
	frame := volcengineHeader(volcMessageAudioOnlyRequest, flag, volcSerializationNone, volcCompressionGzip)
	frame = appendUint32(frame, uint32(len(compressed)))
	frame = append(frame, compressed...)
	return frame, nil
}

func parseVolcengineFrame(frame []byte) (volcengineParsedFrame, error) {
	if len(frame) < 8 {
		return volcengineParsedFrame{}, errors.New("volcengine frame too short")
	}
	headerSize := int(frame[0]&0x0f) * 4
	if headerSize < 4 || len(frame) < headerSize+4 {
		return volcengineParsedFrame{}, errors.New("volcengine invalid header size")
	}
	messageType := frame[1] >> 4
	flags := frame[1] & 0x0f
	serialization := frame[2] >> 4
	compression := frame[2] & 0x0f
	payload := frame[headerSize:]

	switch messageType {
	case volcMessageFullServerResponse, volcMessageServerAck:
		sequence := int32(0)
		if flags == volcFlagPositiveSeq || flags == volcFlagNegativeWithSeq {
			if len(payload) < 8 {
				return volcengineParsedFrame{}, errors.New("volcengine response missing sequence or payload size")
			}
			sequence = int32(binary.BigEndian.Uint32(payload[:4]))
			payload = payload[4:]
		}
		body, err := readVolcenginePayload(payload, compression)
		if err != nil {
			return volcengineParsedFrame{}, err
		}
		if serialization != volcSerializationJSON || len(body) == 0 {
			return volcengineParsedFrame{Final: flags == volcFlagNegativeWithSeq || sequence < 0}, nil
		}
		text := extractVolcengineText(body)
		return volcengineParsedFrame{
			Text:          text,
			Final:         flags == volcFlagNegativeWithSeq || sequence < 0,
			HasTranscript: text != "",
		}, nil
	case volcMessageServerError:
		if len(payload) < 8 {
			return volcengineParsedFrame{}, errors.New("volcengine error frame too short")
		}
		code := binary.BigEndian.Uint32(payload[:4])
		body, err := readVolcenginePayload(payload[4:], compression)
		if err != nil {
			return volcengineParsedFrame{}, volcengineError{Code: code, Message: err.Error()}
		}
		return volcengineParsedFrame{}, volcengineError{Code: code, Message: extractVolcengineErrorMessage(body)}
	default:
		return volcengineParsedFrame{}, nil
	}
}

func readVolcenginePayload(payload []byte, compression byte) ([]byte, error) {
	if len(payload) < 4 {
		return nil, errors.New("volcengine payload missing size")
	}
	size := int(binary.BigEndian.Uint32(payload[:4]))
	payload = payload[4:]
	if size < 0 || size > len(payload) {
		return nil, errors.New("volcengine payload size exceeds frame")
	}
	body := payload[:size]
	if compression == volcCompressionGzip {
		return gunzip(body)
	}
	return body, nil
}

func extractVolcengineText(body []byte) string {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return ""
	}
	return extractVolcengineResultText(root["result"])
}

func extractVolcengineResultText(result any) string {
	switch typed := result.(type) {
	case map[string]any:
		if text, ok := typed["text"].(string); ok {
			return text
		}
	case []any:
		for i := len(typed) - 1; i >= 0; i-- {
			text := extractVolcengineResultText(typed[i])
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func extractVolcengineErrorMessage(body []byte) string {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return string(body)
	}
	for _, key := range []string{"message", "msg", "error"} {
		if value, ok := root[key].(string); ok {
			return value
		}
	}
	return string(body)
}

func volcengineHeader(messageType byte, flags byte, serialization byte, compression byte) []byte {
	return []byte{
		(volcProtocolVersion << 4) | volcHeaderSize,
		(messageType << 4) | flags,
		(serialization << 4) | compression,
		0x00,
	}
}

func appendUint32(out []byte, value uint32) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], value)
	return append(out, buf[:]...)
}

func gzipBytes(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzip(raw []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

func newSpeechRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "wheelmaker-speech"
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(raw[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
