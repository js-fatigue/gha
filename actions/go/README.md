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
| `setup_input` | no | `""` | JSON options for the `setup` command |
| `build_input` | no | `""` | JSON options for the `build` command |
| `cache_dependency_path` | no | `"**/go.sum"` | Glob path to `go.sum` file(s) used as the module cache key (only used by `setup`) |
| `cache_go_modules` | no | `"true"` | Cache `~/go/pkg/mod` when running `setup`. Set to `"false"` to disable |
| `cache` | no | `"true"` | Cache the downloaded binary. Set to `"false"` when building from source in the same job |

## JSON Input Schemas

### `setup_input`

| Field | Type | Default | Description |
|---|---|---|---|
| `go_version` | string | `""` | Exact or partial version to install, e.g. `"1.23.4"` or `"1.23"`. Also accepts `"stable"` or `"oldstable"` |
| `go_version_file` | string | `""` | Path to a file containing the version (`go.mod` or `.go-version`). Takes precedence over `go_version` |
| `check_latest` | bool | `false` | Re-check go.dev even if the requested version is already installed |

### `build_input`

| Field | Type | Default | Description |
|---|---|---|---|
| `working_directory` | string | `""` | Directory to run `go build` from |
| `output` | string | `""` | Output binary path passed to `-o`. If empty, the default Go output name is used |
| `ldflags` | string | `""` | Linker flags passed to `-ldflags`, e.g. `"-s -w"` |
| `cgo_enabled` | bool | `false` | Set `CGO_ENABLED=1` (default is `CGO_ENABLED=0`) |

## Outputs

| Output | Description |
|---|---|
| `go_version` | The resolved Go version that was installed (only set when `command=setup`) |
| `binary_path` | Absolute path to the compiled binary (only set when `output` is non-empty) |

## Usage

### `setup`

Installs Go from go.dev. When `cache_go_modules` is `true` (the default), the module cache at `~/go/pkg/mod` is saved and restored using a key derived from the `go.sum` file(s).

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
          setup_input: '{"go_version_file": "go.mod"}'
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
          setup_input: '{"go_version_file": "go.mod"}'

      - id: build
        uses: bshore/gha/actions/go@go-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: build
          build_input: '{"working_directory": "actions/github", "output": "github", "ldflags": "-s -w"}'

      - run: echo "Binary at ${{ steps.build.outputs.binary_path }}"
```
