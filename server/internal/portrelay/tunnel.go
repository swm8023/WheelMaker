package portrelay

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type registryTunnel struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
	nextID  atomic.Uint32
	closed  chan struct{}
	onClose func()

	mu      sync.RWMutex
	streams map[uint32]*registryStream
}

type registryStream struct {
	id     uint32
	tunnel *registryTunnel

	headers chan Frame
	frames  chan Frame
	closed  chan struct{}
	once    sync.Once
}

func newRegistryTunnel(conn *websocket.Conn, onClose func()) *registryTunnel {
	return &registryTunnel{
		conn:    conn,
		closed:  make(chan struct{}),
		onClose: onClose,
		streams: make(map[uint32]*registryStream),
	}
}

func (t *registryTunnel) openStream(meta any) (*registryStream, error) {
	streamID := t.nextID.Add(1)
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	stream := &registryStream{
		id:      streamID,
		tunnel:  t,
		headers: make(chan Frame, 1),
		frames:  make(chan Frame, 32),
		closed:  make(chan struct{}),
	}
	t.mu.Lock()
	t.streams[streamID] = stream
	t.mu.Unlock()
	if err := t.write(Frame{Type: FrameOpen, StreamID: streamID, Meta: metaBytes}); err != nil {
		t.removeStream(streamID)
		return nil, err
	}
	return stream, nil
}

func (t *registryTunnel) write(frame Frame) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return writeFrame(t.conn, frame)
}

func (t *registryTunnel) readLoop() {
	defer t.Close()
	for {
		frame, err := readFrame(t.conn)
		if err != nil {
			return
		}
		t.route(frame)
	}
}

func (t *registryTunnel) route(frame Frame) {
	t.mu.RLock()
	stream := t.streams[frame.StreamID]
	t.mu.RUnlock()
	if stream == nil {
		return
	}
	switch frame.Type {
	case FrameHeaders:
		_ = stream.enqueueHeader(frame)
	case FrameData, FrameClose, FrameError:
		enqueued := stream.enqueueFrame(frame)
		if frame.Type == FrameClose || frame.Type == FrameError {
			t.removeStream(frame.StreamID)
			if !enqueued {
				stream.closeLocal()
			}
		}
	}
}

func (s *registryStream) enqueueHeader(frame Frame) bool {
	select {
	case s.headers <- frame:
		return true
	case <-s.closed:
		return false
	case <-s.tunnel.closed:
		return false
	}
}

func (s *registryStream) enqueueFrame(frame Frame) bool {
	select {
	case s.frames <- frame:
		return true
	case <-s.closed:
		return false
	case <-s.tunnel.closed:
		return false
	}
}

func (t *registryTunnel) removeStream(streamID uint32) {
	t.mu.Lock()
	delete(t.streams, streamID)
	t.mu.Unlock()
}

func (t *registryTunnel) Close() {
	select {
	case <-t.closed:
		return
	default:
		close(t.closed)
	}
	_ = t.conn.Close()
	t.mu.Lock()
	streams := make([]*registryStream, 0, len(t.streams))
	for _, stream := range t.streams {
		streams = append(streams, stream)
	}
	t.streams = map[uint32]*registryStream{}
	t.mu.Unlock()
	for _, stream := range streams {
		stream.closeLocal()
	}
	if t.onClose != nil {
		t.onClose()
	}
}

func (s *registryStream) waitHeaders(timeout time.Duration) (Frame, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case frame := <-s.headers:
		return frame, nil
	case frame := <-s.frames:
		if frame.Type == FrameError {
			return Frame{}, fmt.Errorf("stream error")
		}
		return Frame{}, fmt.Errorf("stream closed before headers")
	case <-timer.C:
		return Frame{}, fmt.Errorf("stream headers timeout")
	case <-s.closed:
		return Frame{}, fmt.Errorf("stream closed")
	}
}

func (s *registryStream) nextData() (Frame, bool) {
	select {
	case frame := <-s.frames:
		if frame.Type == FrameClose || frame.Type == FrameError {
			s.closeLocal()
		}
		return frame, true
	case <-s.closed:
		return Frame{Type: FrameClose, StreamID: s.id}, true
	case <-s.tunnel.closed:
		return Frame{}, false
	}
}

func (s *registryStream) sendData(flags uint8, payload []byte) error {
	return s.tunnel.write(Frame{Type: FrameData, Flags: flags, StreamID: s.id, Payload: payload})
}

func (s *registryStream) sendClose() error {
	return s.tunnel.write(Frame{Type: FrameClose, StreamID: s.id})
}

func (s *registryStream) Close() {
	s.once.Do(func() {
		_ = s.sendClose()
		s.closeLocal()
		s.tunnel.removeStream(s.id)
	})
}

func (s *registryStream) closeLocal() {
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
}
