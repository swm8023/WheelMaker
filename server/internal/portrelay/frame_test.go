package portrelay

import (
	"bytes"
	"testing"
)

func TestFrameCodecRoundTripBinaryPayload(t *testing.T) {
	frame := Frame{
		Type:     FrameData,
		Flags:    FlagWebSocketBinary,
		StreamID: 42,
		Meta:     []byte(`{"kind":"websocket"}`),
		Payload:  []byte{0, 1, 2, 255},
	}

	encoded, err := EncodeFrame(frame)
	if err != nil {
		t.Fatalf("EncodeFrame() err=%v", err)
	}
	decoded, err := DecodeFrame(encoded)
	if err != nil {
		t.Fatalf("DecodeFrame() err=%v", err)
	}

	if decoded.Type != frame.Type || decoded.Flags != frame.Flags || decoded.StreamID != frame.StreamID {
		t.Fatalf("decoded header=%#v, want %#v", decoded, frame)
	}
	if !bytes.Equal(decoded.Meta, frame.Meta) {
		t.Fatalf("decoded meta=%q, want %q", decoded.Meta, frame.Meta)
	}
	if !bytes.Equal(decoded.Payload, frame.Payload) {
		t.Fatalf("decoded payload=%v, want %v", decoded.Payload, frame.Payload)
	}
}

func TestFrameCodecRejectsBadMagic(t *testing.T) {
	encoded, err := EncodeFrame(Frame{Type: FramePing, StreamID: 1})
	if err != nil {
		t.Fatalf("EncodeFrame() err=%v", err)
	}
	encoded[0] = 'X'

	if _, err := DecodeFrame(encoded); err == nil {
		t.Fatal("DecodeFrame() err=nil, want bad magic error")
	}
}

func TestFrameCodecRejectsOversizedMetadata(t *testing.T) {
	_, err := EncodeFrame(Frame{
		Type:     FrameOpen,
		StreamID: 1,
		Meta:     bytes.Repeat([]byte{'a'}, maxFrameMetaBytes+1),
	})
	if err == nil {
		t.Fatal("EncodeFrame() err=nil, want metadata size error")
	}
}
