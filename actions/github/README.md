<!-- no-op counter: 2-->
# actions/github

A composite GitHub Action that runs GitHub-specific automation commands via a compiled Go binary.

## Commands

| Command | Description |
|---|---|
| `check-pr-title` | Validates a PR title against conventional commit format |
| `changed-dirs` | Lists directories with changed files between a base ref and HEAD |
| `release` | Bumps semver and creates a GitHub Release (dry-run by default) |
| `checkout` | Clones or fetches a repository with full credential and sparse-checkout support |

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
input: '{"max_depth": 2}'

# HCL — friendlier for multiline block scalars
input: |
  max_depth = 2
  base      = "main"

# Arrays in HCL
input: |
  action = "restore"
  key    = "my-key"
  path   = ["~/go/pkg/mod"]
```

### `input` — `changed-dirs` command

| Field | Type | Default | Description |
|---|---|---|---|
| `base` | string | auto-detected | Base ref to diff against. Auto-detected from `origin/HEAD`, then `origin/main`/`origin/master`, then falls back to `"main"` |
| `max_depth` | int | `0` | Max directory depth to include (0 = unlimited) |

### `input` — `release` command

| Field | Type | Default | Description |
|---|---|---|---|
| `action_dir` | string | **required** | Path to the action family directory, e.g. `actions/github` |
| `release` | bool | `false` | Set to `true` to publish; omit or `false` for dry-run |

### `input` — `checkout` command

| Field | Type | Default | Description |
|---|---|---|---|
| `repository` | string | current repo | Repository to clone, e.g. `owner/repo` |
| `ref` | string | default branch | Branch, tag, or SHA to check out |
| `path` | string | `""` | Relative path to clone into |
| `fetch_depth` | int | `1` | Number of commits to fetch (0 = full history) |
| `fetch_tags` | bool | `false` | Fetch all tags |
| `clean` | bool | `true` | Run `git clean` before checkout |
| `submodules` | string | `"false"` | `"false"`, `"true"`, or `"recursive"` |
| `lfs` | bool | `false` | Download Git LFS objects |
| `persist_credentials` | bool | `true` | Persist credentials in local git config |
| `set_safe_directory` | bool | `true` | Mark the checkout path as a safe directory |
| `sparse_checkout` | string | `""` | Newline-separated sparse-checkout patterns |
| `sparse_checkout_cone_mode` | bool | `true` | Use cone mode for sparse checkout |

## Outputs

| Output | Description |
|---|---|
| `dir_names` | JSON array of unique directories containing changed files, e.g. `["actions/github"]` |
| `next_tag` | Tag created by the `release` command (only set when `release=true`) |
| `ref` | The branch or tag ref that was checked out |
| `commit` | The commit SHA that was checked out |

## Usage

### `check-pr-title`

Validates that a pull request title follows [conventional commit](https://www.conventionalcommits.org/) format: `<type>[(<scope>)][!]: <description>`.

```yaml
on:
  pull_request:
    types: [opened, edited, synchronize, reopened]

jobs:
  check-title:
    runs-on: ubuntu-latest
    steps:
      - uses: bshore/gha/actions/github@github-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: check-pr-title
```

### `changed-dirs`

Detects which directories have changed files between a base ref and HEAD. Requires `fetch_depth: 0` on the checkout step so full history is available.

```yaml
jobs:
  detect-changes:
    runs-on: ubuntu-latest
    outputs:
      dirs: ${{ steps.changed.outputs.dir_names }}
    steps:
      - uses: bshore/gha/actions/github@github-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: checkout
          input: '{"fetch_depth": 0}'

      - id: changed
        uses: bshore/gha/actions/github@github-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: changed-dirs
          # base is auto-detected from origin/HEAD; pass input only to override
          input: '{"max_depth": 2}'

  use-changes:
    needs: detect-changes
    if: ${{ needs.detect-changes.outputs.dirs != '[]' }}
    strategy:
      matrix:
        dir: ${{ fromJson(needs.detect-changes.outputs.dirs) }}
    runs-on: ubuntu-latest
    steps:
      - run: echo "Changed directory: ${{ matrix.dir }}"
```

### `release`

Bumps the semver tag for an action family and creates a GitHub Release. Bump type is inferred from the commit/PR title: `!` = major, `feat` = minor, everything else = patch. Tags follow the format `<family>-v<MAJOR>.<MINOR>.<PATCH>`.

**Dry-run on pull request** (previews the next version as a PR comment):

```yaml
on:
  pull_request:
    types: [opened, edited, synchronize, reopened]

jobs:
  preview-release:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: bshore/gha/actions/github@github-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: checkout
          input: '{"fetch_depth": 0}'

      - uses: bshore/gha/actions/github@github-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: release
          input: '{"action_dir": "actions/github"}'
          # release defaults to false → dry-run only
```

**Real release on push to main**:

```yaml
on:
  push:
    branches: [main]

jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: bshore/gha/actions/github@github-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: checkout
          input: '{"fetch_depth": 0}'

      - uses: bshore/gha/actions/github@github-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: release
          input: '{"action_dir": "actions/github", "release": true}'
```

### `checkout`

Clones or fetches the repository with token-based authentication. Supports sparse checkout, submodules, LFS, and custom clone paths.

```yaml
jobs:
  example:
    runs-on: ubuntu-latest
    steps:
      - uses: bshore/gha/actions/github@github-v0.4.1
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: checkout
          input: '{"ref": "${{ github.ref_name }}", "fetch_depth": 0}'
```
