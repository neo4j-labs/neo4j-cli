# `distribution/pypi/` — `neo4j-cli` on PyPI

Maintainer-facing notes for the PyPI distribution channel. There is no runtime
code in this directory — wheel building and publishing are handled entirely by
[`../../.github/workflows/publish-pypi.yml`](../../.github/workflows/publish-pypi.yml)
plus [`go-to-wheel`](https://pypi.org/project/go-to-wheel/), which wraps the
GoReleaser-built binaries into platform-specific wheels.

For the end-to-end release lifecycle (changelog → Release PR → GitHub Release →
PyPI publish), see [`../../RELEASING.md`](../../RELEASING.md). This document
covers PyPI specifics only.

## What this directory does

Nothing at runtime — PyPI shipping is fully handled by the publish-pypi
workflow. Six platform wheels (one per linux/darwin/windows × amd64/arm64) are
built per release by feeding the pre-built GoReleaser binaries into
`go_to_wheel.build_wheel()`, then uploaded in a single `pypi-publish` step.
This directory exists for documentation only.

## User install

```sh
pip install neo4j-cli                # stable releases only (vX.Y.Z)
pip install --pre neo4j-cli          # also picks up alpha/beta/rc
```

`pip` skips PEP 440 pre-releases by default (anything matching `aN`/`bN`/`rcN`),
so `--pre` is required to install an alpha/beta/rc. To pin a specific
prerelease:

```sh
pip install neo4j-cli==0.1.0a6
```

## PyPI project location

[`pypi.org/project/neo4j-cli`](https://pypi.org/project/neo4j-cli/) — the
canonical PyPI project. Six platform wheels are published per release; there
is no source distribution (sdist), since the wheels carry pre-built binaries.

Project ownership is managed via the PyPI account associated with
`PYPI_API_TOKEN` (see [Auth prerequisites](#auth-prerequisites) below).

## Release cadence — stable + prereleases

Unlike Homebrew (which ships only `vX.Y.Z` stable tags), PyPI receives every
release. PEP 440 marks `aN`/`bN`/`rcN` as pre-releases by spec, so
`pip install neo4j-cli` (without `--pre`) keeps resolving to the latest stable
even after a fresh alpha is published — alpha users opt in via `--pre`.

The publish-pypi workflow auto-fires on every successful `release.yml` run via
`workflow_run`, gated on `release-meta.json`'s `include_neo4j == true`.

## PEP 440 version mapping

Git tags in this repo follow `vX.Y.Z` or `vX.Y.Z-(alpha|beta|rc).N`. PEP 440
spells the same prerelease shapes differently, so the workflow normalises the
tag with [`../../.github/scripts/version-to-pep440.sh`](../../.github/scripts/version-to-pep440.sh)
before stamping it onto the wheel:

| Git tag             | PEP 440 wheel version | Pre-release? |
| ------------------- | --------------------- | ------------ |
| `vX.Y.Z`            | `X.Y.Z`               | no           |
| `vX.Y.Z-alpha.N`    | `X.Y.ZaN`             | yes          |
| `vX.Y.Z-beta.N`     | `X.Y.ZbN`             | yes          |
| `vX.Y.Z-rc.N`       | `X.Y.ZrcN`            | yes          |

Worked example: the `v0.1.0-alpha.6` tag (the most recent prerelease in this
repo) publishes as `0.1.0a6` on PyPI. The binary inside the wheel still reports
`0.1.0-alpha.6` from `neo4j-cli --version` — only the wheel package version is
PEP 440-shaped. See REQ-F-006 in
[`../../.plans/prd-release-to-pypi.md`](../../.plans/prd-release-to-pypi.md)
for the rationale.

Anything outside this contract (e.g. `v0.1.0-pre.6`, `v0.1.0.alpha.6`) makes
the workflow fail at the version-normalisation step before any wheel is built.

## Platform wheels published per release

Six wheels per release, one per supported platform. The PyPI platform tag in
each filename is set by `go_to_wheel.PLATFORM_MAPPINGS`:

| Platform        | Wheel filename suffix                     |
| --------------- | ----------------------------------------- |
| linux-amd64     | `-py3-none-manylinux_2_17_x86_64.whl`     |
| linux-arm64     | `-py3-none-manylinux_2_17_aarch64.whl`    |
| darwin-amd64    | `-py3-none-macosx_10_9_x86_64.whl`        |
| darwin-arm64    | `-py3-none-macosx_11_0_arm64.whl`         |
| windows-amd64   | `-py3-none-win_amd64.whl`                 |
| windows-arm64   | `-py3-none-win_arm64.whl`                 |

Example wheel filenames for the `v0.1.0-alpha.6` (= `0.1.0a6`) release:

```
neo4j_cli-0.1.0a6-py3-none-manylinux_2_17_x86_64.whl
neo4j_cli-0.1.0a6-py3-none-manylinux_2_17_aarch64.whl
neo4j_cli-0.1.0a6-py3-none-macosx_10_9_x86_64.whl
neo4j_cli-0.1.0a6-py3-none-macosx_11_0_arm64.whl
neo4j_cli-0.1.0a6-py3-none-win_amd64.whl
neo4j_cli-0.1.0a6-py3-none-win_arm64.whl
```

`pip` picks the right one for the user's OS/arch automatically. The list of
platforms shipped is hard-coded in the wheel-build step of
[`../../.github/workflows/publish-pypi.yml`](../../.github/workflows/publish-pypi.yml);
adding a platform requires editing that list and ensuring GoReleaser builds a
matching `dist/neo4j-cli_<version>_<OS>_<arch>/` directory.

## Local dry-run

Before merging a change to `publish-pypi.yml`, exercise the build + smoke-test
jobs without hitting PyPI by triggering `workflow_dispatch` with `dry_run: true`:

```sh
gh workflow run publish-pypi.yml \
  --ref main \
  -f version=0.1.0-alpha.6 \
  -f dry_run=true
```

That runs the wheel build and the smoke-test job (which installs the
linux-amd64 wheel into a clean venv and `--version`-greps the bundled binary)
but skips the final `pypi-publish` step. Use this to sanity-check workflow
edits against an existing GitHub Release without burning a real version slot
on PyPI.

`dry_run: true` is workflow-only — there is no equivalent local Make target.
The wheels themselves can be reproduced locally by running
`go_to_wheel.build_wheel()` against `make snapshot` output, but the
end-to-end orchestration (artifact wiring, smoke test, publish) only runs in CI.

## Manual recovery

If the auto path fails (transient PyPI 5xx, runner blip mid-publish), re-run
without re-cutting a release via `workflow_dispatch`:

```sh
gh workflow run publish-pypi.yml \
  --ref main \
  -f version=0.1.0-alpha.6 \
  -f dry_run=false
```

The manual path uses `gh release download "v${VERSION}"` to pull the existing
GitHub Release archives, extracts them into the same `dist/<basename>/` layout
the auto path produces, and feeds the binaries through the same wheel-build
step. Do **not** prefix the `version` input with a leading `v` — the workflow
adds it back where needed.

PyPI is largely idempotent for unique filenames; same-version retries on
already-uploaded wheels return `HTTP 400 File already exists`. If you need to
re-publish, bump the version and cut a new release — PyPI does not support
re-uploading a deleted file under the same name.

## Auth prerequisites

The publish step authenticates via a long-lived PyPI API token stored as a
repo secret:

| Secret              | Used by                                  | For                                |
| ------------------- | ---------------------------------------- | ---------------------------------- |
| `PYPI_API_TOKEN`    | `publish-pypi.yml` → `Publish to PyPI`   | `pypa/gh-action-pypi-publish` auth |

The token is scoped to the `neo4j-cli` PyPI project only. Rotate via the PyPI
account settings → API tokens; update the secret on `neo4j/cli` immediately
after rotation.

**Follow-up — OIDC trusted publishing.** PyPI now supports OIDC-based trusted
publishing (analogous to npm's Trusted Publishers), which would remove
`PYPI_API_TOKEN` from the repo secret store. Migration is **out of scope for
the initial PyPI release** but tracked as a follow-up — see "Open Questions"
in [`../../.plans/prd-release-to-pypi.md`](../../.plans/prd-release-to-pypi.md).

## See also

- [`../../RELEASING.md`](../../RELEASING.md) — full release lifecycle
- [`../../.github/workflows/publish-pypi.yml`](../../.github/workflows/publish-pypi.yml) — workflow source of truth
- [`../../.github/scripts/version-to-pep440.sh`](../../.github/scripts/version-to-pep440.sh) — version normaliser
- [`../npm/README.md`](../npm/README.md) — npm channel (parallel structure, OIDC reference)
- [`../homebrew/README.md`](../homebrew/README.md) — Homebrew channel (stable-only counterpart)
