package main

import (
	"bufio"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	wsGUID       = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	wsOpContinue = 0x0
	wsOpText     = 0x1
	wsOpBinary   = 0x2
	wsOpClose    = 0x8
	wsOpPing     = 0x9
	wsOpPong     = 0xA
	wsMaxFrame   = 16 * 1024
	wsMaxPayload = 1 << 20 // sanity cap against a hostile/garbage peer
)

func wsAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func writeWSFrame(w io.Writer, opcode byte, payload []byte, mask bool) error {
	l := len(payload)
	var hdr []byte
	b0 := 0x80 | opcode // FIN=1
	switch {
	case l < 126:
		hdr = []byte{b0, byte(l)}
	case l <= 0xFFFF:
		hdr = []byte{b0, 126, byte(l >> 8), byte(l)}
	default:
		hdr = make([]byte, 10)
		hdr[0] = b0
		hdr[1] = 127
		binary.BigEndian.PutUint64(hdr[2:], uint64(l))
	}
	if !mask {
		if _, err := w.Write(hdr); err != nil {
			return err
		}
		_, err := w.Write(payload)
		return err
	}
	hdr[1] |= 0x80
	key := randBytes(4)
	out := make([]byte, 0, len(hdr)+4+l)
	out = append(out, hdr...)
	out = append(out, key...)
	masked := make([]byte, l)
	for i := 0; i < l; i++ {
		masked[i] = payload[i] ^ key[i%4]
	}
	out = append(out, masked...)
	_, err := w.Write(out)
	return err
}

func readWSFrame(r *bufio.Reader) (byte, []byte, error) {
	b0, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	b1, err := r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	opcode := b0 & 0x0F
	masked := b1&0x80 != 0
	l := int(b1 & 0x7F)
	switch l {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		l = int(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		l = int(binary.BigEndian.Uint64(b[:]))
	}
	if l > wsMaxPayload {
		return 0, nil, errors.New("ws frame too large")
	}
	var key [4]byte
	if masked {
		if _, err := io.ReadFull(r, key[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, l)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= key[i%4]
		}
	}
	return opcode, payload, nil
}

type wsConn struct {
	net.Conn
	br       *bufio.Reader
	isClient bool
	rbuf     []byte
}

func (w *wsConn) Read(p []byte) (int, error) {
	for len(w.rbuf) == 0 {
		op, payload, err := readWSFrame(w.br)
		if err != nil {
			return 0, err
		}
		switch op {
		case wsOpPing:
			_ = writeWSFrame(w.Conn, wsOpPong, payload, w.isClient)
			continue
		case wsOpPong:
			continue
		case wsOpClose:
			return 0, io.EOF
		case wsOpBinary, wsOpText, wsOpContinue:
			if len(payload) == 0 {
				continue
			}
			w.rbuf = payload
		default:
			continue
		}
	}
	n := copy(p, w.rbuf)
	w.rbuf = w.rbuf[n:]
	return n, nil
}

func (w *wsConn) Write(p []byte) (int, error) {
	total := 0
	for len(p) > 0 {
		n := len(p)
		if n > wsMaxFrame {
			n = wsMaxFrame
		}
		if err := writeWSFrame(w.Conn, wsOpBinary, p[:n], w.isClient); err != nil {
			return total, err
		}
		total += n
		p = p[n:]
	}
	return total, nil
}

func (w *wsConn) Close() error {
	_ = writeWSFrame(w.Conn, wsOpClose, nil, w.isClient)
	return w.Conn.Close()
}

func dialWS(addr, path, host string, tlsCfg *tls.Config, timeout time.Duration) (net.Conn, error) {
	var conn net.Conn
	var err error
	if tlsCfg != nil {
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", addr, tlsCfg)
	} else {
		conn, err = net.DialTimeout("tcp", addr, timeout)
	}
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(timeout))

	if path == "" {
		path = "/"
	}
	if host == "" {
		host = addr
	}
	wsKey := base64.StdEncoding.EncodeToString(randBytes(16))
	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + wsKey + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64)\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, err
	}

	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, err
	}
	if !strings.Contains(status, "101") {
		conn.Close()
		return nil, fmt.Errorf("websocket upgrade failed: %s", strings.TrimSpace(status))
	}
	accept := ""
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if i := strings.Index(line, ":"); i > 0 {
			k := strings.TrimSpace(line[:i])
			v := strings.TrimSpace(line[i+1:])
			if strings.EqualFold(k, "Sec-WebSocket-Accept") {
				accept = v
			}
		}
	}
	if accept == "" || accept != wsAcceptKey(wsKey) {
		conn.Close()
		return nil, errors.New("websocket handshake: bad accept key")
	}
	conn.SetDeadline(time.Time{})
	return &wsConn{Conn: conn, br: br, isClient: true}, nil
}

func acceptWS(c net.Conn) (net.Conn, error) {
	c.SetDeadline(time.Now().Add(8 * time.Second))
	br := bufio.NewReader(c)
	if _, err := br.ReadString('\n'); err != nil { // request line, unused
		return nil, err
	}
	headers := map[string]string{}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if i := strings.Index(line, ":"); i > 0 {
			headers[strings.ToLower(strings.TrimSpace(line[:i]))] = strings.TrimSpace(line[i+1:])
		}
	}
	key := headers["sec-websocket-key"]
	if key == "" || !strings.Contains(strings.ToLower(headers["upgrade"]), "websocket") {
		_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 2\r\n\r\nOK"))
		return nil, errors.New("not a websocket upgrade")
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + wsAcceptKey(key) + "\r\n\r\n"
	if _, err := c.Write([]byte(resp)); err != nil {
		return nil, err
	}
	c.SetDeadline(time.Time{})
	return &wsConn{Conn: c, br: br, isClient: false}, nil
}

type wsListener struct{ net.Listener }

func (l *wsListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		wc, err := acceptWS(c)
		if err != nil {
			c.Close()
			continue
		}
		return wc, nil
	}
}
