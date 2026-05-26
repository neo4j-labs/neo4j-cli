# PRD: CLI-157 — `update --force` warns + recommends self-managed install when bypassing pkg-mgr detection

Linear: https://linear.app/neo4j/issue/CLI-157
Parent: https://linear.app/neo4j/issue/CLI-155 (Oplane scan — REQ-00065541 partial)

## Overview

`neo4j-cli update --force` currently bypasses install-method detection silently. When the running binary is package-manager-managed (Homebrew, npm/pnpm/yarn, pipx, uv tool), the in-place swap succeeds but the next `brew upgrade` (or equivalent) reverts it — the user is left on the old version with no warning.

This PRD adds a multi-line stderr warning emitted right before the swap when `--force` is overriding a detected pkg-mgr install. The warning (a) names the channel + resolved binary path, (b) flags the revert risk, and (c) prints the channel-correct **required** remediation: uninstall via the package manager, then install via the curl install script. The command stays non-interactive (no prompt) and the JSON output path on stdout stays clean — `--format json` remains script-safe.

Internally, this also extracts a small set of helpers in `install_method.go` (`channelLabel`, `uninstallCmd`, `selfManagedBlock`) so the existing `Hint()` passthrough and the new `ForceOverrideWarning` share one source of truth for channel labels + remediation block.

## Goals

- Close the Oplane REQ-00065541 verification gap: the user is informed before an irreversible side-effect (binary swap) that conflicts with their package manager.
- Recommend the correct long-term fix (switch to self-managed install) rather than just warning about the symptom.
- Keep `--force --format json` script-safe: stdout JSON unchanged, warning on stderr only.
- Centralise the channel-label / remediation strings so future channels add one row each, not three duplicated copies.

## Non-Goals

- No interactive prompt — sudo owns stdin on the elevated swap path; a runtime `y/n` would also break `--format json` automation.
- No `warnings[]` array in the JSON document. Stderr is the contract.
- No audit log / persistent record of `--force` invocations. Oplane verification cases asking for an interactive prompt + audit log are an awkward fit for a non-interactive CLI; the stderr warning closes the practical gap.
- No `--quiet` / `--no-warning` escape hatch. Warning is unconditional whenever `--force` overrides detection.
- No new exit code. `--force` over a pkg-mgr binary still exits 0 on success; the user explicitly opted in.
- No `SECURITY.md` (tracked separately in the parent Oplane scan).
- No randomised temp-file path for the elevated swap (separate sub-issue of CLI-155).

## Requirements

### Functional Requirements

- **REQ-F-001:** When `--force` is set AND `Detect()` returns one of `InstallMethodHomebrew`, `InstallMethodNpm`, `InstallMethodPipx`, `InstallMethodUv`, `neo4j-cli update` MUST emit a multi-line warning to stderr **before** invoking `swapFn(...)`.
- **REQ-F-002:** The warning MUST name the detected channel (via `channelLabel(method)`) and the resolved absolute binary path returned by `Detect()`.
- **REQ-F-003:** The warning MUST include a remediation block with the channel-correct uninstall command (`brew uninstall neo4j-cli` / `npm uninstall -g @neo4j-labs/cli` / `pipx uninstall neo4j-cli` / `uv tool uninstall neo4j-cli`) and the curl install command (`curl -sSfL https://neo4j.sh/install.sh | bash`).
- **REQ-F-004:** The uninstall line in the remediation block MUST be presented as required (no `optional —` annotation). This change applies to BOTH the new warning and the existing `Hint()` passthrough (single source of truth via `selfManagedBlock`).
- **REQ-F-005:** When `--force` is set AND `Detect()` returns `InstallMethodBinary` (no pkg-mgr match), `neo4j-cli update` MUST NOT emit any warning text on stderr.
- **REQ-F-006:** When `--force` is NOT set, the existing pkg-mgr passthrough behaviour (print `Hint()`, skip swap, exit 0) MUST be preserved.
- **REQ-F-007:** `--format json` with `--force` over a pkg-mgr binary MUST emit a valid JSON document on stdout matching the existing REQ-F-018 shape (fields: `current`, `latest`, `updated`, `check`, `channel`, `install_method`, optional `updated_skills`, optional `skill_install_suggested`). The warning text MUST NOT appear in the JSON document. `--format table` and `--format toon` behave the same.
- **REQ-F-008:** The post-swap success message (`Successfully updated from X to Y`) MUST NOT repeat the warning. The pre-swap stderr warning is the single touchpoint.
- **REQ-F-009:** A new function `ForceOverrideWarning(method InstallMethod, path string) string` MUST be exported from `package update` for unit testing. It MUST return `""` when `method == InstallMethodBinary` and MUST return the full multi-line block (ending in `\n`) for every other method.
- **REQ-F-010:** Three new package-private helpers (`channelLabel(method) string`, `uninstallCmd(method) string`, `selfManagedBlock(method) string`) MUST be added to `install_method.go`. `Hint()` MUST be refactored to call them so the channel preamble, uninstall command, and remediation block have one source of truth.

### Non-Functional Requirements

- **REQ-NF-001:** Warning emission MUST be synchronous and ordered: stderr warning fully flushed before `swapFn(...)` is called. A swap failure MUST NOT swallow the warning.
- **REQ-NF-002:** No new external dependencies. Pure stdlib (`fmt`, `strings`) inside `package update`.
- **REQ-NF-003:** No skill-bundle regeneration required — only runtime stderr output changes; no `Short`/`Long`/flag-Usage edits. `make generate-check` MUST stay green without re-running `go generate`.
- **REQ-NF-004:** Final gates MUST pass: `make test`, `make fmt-check`, `make lint`, `make license-check`.
- **REQ-NF-005:** Cross-platform: warning text is plain ASCII. Resolved path is rendered via `filepath.Abs` output from `Detect()`, which uses OS-native separators (already true today).
- **REQ-NF-006:** No new test seams beyond a new `runWithOptsSplit` helper in `update_test.go` that supplies separate stdout/stderr buffers. Existing seams (`detectFn`, `swapFn`, `latestFn`) remain untouched.

## Technical Considerations

### Files touched

- `neo4j-cli/internal/subcommands/update/install_method.go` — add `channelLabel`, `uninstallCmd`, `selfManagedBlock`, `ForceOverrideWarning`; refactor `Hint` / `formatHint` / `formatHintMulti` to use them.
- `neo4j-cli/internal/subcommands/update/install_method_test.go` — add unit tests for the four helpers; update existing `Hint` goldens (npm preamble shortens, uninstall line loses `# optional…` annotation).
- `neo4j-cli/internal/subcommands/update/update.go` — emit warning at the `result.installMethod = string(method)` site (~line 437), before the plain-text "Current version" narrative and before `swapFn`.
- `neo4j-cli/internal/subcommands/update/update_test.go` — add `runWithOptsSplit` helper + 3 new tests.
- `.changes/unreleased/neo4j-cli-Patch-<ts>.yaml` — changelog entry via `changie new --projects neo4j-cli --kind Patch --body '…'`.

### Warning text (canonical)

Homebrew example:

```
Warning: --force overriding detected Homebrew install at /opt/homebrew/bin/neo4j-cli.
The package manager may revert this change on next upgrade.

To avoid this in future, switch to a self-managed install (so `neo4j-cli update` works directly):
  brew uninstall neo4j-cli
  curl -sSfL https://neo4j.sh/install.sh | bash
```

Per-method substitutions:

| Method | `channelLabel` | `uninstallCmd` |
| ------ | -------------- | -------------- |
| `homebrew` | `Homebrew` | `brew uninstall neo4j-cli` |
| `npm` | `npm/pnpm/yarn` | `npm uninstall -g @neo4j-labs/cli` |
| `pipx` | `pipx` | `pipx uninstall neo4j-cli` |
| `uv` | `uv tool` | `uv tool uninstall neo4j-cli` |

The trailing newline lives inside `ForceOverrideWarning`'s return value so the call site uses `fmt.Fprint`, not `Fprintln` (avoids a double blank line).

### Hint() ripple

Refactoring to `selfManagedBlock` produces two intentional changes to `Hint()` output:

1. The uninstall line loses its `# optional — only needed if PATH still resolves the package-manager binary` annotation (per user direction — uninstall is required for PATH to resolve the new self-managed binary).
2. The npm preamble shortens from `Installed via npm-compatible package manager (global). To upgrade in place, run one of:` to `Installed via npm/pnpm/yarn. To upgrade in place, run one of:` so the channel label matches `channelLabel(InstallMethodNpm)`.

Both are visible in `update` (no-force passthrough) plain-text output. Update existing `install_method_test.go` `Hint` goldens accordingly.

### Placement gotcha

The warning emit site is **between** `result.installMethod = string(method)` (line 438) and the plain-text "Current version" narrative (line 446). It MUST run regardless of `--format` mode (the warning is for humans tailing stderr; structured-output scripts are expected to redirect or discard stderr). Placing it before the `if !isStructuredFormat(...)` block keeps both paths consistent.

### Reuse

- `cmd.ErrOrStderr()` is the same seam used by every other stderr write in `update.go` (lines 314, 338, 446, 487, 556, 572).
- `installScriptCmd` constant (`install_method.go:55`) is reused unchanged.
- Cobra test-buffer pattern: existing `runWithOptsFormat` collapses stdout+stderr into one buffer. New `runWithOptsSplit` mirrors it but uses two buffers — the only change needed for the new tests.

## Acceptance Criteria

- [ ] `update --force` over a Homebrew binary prints the warning block to stderr (including resolved path, revert sentence, `brew uninstall neo4j-cli`, install-script line) BEFORE the swap is attempted.
- [ ] Same for `npm`, `pipx`, `uv` install methods, each with their channel-correct label + uninstall command.
- [ ] `update --force` over an `InstallMethodBinary` install prints nothing extra on stderr (other than the existing `Current version` / `Checking for updates...` narrative).
- [ ] `update --force --format json` over a Homebrew binary emits a valid JSON document on stdout with `install_method: "homebrew"`, `updated: true`, and no warning text bleed; warning appears on stderr only.
- [ ] `update --force --format json | jq .` succeeds (no parse error).
- [ ] `update` (no `--force`) over a Homebrew binary continues to print the existing `Hint()` passthrough (refactored — uninstall line no longer marked optional, npm preamble shortened) and exits 0 without swap.
- [ ] `TestRunUpdate_ForceBypassesPkgMgrCheck` continues to pass with the new warning in the stderr buffer.
- [ ] New table-driven test `TestRunUpdate_Force_NonBinaryMethod_WarnsOnStderr` covers each non-binary method.
- [ ] New test `TestRunUpdate_Force_BinaryMethod_NoWarning` asserts no warning for the binary channel.
- [ ] New test `TestRunUpdate_Force_JSONOutput_WarningOnStderrOnly` asserts JSON purity + stderr warning presence.
- [ ] Unit tests for `channelLabel`, `uninstallCmd`, `selfManagedBlock`, `ForceOverrideWarning` covering all five methods (including binary → empty).
- [ ] `install_method_test.go` `Hint` goldens updated for npm preamble + dropped `optional` annotation. Other Hint goldens (homebrew/pipx/uv) stay bit-identical apart from the dropped annotation.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check` all pass.
- [ ] Changelog entry added under `.changes/unreleased/neo4j-cli-Patch-<ts>.yaml`.

## Out of Scope

- Interactive `y/n` confirmation prompt.
- `warnings[]` array in the JSON output document.
- `SECURITY.md` (parent CLI-155 follow-up).
- Randomised temp-file path on the elevated swap (parent CLI-155 follow-up).
- `--no-warning` / `--quiet` flag.
- Audit log of `--force` invocations.
- Localisation of the warning text.

## Open Questions

None.
