# PRD: Monorepo Changelog Automation

## Overview

The repo currently has a single changie config driving one `CHANGELOG.md` and one version number shared by both `aura-cli` and `neo4j-cli`. This is the legacy of when only `aura-cli` existed. The goal is to introduce per-binary changelogs and versioning while eliminating any manual duplication: a change tagged as `aura-cli` automatically cascades into the `neo4j-cli` changelog and bumps both version numbers, without requiring a second `changie new` invocation. `neo4j-cli`-specific changes (future functionality) can still be added independently.

## Goals

- A single `changie new` call is sufficient for any change; no manual duplication across binaries
- `aura-cli` and `neo4j-cli` each maintain an independent version number and CHANGELOG
- A change to `aura-cli` automatically appears line-for-line in the `neo4j-cli` changelog and triggers a `neo4j-cli` version bump
- `neo4j-cli`-only changes appear only in the `neo4j-cli` changelog and do not bump `aura-cli`
- The CI release pipeline handles both changelogs and injects the correct version into each binary
- Both `aura-cli` and `neo4j-cli` continue to be released together in a single GoReleaser run

## Non-Goals

- Splitting the repo into separate repositories or Go modules
- Independent release cadences (both binaries are always released together)
- Changing the change kinds (Major/Minor/Patch) or changelog format beyond what is needed for the multi-project setup
- Automating PR review or changelog content generation (messages are still written by humans/agents)

## Requirements

### Functional Requirements

- REQ-F-001: The `.changie.yaml` config is migrated to use changie's `projects` feature with two projects: `aura-cli` (key: `aura-cli`) and `neo4j-cli` (key: `neo4j-cli`), each with their own `changesDir` and `changelogPath`
- REQ-F-002: Running `changie new` (or `make changelog`) prompts the developer to select a project (`aura-cli` or `neo4j-cli`) then proceeds as today (kind, body)
- REQ-F-003: Existing version history (`v0.1.0` – `v1.7.0`) is migrated into the `aura-cli` project's changes directory; `neo4j-cli` starts fresh from `v0.1.0`
- REQ-F-004: The `CHANGELOG.md` file is replaced by two separate files: `CHANGELOG-aura.md` and `CHANGELOG-neo4j.md`
- REQ-F-005: The CI `changie` workflow (`.github/workflows/changie.yml`) is updated to:
  1. Copy all unreleased `aura-cli` change files into the `neo4j-cli` unreleased directory (cascade step)
  2. Run `changie batch auto --project aura-cli`
  3. Run `changie batch auto --project neo4j-cli`
  4. Run `changie merge --project aura-cli` and `changie merge --project neo4j-cli`
  5. Determine both new version numbers (`changie latest --project aura-cli`, `changie latest --project neo4j-cli`)
  6. Open a single release PR that includes both updated changelogs and all batched change files
- REQ-F-006: The release PR title and body reference both versions (e.g. "Release aura-cli vX.Y.Z / neo4j-cli vA.B.C")
- REQ-F-007: The CI `release` workflow (`.github/workflows/release.yml`) is updated to:
  1. Trigger on changes to `CHANGELOG-neo4j.md` (the "master" release signal, always updated when any release happens)
  2. Read both latest versions: `AURA_CLI_VERSION=$(changie latest --project aura-cli)` and `NEO4J_CLI_VERSION=$(changie latest --project neo4j-cli)`
  3. Pass `NEO4J_CLI_VERSION` as `GORELEASER_CURRENT_TAG` (the GitHub Release tag)
  4. Pass `AURA_CLI_VERSION` as an additional env var for the aura-cli binary's version injection
- REQ-F-008: `.goreleaser.yaml` is updated so each build uses its own version env var in `ldflags`:
  - `aura-cli`: `-X "main.Version={{.Env.AURA_CLI_VERSION}}"`
  - `neo4j-cli`: `-X "main.Version={{.Env.GORELEASER_CURRENT_TAG}}"`
- REQ-F-009: The `Makefile` is updated with a `changelog` target that runs `changie new`, and separate `changelog-aura` / `changelog-neo4j` targets that pre-select the project (`changie new --project aura-cli` / `--project neo4j-cli`)
- REQ-F-010: `CONTRIBUTING.md` is updated to document the new workflow: run `make changelog` or `make changelog-aura` for aura-cli changes, explain the cascade behaviour, and remove references to manually duplicating changie entries
- REQ-F-011: `AGENTS.md` and `.agents/build.md` and `.agents/deployment.md` are updated to reflect the new multi-project changie setup and dual-version release flow
- REQ-F-012: The CI `release` workflow generates a combined `release-notes.md` file before invoking GoReleaser, structured as:
  ```
  ## Versions
  - neo4j-cli: <NEO4J_CLI_VERSION>
  - aura-cli: <AURA_CLI_VERSION>

  ## Changes
  <body of the neo4j-cli changelog entry for this version>
  ```
  This file is passed to GoReleaser via `--release-notes=release-notes.md`; both per-binary versions are visible in the GitHub Release body
- REQ-F-013: `.goreleaser.yaml` removes the `changelog: disable: false` auto-changelog (which would overwrite the custom notes) and attaches `CHANGELOG-aura.md` and `CHANGELOG-neo4j.md` as downloadable release assets via GoReleaser's `release.extra_files`, so users can browse full history from the GitHub Release page

### Non-Functional Requirements

- REQ-NF-001: The cascade copy step in CI must be idempotent — if no unreleased aura-cli changes exist, it must not fail
- REQ-NF-002: The migration of existing version history must preserve all existing changelog content exactly
- REQ-NF-003: `make changelog` / `make changelog-aura` must work locally without CI (no network required)

## Technical Considerations

### changie Projects Feature

changie v1.x supports a `projects:` array in `.changie.yaml`. Each project inherits root-level settings (kind format, change format, etc.) unless overridden. The project key is passed via `--project <key>` on all changie subcommands. When projects are configured, `changie new` automatically prompts the user to select a project before the usual kind/body prompts — no extra flags required for the interactive case.

Proposed project configuration additions to `.changie.yaml`:

```yaml
projects:
  - label: aura-cli
    key: aura-cli
    changesDir: .changes/aura-cli
    changelogPath: CHANGELOG-aura.md
  - label: neo4j-cli
    key: neo4j-cli
    changesDir: .changes/neo4j-cli
    changelogPath: CHANGELOG-neo4j.md
```

The root-level `changesDir`, `changelogPath`, and other fields are kept as fallback defaults but projects take precedence.

### Cascade Implementation

The cascade is a single shell step in the changie CI workflow, run before batching, that copies unreleased `aura-cli` changes into the `neo4j-cli` unreleased directory:

```bash
cp .changes/aura-cli/unreleased/*.yaml .changes/neo4j-cli/unreleased/ 2>/dev/null || true
```

This is intentionally simple. The copied files are identical YAML — same kind, same body — so they produce identical changelog entries in both projects. changie then assigns independent version numbers to each project based on the kinds present.

### Version Injection in GoReleaser

Currently both builds use `-X "main.Version={{.Env.GORELEASER_CURRENT_TAG}}"`. With two versions, `aura-cli` needs a separate env var. The `.goreleaser.yaml` `builds` entries are updated to:

```yaml
- id: aura-cli
  ldflags:
    - -X "main.Version={{.Env.AURA_CLI_VERSION}}"

- id: neo4j-cli
  ldflags:
    - -X "main.Version={{.Env.GORELEASER_CURRENT_TAG}}"
```

`GORELEASER_CURRENT_TAG` continues to drive the GitHub Release tag (the neo4j-cli version), while `AURA_CLI_VERSION` is set as an additional env var by the release workflow before calling GoReleaser.

### Migration of Existing History

The existing `.changes/v*.md` files and `CHANGELOG.md` belong to `aura-cli`. Migration steps:

1. Move `.changes/unreleased/` → `.changes/aura-cli/unreleased/`
2. Move `.changes/v*.md` (all versioned files) → `.changes/aura-cli/`
3. Move `header.tpl.md` → `.changes/aura-cli/header.tpl.md` (or keep shared at root)
4. Copy `CHANGELOG.md` → `CHANGELOG-aura.md`
5. Create empty `CHANGELOG-neo4j.md` with the same header
6. Create `.changes/neo4j-cli/unreleased/` directory

### Release Trigger

Currently the release workflow triggers on `paths: ["CHANGELOG.md"]`. After migration, change to `paths: ["CHANGELOG-neo4j.md"]`. Since the cascade ensures neo4j-cli is always updated when aura-cli changes, this file is always modified in a release PR, making it the reliable single trigger.

### GoReleaser Release Notes — Single Run Limitation

GoReleaser's `--release-notes` flag is per-release, not per-binary. A single GoReleaser run produces exactly one GitHub Release with one body. There is no native way to attach separate notes to each binary. The three realistic options, and why we chose option B:

- **Option A — Use `CHANGELOG-neo4j.md` alone.** Simple, but the GitHub Release body doesn't show which version of `aura-cli` was released alongside `neo4j-cli`.
- **Option B — Pre-generate combined notes in CI (chosen).** A short shell script assembles a `release-notes.md` with a versions header and the neo4j-cli changelog body. Passed to `--release-notes`. Both version numbers are visible in the GitHub Release.
- **Option C — Two separate GoReleaser runs.** Separate GitHub Releases with separate tags. Contradicts the "single combined release" requirement and doubles CI complexity.

Additionally, GoReleaser's built-in `changelog:` auto-generation (currently `disable: false`) must be disabled or it will overwrite the custom release notes. Setting `changelog: disable: true` in `.goreleaser.yaml` is required alongside custom `--release-notes`.

GoReleaser's `release.extra_files` (v2: `extra_files` under `release:`) attaches arbitrary files as downloadable assets on the GitHub Release. Using this to attach both `CHANGELOG-aura.md` and `CHANGELOG-neo4j.md` gives users full history without bloating the release body.

### Backward Compatibility

After migration, `CHANGELOG.md` can be removed or left as a redirect note. Any existing tooling referencing `CHANGELOG.md` (e.g. GoReleaser `--release-notes`) must be updated to reference `CHANGELOG-neo4j.md` (or the appropriate per-binary file, depending on which release notes are most user-relevant for the GitHub Release).

## Acceptance Criteria

- [ ] `changie new` prompts for project selection (`aura-cli` or `neo4j-cli`) before kind/body
- [ ] `make changelog-aura` creates an unreleased entry in `.changes/aura-cli/unreleased/` without prompting for project
- [ ] `make changelog-neo4j` creates an unreleased entry in `.changes/neo4j-cli/unreleased/` without prompting for project
- [ ] Running the cascade step + `changie batch auto --project neo4j-cli` when only aura-cli unreleased entries exist produces a new neo4j-cli version with the same entries as aura-cli
- [ ] `CHANGELOG-aura.md` contains the full historical aura-cli changelog (v0.1.0 – v1.7.0+)
- [ ] `CHANGELOG-neo4j.md` exists and is populated by the first release after migration
- [ ] The release workflow sets `AURA_CLI_VERSION` and `GORELEASER_CURRENT_TAG` independently
- [ ] `aura-cli --version` and `neo4j-cli --version` report different version strings when versions differ
- [ ] GoReleaser snapshot build succeeds with both `AURA_CLI_VERSION` and `GORELEASER_CURRENT_TAG` set to test values
- [ ] GitHub Release body shows both version numbers and the combined changelog body (verified against a snapshot/dry-run)
- [ ] `CHANGELOG-aura.md` and `CHANGELOG-neo4j.md` appear as downloadable assets on the GitHub Release
- [ ] GoReleaser auto-changelog generation is disabled (`changelog: disable: true`) so custom release notes are not overwritten
- [ ] `CONTRIBUTING.md` documents `make changelog`, `make changelog-aura`, the cascade behaviour, and removes manual duplication instructions
- [ ] Existing `aura-cli` version history is intact in `CHANGELOG-aura.md` after migration

## Out of Scope

- Independent release cadences (aura-cli and neo4j-cli are always released together in one run)
- Automating changelog message content generation
- Adding a `neo4j-cli`-only changie entry to an existing aura-cli release (entries that pre-date the migration)
- Changing the GitHub Release structure (still one release per run)

## Open Questions

1. ~~**Release notes for GitHub Release:**~~ **Resolved.** GoReleaser uses a single `--release-notes` file per run — there is no per-binary release notes capability. A CI script pre-generates `release-notes.md` with a versions header (`neo4j-cli: vA.B.C`, `aura-cli: vX.Y.Z`) followed by the neo4j-cli changelog body. Both `CHANGELOG-aura.md` and `CHANGELOG-neo4j.md` are attached as downloadable release assets. `changelog: disable: true` is set in `.goreleaser.yaml`.
2. **neo4j-cli initial version:** What should `neo4j-cli` start at — `v0.1.0` (fresh start) or a version that signals it already contains a mature `aura-cli` (e.g. `v1.0.0`)? This is a product decision.
3. **Old `CHANGELOG.md`:** Should the root `CHANGELOG.md` be removed, renamed, or kept with a forwarding note pointing to `CHANGELOG-aura.md`?
