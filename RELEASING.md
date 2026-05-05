# Releasing

End-to-end release lifecycle for `aura-cli` and `neo4j-cli`. Most of it is automated — your job as a contributor is one changelog entry per PR; everything downstream happens on merge.

For the why behind individual pieces, see `.agents/deployment.md` (architecture) and `distribution/<channel>/README.md` (channel specifics).

## TL;DR

1. Add a changelog entry on your PR (`make changelog`).
2. Merge your PR. Nothing publishes — `changie` opens a "Release" PR collecting unreleased entries.
3. Merge the Release PR. **This is the publish gate.** GoReleaser ships binaries to GitHub Releases; `publish-npm.yml` ships `@neo4j-labs/cli` to npm.

## What gets released

| Artifact | Channel | Versioned independently | Driven by |
|---|---|---|---|
| `aura-cli` binaries | GitHub Releases | `aura-cli` changelog | GoReleaser |
| `neo4j-cli` binaries | GitHub Releases | `neo4j-cli` changelog | GoReleaser |
| `@neo4j-labs/cli` (super-CLI) | npm | `neo4j-cli` changelog | `publish-npm.yml` |

Aura-cli is **not** published to npm. An aura-cli-only release cycle ships GitHub binaries but skips the npm workflow. See `distribution/npm/README.md` for the gate.

Future channels (pip, Homebrew) will plug in alongside `publish-npm.yml` and follow the same gating rules.

## Step 1 — Add a changelog entry on your PR

User-facing changes (new features, bug fixes, behavior changes visible to CLI users) need a changelog entry. Internal-only changes (CI, refactors, build tooling with no user impact) don't.

```bash
make changelog
```

Interactive: pick one or more projects (`aura-cli`, `neo4j-cli`), a kind (`Major` / `Minor` / `Patch`), and a body. Commit the resulting YAML in `.changes/unreleased/` alongside your code.

Because `neo4j-cli` bundles its children, **any user-facing change to `aura-cli` also needs a `neo4j-cli` entry** — select both. Non-interactive form:

```bash
changie new --projects aura-cli --projects neo4j-cli --kind Patch --body "fix instance list pagination"
```

Only changes specific to the `neo4j-cli` super-CLI wrapper itself (e.g. the `npm i -g @neo4j-labs/cli` install path, the skill bundle for `neo4j-cli`) need a `neo4j-cli`-only entry.

PR review and merge proceed normally. **Nothing publishes when your feature PR merges.**

## Step 2 — `changie.yml` opens a Release PR (auto)

On every push to `main`, `.github/workflows/changie.yml` runs:

1. Detects which projects have unreleased entries (`grep project: aura-cli .changes/unreleased/`, `grep project: neo4j-cli`).
2. Computes the next pre-release suffix per project (`alpha.N+1`).
3. Runs `changie batch` per project — folds `.changes/unreleased/*.yaml` into `.changes/<project>/v<version>.md`.
4. Runs `changie merge` — appends to `CHANGELOG-aura.md` and/or `CHANGELOG-neo4j.md`.
5. Opens a PR titled `Release neo4j-cli vX.Y.Z` (or `aura-cli`, or both) on a `release/...` branch.

This PR is the *request* to ship. It contains only changelog updates — no source changes. Review it like any other PR.

## Step 3 — Merge the Release PR (publish gate)

`.github/workflows/release.yml` triggers on pushes to `main` that touch `CHANGELOG-aura.md` or `CHANGELOG-neo4j.md` — merging the Release PR is what does that.

The job:

- Reads versions: `changie latest --project aura-cli` and `changie latest --project neo4j-cli`.
- Computes `include_aura` / `include_neo4j` based on which `CHANGELOG-*.md` files actually changed in the merge commit (one-project releases skip the other).
- Runs **GoReleaser**:
  - Builds 8 archs each for `aura-cli` and `neo4j-cli`: `linux/{amd64,arm64,386}`, `darwin/{amd64,arm64}`, `windows/{amd64,arm64,386}`. Archives are `.tar.gz` (Unix) / `.zip` (Windows).
  - Code-signs and notarizes the macOS binaries (`MACOS_SIGN_*`, `MACOS_NOTARY_*` secrets).
  - Creates a **GitHub Release** with all archives + checksums attached and tags the commit (e.g. `v0.2.0-alpha.3`).
  - Each binary gets its own version stamped at link time (`AURA_CLI_VERSION`, `GORELEASER_CURRENT_TAG`).
- Surfaces `version` + `include_neo4j` as job outputs.
- Uploads `dist/` and `release-meta.json` (`{ version, include_neo4j }`) as workflow artifacts for the npm workflow to consume.

Merge of the Release PR = release pushed. There is no manual step here.

## Step 4 — `publish-npm.yml` runs (auto, neo4j-cli only)

Triggered by `workflow_run` after `release.yml` completes. The job:

- Skips itself if `release.yml` did not succeed, or if `include_neo4j != true` (aura-cli-only release).
- Downloads the `dist/` artifact from `release.yml`.
- Writes `~/.npmrc` from the `NPM_TOKEN` secret.
- Runs `distribution/npm/publish.sh`:
  - Picks an npm dist-tag from the version: `*-alpha*` → `alpha`; `*-beta*` → `beta`; `*-rc*` → `rc`; any other prerelease → `next`; `X.Y.Z` (no suffix) → `latest`.
  - Publishes the 8 platform packages (`@neo4j-labs/cli-darwin-arm64`, …) first, then the wrapper `@neo4j-labs/cli` last.
  - Skips any `name@version` already on the registry (idempotent — safe to retry).

User effect:

- `npm i @neo4j-labs/cli` → always resolves to the latest stable.
- `npm i @neo4j-labs/cli@alpha` (or `@beta`, `@rc`) → opt-in to a prerelease channel.

For npm specifics — package shape, dist-tag rules, dry-run flow — see [`distribution/npm/README.md`](distribution/npm/README.md).

## Manual recovery: `publish-npm.yml` workflow_dispatch

If the npm publish fails partway (registry hiccup, missing token, transient 5xx), recover via the Actions UI without bumping the version or re-running GoReleaser:

1. **Actions** → **Publish NPM** → **Run workflow**.
2. Enter the version (e.g. `0.2.0-alpha.3` — no leading `v`).
3. The manual path:
   - Runs `gh release download v${VERSION}` to pull archives from the existing GitHub Release (GoReleaser is **not** re-invoked).
   - Extracts each archive into the `dist/<name>/` layout `publish.sh` expects.
   - Re-runs `publish.sh` — already-published packages skip, the rest go through.

This same flow handles: NPM_TOKEN was missing or expired and got rotated; `@neo4j-labs` org permission needed adjusting; you `npm unpublish`d a bad release and want to re-publish from clean state.

## Pre-releases vs stable

Today every push to `main` produces an alpha (`alpha.N+1` per project, computed in `changie.yml`). Stable releases are not yet wired into the changie workflow — when they are added, the dist-tag rules in `publish.sh` already handle the difference, and `npm i @neo4j-labs/cli` (no qualifier) will start resolving to the new stable automatically.

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
| `NPM_TOKEN` | `publish-npm.yml` | Publishing under `@neo4j-labs` scope |

## See also

- `.agents/deployment.md` — release infrastructure architecture (agent reference)
- `distribution/npm/README.md` — npm-specific maintainer view (package shape, dist-tag rules, dry-runs)
- `CONTRIBUTING.md` — changelog entries, local builds, repo conventions
- `.changie.yaml` — multi-project changelog config
- `.goreleaser.yaml` — GoReleaser build matrix, archives, signing
