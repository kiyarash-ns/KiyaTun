cd ~/kiyatun
cat > README.md << 'EOF'
# KiyaTun

**A lightweight, zero-dependency reverse TCP tunnel written in pure Go — built for networks where the exit side can't expose any inbound port.**

[English](#english) | [فارسی](#فارسی)

---

## Important

Both servers need their shared key kept secret and (loosely) synced clocks
if you use port hopping. KiyaTun does not spoof IP addresses or touch raw
sockets — it's a straightforward encrypted TCP/UDP relay, not a Layer 3
tool. If what you actually need is IP-layer evasion, this is not that;
KiyaTun focuses on looking like ordinary encrypted traffic (TLS/WebSocket)
rather than forging packet headers.

---

## English

### What it does

KiyaTun connects two servers: an **entry** box with a stable public IP
(e.g. Iran) and an **exit** box running the real service (e.g. abroad).
The exit side dials **out** to the entry side — so it needs zero open
inbound ports. End users connect to the entry server; traffic rides the
tunnel to the exit, which forwards it to the real local service.

### Features

| | |
|---|---|
| 🩺 Link Doctor | Per-direction loss / RTT / jitter / MTU test, with a verdict, before you commit to a transport |
| 🕵️ No wire fingerprint | Wrong key → zero reply, behaves like a closed port |
| 🔀 Multiple transports | `raw`, `tls`, `ws`, `wss`, `udp` |
| 🌐 CDN-friendly | `ws`/`wss` pass through Arvan, Cloudflare, etc. |
| 🚪 Port hopping | Active port derived from `HMAC(key, time window)` — no coordination traffic |
| 🐢→🚀 Warm-up ramp | Gradual speed increase, not a sudden burst from a fresh IP |
| 📦 Zero dependencies | `go build .` and you're done |

### Architecture

| Role | Runs on | Behavior |
|---|---|---|
| `server` | entry (e.g. Iran) | listens for clients + end users |
| `client` | exit (e.g. abroad) | dials out, forwards to the local service |

### Transports

- **raw** — plain TCP, AES-256-GCM encrypted records, no disguise
- **tls** — same, wrapped in a real TLS 1.3 handshake
- **ws / wss** — WebSocket (optionally inside TLS), passes through an
  HTTP(S)-only CDN
- **udp** — custom reliable transport over UDP (fixed-window ARQ, encrypted)
  for paths where TCP specifically is throttled. *Not real QUIC* — no
  congestion control like BBR; said plainly rather than oversold.

### Quick install

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kiyarash-ns/KiyaTun/main/scripts/install.sh)
```

### Manual

```bash
git clone https://github.com/kiyarash-ns/KiyaTun.git && cd KiyaTun
go build -o tunx .
./tunx keygen

# entry (Iran):
./tunx server -l :9000 -expose :443 -key THE_KEY

# exit (abroad):
./tunx client -connect <ENTRY_IP>:9000 -to 127.0.0.1:8080 -key THE_KEY
```

Full flag reference, CDN setup, troubleshooting: see
[`docs/GUIDE.md`](docs/GUIDE.md).

### Screenshot

![install menu](docs/screenshot-install.png)

### License

MIT

---

## فارسی

### چیکار می‌کنه

KiyaTun دو سرور رو وصل می‌کنه: یه سرور **ورودی** با IP ثابت (مثلاً ایران) و
یه سرور **خروجی** که سرویس واقعی روشه (مثلاً خارج). سمت خروجی خودش به سمت
ورودی وصل می‌شه، پس هیچ پورت بازی لازم نداره. کاربر نهایی به سرور ورودی
وصل می‌شه، ترافیک از تونل رد می‌شه می‌ره سمت خروجی.

### ویژگی‌ها

- Link Doctor: تست لاس/پینگ/jitter/MTU جدا برای هر جهت
- بدون اثر انگشت: کلید غلط = سکوت کامل
- چند ترنسپورت: `raw`، `tls`، `ws`، `wss`، `udp`
- پشت CDN کار می‌کنه (آروان، کلودفلر، …)
- پورت‌هاپینگ واقعی، بدون وابستگی خارجی

### نصب سریع
```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kiyarash-ns/KiyaTun/main/scripts/install.sh)
```

راهنمای کامل (همه‌ی فلگ‌ها، CDN، رفع اشکال): [`docs/GUIDE.md`](docs/GUIDE.md)

### لایسنس
MIT
EOF

git add README.md
git commit -m "polished README"
git push
