# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based framework for writing GitHub Actions. The goal is to replace bash/Python/JavaScript action steps with compiled Go binaries for better performance and lower-level control. Actions are organized into "families" (e.g., `github`), each compiled to a standalone binary and distributed via GitHub Releases.

## Commands

```bash
# Build all packages
go build ./...

# Build a specific action binary
go build -o actions/github/github ./actions/github
go build -o actions/go/go ./actions/go

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
| `comments.go` | `UpsertPRComment(ctx, marker, body)` and `DeletePRComment(ctx, marker)` — idempotent PR comment management via HTML marker strings |

### Action Family Structure — `actions/<family>/`

```
actions/<family>/
├── action.yml          # composite: runs bootstrap.sh then the binary
├── main.go             # reads os.Args[1] → cmd.Dispatch(); on error → upsert PR comment
├── cmd/
│   ├── registry.go     # Register(name, fn) + Dispatch(name)
│   └── <command>.go    # one file per command; self-registers via init()
└── scripts/
    └── bootstrap.sh    # downloads binary from Releases into $RUNNER_TEMP; self-caches via Actions cache API
```

Commands self-register using `init()`:
```go
func init() { Register("command-name", runCommand) }
```

#### Current family: `github`

| Command | Description |
|---------|-------------|
| `check-pr-title` | Validates PR title against conventional commit format |
| `changed-dirs` | Lists directories changed between base and HEAD via `git diff` |
| `release` | Semantic versioning — bumps version and creates GitHub Releases |
| `cache` | Restore or save cache entries via the GitHub Actions Cache API; inputs via `cache_input` JSON |

#### Current family: `go`

| Command | Description |
|---------|-------------|
| `build` | Wraps `go build`; inputs: `working_directory`, `output`, `ldflags`, `cgo_enabled`; output: `binary_path` |
| `setup` | Installs Go from go.dev; restores/saves `~/go/pkg/mod` module cache when `cache_go_modules=true` |
| `cache` | Restore or save cache entries via the GitHub Actions Cache API; inputs via `cache_input` JSON |

The `main.go` pattern:
1. `defer ac.Exit()` at top of `main()`
2. Read command from `os.Args[1]`
3. `ac.NewContext()` to get webhook context
4. `cmd.Dispatch(command)` to run the command
5. On error: upsert PR comment with marker `<!-- github-error -->` and call `ac.SetFailed()`
6. On success: delete stale error comment via `ac.DeletePRComment()`

### Key Conventions

**Composite action env var forwarding**
GitHub Actions composite `run:` steps do NOT auto-populate `INPUT_*` from action inputs. Every input must be explicitly forwarded:
```yaml
env:
  INPUT_WORKING_DIRECTORY: ${{ inputs.working_directory }}
```

**Input naming: underscores only**
`ac.GetInput` uppercases and replaces spaces→underscores only. Hyphens remain as-is, producing invalid env var names. Use underscores in input names.

**Command dispatch via `os.Args[1]`**
The `command` input is passed as a CLI argument, not an env var:
```yaml
run: ${{ github.action_path }}/<family> "${{ inputs.command }}"
```

**Boolean input helper**
`GetBooleanInput` errors on empty string. Use a helper in every command:
```go
func boolInputOrDefault(name string, defaultVal bool) (bool, error) {
    raw, _ := ac.GetInput(name, nil)
    if raw == "" { return defaultVal, nil }
    return ac.GetBooleanInput(name, nil)
}
```

**PR comment upsert**
Use HTML marker comments for idempotent updates. Call `ac.UpsertPRComment(ctx, marker, body)` on failure and `ac.DeletePRComment(ctx, marker)` on success.

**Binary self-caching (`self_cache` input)**
`action.yml` has a `self_cache` input (default `"true"`). When true, a `version` step runs before bootstrap to resolve the tag via the action path (falls back to `"latest"`), setting `GHA_<FAMILY>_VERSION`. `bootstrap.sh` then:
1. Tries the filesystem cache (`$RUNNER_TEMP/actions/<family>/<tag>/<binary>`)
2. Falls back to shell-based Actions cache API restore (curl)
3. Falls back to `gh release download`
4. After download, self-caches by calling `"$BINARY_PATH" cache` with `INPUT_CACHE_INPUT`

The `self_cache` input forwards as `INPUT_SELF_CACHE`; `SelfCacheBinary()` in `cache.go` reads it via `GetBooleanInputOrDefault("self_cache", true, nil)`.

**Source-build callers must pass `self_cache: "false"`** — the binary lands in `$RUNNER_TEMP/actions/<family>/latest/` and the filesystem check passes immediately.

**Sane defaults — inputs should work with token + command only**
Commands should work for the common case without any JSON input. Achieve this via two techniques:

1. **Struct pre-initialization** — initialize the input struct with default values before calling `GetJSONInput`. `json.Unmarshal` only overwrites fields present in the JSON, so absent fields retain their pre-initialized defaults:
```go
inp := MyCommandInput{
    SomeString: "default-value",
    SomeBool:   true,
}
if err := ac.GetJSONInput("my_command_input", &inp); err != nil {
    return fmt.Errorf("parsing my_command_input: %w", err)
}
```

2. **Environment auto-detection** — after JSON parse, probe the environment to fill in any remaining zero-value fields. Log what was detected with `ac.Info`:
```go
if inp.Base == "" {
    inp.Base = defaultBase() // git symbolic-ref → fallback to "main"
    ac.Info(fmt.Sprintf("Auto-detected base branch: %s", inp.Base))
}
if inp.GoVersionFile == "" && inp.GoVersion == "" {
    if _, err := os.Stat("go.mod"); err == nil {
        inp.GoVersionFile = "go.mod"
        ac.Info("Auto-detected go.mod for Go version")
    }
}
```

**Consolidate command options into `<command>_input` JSON, not top-level action inputs**
All options for a given command belong on its `*Input` struct and are passed as a single JSON blob via `<command>_input`. Do NOT add separate top-level action inputs for command-specific options (e.g. `cache_go_modules` belongs in `SetupInput`, not as `INPUT_CACHE_GO_MODULES`). This keeps `action.yml` flat and callers simple.

**Line endings — LF only**
All shell scripts (`*.sh`) and text files must use LF line endings. CRLF causes `cannot execute: required file not found` on Linux runners because the kernel appends `\r` to the shebang interpreter path. A `.gitattributes` file at the repo root enforces this via `* text=auto eol=lf`. Never commit files with CRLF line endings; verify with `file scripts/bootstrap.sh` (must not say "CRLF").

**Exit handling**
- All `actions-core` functions that write to files return `error`; propagate with `ac.SetFailed`.
- `ac.Exit()` calls `os.Exit` with the current exit code — always `defer ac.Exit()` in `main()`.
- File commands (GITHUB_ENV, GITHUB_OUTPUT, GITHUB_STATE, GITHUB_PATH) use heredoc delimiter format; the library handles this automatically.
- Inputs are read from `INPUT_<NAME>` env vars (uppercased, spaces→underscores).
- GHES support is built into `NewClient()` — it detects non-default `GITHUB_API_URL` automatically.

## CI

Two workflow files in `.github/workflows/`:

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `pull-request.yml` | PR opened/edited/synchronized/reopened | Validates PR title; detects changed dirs; runs `release` in dry-run mode to preview version bump |
| `tag.yml` | Push to `main` | Detects changed action dirs; runs `release` with `release=true`; cross-compiles linux-amd64/arm64 binaries and uploads to the release |

All CI workflows pass `self_cache: "false"` to action steps because the binary is built from source earlier in the same job.

The `release` command tags format: `<family>-v<MAJOR>.<MINOR>.<PATCH>` (e.g., `github-v1.0.0`). Bump type is inferred from the commit/PR title using conventional commit conventions (`!` = major, `feat` = minor, everything else = patch).

## Claude Skills

Project-level skills live in `.claude/skills/` and are invoked via the Skill tool:

| Skill | Trigger |
|-------|---------|
| `gha-new-family` | Scaffold a new `<family>` action family |
| `gha-new-command` | Add a new command to an existing `<family>` action |

## Module

```
module github.com/bshore/gha
go 1.23.4
```

Dependencies: `github.com/google/go-github/v68` for the REST client.
