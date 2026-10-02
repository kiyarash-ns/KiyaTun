<div align="center">

# KiyaTun

**یه تانل TCP معکوس، سریع و ضدشناسایی — نوشته‌شده با Go، بدون هیچ وابستگی بیرونی**
**A fast, hard-to-fingerprint reverse TCP tunnel — pure Go, zero external dependencies**

![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-linux-lightgrey)
![License](https://img.shields.io/badge/license-MIT-green)
![Transports](https://img.shields.io/badge/transports-raw%20%7C%20tls%20%7C%20ws%20%7C%20wss%20%7C%20udp-blue)

[فارسی](#--فارسی) | [English](#--english)

</div>

---

## 🇮🇷 فارسی

### فهرست
- [ویژگی‌ها](#ویژگیها)
- [رمزنگاری و امنیت](#رمزنگاری-و-امنیت)
- [نصب سریع](#نصب-سریع)
- [نصب دستی](#نصب-دستی)
- [پشت CDN (آروان و مشابه)](#پشت-cdn-آروان-و-مشابه)
- [دامنه کار نمی‌کنه؟](#دامنه-کار-نمیکنه)
- [همه‌ی فلگ‌ها](#همهی-فلگها)
- [مدیریت سرویس](#مدیریت-سرویس)
- [آپدیت و حذف](#آپدیت-و-حذف)
- [رفع اشکال](#رفع-اشکال)

### ویژگی‌ها

| ویژگی | توضیح |
|---|---|
| 🩺 **Link Doctor** | قبل از زدن تونل، لاس/پینگ/jitter/MTU رو جدا برای هر جهت می‌سنجه و ترنسپورت پیشنهادی می‌ده |
| 🕵️ **بدون اثر انگشت** | اتصال بدون کلید درست = هیچ پاسخی، عین پورت خاموش |
| 🌐 **پشت CDN** | با `ws`/`wss` از پشت آروان/کلودفلر و هر CDN دیگه رد می‌شه |
| 🔀 **چند ترنسپورت** | `raw`، `tls`، `ws`، `wss`، `udp` — بسته به شرایط لینک عوض کن |
| 🚪 **پورت‌هاپینگ واقعی** | پورت از روی کلید + ساعت محاسبه می‌شه، بدون پیام هماهنگی جدا |
| 🐢→🚀 **Warm-up** | سرعت رو تدریجی بالا می‌بره، نه یهو فول از روز اول |
| 📦 **بدون وابستگی** | فقط `go build .`، هیچ کتابخونه‌ی خارجی لازم نیست |

### رمزنگاری و امنیت

| لایه | روش |
|---|---|
| رمزنگاری ترافیک | AES-256-GCM |
| اشتقاق کلید | HMAC-SHA256، برچسب جدا برای هر جهت (`c2s`/`s2c`) و هر زیرسیستم (`udpconn`, `hop`) |
| فرمت روی سیم (raw) | رکوردهای رمزشده‌ی هم‌اندازه، بدون magic byte، بدون طول plaintext |
| رفتار با کلید غلط | سکوت کامل — نه بنر، نه خطا، عین پورت بسته |
| TLS (اختیاری) | TLS 1.3 واقعی (`crypto/tls`)، گواهی واقعی یا self-signed |
| WebSocket (اختیاری) | RFC 6455 دست‌نویس، برای عبور از CDN |
| UDP (اختیاری) | ARQ رمزشده‌ی خودمون — **نه QUIC واقعی**، صداقتاً |
| پورت‌هاپینگ | `HMAC-SHA256(کلید, بازه‌ی زمانی)` — سمت گیرنده ۳ پورت (قبل/فعلی/بعد) باز نگه می‌داره |

### نصب سریع
```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kiyarash-ns/KiyaTun/main/scripts/install.sh)
```
منوی ساده میاد بالا: روی سرور ایران **Quick setup — server**، روی سرور
خارج **Quick setup — client**. کلید رو کپی کن، تموم.

### نصب دستی
```bash
git clone https://github.com/kiyarash-ns/KiyaTun.git && cd <repo>
go build -o tunx .
./tunx keygen                                              # یه بار، یادداشت کن

# ایران:
./tunx server -l :9000 -expose :443 -key THE_KEY

# خارج:
./tunx client -connect <IP_IRAN>:9000 -to 127.0.0.1:8080 -key THE_KEY
```
تست: `curl http://<IP_IRAN>:443/`

### پشت CDN (آروان و مشابه)
```bash
# ایران:
./tunx server -l :9443 -expose :443 -key THE_KEY -transport ws

# خارج:
./tunx client -connect your-domain.com:443 -to 127.0.0.1:8080 -key THE_KEY -transport ws -sni your-domain.com
```
تو پنل CDN: DNS → IP ایران، Proxy **فعال**، origin port = `9443`،
**WebSocket فعال**، SSL mode = Flexible (برای `ws`) یا Full (برای `wss`).

### دامنه کار نمی‌کنه؟

| حالت | مشکل معمول |
|---|---|
| دامنه پشت CDN (ابر نارنجی) + پروتکل `tcp` خام | CDN فقط HTTP/WS می‌فهمه؛ باید `type=ws` بشه (هم تو KiyaTun هم تو هر پروتکل دیگه مثل VLESS) |
| دامنه پشت CDN + WebSocket خاموش تو پنل | باید تو پنل CDN جدا فعالش کنی |
| دامنه DNS-only (ابر خاکستری) و بازم کار نمی‌کنه | `nslookup domain` و `curl -v http://domain:PORT/` بزن، ببین IP درسته و پورت باز شده |
| گواهی TLS خطا می‌ده | دامنه گواهی نداره؛ یا self-signed با `-insecure-tls` تست کن، یا certbot بزن |

مثال واقعی: `vless://...@domain.com:443?type=tcp` وصل نمی‌شه ولی
`vless://...@IP:443?type=tcp` وصل می‌شه → یعنی دقیقاً حالت اول همین جدول.

### همه‌ی فلگ‌ها

```
tunx keygen
tunx agent  -l :PORT -key KEY
tunx link   -peer IP:PORT -key KEY [-n -i -size -speed -nomtu]

tunx server -l :PORT -expose :PORT[,...] -key KEY
            [-transport raw|tls|ws|wss|udp]
            [-cert FILE -key-file FILE]
            [-warmup DUR -warm-mbps N -target-mbps N]
            [-hop -hop-base N -hop-count N -hop-window DUR]

tunx client -connect HOST:PORT -to HOST:PORT -key KEY
            [-transport raw|tls|ws|wss|udp]
            [-sni DOMAIN -insecure-tls]
            [-ws-path PATH -ws-host HOST]
            [-retry DUR]
            [-hop -hop-base N -hop-count N -hop-window DUR]
```

### مدیریت سرویس
```bash
systemctl status tunx-server       # یا tunx-client
journalctl -u tunx-server -f
```

### آپدیت و حذف
```bash
# آپدیت:
systemctl stop tunx-server
# باینری جدید رو جایگزین /usr/local/bin/tunx کن
systemctl start tunx-server

# حذف کامل:
systemctl disable --now tunx-server tunx-client
rm -f /etc/systemd/system/tunx-*.service
rm -rf /etc/tunx /usr/local/bin/tunx
systemctl daemon-reload
```

### رفع اشکال

| علامت | یعنی چی |
|---|---|
| `bad hello` / `handshake failed` | کلید سرور و کلاینت یکی نیست |
| `connection refused` | پورت اشتباه یا سرویس بالا نیست |
| `timeout` | فایروال بسته، یا IP/دامنه غلط |
| سرعت کمه اول کار | warm-up فعاله؛ `-target-mbps 0` یا صبر کن |
| پشت CDN رد نمی‌شه | WebSocket خاموشه، یا پروتکل داخلی هنوز `tcp` خامه |
| `-hop` وصل نمی‌شه | ساعت دو سرور sync نیست یا hop-base/count/window فرق دارن |

---

## 🇬🇧 English

### What it is
KiyaTun is a reverse TCP tunnel in pure Go. The box with the stable public
IP (e.g. Iran) runs `server`; the box with the real service (e.g. abroad)
runs `client` and dials **out** to the server — so the exit box needs no
open inbound port. End users connect to the server's exposed port; traffic
rides the tunnel to the client, which forwards it to the real local service.

### Features
- **Link Doctor** — per-direction loss/RTT/jitter/MTU test before you commit to a transport
- **No wire fingerprint** — wrong key = zero reply, like a closed port
- **CDN-friendly** — `ws`/`wss` transports pass through Arvan/Cloudflare/etc.
- **Multiple transports** — `raw`, `tls`, `ws`, `wss`, `udp`
- **Real port hopping** — active port derived from `HMAC(key, time window)`, no coordination traffic
- **Warm-up ramp** — avoids a sudden full-speed burst from a fresh IP
- **Zero dependencies** — `go build .` and you're done

### Security
AES-256-GCM for all traffic, HMAC-SHA256 key derivation (separate labels
per direction and subsystem), optional real TLS 1.3, hand-rolled RFC 6455
WebSocket for CDN passthrough, and a custom encrypted ARQ transport over UDP
(**not real QUIC** — no congestion control like BBR, said plainly rather
than oversold).

### Quick install
```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kiyarash-ns/KiyaTun/main/scripts/install.sh)
```

### Manual
```bash
git clone https://github.com/kiyarash-ns/KiyaTun.git && cd <repo>
go build -o tunx .
./tunx keygen

# entry side:
./tunx server -l :9000 -expose :443 -key THE_KEY
# exit side:
./tunx client -connect <ENTRY_IP>:9000 -to 127.0.0.1:8080 -key THE_KEY
```

### Behind a CDN
Use `-transport ws` (CDN terminates TLS) or `-transport wss` (TLS all the
way to origin). In the CDN panel: enable proxy, enable WebSocket, set the
origin port to match `-l`, and SSL mode to Flexible (`ws`) or Full (`wss`).
Any protocol tunneled *through* KiyaTun (e.g. a VLESS config on top) must
also be set to `type=ws` if its domain is CDN-proxied — raw `tcp` mode
can't pass through an HTTP(S)-only CDN edge regardless of what's carrying it.

### Flags
See the Persian section above — the flag reference is identical, just read
the table there.

### Known limitations
- `udp` transport has no real congestion control (fixed-window ARQ, not QUIC/BBR)
- `-hop` needs roughly synced clocks (NTP) between both sides
- No automatic transport switching yet — Link Doctor tells you, you pick `-transport` by hand
- No claim of working against any specific country's filtering at any specific time

---

<div align="center">by Kiyarash</div>
