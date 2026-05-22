# PRD: Unknown subcommand exits 2 (usage_error)

Linear: [CLI-141](https://linear.app/neo4j/issue/CLI-141/unknown-subcommand-returns-exit-0-cli-108-b) (parent: [CLI-108](https://linear.app/neo4j/issue/CLI-108)).

## Overview

When a user runs a `neo4j-cli` invocation with an unknown subcommand at any nesting level (root, `aura`, `aura instance`, `aura instance snapshot`), the binary currently prints a "Did you mean … ?" hint but exits with the wrong code (0 / 1 depending on the path). The contract defined by the CLI-108 audit (section B1) and the closed exit-code enum advertised by `neo4j-cli agent-context` is **exit 2 (`usage_error`)**.

The "Did you mean … ?" suggestion engine added by CLI-70 (`common/clicmd/SuggestSubcommand` + `ApplySuggestionsToParents`) already fires at every nesting level. The only defect is the error *type*: `SuggestSubcommand` returns an untyped `fmt.Errorf(...)` instead of a `*clierr.CLIError` with code 2, so `main.exitCodeFor` falls back to the generic exit-1 branch (or to 0 when cobra intercepts).

## Goals

- Unknown subcommands at every nesting level exit **2** (`usage_error`).
- The "Did you mean … ?" hint continues to fire unchanged.
- The error message still names the bad subcommand verbatim (no formatting regression).
- The contract is locked by both an in-process test (per nesting level) and an end-to-end subprocess test (`os.Exit` wiring).
- Agents and scripts that already key off `usage_error = 2` get the correct signal for typos.

## Non-Goals

- JSON error envelope for the `--format=json` path. The acceptance criteria explicitly allow staging behind CLI-108-a; that work belongs to a separate PRD.
- Changing the suggestion text or distance algorithm.
- Suppressing cobra's post-error usage dump (`SilenceUsage` stays default).
- Touching CLI-142's response.go `[]string` formatting or exit-0-on-404 work — explicitly out of scope for this task.

## Requirements

### Functional Requirements

- **REQ-F-001:** `common/clicmd/SuggestSubcommand` MUST return a `*clierr.CLIError` with `Code = 2` when `len(args) > 0`. The message format `unknown command %q for %q%s` (where `%s` is the existing `suggestionsSuffix` block) MUST be preserved exactly.
- **REQ-F-002:** `SuggestSubcommand` MUST continue to return `nil` when `len(args) == 0` so that bare-parent invocations still print help and exit 0.
- **REQ-F-003:** The fix MUST cover all four nesting depths reachable from `app.NewCmd(cfg)`:
  - depth 1 (root): `neo4j-cli <typo>` → exit 2.
  - depth 2: `neo4j-cli aura <typo>` → exit 2.
  - depth 3: `neo4j-cli aura instance <typo>` → exit 2 (the canonical CLI-141 repro `instance lis`).
  - depth 4: `neo4j-cli aura instance snapshot <typo>` → exit 2.
- **REQ-F-004:** `ApplySuggestionsToParents`' guards (no Run/RunE/Args pre-set, has subcommands) MUST remain intact so that runnable parents (e.g. `query`, `config set`, `update`) are not affected.
- **REQ-F-005:** A user-facing changelog entry MUST be added under `.changes/unreleased/` via `changie new --projects neo4j-cli --kind Patch`.

### Non-Functional Requirements

- **REQ-NF-001:** `make fmt-check`, `make lint`, and `make test` MUST pass on all three CI OSes (ubuntu, windows, macos).
- **REQ-NF-002:** No new public API surface in `common/clicmd` or `common/clierr`. The change is a single-line behavioural fix plus tests.
- **REQ-NF-003:** No regression in the existing `TestSuggestionsForTypos` table or `TestSuggestionsDoNotShadowRunnableCommands` table in `neo4j-cli/app/app_suggest_test.go`.
- **REQ-NF-004:** No bundle drift — `make generate-check` clean (this change does not touch cobra Short/Long fields that feed the skill bundle).

## Technical Considerations

### Architecture / integration points

- **`common/clicmd/suggest.go:42`** — replace the `fmt.Errorf(...)` call with `clierr.NewUsageError(...)`. Add `"github.com/neo4j/cli/common/clierr"` to imports; keep `"fmt"` (still used by `suggestionsSuffix`). Update the doc comment on `SuggestSubcommand` (lines 21-37) to state the returned error is a `*clierr.CLIError` (exit code 2).
- **`common/clierr/error.go:33`** — already exports `NewUsageError(msg string, a ...any) error` returning a `*CLIError{Code: 2, ...}`. Pattern is already used at `neo4j-cli/app/app.go:50` to wrap cobra flag-parse errors. No change here.
- **`neo4j-cli/main.go:30`** — `exitCodeFor` already extracts `*clierr.CLIError.Code` via `errors.As`. No change needed.
- **`neo4j-cli/app/app.go:85`** — `clicmd.ApplySuggestionsToParents(cmd)` already walks the full tree and installs the suggestion validator on root + every nested parent (root qualifies because it has no Run/RunE/Args). No change needed.

### Test surface

- **`common/clicmd/suggest_test.go`** — existing tests `TestSuggestSubcommand_UnknownReturnsWrappedError` and `TestApplySuggestionsToParents_RecursesDepth2AndBeyond` already validate message contents via direct calls to the validator. Extend each error-returning assertion with `var ce *clierr.CLIError; require.True(t, errors.As(err, &ce)); assert.Equal(t, 2, ce.Code)`.
- **`neo4j-cli/app/app_suggest_test.go`** — `TestSuggestionsForTypos` (lines 21-94) is table-driven over the live `app.NewCmd(cfg)` tree. Add two table rows for the depths not currently covered:
  - `["aura", "foo"]` (depth 2 — direct aura parent typo).
  - `["aura", "instance", "snapshot", "foo"]` (depth 4 — nested-parent-of-nested-parent typo).
  - Existing rows (depths 1, 2, 3) stay.
  - For every row, add an `errors.As(execErr, &ce)` assertion checking `ce.Code == 2`.
- **`test/e2e/exitcodes/exitcodes_test.go`** — add one scenario to the `TestExitCodes` table at `:271`:
  ```go
  {
      name:       "unknown_subcommand_usage_2",
      args:       []string{"aura", "instance", "lis"},
      wantExit:   2,
      skipServer: true,
  },
  ```
  This locks the `os.Exit` wiring end-to-end via the actual binary subprocess (the in-process tests only check `cmd.Execute()`'s return value).

### Constraints

- AGENTS.md cobra layout: this is a behavioural change inside an existing leaf, not a new command — no file moves or `cmd.AddCommand` reshuffling required.
- Hermetic tests: existing tests already use `testfs.GetTestFs` + in-memory FS; no real-FS dependencies introduced.
- Windows CI: no path-handling or LF-pinned files touched; should be invisible to the Windows job.
- Bundle: no `Long`/`Short` changes; `TestGenerator_RoundTrip` stays green.

### Branch

`oskar/cli-141-unknown-subcommand-exit-2`.

### Changelog

Single Patch entry:

```bash
changie new --projects neo4j-cli --kind Patch \
  --body "Unknown subcommands now exit 2 (usage_error) at every nesting level instead of falling through to a help dump with the wrong exit code."
```

## Acceptance Criteria

- [ ] `common/clicmd/suggest.go` returns `clierr.NewUsageError(...)` (no `fmt.Errorf`) for the `len(args) > 0` branch; doc comment updated.
- [ ] `common/clicmd/suggest_test.go` asserts `errors.As(...)` to `*clierr.CLIError` with `Code == 2` on every error-returning case.
- [ ] `neo4j-cli/app/app_suggest_test.go` `TestSuggestionsForTypos` table includes depth-2 (`aura foo`) and depth-4 (`aura instance snapshot foo`) rows, and every row asserts `Code == 2`.
- [ ] `test/e2e/exitcodes/exitcodes_test.go` `TestExitCodes` includes the `unknown_subcommand_usage_2` scenario with `wantExit: 2` and `skipServer: true`.
- [ ] One Patch changelog entry in `.changes/unreleased/`.
- [ ] Manual smoke: `go run ./neo4j-cli configg; echo $?` → 2; `go run ./neo4j-cli aura foo; echo $?` → 2; `go run ./neo4j-cli aura instance lis; echo $?` → 2; `go run ./neo4j-cli aura instance snapshot foo; echo $?` → 2.
- [ ] Manual smoke (no regression): `go run ./neo4j-cli aura --help`, `go run ./neo4j-cli aura instance --help`, `go run ./neo4j-cli aura instance snapshot --help` all exit 0 and print help.
- [ ] `make fmt-check && make lint && make test` clean locally.
- [ ] CI green on ubuntu / windows / macos.

## Out of Scope

- JSON error envelope for `--format=json` (deferred to CLI-108-a follow-up).
- Suppressing cobra's post-error usage dump (SilenceUsage stays default).
- CLI-142 (`instance get` exit-0 + `[]string`-stringified error) — separate task.
- Refactoring `SuggestSubcommand` or `ApplySuggestionsToParents` shape — single-line behavioural fix only.
- Touching the suggestion text, distance algorithm, or `SuggestionsMinimumDistance`.

## Open Questions

None. Resolved during planning:
- SilenceUsage: leave default (cobra's usage dump after error stays).
- E2E test: yes, add the `unknown_subcommand_usage_2` scenario.
- Changelog kind: `Patch`.
