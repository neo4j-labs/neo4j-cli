# PRD: CLI-85 — Drop `-f` shorthand on `--format`, reassign to `update --force`

## Overview

`common/flags/flags.go:35-40` registers `-f` as the short flag for the persistent `--format` flag. Per the agent-CLI audit (`agent-cli-audit-2026-05-011.md` §2.3), `-f` is globally reserved for `--force`. The repo's only existing `--force` flag (on `neo4j-cli update`) is currently long-form only.

This PRD:

1. Removes the `-f` shorthand from `--format` in `RegisterOutputFlag` so the slot is free.
2. Binds `-f` as the shorthand for `--force` on `neo4j-cli update` so the freed slot is put to canonical use immediately.
3. Updates every place that documented or relied on `-f` for `--format` (hand-written markdown, bundle source, agent-facing docs) and regenerates the skill bundles so `TestGenerator_RoundTrip` stays green.
4. Ships a `Minor` changelog entry: `-f json` / `-f toon` scripts will break and need to switch to `--format json` / `--format toon`.

Linear: https://linear.app/neo4j/issue/CLI-85.

## Goals

- Remove the `"f"` shorthand argument from the `StringP("format", "f", …)` call in `common/flags/flags.go`.
- Bind `"f"` as the shorthand for the existing `--force` flag on `neo4j-cli update` (`neo4j-cli/internal/subcommands/update/update.go:125`).
- Mirror the production registration change in the lone test helper at `neo4j-cli/internal/versioncheck/versioncheck_test.go:278`.
- Add regression tests that lock the freed slot (`--format` has no shorthand) and pin the new binding (`update --force` has shorthand `f`).
- Strip all hand-written `-f` references from skill-bundle source (`additions.md`, `query-additions.md`) and repo docs (`AGENTS.md`, `CONTRIBUTING.md`, `.agents/architecture.md`, `.agents/repo-layout.md`).
- Regenerate both skill bundles (`neo4j-cli/internal/skill/bundle/**` and `neo4j-cli/aura/internal/skill/bundle/**`) so the round-trip gate passes.
- Ship a `Minor`-kind changelog entry under `.changes/unreleased/` describing the user-visible removal + reassignment.

## Non-Goals

- Adding `--force` flags to any other subcommand. CLI-85 is solely the freeing step plus the single canonical re-binding on the command that already had `--force`.
- Introducing a `TOON=1` env var (mentioned in the Linear issue's suggested fix as an alternative; not adopted today).
- Editing historical changelog entries (`CHANGELOG-neo4j.md:94`, `CHANGELOG-aura.md:29`) that record when `-f` was added — they preserve the historical record.
- Rewriting `update`'s `Example:` block to use `-f` (cobra's auto-generated `--help` already surfaces the shorthand in the flag table; example readability stays with `--force`).
- Updating `README.md`, `install.sh`, `install.ps1`, or `gh-pages/**` — verified they do not mention `-f`.

## Requirements

### Functional Requirements

- **REQ-F-001**: `common/flags/flags.go:32` doc comment is updated from `// RegisterOutputFlag adds a persistent --format/-f flag to cmd …` to `// RegisterOutputFlag adds a persistent --format flag to cmd …`.
- **REQ-F-002**: `common/flags/flags.go:35-40` changes the registration call from `cmd.PersistentFlags().StringP("format", "f", "", …)` to `cmd.PersistentFlags().String("format", "", …)`. The help-text string itself (and `clicfg.ValidFormatValues` interpolation) remains unchanged.
- **REQ-F-003**: `neo4j-cli/internal/subcommands/update/update.go:125` changes from `cmd.Flags().BoolVar(&force, forceFlag, false, …)` to `cmd.Flags().BoolVarP(&force, forceFlag, "f", false, …)`. The flag description string is unchanged.
- **REQ-F-004**: `neo4j-cli/internal/versioncheck/versioncheck_test.go:278` changes from `root.PersistentFlags().StringP("format", "f", "", "format")` to `root.PersistentFlags().String("format", "", "format")`. (Test-only seam, mirrors prod registration.)
- **REQ-F-005**: `common/flags/flags_test.go` gains a regression assertion: after calling `RegisterOutputFlag(cmd, cfg)`, `cmd.PersistentFlags().Lookup("format").Shorthand` is the empty string. Lives in a new test function or as a new case in an existing `RegisterOutputFlag`-focused test (whichever fits the file's table-driven layout).
- **REQ-F-006**: `neo4j-cli/internal/subcommands/update/update_test.go` gains a regression assertion: after building the update command, the `force` flag has `Shorthand == "f"`. Additionally, a positive parse-path test confirms `update -f --version <valid>` parses without cobra error (force=true reaches `runOpts`). A separate negative-path test (or a new case in the existing `TestCheckCmd_UnknownForceFlag` style block) confirms `update check -f` still errors with cobra's "unknown shorthand flag" because `--force` is registered on `update` only, not on its child.
- **REQ-F-007**: `neo4j-cli/internal/skill/additions.md` lines 6 and 7 drop the parenthetical `(shorthand `-f`)` and ``(`-f toon`)`` mentions; the sentences otherwise stay intact.
- **REQ-F-008**: `neo4j-cli/internal/skill/query-additions.md` replaces every occurrence of the `-f <value>` short form with `--format <value>`. Confirmed sites (approximate lines, exact set verified by grep): 10 (`:schema -f toon`), 24 (JSON/table-via-`-f` callout), 40, 46, 52, 59, 62, 67, 76, 79–80, 99, 109, 115, 139–140. Surrounding prose is preserved; only the flag token changes.
- **REQ-F-009**: `neo4j-cli/aura/internal/skill/additions.md` lines 5 and 6 drop the same `(shorthand `-f`)` and ``(`-f toon`)`` mentions as REQ-F-007.
- **REQ-F-010**: `AGENTS.md:103` drops the trailing ` (shorthand `-f`)` from the `--format` line. (`CLAUDE.md` is a symlink to `AGENTS.md` per the repo notes; do not edit `CLAUDE.md` directly.)
- **REQ-F-011**: `CONTRIBUTING.md:170` drops the trailing ` (shorthand `-f`)` from the `--format` options line.
- **REQ-F-012**: `.agents/architecture.md:45` drops the trailing ` (shorthand `-f`)` from the `--format` line.
- **REQ-F-013**: `.agents/repo-layout.md:12` drops the trailing ` (shorthand `-f`)` from the persistent-`--format` callout.
- **REQ-F-014**: After all source edits, `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...` is run, and the resulting diffs in `neo4j-cli/internal/skill/bundle/**` and `neo4j-cli/aura/internal/skill/bundle/**` are committed in the same change. Expected drift sites:
  - `neo4j-cli/internal/skill/bundle/SKILL.md` (Global Flags table, Tips section, additions copy)
  - `neo4j-cli/internal/skill/bundle/query-additions.md` (copy of `query-additions.md`)
  - `neo4j-cli/internal/skill/bundle/references/skill.md` (Global Flags table)
  - `neo4j-cli/internal/skill/bundle/references/query.md` (flag table + examples)
  - `neo4j-cli/internal/skill/bundle/references/update.md` (force flag now shows `-f, --force`)
  - `neo4j-cli/aura/internal/skill/bundle/SKILL.md` (description, Tips, additions copy)
- **REQ-F-015**: One `Minor`-kind changelog entry under `.changes/unreleased/` for the `neo4j-cli` project, body: ``"Remove '-f' shorthand from '--format'; '-f' is now the shorthand for 'neo4j-cli update --force' (CLI-85)."``. Author via `changie new --projects neo4j-cli --kind Minor --body "<body>"`, or hand-author the YAML per `.agents/build.md` if changie isn't installed locally.

### Non-Functional Requirements

- **REQ-NF-001**: `make test`, `make fmt-check`, and `make lint` all pass after the change (final gates per `AGENTS.md`). `TestGenerator_RoundTrip` must stay green — it is the bundle-drift detector.
- **REQ-NF-002**: `make generate-check` (which runs `go generate ./...` then `git diff --exit-code`) is clean on the working tree after the bundles have been committed.
- **REQ-NF-003**: Change is platform-agnostic (pure Go flag binding + generated Markdown). No Windows/Unix-specific behaviour.
- **REQ-NF-004**: No new dependencies introduced. No new imports beyond what already exists in the touched files.

## Technical Considerations

### Cobra flag-binding behaviour

- `RegisterOutputFlag` registers `--format` as a *persistent* flag on the root commands (`neo4j-cli/app/app.go:40`, `neo4j-cli/aura/cmd/main.go`, `neo4j-cli/query/query.go`, `common/skill/skill.go:32`). Persistent flags are inherited by every subcommand.
- `update` registers `--force` as a *local* flag on the `update` command only. With the change, `-f` becomes a local shorthand on `update`. `update check` (child) doesn't inherit it because `--force` isn't persistent — confirmed by the existing `TestCheckCmd_UnknownForceFlag` which asserts `update check --force` errors with "unknown flag".
- Once the persistent `--format` loses its `-f` shorthand, no other command in the tree claims `-f`. The `update` local binding therefore cannot collide.

### Bundle-regen gate

- Per `AGENTS.md` "Cobra Help / Skill Bundle Rendering Notes", any `Long:` or flag-help change in the cobra tree must trigger `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`. The flag table inside the generated `SKILL.md` / `references/*.md` reads `.Shorthand` from the live `pflag.Flag` struct, so the `-f` row disappears from `--format` and appears for `update --force` automatically. The hand-written `additions.md` / `query-additions.md` copies are bundled verbatim and therefore must be edited at the source.
- Skipping the regenerate step trips `TestGenerator_RoundTrip` with a "references/<sub>.md differs" diff. CI's `make generate-check` is a backup.

### Test-helper mirror

- `neo4j-cli/internal/versioncheck/versioncheck_test.go:278` is a test-only helper that hand-rolls a minimal cobra root for `MaybeHint` testing. It uses `StringP("format", "f", ...)` to mirror the prod registration. Updating it to `String(...)` keeps the test surface honest — otherwise the test would silently allow `-f` even after prod drops it, which contradicts the regression-locking goal.

### Risk surface

- **Breaking change for shell scripts**: any pipeline using `-f json`, `-f toon`, or `-f table` breaks. This is the explicit intent of the audit's blocker rule and is covered by the `Minor` changelog entry. README + install scripts + gh-pages already use long-form `--format json`, so external surfaces don't regress.
- **No runtime data path changes**: only flag registration metadata changes. `BindFormatFromFlag` (`common/flags/flags.go:67`) looks up `"format"` by name and is unaffected.
- **No code that parses CLI args by shorthand string**: ripgrep confirms no source code keys off `-f` as a literal in production paths; only doc strings and a couple of agent-facing markdown files.

### Files touched

**Source (3 files)**

- `common/flags/flags.go` (1 doc-comment line + 1 call signature)
- `neo4j-cli/internal/subcommands/update/update.go` (1 call signature: `BoolVar` → `BoolVarP`)
- `neo4j-cli/internal/versioncheck/versioncheck_test.go` (1 call signature, test helper)

**Tests (2 files, additions only)**

- `common/flags/flags_test.go` (1 new assertion / case)
- `neo4j-cli/internal/subcommands/update/update_test.go` (1 new shorthand assertion + 1 positive parse test, optional negative `update check -f` case)

**Hand-edited markdown (7 files)**

- `neo4j-cli/internal/skill/additions.md`
- `neo4j-cli/internal/skill/query-additions.md`
- `neo4j-cli/aura/internal/skill/additions.md`
- `AGENTS.md`
- `CONTRIBUTING.md`
- `.agents/architecture.md`
- `.agents/repo-layout.md`

**Regenerated bundles (committed but not hand-edited)**

- `neo4j-cli/internal/skill/bundle/SKILL.md`
- `neo4j-cli/internal/skill/bundle/query-additions.md`
- `neo4j-cli/internal/skill/bundle/references/skill.md`
- `neo4j-cli/internal/skill/bundle/references/query.md`
- `neo4j-cli/internal/skill/bundle/references/update.md`
- `neo4j-cli/aura/internal/skill/bundle/SKILL.md`
- (any other `references/*.md` that pflag.Flag.Shorthand happens to touch — confirmed at generate time)

**New (1 file)**

- `.changes/unreleased/neo4j-cli-Minor-<YYYYMMDD>-<HHMMSS>.yaml`

## Acceptance Criteria

- [ ] `common/flags/flags.go` `RegisterOutputFlag` calls `cmd.PersistentFlags().String("format", "", …)` (no `"f"` shorthand).
- [ ] `common/flags/flags.go:32` doc comment reads `// RegisterOutputFlag adds a persistent --format flag …` (no `/-f`).
- [ ] `neo4j-cli/internal/subcommands/update/update.go` `--force` is registered via `BoolVarP(&force, forceFlag, "f", false, …)`.
- [ ] `neo4j-cli/internal/versioncheck/versioncheck_test.go` test-root uses `String("format", "", "format")`.
- [ ] A new test asserts `RegisterOutputFlag`'s registered `format` flag has empty `Shorthand`.
- [ ] A new test asserts the `update --force` flag has `Shorthand == "f"`, and `update -f --version <valid>` parses without cobra error.
- [ ] All 7 hand-edited markdown source files no longer contain ``(shorthand `-f`)`` or ``-f toon``/``-f json``/``-f table`` references.
- [ ] `go run ./neo4j-cli aura instance list --help` flag block shows only `--format`, no `-f` line.
- [ ] `go run ./neo4j-cli aura instance list -f json` exits non-zero with cobra's "unknown shorthand flag: 'f'".
- [ ] `go run ./neo4j-cli aura instance list --format json` still works.
- [ ] `go run ./neo4j-cli update --help` shows `-f, --force` in its flag block.
- [ ] `go run ./neo4j-cli update -f --version v0.1.0 --help` (or equivalent dry parse) confirms cobra accepts `-f` on update.
- [ ] `go run ./neo4j-cli update check -f` exits non-zero (child does not inherit `--force`).
- [ ] Both skill bundles are regenerated and committed in the same change; `git diff` after a clean `go generate` is empty.
- [ ] `make test` passes (including `TestGenerator_RoundTrip` and the new regression cases).
- [ ] `make fmt-check` reports no files needing `gofmt`.
- [ ] `make lint` is clean.
- [ ] `make generate-check` is clean.
- [ ] A new `.changes/unreleased/neo4j-cli-Minor-*.yaml` exists with the body from REQ-F-015.
- [ ] PR branch is `oskar/cli-85-drop-format-short-flag` (or equivalent `oskar/` prefix).

## Out of Scope

- Adding `--force` to commands other than `update`.
- Introducing a `TOON=1` (or similar) env var as an alternative to `--format toon` typing.
- Re-wording `update`'s `Example:` block to feature `-f` (cobra's `--help` already shows the shorthand).
- Updating historical entries in `CHANGELOG-neo4j.md` / `CHANGELOG-aura.md`.
- Updating `README.md`, install scripts, or `gh-pages` — verified to not reference `-f`.
- Auditing the rest of the flag space for other shorthand collisions (separate effort if needed).

## Open Questions

- None. Changelog kind (`Minor`) and scope (drop `-f` from `--format` AND wire `-f` on `update --force` in the same PR) confirmed during planning.
