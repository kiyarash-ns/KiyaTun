package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Session struct {
	rc      *recConn
	wmu     sync.Mutex
	nextSid uint32

	mu      sync.Mutex
	streams map[uint32]*Stream
	pending map[uint32]chan openResult

	acceptC   chan *Stream
	closed    chan struct{}
	closeOnce sync.Once
}

type openResult struct {
	ok  bool
	msg string
}

func newSession(rc *recConn, isServerSide bool) *Session {
	s := &Session{
		rc:      rc,
		streams: map[uint32]*Stream{},
		pending: map[uint32]chan openResult{},
		acceptC: make(chan *Stream, 64),
		closed:  make(chan struct{}),
	}
	if isServerSide {
		s.nextSid = 2
	} else {
		s.nextSid = 1
	}
	go s.readLoop()
	return s
}

func (s *Session) readLoop() {
	defer s.Close()
	for {
		typ, data, err := s.rc.recv()
		if err != nil {
			return
		}
		if typ != tKeepAlive && len(data) < 4 {
			continue
		}
		var sid uint32
		if typ != tKeepAlive {
			sid = binary.BigEndian.Uint32(data[:4])
		}

		switch typ {
		case tOpen:
			target := string(data[4:])
			st := newStream(s, sid, target)
			s.mu.Lock()
			s.streams[sid] = st
			s.mu.Unlock()
			select {
			case s.acceptC <- st:
			default:
				s.sendCtl(tOpenErr, sid, []byte("backlog full"))
				s.mu.Lock()
				delete(s.streams, sid)
				s.mu.Unlock()
			}

		case tOpenOK:
			s.mu.Lock()
			ch := s.pending[sid]
			delete(s.pending, sid)
			s.mu.Unlock()
			if ch != nil {
				ch <- openResult{ok: true}
			}

		case tOpenErr:
			msg := string(data[4:])
			s.mu.Lock()
			ch := s.pending[sid]
			delete(s.pending, sid)
			s.mu.Unlock()
			if ch != nil {
				ch <- openResult{ok: false, msg: msg}
			}

		case tSData:
			s.mu.Lock()
			st := s.streams[sid]
			s.mu.Unlock()
			if st == nil || atomic.LoadInt32(&st.remoteClosed) == 1 {
				continue
			}
			cp := make([]byte, len(data)-4)
			copy(cp, data[4:])
			select {
			case st.readCh <- cp:
			case <-st.closed:
			case <-time.After(30 * time.Second):
				// receiver stuck: drop this stream rather than stall the
				// whole session for every other stream sharing it.
				st.resetLocal()
			}

		case tSClose:
			s.mu.Lock()
			st := s.streams[sid]
			s.mu.Unlock()
			if st != nil {
				st.remoteClose()
			}

		case tSReset:
			s.mu.Lock()
			st := s.streams[sid]
			delete(s.streams, sid)
			s.mu.Unlock()
			if st != nil {
				st.resetLocal()
			}

		case tKeepAlive:
			// presence of traffic is enough; nothing to do
		}
	}
}

func (s *Session) sendCtl(typ byte, sid uint32, extra []byte) error {
	b := make([]byte, 4+len(extra))
	binary.BigEndian.PutUint32(b, sid)
	copy(b[4:], extra)
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return s.rc.send(typ, b)
}

func (s *Session) Open(target string) (*Stream, error) {
	sid := atomic.AddUint32(&s.nextSid, 2)
	st := newStream(s, sid, target)
	ch := make(chan openResult, 1)
	s.mu.Lock()
	s.streams[sid] = st
	s.pending[sid] = ch
	s.mu.Unlock()

	if err := s.sendCtl(tOpen, sid, []byte(target)); err != nil {
		s.mu.Lock()
		delete(s.streams, sid)
		delete(s.pending, sid)
		s.mu.Unlock()
		return nil, err
	}
	select {
	case r := <-ch:
		if !r.ok {
			s.mu.Lock()
			delete(s.streams, sid)
			s.mu.Unlock()
			return nil, fmt.Errorf("peer rejected stream: %s", r.msg)
		}
		return st, nil
	case <-time.After(10 * time.Second):
		s.mu.Lock()
		delete(s.streams, sid)
		delete(s.pending, sid)
		s.mu.Unlock()
		return nil, errors.New("open timeout")
	case <-s.closed:
		return nil, errors.New("session closed")
	}
}

func (s *Session) Accept() (*Stream, error) {
	select {
	case st := <-s.acceptC:
		return st, nil
	case <-s.closed:
		return nil, errors.New("session closed")
	}
}

func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		close(s.closed)
		s.mu.Lock()
		for _, st := range s.streams {
			st.resetLocal()
		}
		s.streams = map[uint32]*Stream{}
		s.mu.Unlock()
	})
	return nil
}

func (s *Session) Done() <-chan struct{} { return s.closed }

func (s *Session) keepAlive(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if s.sendCtl(tKeepAlive, 0, nil) != nil {
				return
			}
		case <-s.closed:
			return
		}
	}
}

type Stream struct {
	sess   *Session
	sid    uint32
	Target string

	readCh       chan []byte
	buf          []byte
	closed       chan struct{}
	closeOnce    sync.Once
	remoteClosed int32
}

func newStream(s *Session, sid uint32, target string) *Stream {
	return &Stream{
		sess: s, sid: sid, Target: target,
		readCh: make(chan []byte, 256),
		closed: make(chan struct{}),
	}
}

func (st *Stream) Accept() error {
	return st.sess.sendCtl(tOpenOK, st.sid, nil)
}

func (st *Stream) Reject(reason string) error {
	err := st.sess.sendCtl(tOpenErr, st.sid, []byte(reason))
	st.sess.mu.Lock()
	delete(st.sess.streams, st.sid)
	st.sess.mu.Unlock()
	return err
}

func (st *Stream) Read(p []byte) (int, error) {
	if len(st.buf) > 0 {
		n := copy(p, st.buf)
		st.buf = st.buf[n:]
		return n, nil
	}
	select {
	case b, ok := <-st.readCh:
		if !ok {
			return 0, io.EOF
		}
		n := copy(p, b)
		if n < len(b) {
			st.buf = b[n:]
		}
		return n, nil
	case <-st.closed:
		return 0, io.EOF
	}
}

const streamChunk = recMax - 4

func (st *Stream) Write(p []byte) (int, error) {
	total := 0
	for len(p) > 0 {
		n := len(p)
		if n > streamChunk {
			n = streamChunk
		}
		b := make([]byte, 4+n)
		binary.BigEndian.PutUint32(b, st.sid)
		copy(b[4:], p[:n])
		st.sess.wmu.Lock()
		err := st.sess.rc.send(tSData, b)
		st.sess.wmu.Unlock()
		if err != nil {
			return total, err
		}
		total += n
		p = p[n:]
	}
	return total, nil
}

func (st *Stream) Close() error {
	st.sess.mu.Lock()
	delete(st.sess.streams, st.sid)
	st.sess.mu.Unlock()
	st.sess.sendCtl(tSClose, st.sid, nil)
	st.closeOnce.Do(func() { close(st.closed) })
	return nil
}

func (st *Stream) remoteClose() {
	if atomic.CompareAndSwapInt32(&st.remoteClosed, 0, 1) {
		close(st.readCh)
	}
}

func (st *Stream) resetLocal() {
	st.closeOnce.Do(func() { close(st.closed) })
}

func (st *Stream) SetDeadline(time.Time) error      { return nil }
func (st *Stream) SetReadDeadline(time.Time) error  { return nil }
func (st *Stream) SetWriteDeadline(time.Time) error { return nil }
func (st *Stream) LocalAddr() net.Addr              { return streamAddr{} }
func (st *Stream) RemoteAddr() net.Addr             { return streamAddr{} }

type streamAddr struct{}

func (streamAddr) Network() string { return "tunx-stream" }
func (streamAddr) String() string  { return "tunx-stream" }
