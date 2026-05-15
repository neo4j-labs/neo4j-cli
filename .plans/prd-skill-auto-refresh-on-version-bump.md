# PRD: Skill Auto-Refresh on Version Bump

## Overview

When neo4j-cli is upgraded via a package manager (Homebrew, npm, install.sh), the binary version changes but installed skill bundles in agent directories are not automatically refreshed. This leaves agents with stale SKILL.md files that reference the previous version's commands and flags. On the first command run after a version change, neo4j-cli should silently detect the mismatch and refresh all already-installed skill bundles in the background.

## Goals

- Ensure installed skill bundles stay current after package-manager upgrades without requiring manual `neo4j-cli skill install`.
- Be non-intrusive: run in the background, print a brief stderr notice on completion.
- Be safe: never auto-install for agents that don't already have the skill installed.

## Non-Goals

- Auto-installing skills for newly detected agents that have never had a skill installed.
- Blocking the foreground command while the refresh runs.
- Replacing the existing `neo4j-cli update` post-swap refresh (that path already handles in-place binary updates).

## Requirements

### Functional Requirements

- REQ-F-001: On each command invocation, neo4j-cli reads a state file (`skill-refresh.json`) in the OS config directory (`clicfg.ConfigPrefix/neo4j/cli/`) to determine the last version for which skills were refreshed.
- REQ-F-002: If the current binary version differs from the version recorded in `skill-refresh.json` (or the file is absent), neo4j-cli spawns a background goroutine to refresh skill bundles.
- REQ-F-003: The background goroutine calls `commonskill.List()` to enumerate agents, then calls `commonskill.Install()` only for agents where `Installed == true` (already have the skill bundle).
- REQ-F-004: On successful completion, the goroutine writes the current version to `skill-refresh.json` and prints a single line to stderr: `Refreshed neo4j-cli skill for <N> agent(s) (vOLD → vNEW)`.
- REQ-F-005: If refresh fails for one or more agents, the goroutine prints a per-agent warning to stderr (non-fatal); the state file is still updated for agents that succeeded.
- REQ-F-006: A new config key `skill-auto-refresh` (bool, default `true`) is added to `GlobalConfig.ValidConfigKeys`. Setting it to `false` disables the background refresh entirely.
- REQ-F-007: The version recorded in `skill-refresh.json` is updated to the current version only after the refresh attempt completes (success or partial failure), never before.
- REQ-F-008: The goroutine uses `cmd.Context()` so it is cancelled if the parent process exits early (consistent with the `versioncheck.Schedule` pattern).

### Non-Functional Requirements

- REQ-NF-001: Reading `skill-refresh.json` must be synchronous but fast (single file read); any I/O error is treated as "no prior record" — never blocks or panics.
- REQ-NF-002: All goroutine errors are swallowed before surfacing to the user as warnings; the background goroutine must never take down the host process.
- REQ-NF-003: `skill-refresh.json` is written with mode 0600, matching `version-check.json`.
- REQ-NF-004: The feature must be hermetically testable via `afero.Fs` (no real filesystem access in tests).

## Technical Considerations

**State file location and shape** — follow the `versioncheck` cache pattern exactly: a small JSON file (`skill-refresh.json`) in `clicfg.ConfigPrefix/neo4j/cli/` with a single field `last_refreshed_version`. Reads and writes go through `afero.Fs` for testability.

**Integration point in `app.go`** — the refresh check should be added to the `PersistentPreRunE` hook alongside `versioncheck.MaybeHint` / `versioncheck.Schedule` (lines ~53–64 of `neo4j-cli/app/app.go`). Read the state file synchronously, then spawn the goroutine if needed.

**New package** — add `neo4j-cli/internal/skillrefresh/` (mirroring the `versioncheck` package structure: `cache.go` for state file I/O, `skillrefresh.go` for the trigger + goroutine logic). This keeps `app.go` thin.

**Config key** — add `"skill-auto-refresh"` to `GlobalConfig.ValidConfigKeys` in `clicfg.go` and set a default of `true` via `Viper.SetDefault("skill-auto-refresh", true)`. The `skillrefresh` package reads it via `cfg.Global.Get("skill-auto-refresh")` before spawning the goroutine.

**Goroutine cancellation** — pass `cmd.Context()` into the goroutine; wrap blocking calls with a `select` on `ctx.Done()` consistent with other background work.

**Relation to `neo4j-cli update` post-swap refresh** — the existing `refreshSkillBundles` helper in `update.go` handles the binary-swap case and writes the new version into installed SKILL.md files. The new `skillrefresh` package is additive and covers the package-manager upgrade case. The two paths are complementary: after `neo4j-cli update` completes, `skill-refresh.json` will be stale relative to the new version, and the very next command will trigger the background refresh — but since the bundles are already current, `commonskill.List()` will show no drift and `commonskill.Install()` is idempotent. The goroutine should still update `skill-refresh.json` to the new version so subsequent runs skip the check.

## Acceptance Criteria

- [ ] On first command after a version change (binary replaced via Homebrew/npm), installed skill bundles are updated in the background and a one-line stderr notice is printed.
- [ ] `skill-refresh.json` is created on first run and updated each time a refresh completes.
- [ ] Setting `neo4j-cli config set skill-auto-refresh false` suppresses the background refresh entirely.
- [ ] Agents without an existing skill install are not touched (no auto-install side-effect).
- [ ] A per-agent stderr warning is printed if any individual agent refresh fails; the process still exits zero.
- [ ] `make test`, `make fmt-check`, and `make lint` pass with no regressions.
- [ ] New `skillrefresh` package has table-driven unit tests covering: no state file, version match, version mismatch, opt-out via config, partial agent failure.

## Out of Scope

- Auto-installing skills for agents that have never had the skill installed.
- A `--no-skill-refresh` flag on individual commands.
- Any UI change to `neo4j-cli skill check` (still reports drift independently if called manually).
- Refreshing skills on `aura` binary invocations (this feature targets `neo4j-cli` only, as the `aura` standalone binary is no longer shipped).

## Open Questions

- None — all design questions resolved in requirements gathering.
