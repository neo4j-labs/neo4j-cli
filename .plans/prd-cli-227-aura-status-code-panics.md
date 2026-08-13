# PRD: Stop panicking on documented Aura v2beta1 status codes (CLI-227)

Linear: https://linear.app/neo4j/issue/CLI-227/aura-api-client-panics-on-documented-v2beta1-status-codes-413-415-422
Branch: `cli-227-aura-api-client-panics-on-documented-v2beta1-status-codes`

## Overview

`handleResponseError` (`neo4j-cli/aura/internal/api/response.go:79`) is the sole error path for `MakeRequest`, which backs ~55 shipped `aura` subcommands. It **panics** on several HTTP statuses the live Aura v2beta1 spec documents. Panics are recovered only by `main.go`'s top-level `defer` (`neo4j-cli/main.go:133`), which prints a "please report an issue" line and then lets `main` unwind normally — **exiting 0**.

Net effect: a user who hits the Multi-DB database limit (`POST .../instances/{id}/databases` → 422, documented as *"Can be caused by a failed request body validation, the database limit for the Instance being reached, or invalid clone parameters"*) gets a bug-report message and a **success** exit code instead of the API's own explanation.

This PRD makes `handleResponseError` panic-free and returns a class-appropriate `*clierr.CLIError` for every status, carrying the upstream body when that body does not match the `ErrorResponse` shape.

### Current panic / mis-class inventory

| Site | Status | Today | Observed exit |
|---|---|---|---|
| `response.go:83` | body-read failure | `panic(NewFatalError(...))` | 0 |
| `response.go:89` | 307 | `panic(reportIssueFatal)` | 0 |
| `response.go:96` | 400 + unparseable body | `panic(reportIssueFatal)` | 0 |
| `response.go:116` | 403 + unparseable body | `panic(reportIssueFatal)` | 0 |
| `response.go:181` | 415 | `panic(reportIssueFatal)` | 0 |
| `response.go:200` `default:` | 413, 422, any unmodelled status | `panic(NewFatalError("unexpected status code %d and body %s …"))` | 0 |
| `response.go:127` | 404 + unparseable body | `return reportIssueFatal` | 1 |
| `response.go:141` | 405 + unparseable body | `return reportIssueFatal` | 1 |
| `response.go:154` | 402 + unparseable body | `return reportIssueFatal` | 1 |
| `response.go:171` | 409 + unparseable body | `return reportIssueFatal` | 1 |
| `response.go:190` | 5xx + unparseable body | `return reportIssueFatal` | 1 |

### Live-spec audit (drives the design)

Audited `https://api.neo4j.io/v2beta1/spec.json` — OpenAPI 3.0.0, 43 paths / 70 operations:

- **413 and 415** are documented on `POST .../graph-analytics/sessions` and `POST .../graph-analytics/sessions/sizing` — both reachable from the **shipped** `aura graph-analytics session create` command (`neo4j-cli/aura/internal/subcommands/graphanalytics/session/create.go`). This is not hypothetical.
- **422** is documented on `POST .../instances/{instance_id}/databases` and `POST .../instances/{instance_id}/databases/{database_id}/restore`.
- Response-status census: `400`×38, `401`×55, `403`×44, `404`×34, `405`×1, `409`×7, `413`×2, `415`×2, `422`×2, `429`×3, `500`×22.
- **Every single 4xx/5xx response in the whole spec declares only a `description` with no `content` schema — except two** (`400` on `GET /organizations/{org_id}/billing/ledger` and `.../billing/usage`, which use `BillingErrorResponse`). So a schema-less error body is the **normal** case on v2beta1, and the raw-body fallback is the primary path, not an edge case.

### Corrections to the Linear ticket

The ticket asks to model three additional error shapes. Two of those are based on a misread of the spec:

- **`GDSError`** — referenced **only on 2xx responses** (200/202 on the graph-analytics session operations), never as a 4xx/5xx error body. Nothing to model.
- **`InvokeAgentError`** — referenced by **zero** response objects anywhere in the spec.
- **`BillingErrorResponse`** (`errors[].{error, message}`) — real, but confined to the two billing endpoints, for which the CLI ships **no commands**. The scrubbed raw-body fallback surfaces its content verbatim anyway.

All three are therefore out of scope (see Non-Goals). The audit result must be recorded as a code comment so it is not re-litigated.

The ticket also does not mention two things this PRD covers:

- The `default:` panic at `response.go:200` interpolates `resBody` **unscrubbed** into the message, so a secret echoed back by the API can reach stdout. Every new path must go through the scrub/truncate helper.
- The real user-visible symptom is **exit code 0**, not just the wrong message — `recoverPanic` does not re-panic or set an exit code.

## Goals

- `handleResponseError` contains zero `panic` statements; every status returns a typed `*clierr.CLIError`.
- 307, 413, 415, 422, and any unmodelled status produce a **non-zero** exit code and surface the API's own body.
- A 4xx/5xx whose body is empty or non-JSON no longer degrades to `fatal_error` (1) — it keeps the exit code its status class implies.
- `handleResponseError` and `RawStatusError` (`raw.go:135`) agree on the status→exit-code contract, sharing one scrub/truncate helper rather than two parallel implementations.
- No upstream body reaches an error message without passing through `RedactText` + `StripControl` and a byte bound.

## Non-Goals

- The identical panic at `neo4j-cli/aura/internal/subcommands/graphql/corspolicy/allowedorigin/utils.go:44` (panics on any non-200 from the GraphQL CORS get). Same bug class, separate Linear issue.
- Modelling `BillingErrorResponse` / `GDSError` / `InvokeAgentError`, or widening the `Error` struct (`response.go:60`). Per the audit above.
- Changing `RawStatusError`'s behaviour, exit codes, or message format. It is already correct; it is only refactored to share a helper.
- Changing the `clierr.Codes` table (`common/clierr/error.go:88`) or adding a new error category.
- Changing `recoverPanic` / `main.go`. It stays as the safety net for the remaining panics in `api.go` (transport, body read) and `ParseBody`/`ParseRawBody`.
- Changing the `405 → upstream_error (8)` mapping. It is a deliberate pre-existing divergence from the 4xx class, already documented at `raw.go:154-157`; touching it is unrelated scope.
- Removing panics from `api.go` (`:60`, `:87`, `:99`, `:212`) or `ParseBody`/`ParseRawBody` (`response.go:512`, `:533`).

## Requirements

### Functional Requirements

- **REQ-F-001** — In `neo4j-cli/aura/internal/api/raw.go`, extract the body-rendering half of `rawErrorDetail` (`:181-196`) into a package-private helper so `response.go` reuses it verbatim:

  ```go
  // scrubbedBodyTrunc renders an upstream response body for embedding in an
  // error message: RedactText + StripControl, bounded twice. The generous outer
  // cut keeps redaction safe — a secret split by it is dropped by the final one.
  // Returns "" when the body has no printable content.
  func scrubbedBodyTrunc(body []byte) string {
      raw := strings.TrimSpace(scrub(truncateBytes(string(body), rawErrorBodyLimit*16)))
      return strings.TrimSpace(truncateBytes(raw, rawErrorBodyLimit))
  }
  ```

  Refactor `rawErrorDetail` to call it. `rawErrorDetail`'s own output must be byte-identical to today — the existing `raw_status_test.go` assertions (truncation at 10KB and 500KB, multibyte-UTF-8 boundary, C0 stripping, body redaction) are the regression guard and must pass unchanged. Do not alter `truncateBytes` (`:200`) or `rawErrorBodyLimit` (`:117`).

- **REQ-F-002** — In `response.go`, add a fallback-message helper so the ~10 unparseable-body sites do not each repeat two lines:

  ```go
  // upstreamDetail describes a response whose body did not match ErrorResponse —
  // the normal case on v2beta1, where every documented 4xx/5xx except two billing
  // 400s declares a description with no content schema. The body always goes
  // through scrubbedBodyTrunc: the panic this replaced interpolated resBody raw,
  // so a secret echoed back by the API could reach stdout.
  func upstreamDetail(statusCode int, resBody []byte) string {
      if body := scrubbedBodyTrunc(resBody); body != "" {
          return fmt.Sprintf("upstream error [status %d]: %s", statusCode, body)
      }
      return fmt.Sprintf("upstream error [status %d] with no response body", statusCode)
  }
  ```

  The `upstream error [status N]` prefix is deliberate: `rawErrorDetail`'s `aura api request failed with status …` wording is wrong here, because the user ran e.g. `aura instance list`, not `aura api`.

- **REQ-F-003** — Rewrite `handleResponseError`'s status dispatch to this table. Branches where the body **does** parse as `ErrorResponse` keep their current message shape and metadata verbatim — `formatBracketedMessages` (`:31`), the `field: ` prefix on 400, `WithResource` + `suggestionForResource` on 404, `suggestionForPaymentRequired` on 402, `authSuggestion` on 401/403, the 429 `Retry-After` message and `RetryAfter` field. Only the failure arms and the missing statuses change:

  | Status | Body parses as `ErrorResponse` | Body empty / unparseable |
  |---|---|---|
  | 400 | validation (6) — unchanged | validation (6) — **was panic** |
  | 401 | auth (4) — unchanged (`formatAuthorizationError`) | auth (4) — already handled at `:541` |
  | 403 | auth (4) — unchanged | auth (4) + `authSuggestion` — **was panic** |
  | 402 | conflict (5) — unchanged | conflict (5) — was fatal (1) |
  | 404 | not_found (3) — unchanged | not_found (3) — was fatal (1) |
  | 405 | upstream (8) — unchanged | upstream (8) — was fatal (1) |
  | 409 | conflict (5) — unchanged | conflict (5) — was fatal (1) |
  | **413 / 415 / 422** | try `ErrorResponse` first, then fall back | validation (6) — **new**; 415 was panic, 413/422 hit `default:` |
  | 429 | rate_limited (7) — unchanged | n/a (header-driven, no body parse) |
  | 5xx (500/502/503/504) | upstream (8) — unchanged | upstream (8) — was fatal (1) |
  | **307 / other 3xx** | — | upstream (8) — **new**, was panic |
  | **unmodelled 4xx** | — | validation (6) — **new**, was panic |
  | **unmodelled 5xx / anything else** | — | upstream (8) — **new**, was panic |
  | body-read failure (`:83`) | — | upstream (8) — **was panic** |

  Implementation notes:
  - Give 413/415/422 a named `case` (even though the handling equals the unmodelled-4xx fallback) purely so a doc comment can name the spec operations that return them. The `default:` arm splits on class: `>=400 && <500` → `NewValidationError`, everything else → `NewUpstreamError`. This mirrors `RawStatusError` (`raw.go:167-174`) — mirror its comment about unmodelled transient 4xx (408, 425) being reported as permanent because class is the only available signal.
  - The 403 fallback must attach `authSuggestion` (`response.go:24`) to match the parseable-body path and `RawStatusError` (`raw.go:149`).
  - Every branch that currently does `json.Unmarshal(resBody, &errorResponse)` and then panics or returns `reportIssueFatal` instead calls `upstreamDetail(statusCode, resBody)` and returns its class's constructor.
  - The 404 fallback still calls `parseResourceFromRequest` + `WithResource` + `WithSuggestion` — the resource identity comes from the request URL, not the body, so it is available even when the body is unparseable.

- **REQ-F-004** — Route a **parseable body that yields no error messages** to the same fallback. `json.Unmarshal` into `ErrorResponse` succeeds for any valid JSON object, leaving `Errors == nil`; `formatBracketedMessages` (`response.go:31`) then renders the literal `[\n\t\n]` — an empty bracketed message with no information in it. Because every documented v2beta1 4xx/5xx declares no `content` schema, this path is common and getting commoner.

  In every branch that builds messages from `errorResponse.Errors`, treat "parsed but produced zero messages" identically to "did not parse": call `upstreamDetail(statusCode, resBody)` instead of `formatBracketedMessages(messages)`. Prefer a single guard over repeating the check at each site, e.g.:

  ```go
  // errorMessages extracts the upstream messages, returning nil when the body
  // did not parse OR parsed to an empty errors[] — any valid JSON object
  // unmarshals into ErrorResponse, and rendering that as "[\n\t\n]" tells the
  // user nothing. Both cases fall back to the raw body via upstreamDetail.
  // withField prefixes "<field>: " (the 400 branch's shape); others pass false.
  func errorMessages(resBody []byte, withField bool) []string
  ```

  This narrows REQ-NF-002: message text is frozen only where at least one `Errors` entry exists. The empty-`Errors` case changes from `[\n\t\n]` to `upstream error [status N]: <body>`, which is the intent. It also means a 200-with-empty-`errors[]` is unaffected — `extractEmbeddedErrors` (`response.go:41`) already returns nil for that and is not touched.

  Add explicit test cases: for each of 400/402/404/409/5xx, a body of `{}` and a body of `{"errors":[]}`, asserting the status-class exit code and that the message contains neither `[` + tab + `]` nor an empty bracket pair. The 404 case must still carry `ResourceType`/`ResourceID`/`Suggestion`, since those come from the request URL.

- **REQ-F-005** — Delete `reportIssueFatal` (`response.go:70-77`). `grep -rn reportIssueFatal --include='*.go' .` confirms it has callers only inside `handleResponseError`. Do **not** remove the `os` or `clievents` imports — `formatAuthorizationError` (`:541`, `:549`) still uses both.

- **REQ-F-006** — After the change, `grep -n panic neo4j-cli/aura/internal/api/response.go` must return exactly two hits: `ParseBody` (currently `:512`) and `ParseRawBody` (currently `:533`). Both are out of scope.

- **REQ-F-007** — Rewrite `TestHandleResponseError_RedactsSecretArgs` (`response_test.go:27-107`) as `TestHandleResponseError_RedactsBodySecrets`. Its premise (recover a panic; assert `os.Args` was redacted in the panic text) no longer exists, but its security intent must be preserved: the secret now arrives in the **response body**. Keep the same statuses (415, 307, 400-malformed, 599) plus 413 and 422. Assert the returned error's message contains `***` and does **not** contain the raw secret value. Model it on `TestRawStatusError_BodyRedacted` (`raw_status_test.go:189`). Remove the `os.Args` save/restore and the `defer recover()` machinery.

- **REQ-F-008** — Update `TestHandleResponseError_ExitCodeMapping` (`response_test.go:113`):

  Retarget three entries and drop their now-wrong `wantMsgContain: "unexpected error [status N]"`:
  - `402 with malformed body`: `wantCode` 1 → **5**
  - `404 with malformed body`: `wantCode` 1 → **3**
  - `502 with malformed body`: `wantCode` 1 → **8**

  Add entries: `400` malformed → 6 · `403` malformed → 4 (assert `wantSuggestion` = `authSuggestion`) · `307` → 8 · `413` schema-less (empty body) → 6 · `413` with parseable `errors[]` → 6 · `415` schema-less → 6 · `422` with parseable `errors[]` → 6 (assert the upstream message text is surfaced) · `422` schema-less plain-text body → 6 · `405` malformed → 8 · `409` malformed → 5 · unmodelled `599` → 8 · unmodelled `451` → 6.

  The 413/415/422 schema-less cases are load-bearing: that is the shape the spec actually documents.

- **REQ-F-009** — Add `TestHandleResponseError_NoPanic` to `response_test.go`. Loop a broad status list — every status in REQ-F-003's table plus unmapped samples (302, 408, 418, 451, 507, 599) — crossed with a few body shapes (`""`, `plain text`, `<html>…</html>`, valid `{"errors":[…]}`) and assert for each: no panic, a non-nil error, and `errors.As` extracts a `*clierr.CLIError` with a `Code` in `clierr.Codes`. This is the invariant CLI-227 is about; it must fail loudly if a `panic` arm is reintroduced.

- **REQ-F-010** — Extend `test/e2e/exitcodes/exitcodes_test.go` (build tag `e2e_exitcodes`) with process-level scenarios proving the exit code, since the exit-0 symptom lives in `main.go`'s recover and no unit test can observe it. Add to the `scenarios` slice (`:313`), reusing the existing `fixtureServer` / `runCLI` / `scopeFlags` helpers:
  - `unprocessable_422_exit_6` — status 422, schema-less body, `wantExit: 6`
  - `unsupported_media_415_exit_6` — status 415, empty body, `wantExit: 6`
  - `payload_too_large_413_exit_6` — status 413, empty body, `wantExit: 6`
  - `permanent_redirect_307_exit_8` — status 307, empty body, `wantExit: 8`

  Each must assert `wantStderrContains` does **not** hold the phrase `please report an issue`; add a `wantStderrOmits` field to the `scenario` struct (`:278`) if no equivalent exists. Update the package doc comment (`:6-30`) which currently lists only 0/2/3/4/5/6/7/8 by their original triggers.

  Note for the 307 scenario: `fixtureServer` writes no `Location` header, so Go's `http.Client` surfaces the 307 to the caller rather than following it (`resp.Location()` returns `ErrNoLocation` and the client returns the response as-is). Verify this holds when implementing; if the client's behaviour differs, assert on the observed status instead of forcing 307 through.

- **REQ-F-011** — Add a changie entry. The exit-code change is user-facing. Non-interactive:

  ```
  changie new --projects neo4j-cli --kind Patch --body 'Aura commands now report the API'"'"'s own error message and a non-zero exit code for HTTP 307, 413, 415, 422 and other previously unhandled statuses, instead of a "please report an issue" message with a success exit code. A 4xx or 5xx whose body is empty or not JSON now also keeps its status-class exit code rather than reporting a generic internal failure.'
  ```

  Per `AGENTS.md`, the body must describe only the observable effect — no mention of `handleResponseError`, panics, or helper extraction. Verify the claim by diffing golden strings in `response_test.go` / the e2e suite, not by describing the implementation.

### Non-Functional Requirements

- **REQ-NF-001** — No upstream-controlled bytes reach an error message unscrubbed or unbounded. Grep the final diff for `resBody` / `string(resBody)` reaching a format string without passing through `scrubbedBodyTrunc`. The pre-existing leak at `response.go:200` (raw `resBody` in the panic) must be closed.

- **REQ-NF-002** — Zero behaviour change on any path where the body parses **and yields at least one `Errors` entry**. Message text, `ResourceType`/`ResourceID`, `Suggestion`, and `RetryAfter` must be byte-identical to today for 400/401/402/403/404/405/409/429/5xx. The unchanged assertions in `TestHandleResponseError_NotFound_TagsResource`, `TestSuggestionForResource`, `TestSuggestionForPaymentRequired`, and `TestParseResourceFromRequest` are the guard. The empty-`Errors` case is explicitly **excluded** from this freeze — REQ-F-004 changes it deliberately.

- **REQ-NF-003** — The `clierr.Codes` closed enum is untouched, so `neo4j-cli agent-context` needs no change and `TestErrorCodesInSyncWithClierr` (`agentcontext/build_test.go:116`) stays green without edits.

- **REQ-NF-004** — No skill-bundle regeneration. No cobra `Short`/`Long`/`Example`, flag surface, or `ValidFormatValues` changes, so `TestGenerator_RoundTrip` stays green. Confirm with `make generate-check` on a clean tree if in doubt.

- **REQ-NF-005** — `make test`, `make fmt-check`, `make lint` all pass. All `.go` files keep the Neo4j copyright header.

## Technical Considerations

**Reuse over reinvention.** The non-panicking sibling already exists in the same package: `MakeRawRequest` / `RawStatusError` / `rawErrorDetail` / `truncateBytes` in `raw.go`, built for CLI-225's `aura api` passthrough. `RawStatusError`'s doc comment (`raw.go:119-134`) already states the exact rationale CLI-227 asks for — *"Statuses the v2beta1 spec documents but the CLI never modelled (413, 415, 422, …) fall back rather than panicking"*. This work converges `handleResponseError` onto that contract and shares one helper; it must not introduce a third mechanism.

**Where the two paths deliberately differ.** `RawStatusError` parses no schema at all and always renders `aura api request failed with status N: <body>`. `handleResponseError` keeps its structured rendering when `ErrorResponse` parses, because v1/v1beta5 endpoints still return that shape and its messages (plus the 404 resource tagging and 402 quota suggestion) are strictly more useful. The convergence is on **exit codes and body scrubbing**, not on message text.

**Why the fallback is the primary path now.** With every documented v2beta1 4xx/5xx declaring no `content` schema, `json.Unmarshal(resBody, &ErrorResponse{})` increasingly *succeeds but yields an empty `Errors` slice* — any valid JSON object unmarshals into it. So "parse failed" is the wrong discriminator; "produced no messages" is the right one. REQ-F-004 makes both route to `upstreamDetail`, which is why the fallback, not the structured path, is the one that has to be right. Note the asymmetry with `extractEmbeddedErrors` (`response.go:41`), which already returns nil for an empty `Errors` on the **2xx** path and stays untouched — there, no messages means the happy path, not an error.

**Test-package layout.** `neo4j-cli/aura/internal/api/response_test.go` is the **internal** `package api` (it calls unexported `handleResponseError`), while `api_test.go` is the **external** `package api_test`. Keep new `handleResponseError` tests in the internal file. Per `AGENTS.md`, prefer table-driven; the anti-monolith rule permits a concern-named file, so if `response_test.go` grows unwieldy a `response_status_test.go` for the status matrix is acceptable and precedented.

**Existing tests that pin the old behaviour.** Only `response_test.go` matches on the old message strings — `grep -rn 'unexpected status code'` across `*_test.go` returns one hit (`:67`), and `main_test.go:215` constructs its own `NewFatalError` to feed `recoverPanic`, so it compiles unaffected.

**e2e suite is not wired into CI.** `grep -rn e2e_exitcodes Makefile .github/workflows/` returns nothing, so REQ-F-009's scenarios only run when invoked explicitly with `-tags=e2e_exitcodes`. Do not treat their absence from `make test` as a failure; run them manually as part of verification. Wiring the suite into CI is out of scope but worth noting to the user.

## Acceptance Criteria

- [ ] `grep -n panic neo4j-cli/aura/internal/api/response.go` returns exactly two hits (`ParseBody`, `ParseRawBody`).
- [ ] `reportIssueFatal` no longer exists; `os` and `clievents` imports are retained for `formatAuthorizationError`.
- [ ] `scrubbedBodyTrunc` exists in `raw.go` and is called by both `rawErrorDetail` and `response.go`'s fallback helper; `rawErrorDetail` output is unchanged.
- [ ] `TestHandleResponseError_NoPanic` passes across the full status × body-shape matrix.
- [ ] `TestHandleResponseError_ExitCodeMapping` covers 307, 413, 415, 422, unmodelled 4xx, unmodelled 5xx, and a malformed-body variant for each of 400/402/403/404/405/409/5xx.
- [ ] An empty-`Errors` body (`{}` and `{"errors":[]}`) on each of 400/402/404/409/5xx returns the status-class exit code and a message carrying the raw body — never the empty bracketed `[\n\t\n]`. A 404 in this case still carries `ResourceType`/`ResourceID`/`Suggestion`.
- [ ] `TestHandleResponseError_RedactsBodySecrets` passes; no test asserts a panic from `handleResponseError`.
- [ ] `TestHandleResponseError_NotFound_TagsResource`, `TestParseResourceFromRequest`, `TestSuggestionForResource`, `TestSuggestionForPaymentRequired` pass **unmodified**.
- [ ] `go test -tags=e2e_exitcodes ./test/e2e/exitcodes/...` passes, with 422/415/413 exiting 6 and 307 exiting 8, and no `please report an issue` on stderr.
- [ ] A changie entry exists under `.changes/unreleased/` describing only the observable effect.
- [ ] `make test`, `make fmt-check`, `make lint` all pass.

## Out of Scope

- `graphql/corspolicy/allowedorigin/utils.go:44` — identical panic on any non-200. File a separate Linear issue.
- Modelling `BillingErrorResponse` / `GDSError` / `InvokeAgentError`; widening the `Error` struct.
- Any change to `RawStatusError`'s exit codes or message format.
- Any change to `recoverPanic` / `main.go` / `exitCodeFor`.
- The `405 → upstream (8)` mapping.
- Remaining panics in `api.go` and `ParseBody`/`ParseRawBody`.
- Wiring `test/e2e/exitcodes/` into CI or the Makefile.

## Open Questions

**Resolved:**

- ~~Empty `Errors` slice on a parseable body~~ — **decided: route it to `upstreamDetail`.** Now specified as REQ-F-004, with REQ-NF-002 narrowed to exclude it.
- ~~Wiring `test/e2e/exitcodes/` into CI~~ — **decided: out of scope.** REQ-F-010's scenarios run only under an explicit `-tags=e2e_exitcodes`; run them by hand during verification.

**Open, to settle during implementation (not blocking):**

1. **307 through Go's `http.Client`** — REQ-F-010 assumes a `Location`-less 307 reaches `handleResponseError` rather than being followed by the client. Check empirically; if the client behaves otherwise, assert on the observed status in the e2e scenario instead of forcing 307 through. The unit-level 307 case in REQ-F-008 is unaffected either way, since it builds an `*http.Response` directly.
