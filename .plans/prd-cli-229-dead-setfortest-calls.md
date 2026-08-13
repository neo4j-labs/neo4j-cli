# PRD: Remove dead `SetForTest("flag.aura-beta")` calls and guard `SetForTest` (CLI-229)

## Overview

Four calls to `cfg.Flags.SetForTest("flag.aura-beta", true)` remain in `neo4j-cli/aura/internal/api/organization_test.go` (lines 88, 146, 214, 273). They are inert no-ops: `clicfg.Registry` is empty on `main`, and `FlagSet.Enabled` (`common/clicfg/flags.go:68`) returns `false` for an unregistered key *before* consulting `f.overrides`, so the override is never read.

This PRD deletes those dead calls, adds a panic guard to `SetForTest` so the next retired flag surfaces its leftover overrides at `go test` time instead of silently no-op'ing, and corrects two stale claims in `.agents/feature-flags.md`.

### Assessment of the Linear ticket

CLI-229 is **valid** but overstates impact. Corrections carried into this PRD:

- The ticket says "the only production entry is `flag.mcp-server`". Wrong — `Registry` is `map[string]Flag{}`, completely empty on `main`. `flag.mcp-server` lives on the unmerged `oskar/cli-218-mcp-server` branch.
- The ticket calls this a "latent coverage gap". It is not. CLI-154 (`d1fb7e15`) deleted the flag's only production reader; `getVersionPath` (`neo4j-cli/aura/internal/api/api.go:203-214`) now unconditionally returns `"v1"` / `"v1beta5"` / `"v2beta1"` with no `cfg.Flags.Enabled` call anywhere in the tree. There is no flag-on path left to be untested. The four lines are pure dead code.
- Severity is **Low**, not Medium. No production behaviour changes.

### Root cause

CLI-154's own acceptance criterion — `grep -r "flag.aura-beta" .` returns zero hits in Go files (`.plans/archive/prd-cli-154-drop-beta-flag.md:130`) — was never actually run. Its task list enumerated test files by hand and missed `neo4j-cli/aura/internal/api/organization_test.go`, because `api/` is not under `subcommands/`. The guard in REQ-F-002 makes that class of miss self-reporting.

## Goals

- Remove the four dead `SetForTest` calls so `grep` over `*.go` files is clean of `aura-beta` outside `common/configmigrate/`.
- Make a `SetForTest` call with an unregistered key fail loudly, so retiring a future flag cannot leave silently-dead test overrides behind.
- Correct the two stale statements in `.agents/feature-flags.md` that describe `flag.aura-beta` as live.

## Non-Goals

- Changing `FlagSet.Enabled`'s resolution order.
- Re-registering `flag.aura-beta` or adding any legacy alias.
- Any production behaviour change, changelog entry, or skill-bundle regeneration.

## Requirements

### Functional Requirements

- REQ-F-001: Delete the four `cfg.Flags.SetForTest("flag.aura-beta", true)` lines from `neo4j-cli/aura/internal/api/organization_test.go` (currently lines 88, 146, 214, 273). Nothing replaces them — the surrounding assertions already cover the only behaviour that exists. No other edit to that file; do not touch the `buildOrgTestServer` / `buildTestConfig` helpers or the `v2beta1` mock paths.

- REQ-F-002: In `common/clicfg/flags.go`, add a Registry precondition to `SetForTest` (`flags.go:113`) that panics when the key is not registered:

  ```go
  func (f *FlagSet) SetForTest(name string, value bool) {
      if _, ok := Registry[name]; !ok {
          panic(fmt.Sprintf("clicfg.SetForTest: %q is not in Registry — the override would be ignored by Enabled", name))
      }
      f.mu.Lock()
      defer f.mu.Unlock()
      ...
  }
  ```

  The check must run **before** `f.mu.Lock()` so the panic does not unwind through a held mutex. Add `"fmt"` to the import block (`flags.go:6-16`) in the stdlib group, above `"log/slog"`.

- REQ-F-003: Update the `SetForTest` doc comment to state the precondition. Drop the now-meaningless trailing sentence "Matches the previous `AuraConfig.SetBetaEnabled` contract" — that contract predates the registry and the guard changes it.

- REQ-F-004: Add `TestFlagSet_SetForTest_PanicsOnUnregisteredKey` to `common/clicfg/flags_test.go`. It must reuse the existing `registerTestFlag(t)` (`flags_test.go:33`) and `newFlagSetForTest(t)` (`flags_test.go:25`) helpers, and assert both directions:
  - `assert.NotPanics` for the registered sentinel `"flag.test-sentinel"`.
  - `assert.Panics` (or `assert.PanicsWithValue` against the exact message) for an unregistered key such as `"flag.does-not-exist"`.

  `registerTestFlag` already restores the original `Registry` via `t.Cleanup`, so no extra teardown is needed.

- REQ-F-005: In `.agents/feature-flags.md`, rewrite the "Migrating aura.beta-enabled" bullet (line ~40) to past tense: the migration completed in CLI-136, the flag was **retired** in CLI-154, and both `flag.aura-beta` and the legacy `aura.beta-enabled` key are now physically stripped from user configs by config-migration v1 (`common/configmigrate/migrations.go:32`). The current text claims "production + tests now use `flag.aura-beta` via the Registry", which is false.

- REQ-F-006: In `.agents/feature-flags.md` line 8, remove `flag.aura-beta` from the naming-examples list (the flag no longer exists). Keep the remaining hypothetical examples (`flag.docker-command`, `flag.secrets-os-keystore`).

- REQ-F-007: In `.agents/feature-flags.md`, add one line under the "Unknown / removed keys" section recording that `SetForTest` panics on an unregistered key, so retiring a flag surfaces any leftover test overrides. This documents the REQ-F-002 behaviour where a future flag-retirer will look.

### Non-Functional Requirements

- REQ-NF-001: No production behaviour change. The only non-test production edit is the `SetForTest` guard, and `SetForTest` has zero non-test callers (verified: the only call sites are the four being deleted plus one in `flags_test.go:103`).

- REQ-NF-002: `make test`, `make fmt-check`, and `make lint` must all pass — the project's stated final gates (AGENTS.md).

- REQ-NF-003: No changelog entry. Internal test/doc cleanup with zero user-visible change, matching the rationale recorded in CLI-154 REQ-F-009.

- REQ-NF-004: No `go generate` run required. No cobra `Short` / `Long` / `Example` field is touched, `ValidFormatValues` is unchanged, and `.agents/**` does not feed the skill bundle (verified: zero `feature-flags` or `aura-beta` hits under `neo4j-cli/internal/skill/bundle/`). `TestGenerator_RoundTrip` should stay green untouched.

## Technical Considerations

**Why panic rather than returning an error or taking `*testing.T`.** `SetForTest` currently has signature `(name string, value bool)` with no error return and no `testing` dependency — keeping `common/clicfg` free of a `testing` import matters because the package is in the production build graph. A panic preserves both properties and still fails the test run immediately and unmissably. Returning an error would be silently ignorable at exactly the call sites this guard exists to catch.

**Why `Enabled`'s ordering must not change.** The Linear ticket's fix option 3 proposes making `Enabled` consult `f.overrides` before the Registry lookup. Reject it: it inverts the registry-as-source-of-truth invariant and directly contradicts two existing tests — the `"unknown name returns false"` case in `TestFlagSet_Enabled_Precedence` and `TestFlagSet_Enabled_UnknownKeyDebugLog`. The defect is that `SetForTest` *accepts* a key `Enabled` will never honour, not that `Enabled` ignores it.

**Panic placement vs. the mutex.** `FlagSet` guards `overrides` with `f.mu`. Panicking while holding the lock would leave it held during unwind and could deadlock a subsequent deferred read in the same test binary. Put the Registry check first, before any lock acquisition.

**Empty Registry means the guard is currently absolute.** With `Registry` empty on `main`, *any* `SetForTest` call now panics except inside a test that has installed a sentinel via `registerTestFlag`. That is the intended and correct end state — after REQ-F-001 the only remaining caller is `flags_test.go:103`, which is inside `registerTestFlag`'s scope. Anyone adding a real flag registers it first, so the guard costs them nothing.

**Deliberately left alone.** `.agents/feature-flags.md` line 31 still cites `flag.aura-beta` as the example key for the `helper.SetConfigValue` mechanism. Out of scope by decision — it does not affect the Go-file grep gate. The parallel `SetConfigValue` silent-drop path (`neo4j-cli/aura/internal/test/testutils/auratesthelper.go:100`) is likewise not guarded here; it is a generic config setter, not flag-specific, so guarding it would need a `flag.` prefix check and touches many aura tests.

## Acceptance Criteria

- [ ] `grep -rn "aura-beta" --include='*.go' . | grep -v configmigrate` returns zero results.
- [ ] `neo4j-cli/aura/internal/api/organization_test.go` contains no `SetForTest` call.
- [ ] `common/clicfg/flags.go` `SetForTest` panics on an unregistered key, with the Registry check preceding `f.mu.Lock()`.
- [ ] `common/clicfg/flags.go` imports `fmt`; `SetForTest`'s doc comment states the precondition and no longer references `AuraConfig.SetBetaEnabled`.
- [ ] `TestFlagSet_SetForTest_PanicsOnUnregisteredKey` exists in `common/clicfg/flags_test.go` and asserts both the panicking and non-panicking cases.
- [ ] `go test ./neo4j-cli/aura/internal/api/... -run 'TestListOrganizations|TestGetOrganization'` passes.
- [ ] `go test ./common/clicfg/...` passes.
- [ ] `.agents/feature-flags.md` line ~40 describes `flag.aura-beta` as retired in CLI-154, not live.
- [ ] `.agents/feature-flags.md` line 8 no longer lists `flag.aura-beta` as a naming example.
- [ ] `.agents/feature-flags.md` documents the `SetForTest` panic under "Unknown / removed keys".
- [ ] `make test`, `make fmt-check`, `make lint` all pass.
- [ ] No file added under `.changes/unreleased/`; no diff under `neo4j-cli/internal/skill/bundle/`.

## Out of Scope

- `FlagSet.Enabled`'s resolution order — correct as-is, do not touch.
- `common/configmigrate/*` references to `flag.aura-beta` — that code *is* the migration that deletes the key; it must keep naming it.
- `.agents/feature-flags.md` line 31 (`helper.SetConfigValue` example) — decided out of scope.
- Guarding `AuraTestHelper.SetConfigValue` against unregistered `flag.*` keys — noted as a residual gap, not addressed here.
- `.plans/archive/**`, `CHANGELOG.md`, `.changes/**` — historical records, left as written.
- Re-registering `flag.aura-beta` or adding a legacy alias to any successor flag.

## Open Questions

- **Residual gap, tracked not fixed:** `AuraTestHelper.SetConfigValue("flag.<retired>", true)` (`neo4j-cli/aura/internal/test/testutils/auratesthelper.go:100`) is the second silent-drop path — it writes the key into helper config JSON, but `Enabled` still ignores it for an unregistered key. Explicitly excluded from this PRD; worth a follow-up ticket if a future flag retirement leaves dead `SetConfigValue` calls behind.
- **Linear hygiene:** CLI-229 should be moved out of Triage and re-graded **Low**, with the description corrected on three points — `Registry` is empty on `main` (not holding `flag.mcp-server`), there is no coverage gap (CLI-154 removed the gated branch), and fix option 3 is rejected. Not a code task; flagged so it is not forgotten.
