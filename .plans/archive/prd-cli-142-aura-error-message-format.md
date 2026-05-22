# PRD: CLI-142 — Aura API error message format (`[]string` stringification)

Linear: https://linear.app/neo4j/issue/CLI-142/instance-get-exit-0-string-stringified-error-cli-108-c
Parent: CLI-108 audit (B2 + F2).
Source plan: `/Users/oskarhane/.claude/plans/lazy-stirring-oasis.md` (locked).

## Overview

`neo4j-cli aura` HTTP error handling in `neo4j-cli/aura/internal/api/response.go` builds a `[]string` of error messages from the JSON envelope and then formats it via `clierr.NewXError("%s", messages)`. Go's default `[]string` stringification leaks through — the user sees `Error: [DB not found: does-not-exist]` (brackets + space-separated) instead of a human-readable rendering. Six branches share the bug: 400, 402, 404, 405, 409, and 5xx.

The repro's secondary symptom (exit 0 on not-found) is already fixed — `handleResponseError` returns `clierr.NewNotFoundError(...)` (`Code == 3`) on HTTP 404, and `main.exitCodeFor` maps it through `os.Exit(3)`. Confirmed by `response_test.go::TestHandleResponseError_ExitCodeMapping` and `graphanalytics/session/get_test.go:148-151`.

This feature unifies all seven envelope-formatting sites (the six broken branches plus the already-correct `formatAuthorizationError`) on a single in-file helper that produces the canonical multi-line bracketed shape already used by the 401/403 path:

```
Error: [
	msg1,
	msg2
]
```

It also wires the latent `SingleValueResponseData.Errors` path (response.go:380 — currently dead code) through `clierr.NewNotFoundError` for defense-in-depth, and tightens the two flagship not-found tests to assert exit code 3 via `errors.As`.

## Goals

- Eliminate Go's default `[]string` stringification (`[m1 m2]`) from all six broken branches in `handleResponseError`.
- Standardise the rendered envelope shape across every status-code branch on the multi-line bracket format already used by `formatAuthorizationError` (`response.go:440-442`). Single source of truth via a `formatBracketedMessages` helper.
- Lock the new shape into every test currently pinning the buggy single-line shape, plus tighten `instance get` / `tenant get` not-found tests to assert `Code == 3`.
- Close the latent 2xx-with-errors-body gap: a `MakeRequest` 2xx branch with an `errors[]` array surfaces as a `*clierr.CLIError{Code: 3}` rather than silently succeeding.
- Ship a `Patch` changelog entry capturing the user-visible format change.

## Non-Goals

- No changes to exit code mapping or `clierr.CLIError` shape — already correct.
- No changes to the renderer (`common/clierr/render.go`).
- No changes to per-resource suggestions / `WithResource` / `WithSuggestion` chains — keep the existing CLI-143 wiring intact in every touched branch.
- No agent-skill bundle regeneration — runtime error formatting, not cobra help text. `go generate` is unaffected.
- No conversion to single-line (e.g. `; `-joined) output. The ticket text suggested that shape; we are deliberately overriding it to stay consistent with the 401/403 path.
- No restructuring of how messages are collected (the `for _, e := range errorResponse.Errors { messages = append(...) }` loops stay as-is; only the final `%s` formatting changes).

## Requirements

### Functional Requirements

**Producer-side format helper (`neo4j-cli/aura/internal/api/response.go`)**

- REQ-F-001: A new package-private helper `formatBracketedMessages(messages []string) string` returns `fmt.Sprintf("[\n\t%s\n]", strings.Join(messages, ",\n\t"))`. Empty `messages` yields `"[\n\t\n]"` — acceptable; happens only if upstream returned no errors, which is itself a contract violation we surface verbatim.
- REQ-F-002: The six broken branches now build their envelope via `formatBracketedMessages(messages)`:
  - `response.go:69` (400, `NewValidationError`) — preserves `.WithSuggestion("See 'neo4j-cli aura <cmd> --help' for valid flags and values.")`.
  - `response.go:97` (404, `NewNotFoundError`) — preserves `.WithResource(resourceType, resourceID).WithSuggestion(suggestionForResource(resourceType))`.
  - `response.go:110` (405, `NewUpstreamError`).
  - `response.go:123` (402, `NewConflictError`) — preserves the `suggestionForPaymentRequired(errorResponse)` suggestion attachment.
  - `response.go:140` (409, `NewConflictError`).
  - `response.go:159` (5xx, `NewUpstreamError`).
- REQ-F-003: `formatAuthorizationError` (`response.go:440-442`) is refactored to call `formatBracketedMessages(messages)`. Output is byte-identical to today — no test changes required for the existing 401/403 tests.

**Latent 2xx-with-errors-body coverage (`neo4j-cli/aura/internal/api/api.go`)**

- REQ-F-004: A new package-private helper `extractEmbeddedErrors(body []byte) []string` in `response.go` unmarshals `body` into the existing `ErrorResponse` shape and returns each `Error.Message` verbatim (NO `field: message` prefix — matches the 404 path, since `SingleValueResponseData` is the single-resource get shape). Unmarshal failure or empty `Errors` returns `nil` so the happy path is unaffected.
- REQ-F-005: `MakeRequest` (`api.go:103-111`) inspects the 2xx response body before returning. When `extractEmbeddedErrors` yields ≥1 message:
  ```go
  resourceType, resourceID := parseResourceFromRequest(req)
  return responseBody, res.StatusCode,
      clierr.NewNotFoundError("%s", formatBracketedMessages(msgs)).
          WithResource(resourceType, resourceID).
          WithSuggestion(suggestionForResource(resourceType))
  ```
  Otherwise the existing `return responseBody, res.StatusCode, nil` path runs unchanged.
- REQ-F-006: The check runs against the already-read body — no second read of `res.Body`. `responseBody` is still returned (callers that ignore the err and inspect body keep their existing semantics; the typical caller pattern returns immediately on err).

**Test updates — pinned single-line bracket shape (28 sites)**

- REQ-F-007: Every assertion pinning `Error: [<msg>]` (single-line) becomes the multi-line shape using a backtick raw string + tab indent. Pattern:
  ```go
  helper.AssertErr(`Error: [
  	<msg>
  ]`)
  ```
  Verified file list (use `rg -n '"Error: \[' neo4j-cli/aura/internal/subcommands/` before editing to catch any miss):
  - `subcommands/instance/{get,create,delete,update,pause,resume}_test.go`
  - `subcommands/tenant/get_test.go`
  - `subcommands/customermanagedkey/{get,delete}_test.go`
  - `subcommands/graphanalytics/session/{get,delete}_test.go`
  - `subcommands/deployment/{create,get}_test.go`
  - `subcommands/deployment/token/create_test.go` (nested `...]]` site at `token/create_test.go:116` — the OUTER pair is the buggy wrapper, the INNER pair is API payload content; only the outer pair is touched).
  - `subcommands/import/job/{create,get,cancel}_test.go`

**Test updates — exit-code assertions on not-found**

- REQ-F-008: `subcommands/instance/get_test.go::TestGetInstanceNotFoundError` switches from `helper.ExecuteCommand(...)` + stderr-only assertion to `helper.ExecuteCommandE(...)` + `errors.As(err, &ce)` + `require.Equal(t, 3, ce.Code)`. Stderr text assertion stays (now multi-line bracket shape). Mirror `graphanalytics/session/get_test.go:148-151`.
- REQ-F-009: `subcommands/tenant/get_test.go::TestGetTenantNotFoundError` gets the same treatment as REQ-F-008.

**New test — latent 2xx-with-errors-body path**

- REQ-F-010: A new `TestMakeRequest_2xxWithEmbeddedErrors` in `neo4j-cli/aura/internal/api/api_test.go` mocks a 200 response against `/v1/instances/x` returning:
  ```json
  {"data":{"id":"x"},"errors":[{"message":"DB not found: x","reason":"db-not-found"}]}
  ```
  and asserts:
  - returned `err` matches `*clierr.CLIError` via `errors.As` with `Code == 3`,
  - rendered `ce.Message` equals `"[\n\tDB not found: x\n]"`,
  - `ce.ResourceType == "instance"`, `ce.ResourceID == "x"`,
  - `ce.Suggestion == "Run 'neo4j-cli aura instance list' to see available instances."` (from `suggestionForResource("instance")`).

### Non-Functional Requirements

- REQ-NF-001: All gates clean: `make fmt-check`, `make lint`, `make test`, `make generate-check`.
- REQ-NF-002: All touched and new `.go` files start with the standard Neo4j copyright header.
- REQ-NF-003: Changelog entry under `.changes/unreleased/` via `changie new --projects neo4j-cli --kind Patch --body "Aura API error messages now render in the same multi-line bracket format used for auth errors instead of Go's default []string stringification."`.
- REQ-NF-004: No test relying on a stable JSON envelope (`render_test.go`, format=json paths) regresses — the rendered `message` field in `--format json` will newly contain literal `\n` and `\t` characters; consumers (CI scripts, agents) already treat the field as opaque, but flag any envelope-pinning test we find and update accordingly.

## Technical Considerations

### Format helper placement

The bracket format is Aura-specific (Aura wraps multiple API error messages in one envelope). Keep `formatBracketedMessages` package-private inside `neo4j-cli/aura/internal/api/response.go` next to the existing `parseResourceFromRequest` / `suggestionForResource` helpers. Do NOT promote to `common/clierr/` — the rest of the CLI (Bolt, dbms credentials, query) produces single-error messages and doesn't need this shape.

### Why we override the ticket's `; ` suggestion

The Linear ticket text proposes `strings.Join(messages, "; ")`. The same file already has a working format at `response.go:440-442` (`formatAuthorizationError` — `[\n\t%s\n]` with `,\n\t` separator). Introducing a third style (`; ` would be a third) breaks in-file convention. Memory note `feedback-consistent-with-existing-code` captures the general principle. Decision is locked.

### `MakeRequest` defensive parse cost

The 2xx defense-in-depth check adds one extra `json.Unmarshal` per successful response. For a CLI making at most a handful of requests per invocation this is negligible (microseconds). No streaming / large-payload concerns — Aura responses are kilobytes.

### Reusable utilities (already exist)

- `api.parseResourceFromRequest` (`response.go:171-183`) — used by REQ-F-005 unchanged.
- `api.suggestionForResource` (`response.go:191-206`) — used by REQ-F-005 unchanged.
- `clierr.NewNotFoundError` / `NewValidationError` / `NewConflictError` / `NewUpstreamError` — all return `*CLIError` so `.WithSuggestion(...)` / `.WithResource(...)` chains hold.
- `testutils.NewAuraTestHelper.ExecuteCommandE` (`auratesthelper.go:55`) — pattern for `errors.As` exit-code assertions.
- `clierr.CLIError.WithResource` / `WithSuggestion` (`common/clierr/error.go:44, 53`) — mutating, returns receiver.

### Test patterns

Multi-line bracket assertion (28 sites):

```go
helper.AssertErr(`Error: [
	DB not found: 24d18db5
]`)
```

Exit-code assertion (REQ-F-008, REQ-F-009, REQ-F-010):

```go
err := helper.ExecuteCommandE("instance get <id> ...")
var ce *clierr.CLIError
require.True(t, errors.As(err, &ce), "expected *clierr.CLIError, got %T: %v", err, err)
require.Equal(t, 3, ce.Code)
```

### Files touched

- `neo4j-cli/aura/internal/api/response.go` — new `formatBracketedMessages` helper, new `extractEmbeddedErrors` helper, 6 format-string call-site updates, `formatAuthorizationError` refactor.
- `neo4j-cli/aura/internal/api/api.go` — `MakeRequest` 2xx branch defense-in-depth check.
- `neo4j-cli/aura/internal/api/response_test.go` — keep `TestHandleResponseError_ExitCodeMapping` (no message-shape regression expected; check whether the existing 401 test pins exact byte-output and adjust if needed).
- `neo4j-cli/aura/internal/api/api_test.go` — new `TestMakeRequest_2xxWithEmbeddedErrors`.
- `neo4j-cli/aura/internal/subcommands/instance/{get,create,delete,update,pause,resume}_test.go` — multi-line bracket conversions + exit-code assertion in `get_test.go`.
- `neo4j-cli/aura/internal/subcommands/tenant/get_test.go` — multi-line bracket + exit-code assertion.
- `neo4j-cli/aura/internal/subcommands/customermanagedkey/{get,delete}_test.go` — multi-line bracket conversions.
- `neo4j-cli/aura/internal/subcommands/graphanalytics/session/{get,delete}_test.go` — multi-line bracket conversions.
- `neo4j-cli/aura/internal/subcommands/deployment/{create,get}_test.go` and `subcommands/deployment/token/create_test.go` — multi-line bracket conversions.
- `neo4j-cli/aura/internal/subcommands/import/job/{create,get,cancel}_test.go` — multi-line bracket conversions.
- `.changes/unreleased/neo4j-cli-Patch-<timestamp>.yaml` — changelog entry.

### Risks / blast radius

- The rendered message change is user-visible. Anyone scripting against the exact byte-shape of stderr will break. Mitigation: changelog entry calls it out; `--format json` consumers already get a structured `message` field (the embedded newlines are valid JSON-encoded `\n`).
- 28 mechanical test edits — risk is missing one and getting a CI failure. Mitigation: pre-edit grep, post-edit `make test` on every package touched.
- `formatAuthorizationError` refactor must produce byte-identical output to today, or existing 401/403 tests fail. Mitigation: helper signature chosen so the format string is literally the same; cross-check `instance/get_test.go:367-370` after refactor.
- `MakeRequest` 2xx defense-in-depth check — if Aura ever returns an unrelated `errors[]` key in a successful body (no current evidence) this would convert that success to a Code==3 failure. Acceptable; matches the intent of the ticket scope ("verify against `SingleValueResponseData.Errors`").

## Acceptance Criteria

- [ ] `response.go` has `formatBracketedMessages(messages []string) string` returning `fmt.Sprintf("[\n\t%s\n]", strings.Join(messages, ",\n\t"))`.
- [ ] All six broken branches (400, 402, 404, 405, 409, 5xx) build their envelope via the helper; suggestion / resource chains preserved.
- [ ] `formatAuthorizationError` calls the helper; existing 401/403 test (`instance/get_test.go:367-370`) still passes without edits.
- [ ] `MakeRequest` 2xx branch detects `errors[]` in the body and returns `*clierr.CLIError{Code: 3}` with `ResourceType` / `ResourceID` / `Suggestion` set via the existing per-resource lookup.
- [ ] `TestMakeRequest_2xxWithEmbeddedErrors` exists in `api/api_test.go` and asserts Code, message shape, resource type/id, and suggestion.
- [ ] All 28 single-line `Error: [<msg>]` assertions across the aura subcommand tree are converted to the multi-line bracket shape.
- [ ] `instance/get_test.go::TestGetInstanceNotFoundError` and `tenant/get_test.go::TestGetTenantNotFoundError` use `ExecuteCommandE` + `errors.As` + `require.Equal(t, 3, ce.Code)`.
- [ ] Manual verification: `bin/neo4j-cli aura instance get does-not-exist --organization-id <real> --project-id <real>` against real Aura produces multi-line bracket stderr and `echo $?` → `3`.
- [ ] Changelog `.changes/unreleased/neo4j-cli-Patch-*.yaml` lands with the body in REQ-NF-003.
- [ ] `make fmt-check && make lint && make test && make generate-check` clean.

## Out of Scope

- Single-line `; `-joined output (ticket text). Overridden in favour of in-file consistency.
- Promoting `formatBracketedMessages` to `common/clierr/`. Aura-specific envelope, keep local.
- Changes to `--format json` schema or `common/clierr/render.go`. The `message` field will newly contain embedded newlines but the field-name contract is unchanged.
- Adding 2xx-with-errors handling to `ListResponseData`. Aura list endpoints don't carry an `errors[]` array in success bodies; introducing a check there is speculative.
- Reformatting the existing `formatAuthorizationError` message shape (still `[\n\t%s\n]`, just via the helper).

## Open Questions

(none — separator, format style, latent-path inclusion, and 2xx message shape all locked in plan-mode Q&A)
