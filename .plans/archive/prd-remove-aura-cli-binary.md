# PRD: Remove standalone aura-cli from the build and release pipeline

## Overview

The repository currently produces two binaries — `neo4j-cli` (the super-CLI) and `aura-cli` (the standalone Aura CLI). The standalone `aura-cli` binary is no longer needed: all Aura functionality is preserved through the `neo4j-cli aura ...` subcommand tree. This PRD removes `aura-cli` from the build, release, and distribution pipelines, while leaving the underlying Go packages (`neo4j-cli/aura/...`) intact so the `aura` subcommand of `neo4j-cli` keeps working unchanged.

The Go source for the standalone entrypoint (`neo4j-cli/aura/cmd/main.go`) remains in place — it will simply no longer be built or shipped. Historical changelog material (`CHANGELOG-aura.md`, `.changes/aura-cli/v*.md`) is preserved as-is for record-keeping; only the active publishing path is removed.

## Goals

- Stop producing the `aura-cli` binary in release builds.
- Stop publishing `aura-cli_*.tar.gz` / `.zip` archives to GitHub Releases.
- Remove all CI/workflow logic that branches on the existence of aura-cli changelog entries.
- Prevent new aura-cli changelog entries from being authored via `changie`.
- Remove dev-loop affordances (`make build-aura`, `make run-aura`, snapshot copy step) so the Makefile does not silently produce an unshipped binary.
- Scrub user-facing docs (`README.md`, `CONTRIBUTING.md`) of references to the standalone `aura-cli` binary, keeping mentions of the `neo4j-cli aura` subcommand.
- Keep `make build`, `make test`, `make lint`, `make fmt-check`, `make generate-check`, and `make snapshot` green after the change.

## Non-Goals

- **Source removal.** `neo4j-cli/aura/cmd/main.go` and the `neo4j-cli/aura/...` package tree stay in place. The `aura` subcommand under `neo4j-cli` is unaffected.
- **Historical changelog removal.** `CHANGELOG-aura.md` and `.changes/aura-cli/v*.md` are preserved as frozen history. They are not deleted, edited, or annotated.
- **Migration guidance in README/CONTRIBUTING.** No "aura-cli has moved" / migration section is added to `README.md` or `CONTRIBUTING.md` — those files are scrubbed of standalone references but no replacement guidance is written. (A short deprecation note in `CHANGELOG-aura.md` is in scope — see REQ-F-009.)
- **Homebrew tap cleanup.** The `homebrew-tap` formula was already neo4j-cli-only; no changes there.
- **npm distribution changes.** The `@neo4j-labs/cli` package and its 8 platform packages already ship neo4j-cli only; out of scope.
- **Behavioural changes to the `neo4j-cli aura` subcommand.** Nothing user-visible inside the aura subcommand changes.

## Requirements

### Functional Requirements

- **REQ-F-001:** `.goreleaser.yaml` — remove the `aura-cli` entry from `builds:`, the `aura-cli-archive` entry from `archives:`, the `CHANGELOG-aura.md` entry from `release.extra_files:`, and `aura-cli` from `notarize.macos[0].ids:`. Only `neo4j-cli` build/archive/notarize entries remain.
- **REQ-F-002:** `.github/workflows/release.yml`:
  - Remove `CHANGELOG-aura.md` from the `on.push.paths:` trigger list (trigger only on `CHANGELOG-neo4j.md`).
  - Remove the `Get aura-cli latest version` step and `AURA_VERSION` extraction from the `Extract version numbers` step.
  - Remove the `INCLUDE_AURA` detection from the `Detect changed changelogs` step (the step now only computes `include_neo4j`, or is removed if redundant).
  - Remove the `aura-cli` Versions/Changes sections from the `Generate release notes` step; the rendered notes contain only the neo4j-cli version and changelog body.
  - Remove `AURA_CLI_VERSION` from `Add env vars` and from the GoReleaser action's `env:` block.
- **REQ-F-003:** `.github/workflows/changie.yml`:
  - Remove the `Compute next pre-release versions` aura branch (only the `neo4j` output is computed).
  - Remove `has_aura` detection from the `Detect unreleased changes per project` step; the workflow gates only on `has_neo4j`.
  - Remove the `Batch aura-cli changes` step.
  - Remove the `Get aura-cli latest version` step and the `AURA_VERSION` line from `Extract version numbers`.
  - Simplify the `Compute PR metadata` step's branching: collapse the three-branch (`HAS_AURA && HAS_NEO4J` / `HAS_AURA` only / `HAS_NEO4J` only) logic to a single neo4j-cli release path.
- **REQ-F-004:** `.changie.yaml` — remove the `aura-cli` entry from `projects:`. The `neo4j-cli` entry is the only remaining project.
- **REQ-F-005:** `Makefile`:
  - Remove `build-aura` and `run-aura` targets.
  - Update `build` to depend only on `build-neo4j` (or inline the single command).
  - Remove `cp dist/aura-cli_*/aura-cli bin/aura-cli` from the `snapshot` target.
  - Remove `build-aura` / `run-aura` from the `.PHONY` list.
- **REQ-F-006:** `README.md` — remove the 3 references to standalone `aura-cli` (install / usage). Mentions of the `neo4j-cli aura` subcommand are preserved.
- **REQ-F-007:** `CONTRIBUTING.md` — scrub the 18 references to standalone `aura-cli` covering install instructions, release process description, build commands, and any "two binaries" framing. Mentions of the `neo4j-cli aura` subcommand and historical context (`.changes/aura-cli/`, `CHANGELOG-aura.md` as historical record) may remain where factual.
- **REQ-F-008:** `AGENTS.md` (the canonical doc, with `CLAUDE.md` as a symlink) — update sections that describe two-binary build output, dual-project changie workflow, GoReleaser dual-build config, release workflow's aura handling, and any `make build-aura` / `make run-aura` references, to reflect the single-binary state. Historical notes about changie multi-project and dual-project release flow may be deleted (no longer applicable) rather than updated.
- **REQ-F-009:** `CHANGELOG-aura.md` — prepend a deprecation note above the most recent version entry recording that `aura-cli` will no longer be released as a standalone binary going forward, and pointing readers to `neo4j-cli aura ...` as the replacement. The note is hand-authored (not generated by changie) since the changie project entry has been removed; format it consistently with surrounding `## vX.Y.Z - DATE` blocks (e.g., a leading `## Deprecated - <date>` heading or equivalent prominent header) so it renders cleanly in the existing markdown. The pre-existing version entries below the note are unchanged.

### Non-Functional Requirements

- **REQ-NF-001:** All existing CI gates continue to pass after the change: `make build`, `make test`, `make fmt-check`, `make lint`, `make generate-check`, `make license-check`. Verified locally and on the CI matrix (ubuntu, windows, macos).
- **REQ-NF-002:** `make snapshot` produces only `bin/neo4j-cli` and exits cleanly (no error from a missing `dist/aura-cli_*/` path).
- **REQ-NF-003:** A trial run of GoReleaser in snapshot mode (`goreleaser release --snapshot --clean --skip=publish,sign,notarize` with `HOMEBREW_TAP_GITHUB_TOKEN=stub`) produces exactly the expected `neo4j-cli` archives in `dist/` (linux/windows/darwin × amd64/arm64), no `aura-cli_*` artifacts, and a single Homebrew formula.
- **REQ-NF-004:** Existing GitHub Releases that contain `aura-cli_*` archives remain untouched — no retroactive deletion. New releases simply no longer include them.
- **REQ-NF-005:** The standalone Go entrypoint `neo4j-cli/aura/cmd/main.go` continues to compile (covered by `go test ./...`'s implicit build of all packages), even though no Make target builds it.
- **REQ-NF-006:** All work for this PRD is performed on a dedicated feature branch off `main` (e.g., `remove-aura-cli-binary` or similar). No commits land directly on `main`. The change is delivered as a single PR for review and merge.

## Technical Considerations

**GoReleaser config simplification.** With only one `builds:` entry remaining, the `id:` and per-archive `ids:` filters could in principle be simplified, but keeping them explicit (matching the current pattern) keeps the diff focused and avoids subtle template behaviour changes (`{{ .Binary }}` vs `{{ .ProjectName }}` produce the same value when there is one binary, so the archive `name_template` does not need editing). Recommend: minimal edit — drop the aura entries, leave the surrounding structure as-is.

**Trigger surface for release.yml.** Once `CHANGELOG-aura.md` is dropped from `paths:`, an edit to that file (e.g., manual touch-up) will not trigger a release. This matches intent — there will be no more aura-cli releases.

**Workflow gate cleanup.** `release.yml`'s job-level `outputs:` block exposes `include_neo4j` to the downstream `publish-npm.yml`. After removal, `include_neo4j` is effectively always `true` whenever the workflow runs (since the only trigger path is `CHANGELOG-neo4j.md`). The output and downstream gate can be left in place defensively (it remains correct) rather than removed — a smaller, lower-risk diff.

**Changie unreleased entry detection.** With aura-cli removed from `.changie.yaml`'s `projects:`, `changie new` will no longer offer aura-cli as a target. Any pre-existing `.changes/unreleased/*.yaml` carrying `project: aura-cli` would, in principle, conflict; verified via `ls .changes/unreleased/` that none exist (only `neo4j-cli-Minor-20260506-070000.yaml` is present).

**Standalone source code rot risk.** Per the Q1 decision, `neo4j-cli/aura/cmd/main.go` stays. `go test ./...` builds every package including `./neo4j-cli/aura/cmd`, so a compile break would surface in CI. No additional gate is needed; no comment is added to the file (per the "no rot-prone comments" CLAUDE.md guidance).

**Skill bundle regeneration.** The standalone aura-cli has its own per-binary skill template at `neo4j-cli/aura/internal/skill/`. The skill subsystem is per-binary (`common/skill/` is binary-agnostic; each binary has its own template + bundle). Removing the standalone build does not require changing the skill template — `go generate ./...` over the aura skill template is still valid because the underlying `aura.NewStandaloneCmd` is still a real cobra tree. `make generate-check` should remain green.

**Notarization.** macOS code-signing/notarization for `aura-cli` is removed by dropping it from `notarize.macos[0].ids:`. `neo4j-cli` continues to be signed and notarized.

**Risk: stale environment variable.** `AURA_CLI_VERSION` is referenced in three places: `release.yml` (set, then re-referenced via `${{ env.AURA_CLI_VERSION }}` in the GoReleaser action's `env:` block) and `.goreleaser.yaml` (consumed in the aura-cli build's `ldflags`). All three references must be removed in the same change — leaving any one in place produces a confusing partial diff. Verified there are no other consumers of this env var.

## Acceptance Criteria

- [ ] `.goreleaser.yaml` contains a single `builds:` entry (`neo4j-cli`), a single `archives:` entry, no `CHANGELOG-aura.md` in `release.extra_files:`, and `notarize.macos[0].ids:` is `[neo4j-cli]`.
- [ ] `.github/workflows/release.yml` triggers only on `CHANGELOG-neo4j.md`; no `AURA_CLI_VERSION`, `INCLUDE_AURA`, `aura-cli` version step, or aura release-notes branch remains.
- [ ] `.github/workflows/changie.yml` has no `has_aura` / `aura-cli` batch / latest-aura steps; PR metadata logic has a single neo4j-cli branch.
- [ ] `.changie.yaml` has a single `projects:` entry (`neo4j-cli`).
- [ ] `Makefile` has no `build-aura` or `run-aura` targets; `make build` produces only `bin/neo4j-cli`; `make snapshot` produces only `bin/neo4j-cli`; `.PHONY` is updated.
- [ ] `make build`, `make test`, `make lint`, `make fmt-check`, `make generate-check`, `make license-check` all pass locally.
- [ ] `goreleaser release --snapshot --clean --skip=publish,sign,notarize` (with `HOMEBREW_TAP_GITHUB_TOKEN=stub`) produces only `neo4j-cli_*` archives in `dist/`; no `aura-cli_*` files anywhere in `dist/`.
- [ ] `README.md` contains no references to installing or running the standalone `aura-cli` binary.
- [ ] `CONTRIBUTING.md` contains no references to building, running, or releasing the standalone `aura-cli` binary; references to the `neo4j-cli aura` subcommand or historical changie multi-project context (where factually correct) may remain.
- [ ] `AGENTS.md` reflects single-binary build state; obsolete dual-project sections are removed or updated.
- [ ] `CHANGELOG-aura.md` has a hand-authored deprecation note prepended above the most recent version entry, stating that aura-cli will no longer be released as a standalone binary and directing readers to `neo4j-cli aura ...`. Existing version entries below the note are unchanged.
- [ ] `.changes/aura-cli/v*.md` files are unchanged on disk.
- [ ] `neo4j-cli/aura/cmd/main.go` and `neo4j-cli/aura/...` package tree are unchanged.
- [ ] All work lands on a dedicated feature branch off `main` and is merged via PR; no direct commits to `main`.
- [ ] CI matrix (ubuntu, windows, macos) passes on the change.
- [ ] A changelog entry (`make changelog --projects neo4j-cli --kind Minor`) is added documenting the build-pipeline change. Aura-cli is not included as a target since it has no user-facing change for that binary's audience.

## Out of Scope

- Deletion of `neo4j-cli/aura/cmd/main.go` or any `neo4j-cli/aura/...` Go source.
- Deletion or editing of `.changes/aura-cli/v*.md` (per-version files). The single hand-authored deprecation note prepended to `CHANGELOG-aura.md` (REQ-F-009) is the only edit to that file; existing version entries below the note are not modified.
- Adding deprecation notices, migration guides, or "aura-cli has moved" messaging anywhere.
- Removing or modifying past GitHub Release artifacts that include `aura-cli_*.tar.gz` / `.zip`.
- Changes to the Homebrew tap (`neo4j-labs/homebrew-tap`).
- Changes to the npm distribution layout.
- Changes to the `neo4j-cli aura` subcommand surface or behaviour.
- Removal of the `aura` subcommand from `neo4j-cli`.

## Open Questions

- None at this time. All scope and behaviour decisions answered during PRD intake (Q1–Q5).
