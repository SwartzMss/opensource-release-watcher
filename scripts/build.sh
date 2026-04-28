#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVER_DIR="${ROOT_DIR}/backend"
WEB_DIR="${ROOT_DIR}/frontend"
BIN_DIR="${ROOT_DIR}/bin"
GOCACHE="${GOCACHE:-/tmp/go-build}"

mkdir -p "${BIN_DIR}"

echo "==> Running Go tests"
(
  cd "${SERVER_DIR}"
  GOCACHE="${GOCACHE}" go test ./...
)

echo "==> Building Go server"
(
  cd "${SERVER_DIR}"
  GOCACHE="${GOCACHE}" go build -o "${BIN_DIR}/opensource-release-watcher-server" ./cmd/server
)

echo "==> Checking frontend dependencies"
(
  cd "${WEB_DIR}"
  if [ ! -x node_modules/.bin/tsc ] || [ ! -x node_modules/.bin/vite ]; then
    if [ -f package-lock.json ]; then
      npm ci
    else
      npm install
    fi
  fi
)

echo "==> Building frontend"
(
  cd "${WEB_DIR}"
  npm run build
)

echo "==> Build complete"
echo "Server binary: ${BIN_DIR}/opensource-release-watcher-server"
echo "Frontend assets: ${WEB_DIR}/dist"
