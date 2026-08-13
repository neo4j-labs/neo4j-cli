# Feature Flags

How we name, gate, test, and retire experimental behaviour in `neo4j-cli`; implementation tracked in CLI-125, user-side cleanup in CLI-134.

## Key naming

- Prefix `flag.`, then `<area>-<feature>` (lowercase kebab). One area token, one feature token.
- Examples: `flag.docker-command`, `flag.secrets-os-keystore`.
- Runtime source of truth: the `Registry` map in `common/clicfg/flags.go`. This doc is the narrative reference.

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
- Aura-side tests toggle flags by writing the dotted key (e.g. `flag.aura-beta`) into the helper config JSON via `helper.SetConfigValue` in `neo4j-cli/aura/internal/test/testutils/auratesthelper.go`; the registry's viper binding picks it up — no Go-side bridge.

## Unknown / removed keys

- Silent at runtime (debug-log only). Old user configs survive CLI upgrades without warnings.
- Stripping retired keys from `config.json` is owned by the config-migration subsystem (see [`.agents/config-migrations.md`](config-migrations.md)), not the registry.
- `SetForTest` panics on an unregistered key, so retiring a flag surfaces leftover test overrides.

## Migrating aura.beta-enabled

- Migration completed in CLI-136; flag retired in CLI-154. Both `flag.aura-beta` and legacy `aura.beta-enabled` are stripped from user configs by config-migration v1 (`common/configmigrate/migrations.go:32`).

## See also

- Linear CLI-125 — this decision.
- [`.agents/config-migrations.md`](config-migrations.md) — owns user-side cleanup of retired flag keys (CLI-134).
- `AGENTS.md` "Feature Flag Notes" links here.
