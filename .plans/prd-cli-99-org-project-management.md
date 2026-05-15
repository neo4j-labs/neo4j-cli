# PRD: Organization and Project Management Commands (CLI-99)

## Overview

The Aura CLI was designed against the v1 API, which uses a flat `tenant` model. The v2beta1 API introduces an explicit organization → project hierarchy and renames "tenants" to "projects". This PRD covers the CLI surface changes needed to align with that hierarchy: adding org/project discovery commands, a new `context` subcommand for setting and listing available org/project pairs, replacing the existing `config project` subsystem with a single `aura.default-context` config key, and deprecating all `tenant` terminology.

This work is a prerequisite for any future v2 command additions (blocked by CLI-120).

## Goals

- Expose organization listing and detail retrieval via `aura organization list` and `aura organization get` commands backed by v2beta1.
- Add `aura project list/get` commands backed by v2beta1 project endpoints, using `aura.default-context` for org resolution.
- Add an `aura context` subcommand group (`context list`, `context use`) to discover and set the active org/project pair as a single `{organizationId}/{projectId}` slug.
- Deprecate (but preserve) `aura tenant list/get` with a stderr warning directing users to the new commands.
- Replace the `aura config project add/use/list/remove` subsystem with the single `aura.default-context` config key written by `context use`.
- Eliminate all "tenant" terminology from new command output, help text, and config keys.

## Non-Goals

- Exposing v2beta1 endpoints beyond organizations and projects (no instance management, IP filters, fleet manager, agents, billing, etc.).
- Supporting project creation, update, or deletion via the CLI.
- Migrating existing stored `config project` data from the old subsystem to the new key.
- Hard removal of `aura tenant list/get` (deferred to a future breaking-change release).
- An interactive context selection mode for `context use`.

## Requirements

### Functional Requirements

- REQ-F-001: Add `aura organization list` command that calls `GET /organizations` on the v2beta1 API and outputs the list of organizations in `--format json|table|toon`. This command is a pure discovery command: it does not read or respect `aura.default-context`.

- REQ-F-002: Add `aura project list` command that calls `GET /organizations/{organizationId}/projects` on v2beta1. Requires either `--organization-id <id>` flag (highest priority) or the org portion of `aura.default-context` (split on `/`, take the left side) as a fallback; fails with a clear error if neither is available. Always lists all projects within the resolved org; does not filter by the project portion of `aura.default-context`.

- REQ-F-003: Add `aura project get <id>` command. **Temporary implementation**: because `GET /organizations/{organizationId}/projects/{projectId}` has not yet been added to v2beta1, the command must fall back to the v1 `GET /tenants/{projectId}` endpoint (`AuraApiVersion1`) and pass the response through as-is. The `--organization-id` flag is accepted but not used in the v1 call (org resolution is not required). This fallback must be replaced with the v2beta1 endpoint once it ships.

- REQ-F-004: Deprecate (do not remove) `aura tenant list` and `aura tenant get`. Both commands must remain functional but: (a) be hidden from `--help` output and skill bundles, and (b) print a deprecation warning to stderr on every invocation (e.g., `"Warning: 'aura tenant list' is deprecated and will be removed in a future release. Use 'aura project list' instead."`). Removal is deferred to a later breaking-change release and noted in the changelog.

- REQ-F-005: Remove the `aura config project add/use/list/remove` subcommand tree entirely. Delete the `config/project/` subcommand directory, the `aura-projects` config storage key, and the `AuraProject` struct in `common/clicfg/projects/`.

- REQ-F-010: Rename all "tenant" references in new command output, table headers, JSON field names, and help text to "project". Deprecated `tenant` commands are exempt.

- REQ-F-011: Add `aura context list` command. It fetches all organizations via `GET /organizations`, then for each org fetches all projects via `GET /organizations/{organizationId}/projects`, and returns a flat list of org/project pairs. Each entry in the output contains:
  - `context` — string, formatted as `{organizationId}/{projectId}`
  - `organizationId` — string
  - `projectId` — string
  - `projectName` — string
  - `default` — boolean, `true` if this entry's `context` value matches the current `aura.default-context` config value, `false` otherwise (including when `aura.default-context` is unset)
  Supports `--format json|table|toon`.

- REQ-F-012: Add `aura context use` command. Accepts a context via either:
  - Positional argument: `{organizationId}/{projectId}` slug (split on first `/`)
  - Flags: `--organization-id <id>` and `--project-id <id>`
  Both org and project must always be provided by one of these two forms; if either is missing, fail with a clear error. The two forms are mutually exclusive — if both are provided simultaneously, fail with a clear error. Validates the org/project pair by calling `GET /organizations/{organizationId}/projects` on v2beta1 and confirming `{projectId}` appears in the returned list; fails with a clear error if the project is not found or the API returns an error. On success, writes `aura.default-context` as `{organizationId}/{projectId}` to config. There is no interactive mode.

- REQ-F-013: `neo4j-cli config set aura.default-context <org-id>/<project-id>` must also be supported. It must apply the same validation as `context use` (same shared function: parse the slug, call `GET /organizations/{organizationId}/projects` on v2beta1 and confirm the project ID is in the returned list, fail if not found or on API error, persist only on success). The `aura.default-context` key must be added to the valid settable keys for `config set`.

- REQ-F-014: Update `cfg.Aura.DefaultTenant()` to use the following resolution order: (1) project portion of `aura.default-context` (right side of `/`) — primary source; (2) `default-tenant` config key — legacy fallback only. This ensures the following non-beta commands continue to work without requiring `--tenant-id`: `instance list`, `instance create`, `customermanagedkey list`, `customermanagedkey create`, `graphanalytics session list`, `graphanalytics session create`. The `--tenant-id` flag name on these commands is unchanged in this PR (renaming is out of scope).

- REQ-F-015: As part of this PR, create a Linear issue in the CLI working group (CLI) team to track the hard removal of the deprecated `aura tenant list` and `aura tenant get` commands. The issue should reference this PR and note that removal is blocked until a suitable deprecation window has passed.

- REQ-F-016: Add `aura organization get <id>` command that calls `GET /organizations/{organizationId}` on the v2beta1 API and outputs the organization details in `--format json|table|toon`. The command takes the organization ID as a positional argument.

- REQ-F-017: Update `aura project get <id>` to call v1 `GET /tenants/{projectId}` (`AuraApiVersion1`) instead of the v2beta1 single-project endpoint (which does not yet exist). The response is passed through as-is. The `--organization-id` flag is accepted but unused. This is a temporary workaround; see Technical Considerations.

- REQ-F-018: Update `validateAndSetDefaultContext` (shared by `context use` and `config set aura.default-context`) to validate the project by calling `GET /organizations/{organizationId}/projects` on v2beta1 and confirming the project ID is present in the returned list, rather than calling the non-existent single-project GET endpoint. Return a clear "project not found in organization" error when the ID is absent from the list. This is a temporary workaround; see Technical Considerations.

### Non-Functional Requirements

- REQ-NF-001: All new commands must follow the existing one-file-per-leaf Cobra layout: `aura context` under `neo4j-cli/aura/internal/subcommands/context/`, `aura organization` under `.../organization/`, `aura project` under `.../project/`.

- REQ-NF-002: All new commands must have `--format json|table|toon` support via `RegisterOutputFlag` (read commands only; `context use` has no output format).

- REQ-NF-003: All new leaf commands must have a flush-left `Example:` field with ≥3 invocations (including at least one `--format json` on read commands), enforced by `TestAllLeafCommands_HaveExamples`. Hidden/deprecated commands are exempt from this test.

- REQ-NF-004: All new and modified commands must have colocated `*_test.go` files with table-driven tests.

- REQ-NF-005: Skill bundles must be regenerated (`go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`) after any command-tree or help-text change.

- REQ-NF-006: A changelog entry (`make changelog --kind Minor`) is required for this user-facing change. The entry must note that `aura tenant list/get` are deprecated and will be removed in a future release.

## Technical Considerations

### API version routing
The existing `api.MakeRequest` function supports `AuraApiVersion1` and `AuraApiVersion2`. All new organization, project, and context commands must use `AuraApiVersion2` (v2beta1: `https://api.neo4j.io/v2beta1`). The existing credential/token flow already works for both v1 and v2 — no auth changes are needed.

### Temporary v1 fallback for `project get`
The `GET /organizations/{organizationId}/projects/{projectId}` endpoint has not yet been added to v2beta1. Until it ships, `aura project get <id>` must call the v1 `GET /tenants/{projectId}` endpoint (`AuraApiVersion1`) and pass the response through as-is. The `--organization-id` flag should still be accepted by the command (for forward compatibility) but is not used in the actual API call. Once the v2beta1 single-project endpoint is available, REQ-F-003 and task-014 must be revisited to switch over.

### List-based validation in `validateAndSetDefaultContext`
Because `GET /organizations/{organizationId}/projects/{projectId}` is not yet available on v2beta1, the `validateAndSetDefaultContext` helper must validate the org/project pair by calling `GET /organizations/{organizationId}/projects` (the list endpoint) and checking whether the project ID appears in the returned list. If the list call fails or the project ID is not present, return a clear error and do not persist. This is a temporary workaround; once the single-project endpoint ships, the validation should switch to a direct GET.

### Organization resolution order
Both `project list`, `project get`, and `context use` need an organization ID. Resolution order:
1. `--organization-id` flag (explicit, highest priority)
2. Org portion of `aura.default-context` config key (split on `/`, take the left side)
3. Hard error: "no organization specified; set a context with `aura context use <org-id>/<project-id>` or pass `--organization-id`"

### `aura.default-context` config key
A single config key `aura.default-context` stores the active org/project pair in the format `{organizationId}/{projectId}`. It is read by `context list`, `project list`, `project get`, and any future commands that need org or project resolution. It replaces the old `default-organization`, `default-project`, and `default-tenant` keys. Add it to `validAuraConfigKeys` (both get and set); remove `default-tenant` from that list.

It is writable via two surfaces that share a single validation function:
1. `aura context use` (positional or flag form — see REQ-F-012)
2. `neo4j-cli config set aura.default-context <slug>` (see REQ-F-013)

Both surfaces parse the `{organizationId}/{projectId}` slug, call `GET /organizations/{organizationId}/projects/{projectId}` on v2beta1 to verify the pair, and persist the value only on success. Extract this logic into a shared helper (e.g. `validateAndSetDefaultContext(cfg, apiClient, slug string) error`) so neither surface duplicates the validation.

### `DefaultTenant()` resolution order for v1 commands
Several non-beta v1 commands (`instance list/create`, `customermanagedkey list/create`, `graphanalytics session list/create`) call `cfg.Aura.DefaultTenant()` when `--tenant-id` is not supplied. In the v1 API, "tenant ID" is equivalent to "project ID" in v2 terminology. Update `DefaultTenant()` to use this resolution order:
1. Project portion of `aura.default-context` (split on `/`, take the right side) — primary source
2. `default-tenant` config key — legacy fallback

This means users who set `aura context use` will immediately get the right default for v1 commands too. Users who haven't migrated yet still work via the legacy `default-tenant` key. The `--tenant-id` flag name on these commands is not renamed in this PR.

### `context list` fan-out
`context list` makes N+1 API calls (1 for orgs, 1 per org for its projects). For users with many orgs this may be slow, but no pagination or parallelism optimization is required for this issue.

### Deprecating `tenant` commands
Use Cobra's `cmd.Hidden = true` on the `tenant list` and `tenant get` leaf commands to suppress them from `--help` output and skill bundle generation. Add a `PersistentPreRunE` (or inline at the top of `RunE`) that writes the deprecation warning to `cmd.ErrOrStderr()`. The `tenant` parent command itself should also be hidden so it does not appear in `aura --help`. `TestAllLeafCommands_HaveExamples` must be updated to skip hidden commands.

### Removing `config project` subsystem
The `config/project/` subcommand directory and its leaf files (`add.go`, `use.go`, `list.go`, `remove.go`, `project.go`) should be deleted. The `aura-projects` config storage key and the `AuraProject` struct in `common/clicfg/projects/` should also be removed. Any references to them in `clicfg.go` or elsewhere must be cleaned up.

### Skill bundle regeneration
After deprecating/hiding the `tenant` command tree, removing the `config project` subtree, and adding the `organization`, `project`, and `context` command trees, run:
```
go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...
```
`TestGenerator_RoundTrip` will catch any stale bundle state.

## Acceptance Criteria

- [ ] `neo4j-cli aura organization list` returns organization data from v2beta1 in all three output formats. It does not read or use `aura.default-context`.
- [ ] `neo4j-cli aura organization get <id>` returns organization details from v2beta1 in all three output formats.
- [ ] `neo4j-cli aura project list` returns project data for the resolved org; fails with a clear error if neither `--organization-id` nor the org portion of `aura.default-context` is available. It does not filter by the project portion of `aura.default-context`.
- [ ] `neo4j-cli aura project get <id>` returns project details via v1 `GET /tenants/{id}`, passing the response through as-is.
- [ ] `neo4j-cli aura context list` returns a flat list of all org/project pairs across all orgs; each entry has `context`, `organizationId`, `projectId`, `projectName`, and `default` fields. Exactly one entry has `default: true` when `aura.default-context` is set and matches that entry; all entries have `default: false` when `aura.default-context` is unset.
- [ ] `neo4j-cli aura context use <org-id>/<project-id>` validates the pair by listing v2beta1 projects for the org and confirming the project ID is present, then writes `aura.default-context`.
- [ ] `neo4j-cli aura context use --organization-id <id> --project-id <id>` validates and writes `aura.default-context` identically to the positional form.
- [ ] `neo4j-cli aura context use` fails with a clear error when org or project is missing, or when both positional and flag forms are mixed.
- [ ] `neo4j-cli aura tenant list` and `neo4j-cli aura tenant get` still function but are hidden from `--help` and print a deprecation warning to stderr.
- [ ] `neo4j-cli aura config project add/use/list/remove` no longer exist.
- [ ] `aura.default-context` is readable via `aura config get default-context` and `aura config list`.
- [ ] `neo4j-cli config set aura.default-context <org-id>/<project-id>` validates the pair via the same shared function as `context use` and writes the key on success; fails with a clear error on invalid slug, 404, or API error.
- [ ] `instance list/create`, `customermanagedkey list/create`, and `graphanalytics session list/create` continue to resolve a default tenant from `aura.default-context` (project portion) when `--tenant-id` is omitted and `default-tenant` is unset.
- [ ] No "tenant" terminology appears in any non-deprecated command output, header, or help text.
- [ ] `make test`, `make fmt-check`, and `make lint` all pass.
- [ ] `TestGenerator_RoundTrip` passes (skill bundles up to date).
- [ ] `TestAllLeafCommands_HaveExamples` passes (all new leaves have examples; hidden commands are exempt).
- [ ] Changelog entry added, noting `aura tenant list/get` deprecation.
- [ ] Linear issue created in the CLI team to track hard removal of `aura tenant list/get`, referencing this PR.

## Out of Scope

- v2beta1 instance management commands (blocked by CLI-120 until important endpoints are available).
- `organization get <id>` command.
- Project creation, update, or deletion.
- User management at org or project level.
- IP filters, fleet manager, agents, billing.
- Automatic migration of existing `config project` data.
- Hard removal of `aura tenant list/get` (deferred to a future breaking-change release).
- Interactive context selection mode for `context use`.
- Pagination or parallelism in `context list` fan-out.
- Renaming `--tenant-id` flags on `instance`, `customermanagedkey`, and `graphanalytics` commands (deferred to a future PR).

## Open Questions

None.
