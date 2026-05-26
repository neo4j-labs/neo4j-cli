# PRD: CLI-146 — Better error message for Aura instance-type quota exceeded

Linear: https://linear.app/neo4j/issue/CLI-146/better-error-message-when-creating-second-aura-free-db
Builds on CLI-143 (CLIError.Suggestion pipeline now wired).

## Overview

`neo4j-cli aura instance create --rw --type free-db ...` when the user has hit their free-db quota currently prints the generic panic-recovery line:

```
Unexpected error running CLI with args ..., please report an issue in https://github.com/neo4j-labs/neo4j-cli
```

Live reproduction against Aura captured the real behaviour:
- HTTP status: **`402 Payment Required`**
- Body: `{"errors":[{"message":"User is not permitted to create any more instances of this type.","reason":"quota-exceeded"}]}`
- Code path: `neo4j-cli/aura/internal/api/response.go:138-140` `default:` panic → `neo4j-cli/main.go:22-24` `recoverPanic` discards the panic value → user sees generic line.

This card adds a typed 402 handler that returns `CLIError.Code == 5` (conflict) with an actionable type-agnostic suggestion when `e.Reason == "quota-exceeded"`, and hardens `recoverPanic` so any future unhandled status code is at least diagnosable from CLI output alone.

## Goals

- Replace the panic / "report an issue" output for 402 with a typed `CLIError` rendered through the existing CLI-143 pipeline (plaintext two-line, JSON envelope `suggestion`, toon envelope `suggestion`).
- For `reason: "quota-exceeded"` specifically, attach a suggestion that tells the user how to free up quota (`instance list` + `instance delete`) or sidestep the type.
- Harden `recoverPanic` so it surfaces the panic value's `Error()` line, so future unhandled status codes are diagnosable without a rebuild.

## Non-Goals

- Pre-flight instance-count / quota check before POST (race-prone; not what the card asks for).
- Generalising the suggestion to other 402 reasons (none observed beyond `quota-exceeded`).
- Changing panic semantics at `response.go` sites themselves; only the recovery line is hardened.
- Type-specific enumeration of paid types (`professional-db`, `enterprise-db`, ...) in the suggestion — kept generic to avoid drift if Aura renames types.

## Requirements

### Functional Requirements

- REQ-F-001: `aura/internal/api/response.go` `handleResponseError` adds `case http.StatusPaymentRequired:` alongside the existing 4xx branches. The case parses `resBody` as `ErrorResponse` and returns `clierr.NewConflictError("%s", messages)` (exit code 5). On `json.Unmarshal` failure, return `clierr.NewFatalError("unexpected error [status %d] running CLI with args %s, ...", ...)` mirroring the existing 404/405/409/5xx fallback.
- REQ-F-002: A private helper `suggestionForPaymentRequired(resp ErrorResponse) string` is added near the existing `suggestionForResource`. It iterates `resp.Errors`; if any entry has `Reason == "quota-exceeded"` it returns `"You've reached your quota for this instance type. Delete an existing instance with 'neo4j-cli aura instance list' then 'neo4j-cli aura instance delete <id>', or pick a different --type."`. Otherwise returns `""`.
- REQ-F-003: The 402 case attaches the suggestion via `.WithSuggestion(s)` only when `suggestionForPaymentRequired` returns non-empty. Empty fall-through means the API's own message remains the primary signal — strictly better than the current state.
- REQ-F-004: `neo4j-cli/main.go` `recoverPanic` is replaced with:
  ```go
  func recoverPanic(w io.Writer, args []string, r any) {
      if err, ok := r.(error); ok {
          fmt.Fprintf(w, "%s\n", err.Error()) //nolint:errcheck
      }
      fmt.Fprintf(w, "Unexpected error running CLI with args %s, please report an issue in https://github.com/neo4j-labs/neo4j-cli\n\n", clievents.RedactArgs(args)) //nolint:errcheck
  }
  ```
  - When `r` implements `error`, both lines print: the diagnostic line first, then the existing fallback line.
  - When `r` does not implement `error` (any non-error value), only the existing fallback line prints — unchanged behaviour.

### Non-Functional Requirements

- REQ-NF-001: Through-pipeline behaviour:
  - `--format=default` (plaintext): renders as `Error: [<API message>] (exit 5)\n<suggestion>\n` on stderr (per CLI-143 renderer).
  - `--format=json`: stdout envelope has `"code":"conflict"`, `"exit_code":5`, `"message":"[<API message>]"`, `"suggestion":"You've reached..."`.
  - `--format=toon`: toon envelope carries the same fields.
- REQ-NF-002: All gates clean: `make fmt-check`, `make lint`, `make test`, `make generate-check`.
- REQ-NF-003: All touched / new `.go` files start with the standard Neo4j copyright header.
- REQ-NF-004: Changelog entry under `.changes/unreleased/` via `changie new --projects neo4j-cli --kind Patch --body "..."` describing the user-facing change.

## Technical Considerations

### Architecture

The fix sits at two layers, each isolated:

1. **API-layer mapping (`response.go`)** — adds a new `case http.StatusPaymentRequired:` to the existing `handleResponseError` switch. Same shape as the 404 / 409 cases: parse `ErrorResponse`, build messages slice, construct typed `CLIError`, optionally chain `.WithSuggestion(...)`. No new types, no new helpers visible outside the file.

2. **Panic recovery hardening (`main.go`)** — single function body change, with a unit-tested seam already in place (`recoverPanic` was extracted from `main` specifically for testing — see existing tests in `main_test.go`).

### Exit code choice

`quota-exceeded` is closest to `conflict` (code 5: "request conflicts with current resource state") in the closed enum at `common/clierr/error.go:73-82`. Not `validation_error` (6) because the input itself is well-formed; not `usage_error` (2) because the user followed the documented usage; not `upstream_error` (8) because retry can't succeed (Retryable is false).

### Reusable utilities (already exist)

- `clierr.NewConflictError`, `NewFatalError` — `common/clierr/error.go`.
- `clierr.CLIError.WithSuggestion` (`common/clierr/error.go:53`) — mutating, returns receiver, chains cleanly.
- `ErrorResponse` / `Error` types — `aura/internal/api/response.go:21-29`.
- Existing render machinery (CLI-143) — `common/clierr/render.go` + tests at `common/clierr/render_test.go:232-444`.

### Body shape stability

`reason: "quota-exceeded"` is a stable enum on Aura's side (matches the v2beta1 error contract). Exact-string match in the helper is robust enough; the empty fall-through is the safety net if Aura adds other 402 reasons.

### `recoverPanic` test seam

`recoverPanic` already takes `w io.Writer` for testability. The existing test exercises the single-line path; we extend it to cover the new error-value branch and the unchanged non-error branch.

### No bundle / generate impact

No cobra command tree changes. `make generate-check` should pass with no `internal/skill/bundle/**` diffs.

### Files touched

- `neo4j-cli/aura/internal/api/response.go` — add `case http.StatusPaymentRequired:` + private `suggestionForPaymentRequired` helper.
- `neo4j-cli/aura/internal/api/response_test.go` — extend mapping table test + new helper table test.
- `neo4j-cli/main.go` — replace `recoverPanic` body.
- `neo4j-cli/main_test.go` — extend `recoverPanic` test.
- `.changes/unreleased/neo4j-cli-Patch-<timestamp>.yaml` — via `changie new`.

## Acceptance Criteria

- [ ] `response.go` `handleResponseError` returns a `*clierr.CLIError` for status 402 (no panic). For the canonical body `{"errors":[{"message":"User is not permitted to create any more instances of this type.","reason":"quota-exceeded"}]}` the returned error has `Code == 5` and `Suggestion == "You've reached your quota for this instance type. Delete an existing instance with 'neo4j-cli aura instance list' then 'neo4j-cli aura instance delete <id>', or pick a different --type."`.
- [ ] For a 402 body whose `errors[].reason` is anything other than `"quota-exceeded"` (e.g. `"something-else"`), the returned error has `Code == 5` and `Suggestion == ""`.
- [ ] For a 402 body that fails to unmarshal, the returned error is a `*clierr.CLIError` of `Code == 1` (fatal) with the standard "unexpected error [status %d] running CLI with args %s" message.
- [ ] `TestSuggestionForPaymentRequired` table-driven test covers: positive `quota-exceeded`; negative unknown reason; empty `errors[]`; multi-error body where one entry is `quota-exceeded` (positive).
- [ ] `recoverPanic` test asserts both lines are printed when `r` is a `*clierr.CLIError` (the original message + the existing fallback line). Asserts only the existing fallback line prints when `r` is a non-error (e.g. a string).
- [ ] Manual end-to-end against live Aura — `aura instance create --rw --type free-db` against a project at quota emits the second stderr line on plaintext and the `suggestion` envelope field on `--format json`; exit code is 5.
- [ ] Changelog entry under `.changes/unreleased/` (`Patch`).
- [ ] `make fmt-check && make lint && make test && make generate-check` all clean.

## Out of Scope

- Pre-flight quota check.
- Generalising 402 handling to other reasons (not observed beyond `quota-exceeded`).
- Changing the panic semantics at the `response.go` panic sites themselves; only the recovery surface is improved.
- Enumeration of specific paid instance types in the suggestion text.

## Open Questions

(none — all locked in plan-mode discussion + live reproduction)
