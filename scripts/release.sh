#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVER_DIR="${ROOT_DIR}/backend"
WEB_DIR="${ROOT_DIR}/frontend"
RELEASE_DIR="${ROOT_DIR}/release"
WORK_DIR="${RELEASE_DIR}/_work"
GOCACHE="${GOCACHE:-/tmp/go-build}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Command '$1' not found. Please install it before running release.sh." >&2
    exit 1
  fi
}

version_from_git() {
  if git -C "$ROOT_DIR" describe --tags --always --dirty >/dev/null 2>&1; then
    git -C "$ROOT_DIR" describe --tags --always --dirty
    return
  fi
  date +%Y%m%d%H%M%S
}

to_windows_path() {
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$1"
  else
    printf '%s\n' "$1"
  fi
}

zip_dir() {
  local source_dir="$1"
  local output_file="$2"
  local parent_dir package_name
  parent_dir="$(dirname "$source_dir")"
  package_name="$(basename "$source_dir")"

  if command -v zip >/dev/null 2>&1; then
    (
      cd "$parent_dir"
      zip -qr "$output_file" "$package_name"
    )
    return
  fi

  if command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoProfile -Command "Compress-Archive -Path '$(to_windows_path "$source_dir")' -DestinationPath '$(to_windows_path "$output_file")' -Force"
    return
  fi

  echo "Command 'zip' not found and powershell.exe is unavailable. Cannot create Windows zip package." >&2
  exit 1
}

copy_common_files() {
  local package_dir="$1"
  mkdir -p "$package_dir/frontend"
  cp -R "$WEB_DIR/dist" "$package_dir/frontend/dist"
  cp "$ROOT_DIR/.env.example" "$package_dir/.env.example"
  cp "$ROOT_DIR/README.md" "$package_dir/README.md"
  cp "$ROOT_DIR/LICENSE" "$package_dir/LICENSE"
  mkdir -p "$package_dir/scripts"
  cp "$ROOT_DIR/scripts/deploy.sh" "$package_dir/scripts/deploy.sh"
}

build_frontend() {
  echo "==> Building frontend"
  require_cmd npm
  (
    cd "$WEB_DIR"
    if [ ! -x node_modules/.bin/tsc ] || [ ! -x node_modules/.bin/vite ]; then
      if [ -f package-lock.json ]; then
        npm ci
      else
        npm install
      fi
    fi
    npm run build
  )
}

build_backend_tests() {
  echo "==> Running Go tests"
  require_cmd go
  (
    cd "$SERVER_DIR"
    GOCACHE="$GOCACHE" go test ./...
  )
}

package_target() {
  local goos="$1"
  local goarch="$2"
  local ext="$3"
  local archive_ext="$4"
  local version="$5"
  local target="${goos}-${goarch}"
  local package_name="opensource-release-watcher-${version}-${target}"
  local package_dir="${WORK_DIR}/${package_name}"
  local binary_name="opensource-release-watcher-server${ext}"

  echo "==> Building ${target}"
  rm -rf "$package_dir"
  mkdir -p "$package_dir/bin"
  copy_common_files "$package_dir"
  (
    cd "$SERVER_DIR"
    GOOS="$goos" GOARCH="$goarch" GOCACHE="$GOCACHE" go build -o "$package_dir/bin/$binary_name" ./cmd/server
  )

  echo "==> Packaging ${package_name}.${archive_ext}"
  if [[ "$archive_ext" == "tar.gz" ]]; then
    tar -czf "$RELEASE_DIR/${package_name}.tar.gz" -C "$WORK_DIR" "$package_name"
  else
    zip_dir "$package_dir" "$RELEASE_DIR/${package_name}.zip"
  fi
}

write_checksums() {
  echo "==> Writing checksums"
  (
    cd "$RELEASE_DIR"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum opensource-release-watcher-* > SHA256SUMS
    elif command -v shasum >/dev/null 2>&1; then
      shasum -a 256 opensource-release-watcher-* > SHA256SUMS
    else
      echo "sha256sum/shasum not found; skipping SHA256SUMS" >&2
    fi
  )
}

main() {
  local version="${1:-$(version_from_git)}"
  version="${version#v}"

  mkdir -p "$RELEASE_DIR"
  rm -rf "$WORK_DIR"
  mkdir -p "$WORK_DIR"
  rm -f "$RELEASE_DIR"/opensource-release-watcher-* "$RELEASE_DIR"/SHA256SUMS

  build_frontend
  build_backend_tests

  package_target linux amd64 "" tar.gz "$version"
  package_target linux arm64 "" tar.gz "$version"
  package_target windows amd64 ".exe" zip "$version"

  write_checksums
  rm -rf "$WORK_DIR"

  echo "==> Release complete"
  echo "Artifacts:"
  ls -1 "$RELEASE_DIR"
}

main "$@"
