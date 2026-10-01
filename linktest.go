package main

import (
	"crypto/cipher"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type linkOpts struct {
	count    int
	interval time.Duration
	size     int
	speedSec int
	noMTU    bool
}

var mtuSizes = []int{1000, 1200, 1350, 1400, 1450, 1472}

const mtuProbes = 5


func col(code, s string) string {
	if os.Getenv("NO_COLOR") != "" {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}
func green(s string) string  { return col("32", s) }
func yellow(s string) string { return col("33", s) }
func red(s string) string    { return col("31", s) }
func bold(s string) string   { return col("1", s) }
func dim(s string) string    { return col("2", s) }

func ms(d time.Duration) string { return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000) }

func lossColor(p float64) string {
	s := fmt.Sprintf("%.1f%%", p)
	switch {
	case p < 1:
		return green(s)
	case p < 5:
		return yellow(s)
	}
	return red(s)
}


type probeRec struct {
	sent time.Time
	size int
}

type udpTest struct {
	uc   *net.UDPConn
	aead cipher.AEAD
	sid  uint64
	mu   sync.Mutex
	seq  uint32
	sent map[uint32]probeRec
	rtt  map[uint32]time.Duration
}

func (u *udpTest) probe(size int) uint32 {
	u.mu.Lock()
	u.seq++
	s := u.seq
	u.sent[s] = probeRec{time.Now(), size}
	u.mu.Unlock()
	pkt := udpSeal(u.aead, udpPkt{sid: u.sid, seq: s, ts: time.Now().Unix(), typ: 1}, size)
	u.uc.Write(pkt)
	return s
}

func (u *udpTest) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := u.uc.Read(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		p, ok := udpOpen(u.aead, buf[:n])
		if !ok || p.typ != 2 || p.sid != u.sid {
			continue
		}
		u.mu.Lock()
		if r, ok := u.sent[p.seq]; ok {
			if _, dup := u.rtt[p.seq]; !dup {
				u.rtt[p.seq] = time.Since(r.sent)
			}
		}
		u.mu.Unlock()
	}
}

func (u *udpTest) rtts(lo, hi uint32) ([]time.Duration, int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	var out []time.Duration
	sent := 0
	for s := lo; s <= hi; s++ {
		if _, ok := u.sent[s]; ok {
			sent++
		}
		if r, ok := u.rtt[s]; ok {
			out = append(out, r)
		}
	}
	return out, sent
}

func stats(v []time.Duration) (mn, avg, mx, jit time.Duration) {
	if len(v) == 0 {
		return
	}
	mn = v[0]
	var sum time.Duration
	for _, d := range v {
		if d < mn {
			mn = d
		}
		if d > mx {
			mx = d
		}
		sum += d
	}
	avg = sum / time.Duration(len(v))
	if len(v) > 1 {
		var j float64
		for i := 1; i < len(v); i++ {
			j += math.Abs(float64(v[i] - v[i-1]))
		}
		jit = time.Duration(j / float64(len(v)-1))
	}
	return
}

func pct(part, whole int) float64 {
	if whole <= 0 {
		return 0
	}
	return 100 * float64(whole-part) / float64(whole)
}


func clientHandshake(c net.Conn, secret string) (*recConn, error) {
	c.SetDeadline(time.Now().Add(8 * time.Second))
	salt := randBytes(16)
	if _, err := c.Write(salt); err != nil {
		return nil, err
	}
	rc := newRec(c, secret, salt, false)
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(time.Now().Unix()))
	if err := rc.send(tHello, ts); err != nil {
		return nil, err
	}
	t, _, err := rc.recv()
	if err != nil {
		return nil, err
	}
	if t != tHello {
		return nil, errors.New("unexpected reply")
	}
	c.SetDeadline(time.Time{})
	return rc, nil
}

func classify(err error) string {
	var ne net.Error
	switch {
	case strings.Contains(err.Error(), "refused"):
		return "connection refused (port closed, or agent not running)"
	case errors.As(err, &ne) && ne.Timeout():
		return "timeout (packets silently dropped: firewall / filtering / wrong IP)"
	case strings.Contains(err.Error(), "reset"):
		return "connection reset (possible active interference or wrong key)"
	case strings.Contains(err.Error(), "EOF"):
		return "closed by peer after handshake (wrong key, or interference)"
	}
	return err.Error()
}

type tcpResult struct {
	ok       bool
	connect  time.Duration
	rtts     []time.Duration
	upMbps   float64
	downMbps float64
}

func testTCP(peer, secret string, o linkOpts) (*recConn, net.Conn, tcpResult) {
	var r tcpResult
	t0 := time.Now()
	c, err := net.DialTimeout("tcp", peer, 6*time.Second)
	if err != nil {
		fmt.Printf("  TCP   %s  %s\n", red("FAIL"), classify(err))
		return nil, nil, r
	}
	r.connect = time.Since(t0)
	rc, err := clientHandshake(c, secret)
	if err != nil {
		fmt.Printf("  TCP   %s  connect ok (%s) but handshake failed: %s\n", red("FAIL"), ms(r.connect), classify(err))
		c.Close()
		return nil, nil, r
	}
	r.ok = true
	for i := 0; i < 10; i++ {
		ts := make([]byte, 8)
		binary.BigEndian.PutUint64(ts, uint64(time.Now().UnixNano()))
		t1 := time.Now()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		if rc.send(tPing, ts) != nil {
			break
		}
		if t, _, err := rc.recv(); err != nil || t != tPing {
			break
		}
		r.rtts = append(r.rtts, time.Since(t1))
		time.Sleep(30 * time.Millisecond)
	}
	c.SetDeadline(time.Time{})
	mn, avg, mx, jit := stats(r.rtts)
	fmt.Printf("  TCP   %s  connect %s | rtt min/avg/max %s / %s / %s | jitter %s\n",
		green("OK  "), ms(r.connect), ms(mn), ms(avg), ms(mx), ms(jit))
	return rc, c, r
}

func speedUp(c net.Conn, rc *recConn, secs int) float64 {
	filler := randBytes(recMax)
	if rc.send(tUp, nil) != nil {
		return 0
	}
	end := time.Now().Add(time.Duration(secs) * time.Second)
	for time.Now().Before(end) {
		c.SetDeadline(time.Now().Add(20 * time.Second))
		if rc.send(tData, filler) != nil {
			return 0
		}
	}
	c.SetDeadline(time.Now().Add(30 * time.Second))
	if rc.send(tEnd, nil) != nil {
		return 0
	}
	t, d, err := rc.recv()
	if err != nil || t != tUpRes || len(d) < 16 {
		return 0
	}
	recs := binary.BigEndian.Uint64(d[0:])
	el := time.Duration(binary.BigEndian.Uint64(d[8:]))
	if el <= 0 {
		return 0
	}
	return float64(recs) * recCipher * 8 / el.Seconds() / 1e6
}

func speedDown(c net.Conn, rc *recConn, secs int) float64 {
	d := make([]byte, 4)
	binary.BigEndian.PutUint32(d, uint32(secs*1000))
	if rc.send(tDown, d) != nil {
		return 0
	}
	var recs uint64
	var first time.Time
	for {
		c.SetDeadline(time.Now().Add(30 * time.Second))
		t, _, err := rc.recv()
		if err != nil {
			return 0
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
		return 0
	}
	if first.IsZero() {
		return 0
	}
	el := time.Since(first)
	if el <= 0 {
		return 0
	}
	return float64(recs) * recCipher * 8 / el.Seconds() / 1e6
}


func runLink(peer, secret string, o linkOpts) int {
	raddr, err := net.ResolveUDPAddr("udp", peer)
	if err != nil {
		fmt.Println("bad peer address:", err)
		return 2
	}
	fmt.Println(bold("tunx link doctor"), "->", peer)
	fmt.Println(dim("  (run the same command from the other server too: filtering is often asymmetric)"))
	fmt.Println()

	rc, tc, tr := testTCP(peer, secret, o)
	if tc != nil {
		defer tc.Close()
	}

	uc, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		fmt.Println("udp:", err)
		return 2
	}
	defer uc.Close()
	u := &udpTest{
		uc: uc, aead: newUDPAEAD(secret), sid: randU64(),
		sent: map[uint32]probeRec{}, rtt: map[uint32]time.Duration{},
	}
	go u.readLoop()

	// --- baseline probes (live line)
	fmt.Printf("  UDP   sending %d probes of %d bytes every %s\n", o.count, o.size, o.interval)
	for i := 0; i < o.count; i++ {
		s := u.probe(o.size)
		r, _ := u.rtts(1, s)
		last := "-"
		if len(r) > 0 {
			last = ms(r[len(r)-1])
		}
		fmt.Printf("\r        sent %d/%d  replies %d  last rtt %s   ", s, o.count, len(r), last)
		time.Sleep(o.interval)
	}
	time.Sleep(1500 * time.Millisecond)
	fmt.Print("\r" + strings.Repeat(" ", 70) + "\r")

	baseRTT, baseSent := u.rtts(1, uint32(o.count))
	echoes := len(baseRTT)

	// --- MTU sweep
	type sizeRes struct {
		size, echoed int
		lo, hi       uint32
	}
	var sweep []sizeRes
	if !o.noMTU {
		for _, sz := range mtuSizes {
			lo := u.seq + 1
			for i := 0; i < mtuProbes; i++ {
				u.probe(sz)
				time.Sleep(40 * time.Millisecond)
			}
			sweep = append(sweep, sizeRes{size: sz, lo: lo, hi: u.seq})
		}
		time.Sleep(1200 * time.Millisecond)
		// count echoes only after the final grace period
		for i := range sweep {
			r, _ := u.rtts(sweep[i].lo, sweep[i].hi)
			sweep[i].echoed = len(r)
		}
	}

	// --- server-side view (forward direction)
	var rep *report
	if rc != nil {
		tc.SetDeadline(time.Now().Add(6 * time.Second))
		sid := make([]byte, 8)
		binary.BigEndian.PutUint64(sid, u.sid)
		if rc.send(tReport, sid) == nil {
			if t, d, err := rc.recv(); err == nil && t == tReport {
				var r report
				if json.Unmarshal(d, &r) == nil {
					rep = &r
				}
			}
		}
		tc.SetDeadline(time.Time{})
	}

	mn, avg, mx, jit := stats(baseRTT)
	var fwdLoss, retLoss float64 = -1, -1
	if echoes == 0 && baseSent > 0 && rep == nil {
		fmt.Printf("  UDP   %s  no authenticated reply at all\n", red("FAIL"))
	}
	if rep != nil {
		got := rep.BySize[o.size]
		fwdLoss = pct(got, baseSent)
		retLoss = pct(echoes, got)
		if got == 0 {
			retLoss = -1
		}
	}
	if baseSent > 0 {
		fmt.Printf("  UDP   sent %d  echoed %d  round-trip loss %s\n", baseSent, echoes, lossColor(pct(echoes, baseSent)))
		if echoes > 0 {
			fmt.Printf("        rtt min/avg/max %s / %s / %s | jitter %s\n", ms(mn), ms(avg), ms(mx), ms(jit))
		}
		if fwdLoss >= 0 {
			fmt.Printf("        %s  this host -> peer loss  %s   (peer received %d/%d)\n", bold("=>"), lossColor(fwdLoss), rep.BySize[o.size], baseSent)
		} else if rc == nil {
			fmt.Printf("        %s  per-direction split unavailable (TCP control channel failed)\n", dim("--"))
		}
		if retLoss >= 0 {
			fmt.Printf("        %s  peer -> this host loss  %s\n", bold("<="), lossColor(retLoss))
		}
	}

	// --- MTU table
	maxRT, maxFwd := 0, 0
	if len(sweep) > 0 {
		fmt.Println()
		fmt.Printf("  MTU   %-10s %-14s %s\n", "payload", "round-trip", "this->peer")
		for _, s := range sweep {
			fwd := "n/a"
			if rep != nil {
				n := rep.BySize[s.size]
				fwd = fmt.Sprintf("%d/%d", n, mtuProbes)
				if n >= 3 && s.size > maxFwd {
					maxFwd = s.size
				}
			}
			mark := red("x")
			if s.echoed >= 3 {
				mark = green("ok")
				if s.size > maxRT {
					maxRT = s.size
				}
			}
			fmt.Printf("        %-10d %-3s (%d/%d)     %s\n", s.size, mark, s.echoed, mtuProbes, fwd)
		}
		if maxRT > 0 {
			fmt.Printf("        largest UDP payload that round-trips: %d  (path MTU about %d)\n", maxRT, maxRT+28)
		}
		if rep != nil && maxFwd > 0 && maxFwd != maxRT {
			fmt.Printf("        largest UDP payload this->peer: %d  (directions differ: MTU is asymmetric)\n", maxFwd)
		}
	}

	// --- throughput
	if rc != nil && o.speedSec > 0 {
		fmt.Println()
		fmt.Printf("  SPEED measuring %ds each way over the tunnel-style channel...\n", o.speedSec)
		tr.upMbps = speedUp(tc, rc, o.speedSec)
		fmt.Printf("        this -> peer  %s\n", mbps(tr.upMbps))
		tr.downMbps = speedDown(tc, rc, o.speedSec)
		fmt.Printf("        peer -> this  %s\n", mbps(tr.downMbps))
	}

	// --- verdict
	fmt.Println()
	return verdict(tr, echoes, baseSent, fwdLoss, retLoss, avg, jit, maxRT)
}

func mbps(v float64) string {
	if v <= 0 {
		return red("failed")
	}
	s := fmt.Sprintf("%.1f Mbit/s", v)
	switch {
	case v >= 50:
		return green(s)
	case v >= 10:
		return yellow(s)
	}
	return red(s)
}

func verdict(tr tcpResult, echoes, sent int, fwd, ret float64, avg, jit time.Duration, maxRT int) int {
	score := 100.0
	udpOK := echoes > 0
	rtLoss := pct(echoes, sent)
	var notes []string

	if !udpOK {
		score -= 40
	} else {
		score -= rtLoss * 3
		if avg > 250*time.Millisecond {
			score -= 10
			notes = append(notes, "high latency")
		}
		if jit > 30*time.Millisecond {
			score -= 10
			notes = append(notes, "unstable latency (jitter)")
		}
	}
	if !tr.ok {
		score -= 30
	}
	if tr.ok && tr.upMbps > 0 && tr.upMbps < 5 {
		score -= 10
		notes = append(notes, "low upload throughput")
	}
	if tr.ok && tr.downMbps > 0 && tr.downMbps < 5 {
		score -= 10
		notes = append(notes, "low download throughput")
	}
	if fwd >= 0 && ret >= 0 && math.Abs(fwd-ret) > 5 {
		notes = append(notes, fmt.Sprintf("asymmetric loss (%.1f%% vs %.1f%%): one direction is being degraded", fwd, ret))
		score -= 10
	}
	if score < 0 {
		score = 0
	}

	var rec string
	switch {
	case !tr.ok && !udpOK:
		rec = "peer unreachable on this port over both TCP and UDP. Try another port, check the firewall, or run the test from the other side."
	case !udpOK:
		rec = "UDP is blocked or dropped on this path: use a TCP-based transport (TLS/WSS)."
	case rtLoss > 10 && tr.ok:
		rec = "UDP path is lossy: prefer a TCP-based transport, or a UDP one with FEC/retransmit and conservative rate."
	case !tr.ok:
		rec = "UDP works but TCP to this port fails: use a UDP/QUIC transport, or pick a different TCP port."
	default:
		rec = "both TCP and UDP look healthy: QUIC/UDP transport is fine, TCP fallback available."
	}
	if maxRT > 0 && maxRT < 1350 {
		notes = append(notes, fmt.Sprintf("small path MTU: set tunnel MTU to about %d", maxRT+28-60))
	}

	label := green("GOOD")
	code := 0
	switch {
	case score < 50:
		label, code = red("POOR"), 1
	case score < 80:
		label, code = yellow("FAIR"), 0
	}
	fmt.Printf("  %s  link score %.0f/100  [%s]\n", bold("VERDICT"), score, label)
	fmt.Printf("  %s  %s\n", bold("RECOMMEND"), rec)
	for _, n := range notes {
		fmt.Printf("  %s  %s\n", yellow("note"), n)
	}
	return code
}
