package main

import (
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type udpSess struct {
	seen   map[uint32]struct{}
	bySize map[int]int
	last   time.Time
}

type agent struct {
	secret  string
	udp     cipher.AEAD
	mu      sync.Mutex
	sess    map[uint64]*udpSess
	filler  []byte
	pending int32 // unauthenticated TCP connections currently held open
}

type report struct {
	Recv   int         `json:"recv"`
	BySize map[int]int `json:"by_size"`
}

func runAgent(addr, secret string) error {
	a := &agent{
		secret: secret,
		udp:    newUDPAEAD(secret),
		sess:   map[uint64]*udpSess{},
		filler: randBytes(recMax),
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	ua, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	uc, err := net.ListenUDP("udp", ua)
	if err != nil {
		return err
	}
	log.Printf("agent listening on %s (tcp+udp)", addr)
	go a.udpLoop(uc)
	go a.gc()
	for {
		c, err := ln.Accept()
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		go a.handleTCP(c)
	}
}

func (a *agent) gc() {
	for range time.Tick(30 * time.Second) {
		a.mu.Lock()
		for id, s := range a.sess {
			if time.Since(s.last) > 3*time.Minute {
				delete(a.sess, id)
			}
		}
		a.mu.Unlock()
	}
}

func (a *agent) udpLoop(uc *net.UDPConn) {
	buf := make([]byte, 2048)
	for {
		n, raddr, err := uc.ReadFromUDP(buf)
		if err != nil {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		p, ok := udpOpen(a.udp, buf[:n])
		if !ok || p.typ != 1 {
			continue // silent: unauthenticated packets get no reply at all
		}
		a.mu.Lock()
		s := a.sess[p.sid]
		if s == nil {
			if len(a.sess) >= 2048 {
				a.mu.Unlock()
				continue
			}
			s = &udpSess{seen: map[uint32]struct{}{}, bySize: map[int]int{}}
			a.sess[p.sid] = s
		}
		s.last = time.Now()
		if _, dup := s.seen[p.seq]; !dup && len(s.seen) < 100000 {
			s.seen[p.seq] = struct{}{}
			s.bySize[n]++
		}
		a.mu.Unlock()
		// echo is exactly the same size as the request: no amplification
		out := udpSeal(a.udp, udpPkt{sid: p.sid, seq: p.seq, ts: time.Now().Unix(), typ: 2}, n)
		uc.WriteToUDP(out, raddr)
	}
}

func (a *agent) handleTCP(c net.Conn) {
	defer c.Close()
	if atomic.AddInt32(&a.pending, 1) > 256 {
		atomic.AddInt32(&a.pending, -1)
		return
	}
	authed := false
	defer func() {
		if !authed {
			atomic.AddInt32(&a.pending, -1)
		}
	}()

	c.SetDeadline(time.Now().Add(8 * time.Second))
	salt := make([]byte, 16)
	if _, err := io.ReadFull(c, salt); err != nil {
		return
	}
	rc := newRec(c, a.secret, salt, true)
	typ, data, err := rc.recv()
	if err != nil || typ != tHello || len(data) < 8 {
		// no reply, no banner: hold briefly like a dead port then drop
		time.Sleep(time.Duration(300+rand.Intn(1700)) * time.Millisecond)
		return
	}
	d := time.Now().Unix() - int64(binary.BigEndian.Uint64(data))
	if d > 60 || d < -60 {
		return
	}
	authed = true
	atomic.AddInt32(&a.pending, -1)

	ack := make([]byte, 8)
	binary.BigEndian.PutUint64(ack, uint64(time.Now().Unix()))
	if rc.send(tHello, ack) != nil {
		return
	}

	for {
		c.SetDeadline(time.Now().Add(90 * time.Second))
		typ, data, err := rc.recv()
		if err != nil {
			return
		}
		switch typ {
		case tPing:
			if rc.send(tPing, data) != nil {
				return
			}
		case tReport:
			if len(data) < 8 {
				return
			}
			sid := binary.BigEndian.Uint64(data)
			r := report{BySize: map[int]int{}}
			a.mu.Lock()
			if s := a.sess[sid]; s != nil {
				r.Recv = len(s.seen)
				for k, v := range s.bySize {
					r.BySize[k] = v
				}
			}
			a.mu.Unlock()
			b, _ := json.Marshal(r)
			if rc.send(tReport, b) != nil {
				return
			}
		case tUp:
			if !a.serveUp(c, rc) {
				return
			}
		case tDown:
			if len(data) < 4 {
				return
			}
			if !a.serveDown(c, rc, binary.BigEndian.Uint32(data)) {
				return
			}
		default:
			return
		}
	}
}

// serveUp receives records until tEnd and reports how many it got and how long it took.
func (a *agent) serveUp(c net.Conn, rc *recConn) bool {
	var recs uint64
	var first time.Time
	for {
		c.SetDeadline(time.Now().Add(30 * time.Second))
		t, _, err := rc.recv()
		if err != nil {
			return false
		}
		if t == tData {
			if first.IsZero() {
				first = time.Now()
			}
			recs++
			continue
		}
		if t == tEnd {
			break
		}
		return false
	}
	var el time.Duration
	if !first.IsZero() {
		el = time.Since(first)
	}
	res := make([]byte, 16)
	binary.BigEndian.PutUint64(res[0:], recs)
	binary.BigEndian.PutUint64(res[8:], uint64(el.Nanoseconds()))
	return rc.send(tUpRes, res) == nil
}

// serveDown streams records for the requested duration, then tEnd.
func (a *agent) serveDown(c net.Conn, rc *recConn, ms uint32) bool {
	if ms < 1000 {
		ms = 1000
	}
	if ms > 20000 {
		ms = 20000
	}
	dur := time.Duration(ms) * time.Millisecond
	start := time.Now()
	for time.Since(start) < dur {
		c.SetDeadline(time.Now().Add(20 * time.Second))
		if rc.send(tData, a.filler) != nil {
			return false
		}
	}
	return rc.send(tEnd, nil) == nil
}
