# PRD: CLI-194 — Extend Virtual Graph limitations in query skill

## Overview

The `neo4j-cli query` Claude skill ships a Cypher-authoring prompt (`neo4j-cli/internal/skill/query-additions.md`) with a "Forbidden on Virtual Graphs (today)" list. The list tells the model which Cypher constructs the Aura Virtual Graph engine rejects so the model avoids generating them in the first place. Linear [CLI-194](https://linear.app/neo4j/issue/CLI-194) reports two missing entries; this PRD covers adding them.

## Goals

- Extend the forbidden-construct list with the two missing limitations so the model stops emitting Cypher that Virtual Graphs reject.
- Keep source and generated bundle in lock-step (the existing `make generate-check` / `TestGenerator_RoundTrip` gate).
- Ship as a user-facing patch with a changelog entry.

## Non-Goals

- Restructuring the Virtual Graph section, the Cypher-25 section, or any other unrelated content in `query-additions.md`.
- Editing the runtime `query` command code, the Bolt client, or the EXPLAIN preflight.
- Auditing or expanding the Virtual Graph limitation set beyond the two items called out in CLI-194.
- Mirroring the change anywhere outside the neo4j-cli skill (e.g. external docs, marketing site).

## Requirements

### Functional Requirements

- REQ-F-001: Append `` `OPTIONAL MATCH` `` as a new bullet to the "Forbidden on Virtual Graphs (today)" list in `neo4j-cli/internal/skill/query-additions.md`.
- REQ-F-002: Append `Vector search and full-text search (not supported in any form)` as a new bullet to the same list. (Phrasing finalised — keep terse, no construct backticks since it covers a family.)
- REQ-F-003: Preserve the existing bullet ordering and style; new bullets land at the end of the list, before the "Stick to `MATCH … WHERE … RETURN`…" paragraph.
- REQ-F-004: Regenerate the skill bundle so `neo4j-cli/internal/skill/bundle/query-additions.md` is a verbatim copy of the updated source. Use `go generate ./neo4j-cli/internal/skill/...` (per `embed.go`'s `//go:generate` directive driven by `gen/main.go`).
- REQ-F-005: Add a changie `Patch` entry under `neo4j-cli` describing the prompt change (user-facing skill content).

### Non-Functional Requirements

- REQ-NF-001: `make generate-check` must pass — source and bundle in sync.
- REQ-NF-002: `make test` must pass; `TestGenerator_RoundTrip` is the explicit gate for bundle drift.
- REQ-NF-003: `make fmt-check` and `make lint` must pass (standard CI gates per AGENTS.md).
- REQ-NF-004: Branch named `oskar/cli-194-update-query-cypher-skill-with-more-limitations` (Linear-suggested name with the `oskar/` prefix mandated by global instructions).

## Technical Considerations

- **Source of truth**: `neo4j-cli/internal/skill/query-additions.md`. The generator at `neo4j-cli/internal/skill/gen/main.go` copies it verbatim into `bundle/query-additions.md`. Edit source only; never hand-edit the bundle.
- **Bundle gate**: `TestGenerator_RoundTrip` (and the `make generate-check` CI step) compare bundle to a fresh generate. The single `go generate` invocation refreshes everything needed.
- **No other surfaces**: The Virtual Graph limitations list lives only in `query-additions.md` (and its generated copy). No README, SKILL.md, or `description.txt` references duplicate the list.
- **Scope of change**: Two added lines plus the regenerated bundle file. No Go code, no flags, no command-tree edits, so no `agentcontext` or cobra-help regen beyond the standard `go generate`.
- **Changelog**: Kind `Patch` (small user-visible content tweak, not a feature or fix of a bug in shipped code). Use `changie new --projects neo4j-cli --kind Patch --body "..."`. If changie isn't on PATH, hand-author the YAML under `.changes/unreleased/` (per AGENTS.md fallback).

## Acceptance Criteria

- [ ] `neo4j-cli/internal/skill/query-additions.md` "Forbidden on Virtual Graphs (today)" list contains `` `OPTIONAL MATCH` `` and `Vector search and full-text search (not supported in any form)` as the last two bullets.
- [ ] `neo4j-cli/internal/skill/bundle/query-additions.md` mirrors the source verbatim (regenerated via `go generate`).
- [ ] `rg "OPTIONAL MATCH" neo4j-cli/internal/skill/` returns hits in both `query-additions.md` and `bundle/query-additions.md`.
- [ ] `rg "full-text search" neo4j-cli/internal/skill/` returns hits in both files.
- [ ] `make generate-check` passes against a clean tree.
- [ ] `make test`, `make fmt-check`, `make lint` all pass.
- [ ] A `Patch` changie entry exists under `.changes/unreleased/` for project `neo4j-cli` referencing the change.
- [ ] Changes committed on branch `oskar/cli-194-update-query-cypher-skill-with-more-limitations`.

## Out of Scope

- Reordering, rewording, or pruning existing bullets in the limitations list.
- Updating the Cypher 25 section to mention that vector indexes are unusable on Virtual Graphs (the limitation list already conveys this).
- Tightening or relaxing the Virtual Graph detection logic in `:schema`.
- Any agent-side documentation outside the skill bundle (web docs, blog posts, Aura docs).

## Open Questions

None.
