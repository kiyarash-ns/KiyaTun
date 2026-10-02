package main

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	uOpen    = 1
	uOpenAck = 2
	uData    = 3
	uAck     = 4
	uClose   = 5
	uPing    = 6
	uPong    = 7

	udpSeg    = 1100
	udpWindow = 64
)

func sealU(a cipher.AEAD, plain []byte) []byte {
	nonce := randBytes(a.NonceSize())
	return a.Seal(nonce, nonce, plain, nil)
}

func openU(a cipher.AEAD, pkt []byte) ([]byte, bool) {
	ns := a.NonceSize()
	if len(pkt) < ns+a.Overhead() {
		return nil, false
	}
	pt, err := a.Open(nil, pkt[:ns], pkt[ns:], nil)
	if err != nil {
		return nil, false
	}
	return pt, true
}

func seqLT(a, b uint32) bool { return int32(a-b) < 0 }
func seqGE(a, b uint32) bool { return !seqLT(a, b) }

type inflightPkt struct {
	data  []byte
	sent  time.Time
	tries int
}

type udpSess struct {
	aead     cipher.AEAD
	connID   uint32
	isClient bool
	write    func([]byte) error

	sendMu  sync.Mutex
	nextSeq uint32
	inFlt   map[uint32]*inflightPkt

	recvMu   sync.Mutex
	recvNext uint32
	recvBuf  map[uint32][]byte
	readCh   chan []byte
	rbuf     []byte

	closed    chan struct{}
	closeOnce sync.Once
}

func newUDPSess(aead cipher.AEAD, connID uint32, isClient bool, write func([]byte) error) *udpSess {
	return &udpSess{
		aead: aead, connID: connID, isClient: isClient, write: write,
		inFlt:   map[uint32]*inflightPkt{},
		recvBuf: map[uint32][]byte{},
		readCh:  make(chan []byte, 256),
		closed:  make(chan struct{}),
	}
}

func (s *udpSess) handlePacket(typ byte, rest []byte) {
	switch typ {
	case uData:
		if len(rest) < 4 {
			return
		}
		seq := binary.BigEndian.Uint32(rest[:4])
		payload := rest[4:]
		s.recvMu.Lock()
		if seqGE(seq, s.recvNext) {
			if seq == s.recvNext {
				cp := append([]byte(nil), payload...)
				select {
				case s.readCh <- cp:
				default:
				}
				s.recvNext++
				for {
					b, ok := s.recvBuf[s.recvNext]
					if !ok {
						break
					}
					delete(s.recvBuf, s.recvNext)
					select {
					case s.readCh <- b:
					default:
					}
					s.recvNext++
				}
			} else if seq-s.recvNext <= 32 {
				if _, dup := s.recvBuf[seq]; !dup {
					s.recvBuf[seq] = append([]byte(nil), payload...)
				}
			}
		}
		s.recvMu.Unlock()
		s.sendAck()
	case uAck:
		if len(rest) < 8 {
			return
		}
		ack := binary.BigEndian.Uint32(rest[:4])
		bits := binary.BigEndian.Uint32(rest[4:8])
		s.sendMu.Lock()
		for seq := range s.inFlt {
			if seqLT(seq, ack) {
				delete(s.inFlt, seq)
			}
		}
		for i := uint32(0); i < 32; i++ {
			if bits&(1<<i) != 0 {
				delete(s.inFlt, ack+i)
			}
		}
		s.sendMu.Unlock()
	case uPing:
		pong := make([]byte, 5+len(rest))
		pong[0] = uPong
		binary.BigEndian.PutUint32(pong[1:], s.connID)
		copy(pong[5:], rest)
		_ = s.write(sealU(s.aead, pong))
	case uClose:
		s.resetLocal()
	}
}

func (s *udpSess) sendAck() {
	s.recvMu.Lock()
	ack := s.recvNext
	var bits uint32
	for i := uint32(0); i < 32; i++ {
		if _, ok := s.recvBuf[ack+i]; ok {
			bits |= 1 << i
		}
	}
	s.recvMu.Unlock()
	pkt := make([]byte, 13)
	pkt[0] = uAck
	binary.BigEndian.PutUint32(pkt[1:], s.connID)
	binary.BigEndian.PutUint32(pkt[5:], ack)
	binary.BigEndian.PutUint32(pkt[9:], bits)
	_ = s.write(sealU(s.aead, pkt))
}

func (s *udpSess) Write(p []byte) (int, error) {
	total := 0
	for len(p) > 0 {
		n := len(p)
		if n > udpSeg {
			n = udpSeg
		}
		chunk := p[:n]
		s.sendMu.Lock()
		for len(s.inFlt) >= udpWindow {
			s.sendMu.Unlock()
			select {
			case <-time.After(5 * time.Millisecond):
			case <-s.closed:
				return total, errors.New("udp session closed")
			}
			s.sendMu.Lock()
		}
		seq := s.nextSeq
		s.nextSeq++
		pkt := make([]byte, 9+n)
		pkt[0] = uData
		binary.BigEndian.PutUint32(pkt[1:], s.connID)
		binary.BigEndian.PutUint32(pkt[5:], seq)
		copy(pkt[9:], chunk)
		sealed := sealU(s.aead, pkt)
		s.inFlt[seq] = &inflightPkt{data: sealed, sent: time.Now()}
		s.sendMu.Unlock()
		if err := s.write(sealed); err != nil {
			return total, err
		}
		total += n
		p = p[n:]
	}
	return total, nil
}

func (s *udpSess) Read(p []byte) (int, error) {
	if len(s.rbuf) > 0 {
		n := copy(p, s.rbuf)
		s.rbuf = s.rbuf[n:]
		return n, nil
	}
	select {
	case b, ok := <-s.readCh:
		if !ok {
			return 0, io.EOF
		}
		n := copy(p, b)
		if n < len(b) {
			s.rbuf = b[n:]
		}
		return n, nil
	case <-s.closed:
		return 0, io.EOF
	}
}

// fixed-window ARQ, not real congestion control (no CWND growth/backoff
// like QUIC/BBR) -- enough to survive loss, not tuned for max throughput.
func (s *udpSess) retransmitLoop() {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			now := time.Now()
			s.sendMu.Lock()
			for seq, p := range s.inFlt {
				if now.Sub(p.sent) < 400*time.Millisecond {
					continue
				}
				if p.tries > 15 {
					s.sendMu.Unlock()
					s.resetLocal()
					return
				}
				p.tries++
				p.sent = now
				_ = s.write(p.data)
				_ = seq
			}
			s.sendMu.Unlock()
		case <-s.closed:
			return
		}
	}
}

func (s *udpSess) Close() error {
	pkt := make([]byte, 5)
	pkt[0] = uClose
	binary.BigEndian.PutUint32(pkt[1:], s.connID)
	_ = s.write(sealU(s.aead, pkt))
	s.resetLocal()
	return nil
}

func (s *udpSess) resetLocal() {
	s.closeOnce.Do(func() { close(s.closed) })
}

func (s *udpSess) LocalAddr() net.Addr              { return udpAddr{} }
func (s *udpSess) RemoteAddr() net.Addr             { return udpAddr{} }
func (s *udpSess) SetDeadline(time.Time) error      { return nil }
func (s *udpSess) SetReadDeadline(time.Time) error  { return nil }
func (s *udpSess) SetWriteDeadline(time.Time) error { return nil }

type udpAddr struct{}

func (udpAddr) Network() string { return "tunx-udp" }
func (udpAddr) String() string  { return "tunx-udp" }

func dialUDPConn(addr, key string, timeout time.Duration) (net.Conn, error) {
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}
	aead := newAEAD(deriveKey(key, "udpconn", nil))
	connID := uint32(randU64())
	s := newUDPSess(aead, connID, true, func(b []byte) error {
		_, err := conn.Write(b)
		return err
	})

	hello := make([]byte, 13)
	hello[0] = uOpen
	binary.BigEndian.PutUint32(hello[1:], connID)
	binary.BigEndian.PutUint64(hello[5:], uint64(time.Now().Unix()))
	sealedHello := sealU(aead, hello)

	buf := make([]byte, 2048)
	ok := false
	for i := 0; i < 8 && !ok; i++ {
		conn.Write(sealedHello)
		conn.SetReadDeadline(time.Now().Add(timeout / 8))
		n, err := conn.Read(buf)
		if err != nil {
			continue
		}
		pt, valid := openU(aead, buf[:n])
		if valid && len(pt) >= 5 && pt[0] == uOpenAck && binary.BigEndian.Uint32(pt[1:5]) == connID {
			ok = true
		}
	}
	if !ok {
		conn.Close()
		return nil, errors.New("udp handshake timeout")
	}
	conn.SetDeadline(time.Time{})

	go func() {
		rbuf := make([]byte, 2048)
		for {
			n, err := conn.Read(rbuf)
			if err != nil {
				s.resetLocal()
				return
			}
			pt, valid := openU(aead, rbuf[:n])
			if !valid || len(pt) < 5 {
				continue
			}
			if binary.BigEndian.Uint32(pt[1:5]) != connID {
				continue
			}
			s.handlePacket(pt[0], pt[5:])
		}
	}()
	go s.retransmitLoop()
	return s, nil
}

type udpTunnelListener struct {
	pc       *net.UDPConn
	aead     cipher.AEAD
	mu       sync.Mutex
	sessions map[string]*udpSess
	acceptCh chan net.Conn
	closed   chan struct{}
}

func listenUDPConn(addr, key string) (*udpTunnelListener, error) {
	a, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	pc, err := net.ListenUDP("udp", a)
	if err != nil {
		return nil, err
	}
	l := &udpTunnelListener{
		pc:       pc,
		aead:     newAEAD(deriveKey(key, "udpconn", nil)),
		sessions: map[string]*udpSess{},
		acceptCh: make(chan net.Conn, 32),
		closed:   make(chan struct{}),
	}
	go l.readLoop()
	return l, nil
}

func sessKey(addr net.Addr, connID uint32) string {
	return fmt.Sprintf("%s|%d", addr.String(), connID)
}

func (l *udpTunnelListener) readLoop() {
	buf := make([]byte, 2048)
	for {
		n, raddr, err := l.pc.ReadFromUDP(buf)
		if err != nil {
			close(l.closed)
			return
		}
		pt, ok := openU(l.aead, buf[:n])
		if !ok || len(pt) < 5 {
			continue
		}
		typ := pt[0]
		connID := binary.BigEndian.Uint32(pt[1:5])
		k := sessKey(raddr, connID)

		if typ == uOpen {
			l.mu.Lock()
			_, exists := l.sessions[k]
			l.mu.Unlock()
			l.sendOpenAck(raddr, connID)
			if exists {
				continue
			}
			ra := raddr
			s := newUDPSess(l.aead, connID, false, func(b []byte) error {
				_, err := l.pc.WriteToUDP(b, ra)
				return err
			})
			l.mu.Lock()
			l.sessions[k] = s
			l.mu.Unlock()
			go s.retransmitLoop()
			select {
			case l.acceptCh <- s:
			default:
				l.mu.Lock()
				delete(l.sessions, k)
				l.mu.Unlock()
			}
			continue
		}

		l.mu.Lock()
		s := l.sessions[k]
		l.mu.Unlock()
		if s == nil {
			continue
		}
		s.handlePacket(typ, pt[5:])
	}
}

func (l *udpTunnelListener) sendOpenAck(raddr *net.UDPAddr, connID uint32) {
	resp := make([]byte, 13)
	resp[0] = uOpenAck
	binary.BigEndian.PutUint32(resp[1:], connID)
	binary.BigEndian.PutUint64(resp[5:], uint64(time.Now().Unix()))
	l.pc.WriteToUDP(sealU(l.aead, resp), raddr)
}

func (l *udpTunnelListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.acceptCh:
		return c, nil
	case <-l.closed:
		return nil, errors.New("udp listener closed")
	}
}

func (l *udpTunnelListener) Close() error   { return l.pc.Close() }
func (l *udpTunnelListener) Addr() net.Addr { return l.pc.LocalAddr() }
