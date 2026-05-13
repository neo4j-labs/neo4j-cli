# PRD: Differentiated exit codes (CLI-64 + CLI-84)

## Overview

Two related agent-reliability defects in `neo4j-cli`:

1. **CLI-64** — The shipped binary's `main` (`neo4j-cli/main.go:44-48`) treats `cmd.Execute()`'s error as telemetry-only and falls through to a normal return. Every `RunE` failure exits **0** at the process boundary. The standalone-aura `main` (`neo4j-cli/aura/cmd/main.go:51-54`) hard-codes `os.Exit(1)` and so always exits 1.
2. **CLI-84** — `common/clierr/error.go` defines three constructors (`NewUsageError`, `NewUpstreamError`, `NewFatalError`) that all return a plain `fmt.Errorf`. Even after the CLI-64 fix, every error would collapse to a single exit code. Agents/scripts can't tell `--bad-flag` from auth failure from rate-limit from transient 5xx.

Linear:
- https://linear.app/neo4j/issue/CLI-64/investigate-exit-codes
- https://linear.app/neo4j/issue/CLI-84/b3-exit-codes-are-undifferentiated-everything-exits-1-on-error

Audit `agent-cli-auditor.md` §4.1 defines the closed set the binary must emit; §4.3 says the `error.code` string (already exposed via `agent-context`) must mirror the process exit code 1:1. The current `errorCodes` map in `neo4j-cli/internal/subcommands/agentcontext/build.go:55-59` carries only three entries — also has to grow.

## Goals

- The shipped `neo4j-cli` binary returns a non-zero exit code whenever `cmd.Execute()` returns an error (closes CLI-64).
- Errors carry a typed exit code from the audit §4.1 closed set: 1 fatal, 2 usage, 3 not-found, 4 auth, 5 conflict, 6 validation, 7 rate-limit, 8 upstream.
- HTTP-status → exit-code mapping is centralised in `handleResponseError` (`neo4j-cli/aura/internal/api/response.go:35`).
- Unknown-flag and missing-arg errors (cobra's `FlagErrorFunc` path) map to exit 2.
- Cypher errors returned by the Bolt driver map to exit 6 (validation); Bolt transport failures map to exit 8 (upstream).
- The `agent-context` JSON envelope (`exit_codes`, `error_codes` maps) advertises the new closed set so agents can discover it.
- Two changie entries (one Patch for CLI-64, one Minor for CLI-84) ship in the same PR.
- The standalone aura entrypoint (`neo4j-cli/aura/cmd/main.go`) mirrors the same mapping logic even though it is compiled-but-not-shipped, to keep the two `main` packages consistent.

## Non-Goals

- Signal exits 130 (SIGINT), 137 (SIGKILL), 143 (SIGTERM), and 124 (self-inflicted timeout) — separate follow-up.
- Building the full structured JSON error envelope (audit §3.3). `CLIError.RetryAfter` is laid in as a struct field for future serialisation but no `--format json` envelope work happens here.
- Renaming or removing existing exit/error codes — purely additive, so `schemaVersion` stays at 1 (`build.go:33`).
- Differentiating Cypher syntax errors from Cypher runtime errors — both go to exit 6.
- Touching the `update` self-update subcommand's existing exit-code semantics beyond the call-site sweep (its current `NewUsageError` / `NewFatalError` choices stay).
- Re-wording any existing error message; constructor signatures stay `(msg string, a ...any)` except `NewRateLimitError` which adds a leading `retryAfter string` arg.
- Bumping the cobra dependency.

## Requirements

### Functional Requirements

- REQ-F-001: A new `CLIError` struct exists in `common/clierr/error.go` with fields `Code int`, `Message string`, `Err error`, `RetryAfter string`. It implements `Error() string` (returns `Message`) and `Unwrap() error` (returns `Err`).
- REQ-F-002: Eight constructors return `*CLIError` with the documented `Code`. The closed set:

  | Constructor          | Exit | error.code         | Status      |
  |----------------------|------|--------------------|-------------|
  | `NewFatalError`      | 1    | `fatal_error`      | existing    |
  | `NewUsageError`      | 2    | `usage_error`      | existing    |
  | `NewNotFoundError`   | 3    | `not_found`        | **new**     |
  | `NewAuthError`       | 4    | `auth_error`       | **new**     |
  | `NewConflictError`   | 5    | `conflict`         | **new**     |
  | `NewValidationError` | 6    | `validation_error` | **new**     |
  | `NewRateLimitError`  | 7    | `rate_limited`     | **new**     |
  | `NewUpstreamError`   | 8    | `upstream_error`   | existing    |

- REQ-F-003: `NewRateLimitError` signature is `func NewRateLimitError(retryAfter string, msg string, a ...any) error`. The `retryAfter` value lands on `CLIError.RetryAfter`; the human-readable message body still mentions the value (so terminal users see it).
- REQ-F-004: `handleResponseError` in `neo4j-cli/aura/internal/api/response.go` maps HTTP status to the new constructors:
  - 400 → `NewValidationError` (was `NewUpstreamError`)
  - 401 → `NewAuthError` via `formatAuthorizationError`
  - 403 → `NewAuthError` (both `serverError.Error != ""` branch and `formatAuthorizationError` branch)
  - 404 → `NewNotFoundError`
  - 405 → `NewUpstreamError` (unchanged; defensive)
  - 409 → `NewConflictError`
  - 429 → `NewRateLimitError` with `res.Header.Get("Retry-After")` on the struct field
  - 500/502/503/504 → `NewUpstreamError` (unchanged)
- REQ-F-005: `formatAuthorizationError` returns `NewAuthError` on both success and JSON-unmarshal-failure branches (was `NewUsageError`).
- REQ-F-006: `neo4j-cli/main.go` extracts the exit code from the returned error:
  ```go
  if err != nil {
      var ce *clierr.CLIError
      if errors.As(err, &ce) {
          os.Exit(ce.Code)
      }
      os.Exit(1)
  }
  ```
  `clievents.Emit` (success vs. failure tag) and `cfg.Events.Flush()` run before `os.Exit`.
- REQ-F-007: `neo4j-cli/aura/cmd/main.go` performs the same `errors.As` extraction in place of the existing `os.Exit(1)`.
- REQ-F-008: Both root commands set `FlagErrorFunc` to wrap cobra's flag-parse errors into `NewUsageError`. Sites: `neo4j-cli/app/app.go:31` (shipped tree) and `neo4j-cli/aura/aura.go` (standalone). Cobra walks up to root for `FlagErrorFunc`, so setting it once per tree covers every subcommand.
- REQ-F-009: `agentcontext.exitCodes` map in `neo4j-cli/internal/subcommands/agentcontext/build.go:47-50` is extended with entries 2–8 plus their descriptions. `agentcontext.errorCodes` (lines 55-59) is extended with `not_found`, `auth_error`, `conflict`, `validation_error`, `rate_limited`. Existing `usage_error`, `upstream_error`, `fatal_error` keys are unchanged. `schemaVersion` stays at 1.
- REQ-F-010: Cypher errors from the Bolt driver in `neo4j-cli/query/run.go` map to `NewValidationError` (exit 6). Bolt transport-level failures (connect refused, timeout) stay as `NewUpstreamError` (exit 8). Identify the exact call sites during implementation; the `query` package call-site count is small.
- REQ-F-011: Existing call-site sweep (100 calls across 28 files). Any `clierr.NewUpstreamError` that is actually a validation rejection switches to `NewValidationError`. Any `clierr.NewUsageError` that is actually an auth failure switches to `NewAuthError`. Token-flow sites in `neo4j-cli/aura/internal/api/token.go` need explicit review for auth reclassification.
- REQ-F-012: Two changie entries land in the same PR via `changie new --projects neo4j-cli --kind <Patch|Minor> --body <body>`:
  - **Patch** — "Fix `neo4j-cli` exiting 0 on command errors (CLI-64)."
  - **Minor** — "Differentiate exit codes by error category: usage (2), not-found (3), auth (4), conflict (5), validation (6), rate-limit (7), upstream (8). See `neo4j-cli agent-context` for the closed set. (CLI-84)"

### Non-Functional Requirements

- REQ-NF-001: All existing tests continue to pass. `make test`, `make fmt-check`, `make lint`, `make generate-check` are green.
- REQ-NF-002: `make generate-check` stays clean — bundle is not affected (no command-tree shape changes; `agent-context` reflects at runtime per AGENTS.md "Agent Context Notes").
- REQ-NF-003: New `errors.As` and `os.Exit` paths must run after `cfg.Events.Flush()` in both `main.go` files so telemetry is not dropped on error.
- REQ-NF-004: `recoverPanic` defers in both `main.go` files keep working unchanged — `os.Exit` only fires on the non-panic error path, so panic propagation (re-panic via runtime → non-zero exit) is preserved.
- REQ-NF-005: Existing wrappers of `clierr` errors via `fmt.Errorf("...: %w", inner)` continue to expose the typed code through `errors.As`. Constructors return `*CLIError` (pointer) so `errors.As(err, &ce)` works through arbitrary wrapping layers.
- REQ-NF-006: The Windows runner (CRLF risk) and the `make license-check` target need no new files added without the Neo4j copyright header.

## Technical Considerations

- **Cobra `FlagErrorFunc` propagation**: cobra's `FlagErrorFunc()` walks up the parent chain to find the first non-nil function. Setting it once on the root in `app.NewCmd` and once on the standalone aura root is sufficient — no per-subcommand registration.
- **`SilenceErrors` interaction**: existing behaviour leaves cobra's default error-printing on (cobra prints `Error: <msg>` itself before `Execute()` returns). With typed errors carrying `Error() string = Message`, the printed text stays the same. We do not switch `SilenceErrors = true`; that's a larger change tied to the JSON envelope work.
- **`os.Exit` vs deferred Flush**: `os.Exit` skips later defers, so `cfg.Events.Flush()` must run *inline* before `os.Exit`. The `recoverPanic` defer is the only deferred function and only fires on panic — leaving it intact is safe.
- **`agent-context` schema-version policy**: per `build.go:31-33` and AGENTS.md, adding documented enum entries is non-breaking. `schemaVersion` stays at 1. `agentcontext_test.go` has a "locked test" pattern; the fixture(s) extend to cover the new keys.
- **Cypher error categorisation**: `neo4j-cli/query/run.go` is the relevant call site. The Bolt driver returns errors with a `Neo4jError` type carrying a code. Implementation can either map by error type (`neo4j.IsRetryableError`, etc.) or by the textual code prefix `Neo.ClientError.*` → validation (6) vs `Neo.TransientError.*` / `Neo.DatabaseError.*` → upstream (8). Audit during implementation.
- **Backwards-compatibility risk**: scripts that today rely on "exit 1 means any error" continue to work (1 is still a valid error code). Scripts that test for specific exit codes will now see new codes 2-8 — this is a **behaviour change** users will see, hence the Minor changelog. Worth a single sentence in the release notes pointing at `agent-context`.
- **Skill-bundle drift**: no command-tree shape change, so no `references/<cmd>.md` regen. Only `TestGenerator_RoundTrip` gate to keep in mind if any command's `Long`/`Short` changes incidentally during the call-site sweep.

## Acceptance Criteria

- [ ] `*CLIError` type and 8 constructors exist in `common/clierr/error.go`; existing `NewUsageError`, `NewUpstreamError`, `NewFatalError` return the typed error with the correct code.
- [ ] `errors.As(err, &ce)` retrieves the `Code` through one or more `fmt.Errorf("...: %w", ce)` wrappings — proved by unit test in `common/clierr/error_test.go`.
- [ ] HTTP 400/401/403/404/409/429/5xx each surface the documented exit code via `handleResponseError` — proved by table-driven test in `neo4j-cli/aura/internal/api/response_test.go` (or equivalent).
- [ ] `./bin/neo4j-cli aura instance list --bad-flag; echo $?` prints `2`.
- [ ] `./bin/neo4j-cli aura instance get <nonexistent>` against a 404-returning mock prints `3`.
- [ ] `./bin/neo4j-cli aura instance list` with no/invalid token prints `4`.
- [ ] `./bin/neo4j-cli aura instance create` against a 429 mock with `Retry-After: 30` prints `7` AND the message body contains `30`.
- [ ] `./bin/neo4j-cli aura instance list` against a 503 mock prints `8`.
- [ ] `./bin/neo4j-cli aura instance list` happy path prints `0`.
- [ ] `./bin/neo4j-cli; echo $?` (no subcommand, prints help) prints `0`.
- [ ] `neo4j-cli agent-context | jq '.exit_codes, .error_codes'` shows the extended closed set.
- [ ] `neo4j-cli agent-context | jq '.schema_version'` is `1` (unchanged).
- [ ] `agentcontext_test.go` fixtures lock the new entries.
- [ ] New e2e package `test/e2e/exitcodes/` exercises every scenario in REQ-F-004 against a fixture HTTP server (or equivalent). Pattern mirrors `test/e2e/release_fixture/` build-tag setup.
- [ ] `make test`, `make fmt-check`, `make lint`, `make generate-check` all green.
- [ ] Two changie entries (`.changes/unreleased/neo4j-cli-Patch-*.yaml` and `neo4j-cli-Minor-*.yaml`) committed.
- [ ] PR description references both CLI-64 and CLI-84 and notes the behaviour change (new exit codes 2-8) for downstream scripts.

## Out of Scope

- Signal-related exits (124/130/137/143) — separate follow-up.
- Building the structured JSON error envelope (`{"error": {"code": ..., "exit_code": ..., "message": ...}}` per audit §3.3) for `--format json`. The `CLIError.RetryAfter` field is laid in so a future PR can serialise it without breaking the type.
- Splitting Cypher syntax errors from Cypher runtime errors — both stay at exit 6.
- Reworking `update` subcommand's existing exit semantics beyond reclassification in the call-site sweep.
- Reworking `clievents.Emit` payloads to include the exit code or `error.code` — telemetry stays exactly as-is.
- Adding new `--format` output for the error envelope.

## Open Questions

None — all decisions locked during planning:

- `NewRateLimitError` carries retry hint on a struct field (`RetryAfter`). ✓
- Cypher errors map to exit 6 (validation). ✓
- `schemaVersion` stays at 1. ✓
- Two changie entries (Patch + Minor). ✓
- Standalone aura `main` mirrors the fix. ✓
- Unit + e2e test coverage. ✓
