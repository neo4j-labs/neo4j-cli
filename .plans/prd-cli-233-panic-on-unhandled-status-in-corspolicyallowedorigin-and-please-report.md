# PRD: Remove the last "please report an issue" panics/messages (CLI-233)

Linear: https://linear.app/neo4j/issue/CLI-233/panic-on-unhandled-status-in-corspolicyallowedorigin-and-please-report
Branch: `cli-233-panic-on-unhandled-status-in-corspolicyallowedorigin-and`

## Overview

CLI-227 (PR #239) removed `panic(reportIssueFatal(...))` from the Aura API error path `handleResponseError` (`neo4j-cli/aura/internal/api/response.go`), replacing every branch with typed `*clierr.CLIError` returns, scrubbing response bodies via `scrubbedBodyTrunc` (RedactText + StripControl, 4096-byte bound), and adding a class-based `default:` fallback. CLI-233 is the two follow-ups explicitly scoped out of #239's "Out of Scope" section.

### Problem 1 — `graphql/corspolicy/allowedorigin/utils.go` still panics

`getExistingOrigins` (`neo4j-cli/aura/internal/subcommands/graphql/corspolicy/allowedorigin/utils.go:34`) is called by `aura graphql cors-policy allowed-origin add` (`add.go:62`) and `remove` (`remove.go:66`) to diff the existing CORS origin set before PATCHing. It panics in two places:

| Site | Trigger | Today | Observed exit |
|---|---|---|---|
| `utils.go:44` | `statusCode != 200` | `panic(clierr.NewFatalError("unexpected status code %d … please report an issue in %s", …))` | **0** |
| `utils.go:50` | `json.Unmarshal` fails | `panic(err)` | **0** |

A panic is recovered only by `main.go`'s top-level `defer` (`main.go:132`), which prints "please report an issue" and lets `main` unwind normally — **exiting 0**. So a real error is reported as a bug *and* as success; scripts reading the exit code see nothing wrong.

**Reachability (verified).** `api.MakeRequest` (`api.go:56`) routes every non-2xx through `handleResponseError` (typed error, `err != nil`), and `getExistingOrigins` returns early at `utils.go:40-42` for those. The line-44 status panic can **only** fire on a 2xx≠200 (201/202/204/206) — `IsSuccessful` (`api.go:256`) treats all 2xx as success and returns `(body, status, nil)`. It is latent. The line-50 `panic(err)` is the realistic vector: Go's JSON unmarshal error strings embed a raw, **unscrubbed** snippet of the input body, and `errorMessages` (`response.go:66`) returns nil for an unparseable body, so a 2xx + malformed JSON reaches the `json.Unmarshal` at `utils.go:48` with `(body, 200, nil)`.

No sibling files share this panic pattern — repo-wide grep confirmed only `utils.go` panics on status in the graphql subtree; the only other non-test `panic(` on a status is `api.go:211` (version enum, out of scope).

### Problem 2 — `formatAuthorizationError` still says "please report an issue"

`formatAuthorizationError` (`response.go:502-523`) is the 401/403 handler reached from `handleResponseError` (`:102`, `:108`) and `MakeRawRequest` (`raw.go:107`). It already returns typed `NewAuthError` (exit 4) — the exit code is correct. Only the wording is wrong: two branches still carry the "please report an issue" framing CLI-227 deleted everywhere else.

| Line | Trigger | Today |
|---|---|---|
| `response.go:507` | 401/403 body not parseable as `ErrorResponse` | `NewAuthError("unexpected error [status %d] running CLI with args %s, please report an issue in %s", statusCode, RedactArgs(os.Args[1:]), IssuesURL)` |
| `response.go:517` | `ClearAccessToken` fails after auth already failed | appends `"Request failed authorization - … please report an issue in %s"` |

Both are ordinary auth-error paths a user hits on wrong/expired/revoked credentials, an unparseable 401/403 body, or a local keyring failure — none are CLI bugs.

## Goals

- `getExistingOrigins` contains zero `panic` statements; both branches return a typed `*clierr.CLIError` that flows as a return value through `add.go`/`remove.go` `RunE` → `main.go`'s normal error path (non-zero exit).
- `formatAuthorizationError` no longer emits "please report an issue" on any branch; ordinary auth failures read as auth failures, not bugs.
- No upstream body reaches an error message without passing through `RedactText` + `StripControl` and a byte bound — closing the line-50 `panic(err)` body-leak.
- The fix reuses CLI-227's helpers and constructors; it introduces no second mechanism.

## Non-Goals

- The remaining `panic`s in `api.go` (`:60`, `:86`, `:99`, `:212`), `ParseBody`/`ParseRawBody` (`response.go:499`), `token.go` transport/parse panics, `main.go`'s `recoverPanic` safety net, and the MCP `executor.go`/`stdio.go` "please report" messages (those are genuine internal-invariant panics, not user-input paths). All out of scope.
- Changing the `clierr.Codes` table or adding a new error category.
- Changing `recoverPanic` / `main.go` / `exitCodeFor`.
- Wiring `test/e2e/exitcodes/` into CI or the Makefile.
- Modelling any new response schema for the GraphQL CORS GET.

## Requirements

### Functional Requirements

- **REQ-F-001** — In `neo4j-cli/aura/internal/api/raw.go`, export a thin wrapper around the existing unexported `scrubbedBodyTrunc` (`:182`) so command packages outside `api` can surface upstream bodies through `*clierr.CLIError` messages without duplicating redaction logic:

  ```go
  // ScrubbedBodyTrunc renders an upstream response body for embedding in an
  // error message: RedactText + StripControl + size bounds (same as the
  // internal scrubbedBodyTrunc). Exported so command packages outside api can
  // surface upstream bodies through *clierr.CLIError messages without
  // duplicating redaction logic.
  func ScrubbedBodyTrunc(body []byte) string {
      return scrubbedBodyTrunc(body)
  }
  ```

  Internal call sites (`response.go:55`, `raw.go:197`) are unchanged — they keep calling `scrubbedBodyTrunc` directly. Do **not** export `upstreamDetail` (its `"upstream error [status N]"` prefix reads oddly for a 2xx) or duplicate `RedactText` + `StripControl` + `truncateBytes` in `allowedorigin`.

- **REQ-F-002** — In `neo4j-cli/aura/internal/subcommands/graphql/corspolicy/allowedorigin/utils.go`, replace both panics with typed returns. The function already returns `([]string, error)` and both call sites already `return err`, so this is mechanically a panic→return swap.

  Status branch (current `:43-45`) — the latent 2xx≠200 case:

  ```go
  if statusCode != http.StatusOK {
      return nil, clierr.NewUpstreamError("unexpected status code %d from GraphQL Data API CORS policy response: %s", statusCode, api.ScrubbedBodyTrunc(getResBody))
  }
  ```

  Unmarshal branch (current `:48-51`) — the body-leak case:

  ```go
  var parsedGetResBody DetailedBody
  if err := json.Unmarshal(getResBody, &parsedGetResBody); err != nil {
      return nil, clierr.NewUpstreamError("could not parse GraphQL Data API CORS policy response: %s", api.ScrubbedBodyTrunc(getResBody))
  }
  ```

  Drop `err.Error()` from the unmarshal branch — Go's JSON syntax errors echo a raw fragment of the input; the scrubbed body is more diagnostic and is the sanitized form.

  Exit code: `NewUpstreamError` (8, retryable) for both. Consistent with CLI-227's `RawStatusError` "5xx plus every other unmapped status" fallback (`raw.go:174`) and with CLI-227's removal of fatal/"report an issue" framing. A contract-breaking 2xx won't clear on retry — the same limitation already accepted for persistent 5xx.

- **REQ-F-003** — Remove now-unused imports from `utils.go`: `"os"` and `"github.com/neo4j/cli/common/clievents"` (the deleted line-44 message was their only use). Keep `encoding/json`, `fmt`, `net/http`, `clicfg`, `clierr`, `api`.

- **REQ-F-004** — In `neo4j-cli/aura/internal/api/response.go`, reword `formatAuthorizationError` to drop "please report an issue" on both branches. Keep `NewAuthError` (exit 4) and `authSuggestion` (`:24`) on both.

  Line 507 (unparseable 401/403 body) — drop `os.Args`/`RedactArgs`/`IssuesURL`, fold in the scrubbed body (matches CLI-227's move away from args-embedding; `upstreamDetail`'s doc comment at `:56-58` states the panic this replaced "interpolated resBody raw"):

  ```go
  return clierr.NewAuthError("unexpected error [status %d]: %s", statusCode, scrubbedBodyTrunc(resBody)).WithSuggestion(authSuggestion)
  ```

  Line 517 (`ClearAccessToken` failure) — reword, keep the `"Request failed authorization - "` prefix consistent with the success sibling at `:519` ("access token has been cleared and will be refreshed on next request - please retry the command"). Do **not** surface the raw `ClearAccessToken` error value — `ClearAccessToken` (`credentials/aura.go:104`) errors via `c.Get(cred.Name)` and `onUpdate()`, which can embed local filesystem/keyring detail that `RedactText` does not scrub; the current code already discards it, keep discarding:

  ```go
  messages = append(messages, "Request failed authorization - the local access token could not be cleared")
  ```

- **REQ-F-005** — Remove now-unused imports from `response.go`: `"os"` and `"github.com/neo4j/cli/common/clievents"`. Verified by grep — line 507 is the only `os.` and the only `clievents.` use in the file. Keep `encoding/json`, `fmt`, `io`, `net/http`, `strings`, `clicfg`, `credentials`, `clierr`, `output`.

- **REQ-F-006** — Add a NoPanic-style test for `getExistingOrigins`. `getExistingOrigins` is unexported and tests live in external package `allowedorigin_test`, so drive the `add` command through the mock harness. Reuse `registerProjectsMock`, `instanceGetBody`, `testOrgID`, `testProjectID` from `helpers_test.go`. Use `helper.ExecuteCommandE` (returns cobra's error) wrapped in `assert.NotPanics` — a panic inside `RunE` propagates and fails the test.

  New file `neo4j-cli/aura/internal/subcommands/graphql/corspolicy/allowedorigin/get_existing_origins_error_test.go`, table over the two latent cases:

  | Case | Status | Body | Expect |
  |---|---|---|---|
  | 202 accepted status | 202 | `{}` | status branch; msg contains "202"; not "please report" |
  | 204 no content status | 204 | `` | status branch; msg contains "204"; not "please report" |
  | 200 malformed body | 200 | `not valid json` | unmarshal branch; msg contains "not valid json"; not "please report" |

  Mock order: projects list (200) → `/v1/instances/{id}` (200, `tenant_id == projectID` to satisfy the `FetchAndVerifyInstanceInProject` ownership check) → `/v1beta5/instances/{id}/data-apis/graphql/{dataApiId}` (per-row status/body). No PATCH mock needed — `getExistingOrigins` errors before the PATCH. Assert `errors.As` into `*clierr.CLIError`, `ce.Code` in `clierr.Codes`, `NotContains "please report"`. Body fragments like `"not valid json"` survive `RedactText` + `StripControl`, so `Contains` is stable.

  Per AGENTS.md the aura root's `PersistentPreRunE` only runs through subcommands because `cobra.EnableTraverseRunHooks = true` — the existing `add_test.go` tests already work via `ExecuteCommand` which mounts the full tree; mirror that.

- **REQ-F-007** — In `neo4j-cli/aura/internal/api/response_test.go`, extend `TestHandleResponseError_ExitCodeMapping` (the table struct at `:125` already has `wantMsgOmit`/`wantMsgContain` fields, asserted at `:455-468`):

  - 401 row (`:148`, parseable body): add `wantMsgOmit: "please report"`.
  - 403-malformed row (`:281`, `<<<not-json>>>`): add `wantMsgOmit: "please report"`, `wantMsgContain: "403"` (proves the status is still echoed and the body is now folded in via `scrubbedBodyTrunc`).

- **REQ-F-008** — Add two focused direct-call tests in `response_test.go` (internal `package api`, so unexported `formatAuthorizationError` is callable). Reuse the `newAuthFixture` helper at `:109` (builds a real cfg + credential named `"x"`).

  - `TestFormatAuthorizationError_UnparseableBody_NoReportIssue`: body `"<html>gateway 502\n</html>"`, 401, credential from fixture. Assert `errors.As` → `*clierr.CLIError`, `ce.Code == 4`, `NotContains "please report"`, `NotContains "running CLI with args"`, `Contains "401"`, `ce.Suggestion == authSuggestion`.
  - `TestFormatAuthorizationError_ClearAccessTokenFailure_NoReportIssue`: parseable `{"errors":[{"message":"Invalid token"}]}`, credential `&credentials.AuraCredential{Name: "missing-cred"}` — not in the fixture's store, so `ClearAccessToken`'s `c.Get(cred.Name)` (`credentials/aura.go:105`) fails deterministically → line-517 branch. Assert `ce.Code == 4`, `NotContains "please report"`, `Contains "could not be cleared"`, `ce.Suggestion == authSuggestion`. This is the only branch in `formatAuthorizationError` currently without test coverage.

  Existing `api_test.go:428` (`TestGetToken_OtherNon2xx_NamesStatus`) and `:447` (`TestGetToken_EmptyTokenIsError`) already assert `NotContains "please report"` on the `getToken` path — unchanged.

- **REQ-F-009** — Add a changie entry. The exit-code change (panic→exit 8) and the message change are user-facing. Non-interactive:

  ```
  changie new --projects neo4j-cli --kind Patch --body "graphql cors-policy allowed-origin add/remove now return a typed upstream error (exit 8) instead of panicking when the data API responds with an unexpected 2xx status or a malformed CORS policy body, and authorization failure messages no longer say 'please report an issue'."
  ```

  Per `AGENTS.md`, the body describes only the observable effect — no mention of `getExistingOrigins`, panics, or helper extraction. Verify the claim by diffing golden strings in the new tests, not by describing the implementation.

### Non-Functional Requirements

- **REQ-NF-001** — No upstream-controlled bytes reach an error message unscrubbed or unbounded. The line-50 `panic(err)` leak (Go JSON error strings embedding a raw body fragment) is closed by routing through `api.ScrubbedBodyTrunc`. Grep the final diff for `string(getResBody)` / `err.Error()` reaching a format string without passing through `ScrubbedBodyTrunc`.

- **REQ-NF-002** — Zero behaviour change on any `formatAuthorizationError` path where the body parses as `ErrorResponse` and `ClearAccessToken` succeeds (the `:519` clean path and the parseable-body `:522` path). Message text, `authSuggestion`, and exit code 4 are byte-identical to today for those. The existing assertions in `TestHandleResponseError_ExitCodeMapping` for the parseable 401/403 rows are the guard (they only gain a `wantMsgOmit` on the malformed rows).

- **REQ-NF-003** — The `clierr.Codes` closed enum is untouched, so `neo4j-cli agent-context` needs no change and `TestErrorCodesInSyncWithClierr` stays green without edits.

- **REQ-NF-004** — No skill-bundle regeneration. No cobra `Short`/`Long`/`Example`, flag surface, or `ValidFormatValues` changes — `TestGenerator_RoundTrip` stays green. The corspolicy commands' `Long`/`Example` are untouched. Confirm with `make generate-check` on a clean tree if in doubt.

- **REQ-NF-005** — No MCP policy golden change. No command added/renamed — `mcp/server/testdata/policy.golden` is untouched.

- **REQ-NF-006** — `make test`, `make fmt-check`, `make lint` all pass. All `.go` files keep the Neo4j copyright header.

## Technical Considerations

**Reuse over reinvention.** CLI-227 already built the non-panicking contract this PRD extends: `scrubbedBodyTrunc` (`raw.go:182`), `upstreamDetail` (`response.go:54`), and the `clierr` constructors (`common/clierr/error.go:120-163`). `allowedorigin` is a different package, so the one new surface is a thin exported wrapper (`api.ScrubbedBodyTrunc`) — not a second redaction implementation. AGENTS.md singlesources `RedactText`/`StripControl` usage; duplicating them in `allowedorigin` would violate that.

**Why `NewUpstreamError` (8), not `NewFatalError` (1).** `NewFatalError` restates "this is a CLI bug, contact us" — exactly the framing CLI-233 removes. `RawStatusError`'s "5xx plus every other unmapped status" fallback (`raw.go:174`) maps the same class of "unexpected status" to `NewUpstreamError`, and CLI-227's whole direction was to move off fatal/"report an issue" framing. A surprising 2xx is not the user's fault (not validation/auth/usage), so exit 8 (retryable) is where it belongs. Accepted trade-off: a contract-breaking 2xx that persists won't clear on retry — the same limitation already accepted for persistent 5xx/307.

**Why line 507 drops `os.Args`.** CLI-227 moved the `handleResponseError` fallbacks away from embedding `os.Args`/`RedactArgs` in favor of the scrubbed body. `formatAuthorizationError:507` kept the old template. The body is more diagnostic (it is the actual unparseable payload) and is already redacted via `scrubbedBodyTrunc`. Removing it also drops the only `os`/`clievents` use in `response.go`, so both imports go — verified by grep.

**Why line 517 does not surface `err`.** `ClearAccessToken` (`credentials/aura.go:104-116`) errors via `c.Get(cred.Name)` (unknown name) or `onUpdate()` (local filesystem/keyring write). The latter can embed local paths that `RedactText` does not scrub (it is shape-based, not path-based). The current code already discards the value; keep discarding.

**Test-package layout.** `allowedorigin_test` is the external test package and drives the command via `testutils.AuraTestHelper` (mock HTTP). `getExistingOrigins` is unexported, so the command is the only entry point. `response_test.go` is internal `package api` and can call `formatAuthorizationError` directly — reuse the existing `newAuthFixture` (`:109`) rather than building a new cfg. Per AGENTS.md's anti-monolith rule, a new concern-named file (`get_existing_origins_error_test.go`) is preferred over growing `add_test.go`.

**e2e exitcode suite is not in CI.** `grep -rn e2e_exitcodes Makefile .github/workflows/` returns nothing — the suite runs only under explicit `-tags=e2e_exitcodes`. The unit tests already pin exit codes + `NotContains "please report"`; adding e2e scenarios for this fix has low gate-value relative to the fixture extension cost (the allowed-origin command needs a flat `/v1/instances` route plus a `/v1beta5/.../graphql/{dataApiId}` route the fixture doesn't currently serve). **Decision: skip e2e scenarios.** Run the untouched suite manually as a regression guard during verification.

## Acceptance Criteria

- [ ] `grep -n panic neo4j-cli/aura/internal/subcommands/graphql/corspolicy/allowedorigin/utils.go` returns zero hits.
- [ ] `getExistingOrigins` returns a typed `*clierr.CLIError` (exit 8) for both the 2xx≠200 and malformed-body cases; the error flows as a return value, not a panic.
- [ ] No upstream body reaches the `getExistingOrigins` error messages unscrubbed — both go through `api.ScrubbedBodyTrunc`.
- [ ] `formatAuthorizationError` no longer emits "please report an issue" on any branch; exit code stays 4 and `authSuggestion` is attached on both.
- [ ] `grep -rn "please report an issue in" neo4j-cli/aura` returns **zero** matches. (Only remaining repo-wide: `main.go:35` `recoverPanic` safety net.)
- [ ] `api.ScrubbedBodyTrunc` exists in `raw.go` and delegates to `scrubbedBodyTrunc`; internal call sites unchanged.
- [ ] Unused imports (`os`, `clievents`) removed from both `utils.go` and `response.go`.
- [ ] `TestAllowedOriginAdd_GetExistingOrigins_NoPanic` (or equivalent) passes across the 202/204/200-malformed table; asserts `NotContains "please report"` and `ce.Code` in `clierr.Codes`.
- [ ] `TestFormatAuthorizationError_UnparseableBody_NoReportIssue` and `TestFormatAuthorizationError_ClearAccessTokenFailure_NoReportIssue` pass.
- [ ] `TestHandleResponseError_ExitCodeMapping` 401 and 403-malformed rows gain `wantMsgOmit: "please report"` and pass.
- [ ] A changie entry exists under `.changes/unreleased/` describing only the observable effect.
- [ ] `make test`, `make fmt-check`, `make lint` all pass.
- [ ] `go test -tags=e2e_exitcodes -count=1 ./test/e2e/exitcodes/...` passes (regression guard; suite untouched).

## Out of Scope

- Remaining `panic`s in `api.go`, `ParseBody`/`ParseRawBody`, `token.go`, `main.go`'s `recoverPanic`, and MCP `executor.go`/`stdio.go` "please report" messages.
- Changing `clierr.Codes`, `recoverPanic`, `main.go`, or `exitCodeFor`.
- Wiring `test/e2e/exitcodes/` into CI or the Makefile.
- Modelling any new response schema for the GraphQL CORS GET.
- Exporting `upstreamDetail` or duplicating redaction logic in `allowedorigin`.

## Open Questions

**Resolved:**

- ~~Exit code for both `utils.go` branches~~ — **decided: `NewUpstreamError` (8).** Consistent with CLI-227's `RawStatusError` fallback and removal of fatal/"report an issue" framing.
- ~~e2e exitcode scenarios~~ — **decided: skip.** Not in CI; unit tests pin exit codes + wording; fixture extension cost not justified.

**Open, to settle during implementation (not blocking):** none.
