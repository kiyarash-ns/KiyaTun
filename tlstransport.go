package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"time"
)

func dialControl(addr, transport, sni string, insecure bool, timeout time.Duration, wsPath, wsHost, key string) (net.Conn, error) {
	switch transport {
	case "", "raw":
		return net.DialTimeout("tcp", addr, timeout)
	case "tls":
		d := &net.Dialer{Timeout: timeout}
		cfg := &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: insecure,
			MinVersion:         tls.VersionTLS13,
		}
		return tls.DialWithDialer(d, "tcp", addr, cfg)
	case "ws":
		host := wsHost
		if host == "" {
			host = addr
		}
		return dialWS(addr, wsPath, host, nil, timeout)
	case "wss":
		host := wsHost
		if host == "" {
			host = sni
		}
		if host == "" {
			host = addr
		}
		cfg := &tls.Config{ServerName: sni, InsecureSkipVerify: insecure, MinVersion: tls.VersionTLS12}
		return dialWS(addr, wsPath, host, cfg, timeout)
	case "udp":
		return dialUDPConn(addr, key, timeout)
	default:
		return nil, fmt.Errorf("unknown transport %q (use raw, tls, ws, wss or udp)", transport)
	}
}

func listenControl(addr, transport, certFile, keyFile, key string) (net.Listener, error) {
	switch transport {
	case "udp":
		return listenUDPConn(addr, key)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	switch transport {
	case "", "raw":
		return ln, nil
	case "tls":
		cert, err := loadOrSelfSignCert(certFile, keyFile)
		if err != nil {
			ln.Close()
			return nil, err
		}
		cfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}
		return tls.NewListener(ln, cfg), nil
	case "ws":
		return &wsListener{Listener: ln}, nil
	case "wss":
		cert, err := loadOrSelfSignCert(certFile, keyFile)
		if err != nil {
			ln.Close()
			return nil, err
		}
		cfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		return &wsListener{Listener: tls.NewListener(ln, cfg)}, nil
	default:
		ln.Close()
		return nil, fmt.Errorf("unknown transport %q (use raw, tls, ws, wss or udp)", transport)
	}
}

func loadOrSelfSignCert(certFile, keyFile string) (tls.Certificate, error) {
	if certFile != "" && keyFile != "" {
		return tls.LoadX509KeyPair(certFile, keyFile)
	}
	fmt.Println("warning: no -cert/-key-file given, using a self-signed certificate.")
	fmt.Println("         payload is still protected, but a self-signed cert is itself")
	fmt.Println("         a signal to cert-inspecting DPI. For real HTTPS cover, use a")
	fmt.Println("         certificate from a public CA (e.g. certbot) and pass a matching")
	fmt.Println("         -sni on the client.")
	return genSelfSigned()
}

func genSelfSigned() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}, nil
}
