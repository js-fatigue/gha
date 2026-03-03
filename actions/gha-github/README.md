# gha-github

A composite GitHub Action that runs GitHub-specific automation commands via a compiled Go binary.

## Commands

| Command | Description |
|---|---|
| `check-pr-title` | Validates a PR title against conventional commit format |
| `changed-dirs` | Lists directories with changed files between a base ref and HEAD |
| `release` | Bumps semver and creates a GitHub Release (dry-run by default) |

## Inputs

| Input | Required | Default | Description |
|---|---|---|---|
| `token` | no | `github.token` | GitHub token for API calls |
| `command` | **yes** | — | Command to run (see Commands above) |
| `base` | no | `"main"` | Base ref or SHA to compare against (`changed-dirs`, `release`) |
| `max_depth` | no | `"0"` | Max directory depth returned by `changed-dirs` (0 = unlimited) |
| `action_dir` | no | `""` | Path to the action family directory, e.g. `actions/gha-github` (required by `release`) |
| `release` | no | `"false"` | Set to `"true"` to publish the release; omit for dry-run |
| `cache` | no | `"true"` | Cache the downloaded binary. Set to `"false"` when building from source in the same job |

## Outputs

| Output | Description |
|---|---|
| `dir_names` | JSON array of unique directories containing changed files, e.g. `["actions/gha-github"]` |

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
      - uses: actions/checkout@v4

      - uses: bshore/gha/actions/gha-github@gha-github-v1.0.0
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: check-pr-title
```

### `changed-dirs`

Detects which directories have changed files between a base ref and HEAD. Requires `fetch-depth: 0` on the checkout step.

```yaml
jobs:
  detect-changes:
    runs-on: ubuntu-latest
    outputs:
      dirs: ${{ steps.changed.outputs.dir_names }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - id: changed
        uses: bshore/gha/actions/gha-github@gha-github-v1.0.0
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: changed-dirs
          base: main
          max_depth: "2"

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

Bumps the semver tag for an action family and creates a GitHub Release. The bump type is inferred from the commit or PR title using conventional commit conventions: `!` = major, `feat` = minor, everything else = patch. Tags follow the format `<family>-v<MAJOR>.<MINOR>.<PATCH>`.

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
      - uses: actions/checkout@v4

      - uses: bshore/gha/actions/gha-github@gha-github-v1.0.0
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: release
          action_dir: actions/gha-github
          # release defaults to "false" → dry-run only
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
      - uses: actions/checkout@v4

      - uses: bshore/gha/actions/gha-github@gha-github-v1.0.0
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: release
          action_dir: actions/gha-github
          release: "true"
```
