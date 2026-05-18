# PRD: CLI-104 — Cleanup `--help` query/EXPLAIN sentence leak

## Overview

The `Long:` field on the root `neo4j-cli` command and on the `aura` parent command both contain a sentence about `query run` / EXPLAIN preflight / `--rw` gating:

> `` `query run` under neo4j-cli runs EXPLAIN first when --rw is not set and blocks statements classified as writes. ``

That sentence is parked on the wrong commands. `aura` has no `query` child, so the line is meaningless under `neo4j-cli aura --help`. Root is technically a parent of `query`, but the issue (CLI-104) makes the policy call explicit: the sentence should appear **only** under `neo4j-cli query --help`.

The fix is a three-line `Long` edit plus a skill-bundle regen and a changelog entry. Cobra does not propagate `Long` to children, so deeper subcommands (`aura tenant --help`, `aura instance --help`, …) are already clean — verified by running `go run ./neo4j-cli aura tenant --help`. The issue title's `tentant` is a typo for `aura --help`.

Linear: https://linear.app/neo4j/issue/CLI-104.

## Goals

- Remove the query/EXPLAIN/`--rw`-preflight sentence from the `Long:` of `neo4j-cli/app/app.go` (root) and `neo4j-cli/aura/aura.go` (aura parent).
- Add an equivalent sentence to the `Long:` of `neo4j-cli/query/query.go` so the gate stays documented exactly where it applies.
- Regenerate the two skill bundles so `SKILL.md` / `references/aura.md` / `references/query.md` match the new help text — keeps the `TestGenerator_RoundTrip` gate green.
- Ship a `Patch`-kind changelog entry — `--help` output is user-facing.

## Non-Goals

- Rewriting the `Short:` strings on any of the three commands.
- Restructuring the `query` command's existing `Long:` paragraph (`--param`/`:embed` description stays as-is; the new sentence is appended).
- Changing the `--rw` flag's help text in `common/flags/flags.go` — the flag already self-documents.
- Removing the global `Write operations require --rw.` sentence from root / aura `Long:` — it stays as a top-level gate hint, only the query-specific clause is stripped.
- Touching deeper subcommands (`aura tenant`, `aura instance`, `query :embed`, `query :schema`, …); their `--help` is already clean.
- Editing the standalone `aura-cli` `NewStandaloneCmd` path in `neo4j-cli/aura/aura.go` beyond what `NewCmd` already shares (the `Long:` is set on the shared `NewCmd`, so one edit covers both).
- Editing the historical `neo4j-cli/aura/cmd/main.go` standalone entrypoint (compiled but not shipped).

## Requirements

### Functional Requirements

- **REQ-F-001**: `neo4j-cli/app/app.go:36` `Long:` is changed to: `"Allows you to manage Neo4j resources. Write operations require --rw."` — the trailing clause about `query` / EXPLAIN / `--rw`-preflight is removed.
- **REQ-F-002**: `neo4j-cli/aura/aura.go:29` `Long:` is changed to: `"Allows you to programmatically provision and manage your Aura resources. Write operations require --rw."` — same trailing clause removed.
- **REQ-F-003**: `neo4j-cli/query/query.go` `Long:` (currently lines 22–29) appends one new sentence describing the gate:
  > `"Write operations require `--rw`; without `--rw`, an EXPLAIN preflight runs first and statements classified as writes are blocked."`
  Place the new sentence at the end of the existing concatenated `Long:` (after the `query :embed` sentence) so the established `--param` / `:embed` narrative stays unchanged.
- **REQ-F-004**: Wording uses single backticks around `--rw` and `EXPLAIN` only where they appear today in the surrounding `Long:` (no Markdown changes to the existing text). The new sentence uses backticks around `--rw` to match the existing convention in `neo4j-cli/query/query.go:25-27`.
- **REQ-F-005**: After source edits, `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...` is run, and the resulting diffs in `neo4j-cli/internal/skill/bundle/**` and `neo4j-cli/aura/internal/skill/bundle/**` are committed in the same change. Expected drift:
  - `neo4j-cli/internal/skill/bundle/SKILL.md` (root Long change)
  - `neo4j-cli/internal/skill/bundle/references/aura.md` (aura Long change — neo4j-cli bundle reflects aura subtree)
  - `neo4j-cli/internal/skill/bundle/references/query.md` (query Long change)
  - `neo4j-cli/aura/internal/skill/bundle/SKILL.md` (aura Long change — aura standalone bundle)
- **REQ-F-006**: A single `Patch`-kind changelog entry is added under `.changes/unreleased/` for the `neo4j-cli` project, body: `"fix(cli): scope query/EXPLAIN --rw note to `query --help` only (CLI-104)"`. Authored either via `changie new --projects neo4j-cli --kind Patch --body "<body>"` or by hand-authoring the YAML per `.agents/build.md` if changie isn't installed.
- **REQ-F-007**: No `--help` output anywhere in the live `app.NewCmd(cfg)` tree (other than under `neo4j-cli query …`) mentions `EXPLAIN`, `EXPLAIN preflight`, or `runs EXPLAIN`. Verified by `go run ./neo4j-cli --help`, `go run ./neo4j-cli aura --help`, and walking deeper subcommands.

### Non-Functional Requirements

- **REQ-NF-001**: `make test`, `make fmt-check`, and `make lint` all pass after the change (final gate per `AGENTS.md`). `TestGenerator_RoundTrip` in particular must stay green — it is the bundle-drift detector for `Long:` edits.
- **REQ-NF-002**: No new test files are added — the fix is a string edit on three `Long:` fields. The existing skill-bundle round-trip test (`TestGenerator_RoundTrip`) already gates the source-bundle correspondence, so it covers REQ-F-005 implicitly.
- **REQ-NF-003**: The change is platform-agnostic (pure Go string + generated Markdown). No Windows / Unix-specific behaviour involved.

## Technical Considerations

### Why the leak happens

- Cobra's default help template falls back to `Short:` when `Long:` is empty, but does **not** inherit `Long:` from a parent. So `neo4j-cli aura tenant --help` is already clean (tenant has no `Long:`, falls back to `Short:`). The leak is purely on the two commands that explicitly set the wrong `Long:` — `app.go` root and `aura.go` aura parent.
- The text was added in an earlier `--rw`-related refactor and ended up on the parent commands as a discoverability shortcut. CLI-104 calls that out as noise and re-scopes it.

### Bundle-regen gate

- Per `AGENTS.md` "Cobra Help / Skill Bundle Rendering Notes": any `Long:` change in `neo4j-cli/internal/subcommands/credential/...` or `neo4j-cli/query/...` requires `go generate ./neo4j-cli/internal/skill/...`. By extension, changes to `app.go` (root, included in the neo4j-cli bundle's `SKILL.md`) and `aura.go` (included in the aura standalone bundle's `SKILL.md`, and in the neo4j-cli bundle as `references/aura.md`) also require regeneration. Skipping it fails `TestGenerator_RoundTrip` with a "references/<sub>.md differs" message.
- The `query.go` `Long:` is rendered into `neo4j-cli/internal/skill/bundle/references/query.md` (no aura-side analog, since `query` doesn't exist in the aura standalone tree). So `go generate ./neo4j-cli/internal/skill/...` is required for REQ-F-003; `go generate ./neo4j-cli/aura/internal/skill/...` is required for REQ-F-002. Running both is the safe default.

### Files touched

- `neo4j-cli/app/app.go` (1 line — `Long:` string)
- `neo4j-cli/aura/aura.go` (1 line — `Long:` string)
- `neo4j-cli/query/query.go` (one new sentence appended to the existing string-concatenated `Long:`)
- `neo4j-cli/internal/skill/bundle/SKILL.md` (regenerated)
- `neo4j-cli/internal/skill/bundle/references/aura.md` (regenerated)
- `neo4j-cli/internal/skill/bundle/references/query.md` (regenerated)
- `neo4j-cli/aura/internal/skill/bundle/SKILL.md` (regenerated)
- `.changes/unreleased/neo4j-cli-Patch-<YYYYMMDD>-<HHMMSS>.yaml` (new)

### Risk surface

- Zero behavioural change at runtime. The only observable diff is help-text wording in three rendered surfaces (`--help`, skill bundle Markdown, `agent-context` JSON if it surfaces `Long:` — verify in acceptance criteria).
- No test fixtures pin the exact strings — confirmed by `grep -rn "runs EXPLAIN first when --rw"` returning only the two source `Long:` lines and the three generated bundle files.

## Acceptance Criteria

- [ ] `neo4j-cli/app/app.go:36` `Long:` matches REQ-F-001.
- [ ] `neo4j-cli/aura/aura.go:29` `Long:` matches REQ-F-002.
- [ ] `neo4j-cli/query/query.go` `Long:` has the new gate sentence appended per REQ-F-003.
- [ ] `go run ./neo4j-cli --help` does **not** contain the substring `EXPLAIN`.
- [ ] `go run ./neo4j-cli aura --help` does **not** contain the substring `EXPLAIN`.
- [ ] `go run ./neo4j-cli aura tenant --help` is byte-identical (modulo trailing newline) to its pre-change output — i.e. unchanged.
- [ ] `go run ./neo4j-cli query --help` contains the new sentence and reads naturally as a continuation of the existing paragraph.
- [ ] `make test` passes, including `TestGenerator_RoundTrip`.
- [ ] `make fmt-check` reports no files needing `gofmt`.
- [ ] `make lint` is clean.
- [ ] A new `.changes/unreleased/neo4j-cli-Patch-*.yaml` is present with the body from REQ-F-006.
- [ ] Bundle files listed under REQ-F-005 are committed alongside the source edits in the same commit / PR.
- [ ] `grep -rn "runs EXPLAIN first when --rw" .` returns no hits outside `neo4j-cli/query/**` (source + bundle).

## Out of Scope

- Re-wording the `--rw` flag's own usage string in `common/flags/flags.go`.
- Splitting the new sentence across multiple paragraphs in `query.go` — single appended sentence is sufficient.
- Adding a regression test that walks the cobra tree and asserts `EXPLAIN` only appears under `query` — the existing `TestGenerator_RoundTrip` already catches bundle drift, and a custom no-`EXPLAIN`-outside-query gate would be over-engineering for a one-line string fix.
- Updating `README.md`, `CONTRIBUTING.md`, or any prose docs — the sentence isn't surfaced there.
- Reviewing other `Long:` fields for similar cross-command leakage. If the user wants a broader sweep, that's a separate PRD.

## Open Questions

- None. The disposition question (move vs delete vs keep-on-root) was resolved during planning: **move to `query --help` only**.
