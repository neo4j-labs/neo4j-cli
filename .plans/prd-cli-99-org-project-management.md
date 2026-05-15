# PRD: Organization and Project Management Commands (CLI-99)

## Overview

The Aura CLI was designed against the v1 API, which uses a flat `tenant` model. The v2beta1 API introduces an explicit organization → project hierarchy and renames "tenants" to "projects". This PRD covers the CLI surface changes needed to align with that hierarchy: adding org/project discovery commands, replacing the existing `config project` subsystem with simpler `default-organization` / `default-project` config keys, and renaming all `tenant` terminology to `project`.

This work is a prerequisite for any future v2 command additions (blocked by CLI-120).

## Goals

- Expose organization listing via a new `aura organization list` command backed by v2beta1.
- Add `aura project list/get` commands backed by v2beta1 project endpoints.
- Deprecate (but preserve) `aura tenant list/get` with a stderr warning directing users to the new commands.
- Replace the `aura config project add/use/list/remove` subsystem with two flat config keys: `default-organization` and `default-project`.
- Validate `config set default-project` live against the API to confirm the project belongs to the configured org.
- Eliminate all "tenant" terminology from new command output, help text, and config keys.

## Non-Goals

- Exposing v2beta1 endpoints beyond organizations and projects (no instance management, IP filters, fleet manager, agents, billing, etc.).
- Adding `organization get <id>` (not required for this issue; list is sufficient).
- Supporting project creation, update, or deletion via the CLI.
- Migrating existing stored `config project` data from the old subsystem to the new keys.

## Requirements

### Functional Requirements

- REQ-F-001: Add `aura organization list` command that calls `GET /organizations` on the v2beta1 API and outputs the list of organizations in `--format json|table|toon`.
- REQ-F-002: Add `aura project list` command that calls `GET /organizations/{organizationId}/projects` on v2beta1. Requires either `--organization-id <id>` flag or `default-organization` config key to be set; fails with a clear error if neither is provided.
- REQ-F-003: Add `aura project get <id>` command that calls `GET /organizations/{organizationId}/projects/{projectId}` on v2beta1. Same org resolution rules as REQ-F-002.
- REQ-F-004: Deprecate (do not remove) `aura tenant list` and `aura tenant get`. Both commands must remain functional but: (a) be hidden from `--help` output and skill bundles, and (b) print a deprecation warning to stderr on every invocation (e.g., `"Warning: 'aura tenant list' is deprecated and will be removed in a future release. Use 'aura project list' instead."`). Removal is deferred to a later breaking-change release and noted in the changelog.
- REQ-F-005: Remove the `aura config project add/use/list/remove` subcommand tree entirely.
- REQ-F-006: Add `default-organization` as a valid `aura config set/get` key. Setting it stores the organization ID in config.
- REQ-F-007: Add `default-project` as a valid `aura config set/get` key. Setting it uses a two-stage validation: (1) **local check** — fail immediately with a clear error if `default-organization` is not set in config (no API call needed); (2) **remote check** — call `GET /organizations/{organizationId}/projects/{projectId}` on v2beta1 to verify the project exists in that org; fail if the API returns 404 or an error. The value is only persisted if both checks pass.
- REQ-F-008: Setting `default-organization` to a new value (different from current) automatically clears `default-project`.
- REQ-F-009: Remove the `default-tenant` config key. The `aura config set/get/list` commands must no longer accept or display it.
- REQ-F-010: Rename all "tenant" references in command output, table headers, JSON field names, and help text to "project".

### Non-Functional Requirements

- REQ-NF-001: All new commands must follow the existing one-file-per-leaf Cobra layout under `neo4j-cli/aura/internal/subcommands/organization/` and `neo4j-cli/aura/internal/subcommands/project/`.
- REQ-NF-002: All new commands must have `--format json|table|toon` support via `RegisterOutputFlag`.
- REQ-NF-003: All new leaf commands must have a flush-left `Example:` field with ≥3 invocations (including at least one `--format json`), enforced by `TestAllLeafCommands_HaveExamples`.
- REQ-NF-004: All new and modified commands must have colocated `*_test.go` files with table-driven tests.
- REQ-NF-005: Skill bundles must be regenerated (`go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`) after any command-tree or help-text change.
- REQ-NF-006: A changelog entry (`make changelog --kind Minor`) is required for this user-facing change. The entry must note that `aura tenant list/get` are deprecated and will be removed in a future release.

## Technical Considerations

### API version routing
The existing `api.MakeRequest` function supports `AuraApiVersion1` and `AuraApiVersion2`. The new organization and project commands must use `AuraApiVersion2` (v2beta1: `https://api.neo4j.io/v2beta1`). The auth mechanism for v2beta1 is OAuth 2.0 Client Credentials — verify whether the existing credential/token flow already handles this or needs extension.

### Organization resolution order
Both `project list` and `project get` need an organization ID. Resolution order:
1. `--organization-id` flag (explicit, highest priority)
2. `default-organization` config key (fallback)
3. Hard error: "no organization specified; set a default with `aura config set default-organization <id>` or pass `--organization-id`"

### Removing `config project` subsystem
The `config/project/` subcommand directory and its four leaf files (`add.go`, `use.go`, `list.go`, `remove.go`, `project.go`) should be deleted. The `aura-projects` config storage key and the `AuraProject` struct in `common/clicfg/projects/` should also be removed. Any references to them in `clicfg.go` or elsewhere must be cleaned up.

### Config key changes
- Remove `default-tenant` from `validAuraConfigKeys`.
- Add `default-organization` and `default-project` to `validAuraConfigKeys`.
- The `default-project` setter uses two-stage validation: first a local check (is `default-organization` set?), then a remote API call (`GET /organizations/{organizationId}/projects/{projectId}`). The value is only persisted if both pass.

### Backward compatibility
No migration of existing `config project` data is required. Users who had projects configured via the old subsystem will need to reconfigure using the new `config set` keys. This is acceptable given these commands were gated behind `AuraBetaEnabled()`.

### Deprecating `tenant` commands
Use Cobra's `cmd.Hidden = true` on the `tenant list` and `tenant get` leaf commands to suppress them from `--help` output and skill bundle generation. Add a `PersistentPreRunE` (or inline at the top of `RunE`) that writes the deprecation warning to `cmd.ErrOrStderr()`. The `tenant` parent command itself should also be hidden so it does not appear in `aura --help`. Do not add `Example:` fields to deprecated commands — `TestAllLeafCommands_HaveExamples` must be updated to skip hidden commands.

### Skill bundle regeneration
After deprecating/hiding the `tenant` command tree, removing `config project` subtree, and adding `organization` and `project` command trees, run:
```
go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...
```
`TestGenerator_RoundTrip` will catch any stale bundle state.

## Acceptance Criteria

- [ ] `neo4j-cli aura organization list` returns organization data from v2beta1 in all three output formats.
- [ ] `neo4j-cli aura project list` returns project data when `--organization-id` is set or `default-organization` is configured; fails with a clear error otherwise.
- [ ] `neo4j-cli aura project get <id>` returns project details using the same org resolution.
- [ ] `neo4j-cli aura tenant list` and `neo4j-cli aura tenant get` still function but are hidden from `--help` and print a deprecation warning to stderr.
- [ ] `neo4j-cli aura config project add/use/list/remove` no longer exist.
- [ ] `neo4j-cli aura config set default-organization <id>` stores the value and clears `default-project` if the org ID changes.
- [ ] `neo4j-cli aura config set default-project <id>` succeeds when the project exists in the configured org; fails with a clear error when `default-organization` is unset or the project is not found.
- [ ] `neo4j-cli aura config set default-tenant` is rejected as an unknown key.
- [ ] No "tenant" terminology appears in any non-deprecated command output, header, or help text.
- [ ] `make test`, `make fmt-check`, and `make lint` all pass.
- [ ] `TestGenerator_RoundTrip` passes (skill bundles up to date).
- [ ] `TestAllLeafCommands_HaveExamples` passes (all new leaves have examples).
- [ ] Changelog entry added.

## Out of Scope

- v2beta1 instance management commands (blocked by CLI-120 until important endpoints are available).
- `organization get <id>` command.
- Project creation, update, or deletion.
- User management at org or project level.
- IP filters, fleet manager, agents, billing.
- Automatic migration of existing `config project` data.
- Hard removal of `aura tenant list/get` (deferred to a future breaking-change release).

## Open Questions

- Does the existing OAuth token flow in the API client already support v2beta1's Client Credentials auth, or does it need to be wired up? (Needs investigation before implementation.)
- Should `project list` and `project get` also accept a `--project-id` flag for consistency, or is the positional argument sufficient for `get`?
