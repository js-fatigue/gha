#!/bin/bash
# bootstrap.sh — downloads the gha-go binary from GitHub Releases,
# caches it in $RUNNER_TEMP, and exec's it with all arguments forwarded.
# Not used in the POC (action.yml builds from source); intended for production.

set -euo pipefail

REPO="bshore/gha"
BINARY_NAME="gha-go"
CACHE_DIR="${RUNNER_TEMP:-/tmp}/actions/gha-go"

# Normalize OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"

# Normalize architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
esac

# Allow pinning via GHA_GO_VERSION env var; default to latest
RELEASE_TAG="${GHA_GO_VERSION:-latest}"

BINARY_PATH="${CACHE_DIR}/${RELEASE_TAG}/${BINARY_NAME}"

# Serve from cache if available
if [[ -x "$BINARY_PATH" ]]; then
  echo "gha-go: using cached binary at ${BINARY_PATH}"
  exec "$BINARY_PATH" "$@"
fi

mkdir -p "$(dirname "$BINARY_PATH")"

ASSET_NAME="${BINARY_NAME}-${OS}-${ARCH}"

if [[ "$RELEASE_TAG" == "latest" ]]; then
  DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET_NAME}"
else
  DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${RELEASE_TAG}/${ASSET_NAME}"
fi

echo "gha-go: downloading from ${DOWNLOAD_URL}"
curl -fsSL -o "$BINARY_PATH" "$DOWNLOAD_URL"

# Verify checksum
if [[ "$RELEASE_TAG" == "latest" ]]; then
  CHECKSUM_URL="https://github.com/${REPO}/releases/latest/download/checksums.txt"
else
  CHECKSUM_URL="https://github.com/${REPO}/releases/download/${RELEASE_TAG}/checksums.txt"
fi

EXPECTED=$(curl -fsSL "$CHECKSUM_URL" | awk "/^[0-9a-f]+ +${ASSET_NAME}\$/ {print \$1}")
if [[ -z "$EXPECTED" ]]; then
  echo "gha-go: ${ASSET_NAME} not found in checksums.txt" >&2
  rm -f "$BINARY_PATH"
  exit 1
fi
ACTUAL=$(sha256sum "$BINARY_PATH" | cut -d' ' -f1)
if [[ "$EXPECTED" != "$ACTUAL" ]]; then
  echo "gha-go: checksum mismatch for ${ASSET_NAME}" >&2
  echo "  expected: ${EXPECTED}" >&2
  echo "  actual:   ${ACTUAL}" >&2
  rm -f "$BINARY_PATH"
  exit 1
fi
echo "gha-go: checksum OK"

chmod +x "$BINARY_PATH"

exec "$BINARY_PATH" "$@"
