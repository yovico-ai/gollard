#!/bin/bash
set -euo pipefail

# Local (non-Docker) build of a gollard .deb. Requires `go` and `fpm` on PATH.

cd "$(dirname "$0")/.."

VERSION="${GOLLARD_VERSION:-0.6.3.0}"
INSTALL_DIR="$(mktemp -d)"
trap 'rm -rf "$INSTALL_DIR"' EXIT

CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "${INSTALL_DIR}/usr/local/bin/gollard" ./cmd/gollard

fpm -s dir -t deb -n gollard -v "$VERSION" -C "$INSTALL_DIR" \
    --description "gollard - Database management for pedantic people." \
    --license MIT \
    --architecture "$(dpkg --print-architecture 2>/dev/null || echo amd64)" \
    --force \
    usr/local/bin/gollard

echo "Built: gollard_${VERSION}_$(dpkg --print-architecture 2>/dev/null || echo amd64).deb"
