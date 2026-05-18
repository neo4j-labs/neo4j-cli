# PRD: Implement feature flag system (CLI-136)

Linear: https://linear.app/neo4j/issue/CLI-136
Related: CLI-125 / PR #124 (convention doc, merged) — `.agents/feature-flags.md`
Related: CLI-134 (config migration) — owns user-side cleanup of retired flag keys; not blocked by this PR.
Source plan: `/Users/oskarhane/.claude/plans/alright-time-to-get-curried-breeze.md`

## Overview

CLI-125 / PR #124 landed `.agents/feature-flags.md` codifying the convention (`flag.<area>-<feature>` dotted keys, default-false opt-in, env-var + config-file overrides, registry at `common/clicfg/flags.go`, silent-on-unknown, no kill-switches). That PR was docs-only.

CLI-136 implements the runtime. Today exactly one flag exists — `aura.beta-enabled`, runtime-only, set by tests via a bridge in `neo4j-cli/aura/internal/test/testutils/auratesthelper.go:62-66`. The convention doc pre-records its migration path to `flag.aura-beta`.

This PR delivers:

1. A `common/clicfg/flags.go` registry — `map[string]Flag` source of truth, env-var binding via viper, override surface, in-process test override seam.
2. `config set` integration for registered `flag.*` keys (hidden from `config list` / `config get` no-arg listing per the agreed "Yes for set, hide from list" UX).
3. Migration of `aura.beta-enabled` → `flag.aura-beta` in all production call sites AND the 33 test files that toggle it. The legacy key is retained as a debug-logged fallback in the registry until CLI-134 ships physical config-file cleanup.

Outcome: future flags add one entry to the registry and call `cfg.Flags.Enabled("flag.<area>-<feature>")` at the gate. Zero per-flag struct fields, getters, or test bridges.

## Goals

- Land a single source of truth for feature flags at `common/clicfg/flags.go`, matching the convention doc.
- Provide the two documented override surfaces — env var (`NEO4J_CLI_FLAG_<AREA>_<FEATURE>`) and config file (`flag.<area>-<feature>`) — with documented precedence.
- Migrate `aura.beta-enabled` end-to-end (production + tests) so the registry has a real, in-use flag from day one.
- Keep the diff reviewable by reusing existing viper / sjson / clierr patterns and avoiding new abstractions beyond the registry itself.

## Non-Goals

- A `--flag` CLI option for per-invocation override (explicitly excluded by the convention doc).
- Non-boolean flag types (string / enum / int). Boolean only this PR; defer until a concrete need.
- CLI-134's config migration (physically stripping retired keys from `config.json`).
- Renaming or reworking other ad-hoc gating sites (e.g. the telemetry env-var disable at `common/clicfg/clicfg.go:132`).
- Skill-bundle restructuring. Cobra tree is unchanged; bundles should not drift (`make generate-check` is the gate).
- A user-facing `flag.aura-beta` mention in any README, CONTRIBUTING, or skill bundle text. Surfacing the override is a follow-up if/when we expose it to end users; this PR ships the runtime only.
- Telemetry events for flag reads.

## Requirements

### Functional Requirements

- **REQ-F-001:** New file `common/clicfg/flags.go` defines:
  - `Flag` struct with fields `Name string`, `Default bool`, `Owner string`, `Gates string`, `IntroducedIn string`, `RemovalCondition string`, `LegacyKey string` (optional).
  - `Registry map[string]Flag` — exported, populated at package init with one initial entry `"flag.aura-beta"` (defaults: `Default=false`, `Owner="aura-cli team"`, `Gates="aura {dataapi, import, deployment} subcommands; v1beta5 API path"`, `IntroducedIn=<next-release semver>`, `RemovalCondition="Aura beta features ship to GA"`, `LegacyKey="aura.beta-enabled"`).
  - `FlagSet` struct holding a `*viper.Viper`, an `overrides map[string]bool`, and a `legacyLogOnce map[string]*sync.Once` for one-shot-per-process legacy debug logging.
  - `(*FlagSet).Enabled(name string) bool` with this resolution order:
    1. in-process override from `SetForTest`
    2. `viper.IsSet(name)` → `viper.GetBool(name)` (env-bound + config-file values)
    3. `spec.LegacyKey != "" && viper.IsSet(LegacyKey)` → `viper.GetBool(LegacyKey)` AND `slog.Debug` once per process (`"feature flag read from deprecated key"`, with `deprecated` and `new` attrs)
    4. `spec.Default`
    5. Unknown `name` (not in `Registry`) → `slog.Debug("feature-flag lookup for unregistered key", "key", name)` and return `false`.
  - `(*FlagSet).SetForTest(name string, value bool)` — sets the in-process override (lazy-init the map).
  - `(*FlagSet).SetFromConfigCmd(name, value string) error` — used by `config set`: rejects unknown `name` with `clierr.NewUsageError("invalid config key: %q", name)`, rejects values other than `"true"`/`"false"` with `clierr.NewUsageError("invalid value for %q: %s (valid values: true, false)", name, value)`, writes via `sjson.Set(data, name, parsedBool)` + `fileutils.WriteFile`. (`name` here is the full dotted key, e.g. `flag.aura-beta`.)
  - `FlagNameToEnv(name string) string` — pure function: strip leading `flag.` → uppercase remainder → replace `-` with `_` → return `"NEO4J_CLI_FLAG_" + result`. Examples: `flag.aura-beta` → `NEO4J_CLI_FLAG_AURA_BETA`; `flag.docker-command` → `NEO4J_CLI_FLAG_DOCKER_COMMAND`.

- **REQ-F-002:** New file `common/clicfg/flags_test.go` is table-driven and covers:
  - `TestFlagSet_Enabled` precedence: override > env > config file > legacy alias > default; unknown name returns false.
  - `TestFlagSet_LegacyDebugLogIsOneShot` — the debug log for the deprecated key fires once per process per legacy key (tested via a per-test `FlagSet` instance, asserting the `sync.Once`-gated handler runs exactly once across multiple `Enabled` calls).
  - `TestFlagNameToEnv` table — at minimum `flag.aura-beta`, `flag.docker-command`, `flag.secrets-os-keystore`.
  - `TestFlagSet_SetFromConfigCmd` — accepts `"true"`/`"false"`, rejects unknown key, rejects invalid value, writes to the in-memory fs via `testfs.GetTestFs`.

- **REQ-F-003:** `common/clicfg/clicfg.go` is edited as follows:
  - Delete `DefaultAuraBetaEnabled` constant (currently `clicfg.go:31`).
  - Delete `betaEnabled bool` field on `AuraConfig` (currently `clicfg.go:255`).
  - Delete `(*AuraConfig).SetBetaEnabled` and `(*AuraConfig).AuraBetaEnabled` methods (currently `clicfg.go:341-347`).
  - Add `Flags *FlagSet` field to `Config` struct (currently `clicfg.go:47-54`).
  - Add `FlagScope ConfigScope = "flag"` to the existing `ConfigScope` enum (currently `clicfg.go:40-45`).
  - In `bindEnvironmentVariables` (currently `clicfg.go:237-240`): iterate `Registry`; for each entry call `Viper.BindEnv(name, FlagNameToEnv(name))`; if `spec.LegacyKey != ""`, also `Viper.BindEnv(spec.LegacyKey, FlagNameToEnv("flag."+strings.TrimPrefix(spec.LegacyKey, "aura.")))` *(legacy alias env-var binding — keeps `NEO4J_CLI_FLAG_AURA_BETA` working whether the underlying viper key is the old or new name)*. Errors from `BindEnv` may be `//nolint:errcheck`-ignored matching the existing pattern.
  - In `setDefaultValues` (currently `clicfg.go:242-248`): iterate `Registry`; `Viper.SetDefault(name, spec.Default)`; if `spec.LegacyKey != ""`, also `Viper.SetDefault(spec.LegacyKey, spec.Default)` (keeps `viper.IsSet` semantics correct so the legacy branch fires only when the legacy key is explicitly present in config or env).
  - In `NewConfig`: after the viper setup block, initialise `Flags: &FlagSet{viper: Viper}` on the returned `Config`.
  - Extend `ResolveConfigKey` (currently `clicfg.go:482-506`): if `strings.HasPrefix(key, "flag.")` → look up `key` in `Registry`; unknown returns `clierr.NewUsageError("invalid config key: %q", key)`; known returns `(FlagScope, key, nil)` (the "bare key" for flag scope is the full dotted name — `flag.aura-beta` — because flag names ARE the dotted key).

- **REQ-F-004:** `common/clicfg/clicfg_test.go` `TestResolveConfigKey` gains two new rows:
  - `flag.aura-beta` → `(FlagScope, "flag.aura-beta", nil)`.
  - `flag.unknown-thing` → error `invalid config key: "flag.unknown-thing"`.

- **REQ-F-005:** `neo4j-cli/internal/subcommands/config/set.go` (currently `set.go:34-55`) adds `case clicfg.FlagScope:` to the switch on resolved scope → calls `cfg.Flags.SetFromConfigCmd(bareKey, value)` and returns its error (setting `cmd.SilenceUsage = true` on error, matching the `GlobalConfig` branch). `validSetArgs` is unchanged — flags are intentionally excluded from tab-completion.

- **REQ-F-006:** `neo4j-cli/internal/subcommands/config/get.go` (currently `get.go:43-62`) adds `case clicfg.FlagScope:` → constructs a `PrintableConfigEntry{Key: bareKey, Value: cfg.Flags.Enabled(bareKey)}` and passes it to `output.PrintBodyMap`. `validGetArgs` is unchanged — flags are intentionally excluded from tab-completion. (Note: this means `config get flag.aura-beta` works but does not autocomplete.)

- **REQ-F-007:** `neo4j-cli/internal/subcommands/config/set_test.go` and `get_test.go` add table rows for:
  - `config set flag.aura-beta true` — written to config file, observable via `helper.AssertConfigValue("flag.aura-beta", "true")`.
  - `config set flag.aura-beta maybe` — rejected with `invalid value for "flag.aura-beta"` error.
  - `config set flag.unknown-thing true` — rejected with `invalid config key: "flag.unknown-thing"` error.
  - `config get flag.aura-beta` — returns the bool (matching whatever the config-file / default state is).
  - `config list` does NOT include any `flag.*` key (regression guard for the "hide from list" decision).

- **REQ-F-008:** `neo4j-cli/aura/aura.go:41` changes `if cfg.Aura.AuraBetaEnabled()` → `if cfg.Flags.Enabled("flag.aura-beta")`. Surrounding block (the three `cmd.AddCommand(...)` lines for `dataapi`, `_import`, `deployment`) is unchanged.

- **REQ-F-009:** `neo4j-cli/aura/internal/api/api.go:117` (`getVersionPath` — `betaEnabled := cfg.Aura.AuraBetaEnabled()`) changes to `betaEnabled := cfg.Flags.Enabled("flag.aura-beta")`. Surrounding switch unchanged.

- **REQ-F-010:** `neo4j-cli/aura/internal/test/testutils/auratesthelper.go` deletes lines 62–66 (the `if gjson.Get(helper.cfg, "aura.beta-enabled").Bool() { cfg.Aura.SetBetaEnabled(true) }` bridge). The `gjson` import is removed if no other reference remains in the file.

- **REQ-F-011:** All 33 test files calling `helper.SetConfigValue("aura.beta-enabled", true)` (plus the one `h.setConfigValue(...)` call in `neo4j-cli/internal/subcommands/config/project_test.go:148`) are rewritten to use `"flag.aura-beta"` instead. This is a mechanical search-and-replace across:
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/*_test.go`
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/*_test.go`
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/authprovider/*_test.go` (if present)
  - `neo4j-cli/aura/internal/subcommands/organization/{get,list}_test.go`
  - `neo4j-cli/aura/internal/subcommands/import/*_test.go`
  - `neo4j-cli/aura/internal/subcommands/deployment/*_test.go`
  - `neo4j-cli/aura/internal/subcommands/project/*_test.go`
  - `neo4j-cli/aura/internal/subcommands/workspace/*_test.go`
  - `neo4j-cli/aura/internal/subcommands/config/*_test.go`
  - `neo4j-cli/internal/subcommands/config/project_test.go`
  *(Final file list driven by the grep output, not hard-coded in the PRD — but the set must equal the grep result for `aura.beta-enabled` minus the bridge in `auratesthelper.go:64`.)*

- **REQ-F-012:** `.agents/feature-flags.md` is updated:
  - Fix the stale path `common/clicfg/auratesthelper.go:68-70` → `neo4j-cli/aura/internal/test/testutils/auratesthelper.go` (note: the bridge is being deleted; rewrite the line to reflect the post-PR reality — tests now write `flag.aura-beta` directly via the helper's `SetConfigValue`).
  - Compress the "Migrating aura.beta-enabled" section to a one- or two-line note that the migration completed in CLI-136; the legacy key is read as a fallback until CLI-134 ships.
  - Refresh line refs that this PR invalidates (`clicfg.go:32`, `clicfg.go:281`, `clicfg.go:371-377` all reference deleted code) — replace with a pointer to `common/clicfg/flags.go` Registry as the new source of truth.

### Non-Functional Requirements

- **REQ-NF-001:** `make test`, `make fmt-check`, `make lint`, `make license-check`, `make generate-check` all pass on the branch (and on CI ubuntu / windows / macos).
- **REQ-NF-002:** Skill bundle (`neo4j-cli/internal/skill/bundle/**`) is unchanged. `make generate-check` is the regression gate. No cobra command is added, removed, hidden, or renamed; no flag visibility change leaks into help text. (Verified pre-change: `grep -rn "beta-enabled\|aura-beta" neo4j-cli/internal/skill/bundle/` and `grep -rn "beta-enabled" common/skill/` both return zero matches.)
- **REQ-NF-003:** Changelog entry via changie: `changie new --projects neo4j-cli --kind Minor --body "Add feature-flag registry; rename aura.beta-enabled to flag.aura-beta (legacy key still read until CLI-134)"`. Minor (not Patch) because the env-var and config-file override surface is new user-facing behaviour.
- **REQ-NF-004:** Branch named `oskar/cli-136-implement-feature-flag-system` (Oskar prefix per personal convention; descriptive slug from the Linear `gitBranchName`).
- **REQ-NF-005:** Single PR. Commit structure at author discretion; recommended split is (a) registry + clicfg wiring + clicfg tests; (b) config set/get integration + tests; (c) call-site migration + test renames + auratesthelper bridge removal; (d) docs + changelog.
- **REQ-NF-006:** Cross-platform safety — `FlagNameToEnv` uses `strings.ToUpper` and `strings.ReplaceAll`; no path separators or OS-specific shell behaviour. Env-var resolution is delegated to viper, which already abstracts platform differences.
- **REQ-NF-007:** Test seam (`FlagSet.SetForTest`) is process-local; values are not persisted to disk or to viper. Matches the previous `AuraConfig.SetBetaEnabled` contract.
- **REQ-NF-008:** `slog.Debug` calls (legacy alias use, unknown key) must not panic when `slog.Default()` is the no-op handler. Standard library guarantees this; no extra defensive check needed.

## Technical Considerations

### Decisions locked in from plan-mode Q&A

- **Scope:** Registry + migrate `aura.beta-enabled` in same PR (option 1 of the four-question Q&A).
- **Config UX:** `config set flag.x true` works; flags hidden from `config list` and `config get` no-arg listing (option 2).
- **Flag value type:** Boolean only (option 1). Defer string/enum until a concrete need.
- **Test migration:** Migrate all 33 tests to `flag.aura-beta` in this PR (option 2). No dual-period coexistence at the test seam.
- **Changelog kind:** Minor.
- **`IntroducedIn` for `flag.aura-beta`:** next-release semver. Read from `changie latest --project neo4j-cli` at implementation time; bump the Minor digit. (E.g. if latest is `1.7.0`, set to `1.8.0`.)
- **Legacy-alias debug log:** one-shot per process per legacy key, via `sync.Once` map on the `FlagSet`.

### Reuse / existing utilities

- `viper.BindEnv` / `viper.SetDefault` / `viper.IsSet` / `viper.GetBool` — already used to bind `aura.base-url` and `aura.auth-url`. Same binding mechanism.
- `sjson.Set` + `fileutils.WriteFile` — pattern in `GlobalConfig.Set` at `clicfg.go:451-459`. `FlagSet.SetFromConfigCmd` reuses it verbatim.
- `clierr.NewUsageError` — matches the existing `ResolveConfigKey` error shape, surfaces as exit code 2 via the existing `SetFlagErrorFunc` wiring.
- `slog.Debug` — already imported in `clicfg.go:9`; satisfies the convention doc's "silent (debug-log only)" requirement.
- `testfs.GetTestFs(cfg, credentials)` — standard test fs helper for clicfg tests.
- `helper.SetConfigValue(key, value)` — test helper at `auratesthelper.go:88-92`; unchanged API, just feed it the new key.

### Registry shape vs `ValidConfigKeys`

`ValidConfigKeys` on `GlobalConfig` / `AuraConfig` is the source of truth for **listable, non-flag** config keys. The flag `Registry` is a **separate** map. This separation is what naturally implements the "hide from list" decision — `cfg.Printable()` iterates `ValidConfigKeys` only, never touches `Registry`, so flags are invisible to `config list` for free.

### Env-var binding for legacy alias

The convention env-var shape is `NEO4J_CLI_FLAG_<AREA>_<FEATURE>`. For the legacy key `aura.beta-enabled`, the equivalent legacy env shape would historically have been `AURA_BETA_ENABLED` (matching `AURA_BASE_URL` / `AURA_AUTH_URL`). We do NOT bind that legacy env shape — there is no prior art for it, no test or user relies on it, and the new `NEO4J_CLI_FLAG_AURA_BETA` env var covers the use case. The "legacy" support is solely for config-file readers (i.e. someone with `{"aura":{"beta-enabled":true}}` in `config.json`).

### Order of operations in `NewConfig`

`bindEnvironmentVariables` and `setDefaultValues` are called BEFORE `Viper.ReadInConfig()`. The registry iteration must therefore happen inside (or be invoked from) `bindEnvironmentVariables` and `setDefaultValues` — it cannot live in a separate `NewFlagSet` constructor that runs after config is read. The `Flags *FlagSet` assignment on `Config` happens at struct construction time (post-`ReadInConfig`); it just wraps the already-bound viper.

### `cfg.Aura.AuraBetaEnabled()` deletion compatibility

The two production call sites (`aura.go:41`, `api.go:117`) are inside `neo4j-cli/`; no other package depends on the method. Deletion is safe. Any drift inside the aura tree would surface as a compile error caught by `make test`.

### Cobra one-file-per-leaf layout

This repo follows the strict one-file-per-leaf cobra layout documented in AGENTS.md. No new cobra leaf is added by this PR; only `clicfg.go` / `set.go` / `get.go` are edited and `flags.go` is added as a sibling to `clicfg.go` in `common/clicfg/`. Test files are colocated.

### Risk surface

- Test migration is mechanical but spans 33 files. A regex / `sed` sweep is fine; the PR review surface should treat it as a rename and not require per-file scrutiny beyond confirming the substitution was clean.
- `Viper.SetDefault` on the legacy key (`aura.beta-enabled`) means `viper.IsSet("aura.beta-enabled")` returns `false` unless the key is explicitly set in env or config — that's correct, but the test for the legacy-fallback path must explicitly seed the legacy key in the test config JSON to exercise it.
- The `sync.Once` map on `FlagSet` is per-instance, not per-process-truly. Each `NewConfig` creates a new `FlagSet` and a new map. In production this is fine (one `Config` per process). In tests, each test creates a fresh `FlagSet`, so the one-shot is observable per test — exactly what we want.

### PR coordination

No known in-flight PRs touch `common/clicfg/flags.go` (the file does not exist), `common/clicfg/clicfg.go` core, `neo4j-cli/aura/aura.go:41`, or `neo4j-cli/aura/internal/api/api.go:117`. CLI-134 is owned but not in flight. Land independently.

## Acceptance Criteria

- [ ] `common/clicfg/flags.go` exists with the `Flag` struct, `Registry` map (one entry: `flag.aura-beta`), `FlagSet` type with `Enabled` / `SetForTest` / `SetFromConfigCmd` methods, and `FlagNameToEnv` helper.
- [ ] `common/clicfg/flags_test.go` exercises all five precedence layers, the one-shot legacy debug log, the env-var derivation table, and the `SetFromConfigCmd` happy + unhappy paths.
- [ ] `DefaultAuraBetaEnabled`, `AuraConfig.betaEnabled`, `AuraConfig.SetBetaEnabled`, `AuraConfig.AuraBetaEnabled` are deleted from `common/clicfg/clicfg.go`. Confirmed by `grep -n 'AuraBetaEnabled\|SetBetaEnabled\|DefaultAuraBetaEnabled\|betaEnabled' common/clicfg/clicfg.go` returning zero matches.
- [ ] `Config.Flags` field is initialised in `NewConfig` and accessible from any caller holding a `*Config`.
- [ ] `FlagScope` is added to the `ConfigScope` enum and recognised by `ResolveConfigKey` for `flag.*` keys; unknown flag names produce `invalid config key: "flag.<x>"` errors.
- [ ] `clicfg_test.go` has two new `TestResolveConfigKey` rows (accepted + rejected flag-scope cases).
- [ ] `neo4j-cli/internal/subcommands/config/set.go` routes `FlagScope` to `cfg.Flags.SetFromConfigCmd`; `get.go` routes `FlagScope` to `cfg.Flags.Enabled`.
- [ ] `config/set_test.go` and `config/get_test.go` have new cases covering accepted-set, unknown-name-rejected, invalid-value-rejected, get-returns-bool, and list-excludes-flags.
- [ ] `neo4j-cli/aura/aura.go:41` and `neo4j-cli/aura/internal/api/api.go:117` call `cfg.Flags.Enabled("flag.aura-beta")`.
- [ ] `neo4j-cli/aura/internal/test/testutils/auratesthelper.go` no longer references `aura.beta-enabled` or `SetBetaEnabled`; `gjson` import removed if unused.
- [ ] `grep -rn 'aura.beta-enabled' .` returns matches ONLY in `.agents/feature-flags.md` (the legacy-alias note), `common/clicfg/flags.go` (the `LegacyKey` field value), and `common/clicfg/flags_test.go` (legacy-fallback test). No matches in any `_test.go` under `neo4j-cli/`. No matches in any `.go` source under `neo4j-cli/`.
- [ ] `grep -rn 'flag.aura-beta' .` returns matches in: registry, two production call sites, all migrated test files, and the docs.
- [ ] `.agents/feature-flags.md` updated per REQ-F-012 — stale line refs removed; "Migrating" section compressed to legacy-fallback note.
- [ ] `make test` passes locally (and CI green on ubuntu / windows / macos).
- [ ] `make fmt-check`, `make lint`, `make license-check`, `make generate-check` all pass.
- [ ] Skill bundle (`neo4j-cli/internal/skill/bundle/**`) is unchanged in the diff.
- [ ] Changelog entry exists under `.changes/unreleased/` with `project: neo4j-cli`, `kind: Minor`, body referencing the registry + rename.
- [ ] Manual smoke (from the plan's verification section):
  - `bin/neo4j-cli aura --help` with no env / no config → `dataapi` / `import` / `deployment` hidden.
  - `NEO4J_CLI_FLAG_AURA_BETA=1 bin/neo4j-cli aura --help` → those three appear.
  - `bin/neo4j-cli config set flag.aura-beta true --rw` then `bin/neo4j-cli aura --help` → those three appear.
  - `bin/neo4j-cli config set flag.does-not-exist true --rw` → usage error, exit 2.
  - `bin/neo4j-cli config get flag.aura-beta` → prints the bool.
  - `bin/neo4j-cli config list` does NOT include any `flag.*` key.
  - With legacy `{"aura":{"beta-enabled":true}}` in `~/.config/neo4j/cli/config.json` and nothing else → `aura --help` shows the three beta commands (legacy alias path exercised).

## Out of Scope

- `--flag` CLI option for per-invocation override (excluded by convention).
- Non-boolean flag types.
- CLI-134 config migration (physical removal of retired keys from `config.json`).
- Migration of other ad-hoc gates (telemetry disable env var, etc.).
- README / CONTRIBUTING updates surfacing `flag.aura-beta` to end users.
- Binding a legacy env-var shape like `AURA_BETA_ENABLED` (no prior art; users with that env set would have already been doing something custom).
- Telemetry events on flag evaluation.
- Renaming or restructuring other `.agents/` docs beyond `feature-flags.md`.
- Backporting to released versions.

## Open Questions

None — all major decisions are resolved in plan mode:

1. Registry + migration in same PR; tests migrated in same PR; boolean only; flags hidden from `config list`.
2. Changelog = Minor; `IntroducedIn` = next-release semver read from `changie latest`; legacy debug log is one-shot per process per legacy key.
