# PRD: CLI-135 — Remove the aura-cli build config

## Overview

The release-side aura-cli build was already removed in a prior PR (see
`.plans/archive/tasks-remove-aura-cli-binary.yml`): `.goreleaser.yaml`,
`Makefile`, `.changie.yaml` `projects:` list, `.github/workflows/release.yml`
trigger paths, `.github/workflows/changie.yml` dual-project logic,
README.md, CONTRIBUTING.md.

What remains is the **source-side** dual-bundle machinery: a second cobra
entrypoint, a separate skill-bundle subsystem with its own generator and
round-trip test, and stale aura-cli prose/workflow gates that all impose
the cost the Linear issue describes — "when adding thing we'd need to
update both neo4j-cli and aura-cli build configs."

This feature deletes the residual standalone-aura source code, the
parallel skill bundle subsystem, and the now-dead release-meta gating in
the npm publish workflow. After this PRD ships, the repo has **one**
cobra tree, **one** skill bundle, **one** `go:generate` target, and
adding an aura subcommand requires regenerating only the neo4j-cli
bundle.

## Goals

- Remove the dual-bundle generation requirement so changing an aura
  subcommand needs `go generate ./neo4j-cli/internal/skill/...` only,
  never a second invocation against an aura-only bundle.
- Eliminate the `neo4j-cli/aura/cmd/main.go` historical entrypoint that
  still compiles on `go build ./...` but ships nowhere.
- Strip dead `include_neo4j` gating from the release/npm-publish
  workflows now that `release.yml` triggers exclusively on
  `CHANGELOG-neo4j.md`.
- Scrub stale dual-binary prose from `.agents/*`, `RELEASING.md`,
  `distribution/npm/README.md`, `CHANGELOG.md`, and the relevant
  `AGENTS.md` paragraph so contributor docs reflect the single-binary
  reality.
- Preserve `CHANGELOG-aura.md` and `.changes/aura-cli/v*.md` as
  historical record (per user direction).
- Preserve `aura.NewStandaloneCmd` as an uncalled exported constructor
  (per user direction) so a future revival can re-mount the standalone
  binary without rewriting the constructor.

## Non-Goals

- Removing `CHANGELOG-aura.md` or the `.changes/aura-cli/` history
  directory — these stay byte-identical on disk.
- Removing `neo4j-cli/aura/internal/subcommands/credential/` — it remains
  the package `NewStandaloneCmd` mounts as `credential` and is distinct
  from the user-facing `neo4j-cli/internal/subcommands/credential/`.
- Modifying any user-facing CLI behaviour. After this PRD ships, every
  `neo4j-cli ...` command runs exactly as it did before. No exit-code
  change, no flag change, no output change.
- Reviving npm/Homebrew publication for aura-cli (it was never
  published to either channel).
- Touching the historical `CHANGELOG-neo4j.md` body.

## Requirements

### Functional Requirements

- **REQ-F-001:** The directory `neo4j-cli/aura/cmd/` (containing only
  `main.go`) is deleted in its entirety. `go build ./...` no longer
  compiles a standalone-aura `main` package.
- **REQ-F-002:** The directory `neo4j-cli/aura/internal/skill/` is
  deleted recursively: `embed.go`, `description.txt`, `additions.md`,
  `gen/main.go`, `gen/main_test.go`, `install_e2e_test.go`, and the
  whole committed `bundle/` tree (`SKILL.md` + `references/*.md`). No
  `go:generate` directive remains anywhere under `neo4j-cli/aura/`.
- **REQ-F-003:** `neo4j-cli/aura/aura.go` is updated so that:
  - the imports `"github.com/neo4j/cli/common/skill"` and
    `binskill "github.com/neo4j/cli/neo4j-cli/aura/internal/skill"` are
    removed,
  - the call `cmd.AddCommand(skill.NewCmd(cfg, binskill.Bundle, "aura-cli"))`
    inside `NewStandaloneCmd` is removed,
  - the rest of `NewStandaloneCmd` (the `--rw` flag registration, the
    `FlagErrorFunc`, the `config` and `credential` mounts) is preserved.
- **REQ-F-004:** `aura.NewStandaloneCmd` remains defined and exported in
  `neo4j-cli/aura/aura.go` even though it has no callers in-tree after
  REQ-F-001 lands. `make lint` (golangci-lint v2 with the configured
  `unused` linter) reports no warnings against it.
- **REQ-F-005:** `.github/workflows/publish-npm.yml` is updated:
  - the step `Skip — aura-cli-only release (auto)` (currently at
    `line 89`) is removed,
  - the leading comment fragment at line 8 referencing
    "aura-cli-only release runs are skipped" is removed/rewritten so
    no aura-cli prose remains,
  - the `Parse release-meta` step no longer reads, validates, or echoes
    `INCLUDE_NEO4J`/`include_neo4j` — it parses and emits `version`
    only,
  - every per-step `if: ... steps.meta.outputs.include_neo4j == 'true'`
    guard is removed (download dist, verify checksums, publish step,
    etc.). The remaining gating is the workflow_run success check at
    the top of the job plus the explicit `workflow_dispatch` path.
- **REQ-F-006:** `.github/workflows/release.yml` is updated:
  - the `Detect changed changelogs` step (currently lines 37–48) is
    removed,
  - the `include_neo4j` job-level output at line 13 is removed,
  - the `Write release-meta.json` step writes a one-field
    `{"version":"<ver>"}` payload (drop `include_neo4j`) and is run
    unconditionally (no `if:` guard),
  - the `Upload dist artifact` and `Upload release-meta artifact`
    steps are run unconditionally (no `if:` guard).
- **REQ-F-007:** `CHANGELOG.md` is collapsed to a single neo4j-cli
  pointer. A short trailing line notes that the historical
  `CHANGELOG-aura.md` documents the discontinued standalone aura-cli.
- **REQ-F-008:** `RELEASING.md` reads as a single-binary document: the
  `aura-cli` row is dropped from the "What gets released" table; the
  "Aura-cli is not published to npm or Homebrew" paragraph is removed;
  the `changie new --projects aura-cli --projects neo4j-cli ...`
  example becomes `changie new --projects neo4j-cli ...`; every
  reference to `CHANGELOG-aura.md`, `include_aura`, or
  `AURA_CLI_VERSION` in Steps 2 and 3 is removed; the top-of-file
  framing no longer says "for `aura-cli` and `neo4j-cli`".
- **REQ-F-009:** `.agents/build.md` is rewritten to reflect the
  single-binary build:
  - the Makefile-targets table loses `build-aura` and `run-aura` rows;
  - the `build` row reads "Build `bin/neo4j-cli`";
  - the "Dual-Binary GoReleaser Setup" section is removed;
  - the Changelog section drops the multi-project narrative and the
    `--projects aura-cli` example;
  - the `make run-aura` shell example is removed.
- **REQ-F-010:** `.agents/deployment.md` is rewritten to reflect the
  single-binary release:
  - the "Dual-Binary Releases" and "Dual-Version Injection" sections
    are removed;
  - "Strategy", "Release Flow", and "macOS Code Signing" no longer
    reference aura-cli or `AURA_CLI_VERSION`;
  - the "Versioning Policy" section either collapses to a single
    project paragraph or is deleted.
- **REQ-F-011:** `.agents/repo-layout.md` loses the aura-cli-specific
  bullets:
  - the bullet about the aura-cli generator importing
    `aura.NewStandaloneCmd` (currently line 17) is removed,
  - the bullet about `AuraBetaEnabled` and the aura-cli bundle
    surface (currently line 18) is removed,
  - the bullet about `Skill cobra mount` (currently line 19) is
    rewritten to mention only the super-CLI mount in `app.NewCmd`,
  - the trailing "Same applies to any new top-level mount on
    `aura.NewStandaloneCmd` (aura-cli bundle)" clause (currently in
    line 21) is removed.
- **REQ-F-012:** `distribution/npm/README.md` is updated:
  - line 183 ("triggers on `CHANGELOG-neo4j.md` / `CHANGELOG-aura.md`
    changes") drops the `CHANGELOG-aura.md` clause,
  - the line 184–185 phrasing about uploading "only on neo4j-cli
    release runs" is rewritten since the workflow now always uploads,
  - the line 188 "Aura-cli-only release cycles skip via the
    `include_neo4j` gate" sentence is removed.
- **REQ-F-013:** `AGENTS.md` is updated:
  - the bullet under "Makefile Notes" describing how `aura-client`
    credential lives in TWO places and requires two `go generate`
    invocations is rewritten to note that the
    `neo4j-cli/aura/internal/subcommands/credential/` source still
    exists but only feeds `NewStandaloneCmd` and no longer drives a
    second bundle regeneration,
  - any other AGENTS.md paragraph that asserts aura-cli has a
    separately generated bundle is brought into line with the new
    single-bundle state (the standalone-CLI template paragraph itself
    stays — the template description is still factually correct).
- **REQ-F-014:** A `.changes/unreleased/neo4j-cli-Patch-<timestamp>.yaml`
  file is added in the final commit, project `neo4j-cli`, kind `Patch`,
  body wording aligned with: "Remove residual aura-cli source/build
  artifacts (standalone entrypoint, skill bundle subsystem, dead
  workflow gating, stale docs)."

### Non-Functional Requirements

- **REQ-NF-001:** `make build` exits 0 on a clean checkout after these
  changes; the only built artifact under `bin/` is `neo4j-cli` (no
  `aura-cli`).
- **REQ-NF-002:** `make test` exits 0. The neo4j-cli skill round-trip
  test (`TestGenerator_RoundTrip` in `neo4j-cli/internal/skill/gen`)
  continues to pass byte-equally; no test in the repo depends on the
  deleted `neo4j-cli/aura/internal/skill/...` packages.
- **REQ-NF-003:** `make lint` (golangci-lint v2 with `govet`, `errcheck`,
  `staticcheck`, `unused`) exits 0. In particular `unused` does not
  flag `aura.NewStandaloneCmd` despite having no in-tree caller.
- **REQ-NF-004:** `make fmt-check` exits 0 (no gofmt drift).
- **REQ-NF-005:** `make generate-check` exits 0 — after `go generate ./...`
  the working tree is clean. The only remaining `go:generate` directive
  is under `neo4j-cli/internal/skill/`.
- **REQ-NF-006:** `make license-check` exits 0 (no new `.go` files; all
  remaining `.go` files keep their existing Neo4j copyright headers).
- **REQ-NF-007:** `make snapshot` produces only `neo4j-cli_*` artefacts
  in `dist/`; no `aura-cli_*` anything.
- **REQ-NF-008:** `go build ./...` exits 0 — no stranded import paths
  pointing at the deleted `neo4j-cli/aura/cmd` or
  `neo4j-cli/aura/internal/skill` packages.
- **REQ-NF-009:** The grep gate
  `grep -rn 'aura-cli\|AURA_CLI_VERSION\|CHANGELOG-aura\|include_aura\|/aura/cmd\|/aura/internal/skill' . --exclude-dir=.git --exclude-dir=.plans --exclude-dir=.changes --exclude=CHANGELOG-aura.md`
  returns zero hits *except* (a) the deliberate `CHANGELOG-aura.md`
  pointer footnote in `CHANGELOG.md` and (b) the aura source tree's
  legitimate use of the package name "aura" (which the grep pattern
  does not match — only the literal token `aura-cli`).
- **REQ-NF-010:** Each commit must keep the tree green for the gates
  above; the PR may be a single commit or a series, but no intermediate
  commit may break `make build` / `make test` / `make lint`.
- **REQ-NF-011:** Work lands on a branch prefixed `oskar/` (per the
  user's global git convention), e.g.
  `oskar/cli-135-remove-the-aura-cli-build-config`.

## Technical Considerations

### Linter behaviour around the kept `NewStandaloneCmd`

`golangci-lint` v2 is configured with `linters.default: none` and
explicit enables for `govet`, `errcheck`, `staticcheck`, `unused`
(`.golangci.yml`). The `unused` analyzer (from `honnef.co/go/tools`)
does not flag exported identifiers of non-`main` packages by default —
they are treated as API. Therefore `aura.NewStandaloneCmd` can remain
exported with no in-tree caller without triggering a lint warning.
If a future config tightens this (e.g. enabling `exported-fields-are-used`
or similar), the function would need either a single internal sentinel
caller or a `//lint:ignore U1000 ...` directive.

### Workflow gate teardown — order matters

`.github/workflows/release.yml` produces `release-meta.json` whose
`include_neo4j` field is consumed by `.github/workflows/publish-npm.yml`'s
`workflow_run` path. Both sides must change atomically in the same PR
so a re-running upstream cannot produce a payload the downstream
workflow rejects. The publish-npm `Parse release-meta` step's
validation regex for `include_neo4j` is the strictest piece — once that
step stops reading the field, the producer can stop writing it. The
plan does both in this PRD.

The npm publish workflow's `workflow_dispatch` (manual recovery) path
never consumed `include_neo4j` (it always re-runs the publish), so it
needs no change beyond inheriting any renamed step ids.

### `NewStandaloneCmd` after the skill mount goes away

`NewStandaloneCmd` keeps its existing five responsibilities (calling
`NewCmd`, registering `--rw`, setting `FlagErrorFunc`, mounting
`config`, mounting `credential`). It loses the sixth (mounting
`skill`). This is intentional: the super-CLI root in
`neo4j-cli/app/app.go` already mounts `skill.NewCmd` against the
neo4j-cli bundle, so users invoking `neo4j-cli skill ...` are unaffected.
A future revival of a standalone aura-cli would need to choose a skill
bundle to mount; that decision can be deferred until the revival
happens.

### Round-trip test surface change

After REQ-F-002, the only `TestGenerator_RoundTrip` in the repo is the
one at `neo4j-cli/internal/skill/gen/main_test.go`. The aura-side
counterpart (and its `install_e2e_test.go`) is deleted; there is no
equivalent gate to replace because the neo4j-cli bundle now covers the
entire shipped surface.

### Branch / PR ergonomics

Linear's `gitBranchName` is `cli-135-remove-the-aura-cli-build-config`.
Per the user's global `oskar/` prefix convention, the working branch
should be `oskar/cli-135-remove-the-aura-cli-build-config`. The PR body
should link to the Linear issue and to this PRD.

## Acceptance Criteria

- [ ] `neo4j-cli/aura/cmd/` directory does not exist on disk
- [ ] `neo4j-cli/aura/internal/skill/` directory does not exist on disk
- [ ] `grep -rn 'go:generate' neo4j-cli/aura/` returns zero hits
- [ ] `neo4j-cli/aura/aura.go` no longer imports
      `"github.com/neo4j/cli/common/skill"` or the local
      `internal/skill` package, and no longer contains a `skill.NewCmd`
      call
- [ ] `aura.NewStandaloneCmd` remains defined as an exported function
      in `neo4j-cli/aura/aura.go`
- [ ] `.github/workflows/publish-npm.yml` contains zero occurrences of
      `aura-cli`, `aura_cli`, `INCLUDE_NEO4J`, or `include_neo4j`
- [ ] `.github/workflows/release.yml` contains zero occurrences of
      `include_neo4j`, `INCLUDE_NEO4J`, `aura-cli`, `AURA_CLI_VERSION`,
      or `CHANGELOG-aura`, and the `Write release-meta.json` step
      emits `{"version": "..."}` only
- [ ] `CHANGELOG.md` describes a single binary plus a one-line
      historical pointer to `CHANGELOG-aura.md`
- [ ] `RELEASING.md` contains zero occurrences of `AURA_CLI_VERSION`,
      `include_aura`, or "Aura-cli is not published" prose, and its
      `changie new --projects ...` examples list `neo4j-cli` only
- [ ] `.agents/build.md` contains zero `aura-cli`-specific table rows,
      sections, or example commands
- [ ] `.agents/deployment.md` contains zero "dual-binary" /
      "Dual-Version Injection" / `AURA_CLI_VERSION` prose
- [ ] `.agents/repo-layout.md` no longer references the aura-cli
      generator, `aura.NewStandaloneCmd`'s skill mount, or the
      aura-cli bundle round-trip gate
- [ ] `distribution/npm/README.md` no longer references
      `CHANGELOG-aura.md` or the `include_neo4j` aura-cli skip story
- [ ] `AGENTS.md`'s dual-credential paragraph notes that only one
      bundle now regenerates from credential changes
- [ ] `.changes/unreleased/neo4j-cli-Patch-*.yaml` exists with
      `project: neo4j-cli`, `kind: Patch`, body describing the
      build-config removal
- [ ] `CHANGELOG-aura.md` is byte-identical to its pre-PR state
- [ ] `.changes/aura-cli/v*.md` files are byte-identical to their
      pre-PR state
- [ ] `make build` exits 0 and produces only `bin/neo4j-cli`
- [ ] `make test` exits 0
- [ ] `make lint` exits 0
- [ ] `make fmt-check` exits 0
- [ ] `make generate-check` exits 0
- [ ] `make license-check` exits 0
- [ ] `make snapshot` exits 0 and `bin/aura-cli` is absent
- [ ] `go build ./...` exits 0
- [ ] Final grep gate (REQ-NF-009) passes
- [ ] Branch `oskar/cli-135-remove-the-aura-cli-build-config` is pushed
      and a PR is opened linking back to Linear CLI-135 and this PRD

## Out of Scope

- Deleting `CHANGELOG-aura.md` or `.changes/aura-cli/v*.md` history.
- Deleting `neo4j-cli/aura/internal/subcommands/credential/`
  (the standalone-tree credential package that
  `NewStandaloneCmd` still mounts).
- Deleting or merging `aura.NewStandaloneCmd` into `aura.NewCmd`.
- Reviving npm or Homebrew publication for aura-cli.
- Any user-facing change to `neo4j-cli`'s CLI surface, exit codes,
  output formats, or flag set.
- Refactoring the neo4j-cli skill bundle subsystem
  (`neo4j-cli/internal/skill/`).
- Touching `CHANGELOG-neo4j.md`.

## Open Questions

None.
