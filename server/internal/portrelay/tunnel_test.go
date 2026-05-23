package portrelay

import (
	"testing"
	"time"
)

func TestRegistryTunnelRouteBackpressuresInsteadOfDroppingDataFrames(t *testing.T) {
	tunnel := newRegistryTunnel(nil, nil)
	stream := &registryStream{
		id:      1,
		tunnel:  tunnel,
		headers: make(chan Frame, 1),
		frames:  make(chan Frame, 1),
		closed:  make(chan struct{}),
	}
	tunnel.streams[stream.id] = stream

	tunnel.route(Frame{Type: FrameData, StreamID: stream.id, Payload: []byte("first")})

	delivered := make(chan struct{})
	go func() {
		tunnel.route(Frame{Type: FrameData, StreamID: stream.id, Payload: []byte("second")})
		close(delivered)
	}()

	select {
	case <-delivered:
		t.Fatalf("route returned while the stream frame buffer was full; data frames must not be dropped")
	case <-time.After(20 * time.Millisecond):
	}

	first := <-stream.frames
	if string(first.Payload) != "first" {
		t.Fatalf("first payload=%q, want first", string(first.Payload))
	}

	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatalf("route did not resume after the stream frame buffer had space")
	}

	second := <-stream.frames
	if string(second.Payload) != "second" {
		t.Fatalf("second payload=%q, want second", string(second.Payload))
	}
}
