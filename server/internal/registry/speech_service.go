package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

const (
	defaultSpeechIdleTimeout   = 10 * time.Second
	defaultSpeechStartTimeout  = 15 * time.Second
	defaultSpeechFinishTimeout = 5 * time.Second
)

type speechProviderStartRequest struct {
	Provider string
	Model    string
	APIKey   string
	Audio    speechAudioConfig
}

type speechProvider interface {
	Start(ctx context.Context, req speechProviderStartRequest, events speechEventSink) (speechProviderStream, error)
}

type speechProviderStream interface {
	WriteAudio(ctx context.Context, pcm []byte) error
	Finish(ctx context.Context) error
	Cancel()
}

type speechEventSink interface {
	Transcript(text string, final bool)
	Error(code string, message string, retryable bool)
}

type speechService struct {
	provider speechProvider

	idleTimeout   time.Duration
	startTimeout  time.Duration
	finishTimeout time.Duration

	nextStreamID atomic.Int64

	mu         sync.Mutex
	streams    map[string]*activeSpeechStream
	connActive map[string]string
}

type speechServiceOptions struct {
	idleTimeout   time.Duration
	startTimeout  time.Duration
	finishTimeout time.Duration
}

type speechStartResult struct {
	stream speechProviderStream
	err    error
}

type activeSpeechStream struct {
	connectionID string
	streamID     string
	peer         *peerConn
	stream       speechProviderStream
	cancel       context.CancelFunc
	idleTimer    *time.Timer
}

func newSpeechService(provider speechProvider) *speechService {
	return newSpeechServiceWithOptions(provider, speechServiceOptions{})
}

func newSpeechServiceWithOptions(provider speechProvider, options speechServiceOptions) *speechService {
	options = normalizeSpeechServiceOptions(options)
	return &speechService{
		provider:      provider,
		idleTimeout:   options.idleTimeout,
		startTimeout:  options.startTimeout,
		finishTimeout: options.finishTimeout,
		streams:       make(map[string]*activeSpeechStream),
		connActive:    make(map[string]string),
	}
}

func normalizeSpeechServiceOptions(options speechServiceOptions) speechServiceOptions {
	if options.idleTimeout == 0 {
		options.idleTimeout = defaultSpeechIdleTimeout
	}
	if options.startTimeout == 0 {
		options.startTimeout = defaultSpeechStartTimeout
	}
	if options.finishTimeout == 0 {
		options.finishTimeout = defaultSpeechFinishTimeout
	}
	return options
}

func (s *speechService) handleRequest(peer *peerConn, state *connectionState, in envelope) {
	if s == nil {
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeInternal, "speech service unavailable", nil)
		return
	}
	switch in.Method {
	case speechMethodStart:
		s.handleStart(peer, state, in)
	case speechMethodChunk:
		s.handleChunk(peer, state, in)
	case speechMethodFinish:
		s.handleFinish(peer, state, in)
	case speechMethodCancel:
		s.handleCancel(peer, state, in)
	default:
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeInvalidArgument, "unsupported speech method", map[string]any{"method": in.Method})
	}
}

func (s *speechService) handleStart(peer *peerConn, state *connectionState, in envelope) {
	var payload speechStartPayload
	if err := decodeSpeechPayload(in.Payload, &payload); err != nil {
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeInvalidArgument, "invalid speech.start payload", nil)
		return
	}
	if err := validateSpeechStart(payload); err != nil {
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeInvalidArgument, err.Error(), nil)
		return
	}

	streamID := fmt.Sprintf("speech-%d", s.nextStreamID.Add(1))
	ctx, cancel := context.WithCancel(context.Background())
	active := &activeSpeechStream{
		connectionID: state.id,
		streamID:     streamID,
		peer:         peer,
		cancel:       cancel,
	}
	var old *activeSpeechStream
	s.mu.Lock()
	if existing := s.connActive[state.id]; existing != "" {
		old = s.detachStreamLocked(state.id, existing)
	}
	s.streams[streamID] = active
	s.connActive[state.id] = streamID
	s.mu.Unlock()
	cancelSpeechProviderStream(old)

	stream, err := s.startProvider(ctx, speechProviderStartRequest{
		Provider: payload.Provider,
		Model:    payload.Model,
		APIKey:   payload.APIKey,
		Audio:    payload.Audio,
	}, speechStreamEvents{service: s, streamID: streamID})
	if err != nil {
		cancel()
		s.detachStream(state.id, streamID)
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeUnavailable, "speech provider start failed", map[string]any{"error": err.Error()})
		return
	}
	if !s.attachProviderStream(state.id, streamID, stream) {
		cancel()
		stream.Cancel()
		return
	}

	_ = writeSpeechResponse(peer, in.RequestID, in.Method, speechStartResponsePayload{StreamID: streamID})
}

func (s *speechService) handleChunk(peer *peerConn, state *connectionState, in envelope) {
	var payload speechChunkPayload
	if err := decodeSpeechPayload(in.Payload, &payload); err != nil {
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeInvalidArgument, "invalid speech.chunk payload", nil)
		return
	}
	stream, ok := s.lookupOwnedStream(state.id, payload.StreamID)
	if !ok {
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeNotFound, "speech stream not found", map[string]any{"streamId": payload.StreamID})
		return
	}
	pcm, err := base64.StdEncoding.DecodeString(payload.PCM)
	if err != nil {
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeInvalidArgument, "pcm must be base64", nil)
		return
	}
	if len(pcm) == 0 {
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeInvalidArgument, "pcm must not be empty", nil)
		return
	}
	if err := stream.stream.WriteAudio(context.Background(), pcm); err != nil {
		s.cancelStream(stream)
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeUnavailable, "speech provider write failed", map[string]any{"streamId": payload.StreamID})
		return
	}
	s.resetIdleTimer(stream)
	_ = writeSpeechResponse(peer, in.RequestID, in.Method, speechOKPayload{OK: true, StreamID: payload.StreamID})
}

func (s *speechService) handleFinish(peer *peerConn, state *connectionState, in envelope) {
	var payload speechFinishPayload
	if err := decodeSpeechPayload(in.Payload, &payload); err != nil {
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeInvalidArgument, "invalid speech.finish payload", nil)
		return
	}
	stream, ok := s.lookupOwnedStream(state.id, payload.StreamID)
	if !ok {
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeNotFound, "speech stream not found", map[string]any{"streamId": payload.StreamID})
		return
	}
	stream = s.detachStream(state.id, payload.StreamID)
	if stream == nil {
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeNotFound, "speech stream not found", map[string]any{"streamId": payload.StreamID})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.finishTimeout)
	defer cancel()
	if err := stream.stream.Finish(ctx); err != nil {
		cancelSpeechProviderStream(stream)
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeUnavailable, "speech provider finish failed", map[string]any{"streamId": payload.StreamID})
		return
	}
	_ = writeSpeechResponse(peer, in.RequestID, in.Method, speechOKPayload{OK: true, StreamID: payload.StreamID})
}

func (s *speechService) handleCancel(peer *peerConn, state *connectionState, in envelope) {
	var payload speechCancelPayload
	if err := decodeSpeechPayload(in.Payload, &payload); err != nil {
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeInvalidArgument, "invalid speech.cancel payload", nil)
		return
	}
	stream, ok := s.lookupOwnedStream(state.id, payload.StreamID)
	if !ok {
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeNotFound, "speech stream not found", map[string]any{"streamId": payload.StreamID})
		return
	}
	stream = s.detachStream(state.id, payload.StreamID)
	cancelSpeechProviderStream(stream)
	_ = writeSpeechResponse(peer, in.RequestID, in.Method, speechOKPayload{OK: true, StreamID: payload.StreamID})
}

func (s *speechService) cancelConnection(connectionID string) {
	if s == nil {
		return
	}
	stream := s.detachActiveStream(connectionID)
	if stream != nil {
		cancelSpeechProviderStream(stream)
	}
}

func (s *speechService) lookupOwnedStream(connectionID string, streamID string) (*activeSpeechStream, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stream := s.streams[streamID]
	if stream == nil || stream.connectionID != connectionID || stream.stream == nil {
		return nil, false
	}
	return stream, true
}

func (s *speechService) cancelStream(stream *activeSpeechStream) {
	if stream == nil {
		return
	}
	stream = s.detachStream(stream.connectionID, stream.streamID)
	cancelSpeechProviderStream(stream)
}

func (s *speechService) startProvider(ctx context.Context, req speechProviderStartRequest, events speechEventSink) (speechProviderStream, error) {
	resultCh := make(chan speechStartResult, 1)
	go func() {
		stream, err := s.provider.Start(ctx, req, events)
		resultCh <- speechStartResult{stream: stream, err: err}
	}()

	timer := time.NewTimer(s.startTimeout)
	defer timer.Stop()
	select {
	case result := <-resultCh:
		return result.stream, result.err
	case <-timer.C:
		go discardLateSpeechStart(resultCh)
		return nil, context.DeadlineExceeded
	case <-ctx.Done():
		go discardLateSpeechStart(resultCh)
		return nil, ctx.Err()
	}
}

func discardLateSpeechStart(resultCh <-chan speechStartResult) {
	result := <-resultCh
	if result.stream != nil {
		result.stream.Cancel()
	}
}

func (s *speechService) attachProviderStream(connectionID string, streamID string, providerStream speechProviderStream) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	stream := s.streams[streamID]
	if stream == nil || stream.connectionID != connectionID {
		return false
	}
	stream.stream = providerStream
	s.startIdleTimerLocked(stream)
	return true
}

func (s *speechService) startIdleTimerLocked(stream *activeSpeechStream) {
	if s.idleTimeout <= 0 {
		return
	}
	stream.idleTimer = time.AfterFunc(s.idleTimeout, func() {
		s.handleIdleTimeout(stream.connectionID, stream.streamID)
	})
}

func (s *speechService) resetIdleTimer(stream *activeSpeechStream) {
	if stream == nil || s.idleTimeout <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.streams[stream.streamID]
	if current == stream && current.idleTimer != nil {
		current.idleTimer.Reset(s.idleTimeout)
	}
}

func (s *speechService) handleIdleTimeout(connectionID string, streamID string) {
	stream := s.detachStream(connectionID, streamID)
	if stream == nil {
		return
	}
	_ = stream.peer.write(envelope{
		Type:   rp.RegistryEnvelopeTypeEvent,
		Method: speechMethodError,
		Payload: rp.MustRaw(speechErrorEventPayload{
			StreamID:  streamID,
			Code:      codeUnavailable,
			Message:   "speech stream idle timeout",
			Retryable: true,
		}),
	})
	cancelSpeechProviderStream(stream)
}

func (s *speechService) detachActiveStream(connectionID string) *activeSpeechStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.detachStreamLocked(connectionID, s.connActive[connectionID])
}

func (s *speechService) detachStream(connectionID string, streamID string) *activeSpeechStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.detachStreamLocked(connectionID, streamID)
}

func (s *speechService) detachStreamLocked(connectionID string, streamID string) *activeSpeechStream {
	stream := s.streams[streamID]
	if stream == nil || stream.connectionID != connectionID {
		return nil
	}
	if s.connActive[connectionID] == streamID {
		delete(s.connActive, connectionID)
	}
	delete(s.streams, streamID)
	if stream.idleTimer != nil {
		stream.idleTimer.Stop()
		stream.idleTimer = nil
	}
	return stream
}

func cancelSpeechProviderStream(stream *activeSpeechStream) {
	if stream == nil {
		return
	}
	stream.cancel()
	if stream.stream != nil {
		stream.stream.Cancel()
	}
}

type speechStreamEvents struct {
	service  *speechService
	streamID string
}

func (e speechStreamEvents) Transcript(text string, final bool) {
	stream := e.service.streamByID(e.streamID)
	if stream == nil {
		return
	}
	_ = stream.peer.write(envelope{
		Type:   rp.RegistryEnvelopeTypeEvent,
		Method: speechMethodTranscript,
		Payload: rp.MustRaw(speechTranscriptPayload{
			StreamID: e.streamID,
			Text:     text,
			Final:    final,
		}),
	})
	if final {
		stream = e.service.detachStream(stream.connectionID, e.streamID)
		if stream != nil {
			stream.cancel()
		}
	}
}

func (e speechStreamEvents) Error(code string, message string, retryable bool) {
	stream := e.service.streamByID(e.streamID)
	if stream == nil {
		return
	}
	_ = stream.peer.write(envelope{
		Type:   rp.RegistryEnvelopeTypeEvent,
		Method: speechMethodError,
		Payload: rp.MustRaw(speechErrorEventPayload{
			StreamID:  e.streamID,
			Code:      code,
			Message:   message,
			Retryable: retryable,
		}),
	})
	stream = e.service.detachStream(stream.connectionID, e.streamID)
	cancelSpeechProviderStream(stream)
}

func (s *speechService) streamByID(streamID string) *activeSpeechStream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streams[streamID]
}

func decodeSpeechPayload(payload json.RawMessage, out any) error {
	if len(payload) == 0 {
		return errors.New("missing payload")
	}
	return json.Unmarshal(payload, out)
}

func validateSpeechStart(payload speechStartPayload) error {
	if strings.TrimSpace(payload.Provider) != speechProviderVolcengine {
		return errors.New("provider must be volcengine")
	}
	if strings.TrimSpace(payload.Model) != speechModelDoubaoASR2 {
		return errors.New("model must be doubao-streaming-asr-2.0")
	}
	if strings.TrimSpace(payload.APIKey) == "" {
		return errors.New("apiKey is required")
	}
	if payload.Audio.Format != "pcm" || payload.Audio.Codec != "raw" || payload.Audio.Rate != 16000 || payload.Audio.Bits != 16 || payload.Audio.Channel != 1 {
		return errors.New("audio must be pcm/raw 16kHz 16-bit mono")
	}
	return nil
}

func writeSpeechResponse(peer *peerConn, requestID int64, method string, payload any) error {
	return peer.write(envelope{
		RequestID: requestID,
		Type:      rp.RegistryEnvelopeTypeResponse,
		Method:    method,
		Payload:   rp.MustRaw(payload),
	})
}

func writeSpeechError(peer *peerConn, requestID int64, method, code, message string, details map[string]any) error {
	return peer.write(envelope{
		RequestID: requestID,
		Type:      rp.RegistryEnvelopeTypeError,
		Method:    method,
		Payload: rp.MustRaw(errorPayload{
			Code:    code,
			Message: message,
			Details: details,
		}),
	})
}

type unavailableSpeechProvider struct{}

func newUnavailableSpeechProvider() speechProvider {
	return unavailableSpeechProvider{}
}

func (unavailableSpeechProvider) Start(context.Context, speechProviderStartRequest, speechEventSink) (speechProviderStream, error) {
	return nil, errors.New("speech provider unavailable")
}
