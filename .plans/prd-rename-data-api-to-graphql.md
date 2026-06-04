# PRD: Rename `data-api graphql` to `graphql`

## Overview

The current command hierarchy `aura data-api graphql <cmd>` has an unnecessary nesting level. The `data-api` parent exists solely to hold the `graphql` subcommand. This feature collapses that structure so the top-level entry point becomes `aura graphql <cmd>`, with all persistent flags and sub-trees intact.

## Goals

- Expose GraphQL Data API commands directly under `aura graphql` (e.g. `aura graphql list`, `aura graphql create`).
- Remove the intermediate `data-api` parent command entirely (no aliases, no deprecation notices).
- Update Go package layout to match: flatten `subcommands/dataapi/graphql/` → `subcommands/graphql/`.
- Update all descriptions/Short/Long strings to say "GraphQL data APIs" rather than "Data APIs" / "GraphQL Data APIs".
- Make `--name` optional on `graphql create` with auto-generated names (`GraphQL01`, `GraphQL02`, …).

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
- REQ-F-006: `--name` on `graphql create` MUST be optional (remove `MarkFlagRequired`). When omitted the CLI must auto-generate a unique name.
- REQ-F-007: Auto-name generation must query `GET /instances/{id}/data-apis/graphql` (the same endpoint used by `graphql list`) for the given `--instance-id`, collect the `name` field from each entry, then return the lowest unused name of the form `GraphQL01`, `GraphQL02`, …, `GraphQL99`, `GraphQL100`, … (zero-padded to two digits for 1–99, full decimal for 100+), case-insensitively avoiding collisions.

### Non-Functional Requirements

- REQ-NF-001: The skill bundle must be regenerated (`go generate ./neo4j-cli/internal/skill/...`) so `TestGenerator_RoundTrip` passes.
- REQ-NF-004: The name-generation helper must be tested in isolation (unit tests for the `defaultGraphQLName` function) and the auto-name flow must be covered by an integration-style test using the mock HTTP server.
- REQ-NF-002: All existing tests must continue to pass unchanged (or with minimal path/import updates only).
- REQ-NF-003: A changelog entry (kind `Minor`) is required — this is a user-facing breaking change (command rename).

## Technical Considerations (auto-name addition)

### Auto-name helpers

Mirror the instance pattern exactly:

- Add `neo4j-cli/aura/internal/subcommands/graphql/name_helpers.go` with a `defaultGraphQLName(existingNames []string) string` function using the same zero-padding logic as `defaultInstanceName` but with the `GraphQL` prefix.
- Add a `resolveGraphQLName(cfg, name, instanceID string) (string, error)` function (can live in `create.go` or a new `create_core.go`) that returns `name` unchanged when non-empty, or fetches existing names from `GET /instances/{instanceId}/data-apis/graphql` and calls `defaultGraphQLName`.
- The `--name` flag registration stays but `cmd.MarkFlagRequired(nameFlag)` is removed.
- The `RunE` body calls `resolveGraphQLName` before building the request body.
- Add `name_helpers_test.go` with a table-driven unit test for `defaultGraphQLName`.
- Add a test case in `create_test.go` covering the auto-name path (mock the list endpoint to return existing names and assert the generated name in the POST body).

### API call order on create

The auto-name path adds one extra `GET` before the `POST`. This only happens when `--name` is omitted; the fast path (explicit name) is unchanged.

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
- [ ] `graphql create --instance-id X --type-definitions T --rw` (no `--name`) succeeds and sends an auto-generated name (e.g. `GraphQL01`) in the request body.
- [ ] `graphql create --instance-id X --name my-api ...` continues to work as before.
- [ ] Auto-name skips names already used by existing data APIs (collision avoidance verified by test).
- [ ] `name_helpers_test.go` covers the `defaultGraphQLName` function with table-driven cases.

## Out of Scope

- Renaming any flag names (`--instance-id`, `--data-api-id`, `--auth-url`, `--base-url`, etc.).
- Changing any API endpoint paths.
- Modifying any output field names or table column headers.
- Touching the `graphanalytics` or any other unrelated command tree.

## Open Questions

None — requirements are fully resolved.
