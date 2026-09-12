#!/usr/bin/env bash
set -euo pipefail

# scripts/fmt.sh - Formats Go files with goimports and gofumpt
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULE_NAME="github.com/AIAI-Laboratory/aiai-cli"

CHECK_MODE=false
if [[ "${1:-}" == "--check" ]]; then
  CHECK_MODE=true
fi

if [[ "${CHECK_MODE}" == true ]]; then
  echo "==> Checking code formatting..."
  DIFF_IMPORTS=$(go tool goimports -local "${MODULE_NAME}" -l "${ROOT_DIR}")
  DIFF_FUMPT=$(go tool gofumpt -l -extra "${ROOT_DIR}")

  FAILED=0
  if [[ -n "${DIFF_IMPORTS}" ]]; then
    echo "Files with unformatted imports:"
    echo "${DIFF_IMPORTS}"
    FAILED=1
  fi
  if [[ -n "${DIFF_FUMPT}" ]]; then
    echo "Files with unformatted code (gofumpt):"
    echo "${DIFF_FUMPT}"
    FAILED=1
  fi

  if [[ ${FAILED} -ne 0 ]]; then
    echo "Format check failed. Run ./scripts/fmt.sh to fix."
    exit 1
  fi
  echo "==> Code formatting is clean!"
else
  echo "==> Running goimports..."
  go tool goimports -local "${MODULE_NAME}" -w "${ROOT_DIR}"

  echo "==> Running gofumpt..."
  go tool gofumpt -w -extra "${ROOT_DIR}"

  echo "==> Formatting complete!"
fi
