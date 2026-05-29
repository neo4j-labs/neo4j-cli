# PRD: Remove Unused Data Import and Deployment Commands (CLI-163)

## Overview

Delete the `import` and `deployment` subcommand trees from the Aura CLI. Both are gated behind `flag.aura-beta` and have never been visible to regular users. The data importer commands cannot complete an end-to-end flow via the API; the deployment (fleet manager) commands are intended to be replaced with a new "deploy to Aura" surface in the super-CLI context. Removing them now reduces dead code and unblocks CLI-154 (drop beta flag on Aura API commands).

## Goals

- Delete all source and test files under `neo4j-cli/aura/internal/subcommands/import/` and `neo4j-cli/aura/internal/subcommands/deployment/`.
- Remove the corresponding `AddCommand` calls and imports from `aura.go`.
- Leave the `flag.aura-beta` gate and `dataapi` subcommand completely untouched — those are addressed by CLI-154.
- All existing tests pass and the skill bundle does not drift.

## Non-Goals

- Removing or modifying the `flag.aura-beta` feature flag registry entry — that is CLI-154's scope.
- Removing or modifying the `dataapi` subcommand.
- Adding a replacement "deploy to Aura" command.
- Adding a changelog entry — these commands were never user-visible (beta flag was never documented or surfaced to end users).

## Requirements

### Functional Requirements

- REQ-F-001: The `import` subcommand tree (`import job create`, `import job get`, `import job cancel`) is fully deleted.
- REQ-F-002: The `deployment` subcommand tree (`deployment create/get/list/delete`, `deployment database list`, `deployment server list`, `deployment server database list`, `deployment token create/delete/update`) is fully deleted.
- REQ-F-003: `aura.go` no longer imports or registers either package; the `if cfg.Flags.Enabled("flag.aura-beta")` block contains only `dataapi.NewCmd(cfg)`.
- REQ-F-004: No API-layer changes are needed — neither `import` nor `deployment` have dedicated code in `neo4j-cli/aura/internal/api/`.

### Non-Functional Requirements

- REQ-NF-001: `make test`, `make fmt-check`, and `make lint` must all pass after the deletion.
- REQ-NF-002: The skill bundle must not drift — `go generate ./neo4j-cli/internal/skill/...` must produce no diff (import/deployment were never in the bundle because they were behind the beta flag during generation).

## Technical Considerations

**Files to delete (entire directories):**
- `neo4j-cli/aura/internal/subcommands/import/` — contains `import.go` and `job/` subdirectory (`job.go`, `create.go`, `get.go`, `cancel.go` plus their `_test.go` counterparts).
- `neo4j-cli/aura/internal/subcommands/deployment/` — contains `deployment.go`, `create.go`, `delete.go`, `get.go`, `list.go`, their tests, plus `database/`, `server/`, `server/database/`, and `token/` subdirectories.

**`aura.go` changes:**
- Remove import alias `_import "github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/import"`.
- Remove import `"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/deployment"`.
- Inside the `if cfg.Flags.Enabled("flag.aura-beta")` block, remove `cmd.AddCommand(_import.NewCmd(cfg))` and `cmd.AddCommand(deployment.NewCmd(cfg))`.
- The block now contains only `cmd.AddCommand(dataapi.NewCmd(cfg))`.

**Skill bundle:** Since the bundle is generated with default config (beta flag off), import/deployment never appear in `bundle/`. No `go generate` changes are expected, but it should be run to verify there is no drift.

**`flags.go`:** Do not touch — the `Gates` description update (removing `import, deployment` from the field) is deferred to CLI-154 which removes the flag entirely.

## Acceptance Criteria

- [ ] `neo4j-cli/aura/internal/subcommands/import/` does not exist.
- [ ] `neo4j-cli/aura/internal/subcommands/deployment/` does not exist.
- [ ] `aura.go` has no reference to the `import` or `deployment` packages; the beta-flag block contains only `dataapi`.
- [ ] `make test` passes with no failures.
- [ ] `make fmt-check` passes.
- [ ] `make lint` passes.
- [ ] `go generate ./neo4j-cli/internal/skill/...` produces no diff to tracked files.

## Out of Scope

- Replacement "deploy to Aura" command.
- Modifications to `flag.aura-beta` registry entry or any `flags.go` content.
- Changes to the `dataapi` subcommand.
- Changelog entry.

## Open Questions

None — scope is fully confirmed.
