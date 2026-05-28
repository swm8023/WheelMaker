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

func TestSpeechStartReplacesActiveStreamForSameConnection(t *testing.T) {
	provider := newFakeSpeechProvider()
	s := New(Config{})
	s.speech = newSpeechService(provider)
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	firstStreamID := startFakeSpeechStreamWithRequestID(t, client, 2)
	firstStream := provider.stream
	secondStreamID := startFakeSpeechStreamWithRequestID(t, client, 3)

	if secondStreamID == firstStreamID {
		t.Fatalf("replacement streamID=%q, want a new stream", secondStreamID)
	}
	if provider.stream == firstStream {
		t.Fatal("provider should receive a new speech stream")
	}
	select {
	case <-firstStream.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("old speech stream was not cancelled after replacement start")
	}
}

func TestSpeechFinishReleasesActiveStreamImmediately(t *testing.T) {
	provider := newFakeSpeechProvider()
	s := New(Config{})
	s.speech = newSpeechService(provider)
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	firstStreamID := startFakeSpeechStreamWithRequestID(t, client, 2)
	firstStream := provider.stream
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "speech.finish",
		Payload: map[string]any{
			"streamId": firstStreamID,
		},
	})
	finishResp := mustReadEnvelope(t, client)
	if finishResp.Type != "response" || finishResp.Method != "speech.finish" {
		t.Fatalf("speech.finish response=%#v", finishResp)
	}
	if !firstStream.finished {
		t.Fatal("provider stream should be finished")
	}

	secondStreamID := startFakeSpeechStreamWithRequestID(t, client, 4)
	if secondStreamID == firstStreamID {
		t.Fatalf("new streamID=%q, want a new stream", secondStreamID)
	}
}

func TestSpeechFinishKeepsClosingRouteForFinalTranscript(t *testing.T) {
	provider := newFakeSpeechProvider()
	s := New(Config{})
	s.speech = newSpeechServiceWithOptions(provider, speechServiceOptions{
		idleTimeout:         time.Second,
		startTimeout:        time.Second,
		finishTimeout:       time.Second,
		closingRouteTimeout: time.Second,
	})
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	streamID := startFakeSpeechStreamWithRequestID(t, client, 2)
	stream := provider.stream
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 3,
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

	stream.events.Transcript("最终文本", true)
	finalEvent := mustReadEnvelope(t, client)
	if finalEvent.Type != "event" || finalEvent.Method != "speech.transcript" {
		t.Fatalf("final transcript event=%#v", finalEvent)
	}
	if finalEvent.Payload["streamId"] != streamID || finalEvent.Payload["text"] != "最终文本" || finalEvent.Payload["final"] != true {
		t.Fatalf("final transcript payload=%#v", finalEvent.Payload)
	}
}

func TestSpeechFinishClosingRouteForwardsInterimTranscript(t *testing.T) {
	provider := newFakeSpeechProvider()
	s := New(Config{})
	s.speech = newSpeechServiceWithOptions(provider, speechServiceOptions{
		idleTimeout:         time.Second,
		startTimeout:        time.Second,
		finishTimeout:       time.Second,
		closingRouteTimeout: time.Second,
	})
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	streamID := startFakeSpeechStreamWithRequestID(t, client, 2)
	stream := provider.stream
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "speech.finish",
		Payload: map[string]any{
			"streamId": streamID,
		},
	})
	_ = mustReadEnvelope(t, client)

	stream.events.Transcript("中间文本", false)
	_ = client.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	var interim testEnvelope
	if err := client.ReadJSON(&interim); err != nil {
		t.Fatalf("read interim transcript after finish: %v", err)
	}
	_ = client.SetReadDeadline(time.Time{})
	if interim.Type != "event" || interim.Method != "speech.transcript" {
		t.Fatalf("interim transcript event=%#v", interim)
	}
	if interim.Payload["streamId"] != streamID || interim.Payload["text"] != "中间文本" || interim.Payload["final"] != false {
		t.Fatalf("interim transcript payload=%#v", interim.Payload)
	}
}

func TestSpeechFinishClosingRouteExpires(t *testing.T) {
	provider := newFakeSpeechProvider()
	s := New(Config{})
	s.speech = newSpeechServiceWithOptions(provider, speechServiceOptions{
		idleTimeout:         time.Second,
		startTimeout:        time.Second,
		finishTimeout:       time.Second,
		closingRouteTimeout: 20 * time.Millisecond,
	})
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	streamID := startFakeSpeechStreamWithRequestID(t, client, 2)
	stream := provider.stream
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "speech.finish",
		Payload: map[string]any{
			"streamId": streamID,
		},
	})
	_ = mustReadEnvelope(t, client)

	time.Sleep(60 * time.Millisecond)
	stream.events.Transcript("迟到文本", true)
	_ = client.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	var late testEnvelope
	if err := client.ReadJSON(&late); err == nil {
		t.Fatalf("late final transcript should be ignored: %#v", late)
	}
	_ = client.SetReadDeadline(time.Time{})
}

func TestSpeechProviderErrorReleasesActiveStream(t *testing.T) {
	provider := newFakeSpeechProvider()
	s := New(Config{})
	s.speech = newSpeechService(provider)
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	streamID := startFakeSpeechStreamWithRequestID(t, client, 2)
	provider.stream.events.Error(codeUnavailable, "speech provider disconnected", true)
	errEvent := mustReadEnvelope(t, client)
	if errEvent.Type != "event" || errEvent.Method != "speech.error" {
		t.Fatalf("speech.error event=%#v", errEvent)
	}
	if errEvent.Payload["streamId"] != streamID || errEvent.Payload["retryable"] != true {
		t.Fatalf("speech.error payload=%#v", errEvent.Payload)
	}

	nextStreamID := startFakeSpeechStreamWithRequestID(t, client, 3)
	if nextStreamID == streamID {
		t.Fatalf("new streamID=%q, want a new stream", nextStreamID)
	}
}

func TestSpeechIdleTimeoutCancelsStreamAndEmitsError(t *testing.T) {
	provider := newFakeSpeechProvider()
	s := New(Config{})
	s.speech = newSpeechServiceWithOptions(provider, speechServiceOptions{
		idleTimeout:   20 * time.Millisecond,
		startTimeout:  time.Second,
		finishTimeout: time.Second,
	})
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	streamID := startFakeSpeechStreamWithRequestID(t, client, 2)
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	errEvent := mustReadEnvelope(t, client)
	if errEvent.Type != "event" || errEvent.Method != "speech.error" {
		t.Fatalf("idle timeout event=%#v", errEvent)
	}
	if errEvent.Payload["streamId"] != streamID ||
		errEvent.Payload["code"] != codeUnavailable ||
		errEvent.Payload["message"] != "speech stream idle timeout" ||
		errEvent.Payload["retryable"] != true {
		t.Fatalf("idle timeout payload=%#v", errEvent.Payload)
	}
	select {
	case <-provider.stream.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("idle timeout did not cancel provider stream")
	}
	_ = client.SetReadDeadline(time.Time{})

	nextStreamID := startFakeSpeechStreamWithRequestID(t, client, 3)
	if nextStreamID == streamID {
		t.Fatalf("new streamID=%q, want a new stream", nextStreamID)
	}
}

func TestSpeechProviderStartTimeoutReleasesActiveStream(t *testing.T) {
	provider := &timeoutFirstSpeechProvider{}
	s := New(Config{})
	s.speech = newSpeechServiceWithOptions(provider, speechServiceOptions{
		idleTimeout:   time.Second,
		startTimeout:  20 * time.Millisecond,
		finishTimeout: time.Second,
	})
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	writeSpeechStartRequest(t, client, 2)
	resp := mustReadEnvelope(t, client)
	if resp.Type != "error" || resp.Method != "speech.start" {
		t.Fatalf("speech.start timeout response=%#v", resp)
	}
	if resp.Payload["code"] != codeUnavailable {
		t.Fatalf("timeout code=%#v, want %s", resp.Payload["code"], codeUnavailable)
	}

	nextStreamID := startFakeSpeechStreamWithRequestID(t, client, 3)
	if nextStreamID == "" {
		t.Fatal("expected a new stream after start timeout")
	}
}

func TestSpeechProviderFinishTimeoutDoesNotRestoreActiveStream(t *testing.T) {
	provider := &blockingFinishSpeechProvider{}
	s := New(Config{})
	s.speech = newSpeechServiceWithOptions(provider, speechServiceOptions{
		idleTimeout:   time.Second,
		startTimeout:  time.Second,
		finishTimeout: 20 * time.Millisecond,
	})
	ts := httptestNewRegistryServer(t, s.Handler())

	client := dialWS(t, "http://"+ts+"/ws")
	defer client.Close()
	connectRegistryClient(t, client)

	streamID := startFakeSpeechStreamWithRequestID(t, client, 2)
	mustWriteJSON(t, client, testEnvelope{
		RequestID: 3,
		Type:      "request",
		Method:    "speech.finish",
		Payload: map[string]any{
			"streamId": streamID,
		},
	})
	resp := mustReadEnvelope(t, client)
	if resp.Type != "error" || resp.Method != "speech.finish" {
		t.Fatalf("speech.finish timeout response=%#v", resp)
	}
	if resp.Payload["code"] != codeUnavailable {
		t.Fatalf("timeout code=%#v, want %s", resp.Payload["code"], codeUnavailable)
	}

	nextStreamID := startFakeSpeechStreamWithRequestID(t, client, 4)
	if nextStreamID == streamID {
		t.Fatalf("new streamID=%q, want a new stream", nextStreamID)
	}
}

func startFakeSpeechStream(t *testing.T, client *websocket.Conn) string {
	t.Helper()
	return startFakeSpeechStreamWithRequestID(t, client, 2)
}

func startFakeSpeechStreamWithRequestID(t *testing.T, client *websocket.Conn, requestID int64) string {
	t.Helper()
	writeSpeechStartRequest(t, client, requestID)
	resp := mustReadEnvelope(t, client)
	streamID, _ := resp.Payload["streamId"].(string)
	if streamID == "" {
		t.Fatalf("missing streamId in response: %#v", resp)
	}
	return streamID
}

func writeSpeechStartRequest(t *testing.T, client *websocket.Conn, requestID int64) {
	t.Helper()
	mustWriteJSON(t, client, testEnvelope{
		RequestID: requestID,
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

type timeoutFirstSpeechProvider struct {
	calls  int
	stream *fakeSpeechStream
}

func (p *timeoutFirstSpeechProvider) Start(ctx context.Context, req speechProviderStartRequest, events speechEventSink) (speechProviderStream, error) {
	p.calls++
	if p.calls == 1 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	p.stream = &fakeSpeechStream{
		events:    events,
		cancelled: make(chan struct{}),
	}
	return p.stream, nil
}

type blockingFinishSpeechProvider struct {
	stream *blockingFinishSpeechStream
}

func (p *blockingFinishSpeechProvider) Start(_ context.Context, req speechProviderStartRequest, events speechEventSink) (speechProviderStream, error) {
	p.stream = &blockingFinishSpeechStream{
		cancelled: make(chan struct{}),
	}
	return p.stream, nil
}

type blockingFinishSpeechStream struct {
	writes    [][]byte
	cancelled chan struct{}
}

func (s *blockingFinishSpeechStream) WriteAudio(_ context.Context, pcm []byte) error {
	s.writes = append(s.writes, append([]byte(nil), pcm...))
	return nil
}

func (s *blockingFinishSpeechStream) Finish(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (s *blockingFinishSpeechStream) Cancel() {
	select {
	case <-s.cancelled:
	default:
		close(s.cancelled)
	}
}
