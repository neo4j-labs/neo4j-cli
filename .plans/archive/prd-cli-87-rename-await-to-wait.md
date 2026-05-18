# PRD: Rename `--await` to `--wait` (with deprecated alias)

Linear: [CLI-87](https://linear.app/neo4j/issue/CLI-87/f2-await-instead-of-canonical-wait) (parent: CLI-71)
Source plan: `/Users/oskarhane/.claude/plans/let-s-take-on-https-linear-app-neo4j-iss-tender-creek.md`
Source audit: `agent-cli-audit-2026-05-011.md §2.2`
Related: differs from CLI-101 (hard rename) — this rename keeps a deprecated alias for one release.

## Overview

Per the agent-CLI audit §2.2, the canonical async flag in this repo is `--wait`, but every async operation (13 leaf commands) currently uses `--await`. CLI-87 renames `--await` → `--wait` across all 13 commands and updates both `CONTRIBUTING.md §3.5` and the agent-context `async_flag` constant to match.

Unlike CLI-101 (hard rename, no alias), CLI-87 keeps `--await` working as a hidden, deprecated alias that emits a stderr warning when used. Cobra's built-in `MarkDeprecated` provides exactly the behaviour the audit asks for — hides from `--help`, prints `Flag --await has been deprecated, use --wait instead` to stderr — so the alias requires no custom helper. Removal of the alias is tracked as a follow-up Linear issue with a 2026-06-03 due date (3 weeks).

To keep each leaf's diff to ~3 lines and centralise the deprecation behaviour, the rename introduces a single helper `flags.RegisterWait(cmd, &wait, helpText)` under `neo4j-cli/aura/internal/flags/`. All 13 leaves call this helper.

## Goals

- Rename `--await` → `--wait` on 13 leaf commands.
- Accept `--await` as a hidden, deprecated alias that warns on stderr — implemented via cobra's `MarkDeprecated`, not a custom helper.
- Single shared registration helper (`flags.RegisterWait`) so the alias lives in one place and removal next release is a one-file change.
- Update `CONTRIBUTING.md §3.5` so the documented canonical name matches reality (`--wait`).
- Update the agent-context `async_flag` constant from `"--await"` to `"--wait"` so MCP-style agent consumers see the canonical name.
- Regenerate both skill bundles so `references/*.md` reflect `--wait` and `TestGenerator_RoundTrip` stays green.
- Land a single `Minor` changie entry under the `neo4j-cli` project key referencing both CLI-87 and the follow-up cleanup issue.

## Non-Goals

- **No expansion of the alias mechanism beyond `--await`.** Other audit findings (other flag renames, other commands) are tracked as separate `F<N>` audit items under CLI-71.
- **No new behaviour.** This is a flag-name + deprecation-wiring change. No polling logic, no error handling, no async semantics change.
- **No alias for `--wait` itself.** Only `--await` is the deprecated alias.
- **No edits to `CHANGELOG-aura.md` or `.changes/aura-cli/*.md`.** Those are historical; the aura standalone binary is no longer shipped.
- **No removal of the alias in this PR.** Removal is tracked as a follow-up issue (due 2026-06-03).
- **No shorthand** (`-w`). Matches `--await`'s current shape (no shorthand).
- **No custom deprecation warning text.** Cobra's default `Flag --await has been deprecated, use --wait instead` is sufficient.
- **No promotion of `--wait` to a persistent / root-level flag.** Stays per-leaf, matching `--await` today. `clicfg`-style global plumbing is out of scope.

## Requirements

### Functional Requirements

- **REQ-F-001:** New helper at `neo4j-cli/aura/internal/flags/wait.go`:
  ```go
  package flags

  import "github.com/spf13/cobra"

  const (
      WaitFlag       = "wait"
      AwaitFlagAlias = "await" // deprecated; remove after CLI-87 follow-up (due 2026-06-03)
  )

  // RegisterWait registers --wait on cmd, plus --await as a hidden deprecated
  // alias that prints a stderr warning when used. Both flags bind to the same
  // *bool. helpText is the Usage string for --wait; the alias inherits it.
  func RegisterWait(cmd *cobra.Command, wait *bool, helpText string) {
      cmd.Flags().BoolVar(wait, WaitFlag, false, helpText)
      cmd.Flags().BoolVar(wait, AwaitFlagAlias, false, helpText)
      _ = cmd.Flags().MarkDeprecated(AwaitFlagAlias, "use --wait instead")
  }
  ```
  Single `package flags` declaration matches the existing files in that directory (`cloudprovider.go`, `memory.go`, etc.).

- **REQ-F-002:** Helper test at `neo4j-cli/aura/internal/flags/wait_test.go` — table-driven, 4 cases:
  - `--wait` sets the bool to true; stderr empty of deprecation marker.
  - `--await` sets the bool to true AND stderr contains `Flag --await has been deprecated, use --wait instead`.
  - Neither flag → bool stays false; stderr empty.
  - `--wait --await` together → bool true; stderr contains deprecation marker (alias is still flagged even when the canonical is also present). Pattern: build a minimal `&cobra.Command{Use: "test", RunE: func(...) error { return nil }}`, register with `RegisterWait`, capture stderr via `cmd.SetErr(&buf)`, parse args via `cmd.SetArgs([]string{...}); cmd.Execute()`.

- **REQ-F-003:** All 13 leaf commands switch from `cmd.Flags().BoolVar(&await, awaitFlag, false, "<text>")` to `flags.RegisterWait(cmd, &wait, "<text>")`. For each file:
  - Drop the local `awaitFlag = "await"` const (where present — 9 files have it; 4 use literal `"await"` strings).
  - Rename the local `var await bool` to `var wait bool`.
  - Rename `if await {` → `if wait {`.
  - Replace the existing `BoolVar` registration with the helper call.
  - Add `"github.com/neo4j/cli/neo4j-cli/aura/internal/flags"` import if not already present (most files import it for other flag helpers).
  - Update `cmd.Example` strings: every `--await` literal → `--wait`. (CLI-93 added Examples to every command; they're flush-left, multi-invocation, comment-headed; preserve that shape.)
  - Update `cmd.Long` strings: any `--await` mention → `--wait`. (Most don't mention the flag name in `Long`; check each.)
  - Drop any local `awaitFlag` const usage in error strings (the audit survey showed none, but verify).

  Files (each with its existing help text preserved verbatim as the helper's `helpText` argument):

  | File | Existing help text |
  |---|---|
  | `neo4j-cli/aura/internal/subcommands/instance/create.go` | `Waits until created instance is ready.` |
  | `neo4j-cli/aura/internal/subcommands/instance/resume.go` | `Waits until resumed instance is ready.` |
  | `neo4j-cli/aura/internal/subcommands/instance/snapshot/create.go` | `Waits until created snapshot is ready.` |
  | `neo4j-cli/aura/internal/subcommands/instance/overwrite.go` | `Waits until created snapshot is ready` |
  | `neo4j-cli/aura/internal/subcommands/dataapi/graphql/create.go` | `Waits until created GraphQL Data API is ready.` |
  | `neo4j-cli/aura/internal/subcommands/dataapi/graphql/resume.go` | `Waits until GraphQL Data API is resumed.` |
  | `neo4j-cli/aura/internal/subcommands/dataapi/graphql/update.go` | `Waits until updated GraphQL Data API is ready again.` |
  | `neo4j-cli/aura/internal/subcommands/dataapi/graphql/pause.go` | `Waits until GraphQL Data API is paused.` |
  | `neo4j-cli/aura/internal/subcommands/dataapi/graphql/authprovider/create.go` | `Waits until created Authentication provider is ready.` |
  | `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/add.go` | `Waits until updated GraphQL Data API is ready.` |
  | `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/remove.go` | `Waits until updated GraphQL Data API is ready.` |
  | `neo4j-cli/aura/internal/subcommands/customermanagedkey/create.go` | `Waits until created customer managed key is ready.` |
  | `neo4j-cli/aura/internal/subcommands/graphanalytics/session/create.go` | `Waits until created session is ready.` |

- **REQ-F-004:** Existing tests that pass `--await` on the command line are rewritten to pass `--wait`. The alias path is covered exclusively by `wait_test.go` (REQ-F-002); per-command alias regression tests would just re-test the helper. Test funcs are renamed (`…WithAwait` → `…WithWait`):
  - `neo4j-cli/aura/internal/subcommands/instance/create_test.go`: `TestCreateFreeInstanceWithAwait`, `TestCreateFreeInstanceWithAwait_StdoutIsValidJSON`, `TestCreateCredentialStoredBeforeAwait` → `…WithWait` / `…BeforeWait`.
  - `neo4j-cli/aura/internal/subcommands/instance/snapshot/create_test.go`: `TestCreateSnapshotWithAwait` → `TestCreateSnapshotWithWait`.
  - `neo4j-cli/aura/internal/subcommands/instance/overwrite_test.go`: `TestOverwriteWithAwait` → `TestOverwriteWithWait`.
  - `neo4j-cli/aura/internal/subcommands/graphanalytics/session/create_test.go`: any `--await` invocations.
  - Plus any other `--await` literal found via `grep -rn "\-\-await" --include="*_test.go"`.

- **REQ-F-005:** Agent context — `neo4j-cli/internal/subcommands/agentcontext/build.go`:
  - Line 38: `const asyncFlag = "--await"` → `const asyncFlag = "--wait"`.
  - Lines 35-37 comment: drop the "renaming to `--wait` is a separate audit item" wording; replace with one-line `asyncFlag is the canonical async-flag name in this repo (--wait, post-CLI-87).`
  - `neo4j-cli/internal/subcommands/agentcontext/build_test.go` and `agentcontext_test.go`: any `assert.Equal(t, "--await", ctx.AsyncFlag)` → `"--wait"`.

- **REQ-F-006:** `CONTRIBUTING.md §3.5` (line 207) rewritten so canonical name is `--wait`. Add one sentence: `The flag --await is accepted as a deprecated alias and will be removed in the following release.` Reference CLI-87 inline if convention allows (other sections don't link Linear, so probably not).

- **REQ-F-007:** Documentation surfaces — `README.md`, `AGENTS.md` (note: `CLAUDE.md` is a symlink; edit `AGENTS.md` only), `.agents/architecture.md`: every `--await` mention → `--wait`. Grep all of these and apply changes.

- **REQ-F-008:** Both skill bundles regenerate cleanly:
  - `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`
  - Expected diffs in `references/*.md` under both `neo4j-cli/internal/skill/bundle/` and `neo4j-cli/aura/internal/skill/bundle/`: `--await` → `--wait` in flag tables, Example blocks, and any prose mention.
  - **The deprecated alias must NOT appear in bundles** (cobra's `MarkDeprecated` hides flags from help, and the render uses `cmd.Flags().FlagUsages()` style introspection — verify during implementation that the alias is correctly elided).
  - `TestGenerator_RoundTrip` (both packages) is the gate; bundle and source must be committed together.

- **REQ-F-009:** Follow-up Linear issue created BEFORE PR opens (so the changelog can reference its ID):
  - Title: `Remove deprecated --await alias (follow-up to CLI-87)`
  - Team: Neo4j; Project: CLI; Parent: CLI-71
  - Due date: **2026-06-03**
  - Body: file list for mechanical removal — `neo4j-cli/aura/internal/flags/wait.go` (drop `AwaitFlagAlias` const, drop second `BoolVar`, drop `MarkDeprecated`) and `wait_test.go` (drop the two alias cases). Also: regenerate bundles; update `CONTRIBUTING.md §3.5` to drop the alias sentence.

- **REQ-F-010:** Changie entry — one new file under `.changes/unreleased/` named `neo4j-cli-Minor-<YYYYMMDD>-<HHMMSS>.yaml`:
  ```yaml
  project: neo4j-cli
  kind: Minor
  body: 'Renamed --await to --wait on all async commands. --await is accepted as a deprecated alias for one release; see CLI-NNN for removal (CLI-87).'
  time: <RFC3339 timestamp>
  ```
  Substitute the follow-up issue's ID for `CLI-NNN` once REQ-F-009 is done.

### Non-Functional Requirements

- **REQ-NF-001:** `make fmt-check` passes.
- **REQ-NF-002:** `make lint` passes (golangci-lint v2 clean).
- **REQ-NF-003:** `make test` passes on the full matrix locally; `TestGenerator_RoundTrip` in both skill packages must not report bundle drift (regen committed in the same change).
- **REQ-NF-004:** `TestAllLeafCommands_HaveExamples` (`neo4j-cli/internal/subcommands/agentcontext/agentcontext_test.go`) continues to pass — rewritten `Example:` strings must keep flush-left indent, ≥3 invocations, `# comment` per invocation, blank-line separators, `neo4j-cli` prefix, `--rw` on write invocations.
- **REQ-NF-005:** `make license-check` passes (the only new file `wait.go` must carry the Neo4j copyright header).
- **REQ-NF-006:** `make generate-check` passes (CI gate: no diff after `go generate ./...`).

## Technical Considerations

- **Cobra `MarkDeprecated` behaviour.** Two effects: (1) hides the flag from `--help` output (cobra adds `Hidden: true` on the underlying `pflag.Flag`); (2) prints `Flag --<name> has been deprecated, <msg>` to stderr on every invocation where the flag appears. Both effects are exactly what the audit asks for. No custom warning helper needed.
- **Both flags bind to the same `*bool`.** cobra/pflag tracks them as two separate flags but writes to the same backing variable. If both `--wait` and `--await` are passed, the last one parsed wins — since both are bool flags defaulting to true when present, this is observationally indistinguishable. Edge case: `--wait=false --await=true` would set the var to `true`; this is acceptable because no real workflow mixes both.
- **Why a helper, not 13 copy-pastes.** Centralising in `flags.RegisterWait` means the eventual removal (CLI-NNN follow-up) is a single-file change. It also makes the alias-warning behaviour testable in one place rather than per-leaf.
- **Why `aura/internal/flags/`, not a higher-level package.** All 13 async commands are under `neo4j-cli/aura/`. The existing reusable flag types (`cloudprovider.go`, `memory.go`, etc.) live in this same directory and follow the same `package flags` pattern. No need to introduce a new top-level package.
- **Bundle round-trip with deprecated flags.** Cobra hides deprecated flags from `FlagUsages()`. The skill bundle generator (in `common/skill/render/render.go`) reads `cmd.Flags().FlagUsages()` or similar — confirm during implementation that the alias does not leak into bundle output (it should be elided automatically). If it does leak, the generator needs a filter; that's a small follow-up but should be caught before merge by inspecting the regenerated bundle diff.
- **`agent-context async_flag` consumer impact.** Downstream agents reading `async_flag` from the agent-context JSON will see `--wait` after this change. They may have hard-coded `--await`. Since the alias still works, no breakage; agents that update to `--wait` are forward-compatible.
- **Linear branch name.** Linear suggests `cli-87-f2-await-instead-of-canonical-wait`. Per user's global instructions, prefix with `oskar/`: `oskar/cli-87-rename-await-to-wait`.
- **PR title prefix.** Other CLI-* PRs in the repo use `feat(cli):` for additive changes and `refactor(cli):` for renames. CLI-101 used `feat(cli):` for a rename, so this PR follows: `feat(cli): rename --await to --wait (CLI-87)`.

## Acceptance Criteria

- [ ] `bin/neo4j-cli aura instance create --type free-db --wait --rw <…>` works (golden path on canonical name).
- [ ] `bin/neo4j-cli aura instance create --type free-db --await --rw <…>` works AND prints `Flag --await has been deprecated, use --wait instead` to stderr.
- [ ] `bin/neo4j-cli aura instance create --help` shows `--wait` and does NOT show `--await`.
- [ ] `bin/neo4j-cli agent-context | jq -r .async_flag` returns `"--wait"`.
- [ ] All 13 leaf commands' `Example:` blocks use `--wait`; `TestAllLeafCommands_HaveExamples` passes.
- [ ] `flags.RegisterWait` and its test exist in `neo4j-cli/aura/internal/flags/`.
- [ ] All 13 leaves call `flags.RegisterWait`; no leaf has a local `awaitFlag` const or `BoolVar(... "await" ...)` registration left.
- [ ] `CONTRIBUTING.md §3.5` documents `--wait` as canonical and mentions the `--await` alias.
- [ ] `README.md`, `AGENTS.md`, `.agents/architecture.md` mention `--wait`, not `--await`.
- [ ] Both skill bundles regenerated; `references/*.md` under both `neo4j-cli/internal/skill/bundle/` and `neo4j-cli/aura/internal/skill/bundle/` show `--wait` and not `--await`.
- [ ] `make fmt-check && make lint && make test && make generate-check` all pass.
- [ ] Follow-up Linear issue exists under team Neo4j / project CLI / parent CLI-71, due 2026-06-03, titled `Remove deprecated --await alias (follow-up to CLI-87)`.
- [ ] One `neo4j-cli-Minor-*.yaml` added under `.changes/unreleased/` referencing both CLI-87 and the follow-up issue's ID.
- [ ] PR title: `feat(cli): rename --await to --wait (CLI-87)`; body references both Linear issues for auto-link.

## Out of Scope

See **Non-Goals** above. No alias for `--wait` itself, no shorthand, no removal of the `--await` alias (tracked separately), no behavioural changes beyond the flag-name binding + deprecation warning.

## Open Questions

- None. All design decisions locked in plan-mode Q&A (cobra default warning text; follow-up Linear issue with 2026-06-03 due date).
