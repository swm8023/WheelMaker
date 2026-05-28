package registry

import (
	"encoding/base64"

	rp "github.com/swm8023/wheelmaker/internal/protocol"
)

const (
	speechMethodStart  = rp.RegistryMethodSpeechStart
	speechMethodChunk  = rp.RegistryMethodSpeechChunk
	speechMethodFinish = rp.RegistryMethodSpeechFinish
	speechMethodCancel = rp.RegistryMethodSpeechCancel

	speechMethodTranscript = "speech.transcript"
	speechMethodError      = "speech.error"

	speechProviderVolcengine = "volcengine"
	speechModelDoubaoASR2    = "doubao-streaming-asr-2.0"

	volcengineSpeechEndpoint   = "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_async"
	volcengineSpeechResourceID = "volc.seedasr.sauc.duration"
	volcengineSpeechModelName  = "bigmodel"
)

type speechAudioConfig struct {
	Format  string `json:"format"`
	Codec   string `json:"codec"`
	Rate    int    `json:"rate"`
	Bits    int    `json:"bits"`
	Channel int    `json:"channel"`
}

type speechStartPayload struct {
	Provider string            `json:"provider"`
	Model    string            `json:"model"`
	APIKey   string            `json:"apiKey"`
	Audio    speechAudioConfig `json:"audio"`
}

type speechStartResponsePayload struct {
	StreamID string `json:"streamId"`
}

type speechChunkPayload struct {
	StreamID string `json:"streamId"`
	Seq      int64  `json:"seq"`
	PCM      string `json:"pcm"`
}

type speechChunkDebugPayload struct {
	StreamID  string `json:"streamId"`
	Seq       int64  `json:"seq"`
	PCM       string `json:"pcm"`
	ByteCount int    `json:"byteCount"`
}

type speechFinishPayload struct {
	StreamID string `json:"streamId"`
}

type speechCancelPayload struct {
	StreamID string `json:"streamId"`
	Reason   string `json:"reason"`
}

type speechOKPayload struct {
	OK       bool   `json:"ok"`
	StreamID string `json:"streamId"`
}

type speechTranscriptPayload struct {
	StreamID string `json:"streamId"`
	Text     string `json:"text"`
	Final    bool   `json:"final"`
}

type speechErrorEventPayload struct {
	StreamID  string `json:"streamId,omitempty"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func isSpeechRequestMethod(method string) bool {
	return rp.RegistrySpeechMethod(method)
}

func redactSpeechPayload(method string, payload any) any {
	switch method {
	case speechMethodStart:
		if typed, ok := payload.(speechStartPayload); ok {
			typed.APIKey = "[redacted]"
			return typed
		}
	case speechMethodChunk:
		if typed, ok := payload.(speechChunkPayload); ok {
			byteCount := 0
			if decoded, err := base64.StdEncoding.DecodeString(typed.PCM); err == nil {
				byteCount = len(decoded)
			}
			return speechChunkDebugPayload{
				StreamID:  typed.StreamID,
				Seq:       typed.Seq,
				PCM:       "[base64 omitted]",
				ByteCount: byteCount,
			}
		}
	}
	return payload
}
