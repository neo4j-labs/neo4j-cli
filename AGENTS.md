<!-- BEGIN GENERATED: AGENTS-MD -->

# AGENTS.md

Learnings and patterns for future agents working on this project.

## Feedback Instructions

TEST COMMANDS: [`make test`]
BUILD COMMANDS: [`make build`, `make run-aura`, `make run-neo4j`]
LINT COMMANDS: [`make lint`]
FORMAT COMMANDS: [`make fmt`]
LICENSE CHECK: [`make license-check`]

## Project Overview

PRIMARY LANGUAGES: [Go]

Neo4j Aura CLI (`aura-cli`) is a command-line tool for interacting with the [Neo4j Aura API](https://neo4j.com/docs/aura/platform/api/specification/). It allows users to manage Aura instances, credentials, tenants, deployments, data APIs, and more from the terminal.

## Build System

BUILD SYSTEMS: [Go toolchain, Makefile, golangci-lint, GoReleaser, changie]

See [`.agents/build.md`](.agents/build.md) for full details.

- Local build: `make build` (produces `bin/aura-cli` and `bin/neo4j-cli`)
- Local run (no build): `make run-aura` / `make run-neo4j`
- Multi-platform snapshot: `GORELEASER_CURRENT_TAG=dev goreleaser release --snapshot --clean`
- All `.go` files must start with the Neo4j copyright header (enforced in CI via `addlicense`)
- PRs require a changelog entry added via `changie new` (prompts for project selection: `aura-cli` or `neo4j-cli`)
- Cascade rule: `aura-cli` changes automatically cascade into `neo4j-cli` via CI — no second `changie new` call needed

## Testing Framework

TESTING FRAMEWORKS: [Go testing, testify, afero (in-memory FS)]

See [`.agents/testing.md`](.agents/testing.md) for full details.

- Tests are colocated with source as `*_test.go` files
- Run with `go test ./...`; CI runs on ubuntu, windows, and macos
- Mock HTTP server and filesystem helpers live in `neo4j-cli/aura/internal/test/testutils/`
- `neo4j-cli/` (the super-CLI package) has no test files; this is a pre-existing gap

## Architecture

ARCHITECTURE PATTERN: Cobra command tree — one file per leaf command, directory structure mirrors command hierarchy

See [`.agents/architecture.md`](.agents/architecture.md) for full details.

Two binaries are produced:
- **`neo4j-cli`** — super-CLI entrypoint (`neo4j-cli/main.go`); wraps `aura-cli` under the `aura` subcommand
- **`aura-cli`** — standalone Aura CLI (`neo4j-cli/aura/cmd/main.go`)

```
neo4j-cli/
  main.go                  # neo4j-cli entrypoint; mounts aura subcommand as "aura"
  aura/
    cmd/main.go            # aura-cli standalone entrypoint
    aura.go                # Root command, registers subcommands
    internal/
      api/                 # HTTP client for Neo4j Aura REST API
      flags/               # Custom reusable flag types
      output/              # JSON + table rendering
      subcommands/         # One directory per resource, one file per action
        instance/, tenant/, credential/, config/,
        deployment/, dataapi/graphql/, graphanalytics/,
        import/, customermanagedkey/
common/
  clicfg/                  # Config, credentials, project state (OS-specific paths)
  clierr/                  # Shared error types
```

Key CLI conventions (see `CONTRIBUTING.md`):
- Singular nouns for commands (`instance`, not `instances`)
- `<resource> <action>` form (`instance list`, not `list-instance`)
- One positional argument max; extras become flags
- `--output json|table` for all read commands
- `--await` flag for async operations

## Deployment

DEPLOYMENT STRATEGY: GitHub Releases via GoReleaser, triggered by CHANGELOG.md updates on `main`

See [`.agents/deployment.md`](.agents/deployment.md) for full details.

- `changie` batches changelog entries and opens release PRs automatically
- Merging a release PR triggers GoReleaser to publish binaries for linux/windows/darwin (amd64 + arm64)
- macOS binaries are code-signed and notarized

## Makefile Notes

- `license-check` target uses `$(GOPATH)/bin/addlicense` (not bare `addlicense`) — GOPATH/bin may not be on PATH
- `license-check` requires a Unix shell (`find` + `xargs`); won't work natively on Windows

## Changie Multi-Project Notes

- `ProjectConfig` in changie does NOT support `changesDir` or `changelogPath` fields — only `label`, `key`, `changelog`, and `replacements`
- Version files live at `changesDir/<key>/v*.md` (e.g., `.changes/aura-cli/v1.7.0.md`) — changie appends the project key to `changesDir` automatically
- All projects share a single unreleased directory at `changesDir/unreleasedDir/` (e.g., `.changes/unreleased/`) — change files are tagged with `project:` field inside the YAML, not by directory
- `changie latest --project aura-cli` outputs `aura-cliv1.7.0` (project key prepended with no separator by default) — use `--remove-prefix` to strip `v` but key is always prepended; shell workflows must strip `aura-cli` prefix (e.g., `sed 's/^aura-cli//'`)
- `ProjectsVersionSeparator` in `.changie.yaml` can be set to `-` to get `aura-cli-v1.7.0` instead of `aura-cliv1.7.0`; leave unset (empty) for `aura-cliv1.7.0`
- The cascade (aura-cli changes appearing in neo4j-cli) must copy unreleased YAML files and change their `project:` field, not copy between directories
- `changie merge` (no flags) automatically iterates all `projects:` in config and writes each to its own `changelog:` path — confirmed from source (`cmd/merge.go`). Calling `changie merge --project` is not supported by changie's CLI.
- CI cascade pattern: `for file in .changes/unreleased/*.yaml; do [ -e "$file" ] || continue; grep -q "^project: aura-cli$" "$file" && sed 's/^project: aura-cli$/project: neo4j-cli/' "$file" > "${file%.yaml}-neo4j-cli.yaml"; done || true`

## GoReleaser Notes

- GoReleaser v2 deprecates `archives.format` (string) — use `archives.formats` (list)
- GoReleaser v2 deprecates `format_overrides.format` — use `format_overrides.formats`
- Each `archives` entry must have a unique `id`; omitting it defaults to `"default"` and causes errors when there are multiple archive blocks
- Use `{{ .Binary }}` in `name_template` (not `{{ .ProjectName }}`) when building multiple binaries so archives are named per binary

## golangci-lint Notes

- Version installed: v2.11.4 (via Homebrew)
- golangci-lint v2 requires `version: "2"` at the top of `.golangci.yml`
- In v2, `gofmt` is a **formatter** (not a linter); put it under `formatters.enable`, not `linters.enable`
- Use `linters.default: none` to disable auto-enabled defaults (e.g. `ineffassign`) and run only explicitly listed linters
- Config lives at `.golangci.yml` in repo root
- In CI, `golangci/golangci-lint-action@v6` is used as the lint step — it installs, caches, and runs golangci-lint using `.golangci.yml`. This is equivalent to `make lint`. Renovate will pin the SHA.

---

_This AGENTS.md was generated using agent-based project discovery._

<!-- END GENERATED: AGENTS-MD -->
