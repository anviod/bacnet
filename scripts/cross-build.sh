#!/usr/bin/env bash
# Cross-compile for edge (ARMv7+) and common desktop targets. No cgo.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p bin

build_one() {
  local label="$1"
  shift
  echo "==> $label"
  env "$@" go build ./...
  env "$@" go build -o "bin/room-simulator-${label}" ./cmd/room-simulator
  env "$@" go build -o "bin/objlist-probe-${label}" ./cmd/objlist-probe
}

build_one linux-armv7  CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7
build_one linux-arm64  CGO_ENABLED=0 GOOS=linux GOARCH=arm64
build_one linux-amd64  CGO_ENABLED=0 GOOS=linux GOARCH=amd64
build_one windows-amd64.exe CGO_ENABLED=0 GOOS=windows GOARCH=amd64

echo "OK — artifacts in bin/"
ls -la bin/
