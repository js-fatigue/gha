# actions/terraform

A composite GitHub Action that runs Terraform-specific automation commands via a compiled Go binary.

## Commands

| Command | Description |
|---|---|
| `setup` | Installs Terraform, tflint, and Checkov; caches each tool via the Actions cache |
| `init` | Runs `terraform init` in a working directory |

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
input: '{"terraform_version": "1.9.8", "tflint_version": "latest"}'

# HCL — friendlier for multiline block scalars
input: |
  terraform_version = "1.9.8"
  tflint_version    = "latest"
  checkov_version   = "latest"
```

### `input` — `setup` command

All fields are optional. Omit `input` entirely for standard usage; all three tools will be installed at their latest versions.

| Field | Type | Default | Description |
|---|---|---|---|
| `terraform_version` | string | `"latest"` | Terraform version to install, e.g. `"1.9.8"`. When `"latest"`, the newest OSS release is resolved from the HashiCorp releases API |
| `tflint_version` | string | `"latest"` | tflint version to install, e.g. `"0.53.0"`. When `"latest"`, the newest release is resolved from the GitHub Releases API |
| `checkov_version` | string | `"latest"` | Checkov version to install, e.g. `"3.2.0"`. When `"latest"`, the newest version is resolved from the PyPI JSON API. Installed into a Python venv under `$RUNNER_TOOL_CACHE/checkov/<version>/` |

### `input` — `init` command

All fields are optional. Omit `input` entirely to run `terraform init` in the current directory with backend enabled.

| Field | Type | Default | Description |
|---|---|---|---|
| `working_directory` | string | `""` | Directory to `cd` into before running `terraform init`. Defaults to the current directory |
| `backend` | bool | `true` | When `false`, passes `-backend=false` to disable backend initialization |
| `upgrade` | bool | `false` | When `true`, passes `-upgrade` to upgrade provider and module versions |
| `reconfigure` | bool | `false` | When `true`, passes `-reconfigure` to reconfigure the backend, ignoring any saved configuration |
| `lint` | bool | `true` | When `true`, runs tflint and checkov against the working directory before `terraform init`. Requires both tools to be on PATH (i.e. `setup` ran first) |
| `lint_fail_on_findings` | bool | `false` | When `true`, aborts with an error if tflint or checkov exits non-zero, skipping `terraform init` entirely |

## Outputs

| Output | Description |
|---|---|
| `terraform_version` | The resolved Terraform version that was installed (only set when `command=setup`) |
| `tflint_version` | The resolved tflint version that was installed (only set when `command=setup`) |
| `checkov_version` | The resolved Checkov version that was installed (only set when `command=setup`) |

## Usage

### `setup`

Installs Terraform, tflint, and Checkov. Each tool is cached individually via the Actions cache so subsequent runs skip the download. Omit `input` to install the latest version of each tool.

```yaml
jobs:
  terraform:
    runs-on: ubuntu-latest
    steps:
      - uses: js-fatigue/gha/actions/github@github-v0.8.0
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: checkout

      # All tools installed at latest; input can be omitted entirely
      - uses: js-fatigue/gha/actions/terraform@terraform-v0.1.0
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: setup
```

To pin specific versions:

```yaml
      - uses: js-fatigue/gha/actions/terraform@terraform-v0.1.0
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: setup
          input: |
            terraform_version = "1.9.8"
            tflint_version    = "0.53.0"
            checkov_version   = "3.2.0"
```

### `init`

Runs `terraform init` in a given directory. Typically run after `setup`.

```yaml
jobs:
  terraform:
    runs-on: ubuntu-latest
    steps:
      - uses: js-fatigue/gha/actions/github@github-v0.8.0
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: checkout

      - uses: js-fatigue/gha/actions/terraform@terraform-v0.1.0
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: setup

      # Minimal invocation — init in the current directory
      - uses: js-fatigue/gha/actions/terraform@terraform-v0.1.0
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: init
```

To init a subdirectory with upgrade enabled:

```yaml
      - uses: js-fatigue/gha/actions/terraform@terraform-v0.1.0
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          command: init
          input: |
            working_directory = "infra/prod"
            upgrade           = true
```
