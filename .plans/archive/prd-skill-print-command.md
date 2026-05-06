# PRD: `skill print` command

Linear: https://linear.app/neo4j/issue/CLI-37
Branch: `oskar/cli-37-add-neo4j-cli-skill-print-command`

## Overview

Add a new leaf command `skill print` to the existing `common/skill/`
cobra tree. It writes the embedded `SKILL.md` from the binary's bundle
to stdout so users can preview the skill markdown before running
`skill install`. Originated in a Slack ask from Michael Hunger:

> It would be nice to have a `neo4j-cli skill show` to show the skill
> markdown before I install it :)

The Linear issue title canonicalises this as `print` (we ship `print`,
not `show`). Both shipped binaries — `neo4j-cli` (super-CLI) and
standalone `aura-cli` — pick up the leaf for free because both already
mount the shared command tree via `skill.NewCmd(cfg, binskill.Bundle,
"<name>")`.

## Goals

- Let users preview the bundled `SKILL.md` content without writing
  anything to disk.
- Reuse the established `common/skill/` cobra layout (one file per leaf,
  colocated test, generator-driven reference docs).
- Ship via the existing dual-project changie flow as a `Minor` user-
  facing addition.

## Non-Goals

- No previewing of `references/*.md` (the skill's sub-pages). SKILL.md
  is the entry-point; sub-pages are out of scope for v1.
- No `{{VERSION}}` substitution. Output the raw bundle bytes verbatim;
  install-time substitution stays in `installer.go`.
- No new flags (`--format` is registered on the `skill` parent and is
  silently ignored by this leaf — markdown is the only sensible
  output).
- No positional args (`cobra.NoArgs`).
- No new public exports from `common/skill/` — the leaf reads the
  bundle directly and prints; no helper-function refactor.

## Requirements

### Functional Requirements

- REQ-F-001: A new file `common/skill/print.go` MUST define
  `newPrintCmd(cfg *clicfg.Config, bundle fs.FS, skillName string)
  *cobra.Command` returning a leaf with `Use: "print"`,
  `Args: cobra.NoArgs`, and a `RunE` that reads `SKILL.md` from
  `bundle` via `fs.ReadFile` and writes it to `cmd.OutOrStdout()` via
  `cmd.Print`.
- REQ-F-002: `cmd.SilenceUsage = true` MUST be set inside `RunE`
  (matches existing leaves so unrelated `RunE` errors don't dump
  cobra usage on the user).
- REQ-F-003: The `{{VERSION}}` placeholder in SKILL.md MUST appear
  literally in the output (no substitution).
- REQ-F-004: `common/skill/skill.go::NewCmd` MUST mount the leaf with
  one new line: `cmd.AddCommand(newPrintCmd(cfg, bundle, skillName))`,
  placed alongside the existing `install`/`remove`/`list`/`check`
  mounts (file kept ≤80 lines per the repo's cobra-layout convention).
- REQ-F-005: A colocated test file `common/skill/print_test.go` MUST
  cover at minimum (a) raw SKILL.md output with `{{VERSION}}` literal
  preserved, and (b) rejection of any positional argument.
- REQ-F-006: `make generate` MUST regenerate
  `neo4j-cli/internal/skill/bundle/references/skill.md` AND
  `neo4j-cli/aura/internal/skill/bundle/references/skill.md` to include
  the new `## <bin> skill print` section (the generator walks the
  cobra tree, so this happens automatically once the leaf is mounted).
- REQ-F-007: Two changie entries MUST land under
  `.changes/unreleased/`, both `kind: Minor`:
  - `project: aura-cli`
  - `project: neo4j-cli`

  Body (both): `Add 'skill print' command to preview the embedded
  SKILL.md before installing`.
- REQ-F-008: Copyright header MUST be present at the top of both
  new `.go` files (`make license-check` enforces this).

### Non-Functional Requirements

- REQ-NF-001: `make test` passes — incl. new `print_test.go`.
- REQ-NF-002: `make fmt-check` passes (gofmt clean).
- REQ-NF-003: `make lint` passes (golangci-lint v2).
- REQ-NF-004: `make license-check` passes.
- REQ-NF-005: `make generate-check` passes (the CI gate that runs
  `go generate ./...` and asserts `git diff --exit-code`).
- REQ-NF-006: No new dependencies in `go.mod`.

## Technical Considerations

**Files touched:**

- `common/skill/print.go` *(new)*
- `common/skill/print_test.go` *(new)*
- `common/skill/skill.go` *(one-line addition)*
- `neo4j-cli/internal/skill/bundle/references/skill.md` *(regenerated)*
- `neo4j-cli/aura/internal/skill/bundle/references/skill.md`
  *(regenerated)*
- `.changes/unreleased/aura-cli-Minor-<ts>.yaml` *(new)*
- `.changes/unreleased/neo4j-cli-Minor-<ts>.yaml` *(new)*

**Reused infrastructure:**

- Bundle access: `fs.ReadFile(bundle, "SKILL.md")` — same call style
  as `installer.go::copyBundleWithVersion` at `installer.go:250`.
- Output: `cmd.Print` writes to `cmd.OutOrStdout()`, which test
  fixtures already capture via `cmd.SetOut(buf)`.
- Test fixture: existing `newFixture`/`fixtureBundle` helpers in
  `common/skill/cmd_helpers_test.go:25-91`. The fixture's stub
  SKILL.md already contains `version: {{VERSION}}` so the placeholder
  assertion is straightforward.
- Mount points: no edits to `neo4j-cli/app/app.go` or
  `neo4j-cli/aura/aura.go` required — both already pass the bundle
  into `skill.NewCmd`.

**Why `print` and not `show`:** Linear issue title is the canonical
spelling. `print` also lines up with classic UNIX verbs (`cat`,
`echo`); `show` collides with future state-display verbs (e.g. `show
config`).

**Why ignore `--format`:** The parent `skill` cmd registers `--format
json|table|toon` (see `skill.go:32`). This leaf outputs raw markdown,
which doesn't have a JSON/table/toon shape. Cobra silently allows
unread persistent flags, so the flag is accepted but has no effect —
acceptable per the principle of least surprise (matches how `install`
also doesn't render a different shape for `--format json`).

**Risks:** None material. The leaf is read-only on the embedded FS;
no host-FS writes, no network, no agent detection. The only failure
mode is `fs.ReadFile` returning an error, which only happens if the
bundle is corrupt at compile time (a build-system regression that
unit tests in the bundle subsystem would catch first).

## Acceptance Criteria

- [ ] `common/skill/print.go` exists, ≤40 lines, copyright header,
      defines `newPrintCmd(cfg, bundle, skillName) *cobra.Command`.
- [ ] `common/skill/skill.go::NewCmd` mounts the new leaf via
      `cmd.AddCommand(newPrintCmd(cfg, bundle, skillName))`.
- [ ] `common/skill/print_test.go` has at least one test asserting
      raw SKILL.md output (incl. `{{VERSION}}` literal) and one test
      asserting positional-arg rejection.
- [ ] `./bin/neo4j-cli skill print` prints SKILL.md to stdout
      containing the literal `{{VERSION}}` placeholder.
- [ ] `./bin/aura-cli skill print` prints aura-cli's SKILL.md to
      stdout (same placeholder behaviour).
- [ ] `./bin/neo4j-cli skill print --help` prints non-empty short and
      long descriptions.
- [ ] `./bin/neo4j-cli skill print extra-arg` exits non-zero.
- [ ] Both `<bin>/internal/skill/bundle/references/skill.md` files
      contain a `## <bin> skill print` section after `make generate`.
- [ ] Two new YAML files exist under `.changes/unreleased/` (one
      per project, kind `Minor`).
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check`,
      `make generate-check` all pass.

## Out of Scope

- Printing or selecting `references/*.md` files (sub-pages of the
  skill).
- Substituting `{{VERSION}}` to preview the post-install bytes
  (could be a `--substitute-version` flag in a follow-up if asked).
- Wrapping the output in a JSON envelope when `--format json` is
  passed.
- Any change to `install`, `remove`, `list`, or `check` semantics.
- Any change to the bundle generator (`render/`, `gen/main.go`).

## Open Questions

None.
