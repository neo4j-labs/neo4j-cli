# PRD: CLI-144 — Lock §4.2 empty-list-exit-0 with tests

## Overview

CLI-144 is the F5 follow-up from the CLI-108 audit. The current
behaviour is already correct — `instance list`, `tenant list`, and the
`query` command all return `nil` from `RunE` on any 2xx with zero
rows, so the process exits 0 and the renderer emits the empty
envelope. There is no Go source change in scope. The risk is silent
regression: a future refactor could wrap an empty payload in
`clierr.NewUsageError(...)` and nothing in CI would notice. This PRD
scopes the three golden tests that lock the current behaviour in
place.

The "items → exit 0, full array" case is already covered by the
existing happy-path tests
(`TestListInstances`, `TestListTenants`, `TestRunQuery_HappyPath_JSONOutput`).
Only the empty-array case needs new coverage.

## Goals

- A regression that makes `instance list` non-zero exit on an empty
  `{"data": []}` payload fails `make test`.
- Same for `tenant list`.
- Same for `neo4j-cli query` when the underlying Cypher returns zero
  rows (representative coverage for the broader query subsystem).
- New tests follow the file-local style — flat `TestXxx` funcs, not
  table-driven — per the AGENTS.md / auto-memory note
  `feedback_consistent_with_existing_code`: mirror in-file convention
  over ticket-text format proposals.

## Non-Goals

- Any change to production code under `neo4j-cli/aura/internal/subcommands/instance/`,
  `tenant/`, or `neo4j-cli/query/`. Behaviour is verified correct;
  this is test-only coverage.
- Restructuring existing tests into a table-driven shape. The ticket
  text says "Table tests"; the user has chosen to mirror the
  existing flat-func convention instead.
- Adding redundant items-case rows. The existing happy-path tests
  already serve as the lock for the populated-array path.
- Skill-bundle regeneration. `go generate ./neo4j-cli/internal/skill/...`
  is only triggered by changes to cobra-tree inputs (help text,
  flags, `Long`/`Example`). Pure `*_test.go` additions do not affect
  the bundle.
- Changelog entry. Per AGENTS.md, internal changes with no
  user-visible effect skip `changie new`.
- Coverage for `--format table` or `--format toon` empty rendering.
  The `--format json` assertion is sufficient as the regression
  trip-wire; the renderer is shared so a regression in one format
  surface would surface elsewhere too.

## Requirements

### Functional Requirements

- **REQ-F-001**: A new test function
  `TestListInstances_EmptyData` MUST be added to
  `neo4j-cli/aura/internal/subcommands/instance/list_test.go`. It
  MUST:
  - Register the existing `registerProjectsMock` helper.
  - Register a `/v1/instances` mock returning HTTP 200 with body
    `{"data": []}`.
  - Execute `instance list --organization-id <testListOrgID> --project-id <testListProjectID>`.
  - Assert the mock was called once.
  - Assert stdout JSON equals `{"data": []}` via `helper.AssertOutJson`.
- **REQ-F-002**: A new test function
  `TestListTenants_EmptyData` MUST be added to
  `neo4j-cli/aura/internal/subcommands/tenant/list_test.go`. It MUST:
  - Register a `/v1/tenants` mock returning HTTP 200 with body
    `{"data": []}`.
  - Execute `tenant list`.
  - Assert the mock was called once.
  - Assert stdout JSON equals `{"data": []}` via `helper.AssertOutJson`.
  - NOT re-assert the deprecation warning; that is already locked by
    `TestListTenants_DeprecationWarning` and duplicating the assertion
    couples this test to an unrelated concern.
- **REQ-F-003**: A new test function
  `TestRunQuery_EmptyResult_ExitsZero` MUST be added to
  `neo4j-cli/query/run_test.go`. It MUST:
  - Build a `seamRouter` via `newSeamRouter()`.
  - Register an EXPLAIN response (`makeQueryResponse` with empty
    values + `QueryType = neo4j.QueryTypeReadOnly`) for a stable
    canonical statement (suggested: `RETURN 1 AS n WHERE false` plus
    its `EXPLAIN` prefix).
  - Register the run-time response (`makeQueryResponse` with empty
    values).
  - Call `r.install(t)`.
  - Build a `runHarness` with `output = "json"`.
  - Execute the harness with `--uri`, `--password`, and the cypher
    statement.
  - Assert `err == nil` (i.e. cobra would exit 0).
  - Decode stdout into the existing `decodedResult` shape and assert
    `Columns == []string{"n"}`, `Rows` is empty, `Truncated` is
    false.
- **REQ-F-004**: New tests MUST be flat `TestXxx(t *testing.T)`
  functions colocated with the source-file-named test file. No
  table-driven restructuring. No new helper file.

### Non-Functional Requirements

- **REQ-NF-001**: `make fmt-check`, `make lint`, and `make test` MUST
  all pass on the resulting branch (the AGENTS.md final-gate rule).
- **REQ-NF-002**: New test names MUST follow the existing
  `TestListInstances_*` / `TestListTenants_*` / `TestRunQuery_*`
  prefix patterns so `go test -run` discovery stays predictable.
- **REQ-NF-003**: No new external dependencies, no new test helpers,
  no new package-level seams. All three tests reuse the existing
  `testutils.NewAuraTestHelper`, `newSeamRouter`, `newRunHarness`,
  `makeQueryResponse`, and `decodedResult` machinery already in the
  packages.

## Technical Considerations

- **Why JSON-only assertion?** Stdout-rendering for `--format table`
  and `--format toon` shares the same upstream `output.PrintBodyMap` /
  `output.PrintBody` path that the JSON assertion already exercises.
  Locking the JSON envelope is sufficient as a trip-wire; layering
  three format assertions would add maintenance cost without raising
  the regression-detection rate.
- **Why a separate runQuery test rather than extending `output_test.go`?**
  `TestRenderRows_JSON` in `output_test.go` covers the renderer in
  isolation. The CLI-108 audit finding is specifically about the
  full `runQuery` exit-code path — a renderer-only test wouldn't
  catch a regression where someone adds `if len(rows)==0 { return err }`
  in `runQuery` itself. The new test exercises the full RunE.
- **EXPLAIN preflight.** The query subsystem always preflights with
  an `EXPLAIN` to classify read vs. write. The new test must register
  *both* an EXPLAIN response and a runtime response for the canonical
  statement, mirroring `TestRunQuery_HappyPath_JSONOutput`.
- **AssertOutJson normalisation.** `helper.AssertOutJson` trims
  whitespace and re-parses both sides as JSON, so the assertion
  string can be pretty or compact — `{"data": []}` is the simplest
  form.
- **Tenant deprecation noise.** `tenant list` emits a deprecation
  warning on stderr unconditionally. The new test ignores stderr to
  stay focused on the exit-code/stdout lock and to avoid coupling to
  warning copy that may change.
- **Test placement.** Each new function lives next to the existing
  `TestList*` siblings in the same file — consistent with the AGENTS.md
  guidance to name test files per command and not aggregate into a
  package-level `*_test.go`.

## Acceptance Criteria

- [ ] `TestListInstances_EmptyData` exists in
  `neo4j-cli/aura/internal/subcommands/instance/list_test.go` and
  passes.
- [ ] `TestListTenants_EmptyData` exists in
  `neo4j-cli/aura/internal/subcommands/tenant/list_test.go` and
  passes.
- [ ] `TestRunQuery_EmptyResult_ExitsZero` exists in
  `neo4j-cli/query/run_test.go` and passes.
- [ ] No production source file under `neo4j-cli/` is modified.
- [ ] No new files are created (all three tests land in existing
  `*_test.go` files).
- [ ] No new changelog entry is added.
- [ ] `make fmt-check`, `make lint`, `make test` are clean.
- [ ] Mutation check (manual, not committed): temporarily making one
  of the three production handlers return a `clierr.NewUsageError`
  on empty data causes the corresponding new test to fail; reverting
  restores green. Confirms the lock is meaningful.

## Out of Scope

- Refactoring existing `TestList*` happy-path tests into a
  table-driven shape.
- Format-specific empty-rendering assertions (`--format table`,
  `--format toon`).
- Coverage for other list endpoints (`project list`, `credential aura-client list`,
  etc.). CLI-108 F5 names only the three above; broader sweep
  belongs to a follow-up if the audit calls for it.
- Any update to `neo4j-cli/aura/internal/subcommands/project/`
  (project list) — out of CLI-144 scope even though it shares the
  same code path.

## Open Questions

None. Style and scope choices were resolved via clarifying questions
prior to PRD generation (mirror existing flat-func style; treat
existing happy-path tests as the items-case lock).
