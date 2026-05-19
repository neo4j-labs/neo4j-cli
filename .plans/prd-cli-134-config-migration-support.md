# PRD: CLI-134 — Config migration support

Linear: https://linear.app/neo4j/issue/CLI-134/config-migration-support
Branch: `cli-134-config-migration-support`

## Overview

`config.json` (and related state files) evolve with the binary. Today there is no mechanism to carry users forward across schema/state changes — a commented-out migration block in `common/clicfg/clicfg.go:86-126` is preserved as reference but never runs. The first concrete need is **removing stale feature-flag keys when a flag graduates** (e.g. once `flag.aura-beta` goes live, every existing user's `config.json` still carries the value, polluting `config get`/`list` and confusing the registry).

Because the install surface is fragmented (Homebrew / npm / PyPI / curl|sh / raw download), a post-install hook is not viable — migrations must trigger automatically on **first invocation after a CLI version bump**.

Go has no stdlib support and no dominant third-party lib for CLI config migration (`golang-migrate` / `goose` are for DBs). The conventional shape across CLIs (`gh`, `helm`, `kubectl`) is: schema-version field in the file, ordered registered migration funcs, idempotent on-load application. This card adds exactly that, modelled on the existing `neo4j-cli/internal/skillrefresh/` "do something once per version bump" pattern but synchronous and keyed by a separate monotonic schema version rather than CLI semver.

The registry ships **empty** — engine only. The first real migration lands when a flag actually graduates.

## Goals

- Replace the commented-out migration block in `common/clicfg/clicfg.go` with a working synchronous migration engine that runs inside `clicfg.NewConfig` after `Viper.ReadInConfig()`.
- Track applied state via a top-level `_schema_version` integer key inside `config.json` itself (no sidecar file, no drift risk if config is copied between machines).
- Provide a registry shape (`type Migration struct { Version int; Description string; Apply func([]byte) ([]byte, error) }`) so future migrations are a one-line append + a transform function.
- Mirror the existing `skillrefresh` "never break the foreground command" posture: on migration error, single-line stderr warning and continue with the un-migrated config; never panic.
- Ship the engine with **zero registered migrations** — no user-visible behaviour change until a real migration is added later.

## Non-Goals

- Migrating `credentials.json`, `skill-refresh.json`, `version-check.json`, or any other state file. Scope is `config.json` only.
- Consolidating with `skillrefresh` into one package — operationally different (sync vs async, schema-version vs CLI-version). Side-by-side placement under `neo4j-cli/internal/` is the only shared signal.
- Down-migrations / rollback. Migrations are forward-only.
- A user-facing `neo4j-cli config migrate` command. Trigger is automatic.
- Visible output on the happy path. Silent when migrations apply cleanly.
- Shipping any real migration in this PR (no flag is actually graduating yet — wait per CLI-134 discussion).
- Changing `--help` text, the cobra command tree, or the skill bundle. No `go generate` needed.

## Requirements

### Functional Requirements

- REQ-F-001: A new package `neo4j-cli/internal/configmigrate/` exposes `Run(fs afero.Fs, configPath string, stderr io.Writer) (mutated bool, err error)`. It reads the file, parses the current `_schema_version` (default 0 when absent or non-integer), iterates the package-level `migrations` slice in order, applies each `m` where `m.Version > current`, bumps `_schema_version` to `m.Version` after each successful apply, writes the file **once at the end** via `fileutils.WriteFile`, and returns `mutated=true` iff at least one migration applied.
- REQ-F-002: The `migrations` slice in `neo4j-cli/internal/configmigrate/migrations.go` ships **empty** (`var migrations = []Migration{}`). Engine MUST treat empty registry as a no-op: no read, no write, no version bump, no log output.
- REQ-F-003: `Run` validates that `migrations` is contiguous and ascending (`migrations[i].Version == i+1`) at package init via `init()` panic; gaps or duplicates panic at startup so they cannot ship.
- REQ-F-004: On any individual migration's `Apply` returning a non-nil error, `Run` writes a single-line warning to `stderr` (`"Warning: config migration v%d (%s) failed: %v; continuing with un-migrated config\n"`), stops applying further migrations, does NOT write the file, and returns `mutated=false, err=nil` so the foreground command continues with the un-migrated config.
- REQ-F-005: On file-read error other than `os.ErrNotExist`, `Run` writes a single-line warning to `stderr` and returns `mutated=false, err=nil`. `os.ErrNotExist` is treated as a no-op (no file = nothing to migrate; `clicfg.NewConfig` has already called `Viper.SafeWriteConfig()` upstream so this is unreachable in practice, but defensive).
- REQ-F-006: `common/clicfg/clicfg.go` replaces the commented-out block at lines 86–126 with `configmigrate.Run(fs, fullConfigPath, os.Stderr)` followed by an unconditional `Viper.ReadInConfig()` re-read so subsequent code observes the migrated values. The re-read uses the same panic-on-error posture as the existing call above.
- REQ-F-007: `_schema_version` is NOT added to `ValidConfigKeys` in `common/clicfg/clicfg.go`. It must not appear in `config get`, `config set`, or `config list` output. Set attempts via `config set _schema_version` MUST be rejected with the existing "invalid config key" error.
- REQ-F-008: When `migrations` is empty, the existing `make generate-check` gate stays clean — no bundle regen, no changelog required (internal infrastructure, no user-visible change).

### Non-Functional Requirements

- REQ-NF-001: All `Run` invocations complete in <5ms on a typical config file (single read, single write, gjson/sjson surgery). Synchronous placement means it runs on every invocation; cost must stay invisible to the user.
- REQ-NF-002: File write uses `fileutils.WriteFile` (`common/clicfg/fileutils/fileutils.go`) — atomic temp + fsync + rename, mode 0600 — so a crash mid-write cannot corrupt `config.json`.
- REQ-NF-003: Tests live in `neo4j-cli/internal/configmigrate/configmigrate_test.go` and use `testfs.GetTestFs("{}", "{}")` (NOT `afero.NewOsFs`) per AGENTS.md's credentials-contamination warning.
- REQ-NF-004: A test-only seam (unexported `runWith(fs, configPath, stderr, []Migration)`) lets tests inject fixture migrations without mutating the package-level `migrations` slice. The exported `Run` is a thin wrapper that calls `runWith(..., migrations)`.
- REQ-NF-005: All new `.go` files start with the standard Neo4j copyright header (enforced by CI's `addlicense`).
- REQ-NF-006: `make fmt-check`, `make lint`, `make test`, `make generate-check`, `make license-check` all clean.
- REQ-NF-007: No new third-party dependencies. Use only `github.com/tidwall/gjson` and `github.com/tidwall/sjson` (already vendored, used by the commented-out block).

## Technical Considerations

### Architecture

The fix sits at two layers, each isolated:

1. **New package `neo4j-cli/internal/configmigrate/`** — owns the engine, the registry, and the marker key. Three files:
   - `configmigrate.go` — `Run` / `runWith` entry points, gjson/sjson surgery, error handling.
   - `migrations.go` — the `Migration` struct, the `migrations` slice (ships empty), `init()` validator.
   - `configmigrate_test.go` — table-driven tests against an `afero.MemMapFs`.

2. **Integration in `common/clicfg/clicfg.go`** — single-line call to `configmigrate.Run`, replacing lines 86–126, followed by re-`ReadInConfig`. Imports `github.com/neo4j/cli/neo4j-cli/internal/configmigrate`.

### Why schema version (not CLI version) for the marker

Skill-refresh uses CLI version because the action (reinstall bundle) is keyed off CLI bumps. Migrations are keyed off **schema bumps** — a separate cadence (multiple migrations per CLI version, or zero). Monotonic ints are also easier to test and reason about than semver comparisons, and decouple migration history from release cadence.

### Why inside `config.json`, not a sidecar file

- Co-located with the data it describes — no file/marker drift if a user copies their config between machines.
- Matches the convention of embedded JSON schema-version (`lazy-schema-migration`, `gh`, etc.).
- One fewer file to document and back up.
- `_` prefix marks it visibly internal and keeps it out of `ValidConfigKeys`.

### Why synchronous (not async like skillrefresh)

The migrated values MUST be visible to the rest of the invocation (e.g. a migration that renames a key would otherwise leave `Viper` reading the old key). Skillrefresh can be async because its work (reinstalling external bundles) does not affect the current invocation's state.

### `migrations` validation at init

Catching gaps/duplicates at `init()` (panic) rather than at `Run` time prevents a malformed registry from ever shipping. Any developer adding a migration sees the panic on first `go test` or `go build`.

### Reused utilities (already exist)

- `common/clicfg/fileutils.WriteFile` — atomic write.
- `common/clicfg/fileutils.ReadFileSafe` — read returning `[]byte{}` on missing/error (we use `afero.ReadFile` directly instead so we can distinguish `ErrNotExist`).
- `github.com/tidwall/gjson` — read `_schema_version`.
- `github.com/tidwall/sjson` — set `_schema_version`, run per-migration transforms.
- `neo4j-cli/aura/internal/test/testutils.GetTestFs` — mem-fs with empty credentials for tests.

### Test seam shape

```go
// Run is the production entry point.
func Run(fs afero.Fs, configPath string, stderr io.Writer) (bool, error) {
    return runWith(fs, configPath, stderr, migrations)
}

// runWith is the test seam — tests pass a fixture slice without
// touching the package-level migrations var.
func runWith(fs afero.Fs, configPath string, stderr io.Writer, ms []Migration) (bool, error) { ... }
```

### Files touched

- `neo4j-cli/internal/configmigrate/configmigrate.go` (new).
- `neo4j-cli/internal/configmigrate/migrations.go` (new) — empty registry + `init()` validator.
- `neo4j-cli/internal/configmigrate/configmigrate_test.go` (new) — table-driven tests with fixture migrations injected via `runWith`.
- `common/clicfg/clicfg.go` — replace lines 86–126 with the `configmigrate.Run` call + re-`ReadInConfig`. Add the import.
- No changelog entry (internal infrastructure; no user-visible behaviour until first real migration ships, which is a separate card).
- No bundle regen, no skill template changes.

## Acceptance Criteria

- [ ] `configmigrate.Run` against a fresh `config.json` (no `_schema_version`, empty `migrations`) returns `(false, nil)` and leaves the file byte-identical.
- [ ] `configmigrate.Run` against a `config.json` containing `_schema_version: 3` with a fixture registry of `[v1, v2, v3, v4, v5]` applies only `v4` and `v5`, in order, bumps `_schema_version` to 5, writes once.
- [ ] `configmigrate.Run` against a config with no `_schema_version` and a fixture registry of `[v1, v2]` applies both and sets `_schema_version: 2`.
- [ ] `configmigrate.Run` against an already-up-to-date config (`_schema_version` equals max registered version) returns `(false, nil)` and leaves the file untouched.
- [ ] When a fixture migration's `Apply` returns an error, `runWith` prints the documented warning line to the supplied `stderr`, stops the loop, does NOT write the file, and returns `(false, nil)`.
- [ ] Running `runWith` twice in a row with the same fixture registry produces the same final file contents on the second run as the first (idempotency).
- [ ] `init()` panics with a clear message when `migrations` contains a gap (`[v1, v3]`), a duplicate (`[v1, v1]`), or starts above 1 (`[v2]`).
- [ ] `config get _schema_version` returns the existing "invalid config key" error (REQ-F-007). Verified by an integration-style test against the cobra `config` subtree, OR documented as covered by the existing `ValidConfigKeys` mechanism if no new test is needed.
- [ ] `common/clicfg/clicfg.go` no longer contains the commented-out lines 86–126; in their place is the `configmigrate.Run` call + re-`ReadInConfig`.
- [ ] Live smoke: with `migrations` still empty, build the binary, run `./bin/neo4j-cli config list` against a real `config.json` that already exists, and confirm the file is **byte-identical** before and after (sha256 unchanged).
- [ ] `make fmt-check && make lint && make test && make generate-check && make license-check` all clean.

## Out of Scope

- Migrating any state file other than `config.json`.
- Shipping any real migration entry in the registry.
- Down-migrations / rollback.
- A user-facing `config migrate` cobra command.
- Visible UX (progress, summary, `--verbose` reporting) when migrations apply.
- Consolidating skillrefresh and configmigrate into a single post-upgrade hook framework.
- Changing `Viper`'s read pipeline or moving away from JSON.

## Open Questions

(none — all design decisions locked in plan-mode discussion: empty registry on first ship, silent on happy path, `_schema_version` marker key inside `config.json`, warn-and-continue on error, config-only scope.)
