# PRD: Filter Config List Output to Valid Keys

## Overview

The `config list` command prints the entire `aura` section of the config file by calling `viper.Get("aura")`, which returns a raw map that includes any keys stored on disk — including unrecognised or previously-valid keys (e.g. a leftover `beta-enabled: true`). The output should be filtered so only keys present in `ValidConfigKeys` are shown.

## Goals

- Ensure `config list` only surfaces keys that are currently valid and user-facing.
- Prevent stale or unrecognised config keys from leaking into CLI output.

## Non-Goals

- Changing validation logic for `config set` or `config get`.
- Modifying the on-disk config file format.
- Removing unrecognised keys from the config file automatically.

## Requirements

### Functional Requirements

- REQ-F-001: `config list` output must contain only keys present in `ValidConfigKeys`.
- REQ-F-002: Unrecognised keys that exist in the config file on disk must be silently omitted from `config list` output.
- REQ-F-003: The output structure and JSON encoding must remain unchanged — a flat JSON object keyed by config key name.
- REQ-F-004: The filtered output must use the canonical values from viper (i.e. respecting defaults and env var overrides), not raw file bytes.

### Non-Functional Requirements

- REQ-NF-001: Adding a new valid config key to `ValidConfigKeys` must automatically include it in `config list` output with no further changes required.

## Technical Considerations

**Current implementation** — `print()` in `common/clicfg/clicfg.go` calls `config.viper.Get("aura")` which returns the entire `aura` subtree as `interface{}` (a `map[string]interface{}`). This is JSON-encoded directly.

**Proposed approach** — Replace the `viper.Get("aura")` call with a loop over `ValidConfigKeys`, building a `map[string]interface{}` by calling `config.viper.Get("aura.<key>")` for each key. Encode that filtered map instead. This keeps `ValidConfigKeys` as the single source of truth.

**Impact on `PrintAuraProjects`** — The `print` method is shared between `PrintAuraConfig` and `PrintAuraProjects`. The filtering only applies to the `aura` config path, not `aura-projects`. The two print paths should be separated so the filtering does not accidentally affect project output.

**Output format** — The existing test asserts a flat JSON object (`{"auth-url": "...", "base-url": "...", "output": "default"}`). The filtered map approach produces the same structure; key ordering may differ but `AssertOutJson` should handle that if it does a semantic comparison (verify this).

## Acceptance Criteria

- [ ] `config list` output contains exactly the keys in `ValidConfigKeys` and no others.
- [ ] A user with `beta-enabled: true` on disk sees no `beta-enabled` field in `config list` output.
- [ ] Values shown for each key match viper's resolved value (default or on-disk, with env var overrides respected).
- [ ] `config list` output remains valid JSON with the same flat object structure.
- [ ] `PrintAuraProjects` behaviour is unchanged.
- [ ] All existing tests pass (`make test`).

## Out of Scope

- Migrating `aura-projects` output through the same filter.
- Stripping unrecognised keys from the config file on disk.
- Surfacing a warning when unrecognised keys are detected.

## Open Questions

None.
