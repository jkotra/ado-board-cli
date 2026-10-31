#!/usr/bin/env bash
set -e

REPO="jkotra/ado-board-cli"
RELEASE_URL="https://github.com/${REPO}/releases/latest/download"

echo "=== ado-board-cli Installer ==="

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  linux*)
    GOOS="linux"
    ;;
  darwin*)
    GOOS="darwin"
    ;;
  *)
    echo "Error: Unsupported operating system: $OS. Supported OS: Linux, macOS."
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64|amd64)
    GOARCH="amd64"
    ;;
  aarch64|arm64)
    GOARCH="arm64"
    ;;
  *)
    echo "Error: Unsupported architecture: $ARCH. Supported: x86_64, arm64."
    exit 1
    ;;
esac

ARCHIVE="ado-board-cli_${GOOS}_${GOARCH}.tar.gz"
DOWNLOAD_URL="${RELEASE_URL}/${ARCHIVE}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "--> Downloading ${DOWNLOAD_URL}..."
if ! curl -sSfL "$DOWNLOAD_URL" -o "$TMP_DIR/$ARCHIVE"; then
  echo "Error: Failed to download $DOWNLOAD_URL"
  exit 1
fi

echo "--> Extracting archive..."
tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR"

INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  if command -v sudo >/dev/null 2>&1; then
    echo "--> Installing to $INSTALL_DIR (requires sudo)..."
    sudo install -m 755 "$TMP_DIR/ado-board-cli" "$INSTALL_DIR/ado-board-cli"
  else
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
    echo "--> Installing to $INSTALL_DIR..."
    install -m 755 "$TMP_DIR/ado-board-cli" "$INSTALL_DIR/ado-board-cli"
    echo "NOTE: Ensure $INSTALL_DIR is added to your PATH environment variable."
  fi
else
  echo "--> Installing to $INSTALL_DIR..."
  install -m 755 "$TMP_DIR/ado-board-cli" "$INSTALL_DIR/ado-board-cli"
fi

echo "--> ado-board-cli installed successfully!"
if command -v ado-board-cli >/dev/null 2>&1; then
  ado-board-cli version 2>/dev/null || true
fi
