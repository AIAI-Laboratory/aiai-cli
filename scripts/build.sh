#!/usr/bin/env bash
set -euo pipefail

# scripts/build.sh - Builds the aiai binary
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${ROOT_DIR}/bin"
OUTPUT="${BIN_DIR}/aiai"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"

mkdir -p "${BIN_DIR}"

echo "Building aiai (${VERSION}) into ${OUTPUT}..."
go build \
  -trimpath \
  -ldflags "-s -w -X main.version=${VERSION}" \
  -o "${OUTPUT}" \
  "${ROOT_DIR}/cmd/aiai"

echo "Build complete: ${OUTPUT}"
