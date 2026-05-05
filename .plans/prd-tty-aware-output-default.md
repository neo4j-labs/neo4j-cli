# PRD: TTY-aware output default for `query` and `:schema`

## Overview

When `neo4j-cli query` (and its `:schema` subcommand) runs without an explicit `--output` flag, the CLI currently emits a `go-pretty` table with box-drawing characters and uppercased headers. That format is fine for humans but hostile to `jq`, `grep`, file redirects, and shell scripts — users have to remember `--output json` every time they pipe.

This feature adds TTY-aware defaulting inside the `query` package: when stdout is a terminal the renderer keeps the table; when stdout is a pipe, redirect, or `/dev/null` the renderer emits JSON. An explicit `--output table|json` always wins.

Scope is the `query` package only (`renderRows` in `output.go`, `renderSchema` in `schema.go`). Aura subcommand renderers are explicitly out of scope.

## Goals

- Eliminate the `--output json` boilerplate for pipe/redirect use of `neo4j-cli query`.
- Apply the same TTY-aware default to `:schema` so its output adapts to context too.
- Preserve the existing interactive table experience for users running on a real terminal with no flags.
- Keep changes contained to one Go package; zero churn in aura code paths.
- Provide a deterministic test seam so existing assertions stay green and new behaviour is unit-testable without a real terminal.

## Non-Goals

- Aura subcommand renderers (`aura/internal/output/output.go`) — left untouched.
- The `aura config print` JSON-only path — left untouched.
- Any persisted-config behaviour or interaction with `aura config set output` — out of scope.
- `NO_COLOR` / `CI` / other environment-variable heuristics — **hard non-goal**. Detection is strictly TTY-based; users in unusual environments pass `--output` explicitly.
- Stderr or interactive-prompt detection (existing `stdinIsTTY` seam covers password prompting; not affected).
- New end-to-end / docker-backed smoke tests for this feature.

## Requirements

### Functional Requirements

- **REQ-F-001**: Add a package-private `resolveOutput(cmd, cfg)` helper in the `query` package that returns `"json"` or `"table"`. When the underlying `cfg.Aura.Output()` value is `"default"` or `""`, the helper resolves it via TTY detection on `cmd.OutOrStdout()`. Any other value passes through unchanged.
- **REQ-F-002**: Add a package-level seam `var stdoutIsTerminal = func(io.Writer) bool { ... }` mirroring the existing `stdinIsTTY` pattern at `neo4j-cli/query/run.go:34`. Production: assert the writer is `*os.File` and call `term.IsTerminal(int(f.Fd()))`. Non-`*os.File` returns `false`.
- **REQ-F-003**: `renderRows` (`neo4j-cli/query/output.go:35-46`) routes through `resolveOutput` instead of reading `cfg.Aura.Output()` directly.
- **REQ-F-004**: `renderSchema` (`neo4j-cli/query/schema.go:270-276`) routes through `resolveOutput`. Update its doc comment to describe the new behaviour: "default → table when stdout is a TTY, JSON when piped or redirected; explicit `--output X` overrides".
- **REQ-F-005**: Explicit `--output table` and `--output json` always win, regardless of TTY state.
- **REQ-F-006**: Update README's query section with one line: "When stdout is not a terminal (piped or redirected), `--output` defaults to `json`. Applies to both `query` and `:schema`."
- **REQ-F-007**: Add a changelog entry: `changie new --projects neo4j-cli --kind Minor --body "query/:schema: auto-detect piped/redirected stdout and default --output to json"`.

### Non-Functional Requirements

- **REQ-NF-001**: No new module dependencies. `golang.org/x/term` is already imported by `neo4j-cli/query/run.go:13`.
- **REQ-NF-002**: All existing query-package tests pass without per-test edits, with one targeted exception (REQ-NF-003).
- **REQ-NF-003**: A single new test helper (e.g. `neo4j-cli/query/testseam_test.go`) sets `stdoutIsTerminal` to return `true` via `TestMain` (or `init()`) for the entire query test package, so existing assertions on table output stay green.
- **REQ-NF-004**: Unit tests cover all four explicit/auto branches for `renderRows` (TTY+default, non-TTY+default, TTY+`--output json`, non-TTY+`--output table`) and the corresponding three branches for `:schema` (TTY+default → tables, non-TTY+default → JSON, explicit flags).
- **REQ-NF-005**: Existing `:schema` JSON-output test must be adjusted to either set `--output json` explicitly or flip the seam locally (today it relies on `"default"` falling through to JSON, which the resolver no longer does on TTY).
- **REQ-NF-006**: `make test`, `make fmt-check`, and `make lint` all pass before merge (per AGENTS.md gate).
- **REQ-NF-007**: No regressions in aura subcommand tests (zero aura code changed; verified by green CI).

## Technical Considerations

**Resolver placement.** The resolver lives in the `query` package, not in `common/clicfg`. That keeps `cfg.Aura.Output()` semantically pure (returns the configured string) and confines TTY coupling to the only consumer that needs it. If aura subcommands later adopt the same pattern, the resolver can be lifted to a shared package — but YAGNI for now.

**Detection target.** `cmd.OutOrStdout()` returns `io.Writer`. In production it's `os.Stdout` (set in `neo4j-cli/main.go:28` and `neo4j-cli/aura/cmd/main.go:30`). In tests it's a `*bytes.Buffer`. The `*os.File` type assertion + `term.IsTerminal(fd)` cleanly distinguishes the two without breaking the abstraction cobra provides.

**Test seam.** Mirrors `stdinIsTTY` at `neo4j-cli/query/run.go:34-36`. Defaulting the seam to `true` in test setup keeps every existing assertion that expects table output stable, while letting individual tests flip to `false` to exercise the JSON branch.

**`:schema` behaviour change.** Today `:schema` (`neo4j-cli/query/schema.go:270-276`) defaults to JSON regardless of TTY (only explicit `--output table` triggers the stacked-table render). After this change, an interactive `:schema` invocation defaults to tables; piped/redirected `:schema` keeps emitting JSON, matching today's piped behaviour. Called out as a deliberate UX improvement.

**Sentinel handling.** `cfg.Aura.Output()` returns a viper-resolved string. Default is `"default"` (set in `common/clicfg/clicfg.go:94`). The resolver treats both `"default"` and `""` as "auto-detect" so empty-string accidents are tolerated. `ValidOutputValues = [3]string{"default", "json", "table"}` constrains user-supplied values via flag validation, so we don't need to defend against arbitrary inputs.

**Files touched.**
- `neo4j-cli/query/output.go` — add `stdoutIsTerminal`, `resolveOutput`; route `renderRows` through.
- `neo4j-cli/query/schema.go` — route `renderSchema` through `resolveOutput`; update doc comment.
- `neo4j-cli/query/output_test.go` — new table-driven test for the four branches.
- `neo4j-cli/query/schema_test.go` — adjust the JSON test (explicit `--output json` or local seam flip); add a TTY-default-tables case.
- `neo4j-cli/query/testseam_test.go` (new, small) — `TestMain` sets the seam to true for the package.
- `README.md` — one-line note in query section.
- `.changes/unreleased/neo4j-cli-Minor-*.yaml` — changelog entry via `changie new`.

**Files explicitly NOT touched.** `common/clicfg/clicfg.go`, `neo4j-cli/aura/internal/output/output.go`, anything under `neo4j-cli/aura/internal/subcommands/`.

## Acceptance Criteria

- [ ] `bin/neo4j-cli query --conn ... "RETURN 1 AS n"` (interactive shell) → table output, unchanged from today.
- [ ] `bin/neo4j-cli query --conn ... "RETURN 1 AS n" | jq .` → JSON output (no `--output json` needed).
- [ ] `bin/neo4j-cli query --conn ... "RETURN 1 AS n" > out.txt` → `out.txt` contains JSON.
- [ ] `bin/neo4j-cli query --conn ... "RETURN 1 AS n" --output table | cat` → table (explicit flag wins).
- [ ] `bin/neo4j-cli query --conn ... "RETURN 1 AS n" --output json` (interactive) → JSON (explicit flag wins).
- [ ] `bin/neo4j-cli query :schema --conn ...` (interactive shell) → 5 stacked tables (new default for TTY; today defaults to JSON).
- [ ] `bin/neo4j-cli query :schema --conn ... | jq .` → JSON.
- [ ] `bin/neo4j-cli query :schema --conn ... --output json` (interactive) → JSON (explicit flag wins on TTY).
- [ ] `make test` green — including new TTY-detection unit tests for both `renderRows` and `renderSchema`.
- [ ] `make fmt-check` green (no gofmt drift).
- [ ] `make lint` green.
- [ ] `make license-check` green.
- [ ] No file under `neo4j-cli/aura/` is modified by this change.
- [ ] Changelog entry exists at `.changes/unreleased/neo4j-cli-Minor-*.yaml`.
- [ ] README query section mentions the TTY-aware behaviour.

## Out of Scope

- Aura subcommand renderers and `aura config print`.
- `NO_COLOR`, `CI`, `TERM=dumb`, or any other env-var-based detection.
- Persisted-config interactions (`aura config set output`).
- New end-to-end / docker-backed smoke tests.
- Stderr TTY detection (e.g. for log/warning routing).
- Refactoring `cfg.Aura.Output()` itself or moving the resolver into `common/clicfg`.
- Auto-color detection for the table renderer (separate concern).

## Open Questions

(none — all clarifying questions resolved during planning.)
