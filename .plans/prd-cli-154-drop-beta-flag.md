# PRD: Drop `flag.aura-beta` (CLI-154)

## Overview

Remove the `flag.aura-beta` feature flag and all its scaffolding from the codebase. The commands it previously gated are already unconditionally registered; the only remaining behavioural effect of the flag is a conditional `AuraApiVersion1 → v1beta5` API path redirect in `api.go`. Removing the flag means `AuraApiVersion1` always uses `v1`, which is correct: every API surface that legitimately needs `v1beta5` already requests it via explicit `AuraApiVersionBeta1`. A config migration cleans the keys (`flag.aura-beta`, `aura.beta-enabled`) out of users' `config.json` files.

## Goals

- Permanently delete the `flag.aura-beta` registry entry and all production code that reads it.
- Simplify `getVersionPath` in `api.go`: `AuraApiVersion1` always maps to `"v1"` with no conditional branch.
- Add a config migration (v1) to remove `flag.aura-beta` and legacy `aura.beta-enabled` keys from users' `config.json` on the next CLI invocation.
- Remove all test references to `flag.aura-beta` (74 occurrences across org, workspace, project, utils, config sub-packages).
- Delete stale `neo4j-cli/internal/subcommands/config/project_test.go` (tests a non-existent `config project` source command).
- Leave all other infrastructure (`FlagSet`, `Registry`, `configmigrate` engine, `AuraApiVersionBeta1`, graphql commands) untouched.

## Non-Goals

- Renaming or removing `AuraApiVersionBeta1` — graphql commands still use it to call `v1beta5` explicitly.
- Implementing or removing the `neo4j config project` command — out of scope.
- Changing any API response contract, command flags, or output format.
- Removing the `FlagSet`/`Registry` mechanism itself — it remains for future flags.

## Requirements

### Functional Requirements

- REQ-F-001: Delete the `"flag.aura-beta"` entry from `var Registry` in `common/clicfg/flags.go`. Callers that call `Enabled("flag.aura-beta")` will receive `false` via the unknown-key path — but no caller should remain after this PR.
- REQ-F-002: In `getVersionPath` (`neo4j-cli/aura/internal/api/api.go` lines 159–173), remove the `if cfg.Flags.Enabled("flag.aura-beta")` branch from the `AuraApiVersion1` case so it unconditionally returns `"v1"`. Inline `"v1beta5"` in the `AuraApiVersionBeta1` case and `"v2beta1"` in the `AuraApiVersion2` case (see technical considerations).
- REQ-F-003: Remove `BetaPathV1() string` and `BetaPathV2() string` methods from `common/clicfg/clicfg.go` (lines ~317–321). They are only called inside `getVersionPath`, which is being inlined.
- REQ-F-004: Add migration version 1 to `common/configmigrate/migrations.go`. The `Apply` function must use `sjson.DeleteBytes` to delete both `"flag\.aura-beta"` (dot-escaped sjson path) and `"aura.beta-enabled"` from the config bytes. The migration is a no-op when neither key is present.
- REQ-F-005: Delete `neo4j-cli/internal/subcommands/config/project_test.go` — it references a `neo4j config project` command that has no source file and will never be registered without the beta flag.
- REQ-F-006: Remove all remaining `flag.aura-beta` test references. Specifically:
  - `neo4j-cli/internal/subcommands/config/get_test.go` — remove the two test cases that read `flag.aura-beta` (lines ~108–121).
  - `neo4j-cli/internal/subcommands/config/set_test.go` — remove the test cases that set `flag.aura-beta true/false` (lines ~115–124).
  - `neo4j-cli/aura/internal/subcommands/config/get_test.go` — remove the test case that sets `flag.aura-beta` to test `beta-enabled` rejection (line ~66). **Keep** the test asserting `beta-enabled` is rejected as an invalid argument — that assertion is still correct; just remove the `SetConfigValue("flag.aura-beta", true)` setup line.
  - `neo4j-cli/aura/internal/subcommands/config/set_test.go` — keep the two test cases that assert `beta-enabled` is rejected; they remain valid.
  - `neo4j-cli/aura/internal/subcommands/config/list_test.go` — keep the test asserting `beta-enabled` is filtered from list output; remove just the `SetConfigValue("flag.aura-beta", true)` setup if present.
  - `neo4j-cli/aura/internal/subcommands/organization/{get,list}_test.go` — remove `SetConfigValue("flag.aura-beta", true)` lines (commands are unconditionally registered; the beta flag was only needed while they were gated).
  - `neo4j-cli/aura/internal/subcommands/workspace/{use,list,validate}_test.go` — same: remove `SetConfigValue`/`SetForTest` calls for `flag.aura-beta`.
  - `neo4j-cli/aura/internal/subcommands/project/list_test.go` — remove `SetConfigValue("flag.aura-beta", true)` calls.
  - `neo4j-cli/aura/internal/subcommands/utils/resolve_test.go` — remove `cfg.Flags.SetForTest("flag.aura-beta", true)` and the `"beta-enabled": true` literal in the inline config JSON.
- REQ-F-007: Remove or replace any test in `common/clicfg/flags_test.go` that specifically validates the `flag.aura-beta` registry entry (e.g. `TestRegistry_AuraBetaEntry`). If that test is parameterised over all Registry entries, no change is needed; if it hardcodes `flag.aura-beta`, delete or update it.
- REQ-F-008: Regenerate the skill bundle after any command-tree or Long/Example changes: `go generate ./neo4j-cli/internal/skill/...`.
- REQ-F-009: ~~Add a `Patch` changelog entry~~ — no changelog entry required. The `flag.aura-beta` feature was never released to end users; this is an internal cleanup with no user-visible behaviour change.

### Non-Functional Requirements

- REQ-NF-001: `make test`, `make fmt-check`, and `make lint` all pass after the change.
- REQ-NF-002: No change to any command's user-visible behaviour, flags, or output.
- REQ-NF-003: Migration is warn-and-continue — a missing or unwritable `config.json` must never crash the CLI.

## Technical Considerations

### `getVersionPath` simplification (`api.go` lines 159–173)

Before:
```go
func getVersionPath(cfg *clicfg.Config, version AuraApiVersion) string {
    switch version {
    case AuraApiVersion1:
        if cfg.Flags.Enabled("flag.aura-beta") {
            return cfg.Aura.BetaPathV1()
        }
        return "v1"
    case AuraApiVersionBeta1:
        return cfg.Aura.BetaPathV1()
    case AuraApiVersion2:
        return cfg.Aura.BetaPathV2()
    default:
        panic(fmt.Sprintf("version not set in requests %s", version))
    }
}
```

After:
```go
func getVersionPath(version AuraApiVersion) string {
    switch version {
    case AuraApiVersion1:
        return "v1"
    case AuraApiVersionBeta1:
        return "v1beta5"
    case AuraApiVersion2:
        return "v2beta1"
    default:
        panic(fmt.Sprintf("version not set in requests %s", version))
    }
}
```

The `cfg` parameter can be dropped from the signature if `flag.aura-beta` is the only reason it was threaded through. Check the full call site to confirm.

### Config migration (`common/configmigrate/migrations.go`)

Append one entry to the currently empty `migrations` slice:
```go
var migrations = []Migration{
    {
        Version:     1,
        Description: "remove retired flag.aura-beta and aura.beta-enabled keys",
        Apply: func(data []byte) ([]byte, error) {
            // sjson requires dots escaped as \. to treat them as literal
            data, err := sjson.DeleteBytes(data, `flag\.aura-beta`)
            if err != nil {
                return nil, err
            }
            return sjson.DeleteBytes(data, "aura.beta-enabled")
        },
    },
}
```

Note: `sjson.DeleteBytes` with `"aura.beta-enabled"` treats the dot as a path separator, targeting the nested key `aura` → `beta-enabled`. Verify this matches the actual JSON shape (`{"aura": {"beta-enabled": true}}`) before committing.

### Test cleanup scope

74 test-file lines reference `flag.aura-beta`. Most are `SetConfigValue`/`SetForTest` guard-rails carried over from when commands were command-registration-gated. After the gate is gone, simply delete these setup lines and verify the tests still pass (the commands are registered regardless, and the mock HTTP paths in those tests use `v2beta1`, which is `AuraApiVersion2` routing — unaffected by this change).

### Skill bundle

`go generate ./neo4j-cli/internal/skill/...` is required if any `Long`/`Example` field changes, any command is added/removed, or `ValidFormatValues` changes. Removing the beta gate doesn't change the command tree (no new/removed commands), but double-check with `TestGenerator_RoundTrip` as the gate.

## Acceptance Criteria

- [ ] `var Registry` in `common/clicfg/flags.go` contains no `"flag.aura-beta"` entry.
- [ ] `getVersionPath` in `api.go` has no reference to `cfg.Flags.Enabled` or `flag.aura-beta`.
- [ ] `BetaPathV1()` and `BetaPathV2()` no longer exist in `clicfg.go`.
- [ ] `common/configmigrate/migrations.go` has exactly one migration (v1) that deletes both `flag.aura-beta` and `aura.beta-enabled`.
- [ ] `neo4j-cli/internal/subcommands/config/project_test.go` is deleted.
- [ ] `grep -r "flag.aura-beta" .` returns zero results in non-comment production or test Go files.
- [ ] `grep -r "aura.beta-enabled\|beta-enabled" .` returns zero results outside of migration code and the aura/config tests that assert the key is rejected.
- [ ] `make test` passes (including `TestGenerator_RoundTrip`).
- [ ] `make fmt-check` passes.
- [ ] `make lint` passes.
- [ ] No changelog entry is added (flag was never user-facing).

## Out of Scope

- Renaming `AuraApiVersionBeta1` — graphql commands depend on it.
- Removing the `FlagSet`/`Registry` infrastructure.
- Implementing `neo4j config project` or any other previously-gated feature.
- Changes to the `neo4j-cli aura` command tree beyond what's needed to delete the flag reads.

## Open Questions

None — scope is fully defined by CLI-154 and the Q&A above.
