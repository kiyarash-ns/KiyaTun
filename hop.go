package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

func hopPort(key string, base, count int, window time.Duration, t time.Time) int {
	if count <= 0 {
		count = 1
	}
	winIdx := t.Unix() / int64(window.Seconds())
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(winIdx))
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte("tunx-hop|"))
	h.Write(buf)
	sum := h.Sum(nil)
	n := binary.BigEndian.Uint64(sum[:8])
	return base + int(n%uint64(count))
}

// resolveHopAddr swaps the port in addr for the one both sides will compute
// independently for the current time window (offset in units of `window`,
// e.g. -1/0/1 for previous/current/next).
func resolveHopAddr(addr, key string, base, count int, window time.Duration, offset int) (string, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr // no ":port" given at all; treat the whole thing as host
	}
	port := hopPort(key, base, count, window, time.Now().Add(time.Duration(offset)*window))
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

// hopListener keeps listeners open on the previous/current/next hop ports
// simultaneously (tolerating modest clock drift between the two sides) and
// rotates them as time passes, fed by listenControl so it composes with
// raw/tls/ws/wss underneath.
type hopListener struct {
	host                     string
	key, transport           string
	certFile, keyFile        string
	base, count              int
	window                   time.Duration

	mu     sync.Mutex
	active map[int]net.Listener

	acceptCh chan net.Conn
	stop     chan struct{}
}

func newHopListener(host, key, transport, certFile, keyFile string, base, count int, window time.Duration) *hopListener {
	hl := &hopListener{
		host: host, key: key, transport: transport, certFile: certFile, keyFile: keyFile,
		base: base, count: count, window: window,
		active:   map[int]net.Listener{},
		acceptCh: make(chan net.Conn, 16),
		stop:     make(chan struct{}),
	}
	hl.sync()
	go hl.loop()
	return hl
}

func (hl *hopListener) wantedPorts() map[int]bool {
	w := map[int]bool{}
	for _, off := range []int{-1, 0, 1} {
		w[hopPort(hl.key, hl.base, hl.count, hl.window, time.Now().Add(time.Duration(off)*hl.window))] = true
	}
	return w
}

func (hl *hopListener) sync() {
	want := hl.wantedPorts()
	hl.mu.Lock()
	defer hl.mu.Unlock()
	for p, ln := range hl.active {
		if !want[p] {
			ln.Close()
			delete(hl.active, p)
		}
	}
	for p := range want {
		if _, ok := hl.active[p]; ok {
			continue
		}
		addr := net.JoinHostPort(hl.host, strconv.Itoa(p))
		ln, err := listenControl(addr, hl.transport, hl.certFile, hl.keyFile, hl.key)
		if err != nil {
			log.Printf("hop: listen %s: %v", addr, err)
			continue
		}
		hl.active[p] = ln
		go hl.acceptLoop(ln)
	}
}

func (hl *hopListener) acceptLoop(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		select {
		case hl.acceptCh <- c:
		case <-hl.stop:
			c.Close()
			return
		}
	}
}

func (hl *hopListener) loop() {
	t := time.NewTicker(hl.window / 4)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			hl.sync()
		case <-hl.stop:
			hl.mu.Lock()
			for _, ln := range hl.active {
				ln.Close()
			}
			hl.mu.Unlock()
			return
		}
	}
}

func (hl *hopListener) Accept() (net.Conn, error) {
	select {
	case c := <-hl.acceptCh:
		return c, nil
	case <-hl.stop:
		return nil, fmt.Errorf("hop listener stopped")
	}
}

func (hl *hopListener) ports() []int {
	hl.mu.Lock()
	defer hl.mu.Unlock()
	var ps []int
	for p := range hl.active {
		ps = append(ps, p)
	}
	return ps
}
