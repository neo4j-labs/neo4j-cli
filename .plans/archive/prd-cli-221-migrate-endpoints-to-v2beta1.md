# PRD: Migrate Aura CLI endpoints to v2beta1 where possible (CLI-221)

## Overview

The Aura v1 API will **not** be updated to support the device-authorization
grant ("seamless" login without a client-id/secret) being introduced in parent
issue CLI-202. To let as much CLI functionality as possible work under
seamless login, this feature migrates every Aura endpoint that has a v2beta1
equivalent from v1 (or the flat top-level v1 paths) to the org/project-scoped
v2beta1 API.

v2beta1 is fully nested under `/organizations/{org_id}/projects/{project_id}/…`.
The CLI already resolves org + project for every command via
`utils.ResolveAndValidateOrgProject` (default-workspace config,
`--organization-id`/`--project-id` flags), so the scoping context needed by
v2beta1 is already available — this migration reuses it rather than introducing
new UX.

This PRD covers **endpoint/version/path/response changes only**. The
device-authorization grant login flow itself is out of scope (owned by
CLI-202).

## Goals

- Move every Aura endpoint that has a v2beta1 equivalent onto v2beta1, so those
  commands function under seamless (device-auth) login.
- Reuse the existing org/project resolution (`ResolveAndValidateOrgProject`)
  for the newly-scoped endpoints — no new user-facing scoping mechanism.
- Preserve existing CLI output contracts as far as the new API allows:
  `status` stays `status` (normalise v2beta1 `legacy_status` → `status`);
  `tenant_id` becomes `project_id` in output.
- Remove now-redundant machinery where v2beta1's native scoping supersedes it
  (e.g. the tenant-ownership preflight checks in `utils/resolve.go`).
- Leave endpoints without a v2beta1 equivalent on their current version, clearly
  documented, so the split is intentional and discoverable.

## Non-Goals

- Implementing the device-authorization grant / seamless login flow (CLI-202).
- Migrating endpoints that have **no** v2beta1 equivalent:
  - instance `pause`, `resume`, `update` (PATCH), `overwrite`
  - `snapshot` create/list/get (v2beta1 models this as `databases`/`backups`,
    which is not a drop-in replacement)
  - `customer-managed-key` create/list/delete
  - `graphql` data-apis and sub-resources (auth-provider, cors-policy) — remain
    on v1beta5
- Redesigning the snapshot feature to use the v2beta1 databases/backups model.
- Changing the OAuth client-credentials token exchange in `token.go`.
- Adding list pagination. The CLI does not follow pages today (list commands
  render the single response the API returns); v2beta1 migration preserves that
  behaviour and does not introduce page-token following.
- Gating the migration behind a feature flag. The switch to v2beta1 is
  unconditional for the migrated endpoints.

## Requirements

### Functional Requirements

**Endpoints to migrate to v2beta1 (equivalent exists):**

- REQ-F-001: `aura instance list` → `GET /organizations/{orgID}/projects/{projectID}/instances`.
- REQ-F-002: `aura instance create` → `POST /organizations/{orgID}/projects/{projectID}/instances` (including the auto-naming pre-list lookup in `create_core.go`).
- REQ-F-003: `aura instance get` (and `utils.FetchAndVerifyInstanceInProject`) → `GET /organizations/{orgID}/projects/{projectID}/instances/{instanceID}`.
- REQ-F-004: `aura instance delete` → `DELETE /organizations/{orgID}/projects/{projectID}/instances/{instanceID}`.
- REQ-F-005: `aura graph-analytics session list` → `GET /organizations/{orgID}/projects/{projectID}/graph-analytics/sessions`.
- REQ-F-006: `aura graph-analytics session create` → `POST /organizations/{orgID}/projects/{projectID}/graph-analytics/sessions`.
- REQ-F-007: `aura graph-analytics session delete` (and `utils.FetchAndVerifySessionInProject`) → `.../graph-analytics/sessions/{sessionID}` (GET for verify, DELETE for delete).
- REQ-F-008: Polling helpers that target migrated resources (`PollInstance`, `PollGraphAnalyticsSessionReady`) must poll the corresponding v2beta1 org/project-scoped path/version.
- REQ-F-008a: `aura project get` must move off v1 `/tenants/{id}`. Since v2beta1 has no bare single-project GET, derive the single project from `GET /organizations/{orgID}/projects` (v2beta1) by filtering the list for the requested `projectID`, and return a not-found error (matching the existing message/exit-code) when it is absent. Reuse `api.ListProjects` rather than adding a second list path.

**Scoping:**

- REQ-F-009: Migrated commands must obtain org + project via the existing
  `utils.ResolveAndValidateOrgProject` (default-workspace → flags → error). No
  new scoping flags or config keys are introduced.
- REQ-F-010: Because v2beta1 scopes natively, the tenant-ownership preflight
  checks for migrated resources (`FetchAndVerify*` returning "could not find X
  in project Y" via `tenant_id` comparison) must be removed or replaced by the
  v2beta1 path's native 404 for that project.

**Output contract:**

- REQ-F-011: Output field `status` must be preserved. Where v2beta1 returns
  `legacy_status`, normalise it to `status` before output.
- REQ-F-012: The output field previously named `tenant_id` must be renamed to
  `project_id` (matching v2beta1). Update JSON/TOON keys, table headers, and
  `Print*` `fields` slices accordingly.
- REQ-F-013: Any other v2beta1 response-shape differences (e.g. lean
  `InstanceSummary` in list vs `InstanceDetails`, `connection_url`/`username`/
  `password` on create) must be mapped so existing output columns remain
  populated; document any column that has no v2beta1 source.

**Error handling under nested v2beta1 paths:**

- REQ-F-013a: `parseResourceFromRequest` (`response.go`) must correctly identify
  the resource type/id from the deep v2beta1 path shape
  `/v2beta1/organizations/{org}/projects/{proj}/instances/{id}`. Today it takes
  `segments[1]`/`segments[2]` (assuming `/<version>/<plural>/<id>`), which for a
  nested path yields `organization`/`{orgID}` — wrong. It must instead resolve
  the *trailing* plural/id pair so 404 envelopes carry the correct resource type
  and `suggestionForResource` hint. Preserve correct behaviour for the
  still-on-v1 flat paths.

**Endpoints explicitly NOT migrated (must keep working on current version):**

- REQ-F-014: instance `pause`, `resume`, `update`, `overwrite` remain on v1.
- REQ-F-015: `snapshot` create/list/get remain on v1.
- REQ-F-016: `customer-managed-key` create/list/delete remain on v1.
- REQ-F-017: `graphql` (data-apis, auth-provider, cors-policy) remain on v1beta5.
- (`project get` is migrated per REQ-F-008a, not left on v1.)

### Non-Functional Requirements

- REQ-NF-001: No regression in redaction/telemetry/tee behaviour — all migrated
  requests continue to route through `api.MakeRequest` and its debug/redaction
  path.
- REQ-NF-002: `aura --debug` traces must show the new v2beta1 paths and pass
  through `RedactText` + `StripControl` as before.
- REQ-NF-003: Final gates pass: `make test`, `make fmt-check`, `make lint`,
  `make license-check`.
- REQ-NF-004: `go generate ./neo4j-cli/internal/skill/...` re-run if any
  `Long`/`Example` text changes for migrated commands; committed bundle must not
  drift (`TestGenerator_RoundTrip`).
- REQ-NF-005: Output-name changes (`tenant_id`→`project_id`) must satisfy the
  casing gates (`common/output/casing_gate_test.go`) and update any
  `test/e2e/` decoders/golden files.
- REQ-NF-006: User-facing changelog entry via `changie` (Minor kind) describing
  the endpoint migration and the `tenant_id`→`project_id` output rename.

## Technical Considerations

- **Version plumbing**: `getVersionPath` already maps `AuraApiVersion2` →
  `v2beta1`. Migration is per-call: set `Version: api.AuraApiVersion2` and change
  the `path` string to the org/project-scoped form. Most migrated calls need the
  resolved `orgID`/`projectID` threaded into a `fmt.Sprintf` path.
- **Central helpers**: `utils/resolve.go` (`FetchAndVerifyInstanceInProject`,
  `FetchAndVerifySessionInProject`) currently do flat `/instances/{id}` +
  `tenant_id` comparison. These become scoped v2beta1 GETs; the ownership check
  collapses into the path.
- **Polling**: `poll.go` — `PollInstance` currently uses default v1; move to a
  scoped v2beta1 path via `PollWithVersion`. `PollGraphAnalyticsSessionReady`
  likewise. `PollSnapshot`/`PollCMK` stay on v1.
- **Response parsing** (`response.go`): confirm `ParseBody`/`GetSingleOrError`
  handle the v2beta1 envelope; `parseResourceFromRequest` regex currently keys
  off `/v1/…`/`/v1beta5/…` path shapes — extend it for the deeper v2beta1
  org/project paths so not-found resource typing still works.
- **Field mapping**: introduce a small normalisation step (`legacy_status` →
  `status`; ensure `project_id` present) at the output boundary for migrated
  resources. Keep it centralized rather than per-command.
- **Split instance surface**: after migration, instance `list/create/get/delete`
  are v2beta1 while `pause/resume/update/overwrite` stay v1. Both sets already
  resolve org/project, so the scoping story is consistent even though the API
  version differs; document this in code comments.
- **base-url backward compat**: `AuraConfig.BaseUrl()` already strips a legacy
  trailing `/v1`; version path is appended by `getVersionPath`, so no base-url
  change is needed.

## Acceptance Criteria

- [ ] `aura instance list/create/get/delete` issue requests to the v2beta1
      org/project-scoped instance paths.
- [ ] `aura graph-analytics session create/list/get/delete` issue requests to
      the v2beta1 org/project-scoped session paths.
- [ ] `aura project get` derives its result from the v2beta1 project-list
      endpoint (no v1 `/tenants/{id}` call) and preserves the existing not-found
      message and exit code.
- [ ] Migrated commands resolve org/project through
      `ResolveAndValidateOrgProject`; no new scoping flags/config added.
- [ ] Redundant `tenant_id`-based ownership preflight for migrated resources is
      removed.
- [ ] Polling for migrated resources targets v2beta1 paths.
- [ ] CLI output preserves `status` (normalising `legacy_status`) and renames
      `tenant_id` → `project_id`; all other columns remain populated or are
      documented as unavailable.
- [ ] instance `pause/resume/update/overwrite`, `snapshot`, `customer-managed-key`,
      and `graphql` commands are unchanged and still function on their existing
      versions.
- [ ] Debug traces, redaction, telemetry, and tee behaviour unchanged for
      migrated calls.
- [ ] Casing gates, e2e decoders, and golden files updated for the
      `tenant_id`→`project_id` rename.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check` pass;
      skill bundle regenerated with no drift.
- [ ] Changelog entry added via `changie`.

## Out of Scope

- Device-authorization grant / seamless login implementation (CLI-202).
- Migrating instance `pause`/`resume`/`update`/`overwrite`, `snapshot`,
  `customer-managed-key`, or `graphql` endpoints.
- Adopting the v2beta1 `databases`/`backups` model as a snapshot replacement.
- Changes to the OAuth token exchange.

## Implementation-time verifications (no product decision needed)

These are confirmations to make while implementing/testing, not open product
questions.

- **V-1**: Confirm the v2beta1 instance create body wraps as `{ "data": {...} }`
  and unwraps via `ParseBody`/`GetSingleOrError` (agent/org commands already run
  on v2beta1, so this is expected). The create response carries the same field
  names the create path reads (`connection_url`/`username`/`password`/
  `project_id`) per the `CreateInstanceResponse` schema — verify no surprises.
- **V-2**: v2beta1 returns `legacy_status` rather than `status`; ensure the
  `legacy_status`→`status` normalisation (REQ-F-011) is applied before any
  `--wait`/`PollInstance` reads it, since the create render itself does not
  print status.
- **V-3**: The instance/session 4xx responses are untyped (description-only) in
  the v2beta1 spec, and the one typed error schema (`BillingErrorResponse`) uses
  `errors[].{error,message}`. The CLI's `suggestionForPaymentRequired` reads
  `errors[].reason == "quota-exceeded"`, which an existing code comment asserts
  is the stable runtime v2beta1 value. Verify against a live v2beta1 402/404
  body that `extractEmbeddedErrors` and the `reason` handling still fire; adjust
  only if the runtime shape differs from that assertion.

## Open Questions

- (none — all resolved.)
