# PRD: Structured JSON error envelope on `--format=json` (CLI-140)

## Overview

When a `neo4j-cli` command fails and the user has passed `--format=json`, the CLI must emit a structured `{"error": {...}}` envelope to stdout plus a one-line human summary to stderr. Today on the same failure stdout is empty and stderr carries cobra's default plaintext error — violating `agent-cli-auditor.md` §3.1 (stream contract), §3.3 (envelope shape), and §6.2 (output mode table). This PRD is the foundation slice from the [CLI-108 audit](https://linear.app/neo4j/issue/CLI-108) — it lands the envelope renderer and the `CLIError` field extensions; downstream tickets (CLI-143 suggestion content, CLI-141 / CLI-142 specific bug fixes) build on top of it.

Linear: <https://linear.app/neo4j/issue/CLI-140>

## Goals

- Emit a stable, machine-parseable error envelope on stdout when a command fails and `--format=json` is in effect.
- Keep plaintext (default / TTY / unrecognised format) behaviour stable enough that existing scripts and tests don't churn — the only intentional change is appending ` (exit <N>)` to the error line so the exit code is visible to humans.
- Extend `common/clierr.CLIError` with the fields the envelope needs (`CodeName`, `Suggestion`, `Retryable`, `ResourceType`, `ResourceID`), so follow-up tickets can populate them at the appropriate call sites without further type churn.
- Make the closed enum of machine error codes a single source of truth in `common/clierr` and rebuild `neo4j-cli/internal/subcommands/agentcontext` from it so drift is structurally impossible.

## Non-Goals

- Suggestion / next-action content on the existing error sites — covered by [CLI-143](https://linear.app/neo4j/issue/CLI-143).
- Fixing the unknown-subcommand exit-0 bug — covered by [CLI-141](https://linear.app/neo4j/issue/CLI-141).
- Fixing the `instance get` exit-0 + `[]string`-stringified message — covered by [CLI-142](https://linear.app/neo4j/issue/CLI-142).
- Adding a `documentation_url` field to the envelope — deliberately omitted in v1 per the audit decision (no docs URL pattern exists yet).
- List-response pagination envelope (§3.3) — separate, larger refactor.
- Changing success-path JSON output (`--format=json` on a successful command emits the existing body unchanged; we do not wrap it in `{"data": ..., "error": null}`).

## Requirements

### Functional Requirements

- **REQ-F-001** — `common/clierr/error.go` `CLIError` struct gains five new fields: `CodeName string`, `Retryable bool`, `ResourceType string`, `ResourceID string`, `Suggestion string`. The existing `Code`, `Message`, `Err`, `RetryAfter` fields are preserved.
- **REQ-F-002** — `clierr.New*Error` constructors return `*CLIError` (concrete pointer) rather than `error`. The pointer continues to satisfy `error`, so existing callers (e.g. `return clierr.NewNotFoundError(...)` from a function returning `error`) compile and behave identically.
- **REQ-F-003** — Each constructor populates `CodeName` and `Retryable` automatically from the exit code:

  | Exit code | `CodeName` | `Retryable` |
  |---|---|---|
  | 1 | `fatal_error` | false |
  | 2 | `usage_error` | false |
  | 3 | `not_found` | false |
  | 4 | `auth_error` | false |
  | 5 | `conflict` | false |
  | 6 | `validation_error` | false |
  | 7 | `rate_limited` | true |
  | 8 | `upstream_error` | true |

- **REQ-F-004** — `*CLIError` exposes two fluent setters: `WithResource(resourceType, resourceID string) *CLIError` and `WithSuggestion(s string) *CLIError`. Each mutates the receiver in place and returns it.
- **REQ-F-005** — `common/clierr/render.go` (new file) exports:
  - `type Envelope struct { Error EnvelopeBody json:"error" }`
  - `type EnvelopeBody struct { Code string; ExitCode int; Message string; ResourceType string (omitempty); ResourceID string (omitempty); Suggestion string (omitempty); Retryable bool }`
  - `func (e *CLIError) BuildEnvelope() Envelope` — pure, no I/O.
  - `func Render(err error, stdout, stderr io.Writer, format string)` — writes the envelope + summary to the appropriate streams based on `format`.
- **REQ-F-006** — `Render` behaviour:
  - When `format == "json"` and `err` is a `*CLIError` (directly or via `errors.As`): marshal `BuildEnvelope()` to stdout with a trailing newline; write `Error: <message> (exit <N>)\n` to stderr.
  - When `format` is `""` / `"default"` / `"table"` / `"toon"` / any other value: write `Error: <message> (exit <N>)\n` to stderr; if `Suggestion` is set, write `<suggestion>\n` to stderr as a second line; stdout is untouched.
  - When `err` is non-nil but not a `*CLIError`: wrap as if it came from `NewFatalError` (exit 1, `code: "fatal_error"`, `retryable: false`) and apply the rules above.
  - When `err` is nil: no-op.
- **REQ-F-007** — In `neo4j-cli/app/app.go`, the root cobra command sets `cmd.SilenceErrors = true` so cobra's default stderr error print is suppressed. `cmd.SilenceUsage` continues to be managed by the existing `silenceUsageOnError` hook in `common/flags/`.
- **REQ-F-008** — `neo4j-cli/main.go` calls `clierr.Render(err, os.Stdout, os.Stderr, cfg.Global.Format())` between the existing `cfg.Events.Flush()` (line 67) and `os.Exit(...)` (line 70). The existing `exitCodeFor(err)` exit-code routing is unchanged.
- **REQ-F-009** — `clierr` exports a canonical closed-enum table (e.g. `clierr.CodeNames map[int]string` + `clierr.CodeDescriptions map[int]string`, or a single `map[int]struct{Name, Description string}`) covering exit codes 1–8. The corresponding `errorCodes` map in `neo4j-cli/internal/subcommands/agentcontext/build.go:62-71` is deleted and rebuilt from the `clierr` export at package init. If a circular-import surfaces, fall back to duplicating the table in `agentcontext` + a `TestErrorCodesInSync` test in `clierr_test.go` that imports `agentcontext` (only test-side imports cross the boundary).
- **REQ-F-010** — `neo4j-cli/aura/internal/api/response.go` 404 branch (`response.go:79-91`) chains `.WithResource(resourceType, resourceID)` onto the `NewNotFoundError(...)` return. The implementation parses `res.Request.URL.Path` into segments, walks past the `v1` (or equivalent) version segment, and uses the next segment as `resourceType` (singularised — e.g. `instances` → `instance`) and the segment after that as `resourceID`. Unrecognised paths leave the fields empty. No other error sites in `response.go` are touched in this PR.

### Non-Functional Requirements

- **REQ-NF-001** — Envelope JSON shape is a public contract. Field names use `snake_case`; once shipped, fields are never renamed or retyped (auditor §3.5). Optional fields use `omitempty`. `retryable` is always present.
- **REQ-NF-002** — Plaintext stderr summary remains a single line per error (plus an optional second `<suggestion>` line in plaintext mode). No multi-line stack traces unless `--debug` (out of scope here; existing behaviour preserved).
- **REQ-NF-003** — Render path adds no measurable latency to the failure case (target: under 5ms wall-clock difference vs the current cobra default print).
- **REQ-NF-004** — Backwards compatibility: callers that today write `return clierr.NewXError(...)` from functions returning `error` must continue to compile and pass tests with no source change. The constructor signature change from `error` to `*CLIError` is the only API churn; verified by running the full `make test` suite.

## Technical Considerations

### Architecture

- `common/clierr` becomes the home of: the error type, the constructors, the closed-enum metadata table, the envelope types, and the renderer. Single package, no new dependencies.
- `neo4j-cli/main.go` is the only entrypoint that needs to call `clierr.Render`. Other binaries built from this repo (if any standalone trees re-emerge) should mirror the pattern.
- `neo4j-cli/internal/subcommands/agentcontext` becomes a consumer of `clierr` for the `error_codes` map. No new import cycle is expected — `agentcontext` already imports `cobra` and `clicfg`, and `clierr` is a leaf package with no internal deps.

### Stream contract (auditor §3.1)

| Mode | Failure → stdout | Failure → stderr |
|---|---|---|
| `--format=json` | Envelope JSON + `\n` | `Error: <message> (exit <N>)\n` |
| `--format=default` / `table` / `toon` / unset | (empty) | `Error: <message> (exit <N>)\n` + optional `<suggestion>\n` |

### Format detection edge cases

- `cfg.Global.Format()` returns `""` on a flag-parse error that fired before `PersistentPreRunE` could bind the format flag. `Render` treats `""` as plaintext — safe fallback. No attempt is made to recover the intended format from raw `os.Args`.
- Cobra's `FlagErrorFunc` (already installed in `app.go:49-51`) wraps flag-parse errors as `clierr.NewUsageError`. With `SilenceErrors = true`, cobra no longer prints the wrapped message — `Render` does, in the appropriate format.
- Flag-parse errors today trigger cobra's usage print (managed by `silenceUsageOnError` in `common/flags/`). That continues — usage on stderr after the error line is acceptable and is part of "what to do next" for malformed invocations.

### Test seams

- `clierr.Render` takes `io.Writer` for stdout/stderr and a `string` for format → unit-testable with `bytes.Buffer`.
- `(*CLIError).BuildEnvelope()` is pure → golden-testable without I/O.
- The existing `exitCodeFor` extraction in `main.go:30-39` is the model — same pattern for the new render call.

### Known intentional behaviour change

- Plaintext error line shape moves from `Error: <message>\n` (cobra default) to `Error: <message> (exit <N>)\n`. This is a small but visible change for humans and any shell scripts grepping stderr. Justification: the auditor §6.2 table calls for "Human-readable, single line, suggestion on next line"; surfacing the exit code on the same line is consistent with `gh` and `kubectl` conventions and gives operators the most useful piece of failure metadata without changing structure. Existing e2e exit-code tests assert exit codes, not stderr exact strings, so the blast radius is small. Any tests that did assert the exact stderr string will be updated as part of this PR.

### Risks / mitigations

- **Risk:** changing constructor return types from `error` to `*CLIError` breaks an obscure caller that relies on the interface return.
  - **Mitigation:** Go's assignability rules make `*CLIError → error` conversion implicit at every return site; the change is source-compatible. `make test` is the gate.
- **Risk:** `errors.As` users in the existing codebase expect the chain shape unchanged.
  - **Mitigation:** the internal chain (`Err` field, `Unwrap`) is untouched. New fields are additive.
- **Risk:** circular import between `clierr` and `agentcontext` when moving the closed enum.
  - **Mitigation:** `clierr` has zero internal imports today; `agentcontext` consumes `clierr`. The direction is acyclic. If anything breaks, fall back to the duplicate + sync test path (REQ-F-009).
- **Risk:** the `WithResource` URL-parse heuristic mis-tags a path (e.g. nested sub-resources like `/v1/instances/{id}/snapshots/{sid}`).
  - **Mitigation:** the implementation uses a known short list of Aura resource paths via a switch; unknown paths leave the fields empty (`omitempty`). Specific multi-segment paths can be added in a follow-up without API change.

## Acceptance Criteria

- [ ] `common/clierr/error.go` exports `*CLIError` with the five new fields; all eight constructors return `*CLIError`; `WithResource` and `WithSuggestion` are chainable.
- [ ] `common/clierr/render.go` exists with `Envelope`, `EnvelopeBody`, `BuildEnvelope`, and `Render` as described in REQ-F-005 / REQ-F-006.
- [ ] `common/clierr/error_test.go` (extend or create) has table tests asserting `Code` + `CodeName` + `Retryable` for each constructor and the fluent setters' behaviour.
- [ ] `common/clierr/render_test.go` (new) has table-driven golden tests covering every `CodeName` in JSON mode, plaintext mode (with and without suggestion), the untyped-error fallback, and the empty-format fallback.
- [ ] `neo4j-cli/app/app.go` sets `cmd.SilenceErrors = true` on the root command.
- [ ] `neo4j-cli/main.go` invokes `clierr.Render(err, os.Stdout, os.Stderr, cfg.Global.Format())` between `cfg.Events.Flush()` and `os.Exit(exitCodeFor(err))`.
- [ ] `neo4j-cli/main_test.go` has at least one test case driving `clierr.Render` with `bytes.Buffer` for stdout + stderr, asserting exact bytes for one JSON-mode case and one plaintext-mode case.
- [ ] `clierr` exports the canonical closed-enum table. `neo4j-cli/internal/subcommands/agentcontext/build.go` no longer duplicates it; its existing `errorCodes` map is rebuilt from `clierr` at init time (or, if the import fails, retains the duplicate plus a `TestErrorCodesInSync` gate).
- [ ] `agent-context` JSON output continues to include `error_codes` with the same eight entries and (functionally) the same descriptions. Existing `agentcontext_test.go` assertions pass with at most cosmetic updates.
- [ ] `neo4j-cli/aura/internal/api/response.go` 404 branch chains `.WithResource(resourceType, resourceID)` from the parsed request path; an existing or new test in the package exercises a `/v1/instances/{id}` 404 and asserts the resulting `*CLIError` has the expected `ResourceType`/`ResourceID`.
- [ ] `test/e2e/exitcodes/exitcodes_test.go` adds at least one case asserting end-to-end: `aura instance list --bad-flag --format=json` → exit 2, stdout parses as JSON, envelope has `error.code == "usage_error"` and `error.exit_code == 2`. If feasible, a second case exercises a 404 against the suite's fixture HTTP server and asserts `error.code == "not_found"`, `error.resource_type == "instance"`.
- [ ] A changelog entry under `.changes/unreleased/` of kind `Minor` documents the new JSON error envelope (`make changelog --kind Minor --body "Emit structured JSON error envelope when --format=json and a command fails (CLI-140)"`).
- [ ] `make fmt-check && make lint && make test` clean on a fresh checkout.
- [ ] `make generate-check` clean — confirms no skill-bundle drift. (Run `go generate ./neo4j-cli/internal/skill/...` and diff to verify; no bundle regen is expected because the bundle does not reference error code names today.)
- [ ] Manual smoke: rebuild via `make build`, then run `./bin/neo4j-cli aura instance list --bad-flag --format=json` — stdout contains a valid envelope with `code: "usage_error"`, `exit_code: 2`; stderr contains `Error: ... (exit 2)`; `$?` is 2.
- [ ] Manual smoke: `./bin/neo4j-cli aura instance list --bad-flag` — stderr has the existing plaintext shape with the new `(exit 2)` suffix; stdout empty; `$?` is 2.

## Out of Scope

- Populating `Suggestion` content on existing error sites — [CLI-143](https://linear.app/neo4j/issue/CLI-143).
- Fixing unknown-subcommand exit 0 — [CLI-141](https://linear.app/neo4j/issue/CLI-141).
- Fixing `instance get` exit 0 + `[]string` formatting — [CLI-142](https://linear.app/neo4j/issue/CLI-142).
- `documentation_url` field on the envelope.
- List-response pagination envelope on success path.
- Skill bundle copy that teaches agents about the envelope shape — defer to a doc PR once content is settled.
- Updating `agent-cli-auditor.md` self-references in the bundle — separate doc PR.

## Open Questions

None — all three "small calls" were resolved during plan-mode review:

- `clierr.CodeNames` is exported and consumed by `agentcontext` (single source of truth).
- The 404 path in `response.go` seeds `.WithResource` so the new API ships exercised.
- The e2e `exitcodes_test.go` suite gains at least one `--format=json` case.
