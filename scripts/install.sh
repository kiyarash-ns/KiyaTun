#!/usr/bin/env bash
# KiyaTun installer — https://github.com/kiyarash-ns/KiyaTun

set -euo pipefail

REPO="kiyarash-ns/KiyaTun"
BIN="/usr/local/bin/tunx"
CONF_DIR="/etc/tunx"

G='\033[1;32m' R='\033[1;31m' B='\033[1;36m' N='\033[0m'
ok()  { echo -e "${G}[+]${N} $*"; }
err() { echo -e "${R}[!]${N} $*"; }
say() { echo -e "${B}[*]${N} $*"; }

[ "$(id -u)" -eq 0 ] || { err "Please run as root."; exit 1; }

arch_name() {
    case "$(uname -m)" in
        x86_64)        echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             return 1 ;;
    esac
}

need_tunx() { [ -x "$BIN" ] || { err "tunx is not installed — run option 1 first."; return 1; }; }

# prints "-transport <value>" only if the binary actually supports the flag
trans_flag() {
    if "$BIN" --help 2>&1 | grep -qi -- '-transport'; then
        echo "-transport $1"
    fi
}

install_tunx() {
    local A
    A="$(arch_name)" || { err "Unsupported architecture."; return 1; }

    say "Looking for a release binary (linux-$A) ..."
    local url=""
    url="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
        | grep -o '"browser_download_url": *"[^"]*"' | cut -d'"' -f4 \
        | grep -i "linux" | grep -i "$A" | head -n1 || true)"

    if [ -n "$url" ]; then
        say "Downloading: $url"
        curl -fL "$url" -o /tmp/tunx.dl
        install -m 0755 /tmp/tunx.dl "$BIN"
        rm -f /tmp/tunx.dl
    else
        say "No release binary — building from source."
        command -v go >/dev/null 2>&1 || {
            command -v apt-get >/dev/null || { err "No Go and no apt — install Go manually and retry."; return 1; }
            say "Installing Go via apt ..."
            apt-get update -qq
            apt-get install -y -qq golang-go
        }
        rm -rf /tmp/kiyatun-src
        git clone --depth 1 "https://github.com/$REPO.git" /tmp/kiyatun-src
        ( cd /tmp/kiyatun-src && go build -o /tmp/tunx.build . ) \
            || { err "Build failed — maybe Go is too old for go.mod. Install a newer Go and retry."; return 1; }
        install -m 0755 /tmp/tunx.build "$BIN"
        rm -f /tmp/tunx.build
    fi
    ok "tunx installed at $BIN"
    say "All flags: $BIN --help"
}

gen_key() {
    need_tunx || return 0
    echo
    "$BIN" keygen
    echo
    say "Keep this key — the exact same key goes on both servers."
}

setup_server() {
    need_tunx || return 0
    echo
    say "KiyaTun SERVER — entry side (e.g. Iran)"
    local listen expose transport key targs
    read -rp "Control port [9000]: " listen;  listen="${listen:-9000}"
    read -rp "Expose port  [443]: "  expose;  expose="${expose:-443}"
    read -rp "Transport raw/tls/ws/wss [raw]: " transport; transport="${transport:-raw}"
    read -rp "Key: " key
    [ -n "$key" ] || { err "Key cannot be empty."; return 0; }

    targs="$(trans_flag "$transport")"
    [ -n "$targs" ] || say "Note: -transport flag not detected — raw will be used. Check: tunx --help"

    mkdir -p "$CONF_DIR"
    cat > "$CONF_DIR/server.env" <<UNIT
TUNX_LISTEN=:$listen
TUNX_EXPOSE=:$expose
TUNX_TRANSPORT=$transport
TUNX_KEY=$key
UNIT
    chmod 600 "$CONF_DIR/server.env"

    cat > /etc/systemd/system/tunx-server.service <<UNIT
[Unit]
Description=KiyaTun Server (entry)
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=$CONF_DIR/server.env
ExecStart=$BIN server -l :$listen -expose :$expose -key $key $targs
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
UNIT

    systemctl daemon-reload
    systemctl enable tunx-server >/dev/null 2>&1
    systemctl restart tunx-server
    ok "Server is up — control :$listen, exposed :$expose"
    echo
    systemctl --no-pager -l status tunx-server 2>/dev/null | sed -n '1,6p' || true
}

setup_client() {
    need_tunx || return 0
    echo
    say "KiyaTun CLIENT — exit side (abroad)"
    local connect to transport key targs
    read -rp "Connect to (entry server, e.g. 1.2.3.4:9000): " connect
    read -rp "Forward to [127.0.0.1:443]: " to; to="${to:-127.0.0.1:443}"
    read -rp "Transport raw/tls/ws/wss [raw]: " transport; transport="${transport:-raw}"
    read -rp "Key: " key
    { [ -n "$connect" ] && [ -n "$key" ]; } || { err "Connect address and key are required."; return 0; }

    targs="$(trans_flag "$transport")"

    mkdir -p "$CONF_DIR"
    cat > "$CONF_DIR/client.env" <<UNIT
TUNX_CONNECT=$connect
TUNX_TO=$to
TUNX_TRANSPORT=$transport
TUNX_KEY=$key
UNIT
    chmod 600 "$CONF_DIR/client.env"

    cat > /etc/systemd/system/tunx-client.service <<UNIT
[Unit]
Description=KiyaTun Client (exit)
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=$CONF_DIR/client.env
ExecStart=$BIN client -connect $connect -to $to -key $key $targs
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
UNIT

    systemctl daemon-reload
    systemctl enable tunx-client >/dev/null 2>&1
    systemctl restart tunx-client
    ok "Client is up — $connect -> $to"
    echo
    systemctl --no-pager -l status tunx-client 2>/dev/null | sed -n '1,6p' || true
}

link_test() {
    need_tunx || return 0
    echo
    echo "  1) Run agent on THIS server (Ctrl+C to stop)"
    echo "  2) Run link test from THIS server"
    local choice port key peer
    read -rp "Choose [1/2]: " choice
    read -rp "Port [9000]: " port; port="${port:-9000}"
    read -rp "Key: " key
    case "$choice" in
        2)  read -rp "Peer (IP:PORT): " peer
            "$BIN" link -peer "$peer" -key "$key" ;;
        *)  "$BIN" agent -l ":$port" -key "$key" ;;
    esac
}

show_status() {
    systemctl --no-pager -l status tunx-server tunx-client 2>/dev/null || true
}

uninstall() {
    systemctl disable --now tunx-server tunx-client >/dev/null 2>&1 || true
    rm -f /etc/systemd/system/tunx-server.service /etc/systemd/system/tunx-client.service
    systemctl daemon-reload
    rm -f "$BIN"
    local a
    read -rp "Remove config/keys too ($CONF_DIR)? [y/N]: " a
    case "$a" in y|Y|yes) rm -rf "$CONF_DIR" ;; esac
    ok "KiyaTun removed."
}

while true; do
    echo
    echo -e "  ${B}=====================================${N}"
    echo -e "  ${B} KiyaTun${N}"
    echo -e "  ${B}=====================================${N}"
    echo   "   1) Install / update tunx"
    echo   "   2) Generate key"
    echo   "   3) Set up SERVER (entry — e.g. Iran)"
    echo   "   4) Set up CLIENT (exit — abroad)"
    echo   "   5) Link test (agent / link)"
    echo   "   6) Service status"
    echo   "   7) Uninstall"
    echo   "   0) Exit"
    echo
    read -rp "  Choose: " c
    case "$c" in
        1) install_tunx ;;
        2) gen_key ;;
        3) setup_server ;;
        4) setup_client ;;
        5) link_test ;;
        6) show_status ;;
        7) uninstall ;;
        0) exit 0 ;;
        *) echo "  ?" ;;
    esac
done
