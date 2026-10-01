# KiyaTun

**KiyaTun** is a lightweight reverse TCP tunnel written in pure Go with zero external dependencies.

It is designed for restricted networks (especially Iran <-> abroad) where the exit server should not expose any inbound ports.

## Features

- Reverse tunnel architecture (abroad side dials out)
- AES-256-GCM encryption with fixed-size records
- No magic numbers, banners, or plaintext length fields
- Silent drop on wrong key (anti active-probing)
- Multiplexing (multiple streams over one control connection)
- Transports: `raw`, `tls`, `ws`, `wss`
- CDN support via WebSocket (Arvan, Cloudflare, ...)
- Built-in Link Doctor (RTT, loss, jitter, MTU, throughput)
- Gradual speed warm-up

## Architecture

| Role | Location | Behavior |
|------|----------|----------|
| **Server (Entry)** | Iran | Listens on control port + exposed ports |
| **Client (Exit)** | Abroad | Dials out to Iran, forwards traffic locally |

The abroad server needs **no open inbound ports**.

## Quick Start

### Build

```bash
go build -o tunx .
./tunx keygen
```

### Run Server (Iran)

```bash
./tunx server -l :9000 -expose :443 -key YOUR_KEY
```

### Run Client (Abroad)

```bash
./tunx client -connect IRAN_IP:9000 -to 127.0.0.1:443 -key YOUR_KEY
```

## فارسی

**KiyaTun** یک تانل معکوس TCP سبک است که با Go خالص نوشته شده و هیچ وابستگی خارجی ندارد.

### ویژگی‌ها

- تانل معکوس (سرور خارج پورت باز نمی‌خواهد)
- رمزنگاری AES-256-GCM
- بدون اثر انگشت روی شبکه
- پشتیبانی از TLS و WebSocket (عبور از CDN)
- ابزار تست لینک (پینگ، لاس، MTU، سرعت)

## License

MIT
