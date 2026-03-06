#!/bin/bash
# bootstrap.sh — downloads the go binary from GitHub Releases,
# caches it in $RUNNER_TEMP, and exec's it with all arguments forwarded.

set -euo pipefail

REPO="bshore/gha"
BINARY_NAME="go"
CACHE_DIR="${RUNNER_TEMP:-/tmp}/actions/go"

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

# Serve from filesystem cache if available
if [[ -x "$BINARY_PATH" ]]; then
  echo "go: using cached binary at ${BINARY_PATH}"
  exec "$BINARY_PATH" "$@"
fi

# Try Actions cache restore
CACHE_KEY="${BINARY_NAME}-${RUNNER_OS:-Linux}-${RUNNER_ARCH:-X64}-${RELEASE_TAG}"
CACHE_VERSION=$(printf "%s\n%s\n" "$(dirname "$BINARY_PATH")" "${RUNNER_OS:-Linux}" | sha256sum | cut -d' ' -f1)
if [[ -n "${ACTIONS_RESULTS_URL:-}" && -n "${ACTIONS_RUNTIME_TOKEN:-}" ]]; then
  RESTORE_RESP=$(curl -sf \
    -X POST \
    -H "Authorization: Bearer $ACTIONS_RUNTIME_TOKEN" \
    -H "Content-Type: application/json" \
    "${ACTIONS_RESULTS_URL%/}/twirp/github.actions.results.api.v1.CacheService/GetCacheEntryDownloadURL" \
    -d "{\"key\":\"${CACHE_KEY}\",\"restoreKeys\":[],\"version\":\"${CACHE_VERSION}\"}" \
    2>/dev/null || echo "")
  ARCHIVE_URL=$(echo "$RESTORE_RESP" | grep -o '"signed_download_url":"[^"]*"' | head -1 | sed 's/"signed_download_url":"//;s/"$//')
  if [[ -n "$ARCHIVE_URL" ]]; then
    echo "go: cache hit — restoring binary"
    mkdir -p "$(dirname "$BINARY_PATH")"
    if curl -sL "$ARCHIVE_URL" | tar -xz -C / 2>/dev/null && [[ -x "$BINARY_PATH" ]]; then
      exec "$BINARY_PATH" "$@"
    fi
    echo "go: cache restore failed, falling back to download"
  fi
fi

mkdir -p "$(dirname "$BINARY_PATH")"

ASSET_NAME="${BINARY_NAME}-${OS}-${ARCH}"

CHECKSUMS_PATH="$(dirname "$BINARY_PATH")/checksums.txt"

echo "go: downloading ${ASSET_NAME} (release: ${RELEASE_TAG})"
if [[ "$RELEASE_TAG" == "latest" ]]; then
  gh release download --repo "$REPO" --pattern "$ASSET_NAME" --output "$BINARY_PATH" --clobber
  gh release download --repo "$REPO" --pattern "checksums.txt" --output "$CHECKSUMS_PATH" --clobber
else
  gh release download "$RELEASE_TAG" --repo "$REPO" --pattern "$ASSET_NAME" --output "$BINARY_PATH" --clobber
  gh release download "$RELEASE_TAG" --repo "$REPO" --pattern "checksums.txt" --output "$CHECKSUMS_PATH" --clobber
fi

EXPECTED=$(awk "/^[0-9a-f]+ +${ASSET_NAME}\$/ {print \$1}" "$CHECKSUMS_PATH")
if [[ -z "$EXPECTED" ]]; then
  echo "go: ${ASSET_NAME} not found in checksums.txt" >&2
  rm -f "$BINARY_PATH"
  exit 1
fi
ACTUAL=$(sha256sum "$BINARY_PATH" | cut -d' ' -f1)
if [[ "$EXPECTED" != "$ACTUAL" ]]; then
  echo "go: checksum mismatch for ${ASSET_NAME}" >&2
  echo "  expected: ${EXPECTED}" >&2
  echo "  actual:   ${ACTUAL}" >&2
  rm -f "$BINARY_PATH"
  exit 1
fi
echo "go: checksum OK"

chmod +x "$BINARY_PATH"

exec "$BINARY_PATH" "$@"
