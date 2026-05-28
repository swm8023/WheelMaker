package registry

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSpeechMethodsAreClientOnly(t *testing.T) {
	for _, method := range []string{
		"speech.start",
		"speech.chunk",
		"speech.finish",
		"speech.cancel",
	} {
		if !methodAllowed("client", method) {
			t.Fatalf("client should be allowed to call %s", method)
		}
		if methodAllowed("hub", method) {
			t.Fatalf("hub should not be allowed to call %s", method)
		}
		if methodAllowed("monitor", method) {
			t.Fatalf("monitor should not be allowed to call %s", method)
		}
		if !isSpeechRequestMethod(method) {
			t.Fatalf("%s should be recognized as a speech request method", method)
		}
	}
}

func TestRedactSpeechPayloadHidesSecretsAndAudio(t *testing.T) {
	start := redactSpeechPayload("speech.start", speechStartPayload{
		Provider: "volcengine",
		Model:    "doubao-streaming-asr-2.0",
		APIKey:   "secret-key",
		Audio: speechAudioConfig{
			Format:  "pcm",
			Codec:   "raw",
			Rate:    16000,
			Bits:    16,
			Channel: 1,
		},
	})
	startPayload, ok := start.(speechStartPayload)
	if !ok {
		t.Fatalf("redacted start type=%T, want speechStartPayload", start)
	}
	if startPayload.APIKey != "[redacted]" {
		t.Fatalf("apiKey=%q, want [redacted]", startPayload.APIKey)
	}
	if startPayload.Provider != "volcengine" || startPayload.Audio.Rate != 16000 {
		t.Fatalf("redaction should preserve non-secret metadata: %#v", startPayload)
	}

	chunk := redactSpeechPayload("speech.chunk", speechChunkPayload{
		StreamID: "speech-1",
		Seq:      12,
		PCM:      "AQIDBA==",
	})
	chunkPayload, ok := chunk.(speechChunkDebugPayload)
	if !ok {
		t.Fatalf("redacted chunk type=%T, want speechChunkDebugPayload", chunk)
	}
	if chunkPayload.PCM != "[base64 omitted]" {
		t.Fatalf("pcm=%q, want omitted marker", chunkPayload.PCM)
	}
	if chunkPayload.ByteCount != 4 {
		t.Fatalf("byteCount=%d, want 4", chunkPayload.ByteCount)
	}
}

func TestSpeechLifecycleUsesRegistryLocalProvider(t *testing.T) {
	provider := newFakeSpeechProvider()
	s := New(Config{})
	s.speech = newSpeechService(provider)
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "speech.start",
		Payload: map[string]any{
			"provider": "volcengine",
			"model":    "doubao-streaming-asr-2.0",
			"apiKey":   "secret-key",
			"audio": map[string]any{
				"format":  "pcm",
				"codec":   "raw",
				"rate":    16000,
				"bits":    16,
				"channel": 1,
			},
		},
	})
	startResp := mustReadEnvelope(t, client)
	if startResp.Type != "response" || startResp.Method != "speech.start" {
		t.Fatalf("speech.start response=%#v", startResp)
	}
	streamID, ok := startResp.Payload["streamId"].(string)
	if !ok || streamID == "" {
		t.Fatalf("missing streamId in response: %#v", startResp.Payload)
	}
	if provider.start.APIKey != "secret-key" || provider.start.Audio.Rate != 16000 {
		t.Fatalf("provider start=%#v", provider.start)
	}

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "speech.chunk",
		Payload: map[string]any{
			"streamId": streamID,
			"seq":      1,
			"pcm":      "AQIDBA==",
		},
	})
	chunkResp := mustReadEnvelope(t, client)
	if chunkResp.Type != "response" || chunkResp.Method != "speech.chunk" {
		t.Fatalf("speech.chunk response=%#v", chunkResp)
	}
	if !bytes.Equal(provider.stream.writes[0], []byte{1, 2, 3, 4}) {
		t.Fatalf("provider audio=%v, want [1 2 3 4]", provider.stream.writes[0])
	}

	provider.stream.events.Transcript("你好", false)
	transcript := mustReadEnvelope(t, client)
	if transcript.Type != "event" || transcript.Method != "speech.transcript" {
		t.Fatalf("transcript event=%#v", transcript)
	}
	if transcript.Payload["streamId"] != streamID || transcript.Payload["text"] != "你好" || transcript.Payload["final"] != false {
		t.Fatalf("transcript payload=%#v", transcript.Payload)
	}

	mustWriteJSON(t, client, testEnvelope{
		RequestID: 4,
		Type:      "request",
		Method:    "speech.finish",
		Payload: map[string]any{
			"streamId": streamID,
		},
	})
	finishResp := mustReadEnvelope(t, client)
	if finishResp.Type != "response" || finishResp.Method != "speech.finish" {
		t.Fatalf("speech.finish response=%#v", finishResp)
	}
	if !provider.stream.finished {
		t.Fatal("provider stream should be finished")
	}
}

func TestSpeechChunkRejectsBadBase64(t *testing.T) {
	provider := newFakeSpeechProvider()
	s := New(Config{})
	s.speech = newSpeechService(provider)
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	streamID := startFakeSpeechStream(t, client)
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "speech.chunk",
		Payload: map[string]any{
			"streamId": streamID,
			"seq":      1,
			"pcm":      "$$$",
		},
	})
	resp := mustReadEnvelope(t, client)
	if resp.Type != "error" || resp.Method != "speech.chunk" {
		t.Fatalf("speech.chunk bad base64 response=%#v", resp)
	}
	if resp.Payload["code"] != codeInvalidArgument {
		t.Fatalf("code=%#v, want %s", resp.Payload["code"], codeInvalidArgument)
	}
	if len(provider.stream.writes) != 0 {
		t.Fatalf("provider should not receive invalid audio: %#v", provider.stream.writes)
	}
}

func TestSpeechDisconnectCancelsActiveStreams(t *testing.T) {
	provider := newFakeSpeechProvider()
	s := New(Config{})
	s.speech = newSpeechService(provider)
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	connectRegistryClient(t, client)
	_ = startFakeSpeechStream(t, client)

	_ = client.Close()
	select {
	case <-provider.stream.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("speech stream was not cancelled after client disconnect")
	}
}

func startFakeSpeechStream(t *testing.T, client *websocket.Conn) string {
	t.Helper()
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 2,
		Type:      "request",
		Method:    "speech.start",
		Payload: map[string]any{
			"provider": "volcengine",
			"model":    "doubao-streaming-asr-2.0",
			"apiKey":   "secret-key",
			"audio": map[string]any{
				"format":  "pcm",
				"codec":   "raw",
				"rate":    16000,
				"bits":    16,
				"channel": 1,
			},
		},
	})
	resp := mustReadEnvelope(t, client)
	streamID, _ := resp.Payload["streamId"].(string)
	if streamID == "" {
		t.Fatalf("missing streamId in response: %#v", resp)
	}
	return streamID
}

type fakeSpeechProvider struct {
	start  speechProviderStartRequest
	stream *fakeSpeechStream
}

func newFakeSpeechProvider() *fakeSpeechProvider {
	return &fakeSpeechProvider{}
}

func (p *fakeSpeechProvider) Start(_ context.Context, req speechProviderStartRequest, events speechEventSink) (speechProviderStream, error) {
	p.start = req
	p.stream = &fakeSpeechStream{
		events:    events,
		cancelled: make(chan struct{}),
	}
	return p.stream, nil
}

type fakeSpeechStream struct {
	events    speechEventSink
	writes    [][]byte
	finished  bool
	cancelled chan struct{}
}

func (s *fakeSpeechStream) WriteAudio(_ context.Context, pcm []byte) error {
	s.writes = append(s.writes, append([]byte(nil), pcm...))
	return nil
}

func (s *fakeSpeechStream) Finish(_ context.Context) error {
	s.finished = true
	return nil
}

func (s *fakeSpeechStream) Cancel() {
	select {
	case <-s.cancelled:
	default:
		close(s.cancelled)
	}
}
