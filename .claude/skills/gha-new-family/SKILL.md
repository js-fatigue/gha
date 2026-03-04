# Skill: Scaffold a new `<family>` action family

**Trigger:** User wants to scaffold or create a new `<family>` action family in this project.

---

## Directory structure

```
actions/<family>/
├── main.go
├── action.yml
├── cmd/
│   ├── registry.go
│   └── <first-command>.go
└── scripts/
    └── bootstrap.sh
```

No workflow changes needed — the CI matrix reads `changed-dirs` output, so new `actions/` dirs are picked up automatically.

---

## `main.go`

Replace `<family>` with the family name (e.g. `go`, `aws`).

```go
package main

import (
    "fmt"
    "os"
    "strings"

    "github.com/bshore/gha/actions/<family>/cmd"
    ac "github.com/bshore/gha/internal/actions-core"
)

const prCommentMarker = "<!-- <family>-error -->"

func main() {
    defer ac.Exit()

    if len(os.Args) < 2 {
        ac.SetFailed("usage: <family> <command>")
        return
    }
    command := os.Args[1]

    ctx, err := ac.NewContext()
    if err != nil {
        ac.SetFailed(fmt.Sprintf("failed to read context: %v", err))
        return
    }

    if err := cmd.Dispatch(command); err != nil {
        ac.SetFailed(err.Error())
        ac.UpsertPRComment(ctx, prCommentMarker, buildErrorCommentBody(command, err))
    } else {
        ac.DeletePRComment(ctx, prCommentMarker)
    }
}

func buildErrorCommentBody(command string, cmdErr error) string {
    var sb strings.Builder
    sb.WriteString(prCommentMarker + "\n")
    fmt.Fprintf(&sb, "## <Family> Actions `%s` Failed\n\n", command)
    fmt.Fprintf(&sb, "**Error:** %s\n", cmdErr.Error())
    return sb.String()
}
```

---

## `cmd/registry.go`

Copy verbatim — same for every family.

```go
package cmd

import (
    "fmt"
    "sort"
)

type CommandFunc func() error

var registry = map[string]CommandFunc{}

func Register(name string, fn CommandFunc) {
    registry[name] = fn
}

func Dispatch(name string) error {
    fn, ok := registry[name]
    if !ok {
        keys := make([]string, 0, len(registry))
        for k := range registry {
            keys = append(keys, k)
        }
        sort.Strings(keys)
        return fmt.Errorf("unknown command %q; registered: %v", name, keys)
    }
    return fn()
}
```

---

## `cmd/<command>.go` (first command)

See the `gha-new-command` skill for the full command template.

```go
package cmd

import (
    ac "github.com/bshore/gha/internal/actions-core"
)

func init() { Register("<command-name>", run<Command>) }

func run<Command>() error {
    // ... implement
    return nil
}
```

---

## `action.yml`

Key rules:
- `command` is passed as a CLI arg (`"${{ inputs.command }}"`), not an env var.
- Every family-specific input **must** be forwarded in the `run` step `env:` block as `INPUT_<NAME>`.
- Input names **must use underscores** (not hyphens) — `ac.GetInput` only replaces spaces→underscores.
- Cache vars: `GHA_<FAMILY>_VERSION` (uppercased family name).
- Cache path: `${{ runner.temp }}/actions/<family>`.

```yaml
name: <Family> Actions
description: Runs <family>-specific commands via a compiled Go binary.

inputs:
  token:
    description: GitHub token for API calls
    required: true
    default: ${{ github.token }}
  command:
    description: Command to run
    required: true
  # --- family-specific inputs below ---
  my_input:
    description: Description of my_input
    required: false
    default: ""
  cache:
    description: >
      When true (default), cache the downloaded binary using actions/cache.
      Set to false when the binary is built from source in the same job.
    required: false
    default: "true"

outputs:
  my_output:
    description: Description of my_output
    value: ${{ steps.run.outputs.my_output }}

runs:
  using: composite
  steps:
    - id: version
      if: inputs.cache == 'true'
      shell: bash
      env:
        GITHUB_TOKEN: ${{ inputs.token }}
      run: |
        v="${GHA_<FAMILY>_VERSION:-}"
        if [[ -z "$v" ]]; then
          v=$(gh release view --repo bshore/gha latest --json tagName -q .tagName 2>/dev/null || echo "latest")
        fi
        echo "tag=${v}" >> "$GITHUB_OUTPUT"

    - uses: actions/cache@v4
      if: inputs.cache == 'true'
      with:
        path: ${{ runner.temp }}/actions/<family>
        key: <family>-${{ runner.os }}-${{ runner.arch }}-${{ steps.version.outputs.tag }}
        restore-keys: |
          <family>-${{ runner.os }}-${{ runner.arch }}-

    - id: run
      shell: bash
      env:
        GITHUB_TOKEN: ${{ inputs.token }}
        INPUT_MY_INPUT: ${{ inputs.my_input }}
        GHA_<FAMILY>_VERSION: ${{ steps.version.outputs.tag }}
      run: |
        chmod +x ${{ github.action_path }}/scripts/bootstrap.sh
        ${{ github.action_path }}/scripts/bootstrap.sh "${{ inputs.command }}"
```

---

## `scripts/bootstrap.sh`

Change three variables: `BINARY_NAME`, `CACHE_DIR`, `RELEASE_TAG` env var name.

```bash
#!/bin/bash
set -euo pipefail

REPO="bshore/gha"
BINARY_NAME="<family>"
CACHE_DIR="${RUNNER_TEMP:-/tmp}/actions/<family>"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
esac

RELEASE_TAG="${GHA_<FAMILY>_VERSION:-latest}"
BINARY_PATH="${CACHE_DIR}/${RELEASE_TAG}/${BINARY_NAME}"

if [[ -x "$BINARY_PATH" ]]; then
  echo "<family>: using cached binary at ${BINARY_PATH}"
  exec "$BINARY_PATH" "$@"
fi

mkdir -p "$(dirname "$BINARY_PATH")"
ASSET_NAME="${BINARY_NAME}-${OS}-${ARCH}"

if [[ "$RELEASE_TAG" == "latest" ]]; then
  DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET_NAME}"
else
  DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${RELEASE_TAG}/${ASSET_NAME}"
fi

echo "<family>: downloading from ${DOWNLOAD_URL}"
curl -fsSL -o "$BINARY_PATH" "$DOWNLOAD_URL"

if [[ "$RELEASE_TAG" == "latest" ]]; then
  CHECKSUM_URL="https://github.com/${REPO}/releases/latest/download/checksums.txt"
else
  CHECKSUM_URL="https://github.com/${REPO}/releases/download/${RELEASE_TAG}/checksums.txt"
fi

EXPECTED=$(curl -fsSL "$CHECKSUM_URL" | awk "/^[0-9a-f]+ +${ASSET_NAME}\$/ {print \$1}")
if [[ -z "$EXPECTED" ]]; then
  echo "<family>: ${ASSET_NAME} not found in checksums.txt" >&2
  rm -f "$BINARY_PATH"
  exit 1
fi
ACTUAL=$(sha256sum "$BINARY_PATH" | cut -d' ' -f1)
if [[ "$EXPECTED" != "$ACTUAL" ]]; then
  echo "<family>: checksum mismatch for ${ASSET_NAME}" >&2
  echo "  expected: ${EXPECTED}" >&2
  echo "  actual:   ${ACTUAL}" >&2
  rm -f "$BINARY_PATH"
  exit 1
fi
echo "<family>: checksum OK"

chmod +x "$BINARY_PATH"
exec "$BINARY_PATH" "$@"
```

---

## Checklist

- [ ] `go build ./actions/<family>/...` — compiles cleanly
- [ ] `go vet ./actions/<family>/...` — no issues
- [ ] All inputs forwarded as `INPUT_<NAME>` in `action.yml` run step env block
- [ ] Input names use underscores only
- [ ] PR comment marker is `<!-- <family>-error -->`
- [ ] `cache: "false"` set in CI workflows that build from source
- [ ] `scripts/bootstrap.sh` uses LF line endings — verify with `file scripts/bootstrap.sh` (must not say "CRLF"); `.gitattributes` enforces this on commit
