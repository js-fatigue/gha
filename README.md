# gha

A Go-based framework for writing GitHub Actions. Replaces bash/Python/JavaScript action steps with compiled Go binaries for better performance and lower-level control. Actions are organized into "families", each compiled to a standalone binary and distributed via GitHub Releases.

## Action Families

| Family | Description |
|--------|-------------|
| [`github`](actions/github/README.md) | GitHub-specific commands: PR title validation, changed-dirs detection, releases, checkout |
| [`go`](actions/go/README.md) | Go toolchain commands: build, setup, cache |
| [`yoink`](actions/yoink/README.md) | Minimal Docker action that captures Actions cache service env vars for composite steps |


TODOS:

Deep dive to double-check my intention versus what claude produced
High level overview of project architecture
Diagram of action release process
Prep for demo/walkthrough
List of pros/cons when compared to bash/ts actions
Have claude write more unit tests and make the code more testable where possible

test an action without specifying secrets.GITHUB_TOKEN
