# PRD: npm distribution of `neo4j-cli` as `@neo4j-labs/cli`

## Overview

Distribute the `neo4j-cli` super-CLI on npm so users can install it with `npm i -g @neo4j-labs/cli` and get the right prebuilt binary for their OS/arch automatically. Today the CLI ships only as raw archives on GitHub Releases — users must download, unzip, and place the binary on PATH manually. This is the first non-GitHub distribution channel; the layout is designed so pip and homebrew can be added later under the same `distribution/` tree without re-architecting.

Background plan (full design detail): `/Users/oskarhane/.claude/plans/focus-only-on-npm-eager-unicorn.md`.

## Goals

1. `npm i -g @neo4j-labs/cli` installs `neo4j-cli` on PATH on macOS (arm64, x64), Linux (arm64, ia32, x64), and Windows (arm64, ia32, x64).
2. Installation does not run any postinstall script — works under `npm i --ignore-scripts`, in pnpm/Bun strict environments, in airgapped CI behind locked-down registries.
3. `npm i @neo4j-labs/cli` (no qualifier) only ever resolves to a stable release; alpha/beta/rc cycles are opt-in via `npm i @neo4j-labs/cli@alpha` (or `@beta`, `@rc`).
4. npm publishing runs automatically after each successful neo4j-cli release, AND can be re-run manually from the GitHub Actions UI for failure recovery, without needing to re-run GoReleaser or bump the version.
5. The release pipeline tolerates partial-publish failures: a same-version retry resumes from where it left off (idempotent at the per-package level).
6. Aura-cli-only release cycles do NOT trigger an npm publish.
7. The directory layout (`distribution/npm/`, `distribution/pypi/`, `distribution/homebrew/`, shared `distribution/platforms.tsv`) keeps future channels additive — adding pip means adding `distribution/pypi/publish.sh` + a sibling workflow, no refactor of the npm pieces.

## Non-Goals

- pip distribution (PyPI) — placeholder slot reserved at `distribution/pypi/`, not implemented.
- Homebrew distribution — placeholder slot reserved at `distribution/homebrew/`, not implemented. Future implementation likely uses GoReleaser's built-in `brews:` config rather than custom scripts.
- npm provenance attestation (sigstore via GH OIDC). Future toggle: add `--provenance` to `npm publish` + `id-token: write` permission to the workflow.
- Shipping `aura-cli` on npm. Only the super-CLI `neo4j-cli` ships in this scope.
- Linux 32-bit ARM (`linux-arm`). Not built by GoReleaser today — out of scope.
- Custom `aura-cli` npm scope (`@neo4j/aura-cli`). Out of scope.
- Auto-promotion of prerelease tags to `latest`. Promoting a beta to stable is a manual `npm dist-tag add` operation, not part of this automation.

## Requirements

### Functional Requirements

**Package shape**

- REQ-F-001: One wrapper npm package `@neo4j-labs/cli` containing only a JS bin shim and `optionalDependencies` listing all 8 platform packages pinned to the same version.
- REQ-F-002: Eight platform packages — `@neo4j-labs/cli-{darwin-arm64, darwin-x64, linux-arm64, linux-ia32, linux-x64, win32-arm64, win32-ia32, win32-x64}` — each containing exactly one binary in `bin/neo4j-cli` (`bin/neo4j-cli.exe` on win32) and a `package.json` with `os` + `cpu` constraints matching the platform.
- REQ-F-003: Wrapper `package.json` declares `bin: { "neo4j-cli": "bin/neo4j-cli.js" }`. Platform packages declare `bin: { "neo4j-cli": "bin/neo4j-cli[.exe]" }`. License: `Apache-2.0`. `engines.node: ">=18"`.

**Wrapper bin shim (`distribution/npm/cli/bin/neo4j-cli.js`)**

- REQ-F-004: Resolves the platform binary via `require.resolve('@neo4j-labs/cli-' + process.platform + '-' + process.arch + '/bin/neo4j-cli' + (process.platform === 'win32' ? '.exe' : ''))`.
- REQ-F-005: Execs the binary via `child_process.spawnSync(binary, process.argv.slice(2), { stdio: 'inherit' })` so signals (SIGINT etc.) and TTY behavior pass through cleanly.
- REQ-F-006: Propagates the child exit code (`process.exit(result.status ?? 1)`) so shell `&&`-chains work.
- REQ-F-007: When `require.resolve` fails (unsupported platform OR `--omit=optional` install), prints a clear error naming the platform/arch detected, the supported list, and a hint about `--no-optional` / `--omit=optional`. Exits non-zero.

**Platform mapping**

- REQ-F-008: A shared platform table at `distribution/platforms.tsv` lists the 8 rows: `(goreleaser_dirname_template, npm_os, npm_cpu, bin_filename)`. `distribution/npm/publish.sh` and any future `distribution/pypi/publish.sh` source this single file. Adding a new platform = one row added; no script edits.

**Publish script (`distribution/npm/publish.sh`)**

- REQ-F-009: Reads version from `GORELEASER_CURRENT_TAG`, strips leading `v`. Errors if unset.
- REQ-F-010: Renders `package.json` files from `*.tmpl` templates via `sed` substitution (no Node toolchain required at publish time). Templates kept as `.tmpl` so editors don't try to validate them as JSON.
- REQ-F-011: Publishes the 8 platform packages first, then the wrapper. Wrapper-last ordering is mandatory — wrapper's `optionalDependencies` only resolve if the platform pkgs already exist on the registry.
- REQ-F-012: Idempotency: before each `npm publish`, runs `npm view <name>@<version> version 2>/dev/null`; if it returns non-empty, prints `[skip] <name>@<version> already published` and continues. Same-version retries are no-ops at per-package granularity.
- REQ-F-013: Pre-release dist-tag selection from version string:
  | version pattern              | `--tag` value             |
  |------------------------------|---------------------------|
  | `X.Y.Z`                      | (omit flag → `latest`)    |
  | `X.Y.Z-alpha*`               | `alpha`                   |
  | `X.Y.Z-beta*`                | `beta`                    |
  | `X.Y.Z-rc*`                  | `rc`                      |
  | anything else with `-`       | `next`                    |
  Tag applied identically to all 9 packages in a publish run. Implementation: `case "$VERSION" in *-alpha*) TAG=alpha ;; *-beta*) TAG=beta ;; *-rc*) TAG=rc ;; *-*) TAG=next ;; *) TAG= ;; esac`.
- REQ-F-014: Supports `--dry-run` flag that passes `--dry-run` through to `npm publish` without writing to the registry.
- REQ-F-015: Uses `set -euo pipefail`. Header comment documents idempotency, ordering, and recovery flow prominently.
- REQ-F-016: Authenticates via `~/.npmrc` containing `//registry.npmjs.org/:_authToken=${NPM_TOKEN}` with `--access public` on every publish (required for first publish of scoped packages).

**CI workflow (`.github/workflows/publish-npm.yml`)**

- REQ-F-017: Two triggers: `workflow_run` (after `release.yml` success) AND `workflow_dispatch` (manual, with required `version` input — string, no leading `v`).
- REQ-F-018: `workflow_run` path skips publish entirely when `include_neo4j != true` (aura-cli-only release cycle) or when `workflow_run.conclusion != 'success'`.
- REQ-F-019: `workflow_run` path consumes a `dist/` artifact + `release-meta.json` artifact (`{ version, include_neo4j }`) uploaded by `release.yml`.
- REQ-F-020: `workflow_dispatch` path downloads existing GH release assets via `gh release download v${VERSION} --pattern 'neo4j-cli_*' --dir dist-archives/`, then extracts each `.tar.gz` / `.zip` into `dist/<archive_basename>/` to recreate the GoReleaser layout the publish script expects. Does NOT re-invoke GoReleaser.
- REQ-F-021: Configures `~/.npmrc` from `secrets.NPM_TOKEN`, then runs `GORELEASER_CURRENT_TAG=v${VERSION} distribution/npm/publish.sh`.

**Existing `release.yml` modifications**

- REQ-F-022: Surface `include_neo4j` (already computed at `release.yml:53-57`) as a job output.
- REQ-F-023: New "upload artifacts" step at end of release job: uploads `dist/` and a `release-meta.json` file containing `{ version, include_neo4j }`.

**Repo plumbing**

- REQ-F-024: `Makefile` gains a `npm-publish-dry` target that runs `distribution/npm/publish.sh --dry-run` against the current `make snapshot` output.
- REQ-F-025: `.gitattributes` extended with `distribution/**/*.{json,tmpl,js,sh} text eol=lf` so Windows checkouts don't introduce CRLF drift in committed templates/scripts.
- REQ-F-026: `README.md` install section updated to include `npm i -g @neo4j-labs/cli`.
- REQ-F-027: Changelog entry added via `make changelog --projects neo4j-cli` (Minor, body: "neo4j-cli is now installable via `npm i -g @neo4j-labs/cli`"). Aura-cli does NOT get a changelog entry — aura-cli is not on npm.
- REQ-F-028: Maintainer-facing `distribution/npm/README.md` documents: design rationale (multi-package vs postinstall), package shape diagram, install resolution flow, dist-tag channels, local dev/test flow, release flow (auto + manual), failure recovery, how to add a new platform, future channels. ~200 lines max, links to `.goreleaser.yaml` and `release.yml` rather than duplicating.
- REQ-F-029: User-facing `distribution/npm/cli/README.md` ships inside the `@neo4j-labs/cli` tarball — short install + usage doc.

### Non-Functional Requirements

- REQ-NF-001: Publish script must run on Ubuntu (GitHub Actions default). No macOS-specific shell features. POSIX `sh`-compatible or `bash` features available on `ubuntu-latest`.
- REQ-NF-002: No new build dependencies introduced into the Go toolchain. The publish path uses only `bash`, `sed`, `tar`, `unzip`, `gh`, and `npm` — all present on `ubuntu-latest` runners.
- REQ-NF-003: The `dist/` workflow artifact (8 archives × ~10MB plus checksums) must fit comfortably under standard GH Actions storage quotas. Retention: 7 days (default fine).
- REQ-NF-004: Wrapper bin shim cold-start overhead under 100ms on a modern machine — mostly Node startup. No additional npm packages required at runtime; only Node stdlib.
- REQ-NF-005: All committed files under `distribution/` use LF line endings (enforced via `.gitattributes`).
- REQ-NF-006: Publish script logs are clear enough that a maintainer can tell from CI logs which packages got published, which were skipped (idempotency), and which failed — for fast diagnosis when a partial publish needs manual recovery.

## Technical Considerations

**Architecture**

The release pipeline becomes:
```
release.yml (build + GH release via GoReleaser)
    │
    └── uploads dist/ + release-meta.json artifacts, surfaces include_neo4j
        │
        └── publish-npm.yml (workflow_run, gated on include_neo4j == true)
            │
            └── distribution/npm/publish.sh (8 platforms → wrapper)

publish-npm.yml (workflow_dispatch, manual)
    │
    └── gh release download → extract → distribution/npm/publish.sh
```

The two-workflow split keeps the manual-recovery path ergonomically separate from the auto path. Putting npm as a job inside `release.yml` would force re-running GoReleaser on retry, which fails because the GH release tag already exists.

**Composability with future channels**

`distribution/platforms.tsv` is the single source of truth for the platform×arch matrix. Both npm publish and (future) pypi publish source it. Homebrew uses GoReleaser's built-in `brews:` config — no platform table consumer needed.

Adding pip:
1. Create `distribution/pypi/{publish.sh, wheel templates}`.
2. Add `.github/workflows/publish-pip.yml` mirroring `publish-npm.yml` two-trigger shape.
3. Source `distribution/platforms.tsv` for the same arch list.

No edits to npm code.

**Integration points**

- `.goreleaser.yaml` lines 17-39 (builds), 42-79 (archives) — produces `dist/neo4j-cli_<version>_<OS>_<arch>/neo4j-cli[.exe]`. Read-only consumer.
- `.github/workflows/release.yml` lines 28-33 (changie version extraction), 53-57 (`include_neo4j` boolean computation), 90-102 (GoReleaser step). Modified to surface job outputs and upload artifacts.
- `changie` flow drives version. `changie latest --project neo4j-cli` produces the source-of-truth string.
- npm `@neo4j` scope. The user owns this. NPM_TOKEN secret added to the repo.

**Potential challenges**

- *First-publish ordering*: wrapper's `optionalDependencies` reference platform pkgs that must already exist on the registry, else `npm install @neo4j-labs/cli` returns 404 for the first platform npm tries to resolve. The script publishes platforms first by design — must preserve that order.
- *Scope publish permissions*: `--access public` needed for first publish of a scoped package. The `@neo4j` org may need pre-configured publish settings or pre-created package stubs. Verify before the first real release.
- *Pre-stable install UX*: until the first stable `X.Y.Z` exists, `npm i @neo4j-labs/cli` (no qualifier) errors with "no matching version". This is intentional — alpha users opt in via `@alpha`. Documented in user-facing README.
- *Windows runner / line endings*: `.gitattributes` LF pinning matters because `make generate-check` runs on Windows in CI; if anything under `distribution/` is consumed by a Go test in the future, CRLF would break byte-equal checks. Pin LF up front.
- *Workflow artifact size*: 8 archives × ~10MB ≈ 80MB per release. Fine for standard quotas but should monitor.

**Reused existing pieces**

- GoReleaser `dist/` archive layout — npm script reads from here unchanged.
- Version extraction expression from `release.yml:33` — npm script reuses the same `changie latest --project neo4j-cli | sed` pattern.
- `include_neo4j` boolean from `release.yml:53-57` — gates both the existing release notes step and the new npm workflow.
- `.gitattributes` LF pinning convention — extend with new globs, don't redesign.
- `make snapshot` for local goreleaser runs — basis of the local dry-run flow.

## Acceptance Criteria

**Local**
- [ ] `make snapshot` followed by `make npm-publish-dry` produces no errors and prints what would be published for all 9 packages.
- [ ] `node distribution/npm/cli/bin/neo4j-cli.js --version` works via `npm link` against a locally-built platform pkg.

**CI dry-run on a feature branch**
- [ ] Triggering an aura-cli-only release path → `publish-npm.yml` skips itself via `include_neo4j != true`. Confirmed in the Actions log.
- [ ] Manual `workflow_dispatch` of `publish-npm.yml` with a version input pointing at an existing alpha tag → all 9 packages report `[skip] already published` and the workflow exits 0.

**End-to-end on a real release**
- [ ] Cut a `neo4j-cli` alpha release (e.g. `0.2.0-alpha.1`). After workflow completes:
  - `npm view @neo4j-labs/cli dist-tags` shows `alpha: 0.2.0-alpha.1` and NO `latest` pointer at it.
  - `npm i @neo4j-labs/cli` (no qualifier) does NOT install the alpha.
  - `npm i @neo4j-labs/cli@alpha` installs `0.2.0-alpha.1`.
  - `npm view @neo4j-labs/cli-darwin-arm64 versions` (and the other 7 platform pkgs) shows the new version.
- [ ] On macOS, Linux (e.g. `docker run -it node:20`), and Windows (PowerShell): `npm i -g @neo4j-labs/cli@alpha` followed by `neo4j-cli --version` returns the right version on each.
- [ ] `npm i --omit=optional @neo4j-labs/cli@alpha` followed by `neo4j-cli` produces the friendly "no prebuilt binary" error from the bin shim, exits non-zero, no stack trace.
- [ ] After cutting a stable release later, `npm view @neo4j-labs/cli dist-tags` shows `latest: X.Y.Z` AND the alpha tag still points at the original alpha (independent channels).
- [ ] Manual recovery drill: re-run `publish-npm.yml` via `workflow_dispatch` against a fully-published version → all 9 packages skip, workflow exits 0.

**Documentation**
- [ ] `distribution/npm/README.md` exists and covers all 10 outlined sections.
- [ ] `distribution/npm/cli/README.md` ships in the published tarball and shows install + usage.
- [ ] Repo-root `README.md` install section mentions `npm i -g @neo4j-labs/cli`.
- [ ] Changelog entry exists in `.changes/unreleased/` for `neo4j-cli` only (not aura-cli).

## Out of Scope

(Restated for clarity — overlap with Non-Goals.)

- pip / PyPI distribution.
- Homebrew distribution.
- npm provenance attestation.
- Shipping `aura-cli` on npm.
- Linux 32-bit ARM (`linux-arm`).
- Auto-promoting prerelease tags to `latest`.
- An `npm install` UX that bypasses the prerelease gate (e.g. installing alpha by default during alpha cycle).

## Open Questions

1. Does the `@neo4j` npm scope already permit publishing public packages, or must the maintainer pre-create stubs / configure scope-level publish settings before the first release?
2. Should the maintainer-facing `distribution/npm/README.md` link directly to the npm package URLs (`https://www.npmjs.com/package/@neo4j-labs/cli`), or stay registry-agnostic until the scope is confirmed live?
3. Is the `next` catch-all dist-tag (for non-alpha/beta/rc prereleases) actually needed, or should the script reject any unrecognized prerelease format outright? (Defensive default vs strict validation.)
4. Should the user-facing `distribution/npm/cli/README.md` document the alpha/beta channels, or keep the surface minimal until a stable ships?
5. Retention policy for the `dist/` workflow artifact — default 7 days, or shorter to save quota?
