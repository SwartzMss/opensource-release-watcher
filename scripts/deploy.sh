#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ENV_FILE:-$ROOT/.env}"
FRONTEND_BUILD="$ROOT/frontend/dist"
BIN_PATH="$ROOT/bin/opensource-release-watcher-server"
STATIC_DEST="${STATIC_DEST:-/var/www/opensource-release-watcher}"
SERVICE_NAME="${SERVICE_NAME:-opensource-release-watcher}"
BACKEND_UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"
ORIG_USER="${SUDO_USER:-$(id -un)}"
ORIG_HOME="$(getent passwd "$ORIG_USER" | cut -d: -f6)"

ensure_root() {
  if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
    echo "This script must be run as root (use sudo)." >&2
    exit 1
  fi
}

load_env_file() {
  if [[ -f "$ENV_FILE" ]]; then
    # shellcheck disable=SC1090
    set -a && source "$ENV_FILE" && set +a
  fi
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Command '$1' not found. Please install it before running deploy.sh." >&2
    exit 1
  fi
}

setup_user_toolchain() {
  if [[ -n "$ORIG_HOME" ]]; then
    export HOME="$ORIG_HOME"
    export PATH="/usr/local/go/bin:$ORIG_HOME/go/bin:$PATH"
    if [[ -s "$ORIG_HOME/.nvm/nvm.sh" ]]; then
      export NVM_DIR="${NVM_DIR:-$ORIG_HOME/.nvm}"
      # shellcheck disable=SC1090
      source "$ORIG_HOME/.nvm/nvm.sh"
    fi
  fi
}

run_as_original_user() {
  if [[ ${EUID:-$(id -u)} -eq 0 && "$ORIG_USER" != "root" ]]; then
    sudo -H -u "$ORIG_USER" env \
      HOME="$ORIG_HOME" \
      PATH="$PATH" \
      GOCACHE="${GOCACHE:-/tmp/go-build}" \
      "$@"
  else
    "$@"
  fi
}

load_env_file
require_cmd bash

usage() {
  echo "Usage: $0 [dev|install|start|stop|restart|status|build|clean-static|uninstall]" >&2
  exit 1
}

ACTION="${1:-start}"
shift || true

case "$ACTION" in
  dev|build)
    setup_user_toolchain
    ;;
  install|start|restart)
    ensure_root
    setup_user_toolchain
    require_cmd nginx
    require_cmd rsync
    ;;
  stop|status|clean-static|uninstall)
    ensure_root
    ;;
  *)
    usage
    ;;
esac

SERVICE_USER="${SERVICE_USER:-$ORIG_USER}"
SERVICE_GROUP="${SERVICE_GROUP:-$SERVICE_USER}"
NGINX_SERVICE="${NGINX_SERVICE:-nginx}"
CLIENT_MAX_BODY_SIZE="${CLIENT_MAX_BODY_SIZE:-200M}"
SERVER_ADDR="${SERVER_ADDR:-127.0.0.1:8000}"
DB_PATH="${DB_PATH:-$ROOT/data/watcher.db}"

build() {
  setup_user_toolchain
  require_cmd go
  require_cmd npm
  run_as_original_user bash "$ROOT/scripts/build.sh"
}

dev() {
  local host port backend_url server_pid
  host="${DEV_HOST:-127.0.0.1}"
  port="${DEV_PORT:-5173}"
  backend_url="${BACKEND_URL:-http://${SERVER_ADDR}}"

  cleanup() {
    if [[ -n "${server_pid:-}" ]] && kill -0 "$server_pid" >/dev/null 2>&1; then
      kill "$server_pid"
      wait "$server_pid" >/dev/null 2>&1 || true
    fi
  }
  trap cleanup EXIT INT TERM

  echo "==> Starting backend on ${SERVER_ADDR}"
  (
    cd "$ROOT/backend"
    setup_user_toolchain
    require_cmd go
    GOCACHE="${GOCACHE:-/tmp/go-build}" SERVER_ADDR="$SERVER_ADDR" DB_PATH="$DB_PATH" go run ./cmd/server
  ) &
  server_pid=$!

  sleep 1

  echo "==> Starting frontend at http://${host}:${port}"
  (
    cd "$ROOT/frontend"
    setup_user_toolchain
    require_cmd npm
    if [[ ! -x node_modules/.bin/vite ]]; then
      if [[ -f package-lock.json ]]; then
        npm ci
      else
        npm install
      fi
    fi
    BACKEND_URL="$backend_url" npm run dev -- --host "$host" --port "$port"
  )
}

sync_static_assets() {
  if [[ ! -d "$FRONTEND_BUILD" ]]; then
    echo "frontend build not found at $FRONTEND_BUILD; run build first" >&2
    exit 1
  fi
  mkdir -p "$STATIC_DEST"
  rsync -a --delete "$FRONTEND_BUILD"/ "$STATIC_DEST"/
  chown -R "$SERVICE_USER:$SERVICE_GROUP" "$STATIC_DEST"
}

clean_static() {
  if [[ -d "$STATIC_DEST" ]]; then
    rm -rf "$STATIC_DEST"
    echo "Removed static assets at $STATIC_DEST"
  else
    echo "Static directory $STATIC_DEST does not exist"
  fi
}

prepare_runtime_dirs() {
  local db_dir
  db_dir="$(dirname "$DB_PATH")"
  mkdir -p "$db_dir"
  chown "$SERVICE_USER:$SERVICE_GROUP" "$db_dir"
}

read_nginx_vars() {
  DOMAIN="${DOMAIN:-${DEPLOY_DOMAIN:-}}"
  EXTERNAL_PORT="${EXTERNAL_PORT:-${DEPLOY_EXTERNAL_PORT:-443}}"
  CERT_PATH="${CERT_PATH:-${DEPLOY_CERT_PATH:-}}"
  KEY_PATH="${KEY_PATH:-${DEPLOY_KEY_PATH:-}}"
  BACKEND_BIND="${BACKEND_BIND:-${SERVER_ADDR}}"

  if [[ -z "${DOMAIN:-}" || -z "${CERT_PATH:-}" || -z "${KEY_PATH:-}" ]]; then
    cat >&2 <<EOF
nginx requires DOMAIN, CERT_PATH, KEY_PATH.
Provide them via environment variables or $ENV_FILE, e.g.:
  DOMAIN=watcher.example.com
  CERT_PATH=/etc/letsencrypt/live/watcher/fullchain.pem
  KEY_PATH=/etc/letsencrypt/live/watcher/privkey.pem
EOF
    exit 1
  fi
}

configure_nginx() {
  read_nginx_vars
  sync_static_assets

  local nginx_conf="/etc/nginx/sites-available/${SERVICE_NAME}.conf"
  cat >"$nginx_conf" <<EOF
server {
    listen 80;
    server_name $DOMAIN;
    return 301 https://\$host:$EXTERNAL_PORT\$request_uri;
}

server {
    listen $EXTERNAL_PORT ssl;
    server_name $DOMAIN;

    ssl_certificate $CERT_PATH;
    ssl_certificate_key $KEY_PATH;
    client_max_body_size $CLIENT_MAX_BODY_SIZE;

    root $STATIC_DEST;
    index index.html;

    location /api/ {
        proxy_pass http://$BACKEND_BIND;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_http_version 1.1;
    }

    location /healthz {
        proxy_pass http://$BACKEND_BIND;
        proxy_set_header Host \$host;
    }

    location / {
        try_files \$uri /index.html;
    }
}
EOF

  ln -sf "$nginx_conf" "/etc/nginx/sites-enabled/${SERVICE_NAME}.conf"
}

write_unit_files() {
  if [[ ! -x "$BIN_PATH" ]]; then
    echo "server binary not found at $BIN_PATH; run build first" >&2
    exit 1
  fi

  tee "$BACKEND_UNIT_PATH" >/dev/null <<EOF
[Unit]
Description=opensource-release-watcher backend
After=network-online.target
Wants=network-online.target

[Service]
Environment="SERVER_ADDR=$SERVER_ADDR"
Environment="DB_PATH=$DB_PATH"
EnvironmentFile=-$ENV_FILE
WorkingDirectory=$ROOT
ExecStart=$BIN_PATH
Restart=on-failure
RestartSec=3
User=$SERVICE_USER
Group=$SERVICE_GROUP

[Install]
WantedBy=multi-user.target
EOF
}

start_services() {
  systemctl daemon-reload
  systemctl enable "${SERVICE_NAME}.service" >/dev/null
  systemctl start "${SERVICE_NAME}.service"
}

stop_services() {
  systemctl stop "${SERVICE_NAME}.service" >/dev/null 2>&1 || true
}

status_services() {
  systemctl status "${SERVICE_NAME}.service" --no-pager
}

reload_nginx() {
  nginx -t
  systemctl reload "${NGINX_SERVICE}.service"
}

remove_systemd_unit() {
  systemctl stop "${SERVICE_NAME}.service" >/dev/null 2>&1 || true
  systemctl disable "${SERVICE_NAME}.service" >/dev/null 2>&1 || true
  rm -f "$BACKEND_UNIT_PATH"
  systemctl daemon-reload
}

remove_nginx_config() {
  rm -f "/etc/nginx/sites-enabled/${SERVICE_NAME}.conf"
  rm -f "/etc/nginx/sites-available/${SERVICE_NAME}.conf"
  systemctl reload "${NGINX_SERVICE}.service" >/dev/null 2>&1 || true
}

uninstall() {
  echo "Stopping services..."
  stop_services
  echo "Removing systemd unit..."
  remove_systemd_unit
  echo "Removing nginx config..."
  remove_nginx_config
  if [[ -d "$STATIC_DEST" ]]; then
    echo "Removing static assets at $STATIC_DEST"
    rm -rf "$STATIC_DEST"
  fi
  echo "Uninstall completed."
}

case "$ACTION" in
  dev)
    dev
    ;;
  install)
    stop_services
    build
    write_unit_files
    configure_nginx
    prepare_runtime_dirs
    start_services
    reload_nginx
    ;;
  build)
    build
    ;;
  start)
    build
    write_unit_files
    configure_nginx
    prepare_runtime_dirs
    start_services
    reload_nginx
    ;;
  stop)
    stop_services
    ;;
  restart)
    stop_services
    build
    write_unit_files
    configure_nginx
    prepare_runtime_dirs
    start_services
    reload_nginx
    ;;
  status)
    status_services
    ;;
  clean-static)
    clean_static
    ;;
  uninstall)
    uninstall
    ;;
  *)
    usage
    ;;
esac
