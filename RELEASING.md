# Releasing

End-to-end release lifecycle for `neo4j-cli`. Most of it is automated — your job as a contributor is one changelog entry per PR; everything downstream happens on merge.

For the why behind individual pieces, see `.agents/deployment.md` (architecture) and `distribution/<channel>/README.md` (channel specifics).

## TL;DR

1. Add a changelog entry on your PR (`make changelog`).
2. Merge your PR. Nothing publishes — `changie` opens a "Release" PR collecting unreleased entries.
3. Merge the Release PR. **This is the publish gate.** GoReleaser ships binaries to GitHub Releases; `publish-npm.yml` ships `@neo4j-labs/cli` to npm.

## What gets released

| Artifact | Channel | Driven by |
|---|---|---|
| `neo4j-cli` binaries | GitHub Releases | GoReleaser |
| `@neo4j-labs/cli` (super-CLI) | npm | `publish-npm.yml` |
| `neo4j-cli` Homebrew formula | `neo4j-labs/homebrew-tap` (stable only) | GoReleaser (`brews:`) |

Future channels (pip) will plug in alongside `publish-npm.yml`.

## Step 1 — Add a changelog entry on your PR

User-facing changes (new features, bug fixes, behavior changes visible to CLI users) need a changelog entry. Internal-only changes (CI, refactors, build tooling with no user impact) don't.

```bash
make changelog
```

Interactive: pick a kind (`Major` / `Minor` / `Patch`) and a body. Commit the resulting YAML in `.changes/unreleased/` alongside your code.

Non-interactive form:

```bash
changie new --projects neo4j-cli --kind Patch --body "fix instance list pagination"
```

PR review and merge proceed normally. **Nothing publishes when your feature PR merges.**

## Step 2 — `changie.yml` opens a Release PR (auto)

On every push to `main`, `.github/workflows/changie.yml` runs:

1. Detects unreleased entries (`grep project: neo4j-cli .changes/unreleased/`).
2. Computes the next pre-release suffix (`alpha.N+1`).
3. Runs `changie batch` — folds `.changes/unreleased/*.yaml` into `.changes/neo4j-cli/v<version>.md`.
4. Runs `changie merge` — appends to `CHANGELOG-neo4j.md`.
5. Opens a PR titled `Release neo4j-cli vX.Y.Z` on a `release/...` branch.

This PR is the *request* to ship. It contains only changelog updates — no source changes. Review it like any other PR.

## Step 3 — Merge the Release PR (publish gate)

`.github/workflows/release.yml` triggers on pushes to `main` that touch `CHANGELOG-neo4j.md` — merging the Release PR is what does that.

The job:

- Reads the version: `changie latest --project neo4j-cli`.
- Runs **GoReleaser**:
  - Builds `neo4j-cli` for 8 archs: `linux/{amd64,arm64,386}`, `darwin/{amd64,arm64}`, `windows/{amd64,arm64,386}`. Archives are `.tar.gz` (Unix) / `.zip` (Windows).
  - Code-signs and notarizes the macOS binaries (`MACOS_SIGN_*`, `MACOS_NOTARY_*` secrets).
  - Creates a **GitHub Release** with all archives + checksums attached and tags the commit (e.g. `v0.2.0-alpha.3`).
  - The binary version is stamped at link time via `GORELEASER_CURRENT_TAG`.
- Surfaces `version` as a job output.
- Uploads `dist/` and `release-meta.json` (`{ version }`) as workflow artifacts for the npm workflow to consume.

Merge of the Release PR = release pushed. There is no manual step here.

## Step 4 — `publish-npm.yml` runs (auto)

Triggered by `workflow_run` after `release.yml` completes. The job:

- Skips itself if `release.yml` did not succeed.
- Downloads the `dist/` artifact from `release.yml`.
- Authenticates to the registry via npm Trusted Publishers (OIDC); no long-lived token in CI.
- Runs `distribution/npm/publish.sh`:
  - Picks an npm dist-tag from the version: `*-alpha*` → `alpha`; `*-beta*` → `beta`; `*-rc*` → `rc`; any other prerelease → `next`; `X.Y.Z` (no suffix) → `latest`.
  - Publishes the 8 platform packages (`@neo4j-labs/cli-darwin-arm64`, …) first, then the wrapper `@neo4j-labs/cli` last.
  - Skips any `name@version` already on the registry (idempotent — safe to retry).

User effect:

- `npm i @neo4j-labs/cli` → always resolves to the latest stable.
- `npm i @neo4j-labs/cli@alpha` (or `@beta`, `@rc`) → opt-in to a prerelease channel.

For npm specifics — package shape, dist-tag rules, dry-run flow — see [`distribution/npm/README.md`](distribution/npm/README.md).

## Manual recovery: `publish-npm.yml` workflow_dispatch

If the npm publish fails partway (registry hiccup, transient 5xx, OIDC binding hiccup), recover via the Actions UI without bumping the version or re-running GoReleaser:

1. **Actions** → **Publish NPM** → **Run workflow**.
2. Enter the version (e.g. `0.2.0-alpha.3` — no leading `v`).
3. The manual path:
   - Runs `gh release download v${VERSION}` to pull archives from the existing GitHub Release (GoReleaser is **not** re-invoked).
   - Extracts each archive into the `dist/<name>/` layout `publish.sh` expects.
   - Re-runs `publish.sh` — already-published packages skip, the rest go through.

This same flow handles: `@neo4j-labs` org permission needed adjusting; you `npm unpublish`d a bad release and want to re-publish from clean state.

## Pre-releases vs stable

Today every push to `main` produces an alpha (`alpha.N+1`, computed in `changie.yml`). Stable releases are not yet wired into the changie workflow — when they are added, the dist-tag rules in `publish.sh` already handle the difference, and `npm i @neo4j-labs/cli` (no qualifier) will start resolving to the new stable automatically.

To promote an existing alpha to stable later, `npm dist-tag add @neo4j-labs/cli@<version> latest` — no republish needed.

## Local dry-runs

Before pushing a Release PR you want to be confident GoReleaser + the npm script will succeed.

- GoReleaser: `make snapshot` (single-platform) or `make snapshot-all`. See `CONTRIBUTING.md` "Building".
- npm publish dry-run: `make npm-publish-dry`. Renders all 9 `package.json` files, runs `npm publish --dry-run` for each, never touches the registry. See `distribution/npm/README.md` "Local dev / testing".

## Required secrets

Configured at the repo level. The user owns these.

| Secret | Used by | For |
|---|---|---|
| `TEAM_GRAPHQL_PERSONAL_ACCESS_TOKEN` | `changie.yml`, `release.yml` | Opening Release PRs, creating GitHub Releases |
| `MACOS_SIGN_P12`, `MACOS_SIGN_PASSWORD` | `release.yml` | macOS code-signing |
| `MACOS_NOTARY_ISSUER_ID`, `MACOS_NOTARY_KEY_ID`, `MACOS_NOTARY_KEY` | `release.yml` | macOS notarization |
| `HOMEBREW_TAP_APP_ID`, `HOMEBREW_TAP_APP_PRIVATE_KEY` | `release.yml` | Mint short-lived token for pushing the Homebrew formula to `neo4j-labs/homebrew-tap` |

## See also

- `.agents/deployment.md` — release infrastructure architecture (agent reference)
- `distribution/npm/README.md` — npm-specific maintainer view (package shape, dist-tag rules, dry-runs)
- `distribution/homebrew/README.md` — Homebrew tap maintainer view (stable-only cadence, auth prereqs, recovery)
- `CONTRIBUTING.md` — changelog entries, local builds, repo conventions
- `.changie.yaml` — changelog config
- `.goreleaser.yaml` — GoReleaser build matrix, archives, signing
