# Build System

## Tools

- **Go toolchain** — standard `go build`, `go test`, `go fmt`
- **Makefile** — canonical build interface for all local development
- **golangci-lint** — static analysis (configured in `.golangci.yml`)
- **GoReleaser** — multi-platform release builds
- **changie** — changelog management

## Makefile Targets

All targets are `.PHONY`. Run `make <target>`:

| Target | Description |
|--------|-------------|
| `build` | Build both `bin/aura-cli` and `bin/neo4j-cli` |
| `build-aura` | Build `bin/aura-cli` from `./neo4j-cli/aura/cmd` |
| `build-neo4j` | Build `bin/neo4j-cli` from `./neo4j-cli` |
| `snapshot` | Release build for current platform with ldflags baked in; copies to `bin/` |
| `test` | Run `go test ./...` |
| `lint` | Run `golangci-lint run ./...` |
| `fmt` | Run `go fmt ./...` |
| `license-check` | Verify all `.go` files carry the Neo4j copyright header (**Unix-only**) |
| `run-aura` | Run `aura-cli` without building (`go run ./neo4j-cli/aura/cmd`) |
| `run-neo4j` | Run `neo4j-cli` without building (`go run ./neo4j-cli`) |
| `clean` | Remove `bin/` and `dist/` directories |
| `changelog` | Create a changie entry — interactive prompt for project(s) and kind |

The Makefile uses `$(shell go env GOPATH)` to resolve tool paths — `license-check` calls `$(GOPATH)/bin/addlicense` directly because `GOPATH/bin` may not be on `PATH`.

## Commands

```bash
# Local development build (both binaries)
make build

# Run without building (local dev)
make run-aura
make run-neo4j

# Tests, lint, format
make test
make lint
make fmt

# License header check (Unix/macOS only)
make license-check

# Single-platform release build (goreleaser + ldflags, outputs to bin/)
make snapshot

# Multi-platform snapshot (all platforms) — both version env vars required
GORELEASER_CURRENT_TAG=dev AURA_CLI_VERSION=dev goreleaser release --snapshot --clean

# Clean build artifacts
make clean
```

## golangci-lint

Config lives at `.golangci.yml` (golangci-lint v2). Enabled checks:

- **Linters**: `govet`, `errcheck`, `staticcheck`, `unused`
- **Formatters**: `gofmt`
- `linters.default: none` — only explicitly listed linters run (no auto-enabled defaults)
- `max-issues-per-linter: 0` and `max-same-issues: 0` — all issues reported

In CI, `golangci/golangci-lint-action@v6` installs, caches, and runs golangci-lint (equivalent to `make lint`).

## Dual-Binary GoReleaser Setup

GoReleaser builds two separate binaries per release:

| Binary | Entrypoint |
|--------|-----------|
| `aura-cli` | `./neo4j-cli/aura/cmd` |
| `neo4j-cli` | `./neo4j-cli` |

Each binary gets its own archive per platform. Version injection differs per binary:
- `aura-cli` uses `-X "main.Version={{.Env.AURA_CLI_VERSION}}"` (custom env var)
- `neo4j-cli` uses `-X "main.Version={{.Env.GORELEASER_CURRENT_TAG}}"` (GoReleaser built-in)

Config key: each `archives` entry must have a unique `id`; archive `name_template` uses `{{ .Binary }}` (not `{{ .ProjectName }}`) so archives are named per binary.

## Changelog

Uses `changie` with a multi-project config (`aura-cli` and `neo4j-cli` projects). Change files are created in `.changes/unreleased/` and tagged with a `project:` field.

**`make changelog`** opens an interactive prompt for project(s) and kind — the project selector supports multi-select. Because `neo4j-cli` bundles its child CLIs, changes to a child require entries for both the child and `neo4j-cli`.

For non-interactive use (agents/scripts):

```bash
changie new --projects aura-cli --projects neo4j-cli --kind Patch --body "description"
```

Changes specific to the `neo4j-cli` wrapper only need a `neo4j-cli` entry.

Commit the generated files — the `changie` CI workflow batches them and opens a release PR.

## Release Process

Releases are triggered automatically in CI when `CHANGELOG-neo4j.md` is updated on `main`. GoReleaser produces binaries for:
- `linux/amd64`, `linux/arm64`
- `windows/amd64`
- `darwin/amd64`, `darwin/arm64`

macOS binaries are code-signed and notarized using App Store Connect keys stored in GitHub secrets.
