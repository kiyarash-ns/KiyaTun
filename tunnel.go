package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

type tunnelServer struct {
	key string

	mu       sync.Mutex
	sessions []*Session
	rr       int

	limiter *rampLimiter
}

func runTunnelServer(listen, expose, key string, warmup time.Duration, warmMbps, targetMbps float64, transport, certFile, keyFile string, hop bool, hopBase, hopCount int, hopWindow time.Duration) error {
	ts := &tunnelServer{key: key}
	if targetMbps > 0 {
		ts.limiter = newRampLimiter(warmMbps, targetMbps, warmup)
	}

	var acceptor interface {
		Accept() (net.Conn, error)
	}
	if hop {
		host, _, err := net.SplitHostPort(listen)
		if err != nil {
			host = ""
		}
		hl := newHopListener(host, key, transport, certFile, keyFile, hopBase, hopCount, hopWindow)
		acceptor = hl
		log.Printf("control: port hopping enabled (base=%d count=%d window=%s)", hopBase, hopCount, hopWindow)
	} else {
		ln, err := listenControl(listen, transport, certFile, keyFile, key)
		if err != nil {
			return fmt.Errorf("control listen %s: %w", listen, err)
		}
		acceptor = ln
		log.Printf("control: listening on %s (%s) for clients", listen, transportLabel(transport))
	}
	go func() {
		for {
			c, err := acceptor.Accept()
			if err != nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			go ts.acceptClient(c)
		}
	}()

	addrs := splitList(expose)
	if len(addrs) == 0 {
		return fmt.Errorf("no -expose address given")
	}
	for _, addr := range addrs {
		eln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("expose listen %s: %w", addr, err)
		}
		log.Printf("expose: listening on %s for end users", addr)
		go ts.serveExposed(eln)
	}
	if ts.limiter != nil {
		log.Printf("warm-up: ramping %.1f -> %.1f Mbit/s over %s", warmMbps, targetMbps, warmup)
	}
	select {}
}

func (ts *tunnelServer) acceptClient(c net.Conn) {
	rc, err := serverHandshake(c, ts.key)
	if err != nil {
		// wrong key or a probe: no reply, hold briefly like a dead port, drop.
		time.Sleep(time.Duration(300+rand.Intn(1700)) * time.Millisecond)
		c.Close()
		return
	}
	sess := newSession(rc, true)
	go sess.keepAlive(20 * time.Second)

	ts.mu.Lock()
	ts.sessions = append(ts.sessions, sess)
	ts.mu.Unlock()
	log.Printf("client connected: %s", c.RemoteAddr())

	<-sess.Done()
	c.Close()

	ts.mu.Lock()
	for i, s := range ts.sessions {
		if s == sess {
			ts.sessions = append(ts.sessions[:i], ts.sessions[i+1:]...)
			break
		}
	}
	ts.mu.Unlock()
	log.Printf("client disconnected: %s", c.RemoteAddr())
}

func (ts *tunnelServer) pick() *Session {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.sessions) == 0 {
		return nil
	}
	ts.rr = (ts.rr + 1) % len(ts.sessions)
	return ts.sessions[ts.rr]
}

func (ts *tunnelServer) serveExposed(ln net.Listener) {
	for {
		uc, err := ln.Accept()
		if err != nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		go ts.handleUser(uc)
	}
}

func (ts *tunnelServer) handleUser(uc net.Conn) {
	defer uc.Close()
	sess := ts.pick()
	if sess == nil {
		return // no client attached: drop silently, nothing to fingerprint
	}
	st, err := sess.Open("")
	if err != nil {
		return
	}
	defer st.Close()
	relay(uc, st, ts.limiter)
}

func runTunnelClient(connect, to, key string, retry time.Duration, transport, sni string, insecureTLS bool, wsPath, wsHost string, hop bool, hopBase, hopCount int, hopWindow time.Duration) error {
	for {
		dialAddr := connect
		if hop {
			a, err := resolveHopAddr(connect, key, hopBase, hopCount, hopWindow, 0)
			if err != nil {
				log.Printf("hop address resolve failed: %v", err)
				time.Sleep(retry)
				continue
			}
			dialAddr = a
		}
		if err := oneClientSession(dialAddr, to, key, transport, sni, insecureTLS, wsPath, wsHost); err != nil {
			log.Printf("session ended: %v (retrying in %s)", err, retry)
		}
		time.Sleep(retry)
	}
}

func oneClientSession(connect, to, key, transport, sni string, insecureTLS bool, wsPath, wsHost string) error {
	c, err := dialControl(connect, transport, sni, insecureTLS, 8*time.Second, wsPath, wsHost, key)
	if err != nil {
		return err
	}
	rc, err := clientHandshake(c, key)
	if err != nil {
		c.Close()
		return err
	}
	sess := newSession(rc, false)
	go sess.keepAlive(20 * time.Second)
	log.Printf("connected to %s (%s), forwarding to %s", connect, transportLabel(transport), to)

	for {
		st, err := sess.Accept()
		if err != nil {
			c.Close()
			return err
		}
		go func(st *Stream) {
			lc, err := net.DialTimeout("tcp", to, 6*time.Second)
			if err != nil {
				st.Reject(err.Error())
				return
			}
			if err := st.Accept(); err != nil {
				lc.Close()
				return
			}
			defer lc.Close()
			relay(lc, st, nil)
		}(st)
	}
}

func relay(a net.Conn, b io.ReadWriteCloser, limiter *rampLimiter) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		var dst io.Writer = a
		if limiter != nil {
			dst = &limitedWriter{w: a, l: limiter}
		}
		io.Copy(dst, b)
		a.Close()
	}()
	go func() {
		defer wg.Done()
		var dst io.Writer = b
		if limiter != nil {
			dst = &limitedWriter{w: b, l: limiter}
		}
		io.Copy(dst, a)
		b.Close()
	}()
	wg.Wait()
}

func transportLabel(t string) string {
	switch t {
	case "tls", "ws", "wss", "udp":
		return t
	}
	return "raw"
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func serverHandshake(c net.Conn, secret string) (*recConn, error) {
	c.SetDeadline(time.Now().Add(8 * time.Second))
	salt := make([]byte, 16)
	if _, err := io.ReadFull(c, salt); err != nil {
		return nil, err
	}
	rc := newRec(c, secret, salt, true)
	typ, data, err := rc.recv()
	if err != nil || typ != tHello || len(data) < 8 {
		return nil, fmt.Errorf("bad hello")
	}
	d := time.Now().Unix() - int64(binary.BigEndian.Uint64(data))
	if d > 60 || d < -60 {
		return nil, fmt.Errorf("clock skew too large")
	}
	ack := make([]byte, 8)
	binary.BigEndian.PutUint64(ack, uint64(time.Now().Unix()))
	if err := rc.send(tHello, ack); err != nil {
		return nil, err
	}
	c.SetDeadline(time.Time{})
	return rc, nil
}
