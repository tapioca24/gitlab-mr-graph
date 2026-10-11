# Development guide

## Prerequisites

- Go 1.24 or later
- Node.js (for JavaScript tests and syntax checks)
- make, a POSIX shell, and Bash

Connecting to GitLab also requires an authenticated GitLab CLI (`glab`).

## Build and try it out

```sh
make build
./bin/gitlab-mr-graph --demo
```

The executable is generated at `bin/gitlab-mr-graph`. The demo uses sample data, so GitLab authentication is not required.

## Tests and static checks

```sh
make test
make check
```

`make test` runs Go tests with the race detector enabled. Tests use fixed API responses and local HTTP servers, so no connection to GitLab is required.

JavaScript tests use Node.js's built-in test runner to verify filtering and graph layout. No additional packages need to be installed.

`make check` runs `go vet` and syntax checks for JavaScript and shell scripts. CI runs tests, checks, and builds on macOS / Linux with Go 1.24 / 1.26.

## Build release artifacts

```sh
bash scripts/build-release.sh v0.1.0
```

This generates executables for macOS / Linux × arm64 / amd64 and `checksums.txt` in `dist/`. Specify the version in `vMAJOR.MINOR.PATCH` format. Release builds embed this version. When installed with `go install ...@version`, the tool displays the module version from Go's build information.

## Releases

Invoke `$release` in Codex to use the repository's [release skill](.agents/skills/release/SKILL.md). It reviews changes since the previous release and proposes the next version with a rationale. After validation, once the user confirms the version and target commit, it pushes the tag and verifies publication.

The target is normally the tip of `origin/main`. The skill stops if there are uncommitted changes or unpushed commits.

Tag the commit to publish in `vMAJOR.MINOR.PATCH` format and push the tag to GitHub.

```sh
git tag v0.1.0
git push origin v0.1.0
```

The Release workflow runs tests and static checks, builds release artifacts, and publishes the executables and checksums to a GitHub Release.

The same tags are also used for distribution via `go install github.com/tapioca24/gitlab-mr-graph/cmd/gitlab-mr-graph@latest`. No installation script or built-in automatic update feature is provided.
