# PRD: CLI-125 — Feature-flag convention doc

## Overview

CLI-125 ("Decide on how we use feature flags") asks us to write down — once, in the repo — how feature flags should be named, defaulted, overridden, tested, and retired in `neo4j-cli`. Today there is exactly one flag (`aura.beta-enabled`, runtime-only, set by tests via `auratesthelper`), and no convention. Without one, the next handful of flags will each invent their own shape and the cleanup story for old keys will diverge from the config-migration plan (CLI-134).

The deliverable is a single short markdown doc under `.agents/` plus a one-line index pointer in `AGENTS.md`. It does **not** implement the registry, env binding, or migration — those are downstream tasks that must follow the rules this doc codifies.

The convention was discussed with Oskar before this PRD; decisions are settled (separator, override surface, registry shape, treatment of the existing flag, unknown-key behaviour, no kill-switches). See the source plan: `/Users/oskarhane/.claude/plans/i-want-to-create-zany-frost.md`.

Linear: https://linear.app/neo4j/issue/CLI-125 — Linear comment by Oskar (2026-05-18) is the seed for the convention. Related: https://linear.app/neo4j/issue/CLI-134 (config migration, owns user-side cleanup).

## Goals

- Create `/.agents/feature-flags.md` capturing the convention in the same dense bullet style as `.agents/cobra.md`, `.agents/credentials.md`, `.agents/windows-ci.md`.
- Add one index entry in `AGENTS.md` ("Feature Flag Notes" pointer) so the doc is discoverable from the existing repo-doc map; the `CLAUDE.md` symlink propagates automatically.
- Resolve naming (`flag.<area>-<feature>`), default (`false`), override surface (config file + env var only), registry shape (Go map in `common/clicfg/`), unknown-key behaviour (silent + debug log), and lifecycle (delete on GA, no flip-to-true).
- Pre-record the migration path for the one existing flag (`aura.beta-enabled` → `flag.aura-beta`) and the four touch points it has in `common/clicfg/clicfg.go` / `auratesthelper.go`.

## Non-Goals

- Implementing the `common/clicfg/flags.go` registry (separate downstream task).
- Wiring viper env binding for `NEO4J_CLI_FLAG_*` (downstream).
- Renaming `aura.beta-enabled` in source or migrating user `config.json` (CLI-134 + a downstream code task).
- Designing the config-migration mechanism itself (CLI-134).
- Adding a `--flag` CLI flag for per-invocation overrides — explicitly excluded by the agreed override surface.
- Writing tests for the convention. The doc is prose; there is no code under test.
- Refreshing skill bundles. `.agents/**` is not part of the cobra tree and does not feed any `go generate` output.
- Touching `README.md` or `CONTRIBUTING.md`. Feature-flag conventions are agent-facing only at this stage.
- Adding a changelog entry. `.agents/` and `AGENTS.md` are internal docs, not user-facing CLI behaviour — per `AGENTS.md` "Build System" guidance, internal-only changes don't need a changelog.

## Requirements

### Functional Requirements

- **REQ-F-001**: Create `/Users/oskarhane/Development/neo4j-cli-2/.agents/feature-flags.md`. ≤60 lines. Style matches `.agents/cobra.md` / `.agents/credentials.md` (terse bullets, code refs with line numbers, no preamble).
- **REQ-F-002**: The doc contains exactly these `##` sections, in this order, with the content described below:
  1. **`## Key naming`** — Prefix `flag.`, then `<area>-<feature>` (lowercase kebab). One area token, one feature token. Examples: `flag.aura-beta`, `flag.docker-command`, `flag.secrets-os-keystore`. Note: registry at `common/clicfg/flags.go` (a Go `map[string]Flag`) is the runtime source of truth; this doc is the narrative reference.
  2. **`## Defaults & lifecycle`** — Always default `false` on introduction; opt-in only. No kill-switches / on-by-default-with-disable; rollbacks ship via release, not a flipped default. On GA: delete the flag and its gated branch in the same PR — do NOT flip default to `true`. Registry entry must carry: name, default, owner, what it gates, introduced-in version, expected removal condition.
  3. **`## Override surface`** — Listed high → low precedence: (a) env var `NEO4J_CLI_FLAG_<AREA>_<FEATURE>=1` (dot/dash → underscore, uppercased; ideal for CI); (b) config file via `neo4j-cli config set flag.<area>-<feature> true`. Explicit: no `--flag` CLI flag.
  4. **`## Testing`** — Every flag ships with tests for BOTH states while it lives. CI runs the flag-on path explicitly (test build step or env var) until the flag is removed. Aura-side flags use the existing `auratesthelper` pattern (cite `common/clicfg/auratesthelper.go`).
  5. **`## Unknown / removed keys`** — Silent at runtime (debug-log only) so old user configs survive CLI upgrades. CLI-134 config migration is responsible for stripping retired keys from `config.json`.
  6. **`## Migrating aura.beta-enabled`** — Becomes `flag.aura-beta` once the registry lands. Old key reads remain accepted (debug-log "deprecated") until CLI-134 ships. List four touch points with line refs: `common/clicfg/clicfg.go:32` (`DefaultAuraBetaEnabled`), `clicfg.go:281` (struct field), `clicfg.go:371-377` (getter/setter), `auratesthelper.go:68-70` (test seam).
  7. **`## See also`** — Linear CLI-125 (this decision), CLI-134 (config migration). Note that `AGENTS.md` → "Repo Layout Notes" area links here.
- **REQ-F-003**: The doc opens with a single `# Feature Flags` H1 and one introductory sentence: how we name, gate, test, and retire experimental behaviour; implementation tracked in CLI-125, user-side cleanup in CLI-134. No further preamble.
- **REQ-F-004**: Edit `/Users/oskarhane/Development/neo4j-cli-2/AGENTS.md` to add a new entry in the existing "Repo Layout Notes" / "Hermetic Test Notes" / "Windows CI Gotchas" cluster (the bottom half of the file). Format mirrors the existing entries (a one- or two-line summary with a markdown link to the new file). Suggested heading and copy:

  ```
  ## Feature Flag Notes

  See [`.agents/feature-flags.md`](.agents/feature-flags.md) — naming (`flag.<area>-<feature>`), default-false lifecycle, override surface (config + env), registry shape, and the `aura.beta-enabled` → `flag.aura-beta` migration.
  ```
  Edit `AGENTS.md` only; `CLAUDE.md` is a symlink and updates automatically (per `AGENTS.md` "Repo Doc Notes").

- **REQ-F-005**: Every code reference in the new doc uses `path/to/file.go:LINE` format (matching existing `.agents/` docs). Specific refs that must appear: `common/clicfg/clicfg.go:32`, `clicfg.go:281`, `clicfg.go:371-377`, `common/clicfg/auratesthelper.go:68-70`, and `common/clicfg/flags.go` (future home, no line ref).
- **REQ-F-006**: All example flag names in the doc use the dotted form (`flag.aura-beta`, `flag.docker-command`, `flag.secrets-os-keystore`) — no colon-prefix form (`flag:…`) leaks through from the original Linear comment. Verified by `grep -nE 'flag:' .agents/feature-flags.md` returning no matches.

### Non-Functional Requirements

- **REQ-NF-001**: No Go code, test, or generated artefact is touched. `make test`, `make fmt-check`, `make lint`, and `make license-check` therefore do not need to be re-run as gates (markdown is outside their scope). The standard "final gates" rule from `AGENTS.md` is N/A here because there is no buildable change.
- **REQ-NF-002**: The doc is ≤60 lines of rendered Markdown (matching the density of the other short `.agents/` files: `cobra.md` 9 lines, `windows-ci.md` 10 lines, `credentials.md` 35 lines). The shorter the better — high signal, no filler.
- **REQ-NF-003**: Platform-agnostic. The doc references env-var shape (`NEO4J_CLI_FLAG_<AREA>_<FEATURE>`) but does not prescribe Windows-specific behaviour; existing viper env binding handles platform differences.
- **REQ-NF-004**: The doc must read correctly even if the reader hasn't seen the Linear ticket or the source plan — it stands alone as the convention. Linear links are provided in "See also" for context, not as prerequisites.

## Technical Considerations

### Existing landscape

- Exactly one flag exists today: `aura.beta-enabled`, defined at `common/clicfg/clicfg.go:32` (default constant), `clicfg.go:281` (struct field), `clicfg.go:371-377` (getter/setter). It is runtime-only — not in `ValidConfigKeys`, not persisted to `config.json`, not bound to an env var. The only setter outside the struct is `common/clicfg/auratesthelper.go:68-70`, which reads `aura.beta-enabled` from the test helper's JSON config.
- All other `aura.*` config keys are persisted regular config (e.g. `aura.base-url`, `aura.auth-url`, `aura.default-tenant`) — not flags. The new convention deliberately separates flags into their own `flag.` namespace so future cleanup is unambiguous.

### Why these decisions

- **`flag.` (dot) vs `flag:` (colon)**: dotted matches viper's existing convention (`aura.base-url`, `aura.auth-url`, …) and slots into the current `Get`/`Set` codepath without a special separator branch. The colon form was greppable but introduced a parser exception for no real win.
- **Env var + config only, no `--flag`**: a CLI `--flag` would require parsing on the root command and global state plumbing through every leaf; env var already covers the CI / one-shot use case at zero implementation cost.
- **Silent on unknown keys**: a warning on every `config.json` read for a removed flag would spam users mid-upgrade. CLI-134 owns the user-side cleanup; until it ships, debug-log-only is the correct floor.
- **Delete on GA, never flip default**: leaving `default: true` in the registry plus a now-stale `if cfg.Flag(...) { ... }` branch is dead-code-with-cost. The PR that turns the feature on is the same PR that deletes the gate.
- **No kill-switches**: a kill-switch is a config option, not a feature flag. Flags are opt-in only; this rules out a class of confusing "default true, set false to disable" states.

### Style and house conventions

- Per `AGENTS.md` "Repo Doc Notes": never write to `CLAUDE.md` directly — it's a symlink to `AGENTS.md`. Only the AGENTS.md edit is needed.
- Per `.agents/` survey of 13 existing files: tone is bulleted, ≤100 lines (most ≤50), no preamble, code refs use `path:LINE` form, prose stays minimal.
- The doc does **not** need to enumerate every existing flag — there is only one, and that one is called out explicitly in the "Migrating" section.

### Risk surface

- Zero runtime impact. The change is two markdown surfaces.
- Some risk of the convention drifting from reality once the registry is implemented (CLI-125 follow-up). Mitigation: registry implementation PR must update this doc if any rule changes. The `See also` cross-link to CLI-125/CLI-134 makes the dependency visible.

### Files touched

- `/.agents/feature-flags.md` (new, ~50-60 lines).
- `AGENTS.md` (one new `## Feature Flag Notes` subsection, ~3 lines, slotted into the bottom-half index cluster).

## Acceptance Criteria

- [ ] `.agents/feature-flags.md` exists and is ≤60 lines (`wc -l .agents/feature-flags.md`).
- [ ] The file contains all seven sections from REQ-F-002 in the order listed, each with the bulleted content described.
- [ ] `grep -n 'flag:' .agents/feature-flags.md` returns no matches (no leftover colon-prefix examples).
- [ ] `grep -nE 'flag\.(aura-beta|docker-command|secrets-os-keystore)' .agents/feature-flags.md` returns at least one match per example name.
- [ ] `grep -n 'common/clicfg/clicfg.go:32' .agents/feature-flags.md` and `grep -n 'auratesthelper.go:68-70' .agents/feature-flags.md` both return matches (touch-point refs present).
- [ ] `grep -n 'CLI-125' .agents/feature-flags.md` and `grep -n 'CLI-134' .agents/feature-flags.md` both return matches.
- [ ] `AGENTS.md` contains a `## Feature Flag Notes` heading with a markdown link to `.agents/feature-flags.md`. (`grep -n 'feature-flags.md' AGENTS.md`)
- [ ] `ls -la CLAUDE.md` still resolves the symlink to `AGENTS.md` (no accidental break).
- [ ] No Go, YAML, or generated-bundle file is modified by this change. (`git diff --name-only` shows only `.agents/feature-flags.md` and `AGENTS.md`.)
- [ ] No `.changes/unreleased/` entry is added (this is an internal docs change).
- [ ] A quick visual read of the rendered doc reads cleanly end-to-end without referencing the Linear ticket or the source plan.

## Out of Scope

- Implementing `common/clicfg/flags.go` (the Go registry) or any env-binding plumbing.
- Migrating `aura.beta-enabled` in source (touch points are documented; the rename itself is a downstream task).
- The CLI-134 config-migration implementation.
- A `--flag` CLI option for per-invocation override (deliberately excluded).
- Updating `README.md`, `CONTRIBUTING.md`, or skill-bundle content. None of those surface feature-flag conventions today.
- A grep gate / linter that enforces flag naming at build time. The registry's typed `map[string]Flag` will provide the runtime gate once it lands; a build-time linter is over-engineering for one flag.
- Renaming or restructuring any other `.agents/` doc.

## Open Questions

- Registry entry "owner" field shape: GitHub handle, Linear assignee, or team name? Deferred to the registry-implementation PR. The doc says "owner" without prescribing the format.
- Where the deprecation debug-log for `aura.beta-enabled` reads should live (`clicfg.Load()` vs a separate `migrate.go`). Out of scope here; settled when the registry/migration code lands.
- Whether the future registry should also accept a `removeBy` version string (e.g. `removeBy: "1.10.0"`) to make stale flags visible in CI. Worth considering when implementing; the doc lists "expected removal condition" loosely on purpose.
