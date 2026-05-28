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

	rp "github.com/swm8023/wheelmaker/internal/protocol"
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

	nextStreamID atomic.Int64

	mu         sync.Mutex
	streams    map[string]*activeSpeechStream
	connActive map[string]string
}

type activeSpeechStream struct {
	connectionID string
	streamID     string
	peer         *peerConn
	stream       speechProviderStream
	cancel       context.CancelFunc
}

func newSpeechService(provider speechProvider) *speechService {
	return &speechService{
		provider:   provider,
		streams:    make(map[string]*activeSpeechStream),
		connActive: make(map[string]string),
	}
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

	s.mu.Lock()
	if existing := s.connActive[state.id]; existing != "" {
		s.mu.Unlock()
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeConflict, "speech stream already active", map[string]any{"streamId": existing})
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
	s.streams[streamID] = active
	s.connActive[state.id] = streamID
	s.mu.Unlock()

	stream, err := s.provider.Start(ctx, speechProviderStartRequest{
		Provider: payload.Provider,
		Model:    payload.Model,
		APIKey:   payload.APIKey,
		Audio:    payload.Audio,
	}, speechStreamEvents{service: s, streamID: streamID})
	if err != nil {
		cancel()
		s.removeStream(state.id, streamID)
		_ = writeSpeechError(peer, in.RequestID, in.Method, codeUnavailable, "speech provider start failed", map[string]any{"error": err.Error()})
		return
	}
	active.stream = stream

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
	if err := stream.stream.Finish(context.Background()); err != nil {
		s.cancelStream(stream)
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
	s.cancelStream(stream)
	_ = writeSpeechResponse(peer, in.RequestID, in.Method, speechOKPayload{OK: true, StreamID: payload.StreamID})
}

func (s *speechService) cancelConnection(connectionID string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	streamID := s.connActive[connectionID]
	stream := s.streams[streamID]
	s.mu.Unlock()
	if stream != nil {
		s.cancelStream(stream)
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
	stream.cancel()
	if stream.stream != nil {
		stream.stream.Cancel()
	}
	s.removeStream(stream.connectionID, stream.streamID)
}

func (s *speechService) removeStream(connectionID string, streamID string) {
	s.mu.Lock()
	if s.connActive[connectionID] == streamID {
		delete(s.connActive, connectionID)
	}
	delete(s.streams, streamID)
	s.mu.Unlock()
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
		stream.cancel()
		e.service.removeStream(stream.connectionID, e.streamID)
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
