# PRD: npm Trusted Publishers (OIDC)

Linear: [CLI-30](https://linear.app/neo4j/issue/CLI-30/publish-to-npm-via-trusted-publishers-so-we-dont-need-to-use-personal). Branch: `oskar/cli-30-*`.

## Overview

Migrate `@neo4j-labs/cli` (+ 8 platform packages) from long-lived `NPM_TOKEN` authentication to npm Trusted Publishers (OIDC). Each publish becomes a short-lived, GitHub-issued, workflow-bound credential exchange. Eliminates the only npm-related repo secret, enables automatic provenance attestation, and removes the documented blocker on the auto-publish trigger.

## Goals

- Drop `NPM_TOKEN` from CI; npm authentication uses GitHub OIDC only.
- Automatic [provenance attestations](https://docs.npmjs.com/generating-provenance-statements) on every published package version.
- All nine `@neo4j-labs/*` packages claim their names on the registry so Trusted Publisher can be configured (npm has no pending-publisher flow).
- No regression: same `dist-tag` rules, same idempotent re-run behaviour in `publish.sh`, same `workflow_dispatch` recovery path.

## Non-Goals

- Enabling the `workflow_run` auto-trigger (stays commented out; separate follow-up PR).
- Promoting any prerelease `dist-tag` to `latest` (still a manual `npm dist-tag add`).
- Adding new platforms (e.g. 32-bit Linux ARM).
- Touching `aura-cli` distribution (not on npm).
- Migrating other secrets (`MACOS_*`, `TEAM_GRAPHQL_PERSONAL_ACCESS_TOKEN`).

## Requirements

### Functional Requirements

- REQ-F-001: `.github/workflows/publish-npm.yml` `permissions:` block grants `contents: read` and `id-token: write` (and nothing else).
- REQ-F-002: `.github/workflows/publish-npm.yml` does not reference `secrets.NPM_TOKEN`. The `Configure npm auth` step is removed.
- REQ-F-003: `.github/workflows/publish-npm.yml` includes an `actions/setup-node` step pinned to a v6 SHA (with `# v6` trailing comment, repo convention) before the publish step. Inputs: `node-version: '24'`, `registry-url: 'https://registry.npmjs.org'`. This is required so the runner has npm CLI ≥ 11.5.1 (Node 24 ships npm 11.x) and a registry-url-only `~/.npmrc`.
- REQ-F-004: `Publish to npm` step runs `bash distribution/npm/publish.sh` unchanged. `publish.sh` itself is not modified — it remains auth-agnostic (the contract is "caller has set up `~/.npmrc`"; `setup-node` now satisfies that contract instead of the manual write step).
- REQ-F-005: The commented-out `workflow_run` block at the top of the workflow includes a one-line note that re-enabling it requires re-adding `actions: read` to the `permissions:` block (cross-workflow `actions/download-artifact` requirement).
- REQ-F-006: Both `package.json` templates point at the actual repo. In `distribution/npm/cli/package.json.tmpl` and `distribution/npm/cli-platform/package.json.tmpl`:
  - `homepage`: `https://github.com/neo4j-labs/neo4j-cli`
  - `repository.url`: `git+https://github.com/neo4j-labs/neo4j-cli.git`
  - `bugs.url`: `https://github.com/neo4j-labs/neo4j-cli/issues`
- REQ-F-007: New `distribution/npm/bootstrap-stubs.sh`. One-time, local-run helper that:
  - Hardcodes `VERSION=0.0.0-bootstrap.1`.
  - Renders both templates for all 9 packages (8 platform rows from `distribution/platforms.tsv` + 1 wrapper).
  - Stubs every binary with a 1-byte placeholder (same trick `publish.sh --dry-run` uses).
  - Runs `npm publish --tag bootstrap --access public` for each package (8 platforms first, then wrapper — same ordering as `publish.sh`).
  - Idempotent: skips packages already on the registry via the same `already_published` check pattern as `publish.sh`.
  - Errors out if `npm whoami` is not authenticated against the registry.
- REQ-F-008: `RELEASING.md` is updated to remove `NPM_TOKEN`:
  - "Required secrets" table: drop the `NPM_TOKEN` row.
  - Step 4 description: replace "Writes `~/.npmrc` from the `NPM_TOKEN` secret" with wording that names Trusted Publishers + OIDC.
  - "Manual recovery" section: drop the "NPM_TOKEN was missing or expired and got rotated" recovery case.
- REQ-F-009: `distribution/npm/README.md` is updated:
  - Drop the "Open follow-ups" bullet about npm provenance attestation (it becomes implicit / always-on with this change).
  - Add a "Bootstrap (one-time)" subsection documenting `bootstrap-stubs.sh`, the rationale (no pending-publisher flow), the `bootstrap` dist-tag choice, and the post-bootstrap UI configuration steps.
- REQ-F-010: Optional Makefile target `npm-bootstrap` invoking `bootstrap-stubs.sh`, mirroring the discoverability pattern of `npm-publish-dry`.

### Non-Functional Requirements

- REQ-NF-001: No new long-lived secrets in CI. After this PR + the bootstrap + the npm-UI configuration, `secrets.NPM_TOKEN` is unreferenced and can be deleted from repo settings (decommissioning of the secret itself is a maintainer step, out of repo scope).
- REQ-NF-002: `bootstrap-stubs.sh` and the `npm-bootstrap` make target run without invoking GoReleaser, without copying any real binaries, and without touching the production `dist/` layout. They are isolated from the release path.
- REQ-NF-003: The OIDC migration must not change the user-facing install command (`npm i -g @neo4j-labs/cli`) or change the `dist-tag` ladder (`latest` / `alpha` / `beta` / `rc` / `next`). The `bootstrap` dist-tag is the only addition and must never be reachable via an unqualified install.
- REQ-NF-004: The change must continue to work cross-OS for local dev — `make npm-publish-dry` is the gate; no `bash`-only constructs that break on macOS sed (mirror `publish.sh`'s portable-sed style).

## Technical Considerations

**OIDC requirements (npm side).** npm CLI ≥ 11.5.1 and Node ≥ 22.14.0 are required to issue and exchange OIDC tokens. Pinning Node 24 in `actions/setup-node` gives headroom (Node 24 ships npm 11.x). Provenance attestation is generated automatically when (a) `id-token: write` is granted, (b) the workflow runs in GitHub Actions OIDC env, (c) the package's `repository.url` matches the source-repo claim. (b) is true on `ubuntu-latest`; (c) is what REQ-F-006 fixes.

**No pending-publisher flow.** Confirmed against npm docs and community write-ups: a package must exist on the registry before its Trusted Publisher binding can be configured. With nine brand-new package names, the only way forward is to claim each name with a low-version stub, then configure Trusted Publisher for each, then ship the first real release. The `bootstrap` dist-tag + sub-`0.0.1` version ensures unqualified installs never resolve to a stub.

**`workflow_run` and OIDC.** The `workflow_run` auto-trigger is intentionally kept disabled. When it is later re-enabled, the OIDC token issued for that run will reflect the *running* workflow (`publish-npm.yml`), not the upstream `release.yml` — so Trusted Publisher matching against `publish-npm.yml` will succeed. The note in REQ-F-005 documents the only operational requirement (re-add `actions: read` for cross-workflow artifact download).

**`publish.sh` is unchanged.** Its contract — "caller configures `~/.npmrc`" — is preserved. The change is *who* writes `~/.npmrc`: was a manual `printf` step, now `actions/setup-node` (registry-url only, no token). The script's idempotent `already_published` check, dist-tag derivation, and platforms-then-wrapper ordering all carry through.

**`repository.url` mismatch is pre-existing.** Templates point at `github.com/neo4j/cli`; actual repo is `github.com/neo4j-labs/neo4j-cli`. We fix it as part of this work because provenance refuses to attest a mismatch — but it would have been wrong-but-harmless under token auth.

**Bootstrap as a script, not a one-shot manual command.** Keeping `bootstrap-stubs.sh` in the tree (vs. ad-hoc maintainer commands) gives us a re-usable path for future scope additions: when a 10th platform is added, claiming the new package name follows the same script.

## Acceptance Criteria

- [ ] `.github/workflows/publish-npm.yml` has `id-token: write` and no `NPM_TOKEN` reference.
- [ ] `.github/workflows/publish-npm.yml` has `actions: read` removed; commented `workflow_run` block carries the re-enable note.
- [ ] `actions/setup-node@<sha> # v6` step is present, with `node-version: '24'` and `registry-url: 'https://registry.npmjs.org'`.
- [ ] Both `package.json` templates carry `github.com/neo4j-labs/neo4j-cli` URLs in `homepage`, `repository.url`, `bugs.url`.
- [ ] `distribution/npm/bootstrap-stubs.sh` exists, executable, idempotent, errors on non-authenticated npm sessions, publishes 9 stubs under `--tag bootstrap`.
- [ ] `make npm-publish-dry` still passes.
- [ ] `make fmt-check && make lint` still pass.
- [ ] `RELEASING.md` no longer lists `NPM_TOKEN`; step 4 description is rewritten; recovery section is updated.
- [ ] `distribution/npm/README.md` describes the bootstrap flow; provenance follow-up bullet is gone.
- [ ] (External, gated by maintainer) Bootstrap script run + 9 Trusted Publisher configurations done in npm UI before merging the workflow change.
- [ ] (External, gated by maintainer) Manual `workflow_dispatch` of `publish-npm.yml` against version `0.0.0-bootstrap.1` exits 0, logs show `[skip]` per package and zero auth errors.
- [ ] First real release after merge: `npm view @neo4j-labs/cli@<v> --json | jq .dist.attestations` returns provenance metadata.

## Out of Scope

- Enabling the `workflow_run` auto-trigger.
- Deleting the `NPM_TOKEN` repo secret (maintainer task, after merge).
- Migrating any other workflow's secret usage.
- Promoting alpha → stable.
- Adding 32-bit Linux ARM platform.
- Changes to `aura-cli` distribution.

## Open Questions

None.
