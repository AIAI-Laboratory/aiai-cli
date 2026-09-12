#!/usr/bin/env bash
set -euo pipefail

# scripts/test.sh - Runs test suites, vet, and race detector
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "==> Running go vet..."
go vet ./...

echo "==> Running go test with race detector..."
go test -race -v ./...

echo "==> All tests passed!"
