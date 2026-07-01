#!/bin/sh
set -eu

SERVICE_NAME="mcmon-host"
REPO="Ctrl-Creeper/mcmon-host"
VERSION="latest"
BIN_PATH="/usr/local/bin/mcmon-host"
CONFIG_DIR="/etc/mcmon-host"
CONFIG_PATH="${CONFIG_DIR}/config.json"
DATA_DIR="/var/lib/mcmon-host"
DB_PATH="${DATA_DIR}/mcmon-host.db"
LISTEN=":9090"

print_host_summary() {
  port="$(printf '%s' "$LISTEN" | sed -n 's/.*:\([0-9][0-9]*\)$/\1/p')"
  if [ -n "$port" ]; then
    local_url="http://127.0.0.1:${port}"
  else
    local_url="http://127.0.0.1${LISTEN}"
  fi

  cat <<EOF

mcmon-host is installed and running.

Dashboard:
  ${local_url}
  If this server is behind a reverse proxy or public domain, open that external URL instead.

Important files:
  Binary:  ${BIN_PATH}
  Config:  ${CONFIG_PATH}
  Data:    ${DATA_DIR}
  DB:      ${DB_PATH}
  Service: /etc/systemd/system/${SERVICE_NAME}.service

Admin login:
  The dashboard admin token is stored in:
    ${CONFIG_PATH}

  View it with:
    sudo grep '"admin_token"' ${CONFIG_PATH}

Service commands:
  Status:  systemctl status ${SERVICE_NAME} --no-pager -l
  Logs:    journalctl -u ${SERVICE_NAME} -f
  Restart: sudo systemctl restart ${SERVICE_NAME}

Next steps:
  1. Open the dashboard.
  2. Paste the admin token from the config file.
  3. Create an agent/node in Agents.
  4. Copy the generated agent install command from the dashboard.

EOF
}

print_upgrade_summary() {
  cat <<EOF

mcmon-host upgraded and restarted.

Config:
  ${CONFIG_PATH}

Admin token:
  sudo grep '"admin_token"' ${CONFIG_PATH}

Service commands:
  Status: systemctl status ${SERVICE_NAME} --no-pager -l
  Logs:   journalctl -u ${SERVICE_NAME} -f

EOF
}

usage() {
  cat <<EOF
Usage: sudo sh install.sh [command] [options]

Commands:
  install      Install or overwrite mcmon-host (default)
  upgrade      Download the selected release and restart the service
  uninstall    Stop and remove mcmon-host, keeping /etc/mcmon-host and /var/lib/mcmon-host
  status       Show systemd service status
  logs         Follow service logs
  restart      Restart the service

Options:
  --version VERSION       Release tag to install. Defaults to latest.
  --repo OWNER/REPO       GitHub repo for release downloads. Defaults to ${REPO}.
  --listen ADDR          HTTP listen address for a new config. Defaults to ${LISTEN}.
EOF
}

COMMAND="install"
if [ "$#" -gt 0 ]; then
  case "$1" in
    install|upgrade|uninstall|status|logs|restart) COMMAND="$1"; shift ;;
  esac
fi

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --repo) REPO="$2"; shift 2 ;;
    --listen) LISTEN="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    echo "Please run as root, for example: sudo sh install.sh" >&2
    exit 1
  fi
}

require_systemd() {
  if ! command -v systemctl >/dev/null 2>&1; then
    echo "systemd is required for this installer" >&2
    exit 1
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64) echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
  esac
}

resolve_version() {
  if [ "$VERSION" = "latest" ]; then
    VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  fi
  if [ -z "$VERSION" ]; then
    echo "Unable to resolve latest version" >&2
    exit 1
  fi
}

ensure_config() {
  mkdir -p "$CONFIG_DIR" "$DATA_DIR"
  chmod 0750 "$CONFIG_DIR" "$DATA_DIR"
  if [ ! -f "$CONFIG_PATH" ]; then
    discovery_key="$(openssl rand -hex 16 2>/dev/null || date +%s | sha256sum | cut -c1-32)"
    admin_token="$(openssl rand -hex 16 2>/dev/null || date +%s%N | sha256sum | cut -c1-32)"
    cat > "$CONFIG_PATH" <<EOF
{
  "listen": "${LISTEN}",
  "db_path": "${DB_PATH}",
  "discovery_key": "${discovery_key}",
  "admin_token": "${admin_token}"
}
EOF
    chmod 0600 "$CONFIG_PATH"
  fi
}

install_binary() {
  if ! command -v curl >/dev/null 2>&1; then
    echo "curl is required" >&2
    exit 1
  fi
  resolve_version
  arch="$(detect_arch)"
  url="https://github.com/${REPO}/releases/download/${VERSION}/mcmon-host-linux-${arch}"
  tmp="$(mktemp)"
  echo "Downloading ${url}"
  curl -fL "$url" -o "$tmp"
  install -m 0755 "$tmp" "$BIN_PATH"
  rm -f "$tmp"
}

write_service() {
  cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=MCMon Host
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BIN_PATH} -config ${CONFIG_PATH}
WorkingDirectory=${DATA_DIR}
Restart=always
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
EOF
}

install_host() {
  require_root
  require_systemd
  ensure_config
  install_binary
  write_service
  systemctl daemon-reload
  systemctl enable --now "${SERVICE_NAME}.service"
  print_host_summary
}

upgrade_host() {
  require_root
  require_systemd
  install_binary
  systemctl restart "${SERVICE_NAME}.service"
  print_upgrade_summary
}

uninstall_host() {
  require_root
  require_systemd
  systemctl disable --now "${SERVICE_NAME}.service" >/dev/null 2>&1 || true
  rm -f "/etc/systemd/system/${SERVICE_NAME}.service" "$BIN_PATH"
  systemctl daemon-reload
  echo "mcmon-host removed. Config and data were kept:"
  echo "  ${CONFIG_DIR}"
  echo "  ${DATA_DIR}"
}

case "$COMMAND" in
  install) install_host ;;
  upgrade) upgrade_host ;;
  uninstall) uninstall_host ;;
  status) require_systemd; systemctl status "${SERVICE_NAME}.service" --no-pager -l ;;
  logs) require_systemd; journalctl -u "${SERVICE_NAME}.service" -f ;;
  restart) require_root; require_systemd; systemctl restart "${SERVICE_NAME}.service" ;;
esac
