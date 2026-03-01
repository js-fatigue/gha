#!/usr/bin/env bash
# bootstrap.sh — downloads the gha-github binary from GitHub Releases,
# caches it in $RUNNER_TEMP, and exec's it with all arguments forwarded.
# Not used in the POC (action.yml builds from source); intended for production.

set -euo pipefail

REPO="bshore/gha"
BINARY_NAME="gha-github"
CACHE_DIR="${RUNNER_TEMP:-/tmp}/gha-github"

# Normalize OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"

# Normalize architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
esac

# Allow pinning via GHA_GITHUB_VERSION env var; default to latest
RELEASE_TAG="${GHA_GITHUB_VERSION:-latest}"

BINARY_PATH="${CACHE_DIR}/${RELEASE_TAG}/${BINARY_NAME}"

# Serve from cache if available
if [[ -x "$BINARY_PATH" ]]; then
  echo "gha-github: using cached binary at ${BINARY_PATH}"
  exec "$BINARY_PATH" "$@"
fi

mkdir -p "$(dirname "$BINARY_PATH")"

ASSET_NAME="${BINARY_NAME}-${OS}-${ARCH}"

if [[ "$RELEASE_TAG" == "latest" ]]; then
  DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET_NAME}"
else
  DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${RELEASE_TAG}/${ASSET_NAME}"
fi

echo "gha-github: downloading from ${DOWNLOAD_URL}"
curl -fsSL -o "$BINARY_PATH" "$DOWNLOAD_URL"
chmod +x "$BINARY_PATH"

exec "$BINARY_PATH" "$@"
