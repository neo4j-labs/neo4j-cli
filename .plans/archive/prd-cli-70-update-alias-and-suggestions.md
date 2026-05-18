# PRD: CLI-70 — `update` alias and universal "did you mean" suggestions

## Overview

Two DX polish items on the recently-shipped `neo4j-cli update` (self-update) command, tracked as [Linear CLI-70](https://linear.app/neo4j/issue/CLI-70):

1. **Alias `upgrade` → `update`** — match the common synonym used by `brew`, `apt`, `npm`, etc.
2. **Universal "did you mean" suggestions** — when a user types a typo at *any* level of the command tree (not just root), surface cobra's Levenshtein suggestion instead of silently printing help.

Verification during planning showed `neo4j-cli configg list` (root typo) already suggests `config`, but `neo4j-cli config lsit` (nested typo) silently prints `config --help` and exits 0. This is because cobra's `legacyArgs` only emits the `unknown command ... + SuggestionsFor` error path when `!cmd.HasParent()`. Every nested parent (`config`, `aura`, `aura tenant`, `aura dataapi`, `credential`, `docker`, `skill`, etc.) swallows typos silently. Fix: install our own `Args` validator on every parent command with subcommands but no `Run`.

## Goals

- `neo4j-cli upgrade [args]` resolves to `neo4j-cli update [args]` (including subcommands like `upgrade check`).
- Typos at any nesting level emit cobra's "Did you mean ...?" error and exit non-zero.
- The fix is a single chokepoint in `neo4j-cli/app/app.go` — no per-command opt-in required, automatically covering future commands.
- No regression of existing typo handling at the root level.
- No interference with commands that already define their own `Run`/`RunE` (e.g. `update`, `query`, all leaves) or their own `Args` validator (e.g. `cobra.ExactArgs(1)`).
- Agent-context JSON (`neo4j-cli agentcontext`) reflects the new alias automatically via the existing `Aliases` field in `build.go`.

## Non-Goals

- **Flag-typo suggestions** (`--forec` → `--force`). pflag has no Levenshtein hook; would require wrapping `ParseFlags`. Deferred until there's a real signal it's needed.
- **Positional / enum-value suggestions** (`--format jsno` → `json`). Same reason.
- **Aliases on other commands** (`ls`/`rm`/`new`/etc.). Opinion-heavy; can be added per-command on user request. CLI-70 only names `upgrade`.
- **Tuning `SuggestionsMinimumDistance`**. Default of 2 was manually verified to catch `lsit`, `insance`, `updaet`, `auro`. Raising it surfaces noisy false positives.
- **Renaming `update` to `upgrade`**. The canonical name stays `update`; `upgrade` is a pure alias.

## Requirements

### Functional Requirements

- **REQ-F-001**: `neo4j-cli/internal/subcommands/update/update.go` must set `Aliases: []string{"upgrade"}` on the `update` cobra command returned by `NewCmd`.
- **REQ-F-002**: `neo4j-cli upgrade` must execute the same `RunE` as `neo4j-cli update` (cobra-native behaviour once the alias is set).
- **REQ-F-003**: `neo4j-cli upgrade check` must execute the same `RunE` as `neo4j-cli update check` (alias inheritance — cobra-native, no code edits to `check.go`).
- **REQ-F-004**: A new shared package `common/clicmd/` must exist with two exported symbols:
  - `SuggestSubcommand(cmd *cobra.Command, args []string) error` — an `Args` validator that, when `len(args) > 0`, returns `fmt.Errorf("unknown command %q for %q%s", args[0], cmd.CommandPath(), cmd.SuggestionsFor(args[0]))`.
  - `ApplySuggestionsToParents(root *cobra.Command)` — recursively walks the cobra tree; for each command where `HasSubCommands() && Run == nil && RunE == nil && Args == nil`, sets `Args = SuggestSubcommand`.
- **REQ-F-005**: `neo4j-cli/app/app.go` must call `clicmd.ApplySuggestionsToParents(cmd)` as the last statement before `return cmd` in `NewCmd`.
- **REQ-F-006**: `neo4j-cli updaet` (root typo) must continue to print cobra's "Did you mean `update`?" suggestion and exit non-zero (existing behaviour preserved).
- **REQ-F-007**: `neo4j-cli configg list` (root typo with extra args) must continue to print "Did you mean `config`?" and exit non-zero (existing behaviour preserved).
- **REQ-F-008**: `neo4j-cli config lsit` (nested typo) must print "Did you mean `list`?" and exit non-zero (new behaviour).
- **REQ-F-009**: `neo4j-cli aura insance list` (nested typo on `aura` parent) must print "Did you mean `instance`?" and exit non-zero (new behaviour).
- **REQ-F-010**: `neo4j-cli credential aura-clent add` (nested typo on `credential` parent) must print "Did you mean `aura-client`?" and exit non-zero (new behaviour).
- **REQ-F-011**: Commands that already define a custom `Args` validator (e.g. `cobra.ExactArgs(1)`, `cobra.NoArgs`) must retain their validator unchanged — the walker only installs `SuggestSubcommand` when `Args == nil`.
- **REQ-F-012**: Commands that define `Run` or `RunE` (e.g. `update`, `query`, every leaf) must remain unaffected — the walker only installs `SuggestSubcommand` when both are nil.
- **REQ-F-013**: A changelog entry must be created via `changie new --projects neo4j-cli --kind Patch --body "Add 'upgrade' alias for the update command and 'did you mean' suggestions for typo'd subcommands"`.

### Non-Functional Requirements

- **REQ-NF-001**: `make test`, `make fmt-check`, and `make lint` must pass after the change. Existing tree-walking gate test (`TestAllLeafCommands_HaveExamples` in `agentcontext/agentcontext_test.go`) must stay green.
- **REQ-NF-002**: No `go generate` run is required (renderer in `common/skill/render/render.go` only reads `Short`/`Long`, not `Aliases` or `Args`; bundle output is unchanged).
- **REQ-NF-003**: `agentcontext` JSON automatically reflects the new alias via the existing `Aliases []string` field in `neo4j-cli/internal/subcommands/agentcontext/build.go:92` — no edits to `agentcontext` package needed; existing tests covering the JSON shape should continue to pass with the alias appearing in the `update` command's `aliases` array.
- **REQ-NF-004**: The walker must be deterministic and side-effect-free aside from setting `Args` on matched commands.

## Technical Considerations

### Cobra `legacyArgs` behaviour

When a cobra command has subcommands and no Run logic and no explicit `Args` validator, cobra's `legacyArgs` (used by default) only triggers the "unknown command" + `SuggestionsFor` error path when `!cmd.HasParent()`. Otherwise it returns nil — silently dropping the unknown arg and falling through to print help. This is a long-standing cobra UX wart; the standard workaround in production CLIs is to install a custom `Args` validator on every parent. Our walker centralises that policy in one place.

### Walker semantics

Recursive, bottom-up traversal of `root.Commands()`. Three guard conditions ensure we never override deliberate command design:

1. `HasSubCommands()` — only install on parents.
2. `Run == nil && RunE == nil` — never override commands that produce output of their own.
3. `Args == nil` — never override an explicit validator chosen by the command author (e.g. `cobra.ExactArgs(1)` on `credential set <key>`).

### Package layout

`common/clicmd/` is a new top-level shared package. The user chose this over colocating in `neo4j-cli/app/` because if a future second CLI is added, the helper is already reusable without a move. Imports: `fmt`, `github.com/spf13/cobra`. No other dependencies.

### Wire-up site

`neo4j-cli/app/app.go` `NewCmd` is the single entrypoint for the whole command tree. Calling `clicmd.ApplySuggestionsToParents(cmd)` immediately before `return cmd` ensures every subtree mounted via `cmd.AddCommand(...)` (including `aura.NewCmd`, `credential.NewCmd`, `config.NewCmd`, `docker.NewCmd`, `skill.NewCmd`, etc.) is covered without per-subtree opt-in.

### Tests

Two new test files:

- `common/clicmd/suggest_test.go` — unit tests against a synthetic three-level cobra tree, asserting:
  - `SuggestSubcommand` returns the right error format including cobra's `SuggestionsFor` suffix.
  - `ApplySuggestionsToParents` respects all three guard conditions (skips Run-bearing, skips RunE-bearing, skips pre-set Args).
  - Recursion reaches arbitrary depth.
- `neo4j-cli/app/app_suggest_test.go` — integration tests against real `app.NewCmd(cfg)`. Drive each case via `cmd.SetArgs(...)`, `cmd.SetErr(buf)`, `cmd.Execute()`. Use `clicfg` test helpers (`neo4j-cli/aura/internal/test/testutils/testfs.GetTestFs(...)`).

Plus extension to `neo4j-cli/internal/subcommands/update/update_test.go`:

- `TestUpdateCmd_UpgradeAlias` — asserts `cmd.Aliases` contains `"upgrade"` and `rootCmd.Find([]string{"upgrade"})` resolves.

Integration cases table:

| input args                               | expect stderr contains              | expect non-zero exit |
| ---------------------------------------- | ----------------------------------- | -------------------- |
| `["configg", "list"]`                    | `Did you mean ... config`           | yes                  |
| `["config", "lsit"]`                     | `Did you mean ... list`             | yes                  |
| `["aura", "insance", "list"]`            | `Did you mean ... instance`         | yes                  |
| `["credential", "aura-clent", "add"]`    | `Did you mean ... aura-client`      | yes                  |
| `["updaet"]`                             | `Did you mean ... update`           | yes                  |
| `["upgrade", "--help"]` (alias resolves) | n/a — separate alias-resolves test  | no                   |

### Risk: false-positive overrides

A future PR could add a `Run` to a current parent command and forget to clean up — but that's safe because our walker re-evaluates on each `NewCmd` call and the `Run == nil` guard skips it. The walker is idempotent.

### Risk: cross-binary coverage

`neo4j-cli/aura/cmd/main.go` is a historical standalone entrypoint that is compiled but not shipped. If someone runs the standalone aura binary directly, it won't have the walker installed. Acceptable since it's not shipped; if a second binary becomes shipping-relevant, that binary's `NewCmd` can call the same `clicmd.ApplySuggestionsToParents` helper.

## Acceptance Criteria

- [ ] `bin/neo4j-cli upgrade --help` prints the same help text as `bin/neo4j-cli update --help`.
- [ ] `bin/neo4j-cli upgrade check --help` resolves and prints the `check` subcommand help.
- [ ] `bin/neo4j-cli updaet` exits non-zero and stderr contains `Did you mean` and `update`.
- [ ] `bin/neo4j-cli configg list` exits non-zero and stderr contains `Did you mean` and `config`.
- [ ] `bin/neo4j-cli config lsit` exits non-zero and stderr contains `Did you mean` and `list`.
- [ ] `bin/neo4j-cli aura insance list` exits non-zero and stderr contains `Did you mean` and `instance`.
- [ ] `bin/neo4j-cli credential aura-clent add` exits non-zero and stderr contains `Did you mean` and `aura-client`.
- [ ] `common/clicmd/suggest.go` and `common/clicmd/suggest_test.go` exist; unit tests pass.
- [ ] `neo4j-cli/app/app_suggest_test.go` exists and covers the cases in the integration table; all pass.
- [ ] `neo4j-cli/internal/subcommands/update/update_test.go` contains a `TestUpdateCmd_UpgradeAlias` test and it passes.
- [ ] `make test` is green (including the existing `TestAllLeafCommands_HaveExamples` gate).
- [ ] `make fmt-check` produces no output.
- [ ] `make lint` is clean.
- [ ] A new file `.changes/unreleased/neo4j-cli-Patch-<timestamp>.yaml` exists with the body specified in REQ-F-013.
- [ ] Manual smoke: `bin/neo4j-cli agentcontext --format json | jq '..|.aliases? // empty | select(. != [])'` shows `["upgrade"]` among the alias arrays (confirms the alias surfaces through to the agent-context JSON automatically).

## Out of Scope

- Flag-typo suggestions (pflag has no Levenshtein hook; out of scope for this change, deferred until needed).
- Positional / enum-value suggestions (same reason).
- Aliases on other commands (no `ls`/`rm`/`new`/etc. in this PR).
- Tuning `SuggestionsMinimumDistance` (default of 2 is sufficient).
- Renaming `update` → `upgrade` (alias only; canonical name stays `update`).
- Modifying the standalone `aura` binary entrypoint (`neo4j-cli/aura/cmd/main.go`) — not shipped.

## Open Questions

_None._ Both scoping questions resolved during planning:

1. Second alias on `update` beyond `upgrade`? — **No**, only `upgrade`.
2. Audit other commands for aliases in the same PR? — **No**, deferred to its own issue.
