# PRD: Apply Org/Project Filters to All Aura Commands (CLI-120)

## Overview

CLI-99 established the org/project hierarchy: `aura.default-workspace`, the `organization` and `project` command groups, and the `workspace use` workflow. CLI-120 applies that hierarchy to every existing v1 API command. Every command that calls the v1 API — instance, customer-managed-key, and graph-analytics session — must now resolve and validate an org/project pair before executing. This enforces the security and scoping guarantees of the v2 hierarchy while keeping v1 as the wire protocol for these resource types.

## Goals

- Require a resolved organization and project for every v1 Aura command, sourced from explicit flags or `aura.default-workspace`.
- Validate at runtime that the project belongs to the organization via the v2beta1 list-projects endpoint.
- For list commands, filter results to the resolved project's tenant ID via the existing `tenantId` query parameter.
- For get/update/delete/pause/resume commands, verify the target resource belongs to the resolved tenant before any mutating API call.
- For create commands, validate the org/project pair and inject the resolved project ID as the tenant ID.
- Replace `--tenant-id` with `--project-id` across all affected commands, with a Cobra deprecation warning on `--tenant-id`.
- Rename the `tenant_id` field to `project_id` in all v1 API command output (all formats: json, table, toon).

## Non-Goals

- Moving any instance/cmek/graphanalytics commands from v1 to v2beta1.
- Hard removal of `--tenant-id` (deferred to a future breaking-change release).
- Adding org/project enforcement to `aura deployment`, `aura dataapi`, or `aura import` commands (not covered by the issue).
- Interactive org/project discovery.
- Pagination or parallelism in the pre-flight project list call.

## Requirements

### Functional Requirements

**Org/project resolution (shared across all affected commands)**

- REQ-F-001: Add `--organization-id <id>` as a persistent flag on the `instance`, `customermanagedkey`, and `graphanalytics session` parent command groups (i.e. registered in `NewCmd`, propagated to all leaves). Resolution order: (1) `--organization-id` flag; (2) org portion of `aura.default-workspace` (left of `/`); (3) fail with a clear error: `"no organization specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--organization-id'"`.

- REQ-F-002: Add `--project-id <id>` as a persistent flag on the same parent command groups. Resolution order: (1) `--project-id` flag; (2) `--tenant-id` flag (deprecated — Cobra's `MarkDeprecated` prints a standard warning to stderr automatically and keeps the flag functional); (3) project portion of `aura.default-workspace` (right of `/`); (4) if `default-tenant` is set but `aura.default-workspace` is not, fail with a migration message: `"No default workspace set. Run 'aura workspace use <org-id>/<project-id>' to migrate from the legacy default-tenant setting."`; (5) fail with a clear error: `"no project specified; set a default workspace with 'aura workspace use <org-id>/<project-id>' or pass '--project-id'"`.

- REQ-F-003: Extract a shared `ResolveAndValidateOrgProject(cmd *cobra.Command, cfg *clicfg.Config) (orgID, projectID string, err error)` helper, placed in `neo4j-cli/aura/internal/subcommands/utils/`. The function: (1) applies the resolution order from REQ-F-001 and REQ-F-002; (2) calls `GET /organizations/{orgID}/projects` (v2beta1) to list projects; (3) confirms the resolved projectID appears in the returned list; (4) returns a clear `"could not find project {projectID} in organization {organizationID}"` error if absent. All affected commands call this helper at the top of `RunE`, before any other API call. Reuse `api.ListProjects` already used by `ValidateAndSetDefaultWorkspace` in `workspace/validate.go`.

- REQ-F-004: All pre-flight calls (the v2beta1 project list, and any resource-ownership GET) are silent — no output to stdout or stderr unless an error occurs.

**Instance commands**

- REQ-F-010: `aura instance list` calls `ResolveAndValidateOrgProject`, then passes the resolved `projectID` as the `tenantId` query parameter on the v1 `GET /instances` request. Results are scoped to that tenant.

- REQ-F-011: `aura instance get <id>` calls `ResolveAndValidateOrgProject`, then fetches the instance via v1 `GET /instances/{id}`. Verifies the `tenant_id` field in the response matches the resolved `projectID`; fails with `"could not find instance {id} in project {projectID}"` if not.

- REQ-F-012: `aura instance create` calls `ResolveAndValidateOrgProject`, then passes the resolved `projectID` as `tenantId` in the v1 `POST /instances` body. No post-create ownership check is needed.

- REQ-F-013: `aura instance delete <id>` calls `ResolveAndValidateOrgProject`, fetches the instance via v1 `GET /instances/{id}` to verify `tenant_id` matches `projectID` (failing with a clear ownership error if not), then issues the v1 delete call.

- REQ-F-014: `aura instance update <id>`, `aura instance pause <id>`, `aura instance resume <id>` each call `ResolveAndValidateOrgProject`, fetch the instance to verify tenant ownership, then perform the respective v1 operation.

- REQ-F-015: `aura instance snapshot list <instance-id>`, `aura instance snapshot create <instance-id>`, `aura instance snapshot restore <instance-id> <snapshot-id>` (and any other snapshot leaves) call `ResolveAndValidateOrgProject`, verify the parent instance's `tenant_id` matches `projectID` via a pre-flight `GET /instances/{id}`, then perform the snapshot operation.

**Customer-managed key commands**

- REQ-F-020: `aura customermanagedkey list` calls `ResolveAndValidateOrgProject`, then passes the resolved `projectID` as the `tenantId` query parameter on the v1 list request.

- REQ-F-021: `aura customermanagedkey create` calls `ResolveAndValidateOrgProject`, then passes the resolved `projectID` as `tenantId` in the v1 create request body.

- REQ-F-022: Any `customermanagedkey` command that targets a specific existing resource (e.g. delete) calls `ResolveAndValidateOrgProject`, fetches the resource to verify its `tenant_id` matches `projectID` before any mutating call.

**Graph Analytics session commands**

- REQ-F-030: `aura graphanalytics session list` calls `ResolveAndValidateOrgProject`, then passes the resolved `projectID` as the `tenantId` query parameter.

- REQ-F-031: `aura graphanalytics session create` calls `ResolveAndValidateOrgProject`, then passes the resolved `projectID` as `tenantId` in the v1 create request.

- REQ-F-032: Any other graph-analytics session command that targets a specific existing resource calls `ResolveAndValidateOrgProject`, verifies the resource's `tenant_id` matches `projectID`, then executes.

**Deprecation**

- REQ-F-040: `--tenant-id` is removed from each affected leaf command and replaced with `--project-id` on the parent group. To maintain backwards compatibility during the deprecation window, `--tenant-id` is re-registered on the parent as a deprecated persistent flag via `cmd.PersistentFlags().MarkDeprecated("tenant-id", "use --project-id instead")`. Cobra automatically prints a deprecation notice to stderr when it is used and keeps it functional. Hard removal is deferred to a future breaking-change release.

**Error classification**

- REQ-F-050: All pre-flight validation errors (missing org, missing project, project not in org, resource not in tenant) are returned as Go errors and rendered by the Cobra error handler. They do not proceed to any mutating or data-returning API call.

**Field renaming**

- REQ-F-060: Every v1 API response that contains a `tenant_id` field must have that key renamed to `project_id` before the response is passed to the output layer. This transformation applies to all affected commands (instance, customermanagedkey, graphanalytics session) and all output formats (json, table, toon). The renaming is a CLI-side post-processing step; the v1 wire format is unchanged.

- REQ-F-061: The field name passed to `output.PrintBody` in the field list (`[]string{...}`) must use `"project_id"` wherever `"tenant_id"` previously appeared, so that table column headers, JSON keys, and toon output all consistently use the new name.

### Non-Functional Requirements

- REQ-NF-001: `ResolveAndValidateOrgProject` must have unit tests covering: org from flag, org from workspace config, missing org error, project from flag, project from deprecated `--tenant-id`, project from workspace config, `default-tenant`-only migration error, missing project error, project-not-in-org error.

- REQ-NF-002: Each affected command's `*_test.go` must include table-driven test cases for: happy path (flags), happy path (context), missing org, missing project, project not in org, and (for get/delete/update) resource not in tenant.

- REQ-NF-003: All pre-flight API calls produce no output.

- REQ-NF-004: Skill bundles must be regenerated after flag changes: `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`.

- REQ-NF-005: `make test`, `make fmt-check`, and `make lint` must all pass.

- REQ-NF-006: A changelog entry (`--kind Minor`) is required, noting that org/project are now required for all instance, cmek, and graphanalytics session commands, and that `--tenant-id` is deprecated in favour of `--project-id`.

- REQ-NF-007: The flag registration logic for `--organization-id`, `--project-id`, and the deprecated `--tenant-id` must be extracted into a `RegisterOrgProjectFlags(cmd *cobra.Command)` helper in `neo4j-cli/aura/internal/flags/`, following the same pattern as `RegisterWait` in `wait.go`. Each affected parent command calls this helper in `NewCmd` rather than registering flags inline. This keeps flag definitions in one place and makes the deprecation machinery consistent.

## Technical Considerations

### Flag registration via `RegisterOrgProjectFlags`
A `RegisterOrgProjectFlags(cmd *cobra.Command)` helper in `neo4j-cli/aura/internal/flags/` registers `--organization-id`, `--project-id`, and the deprecated `--tenant-id` as persistent flags (via `cmd.PersistentFlags()`), following the same pattern as `RegisterWait` in `wait.go`. Each of the three affected parent commands (`instance`, `customermanagedkey`, `graphanalytics session`) calls this helper once in `NewCmd`. Persistent flags propagate automatically to all child commands, including nested groups like `instance snapshot`, so no leaf-level registration is needed. `common/flags` is not the right home — it contains cross-CLI generic infrastructure, whereas these flags are Aura-specific.

### `ResolveAndValidateOrgProject` placement and signature
The helper lives in `neo4j-cli/aura/internal/subcommands/utils/` alongside existing shared helpers. It takes `*cobra.Command` (to read flags and detect `Changed()`), `*clicfg.Config` (to read `aura.default-workspace` and `default-tenant`), and makes API calls directly (matching the existing pattern used by `ValidateAndSetDefaultWorkspace` in `workspace/validate.go`). Return `(orgID, projectID string, err error)`.

### Detecting the `default-tenant`-only migration case
After failing to resolve the org from `--organization-id` and `aura.default-workspace`, check whether `cfg.Aura.Get("default-tenant")` is non-empty. If it is, return the migration message (REQ-F-002 step 4) rather than the generic missing-org error. This surfaces the migration path specifically for users who previously relied on `default-tenant`.

### Ownership check for get/delete/update/pause/resume
The pre-flight `GET /instances/{id}` (or equivalent) needed for ownership verification in REQ-F-011, REQ-F-013, REQ-F-014, REQ-F-015, REQ-F-022, and REQ-F-032 adds one extra round-trip. This is acceptable since it is a read-only call and protects against accidental cross-tenant mutations. The response body is discarded after the `tenant_id` check; the subsequent command issues its own API call as normal.

### `instance list` — removing the old optional `--tenant-id`
Currently `instance list` has an optional `--tenant-id` leaf flag for filtering. With CLI-120, this flag is superseded by the persistent `--project-id` (which is now required rather than optional). Remove the leaf-level `--tenant-id` from `instance list`; the persistent deprecated `--tenant-id` on the parent covers backwards compatibility.

### Reuse of existing v2beta1 helpers
`workspace/validate.go` already contains `ValidateAndSetDefaultWorkspace`, which calls `api.ListProjects`. `ResolveAndValidateOrgProject` should call `api.ListProjects` directly (the same underlying function) rather than duplicating the HTTP call logic. If `api.ListProjects` is not already exported as a standalone function, extract it.

### Snapshot sub-group nesting
`aura instance snapshot *` is a nested command group. Because `--organization-id` and `--project-id` are persistent on the `instance` parent, they propagate through the `snapshot` parent to each snapshot leaf automatically. No extra flag registration is needed in the snapshot group.

### `tenant_id` → `project_id` field renaming
The v1 API returns `tenant_id` in response bodies. Since the CLI renders output from a `map[string]any` parsed from the raw response bytes, the rename can be applied as a map mutation immediately after parsing: delete the `tenant_id` key and insert a `project_id` key with the same value. A small shared helper `RenameResponseField(data map[string]any, from, to string)` (or equivalent for list responses which are `[]map[string]any`) centralises this logic and is called in each affected command before `output.PrintBody`. The field list passed to `output.PrintBody` must use `"project_id"` rather than `"tenant_id"` so the column header and JSON key both reflect the new name.

## Acceptance Criteria

- [ ] `--organization-id` and `--project-id` flags are present (via inheritance) on all `aura instance`, `aura customermanagedkey`, and `aura graphanalytics session` leaf commands.
- [ ] `--tenant-id` is deprecated with a Cobra warning; `--project-id` is the replacement; both are functionally equivalent during the deprecation window.
- [ ] Running any affected command without org+project (and no `aura.default-workspace`) fails with a clear error message before any API call.
- [ ] Running any affected command with only `default-tenant` set (no `aura.default-workspace`) fails with the migration message directing the user to `aura workspace use`.
- [ ] Running any affected command with a project not in the given org fails with "project not found in organization" before any mutating API call.
- [ ] `aura instance list` results are scoped to the resolved tenant ID.
- [ ] `aura instance get/delete/update/pause/resume <id>` fails with a clear ownership error if the instance's `tenant_id` does not match the resolved project, before any mutating call.
- [ ] `aura instance create` passes the resolved `projectID` as `tenantId` in the request body.
- [ ] `aura instance snapshot *` commands verify the parent instance belongs to the resolved tenant before executing.
- [ ] Same org/project enforcement and ownership-validation behaviour applies to `customermanagedkey` and `graphanalytics session` commands.
- [ ] All pre-flight calls are silent (no stdout/stderr output unless an error occurs).
- [ ] All affected commands output `project_id` instead of `tenant_id` in json, table, and toon formats.
- [ ] `make test`, `make fmt-check`, `make lint` all pass.
- [ ] `TestGenerator_RoundTrip` passes (skill bundles up to date).
- [ ] `TestAllLeafCommands_HaveExamples` passes.
- [ ] Changelog entry added.

## Out of Scope

- Moving instance/cmek/graphanalytics commands from v1 to v2beta1.
- Hard removal of `--tenant-id` (deferred to a future breaking-change release).
- Renaming any other v1 response fields beyond `tenant_id` → `project_id`.
- Adding org/project enforcement to `aura deployment`, `aura dataapi`, or `aura import`.
- Interactive org/project discovery.
- Pagination of the pre-flight project list call.

## Open Questions

None.
