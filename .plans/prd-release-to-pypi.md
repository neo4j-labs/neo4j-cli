# PRD: Release neo4j-cli to PyPI

## Overview

Wire `publish-pypi.yml` into the existing release pipeline so that every successful `release.yml` run automatically publishes a PEP 440-compliant Python wheel for `neo4j-cli` to PyPI. The wheel must wrap the same GoReleaser-built binaries that ship to GitHub Releases, npm, and Homebrew, and the wheel version must be a valid PEP 440 string derived from the Git tag (e.g. `v0.1.0-alpha.6` → `0.1.0a6`).

## Goals

- Publish neo4j-cli to PyPI on every successful release, including pre-releases (alpha / beta / rc).
- Guarantee binary parity between the PyPI wheel and the GitHub Release / npm / Homebrew artifacts (single source of truth = GoReleaser dist).
- Use a PEP 440-compliant version on the wheel without changing the version reported by `neo4j-cli --version` (which stays aligned with the GoReleaser tag).
- Match the publication pattern of [publish-npm.yml](.github/workflows/publish-npm.yml) so future maintainers learn one cross-workflow shape, not two.
- Provide a manual recovery path (`workflow_dispatch`) for re-publishing after a transient PyPI failure without re-running the full release.

## Non-Goals

- No change to GoReleaser config, the GitHub Release event, or the npm/Homebrew publication paths.
- No change to the Go ldflags `Version` value: `neo4j-cli --version` continues to report the original GoReleaser tag (e.g. `0.1.0-alpha.6`), not the PEP 440 form.
- No support for PEP 440 dev / post / local-version segments — current and foreseeable Git tags are `vX.Y.Z` or `vX.Y.Z-(alpha|beta|rc).N` only.
- No PyPI Trusted Publishers (OIDC) migration in this PRD — the workflow continues to authenticate via `secrets.PYPI_API_TOKEN`. (Can be a follow-up.)
- No change to `go-to-wheel` itself; we configure it via flags only.
- No backfill of historical alpha tags to PyPI.

## Requirements

### Functional Requirements

- REQ-F-001: `publish-pypi.yml` MUST trigger automatically via `workflow_run` after `release.yml` completes with `conclusion == 'success'`. The existing `release: types: [released]` trigger MUST be removed.
- REQ-F-002: `publish-pypi.yml` MUST also support `workflow_dispatch` with a `version` string input (no leading `v`, e.g. `0.1.0-alpha.6`) for manual recovery, mirroring [publish-npm.yml](.github/workflows/publish-npm.yml).
- REQ-F-003: The auto path MUST consume the `release-meta` and `dist` artifacts already uploaded by [release.yml](.github/workflows/release.yml), gating on `release-meta.json`'s `include_neo4j == true` (defensive — currently always true since aura-cli was removed).
- REQ-F-004: The manual path MUST rebuild the GoReleaser `dist/` layout from existing GitHub Release archives via `gh release download`, the same approach publish-npm.yml uses.
- REQ-F-005: The workflow MUST normalise the source version to PEP 440 before passing it to `go-to-wheel --version`, using exactly this contract:
  - `X.Y.Z` → `X.Y.Z`
  - `X.Y.Z-alpha.N` → `X.Y.ZaN`
  - `X.Y.Z-beta.N` → `X.Y.ZbN`
  - `X.Y.Z-rc.N` → `X.Y.ZrcN`
  - Any other shape MUST cause the workflow to fail before publishing.
- REQ-F-006: The Go ldflags `main.Version` value MUST remain the original GoReleaser tag (non-PEP 440 form). Only the wheel package version is normalised. This applies to both the auto and manual paths.
- REQ-F-007: The PyPI publish step MUST upload all six platform wheels produced by `go-to-wheel`: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64, windows-amd64, windows-arm64.
- REQ-F-008: The smoke-test job MUST continue to verify that the linux-amd64 wheel's bundled binary reports the original (non-PEP 440) version when invoked with `--version`, since that is what the binary's ldflags actually contain.
- REQ-F-009: Because we now reuse the GoReleaser binary, the `--set-version-var main.Version` flag passed to `go-to-wheel` MUST be removed (or set to the original tag) and the inline `go build` of a "reference" linux-amd64 binary plus its `sha256sum` parity check MUST be removed — parity is now guaranteed by reusing the same GoReleaser artifacts as npm/Homebrew. The `cp neo4j-cli/main.go main.go` shim MUST also be removed if `go-to-wheel` no longer needs to build from source.
- REQ-F-010: The `dry_run` `workflow_dispatch` input MUST be preserved and continue to skip only the publish step (build + smoke test still run).
- REQ-F-011: The workflow MUST keep the `permissions: contents: read` block, and MUST add `actions: read` to the publish job (required by the cross-workflow `actions/download-artifact` calls), matching publish-npm.yml's permissions.
- REQ-F-012: A new directory `distribution/pypi/` MUST be created containing a `README.md` that documents the PyPI distribution channel for maintainers, mirroring the shape and tone of [distribution/homebrew/README.md](distribution/homebrew/README.md) and [distribution/npm/README.md](distribution/npm/README.md). Use the lowercase `pypi/` directory name (PyPI is the proper noun, but filesystem paths in this repo are lowercase — see `npm/`, `homebrew/`). The README MUST cover at minimum:
  - What this directory does (documentation-only, no runtime code — actual publication is handled by `.github/workflows/publish-pypi.yml` plus `go-to-wheel`).
  - End-user install command (`pip install neo4j-cli`, plus `pip install --pre neo4j-cli` for alpha/beta/rc).
  - PyPI project URL and ownership notes.
  - Release cadence — stable + prerelease (unlike Homebrew, PyPI ships every release; PEP 440 `aN`/`bN`/`rcN` are auto-classified as pre-releases).
  - PEP 440 version mapping table (`vX.Y.Z-alpha.N` → `X.Y.ZaN`, etc.).
  - The six platform wheels published per release and what their filename suffixes look like.
  - Local dry-run instructions (`workflow_dispatch` with `dry_run: true`).
  - Manual recovery procedure (`workflow_dispatch` with a `version` input — same pattern as publish-npm.yml).
  - Auth prerequisites (`PYPI_API_TOKEN` repo secret today; OIDC trusted publishing as a flagged follow-up).
  - "See also" links to `RELEASING.md`, `publish-pypi.yml`, the npm README, and the homebrew README.

### Non-Functional Requirements

- REQ-NF-001: All actions MUST stay SHA-pinned with a trailing `# v<major>` comment, matching the repo convention (see [release.yml](.github/workflows/release.yml) and [publish-npm.yml](.github/workflows/publish-npm.yml)).
- REQ-NF-002: The PEP 440 transformation MUST live in a single shell helper inside the workflow (or `.github/scripts/`) — not duplicated between the auto and manual paths.
- REQ-NF-003: The workflow MUST fail loudly (non-zero exit, clear error) on: missing release-meta.json, malformed release-meta.json, version that does not match the PEP 440 contract, or a missing dist artifact.
- REQ-NF-004: Existing `requirements-build.txt` (used for `go-to-wheel` and friends) MUST continue to be installed via `pip install --require-hashes` for supply-chain safety.
- REQ-NF-005: No long-lived secrets beyond `PYPI_API_TOKEN` and `GITHUB_TOKEN` are introduced. (OIDC migration is non-goal.)

## Technical Considerations

**Trigger flow.** [release.yml](.github/workflows/release.yml) already uploads `dist/` and `release-meta.json` artifacts whose lifecycle is shared with [publish-npm.yml](.github/workflows/publish-npm.yml). Reusing them costs nothing — the artifacts already exist for ~7 days post-release — and keeps the npm/PyPI/Homebrew triple in lockstep on identical binaries.

**`workflow_run` gotchas.** Per AGENTS.md, several constraints must be respected:
- `workflows: ["release"]` matches the upstream `name:` field (lowercase) — same string already used by publish-npm.yml.
- Cross-workflow `actions/download-artifact` requires `github-token` AND `run-id: ${{ github.event.workflow_run.id }}`.
- Job-level `if:` must gate on `github.event.workflow_run.conclusion == 'success'`; the per-step `include_neo4j` gate happens after parsing release-meta.json with `jq`.
- `workflow_run` events do NOT have `inputs.*`; `workflow_dispatch` events do not have `github.event.workflow_run.*`. Use the ternary pattern `${{ github.event_name == 'workflow_dispatch' && inputs.x || steps.<auto>.outputs.x }}`.

**Version source of truth.**
- Auto path: read version from `release-meta.json` (`steps.meta.outputs.version`).
- Manual path: read from `inputs.version`.
- Both feed into the same PEP 440 normaliser step.
- `release-meta.json` already strips the leading `v`, so the regex used by publish-npm.yml (`^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$`) is the right pre-validation gate; PEP 440 normalisation is a second pass on top.

**PEP 440 normaliser shape.** A small bash function is enough — no Python dependency just for this:

```bash
to_pep440() {
  local v="${1#v}"
  if [[ "$v" =~ ^([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
    echo "${BASH_REMATCH[1]}"
  elif [[ "$v" =~ ^([0-9]+\.[0-9]+\.[0-9]+)-(alpha|beta|rc)\.([0-9]+)$ ]]; then
    local base="${BASH_REMATCH[1]}" pre="${BASH_REMATCH[2]}" n="${BASH_REMATCH[3]}"
    case "$pre" in
      alpha) echo "${base}a${n}" ;;
      beta)  echo "${base}b${n}" ;;
      rc)    echo "${base}rc${n}" ;;
    esac
  else
    echo "ERROR: version '$v' does not match PEP 440 contract" >&2
    return 1
  fi
}
```

**Wheel binary source.** The GoReleaser dist artifact contains per-platform archives (`neo4j-cli_<version>_<os>_<arch>.tar.gz` / `.zip`) plus raw build directories. Two options for `go-to-wheel`:
1. Pre-extract archives (as publish-npm.yml does) and pass the per-platform binary directories.
2. Pass the raw build directories that GoReleaser already produces inside `dist/`.

Implementation should pick whichever requires the smallest delta against `go-to-wheel`'s existing CLI surface; if `go-to-wheel` only knows how to build from Go source, we may need to invoke it once per platform with a pre-extracted binary. This is the main implementation unknown and SHOULD be confirmed during task breakdown.

**`name=` argument to `go-to-wheel`.** Stays `neo4j-cli`; this becomes the PyPI distribution name. The user-facing PyPI project name MUST be claimed before the first publish.

**Smoke test.** [smoke-test-wheel.sh](.github/scripts/smoke-test-wheel.sh) currently greps the binary's `--version` output for the expected version. Since the wheel version (PEP 440) and the binary's reported version (Go ldflags) now diverge, the smoke test must be passed the *original* (non-PEP 440) version. The CI job already passes `$VERSION`; we just need to make sure that variable carries the original tag, not the PEP 440 form.

**Trusted Publishers / OIDC.** Out of scope for this PRD but worth flagging: PyPI now supports OIDC trusted publishing, which would remove `PYPI_API_TOKEN` from the secret store and bring PyPI auth into line with npm. Track as a follow-up.

## Acceptance Criteria

- [ ] Merging a release PR that publishes `vX.Y.Z-alpha.N` causes [publish-pypi.yml](.github/workflows/publish-pypi.yml) to fire automatically via `workflow_run`, with no manual intervention.
- [ ] The published wheel filename contains the PEP 440 form of the version (e.g. `neo4j_cli-0.1.0a6-py3-none-manylinux_2_17_x86_64.whl` — the PEP 600 form that `go-to-wheel 0.2` emits; equivalent to the legacy `manylinux2014_x86_64`).
- [ ] `pip install neo4j-cli==0.1.0a6` (or the corresponding stable / beta / rc form) succeeds against PyPI from a clean venv on linux-amd64, linux-arm64, darwin-amd64, darwin-arm64, windows-amd64, windows-arm64.
- [ ] The installed wheel ships a binary whose `--version` output matches the original GoReleaser tag (e.g. `0.1.0-alpha.6`), not the PEP 440 form.
- [ ] `sha256sum` of the binary inside the linux-amd64 wheel matches the binary inside the corresponding GitHub Release archive `neo4j-cli_<version>_linux_amd64.tar.gz`.
- [ ] Manually invoking `workflow_dispatch` with `version: 0.1.0-alpha.6` against an existing GitHub Release republishes the same six wheels successfully (idempotent / recovery path).
- [ ] Manually invoking `workflow_dispatch` with `dry_run: true` runs the build + smoke-test jobs but skips the publish job.
- [ ] An invalid tag (e.g. `v0.1.0-pre.6` or `v0.1.0.alpha.6`) causes the workflow to fail at the version-normalisation step before any wheel is built or uploaded.
- [ ] All actions in the new workflow are SHA-pinned with `# v<major>` trailing comments and pass any existing repo-level action-pinning checks.
- [ ] Workflow runs are visible in the Actions tab and surface a clear PyPI URL for the published version on success.
- [ ] `distribution/pypi/README.md` exists, follows the structure of [distribution/homebrew/README.md](distribution/homebrew/README.md) and `distribution/npm/README.md`, documents the install command, version mapping, recovery path, and auth prerequisites, and is linked from the appropriate "See also" sections in the sibling READMEs.

## Out of Scope

- Migrating PyPI authentication from `PYPI_API_TOKEN` to OIDC Trusted Publishers.
- Backfilling historical alpha versions (`v0.0.1-alpha`, `v0.0.1-alpha.0`, `v0.1.0-alpha.1` … `v0.1.0-alpha.5`) to PyPI.
- Changing the version reported by `neo4j-cli --version` to the PEP 440 form.
- Adding a Python source distribution (`.tar.gz` sdist) — wheels only, six platforms.
- Publishing to TestPyPI as part of the standard release flow (a `dry_run` mode is sufficient).
- Adding a `pyproject.toml` to the repo root (the wheel metadata is generated by `go-to-wheel` from CLI flags).
- Changes to `release.yml`, `publish-npm.yml`, GoReleaser config, or Homebrew tap publication.

## Open Questions

- **PyPI project ownership.** Has the `neo4j-cli` (or alternative) name been reserved on PyPI under an account this repo can use? If not, who claims it and when?
- **`go-to-wheel` input shape.** Does the current `go-to-wheel` invocation work cleanly when fed pre-built per-platform binaries from GoReleaser's `dist/`, or does it always rebuild from Go source? This determines the exact build step. Confirm during task breakdown.
- **Failure visibility.** Should a PyPI-publish failure block the GitHub Release from being marked "released," or surface only as a workflow failure (current default)? Recommended: keep release independent — PyPI is a downstream channel and a failure there should not unwind the GitHub Release / npm / Homebrew publication.
- **Pre-release classification on PyPI.** PEP 440 `aN` / `bN` / `rcN` are pre-releases by spec, so `pip install neo4j-cli` (without `--pre`) will skip them — this is correct behaviour and probably desired, but worth confirming with the team.
