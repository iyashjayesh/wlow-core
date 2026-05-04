#!/bin/sh
# install.sh — install the wlow CLI
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/wlow/wlow-core/main/install.sh | sh
#
# Options (environment variables):
#   WLOW_VERSION   specific version to install, e.g. v0.2.0 (default: latest)
#   WLOW_INSTALL   install directory (default: /usr/local/bin)

set -e

REPO="wlow/wlow-core"
BINARY="wlow"
INSTALL_DIR="${WLOW_INSTALL:-/usr/local/bin}"

# ── Detect OS ─────────────────────────────────────────────────────────────────

OS="$(uname -s)"
case "$OS" in
  Linux)  OS=linux ;;
  Darwin) OS=darwin ;;
  *)
    echo "error: unsupported OS: $OS" >&2
    exit 1
    ;;
esac

# ── Detect architecture ───────────────────────────────────────────────────────

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)          ARCH=amd64 ;;
  aarch64|arm64)   ARCH=arm64 ;;
  *)
    echo "error: unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# ── Resolve version ───────────────────────────────────────────────────────────

if [ -z "$WLOW_VERSION" ]; then
  WLOW_VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed 's/.*"tag_name": "\(.*\)".*/\1/')"
fi

if [ -z "$WLOW_VERSION" ]; then
  echo "error: could not determine latest version" >&2
  exit 1
fi

VERSION_STRIPPED="${WLOW_VERSION#v}"

echo "Installing wlow ${WLOW_VERSION} (${OS}/${ARCH})..."

# ── Download and extract ──────────────────────────────────────────────────────

ARCHIVE="${BINARY}_${VERSION_STRIPPED}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${WLOW_VERSION}/${ARCHIVE}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${WLOW_VERSION}/checksums.txt"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

curl -fsSL -o "${TMP}/${ARCHIVE}" "$URL"
curl -fsSL -o "${TMP}/checksums.txt" "$CHECKSUM_URL"

# Verify checksum
cd "$TMP"
grep "${ARCHIVE}" checksums.txt | sha256sum -c -
cd -

tar -xzf "${TMP}/${ARCHIVE}" -C "$TMP" "$BINARY"

# ── Install ───────────────────────────────────────────────────────────────────

if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  chmod +x "${INSTALL_DIR}/${BINARY}"
else
  echo "sudo required to install to ${INSTALL_DIR}"
  sudo mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  sudo chmod +x "${INSTALL_DIR}/${BINARY}"
fi

# ── Done ─────────────────────────────────────────────────────────────────────

echo ""
echo "wlow ${WLOW_VERSION} installed to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "Run:  wlow --help"
echo "Docs: https://github.com/${REPO}#documentation"
