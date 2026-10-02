#!/usr/bin/env bash
# KiyaTun installer / manager
# https://github.com/<your-username>/<your-repo>   <- fill in after you push
set -euo pipefail

NAME="KiyaTun"
REPO_URL="https://github.com/kiyarash-ns/KiyaTun"   # EDIT ME
BIN_PATH="/usr/local/bin/tunx"
CONF_DIR="/etc/tunx"
SERVER_ENV="$CONF_DIR/server.env"
CLIENT_ENV="$CONF_DIR/client.env"
SERVER_UNIT="/etc/systemd/system/tunx-server.service"
CLIENT_UNIT="/etc/systemd/system/tunx-client.service"

YEL='\033[1;33m'
BLU='\033[1;34m'
GRN='\033[1;32m'
RED='\033[1;31m'
NC='\033[0m'

need_root() {
  if [ "$(id -u)" -ne 0 ]; then
    echo -e "${RED}Run this script as root (sudo).${NC}"
    exit 1
  fi
}

logo() {
  echo -e "${BLU}  _  ___               _____         ${YEL}  _____           ${NC}"
  echo -e "${BLU} | |/ (_)_   _  __ _  |_   _|   _ _ __${YEL} |_   _|   _ _ __  ${NC}"
  echo -e "${BLU} | ' /| | | | |/ _\` |   | || | | | '_ \\\\${YEL} | || | | | '_ \\ ${NC}"
  echo -e "${BLU} | . \\| | |_| | (_| |   | || |_| | | | ${YEL}| || |_| | | | |${NC}"
  echo -e "${BLU} |_|\\_\\_|\\__, |\\__,_|   |_| \\__,_|_| |_|${YEL}|_| \\__,_|_| |_|${NC}"
  echo -e "${BLU}         |___/${NC}  ${YEL}by Kiyarash${NC}"
  echo
}

ensure_binary() {
  if [ -x "$BIN_PATH" ]; then
    return
  fi
  local arch
  arch=$(uname -m)
  case "$arch" in
    x86_64) arch=amd64 ;;
    aarch64) arch=arm64 ;;
  esac
  echo "tunx not found -- trying release binary for linux/$arch ..."
  if curl -fsSL "$REPO_URL/releases/latest/download/tunx-linux-$arch" -o "$BIN_PATH"; then
    chmod +x "$BIN_PATH"
    return
  fi
  rm -f "$BIN_PATH"
  echo "No release binary -- building from source."
  command -v git >/dev/null 2>&1 || { echo -e "${RED}git is required. Install it and re-run.${NC}"; exit 1; }
  command -v go >/dev/null 2>&1 || { echo -e "${RED}Go is required to build. Install Go and re-run.${NC}"; exit 1; }
  local tmp
  tmp=$(mktemp -d)
  if ! git clone --depth 1 "$REPO_URL.git" "$tmp" || ! ( cd "$tmp" && go build -o "$BIN_PATH" . ); then
    rm -rf "$tmp"
    echo -e "${RED}Build failed.${NC}"
    exit 1
  fi
  rm -rf "$tmp"
  chmod +x "$BIN_PATH"
}

ensure_user_and_dirs() {
  id tunx >/dev/null 2>&1 || useradd -r -s /usr/sbin/nologin tunx
  mkdir -p "$CONF_DIR"
}

install_server() {
  ensure_binary
  ensure_user_and_dirs

  echo "Transport:"
  echo "  1) raw  (direct connection, no disguise)"
  echo "  2) tls  (direct connection, wrapped in real TLS)"
  echo "  3) CDN - WS   (behind a CDN like Arvan/ArvanCloud/Cloudflare; CDN handles TLS, plain HTTP to this server)"
  echo "  4) CDN - WSS  (behind a CDN; TLS all the way through to this server -- needs a real certificate for the domain)"
  echo "  5) udp  (custom reliable transport over UDP, for when TCP specifically is throttled)"
  read -rp "choice [1-5, default 1]: " t
  case "${t:-1}" in
    2) transport=tls ;;
    3) transport=ws ;;
    4) transport=wss ;;
    5) transport=udp ;;
    *) transport=raw ;;
  esac

  hop_flags=""
  read -rp "Enable port hopping (active port rotates with time+key)? [y/N]: " hopans
  if [[ "${hopans:-N}" =~ ^[Yy]$ ]]; then
    read -rp "Hop base port [20000]: " hb; hb=${hb:-20000}
    read -rp "Hop port count [200]: " hc; hc=${hc:-200}
    read -rp "Hop window in seconds [60]: " hw; hw=${hw:-60}
    hop_flags="-hop -hop-base $hb -hop-count $hc -hop-window ${hw}s"
    echo "Note these same values down -- the client side must use the same ones."
  fi

  cdn_domain=""
  if [ "$transport" = "ws" ] || [ "$transport" = "wss" ]; then
    echo
    echo "CDN mode: this server is the CDN's 'origin'. In your CDN panel, set the"
    echo "origin address to this server's real IP, and the origin port to what"
    echo "you enter below."
    read -rp "Origin port the CDN should forward to [9443]: " oport; oport=${oport:-9443}
    listen=":$oport"
    read -rp "The domain you set up in the CDN (for your records; client needs it as SNI/host): " cdn_domain
    echo "Reminder: enable WebSocket support for this domain in your CDN panel"
    echo "(most CDNs, Arvan included, have a separate on/off switch for WebSocket)."
  else
    read -rp "Control listen address [:9000]: " listen; listen=${listen:-:9000}
  fi

  read -rp "Exposed address(es) for end users, comma-separated [:443]: " expose; expose=${expose:-:443}

  read -rp "Generate a new shared key? [Y/n]: " genkey
  if [[ "${genkey:-Y}" =~ ^[Yy]$ ]] || [ -z "${genkey:-}" ]; then
    key=$("$BIN_PATH" keygen)
    echo -e "${GRN}Shared key (copy this to the client side!):${NC} $key"
  else
    read -rp "Paste the shared key: " key
  fi

  cert=""; keyfile=""
  if [ "$transport" = "tls" ] || [ "$transport" = "wss" ]; then
    read -rp "Path to certificate (.pem/.crt), blank = self-signed: " cert
    if [ -n "$cert" ]; then
      read -rp "Path to private key (.key): " keyfile
    fi
  fi

  cat > "$SERVER_ENV" <<EOF
TUNX_LISTEN=$listen
TUNX_EXPOSE=$expose
TUNX_KEY=$key
TUNX_TRANSPORT=$transport
TUNX_CERT=$cert
TUNX_KEYFILE=$keyfile
TUNX_HOP_FLAGS=$hop_flags
# CDN domain (reference only, not read by tunx): $cdn_domain
EOF

  write_server_unit
  systemctl daemon-reload
  systemctl enable --now tunx-server
  echo -e "${GRN}Server installed and started.${NC}"
  if [ -n "${cdn_domain:-}" ]; then
    echo -e "${YEL}CDN checklist:${NC} in your CDN panel -- point DNS for $cdn_domain at the CDN,"
    echo "set origin = this server's IP : $oport, enable proxy/CDN mode, enable WebSocket."
  fi
  systemctl --no-pager status tunx-server || true
}

write_server_unit() {
  cat > "$SERVER_UNIT" <<'EOF'
[Unit]
Description=KiyaTun tunnel server (entry side)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=-/etc/tunx/server.env
ExecStart=/usr/local/bin/tunx server -l ${TUNX_LISTEN} -expose ${TUNX_EXPOSE} -key ${TUNX_KEY} -transport ${TUNX_TRANSPORT} -cert ${TUNX_CERT} -key-file ${TUNX_KEYFILE} $TUNX_HOP_FLAGS
Restart=on-failure
RestartSec=2
User=tunx
AmbientCapabilities=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF
}

quick_server() {
  ensure_binary
  ensure_user_and_dirs
  echo -e "${YEL}Quick setup -- server (entry side, e.g. Iran)${NC}"
  echo "Plain settings: no CDN, no port hopping, no custom TLS. You can do"
  echo "those later from the Advanced menu without reinstalling."
  echo
  read -rp "Control port [9000]: " cport; cport=${cport:-9000}
  read -rp "Exposed port for end users [443]: " eport; eport=${eport:-443}
  read -rp "Shared key (press Enter to generate a new one): " key
  if [ -z "$key" ]; then
    key=$("$BIN_PATH" keygen)
    echo
    echo -e "${GRN}Shared key -- copy this, you will need it on the abroad server:${NC}"
    echo -e "${GRN}$key${NC}"
    echo
    read -rp "Press Enter once you've saved it: " _
  fi

  cat > "$SERVER_ENV" <<EOF
TUNX_LISTEN=:$cport
TUNX_EXPOSE=:$eport
TUNX_KEY=$key
TUNX_TRANSPORT=raw
TUNX_CERT=
TUNX_KEYFILE=
TUNX_HOP_FLAGS=
EOF
  write_server_unit
  systemctl daemon-reload
  systemctl enable --now tunx-server
  echo -e "${GRN}Server installed and started.${NC}"
  systemctl --no-pager status tunx-server || true
}

install_client() {
  ensure_binary
  ensure_user_and_dirs

  echo "Transport:"
  echo "  1) raw  (direct connection, no disguise)"
  echo "  2) tls  (direct connection, wrapped in real TLS)"
  echo "  3) CDN - WS   (connect through a CDN like Arvan/ArvanCloud/Cloudflare; CDN handles TLS)"
  echo "  4) CDN - WSS  (connect through a CDN, TLS all the way to the origin server)"
  echo "  5) udp  (custom reliable transport over UDP; must match the server)"
  read -rp "choice [1-5, default 1]: " t
  case "${t:-1}" in
    2) transport=tls ;;
    3) transport=ws ;;
    4) transport=wss ;;
    5) transport=udp ;;
    *) transport=raw ;;
  esac

  hop_flags=""
  read -rp "Port hopping enabled on the server? [y/N]: " hopans
  if [[ "${hopans:-N}" =~ ^[Yy]$ ]]; then
    read -rp "Hop base port (same as server) [20000]: " hb; hb=${hb:-20000}
    read -rp "Hop port count (same as server) [200]: " hc; hc=${hc:-200}
    read -rp "Hop window in seconds (same as server) [60]: " hw; hw=${hw:-60}
    hop_flags="-hop -hop-base $hb -hop-count $hc -hop-window ${hw}s"
  fi

  sni=""
  if [ "$transport" = "ws" ] || [ "$transport" = "wss" ]; then
    echo
    echo "CDN mode: connect to the CDN's edge, not to the origin server's IP."
    read -rp "CDN domain (e.g. tunnel.yourdomain.com): " cdn_domain
    read -rp "Edge port the CDN listens on [443]: " eport; eport=${eport:-443}
    connect="$cdn_domain:$eport"
    sni="$cdn_domain"
  elif [ "$transport" = "tls" ]; then
    read -rp "Server control address (IP:port), e.g. 1.2.3.4:9000 : " connect
    read -rp "SNI (only if the server uses a real certificate for a domain, else leave blank): " sni
  else
    read -rp "Server control address (IP:port), e.g. 1.2.3.4:9000 : " connect
  fi

  read -rp "Local address to forward to [127.0.0.1:443]: " to; to=${to:-127.0.0.1:443}
  read -rp "Shared key (same as the server): " key

  cat > "$CLIENT_ENV" <<EOF
TUNX_CONNECT=$connect
TUNX_TO=$to
TUNX_KEY=$key
TUNX_TRANSPORT=$transport
TUNX_SNI=$sni
TUNX_HOP_FLAGS=$hop_flags
EOF

  write_client_unit
  systemctl daemon-reload
  systemctl enable --now tunx-client
  echo -e "${GRN}Client installed and started.${NC}"
  systemctl --no-pager status tunx-client || true
}

write_client_unit() {
  cat > "$CLIENT_UNIT" <<'EOF'
[Unit]
Description=KiyaTun tunnel client (exit side)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=-/etc/tunx/client.env
ExecStart=/usr/local/bin/tunx client -connect ${TUNX_CONNECT} -to ${TUNX_TO} -key ${TUNX_KEY} -transport ${TUNX_TRANSPORT} -sni ${TUNX_SNI} $TUNX_HOP_FLAGS
Restart=on-failure
RestartSec=2
User=tunx
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF
}

quick_client() {
  ensure_binary
  ensure_user_and_dirs
  echo -e "${YEL}Quick setup -- client (exit side, e.g. abroad)${NC}"
  echo "Plain settings: no CDN, no port hopping, no custom TLS -- must match"
  echo "a server set up with Quick setup too."
  echo
  read -rp "Iran server address (IP:port, e.g. 1.2.3.4:9000): " connect
  read -rp "Local address to forward to [127.0.0.1:443]: " to; to=${to:-127.0.0.1:443}
  read -rp "Shared key (shown when you installed the server): " key

  cat > "$CLIENT_ENV" <<EOF
TUNX_CONNECT=$connect
TUNX_TO=$to
TUNX_KEY=$key
TUNX_TRANSPORT=raw
TUNX_SNI=
TUNX_HOP_FLAGS=
EOF
  write_client_unit
  systemctl daemon-reload
  systemctl enable --now tunx-client
  echo -e "${GRN}Client installed and started.${NC}"
  systemctl --no-pager status tunx-client || true
}

get_certificate() {
  if ! command -v certbot >/dev/null 2>&1; then
    echo "Installing certbot..."
    if command -v apt-get >/dev/null 2>&1; then
      apt-get update -y && apt-get install -y certbot
    elif command -v yum >/dev/null 2>&1; then
      yum install -y certbot
    else
      echo -e "${RED}Unsupported distro: install certbot manually.${NC}"; return 1
    fi
  fi
  read -rp "Domain (must already point A/AAAA at this server's real IP -- not behind a CDN during issuance): " domain
  read -rp "Email for renewal notices: " email
  certbot certonly --standalone --non-interactive --agree-tos -m "$email" -d "$domain"
  local cert="/etc/letsencrypt/live/$domain/fullchain.pem"
  local pkey="/etc/letsencrypt/live/$domain/privkey.pem"
  echo -e "${GRN}Certificate: $cert${NC}"
  echo -e "${GRN}Private key: $pkey${NC}"
  if [ -f "$SERVER_ENV" ]; then
    sed -i "s#^TUNX_CERT=.*#TUNX_CERT=$cert#; s#^TUNX_KEYFILE=.*#TUNX_KEYFILE=$pkey#" "$SERVER_ENV"
    if grep -q '^TUNX_TRANSPORT=raw$' "$SERVER_ENV" 2>/dev/null; then
      sed -i "s#^TUNX_TRANSPORT=.*#TUNX_TRANSPORT=tls#" "$SERVER_ENV"
    fi
    systemctl restart tunx-server 2>/dev/null || true
    echo -e "${GRN}server.env updated and service restarted.${NC}"
  fi
  echo "Certbot sets up auto-renewal via a systemd timer or cron automatically."
  echo "If you switch the domain to CDN-proxied (Arvan etc.) afterwards, issuance"
  echo "must be redone with the domain pointed directly at this server first,"
  echo "or use your CDN's own origin-certificate feature instead."
}

manage_ports() {
  [ -f "$SERVER_ENV" ] || { echo "Server is not installed yet."; return; }
  # shellcheck disable=SC1090
  source "$SERVER_ENV"
  echo "Current exposed address(es): $TUNX_EXPOSE"
  read -rp "New comma-separated list: " new
  sed -i "s#^TUNX_EXPOSE=.*#TUNX_EXPOSE=$new#" "$SERVER_ENV"
  systemctl restart tunx-server
  echo -e "${GRN}Updated and restarted.${NC}"
}

generate_key() {
  ensure_binary
  if [ -f "$SERVER_ENV" ] && grep -q '^TUNX_KEY=' "$SERVER_ENV"; then
    echo -e "${YEL}A server is already installed here. Its current key:${NC}"
    grep '^TUNX_KEY=' "$SERVER_ENV" | cut -d= -f2-
    read -rp "Generate a NEW key instead? (does not change the installed server) [y/N]: " c
    [[ "${c:-N}" =~ ^[Yy]$ ]] || return
  fi
  local key
  key=$("$BIN_PATH" keygen)
  echo
  echo -e "${GRN}Shared key (use the SAME key on both servers):${NC}"
  echo -e "${GRN}$key${NC}"
  echo
  echo "Paste it when the installer asks for the key (server: Enter a key / Advanced: answer n"
  echo "to 'Generate a new shared key?'; client: 'Shared key'). Never post it publicly."
}

status_logs() {
  echo "1) server status  2) client status  3) server logs  4) client logs"
  read -rp "choice: " c
  case "$c" in
    1) systemctl --no-pager status tunx-server ;;
    2) systemctl --no-pager status tunx-client ;;
    3) journalctl -u tunx-server -n 100 --no-pager ;;
    4) journalctl -u tunx-client -n 100 --no-pager ;;
  esac
}

restart_services() {
  systemctl restart tunx-server 2>/dev/null && echo "server restarted" || true
  systemctl restart tunx-client 2>/dev/null && echo "client restarted" || true
}

uninstall_all() {
  read -rp "This removes tunx services, binary and config. Continue? [y/N]: " c
  [[ "$c" =~ ^[Yy]$ ]] || return
  systemctl disable --now tunx-server 2>/dev/null || true
  systemctl disable --now tunx-client 2>/dev/null || true
  rm -f "$SERVER_UNIT" "$CLIENT_UNIT"
  systemctl daemon-reload
  rm -rf "$CONF_DIR" "$BIN_PATH"
  echo -e "${GRN}Removed.${NC}"
}

advanced_menu() {
  echo
  echo -e "${YEL}-- Advanced --${NC}"
  echo "1) Install server, custom (CDN / TLS / UDP / port hopping)"
  echo "2) Install client, custom (CDN / TLS / UDP / port hopping)"
  echo "3) Get a TLS certificate for a domain (certbot)"
  echo "4) Change exposed port(s)"
  echo "0) Back"
  read -rp "> " c
  case "$c" in
    1) install_server ;;
    2) install_client ;;
    3) get_certificate ;;
    4) manage_ports ;;
    0) return ;;
    *) echo "invalid choice" ;;
  esac
}

main_menu() {
  need_root
  logo
  echo "1) Quick setup -- server (Iran / entry side)"
  echo "2) Quick setup -- client (abroad / exit side)"
  echo "3) Advanced setup (CDN, TLS certificate, UDP, port hopping)"
  echo "4) Generate shared key"
  echo "5) Status / logs"
  echo "6) Restart service(s)"
  echo "7) Uninstall"
  echo "0) Exit"
  read -rp "> " choice
  case "$choice" in
    1) quick_server ;;
    2) quick_client ;;
    3) advanced_menu ;;
    4) generate_key ;;
    5) status_logs ;;
    6) restart_services ;;
    7) uninstall_all ;;
    0) exit 0 ;;
    *) echo "invalid choice" ;;
  esac
}

main_menu
