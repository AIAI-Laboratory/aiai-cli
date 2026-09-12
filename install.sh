#!/bin/sh
# AIAI CLI Installer
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/AIAI-Laboratory/aiai-cli/main/install.sh | bash
#
# Environment variables:
#   AIAI_INSTALL_DIR - Target installation directory (default: $HOME/.local/bin)
#   AIAI_VERSION     - Specific version/tag to install (default: latest release)

set -e

REPO="AIAI-Laboratory/aiai-cli"
INSTALL_DIR="${AIAI_INSTALL_DIR:-$HOME/.local/bin}"

# Colors (if running in terminal)
if [ -t 1 ]; then
  BOLD="\033[1m"
  GREEN="\033[0;32m"
  YELLOW="\033[0;33m"
  RED="\033[0;31m"
  BLUE="\033[0;34m"
  RESET="\033[0m"
else
  BOLD=""
  GREEN=""
  YELLOW=""
  RED=""
  BLUE=""
  RESET=""
fi

info() {
  printf "${BLUE}==>${RESET} ${BOLD}%s${RESET}\n" "$1"
}

success() {
  printf "${GREEN}==>${RESET} ${BOLD}%s${RESET}\n" "$1"
}

warn() {
  printf "${YELLOW}warning:${RESET} %s\n" "$1"
}

error() {
  printf "${RED}error:${RESET} %s\n" "$1" >&2
  exit 1
}

# 1. Detect Operating System
OS="$(uname -s)"
case "$OS" in
  Darwin)
    OS="darwin"
    ;;
  Linux)
    OS="linux"
    ;;
  MINGW*|MSYS*|CYGWIN*)
    OS="windows"
    ;;
  *)
    error "Unsupported operating system: $OS"
    ;;
esac

# 2. Detect CPU Architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  arm64|aarch64)
    ARCH="arm64"
    ;;
  *)
    error "Unsupported CPU architecture: $ARCH"
    ;;
esac

# 3. Detect downloader (curl or wget)
if command -v curl >/dev/null 2>&1; then
  DOWNLOADER="curl"
elif command -v wget >/dev/null 2>&1; then
  DOWNLOADER="wget"
else
  error "Neither 'curl' nor 'wget' was found. Please install one of them."
fi

download_file() {
  url="$1"
  dest="$2"
  if [ "$DOWNLOADER" = "curl" ]; then
    curl -fsSL "$url" -o "$dest"
  else
    wget -qO "$dest" "$url"
  fi
}

fetch_url() {
  url="$1"
  if [ "$DOWNLOADER" = "curl" ]; then
    curl -fsSL "$url"
  else
    wget -qO- "$url"
  fi
}

# 4. Determine Release Version
VERSION="${AIAI_VERSION:-}"
if [ -z "$VERSION" ]; then
  info "Checking for the latest AIAI release..."
  # Try GitHub API first
  LATEST_JSON="$(fetch_url "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null || true)"
  if [ -n "$LATEST_JSON" ]; then
    VERSION="$(printf "%s" "$LATEST_JSON" | grep '"tag_name":' | head -n 1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
  fi

  # Fallback to redirect header if API fails or rate-limited
  if [ -z "$VERSION" ]; then
    if [ "$DOWNLOADER" = "curl" ]; then
      REDIRECT_URL="$(curl -sI "https://github.com/${REPO}/releases/latest" 2>/dev/null | grep -i "^location:" | tr -d '\r' | awk '{print $2}' || true)"
    else
      REDIRECT_URL="$(wget -S --spider "https://github.com/${REPO}/releases/latest" 2>&1 | grep -i "Location:" | tr -d '\r' | awk '{print $2}' | tail -n 1 || true)"
    fi
    if [ -n "$REDIRECT_URL" ]; then
      VERSION="${REDIRECT_URL##*/}"
    fi
  fi
fi

# If remote release is not found yet, check for local pre-built binary
if [ -z "$VERSION" ]; then
  if [ -f "./bin/aiai" ]; then
    info "No remote release published yet. Found local binary at ./bin/aiai, installing..."
    mkdir -p "$INSTALL_DIR"
    cp "./bin/aiai" "$INSTALL_DIR/aiai"
    chmod +x "$INSTALL_DIR/aiai"
    success "Installed local AIAI binary to $INSTALL_DIR/aiai"
    exit 0
  fi
  error "Could not find any published releases at https://github.com/${REPO}/releases.\n       If you are developing locally, run 'go build -o bin/aiai ./cmd/aiai' first."
fi

# Clean tag format: v0.1.0 -> 0.1.0 for filename
RAW_VERSION="${VERSION#v}"
TAG="$VERSION"
if [ "$TAG" = "$RAW_VERSION" ]; then
  TAG="v$RAW_VERSION"
fi

info "Installing AIAI CLI ${TAG} (${OS}/${ARCH})..."

# 5. Download archive
EXT="tar.gz"
if [ "$OS" = "windows" ]; then
  EXT="zip"
fi

FILENAME="aiai_${RAW_VERSION}_${OS}_${ARCH}.${EXT}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${FILENAME}"

TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t 'aiai')"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

ARCHIVE_PATH="$TMP_DIR/$FILENAME"

info "Downloading ${DOWNLOAD_URL}..."
if ! download_file "$DOWNLOAD_URL" "$ARCHIVE_PATH"; then
  # Fallback: try filename retaining the 'v' prefix
  ALT_FILENAME="aiai_${TAG}_${OS}_${ARCH}.${EXT}"
  ALT_URL="https://github.com/${REPO}/releases/download/${TAG}/${ALT_FILENAME}"
  if ! download_file "$ALT_URL" "$ARCHIVE_PATH"; then
    error "Failed to download release archive from ${DOWNLOAD_URL}.\n       Please verify assets at https://github.com/${REPO}/releases/tag/${TAG}"
  fi
fi

# 6. Extract binary
info "Extracting..."
mkdir -p "$INSTALL_DIR"
if [ "$EXT" = "tar.gz" ]; then
  tar -xzf "$ARCHIVE_PATH" -C "$TMP_DIR"
else
  unzip -q -o "$ARCHIVE_PATH" -d "$TMP_DIR"
fi

BINARY_SOURCE="$TMP_DIR/aiai"
if [ "$OS" = "windows" ]; then
  BINARY_SOURCE="$TMP_DIR/aiai.exe"
fi

if [ ! -f "$BINARY_SOURCE" ]; then
  error "Binary 'aiai' was not found in the downloaded archive."
fi

chmod +x "$BINARY_SOURCE"
TARGET_BIN="$INSTALL_DIR/aiai"
if [ "$OS" = "windows" ]; then
  TARGET_BIN="$INSTALL_DIR/aiai.exe"
fi

mv "$BINARY_SOURCE" "$TARGET_BIN"
success "AIAI CLI successfully installed to ${TARGET_BIN}"

# 7. Verify PATH
case ":$PATH:" in
  *":$INSTALL_DIR:"*)
    ;;
  *)
    echo ""
    warn "${INSTALL_DIR} is not currently in your PATH."
    printf "To use 'aiai' anywhere, add this to your shell config (~/.zshrc or ~/.bashrc):\n\n"
    printf "  ${BOLD}export PATH=\"%s:\$PATH\"${RESET}\n\n" "$INSTALL_DIR"
    ;;
esac

echo ""
success "AIAI CLI is ready! Run 'aiai' or 'aiai --help' to get started."
