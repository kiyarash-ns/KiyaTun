package main

// fixed-size AES-256-GCM records over TCP, authenticated padded packets over UDP

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"time"
)

const (
	recPlain  = 4096
	recHdr    = 3 // type(1) + len(2)
	recMax    = recPlain - recHdr
	recCipher = recPlain + 16 // GCM tag

	tHello  byte = 1
	tPing   byte = 2
	tReport byte = 3
	tUp     byte = 4
	tUpRes  byte = 5
	tDown   byte = 6
	tData   byte = 7
	tEnd    byte = 8

	// multiplexed tunnel frames (used by the server/client relay, not by
	// the agent/link link-test flow above)
	tOpen      byte = 9  // sid(4) + target label
	tOpenOK    byte = 10 // sid(4)
	tOpenErr   byte = 11 // sid(4) + reason text
	tSData     byte = 12 // sid(4) + payload
	tSClose    byte = 13 // sid(4)
	tSReset    byte = 14 // sid(4)
	tKeepAlive byte = 15
)

func deriveKey(secret, label string, salt []byte) []byte {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte("tunx/1/" + label))
	m.Write(salt)
	return m.Sum(nil)
}

func newAEAD(key []byte) cipher.AEAD {
	b, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		panic(err)
	}
	return g
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func randU64() uint64 {
	return binary.BigEndian.Uint64(randBytes(8))
}


type recConn struct {
	rw     io.ReadWriter
	rd, wr cipher.AEAD
	rn, wn uint64
	pt     []byte
	wbuf   []byte
	rbuf   []byte
	pbuf   []byte
}

func newRec(rw io.ReadWriter, secret string, salt []byte, server bool) *recConn {
	c2s := newAEAD(deriveKey(secret, "c2s", salt))
	s2c := newAEAD(deriveKey(secret, "s2c", salt))
	rc := &recConn{rw: rw}
	if server {
		rc.rd, rc.wr = c2s, s2c
	} else {
		rc.rd, rc.wr = s2c, c2s
	}
	return rc
}

func nonceFor(n uint64) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint64(b[4:], n)
	return b
}

func (c *recConn) send(typ byte, data []byte) error {
	if len(data) > recMax {
		return errors.New("record too large")
	}
	if c.pt == nil {
		c.pt = make([]byte, recPlain)
	}
	c.pt[0] = typ
	binary.BigEndian.PutUint16(c.pt[1:3], uint16(len(data)))
	copy(c.pt[recHdr:], data)
	for i := recHdr + len(data); i < recPlain; i++ {
		c.pt[i] = 0
	}
	c.wbuf = c.wr.Seal(c.wbuf[:0], nonceFor(c.wn), c.pt, nil)
	c.wn++
	_, err := c.rw.Write(c.wbuf)
	return err
}

// valid until next call
func (c *recConn) recv() (byte, []byte, error) {
	if c.rbuf == nil {
		c.rbuf = make([]byte, recCipher)
	}
	if _, err := io.ReadFull(c.rw, c.rbuf); err != nil {
		return 0, nil, err
	}
	pt, err := c.rd.Open(c.pbuf[:0], nonceFor(c.rn), c.rbuf, nil)
	if err != nil {
		return 0, nil, err
	}
	c.pbuf = pt
	c.rn++
	n := int(binary.BigEndian.Uint16(pt[1:3]))
	if n > recMax {
		return 0, nil, errors.New("bad record length")
	}
	return pt[0], pt[recHdr : recHdr+n], nil
}


const udpMinPlain = 21

type udpPkt struct {
	sid uint64
	seq uint32
	ts  int64 // unix seconds, only used to reject stale replays
	typ byte  // 1 = probe, 2 = echo
}

func newUDPAEAD(secret string) cipher.AEAD {
	return newAEAD(deriveKey(secret, "udp", nil))
}

func udpSeal(a cipher.AEAD, p udpPkt, total int) []byte {
	ns := a.NonceSize()
	plen := total - ns - a.Overhead()
	if plen < udpMinPlain {
		plen = udpMinPlain
	}
	pt := make([]byte, plen)
	binary.BigEndian.PutUint64(pt[0:], p.sid)
	binary.BigEndian.PutUint32(pt[8:], p.seq)
	binary.BigEndian.PutUint64(pt[12:], uint64(p.ts))
	pt[20] = p.typ
	if plen > udpMinPlain {
		copy(pt[udpMinPlain:], randBytes(plen-udpMinPlain))
	}
	nonce := randBytes(ns)
	out := make([]byte, 0, ns+plen+a.Overhead())
	out = append(out, nonce...)
	return a.Seal(out, nonce, pt, nil)
}

func udpOpen(a cipher.AEAD, b []byte) (udpPkt, bool) {
	ns := a.NonceSize()
	if len(b) < ns+a.Overhead()+udpMinPlain {
		return udpPkt{}, false
	}
	pt, err := a.Open(nil, b[:ns], b[ns:], nil)
	if err != nil || len(pt) < udpMinPlain {
		return udpPkt{}, false
	}
	p := udpPkt{
		sid: binary.BigEndian.Uint64(pt[0:]),
		seq: binary.BigEndian.Uint32(pt[8:]),
		ts:  int64(binary.BigEndian.Uint64(pt[12:])),
		typ: pt[20],
	}
	d := time.Now().Unix() - p.ts
	if d > 60 || d < -60 {
		return udpPkt{}, false
	}
	return p, true
}
