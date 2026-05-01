# PRD: Per-Project Independent Releases

## Overview

The changie workflow (`changie.yml`) fails with `Error: no unreleased changes found for automatic bumping` when one project has no unreleased change files but the other does. Both projects must be able to generate changelogs, release PRs, and GoReleaser runs independently — a release for one should not require or block a release for the other.

## Goals

- Allow a release PR to be created when only one project (aura-cli or neo4j-cli) has unreleased changes.
- Skip batching, changelog merging, and PR creation entirely when neither project has changes.
- Reflect only the released project(s) in PR titles, branch names, and release notes.
- Trigger GoReleaser when either changelog file (`CHANGELOG-aura.md` or `CHANGELOG-neo4j.md`) is updated.

## Non-Goals

- Changing the version scheme or pre-release tagging strategy.
- Supporting more than two projects.
- Decoupling the GoReleaser run per project (both binaries are still built in the same GoReleaser run).

## Requirements

### Functional Requirements

- REQ-F-001: `changie.yml` must detect which projects have unreleased change files before running `changie batch auto`, and skip the batch step for any project with no changes.
- REQ-F-002: If neither project has unreleased changes, `changie.yml` must exit early without running merge, version extraction, or PR creation.
- REQ-F-003: The release PR title must name only the project(s) being released (e.g. `Release neo4j-cli v0.0.2-alpha.0` for a single-project release; the existing combined format for a dual-project release).
- REQ-F-004: The release PR branch name must be derived from the released project(s). Single-project releases use `release/<project>-<version>`; dual-project releases keep the current `release/neo4j-cli-<version>` format.
- REQ-F-005: The release PR body must list only the project(s) being released with links to the relevant changelog file(s).
- REQ-F-006: `release.yml` must trigger on changes to either `CHANGELOG-neo4j.md` or `CHANGELOG-aura.md`.
- REQ-F-007: In `release.yml`, the `## Changes` section of `release-notes.md` must only include subsections for projects whose changelog file was updated in the commit that triggered the workflow. Detection must use `git diff HEAD~1 --name-only`.

### Non-Functional Requirements

- REQ-NF-001: No new external actions or tools may be introduced — use only shell logic, existing changie-action, and existing GitHub Actions primitives.
- REQ-NF-002: The workflow must remain idempotent: re-running on the same commit must not create duplicate PRs or version files (already handled by `create-pull-request` action and changie's existing alpha-counter logic).

## Technical Considerations

### Detecting unreleased changes per project

Unreleased change files live in `.changes/unreleased/` as YAML files containing a `project:` field. Detection can be done with:

```bash
grep -rl '^project: aura-cli' .changes/unreleased/ 2>/dev/null | grep -q .
```

This must run before any `changie batch auto` step and set `GITHUB_OUTPUT` flags (`has_aura`, `has_neo4j`) used by downstream `if:` conditions.

### Conditional batch steps

The two `changie-action` batch steps gain `if: steps.detect.outputs.has_aura == 'true'` / `if: steps.detect.outputs.has_neo4j == 'true'` conditions. No changes needed to the changie-action invocations themselves.

### Early-exit when no changes

Add a step after detection that exits 0 (success) and emits a summary when both flags are false, and add `if: steps.detect.outputs.has_aura == 'true' || steps.detect.outputs.has_neo4j == 'true'` on all subsequent steps (merge, version extraction, PR creation).

GitHub Actions does not support short-circuit job exit natively; the `if:` condition approach on every downstream step is the most straightforward solution without requiring a separate job split.

### Dynamic PR metadata

A `Compute PR metadata` run step after version extraction reads both `has_*` outputs and sets `pr_title`, `pr_branch`, and `pr_body` outputs used by the `create-pull-request` step.

### release.yml trigger and per-project release notes

Add `CHANGELOG-aura.md` to `paths:` in the `on.push` trigger. In the `Generate release notes` step, before writing each project's `## Changes` subsection, check whether that project's changelog was updated in the triggering commit:

```bash
git diff HEAD~1 --name-only | grep -q 'CHANGELOG-neo4j.md' && INCLUDE_NEO4J=true || INCLUDE_NEO4J=false
git diff HEAD~1 --name-only | grep -q 'CHANGELOG-aura.md'  && INCLUDE_AURA=true  || INCLUDE_AURA=false
```

Replace the existing `[ -n "${NEO4J_BODY}" ]` / `[ -n "${AURA_BODY}" ]` guards with `[ "$INCLUDE_NEO4J" = "true" ]` / `[ "$INCLUDE_AURA" = "true" ]` guards. The `## Versions` section continues to list both current versions regardless (GoReleaser requires both `GORELEASER_CURRENT_TAG` and `AURA_CLI_VERSION`).

## Acceptance Criteria

- [ ] Pushing a commit with only neo4j-cli unreleased changes creates a PR titled `Release neo4j-cli <version>` on branch `release/neo4j-cli-<version>`, with no mention of aura-cli.
- [ ] Pushing a commit with only aura-cli unreleased changes creates a PR titled `Release aura-cli <version>` on branch `release/aura-cli-<version>`, with no mention of neo4j-cli.
- [ ] Pushing a commit with changes for both projects creates a combined PR using the existing format.
- [ ] Pushing a commit with no unreleased changes for either project results in the workflow completing successfully with no PR created.
- [ ] Merging a release PR that updates only `CHANGELOG-aura.md` triggers `release.yml` and produces `release-notes.md` with only an `### aura-cli` subsection under `## Changes`.
- [ ] Merging a release PR that updates only `CHANGELOG-neo4j.md` triggers `release.yml` and produces `release-notes.md` with only a `### neo4j-cli` subsection under `## Changes`.
- [ ] The existing combined-release path (both changelogs updated) continues to work as before.

## Out of Scope

- Splitting GoReleaser into per-project runs.
- Changing how `changie new` entries are authored or how the `.changie.yaml` is structured.
- Handling stable (non-pre-release) releases differently from alpha releases.

## Open Questions

None.
