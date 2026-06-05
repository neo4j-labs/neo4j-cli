# PRD: CLI-210 — Default output table in interactive terminals (and TOON for agents)

## Overview

For read commands with no explicit `--format`, the default output in an
**interactive terminal** is currently **JSON instead of table**. This affects
every read command that relies on auto-detection (`docker list`,
`aura instance list`, `query`, `query :schema`, …).

Root cause: `common/output.ResolveOutput` (`common/output/output.go`) auto-detects
a TTY by type-asserting the command's writer as `*os.File`. CLI-109 (tee-on-failure,
commit `ca86b020`, PR #204) wraps stdout in `neo4j-cli/main.go` so a failing
command's output can be persisted:

```go
cmd.SetOut(io.MultiWriter(os.Stdout, buf))   // no longer an *os.File
```

`cmd.OutOrStdout()` now returns an `io.MultiWriter`, the `*os.File` assertion in
`StdoutIsTerminal` fails, it returns `false` unconditionally, and `ResolveOutput`
falls through to `"json"` for every interactive read command.

This PRD covers (a) fixing the regression so TTYs default to `table` again, and
(b) a related, intentional behaviour change: when the CLI is driven by a known AI
**agent harness**, default to **`toon`** (token-efficient) instead.

## Goals

- Restore `table` as the default output for interactive terminals, independent of
  any wrapping of `cmd`'s writer.
- Make default-format resolution robust against future wrapping/replacement of
  `os.Stdout`/`os.Stderr` (tee, capture, pager, color).
- Default to `toon` when an agent harness is detected and no `--format` is given.
- Add a regression test that would have caught CLI-109, plus a guardrail note.

## Non-Goals

- Changing behaviour when `--format` is set explicitly (it always wins).
- Changing color/styling resolution.
- Adding a PTY-backed e2e test (would require the `creack/pty` dependency that
  `test/e2e/writegate` deliberately avoided).
- Reworking the tee-on-failure capture mechanism itself.

## Requirements

### Functional Requirements

- REQ-F-001: `common/output.StdoutIsTerminal` becomes a **parameterless** seam
  `func() bool` that reads the real `os.Stdout` file descriptor directly
  (`term.IsTerminal(int(os.Stdout.Fd()))`), mirroring `common/flags.stdoutIsTerminal`
  and `neo4j-cli/query/run.go` `stdinIsTTY`. Wrapping `cmd`'s writer must not
  affect it.
- REQ-F-002: Add `common/output.IsAgent` seam, defaulting to `common/agent.Detect`.
- REQ-F-003: `ResolveOutput` default (non-explicit `--format`) precedence is:
  explicit flag (`json`/`table`/`toon`) → **agent → `toon`** → **TTY → `table`**
  → `json`. Agent wins over TTY.
- REQ-F-004: Update the only other non-test caller,
  `neo4j-cli/query/schema.go:340` (`printSchemaTables` H2-marker check), to call
  `StdoutIsTerminal()` with no argument.
- REQ-F-005: `ResolveOutput` keeps its `cmd *cobra.Command` parameter (now unused)
  to avoid editing its ~17 call sites; signature unchanged.
- REQ-F-006: Single changelog entry, kind **Minor**: "Fix default table output in
  interactive terminals; default to TOON for AI agents."

### Non-Functional Requirements

- REQ-NF-001: No new third-party dependencies (`common/agent` already exists; no
  import cycle — `agent` imports only `os`/`strings`/`term`).
- REQ-NF-002: All gates pass: `make test`, `make fmt-check`, `make lint`,
  `make generate-check`. Every `.go` file keeps the Neo4j license header.
- REQ-NF-003: Cross-platform (linux/macOS/windows) — detection uses the existing
  `term`/env seams already used repo-wide.

## Technical Considerations

- **Test determinism under an agent harness:** `agent.Detect()` reads env vars
  including `CLAUDECODE`, which **is set in the dev/CI-under-Claude environment**.
  Unguarded, every test exercising the *default* (non-explicit `--format`) branch
  would flip to `toon` and fail. Seed `IsAgent = func() bool { return false }` in:
  - `common/output/` — add a new `TestMain` (none exists today).
  - `neo4j-cli/query/testseam_test.go` — extend the existing `TestMain`.
  - `neo4j-cli/internal/subcommands/desktop/` (doctor_render_test) — per-test
    seed with cleanup, or a package `TestMain`.
  After implementing, run `make test` in the agent environment to surface any
  other package relying on default resolution; fix with the same seed.
- **Test stub migration:** all stubs change from `func(_ io.Writer) bool` →
  `func() bool`; remove now-unused `io` imports. Sites: `common/output/output_test.go`,
  `neo4j-cli/query/testseam_test.go`, `neo4j-cli/query/output_test.go`
  (`withStdoutIsTerminal` helper), `neo4j-cli/internal/subcommands/desktop/doctor_render_test.go`.
- **Generated docs:** default-format wording may live in the skill bundle /
  `--format` help / `agentcontext/build.go`. Run
  `go generate ./neo4j-cli/internal/skill/...` and `make generate-check`; commit
  any refreshed bundle in the same commit. Update `agentcontext/build.go` if it
  hand-codes a default format.
- **Guardrail (AGENTS.md):** add a note that any change wrapping/replacing
  `os.Stdout`/`os.Stderr` must be re-checked against TTY detection — the seams
  deliberately read the global FD, not `cmd.OutOrStdout()` — plus a one-line
  statement of the default-format precedence (agent→toon, TTY→table, else json).

## Acceptance Criteria

- [ ] `StdoutIsTerminal` is parameterless and reads `os.Stdout`; `ResolveOutput`
      and `printSchemaTables` call it with no arg.
- [ ] `IsAgent` seam added; `ResolveOutput` returns `toon` when `IsAgent()` and no
      explicit format, taking precedence over TTY.
- [ ] Wiring-guard regression test: build a command, apply
      `cmd.SetOut(io.MultiWriter(os.Stdout, &tee.LimitedBuffer{}))` (and SetErr),
      seam `StdoutIsTerminal=true`, `IsAgent=false`, format default → asserts
      `ResolveOutput == "table"`. (Fails on pre-fix code.)
- [ ] Positive agent test: `IsAgent()==true` + default format → `"toon"`.
- [ ] `IsAgent=false` seeded in the affected test packages; `make test` green in
      the agent environment.
- [ ] Manual: `neo4j-cli docker list` in a TTY → empty **table**; `… | cat` →
      **JSON**; `CLAUDECODE=1 neo4j-cli docker list` (even piped) → **TOON**;
      `query :schema` in a TTY shows `## Nodes` markers + tables, piped shows
      tables only.
- [ ] `make test`, `make fmt-check`, `make lint`, `make generate-check` all pass.
- [ ] Single **Minor** changie entry added.
- [ ] AGENTS.md guardrail + precedence note added.

## Out of Scope

- The `cfg.Global.StdoutIsTTY` startup-capture variant from the ticket — the
  parameterless seam achieves the same wrapping-immunity with less churn and
  matches the existing `flags`/`query` seam convention.
- PTY-backed e2e test.
- Color/styling resolution.

## Open Questions

None — approach (parameterless seam), agent precedence (agent→toon wins), test
scope (wiring guard only, no PTY), and changelog kind (single Minor) are all
confirmed.
