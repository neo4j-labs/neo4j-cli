# PRD: Route progress/status text to stderr (CLI-82)

## Overview

Multiple commands write plain-text status, success, and banner narration via `cmd.Println` / `cmd.Printf`, which cobra routes to `cmd.OutOrStdout()`. This breaks the agent contract `neo4j-cli ... --format json 2>/dev/null | jq .` — stdout is meant to carry only structured data; narration belongs on stderr.

Linear: https://linear.app/neo4j/issue/CLI-82/b1-progress-and-status-text-emitted-to-stdout-instead-of-stderr

Categories of offender (per `agent-cli-audit-2026-05-011.md` §3.1, §3.8):

1. **`--await` polling narration** — 13 commands print `"Waiting for ... to be ready..."` and `"<Resource> Status: <status>"` on stdout.
2. **Delete success confirmations** — 3 commands print `"Operation Successful"` / `"<Resource> deleted successfully ..."` on stdout, with no JSON body.
3. **Decorative banners / plain-text before JSON body** — 3+ commands print `###` banners or `"New allowed origins: ..."` on stdout before the actual data.
4. **One-off success confirmation** — `config project use` prints `"Set X as default project ..."` on stdout (no newline).
5. **`tenant get` table-format warning** — when `--format table|default`, `tenant get` prints a warning explaining instance_configurations is hidden, on stdout (CLI-95).

Existing patterns already used elsewhere in the repo:

- `output.PrintBodyMap(cmd, cfg, api.NewSingleValueResponseData(map[string]any{...}), fields)` — emits structured output respecting `--format json|table|toon`. Already used in `instance/create.go:231`. Lives at `neo4j-cli/aura/internal/output/output.go:16` (shim) → `common/output/`.
- `fmt.Fprintln/Fprintf(cmd.ErrOrStderr(), ...)` — the canonical stderr write throughout the codebase.

Test harness: `neo4j-cli/aura/internal/test/testutils/auratesthelper.go` exposes `AssertOut`, `AssertErr` (full match), `AssertOutContainsStrings`, `AssertOutJson`, but no `AssertErrContainsStrings`. Adding the symmetric helper keeps the stderr-substring assertions readable.

## Goals

- Every `--format json 2>/dev/null` invocation produces stdout that is valid JSON only (no narration prefix/suffix).
- `--await` polling narration is preserved but emitted to stderr, so interactive users still see progress.
- Delete commands return a structured JSON confirmation on stdout (`{"data": {"deleted": true, "id": "<id>"}}`) so consumers can pipe to `jq` and so `--format` is respected.
- Decorative banners (e.g. the GraphQL "store your API key" warning) keep their existing wording but move to stderr.
- Tests pin both behaviors: data on stdout, narration on stderr.

## Non-Goals

- F9 / CLI-94 — `query :schema` markdown headers on stdout. Separate Linear issue, separate PR.
- ~~F10 / CLI-95 — `tenant get` warning on stdout.~~ Pulled into this PRD as Pattern E (see REQ-F-011).
- F11 / CLI-96 — `update` command status lines on stdout. Separate Linear issue.
- F12 / CLI-97 — error-message structure. Separate Linear issue.
- Renaming `--await` → `--wait` (CLI-87 / F2), `--format json` → `--json` (CLI-86 / F1), or any verb canonicalisation (CLI-88 / F3, CLI-89 / F4). Out of scope.
- Adding `--quiet` / `--verbose` flags. Stderr narration is always-on; if users want it suppressed they redirect `2>/dev/null` (already the documented agent pattern).
- Changing the wording of any preserved narration line — only the destination changes.
- Reworking the `--await` poll loop itself (cadence, timeout, transitions). Same source data, different output stream.
- Bundle regen for unaffected commands. Output-stream changes do not affect cobra `Long` / `Use` / `Example` / flag descriptions, so no bundle drift is expected. `make generate-check` guards against surprise drift.

## Requirements

### Functional Requirements

- REQ-F-001: For every `--await`-supporting command (13 files), the polling narration lines (`"Waiting for ... to be ready..."`, `"<Resource> Status: <status>"`) are written to `cmd.ErrOrStderr()`. Stdout for `--format json` contains exactly the JSON body returned by `output.PrintBodyMap`, with no trailing narration.
- REQ-F-002: `customer-managed-key delete <id>`, `deployment delete <id>`, and `deployment token delete --deployment-id <id>` emit a structured confirmation on stdout via `output.PrintBodyMap`. The identifier field key matches each resource's existing list/get/create output convention (see "Delete-confirmation field keys" below): `"id"` for cmk and deployment, `"deployment_id"` for deployment-token. Stdout for `--format json` is `{"data":{"deleted":true,"<key>":"<value>"}}`. A short human-readable line (e.g. `customer-managed-key <id> deleted`) is written to stderr.
- REQ-F-003: The GraphQL `create` "store your API key" banner (3 lines in `dataapi/graphql/create.go:86-88` and `dataapi/graphql/authprovider/create.go:87-89`) is emitted to stderr. The JSON body emitted by `output.PrintBody` on stdout is unaffected.
- REQ-F-004: `corspolicy/allowedorigin/add.go:77` and `corspolicy/allowedorigin/remove.go:92,94` plain-text origin echoes are emitted to stderr. Stdout (when `--format json` is set) contains only the API response body via existing `output.PrintBody`.
- REQ-F-005: `aura config project use <name>` writes its `"Set X as default project ..."` confirmation to stderr, terminated with `\n`. (The missing `\n` in the current `cmd.Printf` is fixed incidentally — same line spec, just on stderr.)
- REQ-F-006: `instance overwrite --await` polling narration is routed to stderr identically to the other 12 await commands. (Audit-missed offender — added in scope per user confirmation.)
- REQ-F-007: `neo4j-cli/aura/internal/test/testutils/auratesthelper.go` gains an `AssertErrContainsStrings(expected []string)` method mirroring `AssertOutContainsStrings` (L146), with the same semantics: read `helper.err`, assert each substring is present via `assert.Contains`.
- REQ-F-008: All colocated `*_test.go` files for the modified source files are updated:
  - `AssertOut(...)` calls drop the moved-to-stderr lines from their expected strings.
  - New `AssertErrContainsStrings([]string{...})` assertions cover the stderr narration.
  - Delete-command tests switch from `AssertOut("Operation Successful\n")` (and equivalents) to `AssertOutJson(\`{"data":{"deleted":true,"id":"<id>"}}\`)` plus stderr assertions for the human line.
- REQ-F-009: Regression-pinning tests — for every affected command that supports `--format json`, add a test that runs the command with `--format json`, captures stdout, and asserts `json.Unmarshal(stdout, &v)` succeeds (where `v` is `map[string]any` or the response struct). These tests MUST fail on the pre-fix codebase (because stdout contained narration mixed with the JSON body) and pass after the fix. Implementation: a small helper `AssertOutIsValidJSON()` on `AuraTestHelper` that reads `helper.out` and calls `json.Unmarshal`, asserting `err == nil` and quoting the offending stdout in the failure message. Apply to at minimum one test per offender category: one await command (e.g. `instance create --await`), one delete command (e.g. `customer-managed-key delete`), one banner command (e.g. `dataapi graphql create`), one origin-echo command (e.g. `corspolicy allowed-origin add`).
- REQ-F-010: A `Patch`-kind changelog entry is recorded via `changie new --projects neo4j-cli --kind Patch --body "fix(cli): route progress/status text to stderr so --format json output stays jq-parseable (CLI-82, CLI-95)"`.
- REQ-F-011 (CLI-95): `tenant get` writes its table-format warning (`"instance configurations are not visible with table output - please use a different output setting using --format if you would like to view these"`) to stderr via `fmt.Fprintln(cmd.ErrOrStderr(), ...)` instead of `cmd.Println(...)`. The conditional gate (`cfg.Global.Format() == "table" || cfg.Global.Format() == "default"`) is preserved — the warning is irrelevant under `--format json|toon` where the field is included. Wording is preserved verbatim per the no-rewording convention used for every other case in this PRD. Source: `neo4j-cli/aura/internal/subcommands/tenant/get.go:42` (audit cited L44 — line drifted slightly). The colocated test at `tenant/get_test.go` currently asserts the warning via `AssertOut` (L145+) and must be updated to assert it via `AssertErrContainsStrings` plus `AssertOutJson` for the actual data body.

### Non-Functional Requirements

- REQ-NF-001: No external dependency changes.
- REQ-NF-002: Cross-OS tests stay green — no OS-specific code paths introduced. Existing harnesses (`AuraTestHelper`, mock HTTP server, afero `testfs`) are reused as-is.
- REQ-NF-003: `make fmt-check`, `make lint`, `make test`, `make generate-check`, `make license-check` all pass.
- REQ-NF-004: No bundle regen expected. `make generate-check` runs `go generate ./...` then `git diff --exit-code`; on a clean tree it must stay clean after this change. If it drifts, regen and commit in the same PR.
- REQ-NF-005: The fix uses `cmd.ErrOrStderr()` (not `os.Stderr` directly) so cobra's `SetErr` test seam continues to work — tests can capture stderr via `helper.err`.
- REQ-NF-006: No public-API changes in `common/output/`, `common/clicfg/`, or `neo4j-cli/aura/internal/output/`. All edits are at call sites + the new test helper method.

## Technical Considerations

### Files touched — source

**Pattern A: `--await` polling text → stderr** (13 files)

Audit-listed (12):
- `neo4j-cli/aura/internal/subcommands/instance/create.go` — L234, L242.
- `neo4j-cli/aura/internal/subcommands/instance/resume.go` — L52, L63.
- `neo4j-cli/aura/internal/subcommands/instance/snapshot/create.go` — L44, L56.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/create.go` — L93, L104.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/resume.go` — L46, L52.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/update.go` — L89, L95.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/pause.go` — L46, L52.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/authprovider/create.go` — L95, L101.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/add.go` — L80, L86.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/remove.go` — L98, L104.
- `neo4j-cli/aura/internal/subcommands/customermanagedkey/create.go` — L84, L95.
- `neo4j-cli/aura/internal/subcommands/graphanalytics/session/create.go` — L101, L115.

Audit-missed (1, added in scope):
- `neo4j-cli/aura/internal/subcommands/instance/overwrite.go` — L68, L74.

Transformation:
```go
// before
cmd.Println("Waiting for instance to be ready...")
cmd.Println("Instance Status:", pollResponse.Data.Status)
// after
fmt.Fprintln(cmd.ErrOrStderr(), "Waiting for instance to be ready...")
fmt.Fprintln(cmd.ErrOrStderr(), "Instance Status:", pollResponse.Data.Status)
```

**Pattern B: delete success → structured JSON stdout + human stderr line** (3 files)

- `neo4j-cli/aura/internal/subcommands/customermanagedkey/delete.go` — L35.
  ```go
  if statusCode == http.StatusNoContent {
      fmt.Fprintf(cmd.ErrOrStderr(), "customer-managed-key %s deleted\n", args[0])
      output.PrintBodyMap(cmd, cfg,
          api.NewSingleValueResponseData(map[string]any{"deleted": true, "id": args[0]}),
          []string{"deleted", "id"})
  }
  ```
- `neo4j-cli/aura/internal/subcommands/deployment/delete.go` — L55.
  ```go
  if api.IsSuccessful(statusCode) {
      fmt.Fprintf(cmd.ErrOrStderr(), "deployment %s deleted\n", deploymentId)
      output.PrintBodyMap(cmd, cfg,
          api.NewSingleValueResponseData(map[string]any{"deleted": true, "id": deploymentId}),
          []string{"deleted", "id"})
  }
  ```
- `neo4j-cli/aura/internal/subcommands/deployment/token/delete.go` — L57. Token has no id of its own; the deployment id is the natural identifier:
  ```go
  if api.IsSuccessful(statusCode) {
      fmt.Fprintf(cmd.ErrOrStderr(), "deployment-token for deployment %s deleted\n", deploymentId)
      output.PrintBodyMap(cmd, cfg,
          api.NewSingleValueResponseData(map[string]any{"deleted": true, "deployment_id": deploymentId}),
          []string{"deleted", "deployment_id"})
  }
  ```

**Pattern C: banners / plain-text-before-JSON → stderr** (3 files, partly overlapping with Pattern A files)

- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/create.go` — L86–88 banner.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/authprovider/create.go` — L87–89 banner.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/add.go` — L77 plain-text origin echo.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/remove.go` — L92 (empty list) and L94 (populated list) plain-text origin echoes.

All `cmd.Println` / `cmd.Printf` → `fmt.Fprintln` / `fmt.Fprintf` against `cmd.ErrOrStderr()`. Wording preserved.

**Pattern E: table-format warning → stderr** (1 file, CLI-95)

- `neo4j-cli/aura/internal/subcommands/tenant/get.go` — L42.
  ```go
  // before
  if cfg.Global.Format() == "table" || cfg.Global.Format() == "default" {
      cmd.Println("instance configurations are not visible with table output - please use a different output setting using --format if you would like to view these")
  }
  // after
  if cfg.Global.Format() == "table" || cfg.Global.Format() == "default" {
      fmt.Fprintln(cmd.ErrOrStderr(), "instance configurations are not visible with table output - please use a different output setting using --format if you would like to view these")
  }
  ```
  Wording and conditional gate unchanged.

**Pattern D: one-off confirmation → stderr** (1 file)

- `neo4j-cli/aura/internal/subcommands/config/project/use.go` — L23.
  ```go
  // before
  cmd.Printf("Set %s as default project with organization ID %s and project ID %s",
      args[0], defaultProject.OrganizationId, defaultProject.ProjectId)
  // after
  fmt.Fprintf(cmd.ErrOrStderr(), "Set %s as default project with organization ID %s and project ID %s\n",
      args[0], defaultProject.OrganizationId, defaultProject.ProjectId)
  ```
  Adds the missing `\n` incidentally.

### Files touched — tests

- `neo4j-cli/aura/internal/test/testutils/auratesthelper.go` — add two helpers:
  ```go
  func (helper *AuraTestHelper) AssertErrContainsStrings(expected []string) {
      out, err := io.ReadAll(helper.err)
      assert.Nil(helper.t, err)
      for _, exp := range expected {
          assert.Contains(helper.t, string(out), exp)
      }
  }

  // AssertOutIsValidJSON parses stdout via json.Unmarshal and fails the test
  // if stdout is empty or not valid JSON. This is the regression-pinning
  // assertion for CLI-82 — pre-fix, stdout had narration mixed with the JSON
  // body and would fail to unmarshal.
  func (helper *AuraTestHelper) AssertOutIsValidJSON() {
      out, err := io.ReadAll(helper.out)
      assert.Nil(helper.t, err)
      var v any
      assert.NoErrorf(helper.t, json.Unmarshal(out, &v),
          "stdout is not valid JSON; got: %q", string(out))
  }
  ```

- For each modified source file under Patterns A/B/C/D, the colocated `*_test.go`:
  - Drop the moved-to-stderr lines from `AssertOut(...)` / `AssertOutJson(...)` expected text.
  - Add an `AssertErrContainsStrings([]string{"Waiting for ...", "<Resource> Status:"})` (or equivalent) assertion.
  - For delete tests: switch from `AssertOut("Operation Successful\n")` (and equivalents) to `AssertOutJson(\`{"data":{"deleted":true,"id":"<id>"}}\`)` and add `AssertErrContainsStrings([]string{"customer-managed-key <id> deleted"})` (and equivalents).

Concrete tests requiring updates (non-exhaustive; the real gate is `make test`):
- `neo4j-cli/aura/internal/subcommands/instance/create_test.go::TestCreateFreeInstanceWithAwait` (L442+).
- `neo4j-cli/aura/internal/subcommands/instance/resume_test.go` — await rows.
- `neo4j-cli/aura/internal/subcommands/instance/overwrite_test.go` — await rows.
- `neo4j-cli/aura/internal/subcommands/instance/snapshot/create_test.go` — await rows.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/{create,resume,update,pause}_test.go` — await rows.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/authprovider/create_test.go` — await + banner.
- `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/{add,remove}_test.go` — await + origin echo.
- `neo4j-cli/aura/internal/subcommands/customermanagedkey/create_test.go` — await rows.
- `neo4j-cli/aura/internal/subcommands/customermanagedkey/delete_test.go::TestDeleteCustomerManagedKey` (L14+).
- `neo4j-cli/aura/internal/subcommands/deployment/delete_test.go::TestDeleteDeployment` + the org/project-from-config variant.
- `neo4j-cli/aura/internal/subcommands/deployment/token/delete_test.go`.
- `neo4j-cli/aura/internal/subcommands/graphanalytics/session/create_test.go` — await rows.
- `neo4j-cli/aura/internal/subcommands/config/project/use_test.go` — confirmation message.

### Files touched — changelog

- `.changes/unreleased/neo4j-cli-Patch-<ts>.yaml` via `changie new --projects neo4j-cli --kind Patch --body "fix(cli): route progress/status text to stderr so --format json output stays jq-parseable (CLI-82, CLI-95)"`.

### Cobra error/output contract (already verified)

- `cmd.Println` / `cmd.Printf` write to the writer set by `cmd.SetOut(...)` (defaults to `os.Stdout`). Tests bind this to `helper.out`.
- `cmd.ErrOrStderr()` returns the writer set by `cmd.SetErr(...)` (defaults to `os.Stderr`). Tests bind this to `helper.err`.
- `output.PrintBodyMap` / `output.PrintBody` already use `cmd.OutOrStdout()` and respect `--format` — no change needed there; we just gain a new caller in the three delete files.

### Skill bundle regen

The change does not touch any cobra `Long`, `Use`, `Example`, or flag-description string. `make generate-check` should stay clean. If it drifts (unexpected), regen via `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...` and commit in the same PR.

## Acceptance Criteria

- [ ] `neo4j-cli aura instance create --name=t --type=free-db --tenant-id=<x> --await --format json 2>/dev/null | jq .` exits 0, jq prints valid JSON, no narration leaks to stdout.
- [ ] `neo4j-cli aura customer-managed-key delete <id> --format json 2>/dev/null | jq .` exits 0, jq prints `{"data":{"deleted":true,"id":"<id>"}}` (or wrapper produced by `PrintBodyMap`), no `"Operation Successful"` on stdout.
- [ ] `neo4j-cli aura deployment delete <id> --format json 2>/dev/null | jq .` exits 0, jq prints the structured confirmation.
- [ ] `neo4j-cli aura deployment token delete --deployment-id <id> --format json 2>/dev/null | jq .` exits 0, jq prints the structured confirmation.
- [ ] `neo4j-cli aura dataapi graphql create ... --format json 2>/dev/null | jq .` exits 0, jq prints valid JSON, no `###` banner on stdout.
- [ ] `neo4j-cli aura dataapi graphql corspolicy allowed-origin add ... --format json 2>/dev/null | jq .` and `remove ...` exit 0, jq prints valid JSON, no `"New allowed origins: ..."` on stdout.
- [ ] `neo4j-cli aura config project use foo --format json 2>/dev/null | jq -n .` — stdout empty, narration on stderr terminated with `\n`.
- [ ] `neo4j-cli aura instance overwrite --instance-id <x> --source-instance-id <y> --await --format json 2>/dev/null | jq .` exits 0, jq prints valid JSON, no narration on stdout.
- [ ] `neo4j-cli aura tenant get <id> --format table 2>/dev/null` prints only the table on stdout; the "instance configurations are not visible..." warning is on stderr. Under `--format json` the warning is not emitted (gate preserved) and stdout is valid JSON.
- [ ] Stderr still contains the human narration for all of the above (verifiable via `2>&1 1>/dev/null`).
- [ ] Per REQ-F-009, at least one `AssertOutIsValidJSON()` test exists per offender category (await, delete, banner, origin-echo). Each of these tests demonstrably FAILS when the corresponding source-side fix is reverted (manual sanity check during implementation — flip one `fmt.Fprintln(cmd.ErrOrStderr(), ...)` back to `cmd.Println(...)` and confirm the regression test catches it).
- [ ] `make test` passes including new `AssertErrContainsStrings`, `AssertOutIsValidJSON`, and `AssertOutJson` assertions across the affected `*_test.go` files.
- [ ] `make fmt-check`, `make lint`, `make license-check`, `make generate-check` all clean.
- [ ] Changelog entry of kind `Patch` exists under `.changes/unreleased/`.

## Out of Scope

- F9 / CLI-94, F10 / CLI-95, F11 / CLI-96, F12 / CLI-97 — sister audit findings, separate Linear issues.
- CLI-86 / CLI-87 / CLI-88 / CLI-89 / CLI-90 — flag/verb canonicalisation, also separate.
- Refactoring the `--await` poll loop or status-transition logic.
- Adding `--quiet` / `--verbose` flags.
- Touching any non-Aura subtree (e.g. `query`, `credential`, `skill`) — none are in the audit's CLI-82 list and grep found no additional offenders in those subtrees beyond the F9–F11 sister findings.

### Delete-confirmation field keys (consistency rule)

Resolved: each delete confirmation matches that resource's existing list/get/create output convention.

- `customermanagedkey delete` → `{"deleted": true, "id": "<id>"}`. CMK list/get/create already use `"id"` (`customermanagedkey/list.go:41`, `get.go:33`, `create.go:81`).
- `deployment delete` → `{"deleted": true, "id": "<id>"}`. Deployment list/get/create already use `"id"` (`deployment/list.go:54`, `get.go:55`, `create.go:66`).
- `deployment token delete` → `{"deleted": true, "deployment_id": "<deploymentId>"}`. Token create/update output uses `"token"` (the JWT itself, see `deployment/token/create.go:59`), which is unavailable on a 204 delete response. The deployment id is the only honest identifier we have at delete time; naming it `"id"` would mislead consumers into thinking it's a token id.

## Open Questions

- Branch name: `oskar/cli-82-progress-status-text-to-stderr`? (Linear-provided default is `cli-82-b1-progress-and-status-text-emitted-to-stdout-instead-of` — long; recommend the shorter `oskar/` variant per CLAUDE.md convention.)
