# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based framework for writing GitHub Actions. The goal is to replace bash/Python/JavaScript action steps with compiled Go binaries for better performance and lower-level control. Actions are organized into "families" (e.g., `gha-terraform`, `gha-github`), each compiled to a standalone binary and distributed via GitHub Releases.

## Commands

```bash
# Build all packages
go build ./...

# Build a specific action binary
go build -o actions/context-example/context-example ./actions/context-example

# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/actions-core/...

# Vet
go vet ./...

# Format
gofmt -w .
```

## Architecture

### Shared Core Library — `internal/actions-core/` (package `ac`)

A Go port of the TypeScript `@actions/core` and `@actions/github` packages. Imported as `ac "github.com/bshore/gha/internal/actions-core"`.

| File | Purpose |
|------|---------|
| `core.go` | Inputs (`GetInput`, `GetBooleanInput`, `GetMultilineInput`), outputs (`SetOutput`), logging (`Debug`, `Info`, `Warning`, `Error`, `Notice`), log groups (`Group`), environment (`ExportVariable`, `AddPath`), secrets (`SetSecret`), state (`SaveState`/`GetState`), exit (`SetFailed`, `Exit`) |
| `context.go` | Reads `GITHUB_*` env vars into a `Context` struct; parses the webhook payload from `GITHUB_EVENT_PATH`; provides `ctx.Repo()` and `ctx.Issue()` helpers |
| `rest.go` | `NewClient()` — returns an authenticated `*github.Client` (via `go-github/v68`) using `GITHUB_TOKEN`; GHES-aware |
| `summary.go` | Fluent builder for job step summaries written to `GITHUB_STEP_SUMMARY`; use the package-level `ac.JobSummary` instance |

### Action Implementations — `actions/<name>/`

Each action is a self-contained directory with:
- `action.yml` — composite action that builds the binary then executes it
- `main.go` — entry point; always calls `defer ac.Exit()` at the start of `main()`

The current `actions/context-example` is the reference implementation. The `action.yml` builds from the repo root with `go build -o ${{ github.action_path }}/<name> ./<path>`.

### Planned Family Structure (from GHA.md)

For larger action families (e.g., `gha-terraform`), the intended layout is:
```
gha-<family>/
├─ action.yml          # composite: runs bootstrap.sh then the binary
├─ main.go             # dispatches to registered commands
├─ cmd/
│  ├─ registry.go      # Register(name, fn) + dispatch
│  └─ <command>.go     # one file per command, registers via init()
└─ scripts/bootstrap.sh
```

Commands self-register using `init()`:
```go
func init() { Register("command-name", runCommand) }
```

The bootstrap script downloads the pre-built binary from GitHub Releases and caches it in `$RUNNER_TEMP`.

### Key Conventions

- All `actions-core` functions that write to files return `error`; log/propagate these with `ac.SetFailed`.
- `ac.Exit()` calls `os.Exit` with the current exit code — always `defer ac.Exit()` in `main()`.
- File commands (GITHUB_ENV, GITHUB_OUTPUT, GITHUB_STATE, GITHUB_PATH) use heredoc delimiter format; the library handles this automatically.
- Inputs are read from `INPUT_<NAME>` env vars (uppercased, spaces→underscores).
- GHES support is built into `NewClient()` — it detects non-default `GITHUB_API_URL` automatically.

## CI

`.github/workflows/test.yml` runs on all branches except `main`. It sets up Go 1.23 and exercises `./actions/context-example` as a composite action.

## Module

```
module github.com/bshore/gha
go 1.23.4
```

Dependency: `github.com/google/go-github/v68` for the REST client.
