# KiyaTun

KiyaTun — a lightweight reverse TCP tunnel in pure Go, zero external dependencies.

Built for restricted networks (especially Iran <-> abroad) where the exit server must not expose any inbound ports.

Status: under active update and development — در حال آپدیت و توسعه.

| | |
|---|---|
| Author | kiyarash-ns |
| Language | Go — zero external dependencies |
| Encryption | AES-256-GCM |
| Transports | raw / tls / ws / wss |
| License | MIT |

## English

### Features

- Reverse tunnel architecture — the abroad side dials out; no inbound ports needed there
- AES-256-GCM encryption with fixed-size records
- No fingerprint on the wire — no magic numbers, banners, or plaintext length fields. A connection with a wrong key gets no response at all, just like a dead port (anti active-probing)
- Multiplexing — multiple streams over one control connection
- Transports: raw / tls / ws / wss
- CDN support — WebSocket transport passes through Arvan, Cloudflare, etc. — exactly what CDNs proxy themselves
- Real SSL certificate support — to anything inspecting the traffic, it looks like an ordinary HTTPS site
- Built-in Link Doctor — test the path before you tunnel: ping, packet loss, jitter, MTU, throughput, measured per direction (filtering is often one-way; a normal test will fool you)
- Gradual speed warm-up — doesn't go full speed on day one; ramps up slowly so the server doesn't end up under the microscope

### Architecture

| Role | Location | Behavior |
|------|----------|----------|
| Server (Entry) | Iran | Listens on control port + exposed ports |
| Client (Exit) | Abroad | Dials out to Iran, forwards traffic locally |

The abroad server needs no open inbound ports.

### Install

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kiyarash-ns/KiyaTun/main/scripts/install.sh)
```

A simple menu comes up. Steps:

1. On both servers: option 1 (install tunx), then option 2 (generate key) — the same key must be used on both sides
2. Optional, recommended: link test — on one server run option 5, choice 1 (agent), then from the other server option 5, choice 2 (link) with the peer address
3. On the entry server (Iran): option 3 — control port, exposed port, transport, key
4. On the exit server (abroad): option 4 — connect to IRAN_IP:9000, forward to 127.0.0.1:443, same key
5. Check with option 6 — both services should show active (running)

Firewall: on the entry server open the control port and the exposed port, e.g. ufw allow 9000 and ufw allow 443. The exit server needs no inbound ports.

### Manual

```bash
./tunx keygen
./tunx agent -l :9000 -key KEY          # on both servers, for link testing
./tunx link -peer IP:9000 -key KEY      # test from each side separately

./tunx server -l :9000 -expose :443 -key KEY              # entry server (e.g. Iran)
./tunx client -connect IP:9000 -to 127.0.0.1:443 -key KEY # exit side
```

All flags: ./tunx --help

### Not finished yet

No UDP/QUIC yet, and no real port hopping. Found a bug? Open an issue.

---

## فارسی

KiyaTun یه تانل معکوس TCP سبکه که با Go خالص نوشته شده و هیچ وابستگی خارجی نداره.
برای شبکه‌های محدودشده طراحی شده (مخصوصاً ایران <-> خارج) جایی که سرور خروجی نباید هیچ پورت ورودی باز داشته باشه.

وضعیت: در حال آپدیت و توسعه.

### ویژگی‌ها

- تانل معکوس — سمت خروجی خودش وصل می‌شه، هیچ پورت ورودی لازم نداره
- رمزنگاری AES-256-GCM با رکوردهای سایز ثابت
- هیچ اثر انگشتی رو سیم نمی‌ذاره — بدون شماره‌ی جادویی، بدون بنر، بدون طول پکت به‌صورت پلتکست. کانکشن با کلید غلط = هیچ جوابی نمی‌گیری، عین یه پورت خاموش
- مالتی‌پلکسینگ — چند استریم روی یک کانکشن کنترل
- ترنسپورت‌ها: raw / tls / ws / wss
- از پشت CDN هم رد می‌شه (آروان، کلودفلر، هرچی) — با WebSocket، همون چیزی که CDNها خودشون پروکسی می‌کنن
- می‌تونی روش یه گواهی واقعی SSL بذاری تا از نظر هر چیزی که ترافیک رو چک می‌کنه، عین یه HTTPS معمولی باشه
- تست لینک داخلی — قبل از اینکه اصلاً تانل بزنی، می‌تونی لینک بین دو سرور رو بسنجی: پینگ، پکت‌لاس، jitter، MTU و سرعت، جدا برای هر جهت (چون خیلی وقتا فیلترینگ یک‌طرفه‌ست و تست عادی گولت می‌زنه)
- سرعتو یهو فول نمی‌ده از روز اول؛ آروم آروم بالا می‌بره تا سرور تازه زیر ذره‌بین نره

### معماری

| نقش | مکان | رفتار |
|------|----------|----------|
| سرور (ورودی) | ایران | روی پورت کنترل و پورت‌های اکسپوز گوش می‌ده |
| کلاینت (خروجی) | خارج | به ایران وصل می‌شه و ترافیک رو محلی فوروارد می‌کنه |

سمت خارج هیچ پورت ورودی بازی لازم نداره.

### نصب

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kiyarash-ns/KiyaTun/main/scripts/install.sh)
```

یه منوی ساده میاد بالا، عدد بزن، تموم. مراحل:

۱. روی هر دو سرور: گزینه ۱ (نصب tunx) و بعد گزینه ۲ (ساخت کلید) — همین یه کلید باید روی هر دو سرور یکسان باشه
۲. اختیاری ولی توصیه می‌شه: تست لینک — روی یکی از سرورها گزینه ۵ و انتخاب ۱ (agent)، بعد از سرور دیگه گزینه ۵ و انتخاب ۲ (link) با آدرس سرور اول
۳. روی سرور ورودی (ایران): گزینه ۳ — پورت کنترل، پورت اکسپوز، ترنسپورت و کلید رو بده
۴. روی سرور خروجی (خارج): گزینه ۴ — به IP_ایران:9000 وصل شو، فوروارد به 127.0.0.1:443، همون کلید
۵. با گزینه ۶ وضعیت رو چک کن — هر دو سرویس باید active (running) باشن

فایروال: روی سرور ایران پورت کنترل و پورت اکسپوز رو باز کن، مثلاً ufw allow 9000 و ufw allow 443. سرور خارج هیچ پورت ورودی لازم نداره.

### دستی (اگه خواستی خودت کنترل کنی)

```bash
./tunx keygen
./tunx agent -l :9000 -key KEY          # رو هر دو سرور، برای تست لینک
./tunx link -peer IP:9000 -key KEY      # از هر طرف جدا تست کن

./tunx server -l :9000 -expose :443 -key KEY              # سرور ورودی (مثلاً ایران)
./tunx client -connect IP:9000 -to 127.0.0.1:443 -key KEY # سمت خروجی
```

همه‌ی فلگ‌ها: ./tunx --help

### هنوز کامل نیست

UDP/QUIC هنوز نداره، پورت‌هاپینگ واقعی هم نه. اگه باگی چیزی دیدی issue بزن.

## License

MIT
