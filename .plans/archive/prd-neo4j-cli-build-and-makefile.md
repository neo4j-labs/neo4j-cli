# PRD: neo4j-cli Build & Makefile-Based Toolchain

## Overview

The repo currently builds and releases only `aura-cli`. `neo4j-cli/main.go` already exists as a "super CLI" that wraps `aura-cli` as a subcommand, but it is commented out of GoReleaser and not wired into CI or documentation. This PRD covers:

1. Enabling the `neo4j-cli` binary as a fully released artifact alongside `aura-cli`
2. Introducing a `Makefile` as the single developer interface for build, test, lint, and format
3. Replacing `staticcheck` with `golangci-lint` (minimal config)
4. Updating CI workflows and documentation to use `make` targets throughout
5. Establishing separate versioning for `neo4j-cli` and `aura-cli` with a defined cascade rule

## Goals

- Developers and CI use `make <target>` for all common operations — no need to remember raw `go` commands
- Both `aura-cli` and `neo4j-cli` are built, tested, and released as first-class binaries
- `golangci-lint` replaces the current `staticcheck` GitHub Action for linting
- All existing lint errors are resolved without breaking changes
- CONTRIBUTING.md and README reflect the new `make`-based workflow
- `neo4j-cli` and `aura-cli` carry independent version numbers, with a defined policy that a new `aura-cli` release requires a corresponding `neo4j-cli` bump
- All developer-facing, agent-facing, and end-user documentation is kept in sync with the updated toolchain and dual-binary setup

## Non-Goals

- Adding new CLI commands or functionality to `neo4j-cli` beyond what already exists
- Migrating the Go module to a multi-module layout (sub-modules per binary)
- Automating the `neo4j-cli` version bump from CI (a manual changie entry is acceptable for now)
- Changing macOS notarization credentials or Apple Developer accounts

## Requirements

### Functional Requirements

- REQ-F-001: A `Makefile` at the repo root exposes at minimum the following targets: `build`, `build-aura`, `build-neo4j`, `test`, `lint`, `fmt`, `license-check`, `run-aura`, `run-neo4j`, `clean`
- REQ-F-002: `make build` produces both `aura-cli` and `neo4j-cli` binaries under `bin/`
- REQ-F-003: `make test` runs `go test ./...` across the whole module
- REQ-F-004: `make lint` runs `golangci-lint run ./...` using a `.golangci.yml` config at the repo root
- REQ-F-005: `make fmt` runs `go fmt ./...`
- REQ-F-006: `make license-check` runs the existing `addlicense` check
- REQ-F-007: GoReleaser `.goreleaser.yaml` is updated to build and archive both `aura-cli` (from `./neo4j-cli/aura/cmd`) and `neo4j-cli` (from `./neo4j-cli`) for linux/windows/darwin × amd64/arm64
- REQ-F-008: Both binaries receive macOS code signing and notarization in GoReleaser (same secrets as current `aura-cli`)
- REQ-F-009: Both binaries inject their version via `-ldflags "-X main.Version={{.Env.GORELEASER_CURRENT_TAG}}"`
- REQ-F-010: The `.github/workflows/test.yml` CI workflow is updated to call `make build`, `make test`, `make lint`, `make fmt`, `make license-check` instead of raw `go`/`staticcheck` commands
- REQ-F-011: The standalone `license.yml` workflow is updated to call `make license-check`
- REQ-F-012: `CONTRIBUTING.md` is updated so all developer command references use `make` targets
- REQ-F-013: `README.md` local-run and build examples are updated to use `make` targets
- REQ-F-014: `golangci-lint` config (`.golangci.yml`) uses a minimal, sensible linter set — suggested baseline: `govet`, `errcheck`, `staticcheck`, `unused`, `gofmt`
- REQ-F-015: All existing golangci-lint errors are fixed before the feature is considered complete, without breaking changes to CLI behaviour
- REQ-F-016: `AGENTS.md` (between the generated markers) is updated to reflect `make` commands in the Feedback Instructions section, the dual-binary build system, and the `neo4j-cli` binary in the Architecture section
- REQ-F-017: `.agents/build.md` is updated to document all Makefile targets, the switch to `golangci-lint`, the dual-binary GoReleaser setup, and the new separate versioning policy
- REQ-F-018: `.agents/testing.md` is updated to reflect `make test` as the canonical test command
- REQ-F-019: `.agents/deployment.md` is updated to reflect the dual-binary release, separate versioning, and the cascade policy requiring a `neo4j-cli` bump with every `aura-cli` release
- REQ-F-020: `docs/usageGuide/A Guide To The New Aura CLI.md` is updated to document the `neo4j-cli` binary: what it is, how to install it, and how `aura-cli` is accessible as `neo4j-cli aura <command>` in addition to the standalone binary
- REQ-F-021: In `neo4j-cli/main.go`, the cobra subcommand returned by `aura.NewCmd(cfg)` must have its `Use` field overridden to `"aura"` before being registered, so the command path is `neo4j-cli aura <subcommand>` rather than `neo4j-cli aura-cli <subcommand>`; the standalone `aura-cli` binary and its `Use` field are unaffected

### Non-Functional Requirements

- REQ-NF-001: `make` targets must work on macOS, Linux, and Windows (CI matrix); use `go env GOPATH` rather than hardcoded paths where tool discovery is needed
- REQ-NF-002: The `Makefile` must not require any tools beyond `go`, `golangci-lint`, and `addlicense` to be pre-installed (all other tooling already comes from the Go module)
- REQ-NF-003: CI wall-clock time should not increase materially — lint and build steps are already sequential

## Technical Considerations

### Existing `neo4j-cli/main.go`

`neo4j-cli/main.go` is already a valid entrypoint. It creates a `neo4j-cli` root command and registers `aura.NewCmd(cfg)` as a subcommand named `aura-cli`. GoReleaser just needs to be uncommented and re-pointed.

### GoReleaser multi-binary

GoReleaser supports multiple entries in `builds:`. The `neo4j-cli` entry should mirror the `aura-cli` entry with `main: ./neo4j-cli` and `binary: neo4j-cli`. The `archives:` block needs a `builds:` filter so each binary gets its own archive, or they can share one archive per platform. Separate archives per binary are recommended for clean download URLs.

The `notarize.macos[].ids` list currently only includes `aura-cli`; it must be extended to include `neo4j-cli`.

### Separate versioning

Currently there is one `CHANGELOG.md` and one changie flow driving a single version tag. To support separate versioning:

- Two separate changie configurations (`.changie-aura.yaml` / `.changie-neo4j.yaml`) with separate change directories and output changelogs are one option
- Alternatively, the existing single-version approach can be extended by tagging two separate git tags on the same commit (e.g. `aura-cli/v1.7.0` and `neo4j-cli/v1.0.0`) and running GoReleaser twice, once per `GORELEASER_CURRENT_TAG`

The policy is: every `aura-cli` release must be accompanied by a `neo4j-cli` release (since `neo4j-cli` embeds `aura-cli`). Initially this can be enforced as a process rule documented in `CONTRIBUTING.md`. The exact tooling mechanism is left as an open question (see below).

### golangci-lint

`staticcheck` is currently invoked via `dominikh/staticcheck-action`. This action and its workflow step will be removed. A `.golangci.yml` will be added at the repo root. The `golangci-lint` binary must be available in CI — install via the official `golangci-lint-action` or via `go install` in the Makefile.

Using the `golangci-lint-action` GitHub Action (official, caches tool binary) is preferred over `go install` in CI for speed.

Suggested minimal `.golangci.yml`:

```yaml
linters:
  enable:
    - govet
    - errcheck
    - staticcheck
    - unused
    - gofmt
  disable-all: false

issues:
  max-issues-per-linter: 0
  max-same-issues: 0
```

### `neo4j-cli aura` subcommand naming

`aura.NewCmd` sets `Use: "aura-cli"` on the root cobra command it returns — appropriate for the standalone binary but wrong when embedded in `neo4j-cli`. The fix is a single line in `neo4j-cli/main.go`:

```go
auraCmd := aura.NewCmd(cfg)
auraCmd.Use = "aura"
cmd.AddCommand(auraCmd)
```

`aura.NewCmd` itself must not be changed, as it is also called by `neo4j-cli/aura/cmd/main.go` for the standalone binary which must keep `Use: "aura-cli"`.

### Documentation scope

All docs that reference build/test/lint commands or binary installation must be updated together in the same PR as the code changes. The affected files are:

- `AGENTS.md` — agent-facing, between `<!-- BEGIN GENERATED: AGENTS-MD -->` markers only
- `.agents/build.md`, `.agents/testing.md`, `.agents/deployment.md` — detail files for agents
- `CONTRIBUTING.md` — developer-facing, commands and release policy sections
- `README.md` — end-user quick-start commands
- `docs/usageGuide/A Guide To The New Aura CLI.md` — end-user installation and usage guide; add a `neo4j-cli` section describing the super CLI, installation, and the `neo4j-cli aura-cli` invocation path

### License check in Makefile

The `addlicense` command uses `$(find . -name "*.go" ...)` which works on Unix. On Windows CI the `find` command differs; the existing CI already runs this only on `ubuntu-latest` so the Makefile target can be similarly restricted, or we can note in the Makefile that `license-check` requires a Unix shell.

## Acceptance Criteria

- [ ] `make build` produces `bin/aura-cli` and `bin/neo4j-cli` locally
- [ ] `make test` passes with no failures
- [ ] `make lint` passes with zero issues (after fixing pre-existing errors)
- [ ] `make fmt` runs without error
- [ ] `make license-check` passes on all `.go` files
- [ ] CI (`test.yml`) passes on ubuntu, windows, and macos using `make` targets
- [ ] GoReleaser snapshot build (`GORELEASER_CURRENT_TAG=dev goreleaser release --snapshot --clean`) produces archives for both `aura-cli` and `neo4j-cli`
- [ ] Both binaries report a version string when invoked with `--version`
- [ ] `CONTRIBUTING.md` and `README.md` reference `make` commands, not raw `go` commands
- [ ] `.golangci.yml` exists at repo root with minimal linter config
- [ ] `CONTRIBUTING.md` documents the policy that a new `aura-cli` release requires a matching `neo4j-cli` changelog entry
- [ ] `AGENTS.md` Feedback Instructions reference `make` targets; Architecture section mentions `neo4j-cli` entrypoint
- [ ] `.agents/build.md` documents all Makefile targets, golangci-lint, dual-binary GoReleaser, and versioning policy
- [ ] `.agents/testing.md` shows `make test` as the canonical command
- [ ] `.agents/deployment.md` reflects dual-binary releases and cascade versioning rule
- [ ] `docs/usageGuide/A Guide To The New Aura CLI.md` covers `neo4j-cli` installation and the `neo4j-cli aura` subcommand path
- [ ] `neo4j-cli aura instance list` (and other aura subcommands) works correctly via the super CLI
- [ ] `neo4j-cli aura-cli` does not resolve as a valid subcommand (use field is `aura`, not `aura-cli`)

## Out of Scope

- Adding new subcommands to `neo4j-cli` beyond `aura-cli`
- Multi-module Go workspace (`go.work`) setup
- Fully automated `neo4j-cli` version cascading from CI (manual policy for now)
- Changing the changie release PR automation

## Open Questions

1. **Versioning mechanism:** Should two separate changie configs + CHANGELOG files be introduced, or should the single CHANGELOG be extended with a tagging convention? This needs a decision before the release workflow can be updated.
2. **Archive layout:** Should each binary get its own archive (e.g. `neo4j-cli_v1.0.0_Linux_x86_64.tar.gz`) or should both binaries ship in one combined archive per platform?
3. **Windows `make`:** Windows CI currently runs `go` commands directly. Should we add `mingw32-make` or install `make` in the Windows CI job, or exclude `lint`/`license-check` targets from Windows and only run them on ubuntu?
