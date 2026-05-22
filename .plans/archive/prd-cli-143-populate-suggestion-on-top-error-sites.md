# PRD: CLI-143 — Populate `Suggestion` on top error sites

Linear: https://linear.app/neo4j/issue/CLI-143/populate-suggestion-on-top-error-sites-cli-108-d
Parent: CLI-108 audit (F3).

## Overview

CLI-108-a landed `CLIError.Suggestion` (`common/clierr/error.go:28`) and the renderer wiring (plaintext second line, JSON `suggestion` envelope field, toon `suggestion` field; tests at `common/clierr/render_test.go:232-444`). Zero production sites populate it.

This feature wires next-action hints at the top 4xx friction sites identified by the CLI-108 audit (F3), refreshed for the post-CLI-120 project/org/workspace model. After this lands, every common error a user hits ends with one stderr line (or one structured envelope field) telling them exactly which command to run next.

## Goals

- Populate `CLIError.Suggestion` at all top-of-funnel error sites in the Aura command tree: 400 validation, 401/403 auth, 404 (per resource type), 429 rate limit, and the workspace/ownership errors emitted by `utils/resolve.go`.
- Convert the remaining plain `fmt.Errorf` sites in `neo4j-cli/aura/internal/subcommands/utils/resolve.go` to typed `*clierr.CLIError` so they participate in the same envelope and renderer paths.
- Add per-resource-type suggestion text for the 404 envelope (instance, project, organization, customer-managed-key, tenant) via a small central lookup.
- For nested-resource paths where `parseResourceFromRequest` extracts the wrong segment (graph-analytics sessions, instance snapshots), enrich at the call site with a single shared helper.
- Lock the behaviour with focused unit tests at each site asserting `Code`, `ResourceType`, `ResourceID`, and `Suggestion`.

## Non-Goals

- No changes to `common/clierr/render.go` or its tests — wiring is already in place.
- No new exported API surface (`WithSuggestion` / `WithResource` already exist).
- No bundle / generate / SKILL.md changes — no command-tree modifications.

## Requirements

### Functional Requirements

**API-layer enrichment (`neo4j-cli/aura/internal/api/response.go`)**

- REQ-F-001: The 400 (`response.go:47-64`) branch returns a `*clierr.CLIError` whose `Suggestion` is `See 'neo4j-cli aura <cmd> --help' for valid flags and values.`.
- REQ-F-002: The 401 branch (`response.go:65-66` → `formatAuthorizationError` at `:360-383`) and the 403 branch (`response.go:67-78`, both the `serverError.Error != ""` short-circuit and the `formatAuthorizationError` fallthrough) return `*clierr.CLIError`s whose `Suggestion` is `Run 'neo4j-cli credential aura-client add' to refresh credentials, then retry.`.
- REQ-F-003: The 404 branch (`response.go:79-92`) calls a new private helper `suggestionForResource(resourceType string) string` whose return value is attached via `.WithSuggestion(...)`. The lookup table:
  - `instance` → `Run 'neo4j-cli aura instance list' to see available instances.`
  - `project` → `Run 'neo4j-cli aura project list --organization-id <id>' to see available projects.`
  - `organization` → `Run 'neo4j-cli aura organization list' to see available organizations.`
  - `customer-managed-key` → `Run 'neo4j-cli aura customer-managed-key list' to see customer-managed keys.`
  - `tenant` → `Run 'neo4j-cli aura project list' to see available projects (tenants are now called projects).`
  - default → `""` (renderer + `omitempty` already handle no-suggestion gracefully).
- REQ-F-004: The 429 branch (`response.go:121-123`) returns a `*clierr.CLIError` whose `Suggestion` is `Retry after <retryAfter> seconds.` where `<retryAfter>` is the `Retry-After` header value. The existing message body is unchanged for backwards-compat.

**`utils/resolve.go` typed-error conversion**

- REQ-F-005: `resolveIDs` (`utils/resolve.go:55,57,68`) returns `*clierr.NewUsageError(...)` with the following suggestions, preserving the existing message strings:
  - `:55` legacy default-tenant migration → `Run 'neo4j-cli aura workspace use <org-id>/<project-id>' to migrate from the legacy default-tenant setting.`
  - `:57` no org specified → `Run 'neo4j-cli aura workspace use <org-id>/<project-id>' to set a default workspace, or pass '--organization-id'.`
  - `:68` no project specified → `Run 'neo4j-cli aura workspace use <org-id>/<project-id>' to set a default workspace, or pass '--project-id'.`
- REQ-F-006: `validateProjectInOrg` (`utils/resolve.go:88`) returns `*clierr.NewNotFoundError(...).WithResource("project", projectID).WithSuggestion("Run 'neo4j-cli aura project list --organization-id <id>' to see available projects.")`.
- REQ-F-007: `FetchAndVerifyInstanceInProject` (`utils/resolve.go:119`) ownership-mismatch error returns `*clierr.NewNotFoundError(...).WithResource("instance", instanceID).WithSuggestion("Run 'neo4j-cli aura instance list --project-id <id>' to see instances in this project.")`.
- REQ-F-008: `FetchAndVerifySessionInProject` (`utils/resolve.go:153`) ownership-mismatch error returns `*clierr.NewNotFoundError(...).WithResource("graph-analytics-session", sessionID).WithSuggestion("Run 'neo4j-cli aura graph-analytics session list --project-id <id>' to see sessions in this project.")`.
- REQ-F-009: `FetchAndVerifyCMKInProject` (`utils/resolve.go:187`) ownership-mismatch error returns `*clierr.NewNotFoundError(...).WithResource("customer-managed-key", cmkID).WithSuggestion("Run 'neo4j-cli aura customer-managed-key list --project-id <id>' to see keys in this project.")`.
- REQ-F-010: The preflight "unexpected status" `fmt.Errorf` sites at `utils/resolve.go:108,142,176` are left as plain errors (internal contract violation, not user-actionable).

**Caller-side enrichment for nested 404s**

- REQ-F-011: A new private helper `WithNotFoundContext(err error, resourceType, resourceID, suggestion string) error` lives in `neo4j-cli/aura/internal/subcommands/utils/`. It uses `errors.As` to detect a `*clierr.CLIError` with `Code == 3`; if matched, it mutates `ResourceType`, `ResourceID`, and `Suggestion` and returns the same error. Non-matching errors pass through unchanged.
- REQ-F-012: `instance/snapshot/get.go` calls `WithNotFoundContext(err, "snapshot", snapshotID, "Run 'neo4j-cli aura instance snapshot list --instance-id <id>' to see snapshots for this instance.")` after the direct snapshot GET (the preflight `FetchAndVerifyInstanceInProject` already covers the instance-not-found path via REQ-F-007).
- REQ-F-013: `graphanalytics/session/get.go` and `graphanalytics/session/delete.go` call `WithNotFoundContext(err, "graph-analytics-session", sessionID, "Run 'neo4j-cli aura graph-analytics session list --project-id <id>' to see sessions in this project.")` after the direct session call. (Preflight `FetchAndVerifySessionInProject` already covers the ownership-mismatch path via REQ-F-008.)

### Non-Functional Requirements

- REQ-NF-001: No regressions in existing tests. Existing `err.Error()` string-equality assertions in `utils/resolve_test.go` keep their expected strings; new assertions are added alongside, not in place of, the message-shape checks.
- REQ-NF-002: All gates clean: `make fmt-check`, `make lint`, `make test`, `make generate-check`.
- REQ-NF-003: No new copyright header issues — all touched / new `.go` files start with the standard `// Copyright (c) "Neo4j" / Neo4j Sweden AB ...` header.
- REQ-NF-004: Changelog entry under `.changes/unreleased/` via `make changelog` (kind: `Minor`). User-facing change.

## Technical Considerations

### Architecture

Two enrichment layers:

1. **API-layer (single point of change)** — `response.go` already constructs typed `CLIError`s for every HTTP status it handles. Adding `.WithSuggestion(...)` is a one-liner per case. The 404 lookup is a `switch` keyed on `resourceType` from the existing `parseResourceFromRequest` helper — no new path parsing.
2. **Caller-side (narrow)** — Only used where the API-layer's `parseResourceFromRequest` extracts the wrong segment:
   - `/v1/instances/{id}/snapshots/{id}` → returns `"instance"`, but the user thinks "snapshot is missing".
   - `/v1beta5/graph-analytics/sessions/{id}` → returns `"graph-analytic"`, which is wrong.
   - The `WithNotFoundContext` helper overwrites `ResourceType`/`ResourceID` and attaches a `Suggestion`, keeping each caller a one-liner.

### Reusable utilities (already exist)

- `clierr.NewValidationError`, `NewAuthError`, `NewNotFoundError`, `NewRateLimitError`, `NewUsageError` — all return `*CLIError` so `.WithSuggestion(...)` chains cleanly.
- `clierr.CLIError.WithSuggestion` (`common/clierr/error.go:53`) — mutating, returns receiver.
- `clierr.CLIError.WithResource` (`common/clierr/error.go:44`) — mutating, returns receiver.
- `api.parseResourceFromRequest` (`response.go:149`) and `api.singularise` (`response.go:168`) — used by REQ-F-003 unchanged.
- `testutils.NewAuraTestHelper` + `NewRequestHandlerMock` (`aura/internal/test/testutils/auratesthelper.go`) — pattern for http-mocked subcommand tests.

### Test patterns

```go
var ce *clierr.CLIError
require.True(t, errors.As(err, &ce))
require.Equal(t, 3, ce.Code)
require.Equal(t, "instance", ce.ResourceType)
require.Equal(t, "<expected suggestion>", ce.Suggestion)
```

This pattern already appears in `aura/internal/api/response_test.go` (404 / ResourceType cases) — extend, don't duplicate.

### Files touched

- `neo4j-cli/aura/internal/api/response.go` — 400/401/403/404/429 sites + new `suggestionForResource` helper.
- `neo4j-cli/aura/internal/api/response_test.go` — extend existing 4xx cases + add a sub-test per resource-type entry in the lookup table.
- `neo4j-cli/aura/internal/subcommands/utils/resolve.go` — 5 `fmt.Errorf` → typed CLIError conversions.
- `neo4j-cli/aura/internal/subcommands/utils/resolve_test.go` — add `errors.As` + `Suggestion` assertions alongside existing message-shape checks.
- `neo4j-cli/aura/internal/subcommands/utils/` (new file `notfound.go` or extend `rename.go`) — `WithNotFoundContext` helper + colocated test.
- `neo4j-cli/aura/internal/subcommands/instance/snapshot/get.go` — direct-404 enrichment.
- `neo4j-cli/aura/internal/subcommands/instance/snapshot/get_test.go` — new 404 sub-test.
- `neo4j-cli/aura/internal/subcommands/graphanalytics/session/get.go` — direct-404 enrichment.
- `neo4j-cli/aura/internal/subcommands/graphanalytics/session/get_test.go` — new 404 sub-test.
- `neo4j-cli/aura/internal/subcommands/graphanalytics/session/delete.go` — direct-404 enrichment.
- `neo4j-cli/aura/internal/subcommands/graphanalytics/session/delete_test.go` — new 404 sub-test.
- `.changes/unreleased/neo4j-cli-Minor-<timestamp>.yaml` — changelog entry.

### Risks / call-site fanout

`utils/resolve.go` is used at 18+ sites across instance / customermanagedkey / graphanalytics. The conversion does not change return signatures (already `error`), so caller code is untouched. Test files that match the error via `err.Error()` string-equality keep working because the message strings are preserved verbatim.

## Acceptance Criteria

- [ ] `response.go` 400 returns `CLIError.Code == 6` with `Suggestion == "See 'neo4j-cli aura <cmd> --help' for valid flags and values."`.
- [ ] `response.go` 401 and 403 paths return `CLIError.Code == 4` with `Suggestion == "Run 'neo4j-cli credential aura-client add' to refresh credentials, then retry."`.
- [ ] `response.go` 404 returns `CLIError.Code == 3`, `ResourceType` populated from `parseResourceFromRequest`, `Suggestion` populated from `suggestionForResource(resourceType)` per REQ-F-003.
- [ ] `response.go` 429 returns `CLIError.Code == 7` with `Suggestion == "Retry after <retryAfter> seconds."`.
- [ ] All 5 `utils/resolve.go` `fmt.Errorf` sites return `*clierr.CLIError` of the correct code (`2` for the 3 workspace-resolution sites; `3` for the 4 not-found / ownership-mismatch sites) with `Suggestion` and (where applicable) `ResourceType`/`ResourceID` set per REQ-F-005..009.
- [ ] `WithNotFoundContext` helper exists in `utils/`, has a unit test for both the matching and non-matching error path.
- [ ] `instance/snapshot/get.go`, `graphanalytics/session/get.go`, `graphanalytics/session/delete.go` rewrite the misleading `ResourceType` and set `Suggestion` per REQ-F-012, REQ-F-013.
- [ ] Each touched call site has a unit test asserting `Code`, `Suggestion`, and (where set) `ResourceType` / `ResourceID` via `errors.As`.
- [ ] `--format=json` envelope for a 404 instance carries `"suggestion": "Run 'neo4j-cli aura instance list' to see available instances."` (verified via existing `render_test.go` machinery; no new tests required).
- [ ] `--format=default` (TTY) prints the suggestion on a second stderr line (already proven by `common/clierr/render_test.go:232-244`; no new tests required).
- [ ] Changelog entry under `.changes/unreleased/` (`Minor`, body: "Populate next-action suggestions on Aura API errors (401/403/404/400/429) and workspace/ownership errors.").
- [ ] `make fmt-check && make lint && make test && make generate-check` clean.

## Out of Scope

- Caller-side 404 enrichment under `subcommands/tenant/` (hidden surface). The `tenant` entry in `suggestionForResource` still triggers for raw 404s on `/v*/tenants/{id}` paths but no caller-side code changes are made.
- Snapshot `list` and `create` 404 enrichment (not on the audit hit-list).
- 5xx upstream errors (retryable; renderer already advises retry for `Retryable=true`).
- Changes to `common/clierr/render.go` or its tests.
- Changes to the agent-skill bundle or generated reference docs (no command-tree changes).

## Open Questions

(none — all locked in plan-mode Q&A)
