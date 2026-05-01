# PRD: Per-Project Change Summaries in GitHub Release Notes

## Overview

GitHub Releases currently show only the merge commit message (e.g. "Release aura-cli v1.7.1-alpha.0 / neo4j-cli v0.0.1-alpha.0 (#6)") instead of the changelog body. This PRD covers fixing the release notes generation in `release.yml` so that each project's changes appear in a dedicated section on the GitHub Release page.

## Goals

- GitHub Releases display a human-readable summary of what changed in each project.
- Changes are organised per project so readers can quickly see what's new in aura-cli vs neo4j-cli independently.

## Non-Goals

- The release PR body (opened by `changie.yml`) is out of scope.
- Changelog file formats (`.changes/*/`) are not changing.
- No changes to how changie batches or merges versions.

## Requirements

### Functional Requirements

- REQ-F-001: The GitHub Release body must include a `## Changes` section with a subsection per project (`### aura-cli` and `### neo4j-cli`).
- REQ-F-002: Each project subsection must contain the full changelog body for that release (kinds and bullet points), read from `.changes/<project>/<version>.md` with the version header line stripped.
- REQ-F-003: The `## Versions` section must remain, listing both binary versions as at present.
- REQ-F-004: If a project's changes file is absent or empty (no changelog entries), its subsection must be omitted gracefully rather than printing an empty heading.
- REQ-F-005: The fix must address the current regression where GoReleaser falls back to the commit message — confirm `release-notes.md` is written to the workspace root before the GoReleaser step runs, and that the path in `--release-notes` matches.

### Non-Functional Requirements

- REQ-NF-001: The shell script changes in `release.yml` must be POSIX-compatible (ubuntu runner, bash).
- REQ-NF-002: No new external actions or dependencies introduced.

## Technical Considerations

- **Root cause investigation**: The current `release.yml` generates `release-notes.md` with `printf` redirected into `release-notes.md`, then passes `--release-notes=release-notes.md` to GoReleaser. If GoReleaser is showing the commit message, the file likely isn't being generated or found. Verify the working directory is consistent between the "Generate release notes" step and the GoReleaser step.
- **Reading both version files**: After extracting `AURA_VERSION` and `NEO4J_VERSION`, read `.changes/aura-cli/${AURA_VERSION}.md` in addition to the existing neo4j-cli read. Strip the first line of each with `tail -n +2`.
- **Conditional sections**: Use a shell guard (`if [ -n "$BODY" ]`) before writing each project section so an empty file doesn't produce a bare `### aura-cli` heading.
- **Duplicate content**: At present aura-cli changes cascade into neo4j-cli, so both files will contain the same entries. This is acceptable — the per-project structure makes the intent clear and will remain correct if the projects diverge.
- **Format**: Follow the existing Markdown conventions already used in `.changes/` files (headings, bullet points).

## Acceptance Criteria

- [ ] A test release (snapshot or dry-run) produces a `release-notes.md` that contains `## Versions`, `### aura-cli`, and `### neo4j-cli` sections with non-empty bodies.
- [ ] The GitHub Release page for the next real release shows both project sections with their change bullets.
- [ ] GoReleaser no longer falls back to the commit message as release body.
- [ ] If one project's version file is missing or empty, the workflow does not fail and omits only that project's section.

## Out of Scope

- Changing the release PR body created by `changie.yml`.
- Adding per-project sections to `CHANGELOG-aura.md` or `CHANGELOG-neo4j.md`.
- Altering the changie batching or cascade logic.

## Open Questions

- None — scope is fully defined.
