# PRD: Auto-detect Agent Harnesses; Skip `--rw` for Interactive Humans

Linear: [CLI-76](https://linear.app/neo4j/issue/CLI-76/auto-detect-agent-harnesses-and-skip-rw-requirement-if-in-tty-human)
Source plan: `/Users/oskarhane/.claude/plans/time-for-https-linear-app-neo4j-issue-cl-mighty-cocke.md`
Branch: `oskar/cli-76-auto-detect-agent-harnesses`

## Overview

`--rw` is the safety gate that prevents agents from silently mutating Aura, local config, credentials, or the graph. For humans typing commands in a terminal it's friction (`credential dbms add` is obviously a write). This change detects whether the CLI is being driven by a known agent harness via environment variables, and only enforces the `--rw` gate when an agent (or non-interactive script) is the caller. Interactive human invocations skip the gate. The agent-side skill prose is updated to teach agents the new flow: do not preemptively add `--rw`; if a command fails the gate, ask the user once to confirm the write, then re-run with `--rw`.

## Goals

- Drop the `--rw` requirement for humans running write commands in an interactive terminal — the common case becomes `neo4j-cli credential dbms add ...` (no flag).
- Keep the `--rw` gate intact for every known agent harness, so agents can't silently mutate state.
- Keep the gate intact for non-interactive non-agent contexts (CI, piped scripts, `nohup`) — safe default.
- Make agent detection accurate today and easy to update later by sourcing the env-var set from upstream `unjs/std-env`.
- Teach agents (via the skill bundle) the new flow: ask the user once for confirmation before re-running with `--rw`.
- Surface the new behaviour in README and the changelog so humans discover it from release notes.

## Non-Goals

- Escape-hatch env var (e.g. `NEO4J_CLI_FORCE_RW=1` / `NEO4J_CLI_ASSUME_TTY=1`). Confirmed out of scope.
- Auto-inferring `--rw` from command annotations alone (i.e. unconditionally allowing writes for `Annotations["write"] == "true"`). The gate still exists; it just stops firing for humans.
- Changing per-command `Annotations["write"]` markings or which commands count as writes. The `query run` EXPLAIN preflight stays unchanged.
- Removing `--rw` as a flag. It remains as the explicit opt-in; agents will continue to pass it.
- Changing the default `--format` heuristic or any other TTY-driven behaviour.
- Adding interactive in-CLI confirmation prompts. Confirmation lives on the agent side (skill prose), not in the CLI.
- Migrating any non-agent env-var detection (e.g. CI heuristics like `CI=true`). Out of scope; the existing rule "no TTY → require `--rw`" already covers CI.

## Requirements

### Functional Requirements

- **REQ-F-001:** New package `common/agent/` exporting a single function:
  ```go
  // Detect returns true if any known agent-harness env var is set.
  func Detect() bool
  ```
  Hosts the env-var list and the substring-match cases. Picked over inlining in `common/flags/` so other surfaces (e.g. `common/output/`, future telemetry) can reuse it.

- **REQ-F-002:** `common/agent/agent.go` covers the full upstream `unjs/std-env` agents list (verified 2026-05-13 against `https://raw.githubusercontent.com/unjs/std-env/main/src/agents.ts`):

  | Agent       | Detection signal                              |
  | ----------- | --------------------------------------------- |
  | Claude Code | `CLAUDECODE` or `CLAUDE_CODE` set & non-empty |
  | Replit      | `REPL_ID` set & non-empty                     |
  | Gemini CLI  | `GEMINI_CLI` set & non-empty                  |
  | Codex       | `CODEX_SANDBOX` or `CODEX_THREAD_ID` set      |
  | OpenCode    | `OPENCODE` set & non-empty                    |
  | Auggie      | `AUGMENT_AGENT` set & non-empty               |
  | Goose       | `GOOSE_PROVIDER` set & non-empty              |
  | Cursor      | `CURSOR_AGENT` set & non-empty                |
  | Devin       | `EDITOR` contains `devin` (case-insensitive)  |
  | Kiro        | `TERM_PROGRAM` contains `kiro`                |
  | pi          | `PATH` contains `.pi/agent`                   |

  Substring matches use `strings.Contains` after `strings.ToLower` on both sides. The internal env-var lookup goes through an overridable `getenv = os.Getenv` package-level seam so tests don't need `t.Setenv` mutation.

- **REQ-F-003:** `common/agent/agent_test.go` — table-driven test exercising each row of the table above. Each case toggles the relevant env var via the seam (mock `getenv`), asserts `Detect() == true`. Plus one negative case where no env vars set → `Detect() == false`.

- **REQ-F-004:** Extend `EnforceWriteGate` in `common/flags/flags.go:72` to:
  ```go
  func EnforceWriteGate(cmd *cobra.Command) error {
      if cmd.Annotations["write"] != "true" {
          return nil
      }
      if rwFlagSet(cmd) {
          return nil
      }
      if detectAgent() {
          return clierr.NewUsageError("this command writes; pass --rw to allow it")
      }
      if stdoutIsTerminal() {
          return nil
      }
      return clierr.NewUsageError("this command writes; pass --rw to allow it")
  }
  ```
  with two new overridable package-level seams:
  ```go
  var detectAgent       = agent.Detect
  var stdoutIsTerminal  = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
  ```
  Pattern mirrors `neo4j-cli/query/run.go:35`'s `stdinIsTTY` seam.

- **REQ-F-005:** Update `--rw` flag Usage in `common/flags/flags.go:42`:
  > `Allow write operations. Auto-applied in interactive terminals; required when running under an agent harness or non-interactive script.`

- **REQ-F-006:** Update `common/flags/flags_test.go` (or wherever `EnforceWriteGate` is tested) with a new table-driven case covering the gate matrix:

  | `--rw` set | agent detected | stdout TTY | expect          |
  | ---------- | -------------- | ---------- | --------------- |
  | true       | true           | true       | nil             |
  | true       | false          | false      | nil             |
  | false      | true           | true       | usage error     |
  | false      | true           | false      | usage error     |
  | false      | false          | true       | nil (NEW)       |
  | false      | false          | false      | usage error     |

  Toggle the two seams (`detectAgent`, `stdoutIsTerminal`) per row; defer-restore originals. Existing tests asserting the usage error keep passing because they ride the default seams (no TTY, no agent) under `go test`.

- **REQ-F-007:** Update `neo4j-cli/internal/skill/additions.md` (lines 11–12). Replace the current two `--rw` bullets with prose teaching the new flow:
  - State that the CLI auto-detects agent harnesses; agents always need `--rw` for writes.
  - Instruct: do NOT preemptively add `--rw`. If a write command fails with `this command writes; pass --rw to allow it`, ask the user once to confirm the write, then re-run with `--rw`.
  - Keep the existing pointer to `query run` EXPLAIN preflight for write-cypher detection.

- **REQ-F-008:** Update `neo4j-cli/aura/internal/skill/additions.md` with the same `--rw`-flow prose, kept in sync with REQ-F-007 (per AGENTS.md "lives in TWO places" sync rule).

- **REQ-F-009:** Run `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...` so `bundle/SKILL.md` on both surfaces reflects the new additions. Commit regenerated files in the same commit. `TestGenerator_RoundTrip` is the CI gate.

- **REQ-F-010:** Add a short README section (under the existing safety / `--rw` prose) summarising the new behaviour for human readers: "When `neo4j-cli` runs in an interactive terminal, `--rw` is auto-applied for write commands. Agents (Claude Code, Codex, Cursor, …) and non-interactive scripts still need `--rw` explicitly. Override list: `unjs/std-env`."

- **REQ-F-011:** Add a changelog entry:
  ```
  changie new --projects neo4j-cli --kind Minor \
    --body "Auto-detect agent harnesses; --rw no longer required when running interactively in a terminal (CLI-76)"
  ```

### Non-Functional Requirements

- **REQ-NF-001:** Zero new runtime dependencies. `common/agent/` uses only `os` + `strings` from stdlib.
- **REQ-NF-002:** No measurable cold-start cost. `agent.Detect()` is N `os.Getenv` lookups (≤13) per command invocation, executed in `PersistentPreRunE`.
- **REQ-NF-003:** Cross-platform: linux/darwin/windows. `os.Getenv` is portable; substring matches are case-insensitive. No path-separator handling needed (the `.pi/agent` token is a literal substring per upstream).
- **REQ-NF-004:** Hermetic tests — gate test and detection tests both use injection seams; no real env mutation outside subtests that need it.
- **REQ-NF-005:** All `.go` files carry the Neo4j copyright header (CI gate via `addlicense`).
- **REQ-NF-006:** `make fmt-check`, `make lint`, `make test`, `make generate-check` all pass on the final commit.

## Technical Considerations

- **TTY signal:** `term.IsTerminal(int(os.Stdout.Fd()))` is the right anchor. Stdin TTY is too narrow (humans pipe stdin all the time, e.g. `cat creds.txt | neo4j-cli credential dbms add --file -` once that lands). Stdout TTY matches "human staring at the screen".
- **Precedence:** agent-detection wins over TTY. Rationale: if `CLAUDECODE=1` is in a developer's local shell because they sometimes launch Claude Code from that terminal, we still want the gate to fire — opting in to safety is cheaper than a silent write.
- **Seam pattern:** package-level `var detectAgent = agent.Detect` and `var stdoutIsTerminal = func() bool {...}` mirror the existing `stdinIsTTY = func() bool {...}` in `neo4j-cli/query/run.go:35`. Reviewers will recognise it.
- **Skill bundle round-trip:** the gate logic itself is invisible to the bundle. Only `additions.md` changes propagate. Both surfaces (neo4j-cli, standalone aura) must be regenerated; `TestGenerator_RoundTrip` catches drift.
- **No README duplication of agent list:** README points to `unjs/std-env` rather than enumerating env vars, so the list doesn't go stale in two places. Authoritative copy lives in `common/agent/agent.go`.
- **Out-of-scope agent detection elsewhere:** keeping `common/agent/` small and behaviour-only (no logging, no telemetry) leaves room for `common/output/` to consume `agent.Detect()` later if we want auto-`--format json` for agents.
- **No behaviour change for `--rw=true` callers.** Every existing CI script, golden test, and agent that currently passes `--rw` keeps working unchanged.
- **Failure message stays the same string.** Tests asserting `"this command writes; pass --rw to allow it"` continue to match — important because both surfaces and many table tests grep that exact string.

## Acceptance Criteria

- [ ] `common/agent/agent.go` and `agent_test.go` exist; test covers all 11 agents + the no-agent baseline.
- [ ] `EnforceWriteGate` follows the 4-step rule from REQ-F-004; existing usage-error string unchanged.
- [ ] `--rw` flag Usage updated to REQ-F-005 wording on BOTH `neo4j-cli` root and standalone aura root (`flags.RegisterRwFlag` is shared so this is one edit).
- [ ] Gate matrix from REQ-F-006 has a corresponding test case in `common/flags/`; all 6 rows pass.
- [ ] `neo4j-cli/internal/skill/additions.md` updated per REQ-F-007.
- [ ] `neo4j-cli/aura/internal/skill/additions.md` updated per REQ-F-008.
- [ ] `bundle/SKILL.md` on both surfaces regenerated; `TestGenerator_RoundTrip` green.
- [ ] README has a short auto-detect section per REQ-F-010.
- [ ] Changelog entry committed under `.changes/unreleased/` per REQ-F-011.
- [ ] Manual verification in a terminal:
  - `bin/neo4j-cli credential dbms add --name x --uri … --username … --password …` succeeds without `--rw`.
  - `CLAUDECODE=1 bin/neo4j-cli credential dbms add …` fails with `this command writes; pass --rw to allow it`.
  - `bin/neo4j-cli credential dbms add … </dev/null` (no TTY, no agent) fails the same way.
  - With `--rw=true`, all three paths succeed.
- [ ] `make test`, `make fmt-check`, `make lint`, `make generate-check` all pass on the final commit.

## Out of Scope

- Escape-hatch env var.
- README enumeration of the agent env-var list (link to upstream instead).
- In-CLI confirmation prompts (confirmation lives in agent skill prose, not in the binary).
- Changing which commands carry `Annotations["write"] == "true"`.
- Auto-`--format json` for detected agents (future opportunity; left out per non-goals).
- Telemetry / metrics on agent detection.
- Standalone aura README updates beyond the shared bundle prose (the standalone binary is no longer built/shipped per AGENTS.md).

## Open Questions

None — pre-PRD questions resolved:
- Non-TTY non-agent → keep requiring `--rw` (confirmed).
- No escape-hatch env var (confirmed).
- Agent list = upstream `unjs/std-env`, including `pi` (confirmed).
- Detection package home: new `common/agent/` (confirmed).
- README: short auto-detect note (confirmed).
