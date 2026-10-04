<div align="center">

<img src="docs/banner.svg" alt="KiyaTun" width="100%">

```text
  _  ___               _____           _____
 | |/ (_)_   _  __ _  |_   _|   _ _ __ |_   _|   _ _ __
 | ' /| | | | |/ _` |   | || | | | '_ \ | || | | | '_ \
 | . \| | |_| | (_| |   | || |_| | | | | || |_| | | | |
 |_|\_\_|\__, |\__,_|   |_| \__,_|_| |_||_| \__,_|_| |_|
         |___/  by Kiyarash
```

**A lightweight, zero-dependency reverse TCP tunnel written in pure Go,<br>built for networks where the exit side can't expose any inbound port.**


Telegram : https://t.me/net_duck

![Go](https://img.shields.io/badge/Go-pure-00ADD8?logo=go&logoColor=white)
![Dependencies](https://img.shields.io/badge/dependencies-zero-brightgreen)
![Transports](https://img.shields.io/badge/transports-raw%20%7C%20tls%20%7C%20ws%20%7C%20wss%20%7C%20udp-blue)
![Platform](https://img.shields.io/badge/platform-linux-lightgrey?logo=linux&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-green)

[English](#english) · [فارسی](#فارسی)

</div>

---

## Important

Both servers need the **same shared key**, and it must be kept secret (never post it in issues or public chats). If you use port hopping, both server clocks must be roughly in sync.

KiyaTun does **not** spoof IP addresses or touch raw sockets. It is a straightforward encrypted TCP/UDP relay, not a Layer 3 tool. It focuses on looking like ordinary encrypted traffic (TLS / WebSocket) rather than forging packet headers.

---

## English

**Contents:** [What it does](#what-it-does) · [Features](#features) · [Transports](#transports) · [Quick install](#quick-install) · [Connect a panel](#connect-a-panel) · [Port hopping](#port-hopping) · [CDN mode](#cdn-mode) · [Manual usage](#manual-usage) · [Managing the tunnel](#managing-the-tunnel) · [Troubleshooting](#troubleshooting) · [Security notes](#security-notes)

### What it does

KiyaTun connects two servers:

- an **entry** server with a stable public IP (e.g. in Iran), where end users connect;
- an **exit** server (e.g. abroad) that runs the real service: Xray, a 3x-ui / Sanaei panel, or anything that speaks TCP.

The exit side dials **out** to the entry side, so the exit server needs **zero open inbound ports**. End users connect to the entry server; their traffic rides the tunnel to the exit, which hands it to the real local service.

```text
  End user ──► ENTRY server (Iran)  :443
                     ▲
                     │   tunnel  (dialed OUT from the exit side)
                     │
               EXIT server (abroad) ──► 127.0.0.1:PORT   (Xray / 3x-ui panel / any TCP service)
```

### Features

| | |
|---|---|
| 🩺 Link Doctor | Per-direction loss / RTT / jitter / MTU test, with a verdict, before you commit to a transport |
| 🕵️ No wire fingerprint | Wrong key → zero reply, behaves like a closed port |
| 🔀 Multiple transports | `raw`, `tls`, `ws`, `wss`, `udp` |
| 🌐 CDN-friendly | `ws` / `wss` pass through Arvan, Cloudflare, etc. |
| 🚪 Port hopping | Active port derived from `HMAC(key, time window)`, no coordination traffic |
| 🐢→🚀 Warm-up ramp | Gradual speed increase instead of a sudden burst from a fresh IP |
| 🧰 Guided installer | Menu-driven setup of both sides, systemd services, key generation, TLS certificates |
| 📦 Zero dependencies | `go build .` and you're done |

### Transports

| Transport | What it is | Use it when | Firewall |
|---|---|---|---|
| `raw` | Plain TCP, AES-256-GCM encrypted records, no disguise | Direct path works, simplest setup | control port **TCP** |
| `tls` | Same, wrapped in a real TLS 1.3 handshake | You want it to look like ordinary TLS | control port **TCP** |
| `ws` | WebSocket behind a CDN (CDN terminates TLS, plain HTTP to your server) | Only HTTP(S) via a CDN gets through | origin port **TCP** |
| `wss` | WebSocket inside TLS, TLS all the way to your server (needs a real certificate) | Same, end-to-end TLS | origin port **TCP** |
| `udp` | Custom reliable transport over UDP (fixed-window ARQ, encrypted) | TCP specifically is throttled on the path | control port **UDP** |

> `udp` is **not real QUIC**: there is no congestion control like BBR. Said plainly rather than oversold. If UDP is blocked or unstable on your path, use `tls` or `wss`.

Both sides **must use the same transport**.

### Quick install

#### Step 0: what you need

- Two Linux servers with systemd (Ubuntu/Debian commands shown) and root access: the **entry** server (e.g. Iran) and the **exit** server (e.g. abroad).
- `git`, `go` and `curl` on both (the installer builds `tunx` from source until a release binary is published).
- The firewall (and your provider's firewall panel, if any) open for the control port. See the [Transports](#transports) table.

#### Step 1: prepare both servers

```bash
apt update && apt install -y git golang-go curl
timedatectl set-ntp true            # keeps clocks in sync (needed for port hopping)
```

If the build later complains about the Go version, install a newer Go from [go.dev/dl](https://go.dev/dl/).

#### Step 2: the entry server (e.g. Iran), always first

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kiyarash-ns/KiyaTun/main/scripts/install.sh)
```

An interactive menu opens:

| Option | What it does |
|---|---|
| `1` Quick setup, server | `raw` transport, no hopping, no CDN. Asks only for ports and the key |
| `2` Quick setup, client | Same for the exit side |
| `3` Advanced setup | Server/client with **udp / tls / CDN / port hopping**, TLS certificate (certbot), change exposed ports |
| `4` Generate shared key | Prints a new key (or shows the one already installed) |
| `5` Status / logs | Service status and recent logs |
| `6` Restart service(s) | Restarts `tunx-server` / `tunx-client` |
| `7` Uninstall | Removes services, binary and config |

For anything other than plain `raw`, choose **`3` → `1`** (Install server, custom) and answer the prompts:

| Prompt | Answer |
|---|---|
| Transport | `1` raw · `2` tls · `3` CDN-WS · `4` CDN-WSS · `5` udp |
| Port hopping | `N` for a first test (see [Port hopping](#port-hopping)) |
| Control listen address | Enter for `:9000` |
| Exposed address(es) for end users | Enter for `:443`, or your own (comma-separated, e.g. `:443,:8443`) |
| Generate a new shared key? | `Y`. The key is printed. **Copy it now** |

Open the firewall (example for `udp` on `9000` and an exposed TCP port `443`):

```bash
ufw allow 9000/udp      # control port: /udp for the udp transport, /tcp for the others
ufw allow 443/tcp       # exposed port for end users
```

Check it:

```bash
systemctl status tunx-server --no-pager
journalctl -u tunx-server -n 30 --no-pager
```

#### Step 3: the exit server (e.g. abroad)

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kiyarash-ns/KiyaTun/main/scripts/install.sh)
```

Choose **`3` → `2`** (Install client, custom):

| Prompt | Answer |
|---|---|
| Transport | The **same** as the server |
| Port hopping | Same choice and **same values** as the server |
| Server control address | `ENTRY_IP:9000` |
| Local address to forward to | Where the real service listens on this server, e.g. `127.0.0.1:443` (the inbound port of your panel, see below) |
| Shared key | The key from Step 2 |

Check it:

```bash
systemctl status tunx-client --no-pager
journalctl -u tunx-client -f
```

#### Step 4: verify the tunnel

A healthy pair of logs looks like this:

```text
exit  : connected to ENTRY_IP:20101 (udp), forwarding to 127.0.0.1:443
entry : client connected: tunx-udp
```

(With hopping on, the port in the first line is the currently active hop port.)

Make sure something is really listening on the exit server's forward target:

```bash
ss -tlnp | grep ':443'
```

Then connect to `ENTRY_IP:443` from outside. If the real service answers, the tunnel works. Next, set up your panel.

#### Lost the key?

```bash
grep TUNX_KEY /etc/tunx/server.env      # on the entry server
```

### Connect a panel

KiyaTun only carries bytes: whatever listens on the exit server's forward target is what your users reach. The usual setup is a panel such as **3x-ui (Sanaei)**.

**1. Install the panel on the exit server (abroad)**

```bash
bash <(curl -Ls https://raw.githubusercontent.com/mhsanaei/3x-ui/master/install.sh)
```

Follow its prompts and note the panel address, username and password it prints. Run `x-ui` for its management menu; `x-ui settings` shows the current settings. Change the default credentials.

**2. Create an inbound whose port matches the tunnel's forward target**

In the panel: **Inbounds → Add Inbound**.

- **Port:** the same port as the client's "Local address to forward to". Example: if the tunnel forwards to `127.0.0.1:443`, create the inbound on port `443`.
- **Listen IP:** leave blank, or `127.0.0.1` (the tunnel connects locally).
- Pick any protocol/transport you normally use, add a client (user), save.
- Do not use the panel's own web port for an inbound. Two services can't share a port.

Changed your mind about the port? Edit the exit side:

```bash
nano /etc/tunx/client.env          # change TUNX_TO=127.0.0.1:NEW_PORT
systemctl restart tunx-client
```

**3. Give users the entry server's address**

Copy the client link/config from the panel, then change only the **address** to the **entry server IP** and the **port** to the entry server's exposed port (e.g. `443`). Keep every other setting of the inbound (TLS/SNI, Reality, WebSocket path, UUID, …) unchanged. Some panel versions have an *External Proxy* option in the inbound that does this rewrite for you.

```text
  user app ──► ENTRY_IP:443 ══ tunnel ══► EXIT: 127.0.0.1:443 (panel inbound) ──► internet
```

**4. Protect the panel**

- The panel's web port lives on the **exit server's own IP**. Do not expose the panel through the tunnel.
- Restrict it to your IP, or use an SSH tunnel instead of opening it:

```bash
ufw allow from YOUR_HOME_IP to any port PANEL_PORT proto tcp
# or, without opening anything:
ssh -L 2053:127.0.0.1:PANEL_PORT root@EXIT_IP        # then open http://127.0.0.1:2053
```

**Other panels** (Marzban, Hiddify, plain Xray/sing-box, …) work the same way: make sure the service listens on the tunnel's forward target on the exit server, and hand your users the entry server's IP and exposed port.

### Port hopping

Instead of one fixed control port, the active port rotates within a range, derived from `HMAC(key, time window)`. There is no coordination traffic: both sides compute the same port from the shared key and the clock.

Prompts (the installer asks for them on both sides, **values must match**):

| Setting | Default | Meaning |
|---|---|---|
| Hop base port | `20000` | lowest port of the range |
| Hop port count | `200` | number of ports in the range |
| Hop window | `60` s | how often the active port rotates |

Requirements:

1. **Open the whole range** in the firewall. The default range is `20000-20199`:

   ```bash
   ufw allow 20000:20199/udp      # /tcp for raw, tls, ws, wss
   ```

2. **Keep clocks in sync** on both servers: `timedatectl set-ntp true`, then check `timedatectl status | grep -i synchronized`.
3. Same base / count / window on both sides.

When in doubt, first get the tunnel working **without** hopping, then re-run the installer with hopping on.

### CDN mode

With `ws` or `wss` the entry server is the CDN's *origin*.

1. Entry server: **`3` → `1`**, transport `3` (CDN-WS) or `4` (CDN-WSS). Choose the **origin port** the CDN will forward to (default `9443`) and enter your CDN domain.
2. In your CDN panel (Arvan, Cloudflare, …): point the domain's DNS at the CDN, set **origin = entry server IP : origin port**, enable proxy/CDN mode, and switch **WebSocket support on** (many CDNs have a separate toggle).
3. `wss` needs a real certificate for the domain on the entry server (see below). `ws` does not (the CDN handles TLS).
4. Exit server: **`3` → `2`**, same transport. The "server address" is the **CDN domain** and its edge port (default `443`), not the origin IP.

**TLS certificate (certbot):** menu **`3` → `3`** installs certbot, asks for your domain and e-mail, issues a Let's Encrypt certificate, writes it into `server.env` and restarts the server. The domain must point **directly** at the entry server (not CDN-proxied) while the certificate is issued, and port 80 must be free.

### Manual usage

Prefer to skip the installer? Build and run by hand:

```bash
git clone https://github.com/kiyarash-ns/KiyaTun.git && cd KiyaTun
go build -o tunx .
./tunx keygen

# entry server:
./tunx server -l :9000 -expose :443 -key THE_KEY -transport udp

# exit server:
./tunx client -connect <ENTRY_IP>:9000 -to 127.0.0.1:443 -key THE_KEY -transport udp
```

Server flags (from `tunx server -h`):

| Flag | Meaning |
|---|---|
| `-l` | control listen address (default `:9000`) |
| `-expose` | address(es) end users connect to, comma-separated (e.g. `:443,:8443`) |
| `-key` | shared key |
| `-transport` | `raw`, `tls`, `ws`, `wss` or `udp` (default `raw`) |
| `-cert`, `-key-file` | TLS certificate / private key for `tls` / `wss` (self-signed if omitted) |
| `-hop`, `-hop-base`, `-hop-count`, `-hop-window` | port hopping (see above) |
| `-warmup`, `-warm-mbps`, `-target-mbps` | warm-up ramp: start rate, final rate, ramp duration. `-target-mbps 0` (default) means no limit and no warm-up |

Client flags: `-connect`, `-to`, `-key`, `-transport`, `-sni`, plus the same `-hop*` flags. Run `./tunx client -h` for the full list.

The installer's systemd units don't set the warm-up flags. To use them, append them to the `ExecStart` line of `/etc/systemd/system/tunx-server.service`, then `systemctl daemon-reload && systemctl restart tunx-server`. (Re-running the installer rewrites that file.)

Full flag reference, CDN setup and more: [`docs/GUIDE.md`](docs/GUIDE.md).

### Managing the tunnel

| What | Where |
|---|---|
| Services | `tunx-server` (entry), `tunx-client` (exit) |
| Config | `/etc/tunx/server.env`, `/etc/tunx/client.env` |
| Binary | `/usr/local/bin/tunx` |
| Logs | `journalctl -u tunx-server -f` / `journalctl -u tunx-client -f` |

`server.env` keys: `TUNX_LISTEN`, `TUNX_EXPOSE`, `TUNX_KEY`, `TUNX_TRANSPORT`, `TUNX_CERT`, `TUNX_KEYFILE`, `TUNX_HOP_FLAGS`.
`client.env` keys: `TUNX_CONNECT`, `TUNX_TO`, `TUNX_KEY`, `TUNX_TRANSPORT`, `TUNX_SNI`, `TUNX_HOP_FLAGS`.
After editing an env file: `systemctl restart tunx-server` (or `tunx-client`).

**Change exposed ports:** installer → `3` → `4`.

**Change the key (do this on both sides):** generate a new one (`4` in the menu or `tunx keygen`), re-run the installer on the entry server (`3` → `1`) and answer `Y` to create a fresh key (or `n` to paste one made with `tunx keygen`), then on the exit server (`3` → `2`) with the same new key.

**Update to the latest version:**

```bash
systemctl stop tunx-server tunx-client 2>/dev/null
rm -rf /tmp/kiyatun-src && git clone --depth 1 https://github.com/kiyarash-ns/KiyaTun.git /tmp/kiyatun-src
( cd /tmp/kiyatun-src && go build -o /usr/local/bin/tunx . )
systemctl restart tunx-server 2>/dev/null; systemctl restart tunx-client 2>/dev/null
```

**Uninstall:** installer → `7`.

### Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `Unit tunx-client.service could not be found` | Client isn't installed on this machine | Installer → `3` → `2` |
| Client log: `dial tcp ...:9000: connect: connection refused` | Entry server isn't running, wrong address/port, or the port is closed | `systemctl status tunx-server` on the entry server, check the port and firewall (with hopping, open the whole range) |
| Client keeps retrying, no error, nothing on the entry server | Wrong key, or UDP blocked. A wrong key gets **no reply by design** | Compare `TUNX_KEY` in both env files; test with `tls` instead of `udp` |
| "connected" in both logs but users get nothing | Nothing listens on the exit's forward target | `ss -tlnp \| grep PORT`; create the panel inbound or fix `TUNX_TO` |
| Drops right after each hop window | Clocks differ or hop values don't match | `timedatectl set-ntp true`; same base/count/window on both sides |
| `Build failed` during install | `git`/`go` missing, or Go too old for `go.mod` | `apt install -y git golang-go`; or install a newer Go from go.dev |
| Entry server: `address already in use` | Another service uses the exposed port | Stop it, or change the exposed port (installer → `3` → `4`) |
| Works with `raw` but not `udp` | UDP filtered or throttled on the path | Use `tls` / `wss` |

### Security notes

- The shared key is the only secret. Treat it like a password; **rotate it** if it ever leaks.
- The installer passes the key on the command line of the systemd service. Other local users on the same machine could read it with `ps`, so run KiyaTun on servers you control.
- `udp` is a custom ARQ transport, not QUIC. It is encrypted but has no BBR-style congestion control.
- Port hopping depends on synchronized clocks.
- KiyaTun is a relay, not an anonymity tool, and provided as-is. Use it responsibly and within the laws that apply to you.

### License

MIT

---

<div dir="rtl" align="right">

## فارسی

### چیکار می‌کنه

KiyaTun دو سرور رو بهم وصل می‌کنه:

- یه سرور **ورودی** با IP ثابت (مثلاً ایران) که کاربرها بهش وصل می‌شن؛
- یه سرور **خروجی** (مثلاً خارج) که سرویس واقعی روشه: Xray، پنل 3x-ui (سنایی) یا هر سرویس TCP دیگه.

سمت خروجی خودش به سمت ورودی **وصل می‌شه** (dial out)، پس سرور خروجی **هیچ پورت ورودی بازی لازم نداره**. کاربر به سرور ورودی وصل می‌شه، ترافیک از تونل رد می‌شه و به سرویس واقعی روی سرور خروجی می‌رسه.

</div>

```text
  کاربر ──► سرور ورودی (ایران)  :443
                  ▲
                  │   تونل (از سمت خروجی به ورودی وصل می‌شه)
                  │
            سرور خروجی (خارج) ──► 127.0.0.1:PORT   (Xray / پنل 3x-ui / هر سرویس TCP)
```

<div dir="rtl" align="right">

### ویژگی‌ها

| | |
|---|---|
| 🩺 Link Doctor | تست loss / پینگ / jitter / MTU جدا برای هر جهت، همراه با نتیجه‌گیری، قبل از انتخاب ترنسپورت |
| 🕵️ بدون اثر انگشت | کلید غلط = هیچ جوابی، مثل یه پورت بسته |
| 🔀 چند ترنسپورت | `raw`، `tls`، `ws`، `wss`، `udp` |
| 🌐 سازگار با CDN | `ws` / `wss` از آروان، کلودفلر و … رد می‌شن |
| 🚪 پورت‌هاپینگ | پورت فعال از `HMAC(کلید، بازه‌ی زمانی)` محاسبه می‌شه، بدون ترافیک هماهنگی |
| 🐢→🚀 افزایش تدریجی سرعت | به‌جای یه انفجار ناگهانی از یه IP تازه |
| 🧰 نصاب منویی | نصب هر دو سمت، سرویس systemd، ساخت کلید، گواهی TLS |
| 📦 بدون وابستگی | `go build .` و تمام |

### ترنسپورت‌ها

| ترنسپورت | چیه | کی استفاده کنم | فایروال |
|---|---|---|---|
| `raw` | TCP ساده با رکوردهای رمزشده‌ی AES-256-GCM، بدون استتار | مسیر مستقیم کار می‌کنه، ساده‌ترین حالت | پورت کنترل **TCP** |
| `tls` | همون، داخل یه هندشیک واقعی TLS 1.3 | می‌خوای شبیه TLS معمولی باشه | پورت کنترل **TCP** |
| `ws` | وب‌سوکت پشت CDN (CDN پایان TLS رو انجام می‌ده) | فقط HTTP(S) از طریق CDN رد می‌شه | پورت origin **TCP** |
| `wss` | وب‌سوکت داخل TLS تا خود سرور (گواهی واقعی لازم داره) | همون، با TLS سرتاسری | پورت origin **TCP** |
| `udp` | ترنسپورت قابل‌اعتماد سفارشی روی UDP (ARQ با پنجره‌ی ثابت، رمزشده) | TCP توی مسیر کند/محدود شده | پورت کنترل **UDP** |

> `udp` **QUIC واقعی نیست** و کنترل ازدحام مثل BBR نداره. صادقانه نوشتم، اغراق نکردم. اگه UDP توی مسیر بسته یا ناپایداره، از `tls` یا `wss` استفاده کن.

ترنسپورت هر دو سمت **باید یکی باشه**.

### نصب سریع

#### قدم ۰: چی لازم داری

- دو سرور لینوکس با systemd (دستورها برای اوبونتو/دبیان) و دسترسی root: سرور **ورودی** (مثلاً ایران) و سرور **خروجی** (مثلاً خارج).
- `git` و `go` و `curl` روی هر دو (تا وقتی باینری ریلیز نذاشتم، نصاب خودش `tunx` رو از سورس می‌سازه).
- فایروال (و فایروال پنل سرویس‌دهنده) برای پورت کنترل باز باشه. جدول «ترنسپورت‌ها» بالا رو ببین.

#### قدم ۱: آماده‌سازی هر دو سرور

</div>

```bash
apt update && apt install -y git golang-go curl
timedatectl set-ntp true            # هم‌گام کردن ساعت (برای پورت‌هاپینگ لازمه)
```

<div dir="rtl" align="right">

اگه موقع بیلد درباره‌ی نسخه‌ی Go خطا دیدی، Go جدیدتر رو از [go.dev/dl](https://go.dev/dl/) نصب کن.

#### قدم ۲: سرور ورودی (مثلاً ایران)، همیشه اول

</div>

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kiyarash-ns/KiyaTun/main/scripts/install.sh)
```

<div dir="rtl" align="right">

یه منوی تعاملی باز می‌شه:

| گزینه | کارش |
|---|---|
| `1` نصب سریع، سرور | ترنسپورت `raw`، بدون هاپینگ و CDN. فقط پورت‌ها و کلید رو می‌پرسه |
| `2` نصب سریع، کلاینت | همین برای سمت خروجی |
| `3` نصب پیشرفته | سرور/کلاینت با **udp / tls / CDN / پورت‌هاپینگ**، گواهی TLS (certbot)، تغییر پورت‌های expose |
| `4` ساخت کلید مشترک | کلید جدید می‌سازه (یا کلیدِ نصب‌شده رو نشون می‌ده) |
| `5` وضعیت / لاگ | وضعیت سرویس و لاگ‌های اخیر |
| `6` ری‌استارت سرویس‌ها | `tunx-server` / `tunx-client` رو ری‌استارت می‌کنه |
| `7` حذف کامل | سرویس‌ها، باینری و کانفیگ رو پاک می‌کنه |

برای هر چیزی غیر از `raw` ساده، **`3` ← `1`** (Install server, custom) رو بزن و جواب بده:

| سوال | جواب |
|---|---|
| Transport | `1` raw · `2` tls · `3` CDN-WS · `4` CDN-WSS · `5` udp |
| Port hopping | برای اولین تست `N` (بخش «پورت‌هاپینگ» پایین‌تر) |
| Control listen address | Enter برای `:9000` |
| Exposed address(es) | Enter برای `:443`، یا آدرس دلخواه (جدا با کاما، مثلاً `:443,:8443`) |
| Generate a new shared key? | `Y`. کلید چاپ می‌شه، **همین‌جا کپیش کن** |

فایروال رو باز کن (مثال برای `udp` روی `9000` و پورت TCP کاربرها `443`):

</div>

```bash
ufw allow 9000/udp      # پورت کنترل: برای udp از /udp، برای بقیه /tcp
ufw allow 443/tcp       # پورت expose برای کاربرها
```

```bash
systemctl status tunx-server --no-pager
journalctl -u tunx-server -n 30 --no-pager
```

<div dir="rtl" align="right">

#### قدم ۳: سرور خروجی (مثلاً خارج)

</div>

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/kiyarash-ns/KiyaTun/main/scripts/install.sh)
```

<div dir="rtl" align="right">

**`3` ← `2`** (Install client, custom) رو بزن:

| سوال | جواب |
|---|---|
| Transport | **همون** سرور |
| Port hopping | همون انتخاب و **همون مقدارهای** سرور |
| Server control address | `IP_ورودی:9000` |
| Local address to forward to | جایی که سرویس واقعی روی همین سرور گوش می‌ده، مثلاً `127.0.0.1:443` (پورت inbound پنل، پایین‌تر) |
| Shared key | کلیدی که توی قدم ۲ ساختی |

</div>

```bash
systemctl status tunx-client --no-pager
journalctl -u tunx-client -f
```

<div dir="rtl" align="right">

#### قدم ۴: مطمئن شو تونل کار می‌کنه

لاگ سالم این‌طوریه:

</div>

```text
exit  : connected to ENTRY_IP:20101 (udp), forwarding to 127.0.0.1:443
entry : client connected: tunx-udp
```

<div dir="rtl" align="right">

(با هاپینگ، پورت خط اول همون پورت فعالِ همون لحظه‌ست.)

چک کن واقعاً یه سرویس روی مقصد سرور خروجی گوش می‌ده:

</div>

```bash
ss -tlnp | grep ':443'
```

<div dir="rtl" align="right">

بعد از بیرون به `IP_ورودی:443` وصل شو. اگه سرویس واقعی جواب داد، تونل کار می‌کنه. قدم بعدی نصب پنله.

#### کلید رو گم کردی؟

</div>

```bash
grep TUNX_KEY /etc/tunx/server.env      # روی سرور ورودی
```

<div dir="rtl" align="right">

### وصل کردن پنل

KiyaTun فقط بایت جابه‌جا می‌کنه: هر چیزی که روی مقصدِ سرور خروجی گوش بده، همونیه که کاربرها بهش می‌رسن. معمولاً این یه پنل مثل **3x-ui (سنایی)** هست.

**۱. نصب پنل روی سرور خروجی (خارج)**

</div>

```bash
bash <(curl -Ls https://raw.githubusercontent.com/mhsanaei/3x-ui/master/install.sh)
```

<div dir="rtl" align="right">

سوال‌هاش رو جواب بده و آدرس پنل، یوزرنیم و پسورد رو یادداشت کن. با دستور `x-ui` منوی مدیریتش باز می‌شه و `x-ui settings` تنظیمات فعلی رو نشون می‌ده. یوزرنیم و پسورد پیش‌فرض رو عوض کن.

**۲. یه inbound بساز که پورتش با مقصد تونل یکی باشه**

توی پنل: **Inbounds ← Add Inbound**.

- **Port:** همون پورتِ «Local address to forward to» کلاینت. مثلاً اگه تونل به `127.0.0.1:443` وصله، inbound رو روی پورت `443` بساز.
- **Listen IP:** خالی بذار یا `127.0.0.1` (تونل به‌صورت محلی وصل می‌شه).
- هر پروتکل/ترنسپورتی که معمولاً استفاده می‌کنی انتخاب کن، یه کاربر (client) اضافه کن و ذخیره کن.
- پورت وب خود پنل رو برای inbound استفاده نکن، دو سرویس نمی‌تونن یه پورت داشته باشن.

نظرت درباره‌ی پورت عوض شد؟ سمت خروجی رو ویرایش کن:

</div>

```bash
nano /etc/tunx/client.env          # مقدار TUNX_TO=127.0.0.1:PORT_جدید
systemctl restart tunx-client
```

<div dir="rtl" align="right">

**۳. آدرس سرور ورودی رو به کاربرها بده**

لینک/کانفیگ کاربر رو از پنل کپی کن و فقط **آدرس** رو به **IP سرور ورودی** و **پورت** رو به پورت expose سرور ورودی (مثلاً `443`) تغییر بده. بقیه‌ی تنظیمات inbound (TLS/SNI، Reality، مسیر وب‌سوکت، UUID و …) دست‌نخورده بمونه. بعضی نسخه‌های پنل توی inbound گزینه‌ی *External Proxy* دارن که همین جایگزینی رو خودش انجام می‌ده.

</div>

```text
  اپ کاربر ──► ENTRY_IP:443 ══ تونل ══► خروجی: 127.0.0.1:443 (inbound پنل) ──► اینترنت
```

<div dir="rtl" align="right">

**۴. پنل رو امن کن**

- پورت وب پنل روی IP خودِ **سرور خروجی** هست. پنل رو از داخل تونل در دسترس نذار.
- دسترسی رو به IP خودت محدود کن یا به‌جای باز کردنش از تونل SSH استفاده کن:

</div>

```bash
ufw allow from IP_خودت to any port PORT_PANEL proto tcp
# یا بدون باز کردن چیزی:
ssh -L 2053:127.0.0.1:PORT_PANEL root@EXIT_IP        # بعد http://127.0.0.1:2053 رو باز کن
```

<div dir="rtl" align="right">

**پنل‌های دیگه** (مرزبان، هیدیفای، Xray/sing-box خام و …) هم همین‌طوره: مطمئن شو سرویس روی مقصد تونل توی سرور خروجی گوش می‌ده، و IP و پورت expose سرور ورودی رو به کاربرها بده.

### پورت‌هاپینگ

به‌جای یه پورت کنترل ثابت، پورت فعال توی یه بازه می‌چرخه و از `HMAC(کلید، بازه‌ی زمانی)` به‌دست میاد. ترافیک هماهنگی وجود نداره: هر دو سمت از کلید مشترک و ساعت، همون پورت رو حساب می‌کنن.

مقدارهایی که نصاب از هر دو سمت می‌پرسه (**باید یکی باشن**):

| تنظیم | پیش‌فرض | معنی |
|---|---|---|
| Hop base port | `20000` | کمترین پورت بازه |
| Hop port count | `200` | تعداد پورت‌های بازه |
| Hop window | `60` ثانیه | هر چند وقت پورت فعال عوض می‌شه |

شرط‌ها:

1. **کل بازه** رو توی فایروال باز کن. بازه‌ی پیش‌فرض `20000-20199` هست:

</div>

```bash
ufw allow 20000:20199/udp      # برای raw, tls, ws, wss از /tcp
```

<div dir="rtl" align="right">

2. **ساعت دو سرور هم‌گام باشه**: `timedatectl set-ntp true` و بعد `timedatectl status | grep -i synchronized`.
3. base / count / window روی هر دو سمت یکی باشه.

اگه شک داری، اول تونل رو **بدون** هاپینگ راه بنداز، بعد نصاب رو با هاپینگ دوباره اجرا کن.

### حالت CDN

با `ws` یا `wss` سرور ورودی، origin یه CDN می‌شه.

1. سرور ورودی: **`3` ← `1`**، ترنسپورت `3` (CDN-WS) یا `4` (CDN-WSS). **پورت origin** که CDN بهش فوروارد می‌کنه (پیش‌فرض `9443`) و دامنه‌ی CDN رو وارد کن.
2. توی پنل CDN (آروان، کلودفلر و …): DNS دامنه رو به CDN بده، **origin = IP سرور ورودی : پورت origin** رو بذار، حالت proxy/CDN رو روشن کن و **پشتیبانی WebSocket رو فعال کن** (خیلی از CDNها سوئیچ جدا دارن).
3. `wss` روی سرور ورودی گواهی واقعی برای دامنه می‌خواد (پایین‌تر). `ws` نمی‌خواد (CDN خودش TLS رو انجام می‌ده).
4. سرور خروجی: **`3` ← `2`**، همون ترنسپورت. «آدرس سرور» **دامنه‌ی CDN** و پورت edge اونه (پیش‌فرض `443`)، نه IP سرور origin.

**گواهی TLS (certbot):** منوی **`3` ← `3`** خودش certbot رو نصب می‌کنه، دامنه و ایمیل می‌پرسه، گواهی Let's Encrypt می‌گیره، توی `server.env` می‌نویسه و سرور رو ری‌استارت می‌کنه. هنگام صدور، دامنه باید **مستقیم** به سرور ورودی اشاره کنه (نه پشت پراکسی CDN) و پورت ۸۰ آزاد باشه.

### اجرای دستی

نمی‌خوای از نصاب استفاده کنی؟ دستی بساز و اجرا کن:

</div>

```bash
git clone https://github.com/kiyarash-ns/KiyaTun.git && cd KiyaTun
go build -o tunx .
./tunx keygen

# سرور ورودی:
./tunx server -l :9000 -expose :443 -key THE_KEY -transport udp

# سرور خروجی:
./tunx client -connect <ENTRY_IP>:9000 -to 127.0.0.1:443 -key THE_KEY -transport udp
```

<div dir="rtl" align="right">

فلگ‌های سرور (از `tunx server -h`):

| فلگ | معنی |
|---|---|
| `-l` | آدرس listen کنترل (پیش‌فرض `:9000`) |
| `-expose` | آدرس(های) اتصال کاربرها، جدا با کاما (مثلاً `:443,:8443`) |
| `-key` | کلید مشترک |
| `-transport` | `raw`، `tls`، `ws`، `wss` یا `udp` (پیش‌فرض `raw`) |
| `-cert`، `-key-file` | گواهی / کلید خصوصی TLS برای `tls` / `wss` (اگه ندی self-signed) |
| `-hop`، `-hop-base`، `-hop-count`، `-hop-window` | پورت‌هاپینگ (بالا) |
| `-warmup`، `-warm-mbps`، `-target-mbps` | افزایش تدریجی سرعت: نرخ شروع، نرخ نهایی، مدت افزایش. `-target-mbps 0` (پیش‌فرض) یعنی بدون محدودیت و بدون warm-up |

فلگ‌های کلاینت: `-connect`، `-to`، `-key`، `-transport`، `-sni` و همون فلگ‌های `-hop*`. لیست کامل: `./tunx client -h`.

یونیت‌های systemd نصاب فلگ‌های warm-up رو نمی‌ذارن. برای استفاده، اون‌ها رو آخر خط `ExecStart` فایل `/etc/systemd/system/tunx-server.service` اضافه کن و بعد `systemctl daemon-reload && systemctl restart tunx-server`. (اجرای دوباره‌ی نصاب این فایل رو بازنویسی می‌کنه.)

راهنمای کامل فلگ‌ها، CDN و بیشتر: [`docs/GUIDE.md`](docs/GUIDE.md)

### مدیریت تونل

| چی | کجا |
|---|---|
| سرویس‌ها | `tunx-server` (ورودی)، `tunx-client` (خروجی) |
| کانفیگ | `/etc/tunx/server.env`، `/etc/tunx/client.env` |
| باینری | `/usr/local/bin/tunx` |
| لاگ | `journalctl -u tunx-server -f` / `journalctl -u tunx-client -f` |

کلیدهای `server.env`: `TUNX_LISTEN`، `TUNX_EXPOSE`، `TUNX_KEY`، `TUNX_TRANSPORT`، `TUNX_CERT`، `TUNX_KEYFILE`، `TUNX_HOP_FLAGS`.
کلیدهای `client.env`: `TUNX_CONNECT`، `TUNX_TO`، `TUNX_KEY`، `TUNX_TRANSPORT`، `TUNX_SNI`، `TUNX_HOP_FLAGS`.
بعد از ویرایش فایل env: `systemctl restart tunx-server` (یا `tunx-client`).

**تغییر پورت‌های expose:** نصاب ← `3` ← `4`.

**عوض کردن کلید (روی هر دو سمت):** کلید جدید بساز (گزینه `4` منو یا `tunx keygen`)، نصاب رو روی سرور ورودی دوباره اجرا کن (`3` ← `1`) و کلید جدید رو بده، بعد روی سرور خروجی (`3` ← `2`) با همون کلید جدید.

**آپدیت به آخرین نسخه:**

</div>

```bash
systemctl stop tunx-server tunx-client 2>/dev/null
rm -rf /tmp/kiyatun-src && git clone --depth 1 https://github.com/kiyarash-ns/KiyaTun.git /tmp/kiyatun-src
( cd /tmp/kiyatun-src && go build -o /usr/local/bin/tunx . )
systemctl restart tunx-server 2>/dev/null; systemctl restart tunx-client 2>/dev/null
```

<div dir="rtl" align="right">

**حذف کامل:** نصاب ← `7`.

### رفع اشکال

| علامت | علت احتمالی | راه‌حل |
|---|---|---|
| `Unit tunx-client.service could not be found` | کلاینت روی این سرور نصب نشده | نصاب ← `3` ← `2` |
| لاگ کلاینت: `dial tcp ...:9000: connect: connection refused` | سرور ورودی بالا نیست، آدرس/پورت اشتباهه یا پورت بسته‌ست | روی سرور ورودی `systemctl status tunx-server`، پورت و فایروال رو چک کن (با هاپینگ کل بازه باز باشه) |
| کلاینت مدام تلاش می‌کنه، بدون خطا و روی سرور ورودی هم چیزی نیست | کلید غلط یا UDP بسته. کلید غلط **عمداً جواب نمی‌گیره** | `TUNX_KEY` دو فایل env رو مقایسه کن؛ با `tls` به‌جای `udp` تست کن |
| توی هر دو لاگ «connected» هست ولی کاربر چیزی نمی‌گیره | مقصد سرور خروجی چیزی گوش نمی‌ده | `ss -tlnp \| grep PORT`؛ inbound پنل رو بساز یا `TUNX_TO` رو درست کن |
| بعد از هر پنجره‌ی هاپینگ قطع می‌شه | ساعت‌ها فرق دارن یا مقدارهای هاپینگ یکی نیستن | `timedatectl set-ntp true`؛ base/count/window هر دو سمت یکی باشه |
| `Build failed` موقع نصب | `git`/`go` نصب نیست یا Go برای `go.mod` قدیمیه | `apt install -y git golang-go`؛ یا Go جدیدتر از go.dev |
| سرور ورودی: `address already in use` | یه سرویس دیگه روی پورت expose هست | اون رو متوقف کن یا پورت expose رو عوض کن (نصاب ← `3` ← `4`) |
| با `raw` کار می‌کنه ولی با `udp` نه | UDP توی مسیر فیلتر یا کند شده | از `tls` / `wss` استفاده کن |

### نکات امنیتی

- کلید مشترک تنها رازه. مثل پسورد باهاش رفتار کن و اگه لو رفت **عوضش کن**.
- نصاب کلید رو توی خط فرمان سرویس systemd می‌ذاره. کاربرهای دیگه‌ی همون ماشین می‌تونن با `ps` ببیننش، پس KiyaTun رو روی سرورهایی اجرا کن که خودت کنترلشون می‌کنی.
- `udp` یه ARQ سفارشیه، نه QUIC. رمزشده‌ست ولی کنترل ازدحام مثل BBR نداره.
- پورت‌هاپینگ به ساعت هم‌گام وابسته‌ست.
- KiyaTun یه رله‌ست، نه ابزار ناشناس‌سازی، و همین‌طور که هست ارائه می‌شه. با مسئولیت خودت و مطابق قوانینی که برات صدق می‌کنه ازش استفاده کن.

### لایسنس

MIT

</div>
