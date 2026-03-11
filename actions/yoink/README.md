# actions/yoink

Captures `ACTIONS_CACHE_URL`, `ACTIONS_RUNTIME_TOKEN`, `ACTIONS_RESULTS_URL`, and `ACTIONS_RUNTIME_URL` as step outputs so composite `run:` steps can access them. GitHub only injects these variables into Docker action environments, not composite run steps.

## Why Docker, not JavaScript

A JS action is the obvious one-liner solution, but GitHub Actions does not support JavaScript actions on arm64 runners when the runner is using an arm64 Docker image (e.g. Alpine). Avoiding JS actions is a core design goal of this project. Yoink is therefore a minimal Docker action — `busybox:musl` plus a five-line shell script.

## Outputs

| Output | Description |
|---|---|
| `ACTIONS_CACHE_URL` | Actions cache service URL |
| `ACTIONS_RUNTIME_TOKEN` | Actions runtime token |
| `ACTIONS_RESULTS_URL` | Actions results service URL |
| `ACTIONS_RUNTIME_URL` | Actions runtime URL |

## Usage

Yoink is used internally by `actions/github` and `actions/go`. The pattern:

```yaml
steps:
  - id: yoink
    uses: js-fatigue/gha/actions/yoink@main

  - name: Some action
    uses: some-action@foo-v1.2.3
    env:
      ACTIONS_CACHE_URL: ${{ steps.yoink.outputs.ACTIONS_CACHE_URL }}
      ACTIONS_RUNTIME_TOKEN: ${{ steps.yoink.outputs.ACTIONS_RUNTIME_TOKEN }}
      ACTIONS_RESULTS_URL: ${{ steps.yoink.outputs.ACTIONS_RESULTS_URL }}
      ACTIONS_RUNTIME_URL: ${{ steps.yoink.outputs.ACTIONS_RUNTIME_URL }}
    with:
      foo: "bar"
```

## Deployment

The image (`ghcr.io/js-fatigue/gha-yoink`) is rebuilt by `deploy-yoink.yml` on every push to `main` that touches `images/yoink/`, tagged as both `:latest` and `:sha-<7char>`. The SHA is pinned in `action.yml`. Update it after pushing a new image.
