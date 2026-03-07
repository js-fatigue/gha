<!-- no-op counter: 6 -->
# actions/go

A composite GitHub Action that runs Go-specific automation commands via a compiled Go binary.

## Commands

| Command | Description |
|---|---|
| `setup` | Installs a specific Go version from go.dev; optionally caches the module cache |
| `build` | Wraps `go build` with configurable output path, ldflags, and CGO settings |

## Inputs

| Input | Required | Default | Description |
|---|---|---|---|
| `token` | no | `github.token` | GitHub token for API calls |
| `command` | **yes** | — | Command to run (see Commands above) |
| `input` | no | `""` | Options for the command in JSON or HCL (tfvars-style) format — auto-detected. Fields vary by command — see schemas below. All fields have sane defaults; omit entirely for standard usage |
| `self_cache` | no | `"true"` | Cache the downloaded binary via the Actions cache API. Set to `"false"` when the binary is built from source in the same job |

## Input Schemas

The `input` field accepts **JSON** (detected by leading `{`) or **HCL native syntax** (tfvars-style `key = value`). Both formats support the same fields.

```yaml
# JSON
input: '{"go_version_file": "go.mod"}'

# HCL — friendlier for multiline block scalars
input: |
  go_version_file      = "go.mod"
  cache_dependency_path = "go.sum"
  cache_go_install      = false
```

### `input` — `setup` command

All fields are optional. When neither `go_version` nor `go_version_file` is supplied, `go.mod` is detected automatically. Omit `input` entirely for standard usage.

| Field | Type | Default | Description |
|---|---|---|---|
| `go_version` | string | auto-detected | Exact or partial version to install, e.g. `"1.23.4"` or `"1.23"`. Also accepts `"stable"` or `"oldstable"`. Auto-detected from `go.mod` when present; falls back to `"stable"` |
| `go_version_file` | string | auto-detected | Path to a file containing the version (`go.mod` or `.go-version`). Takes precedence over `go_version`. Auto-detected when `go.mod` exists in the working directory |
| `check_latest` | bool | `false` | Re-check go.dev even if the requested version is already installed |
| `cache_go_modules` | bool | `true` | Save and restore `~/go/pkg/mod` using the Actions cache |
| `cache_go_install` | bool | `true` | Save and restore the Go installation directory using the Actions cache |
| `cache_dependency_path` | string | `"**/go.sum"` | Glob path to `go.sum` file(s) used as the module cache key |

### `input` — `build` command

All fields are optional. Omit `input` entirely to run `go build .` in the current directory.

| Field | Type | Default | Description |
|---|---|---|---|
| `working_directory` | string | `"."` | Directory to run `go build` from |
| `output` | string | `""` | Output binary path passed to `-o`. If empty, the default Go output name is used |
| `ldflags` | string | `""` | Linker flags passed to `-ldflags`, e.g. `"-s -w"` |
| `cgo_enabled` | string | `"0"` | Value for `CGO_ENABLED` environment variable |

## Outputs

| Output | Description |
|---|---|
| `go_version` | The resolved Go version that was installed (only set when `command=setup`) |
| `binary_path` | Absolute path to the compiled binary (only set when `output` is non-empty) |

## Usage

### `setup`

Installs Go from go.dev. When `go.mod` is present in the working directory, the version is read from it automatically — no `input` needed. The module cache at `~/go/pkg/mod` and the Go installation are saved and restored via the Actions cache by default.

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: bshore/gha/actions/github@github-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: checkout

      # go.mod is auto-detected; input can be omitted entirely
      - uses: bshore/gha/actions/go@go-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: setup
```

To pin options explicitly:

```yaml
      - uses: bshore/gha/actions/go@go-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: setup
          input: '{"go_version_file": "go.mod", "cache_dependency_path": "go.sum"}'
```

### `build`

Compiles a Go package. The `working_directory` field controls where `go build` is run; `output` sets the `-o` flag and causes `binary_path` to be set as an output.

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: bshore/gha/actions/github@github-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: checkout

      - uses: bshore/gha/actions/go@go-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: setup
          input: '{"go_version_file": "go.mod"}'

      - id: build
        uses: bshore/gha/actions/go@go-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: build
          input: '{"working_directory": "actions/github", "output": "github", "ldflags": "-s -w"}'

      - run: echo "Binary at ${{ steps.build.outputs.binary_path }}"
```
