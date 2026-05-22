package portrelay

import (
	"encoding/binary"
	"fmt"

	"github.com/gorilla/websocket"
)

const (
	frameMagic       = byte('W')
	frameVersion     = byte(1)
	frameHeaderBytes = 16

	maxFrameMetaBytes    = 64 * 1024
	maxFramePayloadBytes = 1024 * 1024
)

const (
	FrameOpen    uint8 = 1
	FrameHeaders uint8 = 2
	FrameData    uint8 = 3
	FrameClose   uint8 = 4
	FrameError   uint8 = 5
	FramePing    uint8 = 6
	FramePong    uint8 = 7
)

const (
	FlagWebSocketText   uint8 = 0x01
	FlagWebSocketBinary uint8 = 0x02
	FlagHalfClose       uint8 = 0x04
	FlagMetadataOnly    uint8 = 0x08
)

type Frame struct {
	Type     uint8
	Flags    uint8
	StreamID uint32
	Meta     []byte
	Payload  []byte
}

func EncodeFrame(frame Frame) ([]byte, error) {
	if len(frame.Meta) > maxFrameMetaBytes {
		return nil, fmt.Errorf("frame metadata too large: %d", len(frame.Meta))
	}
	if len(frame.Payload) > maxFramePayloadBytes {
		return nil, fmt.Errorf("frame payload too large: %d", len(frame.Payload))
	}
	out := make([]byte, frameHeaderBytes+len(frame.Meta)+len(frame.Payload))
	out[0] = frameMagic
	out[1] = frameVersion
	out[2] = frame.Type
	out[3] = frame.Flags
	binary.BigEndian.PutUint32(out[4:8], frame.StreamID)
	binary.BigEndian.PutUint32(out[8:12], uint32(len(frame.Meta)))
	binary.BigEndian.PutUint32(out[12:16], uint32(len(frame.Payload)))
	copy(out[16:], frame.Meta)
	copy(out[16+len(frame.Meta):], frame.Payload)
	return out, nil
}

func DecodeFrame(raw []byte) (Frame, error) {
	if len(raw) < frameHeaderBytes {
		return Frame{}, fmt.Errorf("frame too short: %d", len(raw))
	}
	if raw[0] != frameMagic {
		return Frame{}, fmt.Errorf("bad frame magic")
	}
	if raw[1] != frameVersion {
		return Frame{}, fmt.Errorf("unsupported frame version: %d", raw[1])
	}
	metaLen := int(binary.BigEndian.Uint32(raw[8:12]))
	payloadLen := int(binary.BigEndian.Uint32(raw[12:16]))
	if metaLen > maxFrameMetaBytes {
		return Frame{}, fmt.Errorf("frame metadata too large: %d", metaLen)
	}
	if payloadLen > maxFramePayloadBytes {
		return Frame{}, fmt.Errorf("frame payload too large: %d", payloadLen)
	}
	if len(raw) != frameHeaderBytes+metaLen+payloadLen {
		return Frame{}, fmt.Errorf("frame length mismatch")
	}
	metaStart := frameHeaderBytes
	payloadStart := metaStart + metaLen
	return Frame{
		Type:     raw[2],
		Flags:    raw[3],
		StreamID: binary.BigEndian.Uint32(raw[4:8]),
		Meta:     append([]byte(nil), raw[metaStart:payloadStart]...),
		Payload:  append([]byte(nil), raw[payloadStart:]...),
	}, nil
}

func writeFrame(conn *websocket.Conn, frame Frame) error {
	raw, err := EncodeFrame(frame)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, raw)
}

func readFrame(conn *websocket.Conn) (Frame, error) {
	messageType, raw, err := conn.ReadMessage()
	if err != nil {
		return Frame{}, err
	}
	if messageType != websocket.BinaryMessage {
		return Frame{}, fmt.Errorf("tunnel message must be binary")
	}
	return DecodeFrame(raw)
}
