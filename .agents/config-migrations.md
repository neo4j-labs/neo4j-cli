# Config Migration

Forward-only schema migrations for `config.json`. Lives in `common/configmigrate/`. Wired from `common/clicfg/clicfg.go:NewConfig` (synchronous, between the first and a second `Viper.ReadInConfig()` so migrated values are visible).

Origin: CLI-134. First concrete use case: cleaning up retired feature-flag keys after a flag graduates (see `.agents/feature-flags.md`).

## Marker

- Key `_schema_version` (int) inside `config.json`, at the top level.
- Absent / non-int → treated as 0.
- NOT in `GlobalConfig.ValidConfigKeys` — invisible to `config get`/`set`/`list`. The `_` prefix signals internal.
- Stored alongside user data so the marker travels with the file if it's copied between machines.

## Registry shape

`var migrations = []Migration{}` in `common/configmigrate/migrations.go`. Each `Migration`:

- `Version int` — monotonic, 1-indexed, contiguous. `init()` panics via `validateMigrations` on gap, duplicate, or non-1 start.
- `Description string` — short human label; appears in stderr warnings.
- `Apply func([]byte) ([]byte, error)` — receives raw config bytes, returns transformed bytes. Use `sjson.DeleteBytes` / `sjson.SetBytes`. Engine stamps `_schema_version` after each successful Apply; the migration must NOT do so itself.

To add one: append to the slice (never insert mid-list) with `Version` = current_max + 1.

## Error policy (warn-and-continue, never panic)

Mirrors `skillrefresh` posture: the foreground command must never break because of a migration failure.

- Missing file → silent no-op.
- Read error (not ErrNotExist) → single-line stderr warning, no-op.
- `Apply` returns error → exact warning `Warning: config migration v%d (%s) failed: %v; continuing with un-migrated config\n`, loop stops, file NOT written.
- All migrations applied successfully → one atomic write via `fileutils.WriteFile`.
- Empty registry → no read, no write, silent.

Happy path is silent — no "migrated config to v1" banner.

## Test seam

`runWith(fs, configPath, stderr, ms []Migration)` is the unexported test entry point. Tests inject fixture migrations via this — never touch the package-level `migrations` slice. Use `bytes.Buffer` as stderr sink and assert on the exact warning string.

Tests cannot import `common/clicfg` or `test/utils/testfs` (transitively imports clicfg) — that creates a test-time import cycle once `clicfg` depends on `configmigrate`. Use a local fs seeder with a hard-coded relative path.

## Internal-package gotcha

Package lives under `common/`, NOT `neo4j-cli/internal/`. Go's internal-package rule blocks `common/clicfg` (and anything else under `common/`) from importing `neo4j-cli/internal/*`. Anything wired from `clicfg.NewConfig` must live under `common/`.

## See also

- Linear CLI-134 — this subsystem.
- `.agents/feature-flags.md` — owns the user-side cleanup motivation.
- `common/configmigrate/configmigrate.go` — engine.
- `common/configmigrate/migrations.go` — registry + validator.
- `common/configmigrate/configmigrate_test.go` — scenario coverage + validator unit tests.
