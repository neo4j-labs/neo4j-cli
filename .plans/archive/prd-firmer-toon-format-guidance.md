# PRD: Firmer toon-format Guidance in Skill Bundles & Help Text

## Overview

The neo4j-cli and aura-cli skill bundles currently say "Prefer `--format toon` ... when the output will be read by an LLM or agent". In practice agents still default to JSON or table. This change strengthens the guidance to a directive ("Always pass `--format toon` on read commands") in the skill bundles, and adds a short `(agents: prefer toon)` nudge to the `--format` flag's help text so the signal also appears in live `--help` output and every generated reference page.

No runtime behaviour changes — `--format toon` already exists in `common/clicfg/clicfg.go::ValidFormatValues` and `common/output` already routes it. Default format stays `default`.

## Goals

- Make agents default to `--format toon` for read commands without the user needing to ask.
- Surface the toon nudge in the single source of truth for the `--format` flag (`common/flags/flags.go`) so it propagates to `--help`, the SKILL.md Global Flags table, and every `references/*.md` reference page.
- Keep both skill bundles (`neo4j-cli` and `aura-cli`) in sync.
- Ship a user-facing changelog entry per repo convention.

## Non-Goals

- No change to runtime default format (stays `default`).
- No change to `ValidFormatValues` or to format rendering logic.
- No change to write-command output (still confirmation text only, `--format` ignored).
- Not converting the aura-cli skill bundle into a shipped binary (aura-cli build was retired; bundle still regenerates, that's the only reason it's touched).
- No telemetry or measurement of toon adoption.

## Requirements

### Functional Requirements

- REQ-F-001: Update `common/flags/flags.go` line ~23 so the `--format` flag description ends with `(agents: prefer toon)`. Final string format: `Format to print console output in, from a choice of [<values>]. (agents: prefer toon)`.
- REQ-F-002: In `neo4j-cli/internal/skill/additions.md`, keep "All read commands accept `--format json|table|toon` (shorthand `-f`). Write commands print confirmation text only; pipe-friendly output requires explicit `--format json` where supported." as a factual bullet, then add a directive bullet immediately after: **Always pass `--format toon` (`-f toon`) on read commands** — toon uses ~40% fewer tokens than JSON while encoding the same data, so default to it for every list/get/show command. Only use `--format json` when piping into a JSON-aware tool that requires it; only use `--format table` when the user explicitly asks for a human-readable table.
- REQ-F-003: Apply the same two-bullet structure (factual + directive) in `neo4j-cli/aura/internal/skill/additions.md` at the corresponding position.
- REQ-F-004: Regenerate `bundle/SKILL.md` and `references/*.md` in both `neo4j-cli/internal/skill/` and `neo4j-cli/aura/internal/skill/` via `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...` so the new flag description and gotcha bullets are baked into the committed bundles.
- REQ-F-005: Add a `changie` entry under `.changes/unreleased/` for project `neo4j-cli`, kind `Patch`, body roughly "Nudge agents toward `--format toon` in help text and skill bundles."

### Non-Functional Requirements

- REQ-NF-001: `make generate-check` must be clean — committed bundles match regenerated output.
- REQ-NF-002: `make test` must pass — `TestGenerator_RoundTrip` re-asserts bundle equivalence; no other tests should regress.
- REQ-NF-003: `make fmt-check` and `make lint` must pass.
- REQ-NF-004: Default format behaviour is unchanged — bare `neo4j-cli aura instance list` (no `--format` flag) still prints in `default` mode.
- REQ-NF-005: Cross-platform compatibility unchanged — touched files are all `.go` and `.md`; no path-separator or LF/CRLF concerns beyond what `.gitattributes` already covers (`**/internal/skill/bundle/**`, `**/internal/skill/additions.md`).

## Technical Considerations

### Single source of truth for the flag description

The `--format` flag is registered in one place, `common/flags/flags.go::RegisterOutputFlag` (line ~23), via:

```go
fmt.Sprintf("Format to print console output in, from a choice of [%s]", strings.Join(clicfg.ValidFormatValues[:], ", "))
```

A change here propagates automatically to:

- Live `cmd --help` output for every command that mounts the persistent flag
- Generated `references/*.md` flag tables (e.g. `references/query.md`, `references/skill.md`)
- The `Global Flags` table in `bundle/SKILL.md` for both binaries

That single edit is preferable to per-page hand edits, and the existing `go generate` pipeline already picks it up.

### Generation pipeline

CLAUDE.md `Makefile Notes` documents the generation contract:

- `make generate` runs `go generate ./...`
- `make generate-check` is a CI gate (regenerate + `git diff --exit-code`)
- Editing `additions.md` OR changing the `--format` flag help text requires re-running `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`
- `TestGenerator_RoundTrip` is the gate that catches stale bundles

### Aura skill bundle still regenerates

Per CLAUDE.md `Architecture` notes, `neo4j-cli/aura/internal/skill/` template still exists and regenerates cleanly even though no Make target builds the standalone aura-cli binary. Touching it keeps the two bundles in sync and avoids a generate-check delta if a future build is restored.

### Changie entry

`.changie.yaml` defines kinds `Major | Minor | Patch`. CLAUDE.md `Changie Notes` warns this repo only has the `neo4j-cli` project key (the `aura-cli` project was removed). Non-interactive form per CLAUDE.md:

```bash
changie new --projects neo4j-cli --kind Patch --body "Nudge agents toward --format toon in help text and skill bundles."
```

If `changie` isn't installed locally, hand-author a `.changes/unreleased/neo4j-cli-Patch-<YYYYMMDD>-<HHMMSS>.yaml` per the documented schema.

### Wording precision

- Flag-help nudge: `(agents: prefer toon)` — short enough to not overflow `--help` formatting, distinct enough that an agent grep'ing flag descriptions won't miss it.
- The `%` character inside `Sprintf` does NOT need escaping in this implementation because the chosen wording contains no literal percent. (The plan's earlier alternative wording with `~40%` would have required `%%`; this PRD's chosen wording avoids that concern.)

### Risks / pitfalls

- A stale committed bundle is the most likely failure mode — `make generate-check` and `TestGenerator_RoundTrip` both gate it. Run `go generate` once after every source change.
- `.gitattributes` enforces LF on `**/internal/skill/additions.md` and `**/internal/skill/bundle/**`; CRLF on Windows checkout would break byte-equal golden tests. No new files are added, so existing rules cover this.
- Avoid drift between the two `additions.md` files — mirror the wording exactly except for binary name in any binary-specific bullets (the toon bullet has no binary-specific content, so wording is identical across both files).

## Acceptance Criteria

- [ ] `common/flags/flags.go` flag description ends with `(agents: prefer toon)`.
- [ ] `bin/neo4j-cli aura instance list --help` shows the new flag description with the agents-prefer-toon nudge.
- [ ] Both `additions.md` files contain the factual bullet and the new directive bullet in that order, with the wording specified in REQ-F-002.
- [ ] Both `bundle/SKILL.md` Gotchas sections show the factual + directive bullet pair.
- [ ] Both `bundle/SKILL.md` Global Flags tables and at least one `references/*.md` per binary show the updated flag description.
- [ ] `.changes/unreleased/` contains a new `neo4j-cli-Patch-*.yaml` entry with the documented body.
- [ ] `make generate-check` is clean.
- [ ] `make test` passes (incl. `TestGenerator_RoundTrip`).
- [ ] `make fmt-check` and `make lint` pass.
- [ ] `git diff --stat` is bounded to: `common/flags/flags.go`, `neo4j-cli/internal/skill/additions.md`, `neo4j-cli/aura/internal/skill/additions.md`, regenerated `bundle/SKILL.md` + `references/*.md` in both skill trees, and one new `.changes/unreleased/*.yaml` file.

## Out of Scope

- Changing the runtime default format from `default` to `toon`.
- Modifying `ValidFormatValues` or adding new formats.
- Editing `description.txt` for either skill bundle.
- Per-command flag-description overrides (the global string serves all commands uniformly).
- Updating documentation outside the skill bundles (e.g. `CONTRIBUTING.md`, README) — none of those reference the toon preference today.
- Adding a "preferred" hint to the `--format` enum values themselves.

## Open Questions

None — Q1–Q5 resolved during PRD intake:

- Q1 (flag-help wording): `(agents: prefer toon)`.
- Q2 (additions.md structure): two bullets — factual first, directive second.
- Q3 (default format change): no — docs-only.
- Q4 (scope): both bundles.
- Q5 (changelog): yes — `Patch` kind.
