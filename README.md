# gha

A Go-based framework for writing GitHub Actions. Replaces bash/Python/JavaScript action steps with compiled Go binaries for better performance and lower-level control. Actions are organized into "families", each compiled to a standalone binary and distributed via GitHub Releases.

## Action Families

| Family | Description |
|--------|-------------|
| [`github`](actions/github/README.md) | GitHub-specific commands: PR title validation, changed-dirs detection, releases, checkout |
| [`go`](actions/go/README.md) | Go toolchain commands: build, setup, cache |
| [`yoink`](actions/yoink/README.md) | Minimal Docker action that captures Actions cache service env vars for composite steps |

## High-level Overview

Actions are grouped into "families" of the technology they interact with (actions/go, actions/github, etc).

Each action family contains a single `action.yml` file invoking a `bootstrap.sh` script.

The script prepares the go binary, and executes it with the command from the action's input as an argument.

![](docs/action-family.svg)

TODOS:

Deep dive to double-check my intention versus what claude produced
Prep for demo/walkthrough
List of pros/cons when compared to bash/ts actions
Have claude write more unit tests and make the code more testable where possible
