# KiyaTun

KiyaTun — a lightweight reverse TCP tunnel in pure Go, zero external dependencies.

Built for restricted networks (especially Iran <-> abroad) where the exit server must not expose any inbound ports.

Status: under active update and development.

## English

### Features

- Reverse tunnel architecture — the abroad side dials out; no inbound ports needed there
- AES-256-GCM encryption with fixed-size records
- No fingerprint on the wire — no magic numbers, banners, or plaintext length fields. A connection with a wrong key gets no response at all, just like a dead port (anti active-probing)
- Multiplexing — multiple streams over one control connection
- Transports: raw / tls / ws / wss
- CDN support — WebSocket transport passes through Arvan, Cloudflare, etc., exactly what CDNs proxy themselves
- Real SSL certificate support — to anything inspecting the traffic, it looks like an ordinary HTTPS site
- Built-in Link Doctor — test the path before you tunnel: ping, packet loss, jitter, MTU, throughput, measured per direction (filtering is often one-way; a normal test will fool you)
- Gradual speed warm-up — doesn't go full speed on day one; ramps up slowly so the server doesn't end up under the microscope

### Architecture

- Server (Entry) — Iran — listens on the control port and the exposed ports
- Client (Exit) — Abroad — dials out to Iran and forwards traffic locally

The abroad server needs no open inbound ports.

### Install

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kiyarash-ns/KiyaTun/main/scripts/install.sh)
```

A simple menu comes up — pick a number, done.

### Manual

```bash
./tunx keygen
./tunx agent -l :9000 -key KEY          # on both servers, for link testing
./tunx link -peer IP:9000 -key KEY      # test from each side separately

./tunx server -l :9000 -expose :443 -key KEY              # entry server (e.g. Iran)
./tunx client -connect IP:9000 -to 127.0.0.1:443 -key KEY # exit side
```

All flags: ./tunx --help

---

## فارسی

KiyaTun یه تانل معکوس TCP سبکه که با Go خالص نوشته شده و هیچ وابستگی خارجی نداره.
برای شبکه‌های محدودشده طراحی شده (مخصوصاً ایران <-> خارج) جایی که سرور خروجی نباید هیچ پورت ورودی باز داشته باشه.

وضعیت: در حال آپدیت و توسعه.

### ویژگی‌ها

- قبل از اینکه اصلاً تانل بزنی، می‌تونی لینک بین دو سرور رو بسنجی — پینگ، پکت‌لاس، jitter، MTU، همه جدا برای هر جهت (چون خیلی وقتا فیلترینگ یک‌طرفه‌ست و تست عادی گولت می‌زنه)
- هیچ اثر انگشتی رو سیم نمی‌ذاره. کانکشن با کلید غلط = هیچ جوابی نمی‌گیری، عین یه پورت خاموش
- از پشت CDN هم رد می‌شه (آروان، کلودفلر، هرچی) — با WebSocket، همون چیزی که CDNها خودشون پروکسی می‌کنن
- می‌تونی روش یه گواهی واقعی SSL بذاری تا از نظر هر چیزی که ترافیک رو چک می‌کنه، عین یه HTTPS معمولی باشه
- سرعتو یهو فول نمی‌ده از روز اول؛ آروم آروم بالا می‌بره تا سرور تازه زیر ذره‌بین نره

### معماری

- سرور (ورودی) — ایران — روی پورت کنترل و پورت‌های اکسپوز گوش می‌ده
- کلاینت (خروجی) — خارج — به ایران شماره می‌زنه و ترافیک رو محلی فوروارد می‌کنه

سمت خارج هیچ پورت ورودی بازی لازم نداره.

### نصب

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kiyarash-ns/KiyaTun/main/scripts/install.sh)
```

یه منوی ساده میاد بالا، عدد بزن، تموم.

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
