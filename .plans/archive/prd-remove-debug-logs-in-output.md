# PRD: Remove debug logs in output

Linear: https://linear.app/neo4j/issue/CLI-31
Branch: `oskar/cli-31-remove-the-debug-logs-in-the-output`

## Overview

Both shipped binaries (`neo4j-cli` and `aura-cli`) currently emit TODO-tagged debug lines to stdout on every invocation, including:

- `[<bin>] help displayed: <path>` on every `--help`
- `[<bin>] invalid command with args …: <err>` on cobra failure
- `[<bin>] command executed successfully with args …` after every successful run

These were placeholders for future telemetry but pollute stdout for end users — e.g. piping `neo4j-cli query` output into another tool captures the trailing `command executed successfully` line. This PRD removes the noise. Real metrics work is tracked separately and out of scope.

## Goals

- Stop the two binaries from writing the three TODO log lines to stdout.
- Keep all other behaviour identical: exit codes, cobra-emitted errors, panic/recover, run-hook traversal.
- Ship as a user-facing patch via the standard dual-project changie flow.

## Non-Goals

- No replacement telemetry / metrics implementation (separate future work).
- No changes to logging style anywhere else in the codebase.
- No refactor of `main.go` beyond what the removal makes dead.

## Requirements

### Functional Requirements

- REQ-F-001: `neo4j-cli/main.go` MUST NOT print the `[neo4j-cli] help displayed`, `[neo4j-cli] invalid command with args`, or `[neo4j-cli] command executed successfully with args` lines.
- REQ-F-002: `neo4j-cli/aura/cmd/main.go` MUST NOT print the equivalent three `[aura-cli]` lines.
- REQ-F-003: The custom `SetHelpFunc` wrappers in both binaries MUST be removed (their only purpose was the help log); default cobra help output is preserved.
- REQ-F-004: `cobra.EnableTraverseRunHooks = true` MUST remain in both binaries — it is unrelated to logging.
- REQ-F-005: The `panic`/`recover` block in both `main()` functions MUST remain unchanged.
- REQ-F-006: Cobra's own error message and the `os.Exit(1)` on failure MUST remain.
- REQ-F-007: One changie entry per binary MUST be added under `.changes/unreleased/`:
  - `project: aura-cli`, `kind: Patch`, body describing the removal
  - `project: neo4j-cli`, `kind: Patch`, body describing the removal

### Non-Functional Requirements

- REQ-NF-001: `make test` passes — no existing test asserts on the removed strings (verified via grep).
- REQ-NF-002: `make fmt-check` passes (gofmt clean).
- REQ-NF-003: `make lint` passes — in particular, no unused imports left after removal.
- REQ-NF-004: Copyright header preserved at top of both files.

## Technical Considerations

**Files touched** (only):

- `neo4j-cli/main.go`
- `neo4j-cli/aura/cmd/main.go`
- `.changes/unreleased/` — two new YAML entries

**Per-file deletions** (mirror in both files):

1. The `origHelp := cmd.HelpFunc(); cmd.SetHelpFunc(...)` block (5 lines).
2. The `[<bin>] invalid command with args …` `fmt.Printf` inside the `if err := cmd.Execute(); err != nil` branch.
3. The trailing `[<bin>] command executed successfully with args …` `fmt.Printf` at the end of `main`.
4. The 2-line `// cobra prints the error itself …` comment above `cmd.Execute()` — it explained the now-removed error log.

**Imports / retained code:**

- `github.com/spf13/cobra` import stays in both files because `cobra.EnableTraverseRunHooks = true` still uses it.
- `fmt` import stays in both files because the `panic`/`recover` block still uses `fmt.Printf`.
- `os` import stays (used by `os.Exit`, `os.Args`, `os.Stdout`, `os.Stderr`).

**Risks:** None material. The removals are localised to `main()`; no library code or tests depend on them. Cobra still owns error rendering and exit-code semantics.

**Conventions to follow** (from AGENTS.md):

- Copyright header at top of every `.go` file (preserved — we only delete inner lines).
- Run `make fmt-check` AND `make test` as the final gate (per AGENTS.md "always run … before marking any task complete").
- Dual-project changie entries because `neo4j-cli` bundles `aura-cli` (per AGENTS.md "Build System" notes).
- Hand-authored YAML format if `make changelog` interactive prompt is undesirable: `.changes/unreleased/<project>-Patch-<YYYYMMDD>-<HHMMSS>.yaml` with single-quoted body and RFC3339 time.

## Acceptance Criteria

- [ ] `grep -rn "command executed successfully\|help displayed\|invalid command with args" neo4j-cli/` returns no matches.
- [ ] `make build` succeeds; both `bin/neo4j-cli` and `bin/aura-cli` produced.
- [ ] `./bin/neo4j-cli --help` prints normal cobra help with no `[neo4j-cli] help displayed` line.
- [ ] `./bin/neo4j-cli aura instance list 2>/dev/null` does not contain a trailing `command executed successfully` line.
- [ ] `./bin/neo4j-cli bogus-cmd; echo $?` prints cobra's own error, exits 1, no `[neo4j-cli] invalid command` line on stdout or stderr.
- [ ] Same three runtime checks pass against `./bin/aura-cli`.
- [ ] `make test` passes.
- [ ] `make fmt-check` passes.
- [ ] `make lint` passes.
- [ ] Two new files under `.changes/unreleased/`: one for `aura-cli` (Patch), one for `neo4j-cli` (Patch).

## Out of Scope

- Real telemetry / metrics implementation to replace the removed logs (separate future ticket per the existing TODO comments).
- Logging-framework introduction (zerolog/slog/etc.).
- Any change to the `panic`/`recover` block, cobra error rendering, or exit-code behaviour.
- Refactor of `main.go` beyond the deletions enumerated above.

## Open Questions

None. Changelog kind confirmed as `Patch` for both entries.

## Addendum (post-review course correction)

Mid-PR refinement: rather than fully delete the help/fail/success log sites, leave the structural placeholders in place with a `// add metrics callback for <help|fail|success> here` comment so the future telemetry work has obvious hook points and doesn't have to re-derive the wiring (notably `SetHelpFunc(... origHelp(c, args) ...)`). User-visible behaviour is identical to the full-removal version (no stdout noise); the diff against pre-change `main` is smaller.

See task-005 for the implementation.
