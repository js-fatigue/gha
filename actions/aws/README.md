# AWS Actions

Runs AWS-specific commands via a compiled Go binary. Currently supports GitHub OIDC-based role assumption via AWS STS — no long-lived credentials required.

## Commands

| Command | Description |
|---------|-------------|
| `assume-role` | Exchanges a GitHub OIDC token for temporary AWS credentials via `sts:AssumeRoleWithWebIdentity` |
| `ecr-login` | Authenticates Docker with an ECR registry using `aws ecr get-login-password` |

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `token` | yes | `${{ github.token }}` | GitHub token for API calls |
| `command` | yes | — | Command to run (e.g. `assume-role`) |
| `input` | no | `""` | Command options in JSON or HCL (tfvars-style) format. All fields have sane defaults; omit entirely for standard usage. |
| `self_cache` | no | `"true"` | When true, the binary self-caches via the Actions cache API after first download. Set to false when the binary is built from source in the same job. |

## Input schemas

### `input` — `assume-role` command

All fields are optional except `role`. Omit all other fields for standard usage.

Both JSON (`{"role": "arn:aws:iam::..."}`) and HCL (`role = "arn:aws:iam::..."`) formats are accepted.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `role` | string | — | **Required.** ARN of the IAM role to assume |
| `region` | string | `"us-east-1"` | AWS region for the STS endpoint and exported `AWS_REGION` |
| `session_name` | string | `"auto-detected"` | STS session name. Auto-detected as `<owner>-<repo>@<run-id>` from `GITHUB_REPOSITORY` and `GITHUB_RUN_ID`, truncated to 64 characters |
| `duration` | int | `3600` | Credential lifetime in seconds (min 900, max 43200 unless role allows longer) |
| `audience` | string | `"sts.amazonaws.com"` | OIDC audience claim. Override only if your IAM trust policy requires a custom audience |

### `input` — `ecr-login` command

All fields are optional when `assume-role` has already run in the same job — the region is auto-detected from the registry hostname or `AWS_REGION`.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `registry` | string | — | **Required.** ECR registry URI. Accepts the full image URI (e.g. `123456789012.dkr.ecr.us-east-2.amazonaws.com/repo`) or just the hostname; the repo path is ignored for login purposes |
| `region` | string | `"auto-detected"` | AWS region. Auto-detected from the `dkr.ecr.<region>.amazonaws.com` hostname pattern, then from `AWS_REGION` env var |

## Outputs

| Output | Description |
|--------|-------------|
| `aws_region` | The AWS region used for the assumed role session (only set when `command=assume-role`) |
| `registry` | The ECR registry hostname that was logged in to (only set when `command=ecr-login`) |

The command also exports these environment variables for all subsequent steps:

| Variable | Description |
|----------|-------------|
| `AWS_ACCESS_KEY_ID` | Temporary access key ID |
| `AWS_SECRET_ACCESS_KEY` | Temporary secret access key |
| `AWS_SESSION_TOKEN` | Temporary session token |
| `AWS_REGION` | AWS region |
| `AWS_DEFAULT_REGION` | AWS region (legacy alias) |

## Prerequisites

The calling workflow must have `id-token: write` permission for GitHub to issue an OIDC token:

```yaml
permissions:
  id-token: write
  contents: read
```

The IAM role's trust policy must allow `sts:AssumeRoleWithWebIdentity` from the GitHub OIDC provider (`token.actions.githubusercontent.com`).

## Usage

### Minimal — assume a role with all defaults

```yaml
permissions:
  id-token: write
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: js-fatigue/gha/actions/aws@aws-v1.0.0
        with:
          command: assume-role
          input: |
            role = "arn:aws:iam::123456789012:role/my-deploy-role"
```

### Specify region and duration

```yaml
      - uses: js-fatigue/gha/actions/aws@aws-v1.0.0
        with:
          command: assume-role
          input: |
            role     = "arn:aws:iam::123456789012:role/my-deploy-role"
            region   = "eu-west-1"
            duration = 7200
```

### Use with Docker ECR push

```yaml
permissions:
  id-token: write
  contents: read

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: js-fatigue/gha/actions/aws@aws-v1.0.0
        id: aws
        with:
          command: assume-role
          input: |
            role   = "arn:aws:iam::123456789012:role/ecr-push-role"
            region = "us-east-1"

      - name: Login to ECR
        uses: js-fatigue/gha/actions/aws@aws-v1.0.0
        id: ecr
        with:
          command: ecr-login
          input: |
            registry = "123456789012.dkr.ecr.us-east-1.amazonaws.com/my-image"

      - uses: js-fatigue/gha/actions/docker@docker-v1.0.0
        with:
          command: build
          input: |
            tags = ["123456789012.dkr.ecr.us-east-1.amazonaws.com/my-image:${{ github.sha }}"]
            push = true
```
