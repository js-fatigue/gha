# Docker Actions

Runs Docker-specific commands via a compiled Go binary. Wraps `docker buildx` to build images with support for multi-platform builds, caching, and pushing to registries.

## Commands

| Command | Description |
|---------|-------------|
| `build` | Builds a Docker image using `docker buildx build` |
| `login` | Authenticates with a container registry via `docker login` |

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `token` | yes | `${{ github.token }}` | GitHub token for API calls |
| `command` | yes | — | Command to run (e.g. `build`) |
| `input` | no | `""` | Command options in JSON or HCL (tfvars-style) format. All fields have sane defaults; omit entirely for standard usage. |
| `self_cache` | no | `"true"` | When true, the binary self-caches via the Actions cache API after first download. Set to false when the binary is built from source in the same job. |

## Input schemas

### `input` — `build` command

All fields are optional. Omit `input` entirely for standard usage.

Both JSON (`{"context": "."}`) and HCL (`context = "."`) formats are accepted.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `context` | string | `"."` | Build context path |
| `dockerfile` | string | `""` | Path to Dockerfile. When omitted, Docker uses `{context}/Dockerfile` by default |
| `tags` | list of strings | `[]` | Image tags to apply (e.g. `["ghcr.io/owner/repo:latest"]`) |
| `build_args` | list of strings | `[]` | Build-time variables in `KEY=VALUE` format |
| `platform` | string | `"(native)"` | Target platform(s) (e.g. `"linux/amd64,linux/arm64"`). Defaults to native runner platform |
| `push` | bool | `false` | Push the image to the registry after build |
| `load` | bool | `false` | Load the built image into the local Docker daemon (ignored when `push=true`) |
| `target` | string | `""` | Target build stage name |
| `no_cache` | bool | `false` | Disable build cache |
| `labels` | list of strings | `[]` | Metadata labels in `KEY=VALUE` format |
| `cache_from` | string | `""` | External cache source (e.g. `"type=gha"`) |
| `cache_to` | string | `""` | External cache destination (e.g. `"type=gha,mode=max"`) |

### `input` — `login` command

All fields are optional when targeting GHCR from a GitHub Actions workflow — `username` and `password` are auto-detected from `GITHUB_ACTOR` and `GITHUB_TOKEN`.

Both JSON (`{"registry": "ghcr.io"}`) and HCL (`registry = "ghcr.io"`) formats are accepted.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `registry` | string | `"ghcr.io"` | Registry hostname to log in to |
| `username` | string | `"auto-detected"` | Registry username. Auto-detected from `GITHUB_ACTOR` |
| `password` | string | `"auto-detected"` | Registry password or token. Auto-detected from `GITHUB_TOKEN` |

## Outputs

| Output | Description |
|--------|-------------|
| `image_id` | Image config digest (only set when `command=build`) |
| `digest` | Image content digest (only set when `command=build` and pushing) |

## Usage

### Minimal — build image from current directory

```yaml
- uses: js-fatigue/gha/actions/docker@docker-v1.0.0
  with:
    command: build
```

### Login to GHCR (defaults — no input needed in GitHub Actions)

```yaml
- uses: js-fatigue/gha/actions/docker@docker-v1.0.0
  with:
    command: login
```

### Login to a custom registry

```yaml
- uses: js-fatigue/gha/actions/docker@docker-v1.0.0
  with:
    command: login
    input: |
      registry = "registry.example.com"
      username = "${{ secrets.REGISTRY_USER }}"
      password = "${{ secrets.REGISTRY_PASSWORD }}"
```

### Build and push a tagged image

```yaml
- uses: js-fatigue/gha/actions/docker@docker-v1.0.0
  with:
    command: build
    input: |
      tags = ["ghcr.io/${{ github.repository }}:${{ github.sha }}", "ghcr.io/${{ github.repository }}:latest"]
      push = true
```

### Multi-platform build with Actions cache

```yaml
- uses: js-fatigue/gha/actions/docker@docker-v1.0.0
  with:
    command: build
    input: |
      tags       = ["ghcr.io/${{ github.repository }}:latest"]
      platform   = "linux/amd64,linux/arm64"
      push       = true
      cache_from = "type=gha"
      cache_to   = "type=gha,mode=max"
```

### Build a specific stage with build args

```yaml
- uses: js-fatigue/gha/actions/docker@docker-v1.0.0
  with:
    command: build
    input: |
      {
        "context": "./services/api",
        "tags": ["my-api:test"],
        "target": "test",
        "build_args": ["VERSION=1.2.3", "COMMIT=${{ github.sha }}"]
      }
```
