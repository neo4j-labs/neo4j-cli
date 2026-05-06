# PRD: Homebrew Distribution

Linear: [CLI-11](https://linear.app/neo4j/issue/CLI-11/add-brew-installer) (parent [CLI-22](https://linear.app/neo4j/issue/CLI-22/add-to-distribution-channels)).

## Overview

Ship `neo4j-cli` (the super-CLI bundling `aura-cli` under `neo4j-cli aura`) as a Homebrew formula so macOS / Linux users can install with `brew install neo4j-labs/tap/neo4j-cli`. Today the only install paths are GitHub Releases archives and npm `@neo4j-labs/cli`.

The formula is auto-generated and pushed to a public tap repo by GoReleaser as part of every stable release. No new workflow, no parallel publish script — wires into the existing `release.yml` GoReleaser step.

## Goals

- One-command install on macOS via `brew install neo4j-labs/tap/neo4j-cli`.
- Zero manual steps in the release loop — formula updates ride the existing changelog → release-PR → GoReleaser pipeline.
- Stable users never see prereleases via brew (alphas remain opt-in via npm `@alpha` / GitHub Releases).
- Reuse the already-signed / already-notarized release archives — no recompile on the user's machine, no extra signing path.

## Non-Goals

- Submitting `neo4j-cli` to `homebrew-core`. Tap-only for this iteration; revisit once stable releases stabilise.
- Shipping `aura-cli` as its own brew formula. The super-CLI is the only entry point on brew, mirroring the npm decision.
- Bottle (prebuilt-and-uploaded-to-bintray-style) generation. The formula points users at our GitHub Release archives directly via `url` + `sha256`.
- Opt-in alpha channel on brew (e.g. `neo4j-cli@alpha`). Not in scope now; alphas continue via npm.

## Requirements

### Functional Requirements

- REQ-F-001: GoReleaser writes a Homebrew formula `Formula/neo4j-cli.rb` and pushes it to `neo4j-labs/homebrew-tap` on every **stable** `neo4j-cli` release.
- REQ-F-002: Prerelease versions (any tag containing `-alpha`/`-beta`/`-rc`) MUST NOT push the formula. Achieved via `skip_upload: auto`.
- REQ-F-003: The formula references the `neo4j-cli` super-CLI archives only (`ids: [neo4j-cli-archive]`); the standalone `aura-cli` archives are excluded.
- REQ-F-004: The formula installs the `neo4j-cli` binary onto `PATH` (`bin.install "neo4j-cli"`).
- REQ-F-005: The formula carries a `test` block that runs `neo4j-cli --version` and asserts it matches the formula version, so `brew audit --strict --new` passes.
- REQ-F-006: The formula declares `homepage` (`https://github.com/neo4j/cli`), `desc` ("Command-line interface for Neo4j"), and `license` ("Apache-2.0").
- REQ-F-007: The formula supports macOS amd64 + arm64 and Linux amd64 + arm64 — i.e. all four `*nix` archives GoReleaser already produces for `neo4j-cli`. (Windows is not a brew target.)
- REQ-F-008: Push to the tap is authenticated by a short-lived GitHub App installation token (no PAT). The token is minted per-run by `actions/create-github-app-token` and exposed to GoReleaser as `HOMEBREW_TAP_GITHUB_TOKEN`.
- REQ-F-009: `RELEASING.md` and `distribution/npm/README.md` are updated to reflect that Homebrew is shipping (no longer "future"); a new `distribution/homebrew/README.md` documents tap location, user install command, prerelease skip, and manual recovery (hand-edit + push, next release overwrites).
- REQ-F-010: A `neo4j-cli`-only changelog entry (`Minor`) is added covering the new install path.

### Non-Functional Requirements

- REQ-NF-001: No long-lived secret in CI — the GitHub App pattern issues an installation token scoped to `neo4j-labs/homebrew-tap` only, valid for the workflow run.
- REQ-NF-002: Local dry-run path: `make snapshot` (existing target) must produce the formula at `dist/homebrew/Formula/neo4j-cli.rb` for inspection without touching the tap.
- REQ-NF-003: Repo conventions are preserved: `actions/create-github-app-token` SHA-pinned with a `# v<major>` trailing comment (Renovate handles bumps), per the repo's existing action pinning style.
- REQ-NF-004: `make test`, `make fmt-check`, `make lint` continue to pass. (No Go code changes are expected, but gates run regardless.)

## Technical Considerations

**GoReleaser config (`brews:` block).** GoReleaser natively generates and pushes Homebrew formulas. The block is appended to `.goreleaser.yaml`:

```yaml
brews:
  - name: neo4j-cli
    ids: [neo4j-cli-archive]
    repository:
      owner: neo4j-labs
      name: homebrew-tap
      branch: main
      token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}"
    directory: Formula
    commit_author:
      name: neo4j-cli-bot
      email: noreply@neo4j.com
    commit_msg_template: "Brew formula update for {{ .ProjectName }} version {{ .Tag }}"
    homepage: "https://github.com/neo4j/cli"
    description: "Command-line interface for Neo4j"
    license: "Apache-2.0"
    skip_upload: auto
    install: |
      bin.install "neo4j-cli"
    test: |
      assert_match version.to_s, shell_output("#{bin}/neo4j-cli --version")
```

`skip_upload: auto` is the documented GoReleaser knob for "skip if version is a prerelease" (any tag with a `-` suffix per semver).

**Tap repo.** `neo4j-labs/homebrew-tap` is created externally (out-of-band by the user). Default branch `main`. GoReleaser commits `Formula/neo4j-cli.rb` to it on each stable release. The repo can host other neo4j-labs formulas later without restructuring.

**Auth (GitHub App).**

- A GitHub App installed on `neo4j-labs` with `contents: write` scoped to `homebrew-tap`.
- Two new secrets on `neo4j/cli`: `HOMEBREW_TAP_APP_ID`, `HOMEBREW_TAP_APP_PRIVATE_KEY`.
- New step in `release.yml` (before "Run GoReleaser") mints an installation token via `actions/create-github-app-token`. Token is exposed as `HOMEBREW_TAP_GITHUB_TOKEN` env on the GoReleaser step.
- No long-lived PAT.

**Why no parallel `distribution/homebrew/publish.sh`.** The npm distribution has its own `publish.sh` because it fans out across 8 platform-specific packages plus a wrapper. Homebrew is single-formula and GoReleaser handles it natively — a publish.sh would just shell out to GoReleaser. The reserved slot at `distribution/homebrew/` instead holds documentation only.

**Prerelease skip mechanics.** GoReleaser parses the version (e.g. `v0.2.0-alpha.3`) and short-circuits the brew push when `skip_upload: auto` is set and the version has a prerelease suffix. The rest of the release (GitHub Release, npm publish via downstream workflow) proceeds as normal — only the brew formula is held back. Stable tags (e.g. `v0.2.0`) push.

**Files touched.**

- `.goreleaser.yaml` — append `brews:` block.
- `.github/workflows/release.yml` — add `Mint Homebrew tap token` step + `HOMEBREW_TAP_GITHUB_TOKEN` on the GoReleaser step's env.
- `distribution/homebrew/README.md` — new (channel docs).
- `RELEASING.md` — drop "future" framing for Homebrew, add release-table row, add the two new secrets to the secrets table.
- `distribution/npm/README.md` — update the Homebrew callout (lines 228–231 currently say "future").
- `.changes/unreleased/neo4j-cli-Minor-<timestamp>.yaml` — changelog entry, project `neo4j-cli` only.

**Risks.**

- First stable release after merge will be the first time the App + tap interaction is exercised end-to-end. Mitigated by `make snapshot` confirming formula generation locally pre-merge.
- Tap repo bootstrap (creating it empty) is a manual step. If the App lacks write perms, the GoReleaser step fails the release — caught early by the first stable release attempt.
- `actions/create-github-app-token` has many releases; needs SHA-pin per repo convention.

## Acceptance Criteria

- [ ] `.goreleaser.yaml` has a `brews:` block matching the spec in Technical Considerations; `make snapshot` produces `dist/homebrew/Formula/neo4j-cli.rb` with populated `homepage`, `desc`, `license`, `url`, `sha256`, `bin.install "neo4j-cli"`, and a `test` block.
- [ ] `brew audit --strict --formula dist/homebrew/Formula/neo4j-cli.rb` passes locally (or its non-fatal warnings are documented).
- [ ] `release.yml` mints a GitHub App token in a new step (SHA-pinned action) and passes `HOMEBREW_TAP_GITHUB_TOKEN` to the GoReleaser step env.
- [ ] `distribution/homebrew/README.md` exists and covers: tap repo, install command, stable-only behaviour, local dry-run, manual recovery.
- [ ] `RELEASING.md` no longer marks Homebrew as "future"; the release table and secrets table both include the Homebrew row.
- [ ] `distribution/npm/README.md` Homebrew callout points at the new homebrew README.
- [ ] A `neo4j-cli`-only `Minor` changelog entry is staged in `.changes/unreleased/`.
- [ ] First stable release after merge writes a commit to `neo4j-labs/homebrew-tap` and `brew tap neo4j-labs/tap && brew install neo4j-cli && neo4j-cli --version` succeeds on macOS.
- [ ] An alpha release on `main` does NOT push to the tap (verified by no new commit on `homebrew-tap`).
- [ ] `make test`, `make fmt-check`, `make lint` all pass.

## Out of Scope

- Submission to `homebrew-core`.
- A separate `aura-cli` formula.
- An opt-in alpha brew channel (`neo4j-cli@alpha`).
- A `distribution/homebrew/publish.sh` script (GoReleaser handles it natively).
- Linuxbrew-specific verification beyond what GoReleaser auto-generates for the linux archives.
- Bottle uploads (binary-attached-to-formula pattern).

## Open Questions

- None blocking implementation. Tap-repo creation, App creation, and secret population are user-side prerequisites tracked in `distribution/homebrew/README.md` once written.
