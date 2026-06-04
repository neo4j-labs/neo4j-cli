# PRD: Rename `data-api graphql` to `graphql`

## Overview

The current command hierarchy `aura data-api graphql <cmd>` has an unnecessary nesting level. The `data-api` parent exists solely to hold the `graphql` subcommand. This feature collapses that structure so the top-level entry point becomes `aura graphql <cmd>`, with all persistent flags and sub-trees intact.

## Goals

- Expose GraphQL Data API commands directly under `aura graphql` (e.g. `aura graphql list`, `aura graphql create`).
- Remove the intermediate `data-api` parent command entirely (no aliases, no deprecation notices).
- Update Go package layout to match: flatten `subcommands/dataapi/graphql/` → `subcommands/graphql/`.
- Update all descriptions/Short/Long strings to say "GraphQL data APIs" rather than "Data APIs" / "GraphQL Data APIs".

## Non-Goals

- No backward compatibility — `aura data-api graphql` is removed without aliasing.
- No functional changes to any leaf command (flags, API calls, output format all stay identical).
- No changes to any commands outside the `dataapi/` subtree.

## Requirements

### Functional Requirements

- REQ-F-001: `aura graphql list`, `aura graphql get`, `aura graphql create`, `aura graphql update`, `aura graphql delete`, `aura graphql pause`, `aura graphql resume` must all work identically to their current `aura data-api graphql <cmd>` forms.
- REQ-F-002: `aura graphql auth-provider <cmd>` and `aura graphql cors-policy allowed-origin <cmd>` must work identically to their current `aura data-api graphql auth-provider <cmd>` / `aura data-api graphql cors-policy allowed-origin <cmd>` forms.
- REQ-F-003: The `--auth-url`, `--base-url`, and `--credential` (aura credential) persistent flags currently on `data-api` must move to the new `graphql` parent with the same names, types, and `PersistentPreRunE` binding behaviour.
- REQ-F-004: `aura data-api` and `aura data-api graphql` must no longer be valid commands (removed from the tree entirely).
- REQ-F-005: All `Short`, `Long`, and `Example` strings across every affected command must be updated — `Example` paths change from `aura data-api graphql` to `aura graphql`; descriptions use "GraphQL data API" terminology throughout.

### Non-Functional Requirements

- REQ-NF-001: The skill bundle must be regenerated (`go generate ./neo4j-cli/internal/skill/...`) so `TestGenerator_RoundTrip` passes.
- REQ-NF-002: All existing tests must continue to pass unchanged (or with minimal path/import updates only).
- REQ-NF-003: A changelog entry (kind `Minor`) is required — this is a user-facing breaking change (command rename).

## Technical Considerations

### Affected files

**Deleted:**
- `neo4j-cli/aura/internal/subcommands/dataapi/data_api.go` — the `data-api` parent; its flag/PreRunE logic moves into the new `graphql.go`.

**Moved (directory rename):**
- `subcommands/dataapi/graphql/` → `subcommands/graphql/` — all 34 files (`.go` and `_test.go`) move to the new location.
- Subdirectories `authprovider/` and `corspolicy/allowedorigin/` move with their parent.

**Updated import paths (every file that imported `dataapi` or `dataapi/graphql/...`):**
- `neo4j-cli/aura/aura.go`: replace `dataapi.NewCmd(cfg)` with `graphql.NewCmd(cfg)`; update import.
- New `subcommands/graphql/graphql.go`: package `graphql`, imports `authprovider` and `corspolicy` from new paths.
- `subcommands/graphql/authprovider/auth_provider.go`: update import path.
- `subcommands/graphql/corspolicy/cors_policy.go`: update import path.
- `subcommands/graphql/corspolicy/allowedorigin/allowed_origin.go`: update import path.
- All `*_test.go` files: update any internal import paths that reference `dataapi/graphql`.

**Updated `graphql.go`:**
- `Use: "graphql"` (same).
- `Short` / `Long` updated to "Manage GraphQL data APIs".
- Absorbs the `PersistentPreRunE` hook and `--auth-url`, `--base-url`, credential flag registration from the old `data_api.go`.

**Updated Example strings across all leaf commands:**
- Every `Example:` field that contains `aura data-api graphql` must be updated to `aura graphql`.

### Mount point change in `aura.go`

```go
// Before
import "github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/dataapi"
cmd.AddCommand(dataapi.NewCmd(cfg))

// After
import "github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/graphql"
cmd.AddCommand(graphql.NewCmd(cfg))
```

### Package naming

The inner packages (`authprovider`, `corspolicy`, `allowedorigin`) keep their current package names — only their import paths change. The `graphql` package name is unchanged.

## Acceptance Criteria

- [ ] `aura graphql list --instance-id <id>` works end-to-end.
- [ ] `aura graphql create ...` works end-to-end.
- [ ] `aura graphql auth-provider list --instance-id <id> --data-api-id <id>` works end-to-end.
- [ ] `aura graphql cors-policy allowed-origin add ...` works end-to-end.
- [ ] `aura data-api` returns "unknown command" error.
- [ ] `make test` passes with zero failures.
- [ ] `make fmt-check` passes.
- [ ] `make lint` passes.
- [ ] `make license-check` passes.
- [ ] `TestGenerator_RoundTrip` passes (skill bundle regenerated).
- [ ] Changelog entry present under `.changes/unreleased/`.

## Out of Scope

- Renaming any flag names (`--instance-id`, `--data-api-id`, `--auth-url`, `--base-url`, etc.).
- Changing any API endpoint paths.
- Modifying any output field names or table column headers.
- Touching the `graphanalytics` or any other unrelated command tree.

## Open Questions

None — requirements are fully resolved.
