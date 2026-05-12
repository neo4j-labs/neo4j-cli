# PRD: Silence root --help on PersistentPreRunE errors (CLI-78)

## Overview

When `neo4j-cli` rejects an invocation during root-level `PersistentPreRunE` (missing `--rw`, invalid `--format`, unknown `--credential`), the user sees the focused error followed by the full root `--help` block. The error gets lost in the spam.

Linear: https://linear.app/neo4j/issue/CLI-78/the-missing-rw-flag-output-shows-all-of-help-which-is-not-helpful

Root cause: Cobra v1.10.2 (`command.go:1165`) prints `c.Println(cmd.UsageString())` after an error unless `SilenceUsage` is set somewhere on the path. `common/flags/flags.go` registers three `PersistentPreRunE` wrappers (`RegisterOutputFlag`, `ComposeRootPersistentPreRunE`, `RegisterAuraCredentialFlag`) that return usage errors without setting `SilenceUsage`. In production, both the error and the full usage block flow to stderr (cobra's `Println` uses `OutOrStderr()` which falls back to `os.Stderr`); in tests the usage block goes to `outWriter` (set via `SetOut`) while existing assertions only check `errWriter`, so the bug is invisible.

The `query` parent's own write-rejection path (`neo4j-cli/query/run.go:50`) already sets `cmd.SilenceUsage = true` early in its `RunE`, so the Cypher-write rejection is correct — but unpinned by tests.

## Goals

- A failed `--rw` gate prints ONLY `Error: this command writes; pass --rw to allow it` (no usage block).
- A failed `--format` validation prints ONLY `Error: invalid format value specified: <val>` (no usage block).
- A failed `--credential` lookup prints ONLY `Error: credential "<name>" not found, run \`<hint>\` to see available credentials` (no usage block).
- The query parent's Cypher-write rejection continues to print a focused error (already works; lock it with a test).
- Behavior is exercised by tests that assert BOTH stderr (error message present) AND stdout (no usage spam leaked).

## Non-Goals

- Changing the wording of any existing error message (kept verbatim).
- Adding retry hints, suggested commands, or examples to the error output.
- Reworking cobra error handling globally — `--help` for read commands, unknown-flag errors, unknown-subcommand errors, etc. are out of scope.
- Touching the query parent's `RunE` — it already sets `SilenceUsage = true`; we only add a regression test.
- Touching other write-rejection paths (e.g. `skill` gating) beyond what already goes through the existing PreRunE wrappers.
- Bumping any dependency, including cobra.

## Requirements

### Functional Requirements

- REQ-F-001: Running a write-annotated command (`Annotations["write"] == "true"`) without `--rw` exits with non-zero, prints exactly the line `Error: this command writes; pass --rw to allow it` on stderr, and prints nothing on stdout. No `Usage:` block anywhere.
- REQ-F-002: Running any command with `--format <invalid>` exits with non-zero, prints exactly `Error: invalid format value specified: <invalid>` on stderr, and prints nothing on stdout.
- REQ-F-003: Running an aura subtree command with `--credential <unknown>` exits with non-zero, prints exactly `Error: credential "<unknown>" not found, run \`<hint>\` to see available credentials` on stderr, and prints nothing on stdout. Hint string remains `neo4j-cli aura credential list` / `aura-cli credential list` per existing root-detection logic.
- REQ-F-004: Running `neo4j-cli query "CREATE (n)"` without `--rw` exits with non-zero, prints `Error: this command writes; pass --rw to allow it` on stderr, and prints nothing on stdout. The existing `SilenceUsage = true` at `neo4j-cli/query/run.go:50` continues to satisfy this.
- REQ-F-005: Running `--help` on any command still prints the full help to stdout (cobra's `flag.ErrHelp` path bypasses the SilenceUsage check).
- REQ-F-006: Successful invocations are unchanged: `cmd.SilenceUsage` is NOT mutated on the happy path.
- REQ-F-007: A `Patch`-kind changelog entry is recorded via `changie` describing the fix.

### Non-Functional Requirements

- REQ-NF-001: No public-API changes to `BindFormatFromFlag`, `EnforceWriteGate`, or any exported function in `common/flags/`. The leaf functions stay pure (no `SilenceUsage` side-effect inside them); the side-effect lives in the PreRunE wrappers via a small helper.
- REQ-NF-002: A single private helper (`silenceUsageOnError(cmd, err) error`) in `common/flags/flags.go` handles the mutation, used by all three wrapper sites.
- REQ-NF-003: All three local gates pass: `make fmt-check`, `make lint`, `make test`. CI gates (`make generate-check`, license-check) also stay green.
- REQ-NF-004: No new external dependencies.
- REQ-NF-005: No skill-bundle regeneration required — change is internal to error-path plumbing, no command tree / flag / Long-description edits.
- REQ-NF-006: Cross-OS: tests use existing harnesses (`testfs`, `neo4jTestHelper`, `runHarness`) — no new OS-specific code paths.

## Technical Considerations

### Files touched

- `common/flags/flags.go`
  - New helper:
    ```go
    // silenceUsageOnError sets cmd.SilenceUsage=true when err is non-nil so
    // cobra prints the focused error without appending the full --help block.
    // Returns err unchanged so call sites can `return silenceUsageOnError(cmd, x())`.
    func silenceUsageOnError(cmd *cobra.Command, err error) error {
        if err != nil {
            cmd.SilenceUsage = true
        }
        return err
    }
    ```
  - Wrap the three return sites:
    - `RegisterOutputFlag` PreRunE (L26-28):
      ```go
      cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
          return silenceUsageOnError(cmd, BindFormatFromFlag(cmd, cfg))
      }
      ```
    - `ComposeRootPersistentPreRunE` (L83-90):
      ```go
      return func(cmd *cobra.Command, args []string) error {
          if err := BindFormatFromFlag(cmd, cfg); err != nil {
              return silenceUsageOnError(cmd, err)
          }
          return silenceUsageOnError(cmd, EnforceWriteGate(cmd))
      }
      ```
    - `RegisterAuraCredentialFlag` PreRunE (L112-139): wrap both the prior-hook error and the credential-not-found error.
  - `BindFormatFromFlag`, `EnforceWriteGate`, `RegisterAuraCredentialFlag`'s inner logic stay unchanged (no `SilenceUsage` writes inside them — keeps direct-call tests in `flags_test.go` semantics intact).

- `common/flags/flags_test.go`
  - New `TestComposeRootPersistentPreRunE_SilencesUsageOnError`:
    - write-annotated cmd + no `--rw` → err non-nil AND `cmd.SilenceUsage == true`.
    - cmd with invalid `--format` → err non-nil AND `cmd.SilenceUsage == true`.
    - happy path → err nil AND `cmd.SilenceUsage == false`.
  - New `TestRegisterOutputFlag_SilencesUsageOnError`:
    - invalid `--format` → err non-nil AND `cmd.SilenceUsage == true`.
    - valid `--format` → err nil AND `cmd.SilenceUsage == false`.
  - Extend `TestRegisterAuraCredentialFlag_CredentialNotFound` (L92): also assert `cmd.SilenceUsage == true` on the cred-not-found row.
  - `TestEnforceWriteGate` (L190) stays unchanged — direct calls to `EnforceWriteGate` don't mutate `SilenceUsage`; that's a wrapper concern.

- `neo4j-cli/internal/subcommands/config/set_test.go`
  - The two `no rw` rows at L23-26 and L102-105 currently only call `assertErr(...)`. Extend so they also assert `h.out` is empty (no usage block leaked to `outWriter`). Smallest change: add a follow-up `assert.Equal(h.t, "", strings.TrimSpace(string(out)))` after the existing `assertErr`. May require exposing an `assertOut("")` helper variant or inlining the read.
  - Optionally add a `config set --rw format invalid` row (already at L46-49) augmented with the same stdout-empty assertion to cover REQ-F-002 via integration.

- `neo4j-cli/internal/subcommands/config/helpers_test.go`
  - If we go with an `assertOut(expected string)` style (already exists at L82), just call `h.assertOut("")` in the new assertions. No helper changes needed.

- `neo4j-cli/query/run_test.go`
  - Extend `TestRunQuery_WriteCypherWithoutRwErrorsBeforeExecution` (L229) to assert `h.stdout.String() == ""` after Execute, in addition to the existing `err.Error()` substring check. Pins the existing `SilenceUsage = true` at `run.go:50`.

- `.changes/unreleased/neo4j-cli-Patch-<ts>.yaml`
  - `changie new --projects neo4j-cli --kind Patch --body "Suppress full --help output when a command is rejected for missing --rw, invalid --format, or unknown --credential; show only the focused error message."`

### Cobra error contract (already verified)

- cobra v1.10.2 `ExecuteC` (L1148-1168): on non-help error, calls `PrintErrln` (errWriter, default `os.Stderr`) for the error and `Println(UsageString())` (outWriter, default `os.Stderr`) for the help block.
- `if !cmd.SilenceUsage && !c.SilenceUsage` (L1165): setting `SilenceUsage = true` on EITHER the root or the leaf suppresses the usage print.
- `flag.ErrHelp` (L1152) is checked first → `--help` continues to render normal help. No risk of `--help` regressions.

### Test seam: stdout/stderr split

- Production: `SetOut`/`SetErr` not called → both writers default to `os.Stderr` → users see error + help block both on stderr (the bug).
- Tests: harnesses call `cmd.SetOut(h.out)` and `cmd.SetErr(h.err)` → error to `h.err`, usage block to `h.out`. Existing tests only assert `h.err`, hiding the bug. New assertions on `h.out` (or `h.stdout`) expose and pin it.

## Acceptance Criteria

- [ ] `bin/neo4j-cli config set telemetry false` (no `--rw`) prints exactly one error line, no usage block.
- [ ] `bin/neo4j-cli credential dbms remove foo` (no `--rw`) prints exactly one error line, no usage block.
- [ ] `bin/neo4j-cli --format=bogus aura instance list` prints exactly one error line, no usage block.
- [ ] `bin/neo4j-cli aura instance list --credential=nope` prints exactly one error line, no usage block.
- [ ] `bin/neo4j-cli query "CREATE (n)"` (no `--rw`, with a reachable db) prints exactly one error line, no usage block.
- [ ] `bin/neo4j-cli config set telemetry false --rw` and other valid commands work unchanged.
- [ ] `bin/neo4j-cli --help` and `bin/neo4j-cli config --help` still render full help.
- [ ] `make test` passes including new assertions in `flags_test.go`, `set_test.go`, `run_test.go`.
- [ ] `make fmt-check` clean.
- [ ] `make lint` clean.
- [ ] `make generate-check` clean (no bundle drift expected).
- [ ] Changelog entry of kind `Patch` exists under `.changes/unreleased/`.

## Out of Scope

- Rewording any existing error message.
- Adding new flags, env vars, or commands.
- Touching cobra's help template, usage template, or error prefix.
- Suppressing usage for other error sources (e.g. `Args` validators, unknown subcommand, unknown flag) — those are cobra-internal paths not currently in scope.
- Refactoring `common/flags/` structure beyond the targeted wrapper edits.

## Open Questions

- Branch name: `oskar/cli-78-silence-help-on-prerune-errors`?
