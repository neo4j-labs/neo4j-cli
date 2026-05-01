# PRD: Disable Beta Flag

## Overview

The aura-cli has a `beta-enabled` config key that gates three command groups (`dataapi`, `import`, `deployment`) and switches API path versions. This flag should no longer be user-settable — the beta commands remain in the codebase for easy re-enablement, but users cannot activate them via config.

## Goals

- Prevent users from enabling beta mode via `config set beta-enabled true`
- Silently ignore any existing `beta-enabled: true` in a user's config file on disk
- Keep all beta-gated code intact so the feature can be re-enabled by reverting a small set of changes

## Non-Goals

- Removing the beta command implementations (`dataapi`, `import`, `deployment`)
- Removing the `AuraBetaEnabled()` method, `BetaPathV1()`/`BetaPathV2()` methods, or the conditional in `aura.go`
- Producing warnings or errors for users who already have `beta-enabled: true` on disk

## Requirements

### Functional Requirements

- REQ-F-001: `config set beta-enabled <any value>` must return the same "invalid config key" error as any unrecognised key (e.g. `config set potato does-not-exist`)
- REQ-F-002: `beta-enabled` must be removed from `ValidConfigKeys` in `clicfg.go` so the existing validation path naturally rejects it
- REQ-F-003: `AuraBetaEnabled()` must always return `false`, regardless of what is stored in the config file on disk
- REQ-F-004: `beta-enabled` must not appear in `config list` or `config get` output
- REQ-F-005: The beta-gated commands (`dataapi`, `import`, `deployment`) and the `if cfg.Aura.AuraBetaEnabled()` block in `aura.go` must remain untouched

### Non-Functional Requirements

- REQ-NF-001: Re-enabling beta mode must require only: restoring `beta-enabled` to `ValidConfigKeys` and reverting `AuraBetaEnabled()` to read from viper

## Technical Considerations

**`AuraBetaEnabled()` change** — replace the viper read with `return false`. This is the single source of truth that prevents beta commands from being registered and beta API paths from being used, regardless of what is stored on disk.

**`ValidConfigKeys` change** — remove `"beta-enabled"` from the slice in `clicfg.go`. The `config set` command's existing validation (`IsValidConfigKey`) will then reject it automatically, matching the behaviour of any unknown key.

**`DefaultAuraBetaEnabled` constant** — can be retained in `clicfg.go` to make re-enabling straightforward; it is not user-visible.

**Viper default** — the `Viper.SetDefault("aura.beta-enabled", DefaultAuraBetaEnabled)` call in `setDefaultValues` can be removed since the value is no longer read; removing it also prevents the key appearing in any viper-backed config dump. However it is not strictly necessary to remove it — whatever is simplest for re-enablement.

**API path selection** — `getVersionPath` in `api.go` still branches on `AuraBetaEnabled()`. With the method hardcoded to `false`, beta paths (`v1beta5`, `v2beta1`) will never be used. No changes needed to `api.go`.

**Tests** — existing tests that call `config set beta-enabled true` and assert success (`TestSetBetaEnabledConfig`) must be updated to assert the "invalid config key" error. Tests that call `helper.SetConfigValue("aura.beta-enabled", true)` directly (bypassing the CLI) are testing command behaviour that depends on beta mode already being on — these tests remain valid and do not need changing, as they exercise code paths that still exist and need coverage.

## Acceptance Criteria

- [ ] `aura config set beta-enabled true` returns `Error: invalid config key specified: beta-enabled`
- [ ] `aura config set beta-enabled false` returns `Error: invalid config key specified: beta-enabled`
- [ ] `aura config list` output does not include a `beta-enabled` field
- [ ] `aura config get beta-enabled` returns `Error: invalid config key specified: beta-enabled`
- [ ] A user with `beta-enabled: true` in their config file sees no error or warning at startup and cannot access beta commands
- [ ] `aura dataapi`, `aura import`, and `aura deployment` are not registered and return "unknown command" errors
- [ ] All existing tests pass (`make test`)

## Out of Scope

- Removing beta command source files
- Migrating beta commands to GA
- Communicating the deprecation to existing users

## Open Questions

None.
