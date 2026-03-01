# GHA

Idea is to have dozens or hundreds of github action business logic implemented in Go and shipped as a standalone action that
pipes the desired inner action to run via GH Action input. I want to replace where bash, python, or javascript would be used
to perform action steps to get more lower-level control and performance. The binary artifact has a fixed behavior unlike the
python/javascript which have to be interpreted at runtime, and bash is ugly.

Below is a clean “enterprise-scale” layout that can support 100+ GitHub Actions across families while keeping every repo small, releases simple, and maintenance manageable.

The key ideas:
 - Action families (terraform, github, docker, etc.)
- One CLI per family
- Shared internal Go library
- Thin GitHub Action wrapper
- Binary distributed via releases

High-level ecosystem organization:

```
gha/
├─ internal/actions-core
├─ gha-terraform
├─ gha-github
├─ gha-docker
├─ gha-onepassword
├─ gha-aws
```

Each family is independent, but they share common utilities.

Shared core library `internal/actions-core/`

Purpose: reusable utilities for all action families.

Structure:

```
internal/actions-core/
├─ core.go - Core GitHub Action utilities (handling inputs, outputs, logging, etc.)
├─ context.go - workflow context utilities
├─ rest.go - Authenticated GitHub REST API client
├─ summary.go - Workflow Summary step utilities
```

Example family: terraform actions

Directory: `gha-terraform/` (not sold on the naming convenction, mostly don't want to conflict with the actual `terraform` CLI binary name)

Structure:

gha-terraform/
├─ action.yml
├─ main.go
├─ scripts/bootstrap.sh
├─ cmd/
│  ├─ registry.go
│  ├─ terraform_init.go
│  ├─ terraform_validate.go
│  ├─ terraform_plan.go
│  ├─ terraform_apply.go
└─ go.mod

Binary: gha-terraform

## Bootstrap script:

```bash
#!/usr/bin/env bash
set -euo pipefail

COMMAND="$1"

REPO="bshore/gha"
VERSION="${GITHUB_ACTION_REF:-v1}"
BIN="gha-terraform"

OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

[ "$ARCH" = "x86_64" ] && ARCH="amd64"

URL="https://github.com/$REPO/releases/download/$VERSION/$BIN-$OS-$ARCH"

BIN_PATH="$RUNNER_TEMP/$BIN"

if [ ! -f "$BIN_PATH" ]; then
  curl -sL "$URL" -o "$BIN_PATH"
  chmod +x "$BIN_PATH"
fi

"$BIN_PATH" "$COMMAND"
```

## Dispatcher action

TODO: the action should cache the downloaded binary so it doesn't need to be downloaded every time.

action.yml
```yaml
name: Terraform Actions

inputs:
  command:
    description: command to run
    required: true

runs:
  using: composite
  steps:
    - shell: bash
      run: |
        $GITHUB_ACTION_PATH/scripts/bootstrap.sh "${{ inputs.command }}"
    - uses: bshore/gha/gha-terraform@v1 # @{this-branch-for-testing-purposes}
      with:
        command: terraform-init
    
    - uses: bshore/gha/gha-terraform@v1 # @{this-branch-for-testing-purposes}
      with:
        command: terraform-plan
    
    - uses: bshore/gha/gha-terraform@v1 # @{this-branch-for-testing-purposes}
      with:
        command: terraform-apply
```


## Example command implementation

```go
cmd/terraform_plan.go

package cmd

import (
	"os"
	"os/exec"
)

func init() {
	Register("terraform-plan", runTerraformPlan)
}

func runTerraformPlan() error {
	cmd := exec.Command("terraform", "plan", "-out=tfplan")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

Adding a new action = one file.

Example second family: GitHub actions `gha-github/` structure:

```
gha-github/
├─ action.yml
├─ main.go
├─ cmd/
│  ├─ comment_pr.go
│  ├─ create_release.go
│  ├─ label_pr.go
│  ├─ download_artifact.go
│  └─ upload_artifact.go
└─ scripts/bootstrap.sh
```

Binary:

gha-github

## Versioning

Will need to build a workflow that will build and release the assets when this repository has a release tag.

Each family versions releases independently within this repository:

gha-terraform-v1.4.2
gha-github-v2.1.0
gha-docker-v1.0.3

Users reference:

uses: bshore/gha/gha-terraform@v1
uses: bshore/gha/gha-github@v2
uses: bshore/gha/gha-docker@v1
