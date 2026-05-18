# Feature Flags

How we name, gate, test, and retire experimental behaviour in `neo4j-cli`; implementation tracked in CLI-125, user-side cleanup in CLI-134.

## Key naming

- Prefix `flag.`, then `<area>-<feature>` (lowercase kebab). One area token, one feature token.
- Examples: `flag.aura-beta`, `flag.docker-command`, `flag.secrets-os-keystore`.
- Runtime source of truth: a Go `map[string]Flag` registry at `common/clicfg/flags.go` (future home). This doc is the narrative reference.

## Defaults & lifecycle

- Default `false` on introduction. Opt-in only.
- No kill-switches. No "default true, set false to disable" states. Rollbacks ship via release, not a flipped default.
- On GA: delete the flag AND its gated branch in the same PR. Never flip default to `true`.
- Registry entry carries: name, default, owner, what it gates, introduced-in version, expected removal condition.

## Override surface

High → low precedence:

1. Env var `NEO4J_CLI_FLAG_<AREA>_<FEATURE>=1` (dot/dash → underscore, uppercased; ideal for CI).
2. Config file via `neo4j-cli config set flag.<area>-<feature> true`.

Explicit: no `--flag` CLI option. CI / one-shot use is covered by the env var at zero plumbing cost.

## Testing

- Every flag ships with tests for BOTH states while it lives.
- CI runs the flag-on path explicitly (test build step or env var) until the flag is removed.
- Aura-side flags use the existing test-helper pattern at `common/clicfg/auratesthelper.go:68-70`.

## Unknown / removed keys

- Silent at runtime (debug-log only). Old user configs survive CLI upgrades without warnings.
- Stripping retired keys from `config.json` is owned by CLI-134 (config migration), not the registry.

## Migrating aura.beta-enabled

- `aura.beta-enabled` becomes `flag.aura-beta` once the registry lands.
- Reads of the old key remain accepted (debug-log "deprecated") until CLI-134 ships.
- Touch points:
  - `common/clicfg/clicfg.go:32` — `DefaultAuraBetaEnabled` constant.
  - `common/clicfg/clicfg.go:281` — struct field on `AuraConfig`.
  - `common/clicfg/clicfg.go:371-377` — `SetBetaEnabled` / `AuraBetaEnabled` getter+setter.
  - `common/clicfg/auratesthelper.go:68-70` — test seam that reads `aura.beta-enabled` from the helper JSON.

## See also

- Linear CLI-125 — this decision.
- Linear CLI-134 — config migration (owns user-side cleanup of retired flag keys).
- `AGENTS.md` "Feature Flag Notes" links here.
