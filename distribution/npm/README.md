# `distribution/npm/` — `@neo4j-labs/cli` on npm

Maintainer-facing design + rationale. Not shipped to npm. The user-facing
README that ships in the published tarball is at [`cli/README.md`](./cli/README.md).

For the end-to-end release lifecycle (changelog → Release PR → GitHub Release → npm publish), see [`RELEASING.md`](../../RELEASING.md). This document covers npm specifics only.

## What this directory does

Publishes the `neo4j-cli` super-CLI to npm as `@neo4j-labs/cli`, so users can run
`npm i -g @neo4j-labs/cli` and get the right prebuilt binary on PATH for their
OS/arch. No postinstall script, no source build, no runtime Node toolchain
beyond the bin shim.

```
distribution/
  platforms.tsv                shared 8-row platform×arch matrix (npm + future pip)
  npm/
    README.md                  this file
    publish.sh                 orchestration: render templates, copy binaries, npm publish
    cli/                       wrapper package source
      bin/neo4j-cli.js         Node bin shim: require.resolve + spawnSync passthrough
      package.json.tmpl        wrapper package.json template (sed-rendered)
      README.md                user-facing README, ships in published tarball
    cli-platform/
      package.json.tmpl        per-platform package.json template (sed-rendered)
```

## Package shape

One wrapper + 8 platform packages. Wrapper depends on all 8 via
`optionalDependencies`; npm only installs the one matching `process.platform` +
`process.arch`.

```
                              ┌─────────────────────────────┐
                              │ @neo4j-labs/cli (wrapper)   │
                              │  bin/neo4j-cli.js           │
                              │  optionalDependencies: 8    │
                              └──────────────┬──────────────┘
                                             │ require.resolve at runtime
           ┌─────────────────────────────────┼─────────────────────────────────┐
           ▼                                 ▼                                 ▼
┌──────────────────────┐          ┌──────────────────────┐          ┌──────────────────────┐
│ @neo4j-labs/cli-     │   ...    │ @neo4j-labs/cli-     │   ...    │ @neo4j-labs/cli-     │
│   darwin-arm64       │          │   linux-x64          │          │   win32-x64          │
│ os: [darwin]         │          │ os: [linux]          │          │ os: [win32]          │
│ cpu: [arm64]         │          │ cpu: [x64]           │          │ cpu: [x64]           │
└──────────────────────┘          └──────────────────────┘          └──────────────────────┘
```

Matrix lives in [`../platforms.tsv`](../platforms.tsv) — single source of truth
for `publish.sh` and any future `distribution/pypi/` consumer.

| `process.platform` | `process.arch` | npm package                       | binary          |
| ------------------ | -------------- | --------------------------------- | --------------- |
| `darwin`           | `arm64`        | `@neo4j-labs/cli-darwin-arm64`    | `neo4j-cli`     |
| `darwin`           | `x64`          | `@neo4j-labs/cli-darwin-x64`      | `neo4j-cli`     |
| `linux`            | `arm64`        | `@neo4j-labs/cli-linux-arm64`     | `neo4j-cli`     |
| `linux`            | `ia32`         | `@neo4j-labs/cli-linux-ia32`      | `neo4j-cli`     |
| `linux`            | `x64`          | `@neo4j-labs/cli-linux-x64`       | `neo4j-cli`     |
| `win32`            | `arm64`        | `@neo4j-labs/cli-win32-arm64`     | `neo4j-cli.exe` |
| `win32`            | `ia32`         | `@neo4j-labs/cli-win32-ia32`      | `neo4j-cli.exe` |
| `win32`            | `x64`          | `@neo4j-labs/cli-win32-x64`       | `neo4j-cli.exe` |

## Why multi-package, not postinstall?

esbuild popularized this layout for a reason. A `postinstall` that downloads
the binary at install time fails in: `npm i --ignore-scripts`, pnpm strict
mode, Bun, and airgapped CI behind locked-down registries. The wrapper +
optional-deps shape lets npm itself do platform selection during
`npm install`, with no script execution. Trade-off: 9 packages to publish per
release. The publish script handles that.

## Install resolution flow

1. `npm i -g @neo4j-labs/cli` reads the wrapper's 8 `optionalDependencies`.
2. For each, npm checks `os` + `cpu` constraints in the platform package's
   `package.json`. Only the matching one installs; others are skipped silently.
3. npm symlinks `bin/neo4j-cli.js` into the user's PATH.
4. On invocation, the shim does
   `require.resolve('@neo4j-labs/cli-' + platform + '-' + arch + '/bin/neo4j-cli' + (win32 ? '.exe' : ''))`,
   then `spawnSync(binary, argv.slice(2), { stdio: 'inherit' })`, then
   `process.exit(result.status ?? 1)`.
5. If the platform package is missing (unsupported arch or `--omit=optional`
   install), the shim prints a friendly error naming the detected platform/arch
   + the supported list + the omit-flag hint, then exits non-zero. No stack trace.

## Dist-tag channels

The publish script picks the npm `dist-tag` from the version string:

| version pattern         | `--tag`                   | install command                    |
| ----------------------- | ------------------------- | ---------------------------------- |
| `X.Y.Z`                 | (omit → default `latest`) | `npm i -g @neo4j-labs/cli`         |
| `X.Y.Z-alpha*`          | `alpha`                   | `npm i -g @neo4j-labs/cli@alpha`   |
| `X.Y.Z-beta*`           | `beta`                    | `npm i -g @neo4j-labs/cli@beta`    |
| `X.Y.Z-rc*`             | `rc`                      | `npm i -g @neo4j-labs/cli@rc`      |
| anything else with `-`  | `next`                    | `npm i -g @neo4j-labs/cli@next`    |

`npm i @neo4j-labs/cli` (no qualifier) only resolves to a `latest` version. Until
the first stable `X.Y.Z` exists, the unqualified install errors with "no
matching version" — alpha users opt in via `@alpha`. Promoting a prerelease to
stable is a manual `npm dist-tag add` call, not part of this automation.

## Local dev / test flow

```sh
make snapshot           # build current platform binary into dist/
make npm-publish-dry    # render templates + exercise publish ordering, no registry hit
```

`make npm-publish-dry` invokes `distribution/npm/publish.sh --dry-run` with
`GORELEASER_CURRENT_TAG=v0.0.0-dry`. In dry-run mode, missing platform binaries
are stubbed with a 1-byte placeholder so the maintainer can sanity-check
template rendering and publish ordering without building all 8 platforms (only
the current one is built by `make snapshot`). Real publishes (CI) hard-error
on a missing binary.

To test the wrapper bin shim against a locally-built platform package: render
`cli-platform/package.json.tmpl` with `sed`, drop the binary into `bin/`,
`cd cli && npm link`, then `node bin/neo4j-cli.js --version`.

## Bootstrap (one-time)

npm Trusted Publishers must be configured per package in the npm UI, and
unlike PyPI, npm has no pending-publisher flow — the package name has to
exist on the registry before the OIDC binding can be set. So before the
first OIDC publish, a maintainer claims all 9 `@neo4j-labs/*` names with a
throwaway stub release:

```sh
npm login --scope=@neo4j-labs    # short-lived, only for the bootstrap
make npm-bootstrap                # or: bash distribution/npm/bootstrap-stubs.sh
npm logout                        # drop the local credential
```

`bootstrap-stubs.sh` hardcodes `VERSION=0.0.0-bootstrap.1` and always passes
`--tag bootstrap`. The 1-byte stub binaries it ships are unrunnable by
design — the dist-tag keeps them out of `latest`/`alpha`/`beta`/`rc`/`next`,
so `npm i @neo4j-labs/cli` (and every qualified-channel install) keeps
returning `no matching version` until a real release lands. Re-running the
script is safe: each package's `npm view <name>@<version> version` check
makes it print `[skip] <pkg>@0.0.0-bootstrap.1 already published` and exit 0.

After the script finishes, configure Trusted Publishers in the npm UI for
each of the 9 packages (`@neo4j-labs/cli` plus the 8 platform packages).
For every package, the binding is the same:

| Field          | Value             |
| -------------- | ----------------- |
| Organization   | `neo4j-labs`      |
| Repository     | `neo4j-cli`       |
| Workflow       | `publish-npm.yml` |
| Environment    | (leave blank)     |

The first real CI publish after the bindings are saved authenticates via
OIDC — no token is ever issued or stored. The bootstrap stubs can be
deprecated or `npm unpublish`d once a real version exists, but they are
harmless if left in place.

## Release flow

```
release.yml (build + GH release via GoReleaser)
    │
    └── uploads dist/ + release-meta.json artifacts; surfaces include_neo4j
        │
        └── publish-npm.yml (workflow_run, gated on workflow_run.conclusion == 'success')
            │
            └── parses release-meta.json, gates on include_neo4j == 'true'
                └── distribution/npm/publish.sh (8 platforms → wrapper)

publish-npm.yml (workflow_dispatch, manual; for failure recovery)
    │
    └── gh release download → extract archives → distribution/npm/publish.sh
```

- [`.goreleaser.yaml`](../../.goreleaser.yaml) — produces
  `dist/neo4j-cli_<version>_<OS>_<arch>/neo4j-cli[.exe]`. The
  `archives.name_template` defines the dir layout `publish.sh` expects.
- [`.github/workflows/release.yml`](../../.github/workflows/release.yml) —
  triggers on `CHANGELOG-neo4j.md` / `CHANGELOG-aura.md` changes. Surfaces
  `include_neo4j` and `version` as job outputs; uploads `dist/` +
  `release-meta.json` artifacts (only on neo4j-cli release runs).
- [`.github/workflows/publish-npm.yml`](../../.github/workflows/publish-npm.yml)
  — auto path on `workflow_run`; manual `workflow_dispatch` for recovery.
  Aura-cli-only release cycles skip via the `include_neo4j` gate.

## Failure recovery

`publish.sh` is idempotent at the per-package level: before each `npm publish`
it runs `npm view <name>@<version> version` and skips when the package+version
is already on the registry. Same-version retries are safe.

- **Transient registry 5xx after platforms 1-3**: trigger `publish-npm.yml`
  via `workflow_dispatch` with the same version. Platforms 1-3 hit `[skip]`,
  4-8 publish, wrapper publishes. Workflow exits 0.
- **Wrapper published before all platforms** (shouldn't happen — script order
  is platforms-then-wrapper): same-version manual re-run fixes it; wrapper
  hits `[skip]`, missing platform(s) publish.
- **Bad release**: no rollback step. Use `npm unpublish` only within npm's
  72-hour window, or `npm deprecate` after. Then publish a new patch.

CI logs show `[publish]`, `[skip]`, `[ERROR]` lines per package so a maintainer
can tell at a glance which package and where a partial publish failed.

## Adding a new platform

Three coordinated edits, no script changes:

1. Add a row to [`../platforms.tsv`](../platforms.tsv) with
   `(goreleaser_dirname_template, npm_os, npm_cpu, bin_filename)`.
2. Make sure GoReleaser produces an archive matching that dirname pattern
   ([`.goreleaser.yaml`](../../.goreleaser.yaml) `archives.name_template`).
3. Add the package name to the wrapper template's `optionalDependencies`
   ([`cli/package.json.tmpl`](./cli/package.json.tmpl)) and the `SUPPORTED`
   array in the bin shim ([`cli/bin/neo4j-cli.js`](./cli/bin/neo4j-cli.js)).

Reviewer catches the three-place edit; tests don't enforce it because the shim
ships standalone in the tarball with no runtime access to `platforms.tsv`.

## Future channels

- **pip / PyPI**: create `distribution/pypi/{publish.sh, wheel templates}` +
  `.github/workflows/publish-pip.yml` mirroring `publish-npm.yml`'s two-trigger
  shape. Source the same `distribution/platforms.tsv`. No edits to the npm side.
- **Homebrew**: GoReleaser has a built-in `brews:` config block — configure
  there rather than write a parallel publish script. Reserve
  `distribution/homebrew/` for any wrapper formula assets that aren't
  auto-generated.

Open follow-ups (not blocking the npm channel): Linux 32-bit ARM
(`linux-arm`, not built by GoReleaser today); auto-promotion of prerelease
`dist-tag` to `latest`.
