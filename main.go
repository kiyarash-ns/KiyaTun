package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"time"
)

const version = "0.1.0-dev"

func usage() {
	fmt.Println(`tunx ` + version + `

  tunx keygen                                  generate a shared key

  tunx agent  -l :9000 -key KEY                run the link-test agent (both servers)
  tunx link   -peer IP:9000 -key KEY           test the link to a peer, per direction

  tunx server -l :9000 -expose :8080 -key KEY  entry side (e.g. Iran): exposes traffic
  tunx client -connect IP:9000 -to 127.0.0.1:80 -key KEY
                                                exit side (e.g. abroad): dials out, forwards

Run the agent on BOTH servers and run "link" from each side: filtering is
often asymmetric, so Iran->abroad and abroad->Iran must be measured separately.
The key can also be given via the TUNX_KEY environment variable.
-expose accepts a comma-separated list (e.g. :443,:8443) to spread across
several ports.
-transport tls wraps the control connection in a real TLS 1.3 handshake
(with -cert/-key-file on the server, -sni on the client) so it looks like
ordinary HTTPS to a passive observer. Must match on both sides.
-transport ws/wss additionally speaks WebSocket, so the connection can pass
through an HTTP(S)-only CDN (Arvan, Cloudflare, etc.) that doesn't proxy raw
TCP. Point the CDN's origin at the server's real IP, point -connect at the
CDN's domain, and set -sni / -ws-host to that domain.`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "keygen":
		fmt.Println(hex.EncodeToString(randBytes(24)))

	case "agent":
		fs := flag.NewFlagSet("agent", flag.ExitOnError)
		listen := fs.String("l", ":9000", "listen address (TCP and UDP)")
		key := fs.String("key", os.Getenv("TUNX_KEY"), "shared key")
		fs.Parse(os.Args[2:])
		if *key == "" {
			fmt.Println("missing -key (or TUNX_KEY)")
			os.Exit(2)
		}
		if err := runAgent(*listen, *key); err != nil {
			fmt.Println("agent:", err)
			os.Exit(1)
		}

	case "link":
		fs := flag.NewFlagSet("link", flag.ExitOnError)
		peer := fs.String("peer", "", "peer address host:port")
		key := fs.String("key", os.Getenv("TUNX_KEY"), "shared key")
		n := fs.Int("n", 100, "number of UDP probes")
		iv := fs.Duration("i", 100*time.Millisecond, "interval between probes")
		size := fs.Int("size", 128, "probe size in bytes (min 49)")
		speed := fs.Int("speed", 5, "seconds per direction for throughput test (0 = skip)")
		nomtu := fs.Bool("nomtu", false, "skip MTU sweep")
		fs.Parse(os.Args[2:])
		if *peer == "" || *key == "" {
			fmt.Println("need -peer and -key")
			os.Exit(2)
		}
		if *size < 49 {
			*size = 49
		}
		os.Exit(runLink(*peer, *key, linkOpts{
			count: *n, interval: *iv, size: *size, speedSec: *speed, noMTU: *nomtu,
		}))

	case "server":
		fs := flag.NewFlagSet("server", flag.ExitOnError)
		listen := fs.String("l", ":9000", "control listen address (clients dial in here)")
		expose := fs.String("expose", "", "address(es) end users connect to, comma-separated (e.g. :443,:8443)")
		key := fs.String("key", os.Getenv("TUNX_KEY"), "shared key")
		warmup := fs.Duration("warmup", 10*time.Minute, "duration to ramp up to full speed")
		warmMbps := fs.Float64("warm-mbps", 2, "starting rate during warm-up, in Mbit/s")
		targetMbps := fs.Float64("target-mbps", 0, "target rate after warm-up, in Mbit/s (0 = no limit, no warm-up)")
		transport := fs.String("transport", "raw", "control transport: raw, tls, ws or wss")
		cert := fs.String("cert", "", "TLS certificate file (transport=tls/wss; self-signed if omitted)")
		keyFile := fs.String("key-file", "", "TLS private key file (transport=tls/wss)")
		fs.Parse(os.Args[2:])
		if *expose == "" || *key == "" {
			fmt.Println("need -expose and -key")
			os.Exit(2)
		}
		if err := runTunnelServer(*listen, *expose, *key, *warmup, *warmMbps, *targetMbps, *transport, *cert, *keyFile); err != nil {
			fmt.Println("server:", err)
			os.Exit(1)
		}

	case "client":
		fs := flag.NewFlagSet("client", flag.ExitOnError)
		connect := fs.String("connect", "", "server control address to dial, e.g. IRAN_IP:9000")
		to := fs.String("to", "", "local address to forward accepted connections to, e.g. 127.0.0.1:443")
		key := fs.String("key", os.Getenv("TUNX_KEY"), "shared key")
		retry := fs.Duration("retry", 3*time.Second, "reconnect delay if the control connection drops")
		transport := fs.String("transport", "raw", "control transport: raw, tls, ws or wss (must match the server)")
		sni := fs.String("sni", "", "TLS server name to send (transport=tls/wss); the CDN-fronted domain, if using one")
		insecure := fs.Bool("insecure-tls", false, "skip TLS certificate verification (only for a self-signed server cert)")
		wsPath := fs.String("ws-path", "/", "HTTP path for the WebSocket upgrade (transport=ws/wss)")
		wsHost := fs.String("ws-host", "", "Host header for the WebSocket upgrade (transport=ws/wss); defaults to -sni, then to -connect")
		fs.Parse(os.Args[2:])
		if *connect == "" || *to == "" || *key == "" {
			fmt.Println("need -connect, -to and -key")
			os.Exit(2)
		}
		if err := runTunnelClient(*connect, *to, *key, *retry, *transport, *sni, *insecure, *wsPath, *wsHost); err != nil {
			fmt.Println("client:", err)
			os.Exit(1)
		}

	default:
		usage()
		os.Exit(2)
	}
}
