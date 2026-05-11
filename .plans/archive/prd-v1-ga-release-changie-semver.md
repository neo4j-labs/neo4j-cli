# PRD: v1 GA Release and Changie-Driven SemVer

## Overview

Prepare the release workflow and GoReleaser config to ship neo4j-cli v1.0.0 as a proper GA (non-prerelease) GitHub Release, and update the changie workflow to use `batch auto` so all future versions are driven by Major/Minor/Patch changelog entries rather than a hardcoded alpha version string.

## Goals

- Ship neo4j-cli v1.0.0 as a full GA GitHub Release (not marked prerelease).
- Remove the hardcoded `v0.1.0 --prerelease alpha.N` version logic from `changie.yml`.
- Switch to `changie batch auto --project neo4j-cli` so the next version is always derived from the highest-kind unreleased change entry.
- Confirm that npm, PyPI, and the website propagation workflows handle a non-alpha version correctly and produce the expected artifacts without manual intervention.

## Non-Goals

- **Merging the v1.0.0 release PR or executing the actual release** — this work ends when the PR is open and ready to merge. The merge is a deliberate, separate human action.
- Post-release distribution channel verification (npm dist-tag, PyPI wheel, website, Homebrew) — that happens after the release PR is merged, not as part of this work.
- Changing how `release.yml` detects or consumes the version (it already uses `changie latest`).
- Modifying `publish-npm.yml`, `publish-pypi.yml`, or `update-website.yml` — all three are version-format-agnostic and handle `1.0.0` correctly without changes.
- Adding pre-release support for future alpha/beta/rc cycles — those will use separate branches.
- Releasing `aura-cli` as part of this work.

## Requirements

### Functional Requirements

- REQ-F-001: `.goreleaser.yaml` `release.prerelease` must be set to `false` so GitHub marks the v1.0.0 (and all future) releases as GA.
- REQ-F-002: `changie.yml` "Compute next pre-release versions" step must be removed entirely (it parses `alpha.[0-9]*` filenames and produces the `steps.prerelease` output — no longer needed).
- REQ-F-003: The `changie batch` invocation in `changie.yml` must change from `batch v0.1.0 --project neo4j-cli --prerelease ${{ steps.prerelease.outputs.neo4j }}` to `batch auto --project neo4j-cli`.
- REQ-F-004: A Major-kind changie entry must be authored for the v1.0.0 release (body: "GA release of neo4j-cli" or equivalent) so that `batch auto` increments from `v0.1.0-alpha.12` to `v1.0.0`.
- REQ-F-005: After all changes land on `main`, the changie workflow must detect the pending Major entry and open a release PR titled "Release neo4j-cli v1.0.0". The scope of this work ends here — merging the PR is a separate human action.

### Non-Functional Requirements

- REQ-NF-001: The change to `changie.yml` must not break the PR creation flow for future Minor/Patch releases — `changie batch auto` must correctly resolve the version for all three kinds.
- REQ-NF-002: No manual version strings should remain in `changie.yml` after this change.
- REQ-NF-003: No code changes are required in any downstream workflow (`publish-npm.yml`, `publish-pypi.yml`, `update-website.yml`) — they already handle stable semver versions correctly (audited and verified as part of PRD preparation).

## Technical Considerations

**changie `batch auto` from an alpha version — VERIFIED:** Tested locally with `changie v1.24.0`. With a Major kind entry present, `changie batch auto --project neo4j-cli --dry-run` resolves to `v1.0.0` from `v0.1.0-alpha.12`. No one-off explicit version needed. The `-alpha.12` pre-release suffix is treated as a pre-release of `0.1.0`, so a major bump correctly promotes to `1.0.0`. For reference: a Patch-only bump from `v0.1.0-alpha.12` resolves to `v0.1.0` (promotes the alpha to the stable patch release), and future Patch bumps from a GA baseline like `v1.0.0` will produce `v1.0.1` as expected.

**`release.yml` version flow:** `release.yml` runs `changie latest --project neo4j-cli`, strips the `neo4j-cli` prefix (e.g., `neo4j-cliv1.0.0` → `v1.0.0`), and sets `GORELEASER_CURRENT_TAG`. This already works for any semver string — no changes needed.

**Downstream publish workflows — audit summary:** All three downstream workflows handle `1.0.0` correctly without code changes:

- **npm (`publish-npm.yml` + `distribution/npm/publish.sh`):** Validates version with regex `^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$` — `1.0.0` passes. The dist-tag logic uses glob patterns (`*-alpha*`, `*-beta*`, `*-rc*`, `*-*`, `*`) — a plain `1.0.0` falls through to the empty-tag case and publishes to `latest`. This is the **correct** GA behaviour and a notable difference from alpha releases which published to the `alpha` dist-tag.
- **PyPI (`publish-pypi.yml` + `version-to-pep440.sh`):** The PEP 440 conversion script has two branches: plain `X.Y.Z` → passthrough; `X.Y.Z-(alpha|beta|rc).N` → normalised pre-release form. `1.0.0` hits the passthrough branch and the wheel is built as `1.0.0`. Note: any future pre-release version must use exactly the `-(alpha|beta|rc).N` suffix format or the PyPI workflow will fail at conversion.
- **Website (`update-website.yml`):** Triggers on `release: types: [published]` — fires for every GoReleaser publication. Confirmed working for all alpha releases; no changes needed for v1.0.0.

**`release-meta.json`:** Already strips the leading `v` (`NEO4J_VERSION="${NEO4J_VERSION_TAG#v}"`). Works correctly for `v1.0.0` → `1.0.0`.

**Sequencing:** The workflow changes to `changie.yml` and `.goreleaser.yaml` must land on `main` before or at the same time as the Major changie entry is pushed to `main` — otherwise the release PR could be opened by the old workflow using the alpha logic.

## Acceptance Criteria

- [ ] `.goreleaser.yaml` `release.prerelease` is `false`.
- [ ] `changie.yml` contains no references to `alpha`, `prerelease`, or hardcoded `v0.1.0`.
- [ ] `changie.yml` uses `batch auto --project neo4j-cli`.
- [ ] A Major-kind changie entry exists in `.changes/unreleased/`.
- [x] `changie batch auto --project neo4j-cli --dry-run` resolves to `v1.0.0` when a Major entry is present — verified locally with changie v1.24.0.
- [ ] The changie workflow creates a PR titled "Release neo4j-cli v1.0.0". _(scope ends here — merging is a separate action)_

## Out of Scope

- Merging the v1.0.0 release PR — that is a deliberate human action after this work is complete.
- Post-release distribution verification: npm `latest` tag, PyPI `1.0.0` wheel, website update, Homebrew formula — all confirmed correct by workflow audit but verified only after the release PR is merged.
- Code changes to `release.yml`, `publish-npm.yml`, `publish-pypi.yml`, or `update-website.yml`.
- Pre-release workflow infrastructure for future alpha/beta/RC cycles.
- Bumping the Homebrew formula `version` field manually — GoReleaser handles that.
- Expanding `version-to-pep440.sh` to handle non-standard pre-release formats (e.g., `-dev`, `-preview`) — the strict format is a documented constraint, not a bug to fix here.

## Open Questions

None — all questions resolved.
