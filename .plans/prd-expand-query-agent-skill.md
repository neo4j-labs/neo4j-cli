# PRD: Expand neo4j-cli query agent skill with porting guidance from neo4j-query

Linear: [CLI-77](https://linear.app/neo4j/issue/CLI-77/update-query-sub-skill-with-useful-information)
Source plan: `/Users/oskarhane/.claude/plans/the-agent-skill-for-wondrous-neumann.md`

## Overview

Port the rich agent-facing guidance from `../neo4j-query/skills/neo4j-query/SKILL.md` into neo4j-cli's bundled agent skill, adapted to neo4j-cli's flag/command surface and `:schema` output shape. The current SKILL.md only has 16 bullet-form gotchas; agents lack the schema-first workflow, parameter usage examples, embeddings playbook, and Cypher 25 vs Cypher 5 syntax tips that the standalone `neo4j-query` skill carries. Land the new guidance as a separate `query-additions.md` companion document inside the bundle so agents can be pointed at it as required pre-reading from SKILL.md, and rename the bundle's `## Gotchas` heading to `## Tips & Gotchas` so the broader content fits semantically.

## Goals

- Give agents using `neo4j-cli query` first-class guidance on schema-first workflow, parameter passing, embeddings, and Cypher dialect selection — at parity with the standalone `neo4j-query` skill.
- Adapt all content to neo4j-cli's actual surface: Bolt URIs, `--param` (no `-P` shorthand), `query :schema` / `query :embed` colon-prefixed leaves, `--rw` + EXPLAIN preflight, `--credential` for stored connections, `--format toon` as the agent default, and the neo4j-cli `:schema` shape (`database.default_language`, flat `nodes`/`relationships`, separate `relationship_paths`).
- Keep the new guidance in its own file (`query-additions.md`) so SKILL.md stays a top-level overview and agents have a stable pointer to deeper guidance.
- Rename `## Gotchas` → `## Tips & Gotchas` globally so the section title reflects mixed gotcha + guidance content; regenerate both the neo4j-cli and aura bundles.
- Keep generator API stable: gen/main.go does the extra file-write work; `render.Options` does not gain new fields.
- Ship the change with a changie Patch entry so users see the skill enrichment in the release notes.

## Non-Goals

- No changes to cobra `Long` strings on `query`, `query :schema`, `query :embed`, or `--help` output. The auto-generated `bundle/references/query.md` stays as-is.
- No port of Ollama/HuggingFace env-var setup examples from the source SKILL.md — existing gotcha #11 already covers config precedence + per-provider API key fallback.
- No restating of `--env` walk-up or stored-credential precedence in the new "Running queries" section — gotcha #11 covers it.
- No expansion of the aura standalone skill's own gotchas (a similar split could help there, but is out of scope).
- No reshaping of the `render.Options` API to introduce per-command additions or a generic `ExtraFiles` slot — the gen/main.go-side write is enough for this change.

## Requirements

### Functional Requirements

- REQ-F-001: Rename the SKILL.md section heading emitted by `common/skill/render/render.go` from `## Gotchas` to `## Tips & Gotchas` (and update the matching comments at lines 6, 50–52, 105, 149).
- REQ-F-002: Add a new hand-authored file `neo4j-cli/internal/skill/query-additions.md` containing the ported guidance, with these top-level sections:
  - `# Query usage guidance` (H1 intro: file is required reading for agents before using `neo4j-cli query`).
  - `## Schema-first workflow` — CRITICAL: run `neo4j-cli query :schema -f toon` first; don't guess labels/rels/props. Skip when user supplies Cypher directly.
  - `## What `:schema` returns` — describe neo4j-cli's actual shape: `database{name, versions[], edition, default_language}`; `nodes[]` (flat one-row-per-property: `nodeType`, `nodeLabels[]`, `propertyName`, `propertyTypes[]`, `mandatory`); `relationships[]` (flat: `relType`, `propertyName`, `propertyTypes[]`, `mandatory`); `relationship_paths[]` (`relType`, `from[]`, `to[]`); `indexes[]`; `constraints[]`. Call out `database.default_language` as the hint for Cypher dialect selection.
  - `## Running queries` — minimal examples: positional cypher, stdin pipe, `--credential <name>`, `--param k=v`, `--format toon` (agent default).
  - `## Handling user requests` — flow: (a) user supplies Cypher → run it; (b) data question without Cypher → `:schema` first; (c) write/modify/delete intent → ask before adding `--rw`.
  - `## Parameters` — `--param NAME=VALUE` repeats; JSON-typed when value parses as JSON, string otherwise; cross-link to Embeddings for the `:embed` modifier.
  - `## Embeddings & vector search` — opt-in via `--embed-provider`/`--embed-model` (or env / stored credential / `--embed-credential <name>`). Two examples: vector search via `--param q:embed='...'`; standalone preview via `neo4j-cli query :embed "text"`. Do not restate the full env-var precedence (gotcha #11 covers it).
  - `## Cypher 25 vs Cypher 5 syntax` — vector-index query contrast: Cypher 5 (`CALL db.index.vector.queryNodes('name', k, $q) YIELD node, score`) vs Cypher 25 (`SEARCH n IN (VECTOR INDEX name FOR n.embedding LIMIT k) SCORE AS score`). Direct agents to read `database.default_language` from `:schema` to pick. Note `CALL` form is deprecated in Cypher 25 but still works; start with the Cypher 5 form when dialect unknown.
  - `## Tips` — bullet list adapted to neo4j-cli flags: prefer `-f toon`; use `LIMIT` for exploration; prefer `--param` over string interpolation; relationship directions matter (check `relationship_paths.from` → `to`); property types drive comparison choices; `--truncate-arrays-over N` hides embedding vectors (default 100, 0 disables); `--max-rows N` caps output (default 100, sets `truncated=true` in JSON).
- REQ-F-003: Update `neo4j-cli/internal/skill/gen/main.go` to read `query-additions.md` from the source skill directory after `render.Bundle(...)` returns, and add it to the output file map under key `query-additions.md` so the existing write loop emits `bundle/query-additions.md`.
- REQ-F-004: Update `neo4j-cli/internal/skill/additions.md` to add ONE new bullet at the top of the existing list, flagging `query-additions.md` as required pre-reading before using `neo4j-cli query`. Wording: ``- **Before using `neo4j-cli query`, read [query-additions.md](query-additions.md) — required pre-reading covering schema-first workflow, parameters, embeddings, Cypher 25 vs 5, and tips.**``
- REQ-F-005: Regenerate both bundles via `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...`. Expected committed diffs: both `bundle/SKILL.md` files (heading rename + new top bullet in neo4j-cli only) plus new file `neo4j-cli/internal/skill/bundle/query-additions.md`.
- REQ-F-006: Update any render-package golden testdata that pins the literal string `## Gotchas` to use `## Tips & Gotchas` instead. (`common/skill/render/testdata/**` is LF-pinned; verify byte-equal goldens after rename.)
- REQ-F-007: Add a changie Patch entry via `changie new --projects neo4j-cli --kind Patch --body "Expand the query subcommand's agent skill with schema-first workflow, parameter usage, embeddings, and Cypher 25 vs Cypher 5 syntax guidance via a new query-additions.md companion document."`. If `changie` isn't installed locally, hand-author the YAML at `.changes/unreleased/neo4j-cli-Patch-<YYYYMMDD>-<HHMMSS>.yaml` with the documented schema (`project`, `kind`, `body`, `time` RFC3339, single-quoted body).
- REQ-F-008: Verify that the repo-wide bundle gate `common/skill/bundles_test.go::TestCommittedBundlesAndTestdataAreLF` includes the new top-level `bundle/query-additions.md` (the walk currently covers `**/internal/skill/bundle/**`). If `TestGenerator_RoundTrip` only walks files produced by `render.Bundle()`, extend it (or the equivalent generator test) to cover the gen-side-only file too, so future drift between `query-additions.md` source and `bundle/query-additions.md` is caught.

### Non-Functional Requirements

- REQ-NF-001: All committed bundle content is LF-line-ended. The repo-root `.gitattributes` already covers `**/internal/skill/bundle/**` and `**/internal/skill/additions.md`; if a new path needs explicit pinning (e.g. `**/internal/skill/query-additions.md`), add an entry so Windows CI checkouts don't rewrite to CRLF.
- REQ-NF-002: Generated content stays deterministic — no map-iteration ordering issues in the gen-side write. The file-name key `query-additions.md` is a fixed string.
- REQ-NF-003: `make test`, `make fmt-check`, `make lint`, and `make generate-check` must all be clean on a fresh checkout after the change. CI runs across linux/macos/windows.
- REQ-NF-004: The new `query-additions.md` body should be concise and scannable — agents read it inline. Target ≤ ~250 lines; avoid duplicating content already in references/query.md or in the existing 16 SKILL.md gotchas.
- REQ-NF-005: Content adaptation must be accurate against neo4j-cli's actual `:schema` shape (not the source's nested shape). No references to `defaultCypherVersion`, `-P`, plain `schema`, or HTTP-only behaviors.

## Technical Considerations

- **Render pipeline contract.** `common/skill/render/render.go` returns `map[string][]byte` for the Bundle. gen/main.go iterates the map and writes each entry under `bundle/`. To add `query-additions.md` without changing the render API, gen/main.go reads the source file and inserts it into that map before the write loop. Single-binary impact — the aura side's `gen/main.go` does not change.
- **Heading rename blast radius.** The `## Gotchas` → `## Tips & Gotchas` change in `common/skill/render/render.go:151` affects both bundles (neo4j-cli and aura standalone) because the package is shared. Regenerate both in the same commit so `make generate-check` stays clean.
- **Golden tests.** `common/skill/render/*_test.go` may pin `## Gotchas` in testdata. Need to verify and update goldens in lockstep. `bundles_test.go::TestCommittedBundlesAndTestdataAreLF` checks LF on `internal/skill/bundle/**`, `internal/skill/additions.md`, `internal/skill/description.txt` — extend the pattern to cover `internal/skill/query-additions.md` if needed.
- **Generator round-trip coverage.** `TestGenerator_RoundTrip` validates that `bundle/` matches what the generator produces. If the test walks the file map returned by `render.Bundle()` it won't see the gen-side-only `query-additions.md`. Two options: (a) extend the test to also re-run gen/main.go's `query-additions.md` copy step, or (b) keep the test as-is and rely on `make generate-check` (which does `git diff --exit-code` after `go generate ./...`) to catch drift. Option (b) is the minimum viable path; option (a) is more honest. Pick during implementation.
- **`:schema` shape mismatch with source.** The source SKILL.md describes `database.defaultCypherVersion`. neo4j-cli emits `database.default_language` (different field name). Content must be re-authored against neo4j-cli's actual struct in `neo4j-cli/query/schema.go` (`databaseInfo{Name, Versions[], Edition, DefaultLanguage}` + flat `Nodes`/`Relationships` + separate `RelationshipPaths`).
- **No cobra Long string edits.** Keeps `--help` output stable and avoids forcing a regen of `references/query.md`, which has its own LF + golden constraints.
- **Linking semantics.** Markdown link `[query-additions.md](query-additions.md)` is a relative path within the installed skill directory. Verify the path renders correctly when an agent reads it from `~/.claude/skills/.../SKILL.md` (relative to SKILL.md's directory; siblings should resolve).
- **Changie kind.** This is a skill-content enrichment (no API, no behavior change). `Patch` is appropriate — the bundle ships in the next patch release alongside any other small fixes.

## Acceptance Criteria

- [ ] `common/skill/render/render.go:151` emits `## Tips & Gotchas`; comments at lines 6, 50–52, 105, 149 updated accordingly.
- [ ] `neo4j-cli/internal/skill/query-additions.md` exists with all eight content sections (Schema-first workflow, What `:schema` returns, Running queries, Handling user requests, Parameters, Embeddings & vector search, Cypher 25 vs Cypher 5 syntax, Tips). All examples reference neo4j-cli flags/commands; no `-P` or plain `schema` references.
- [ ] `neo4j-cli/internal/skill/gen/main.go` writes `bundle/query-additions.md` alongside the render.Bundle output.
- [ ] `neo4j-cli/internal/skill/additions.md` has a new top bullet linking `query-additions.md` as required pre-reading.
- [ ] `neo4j-cli/internal/skill/bundle/SKILL.md` regenerated: top bullet linking to `query-additions.md` is present, heading reads `## Tips & Gotchas`.
- [ ] `neo4j-cli/internal/skill/bundle/query-additions.md` committed with LF line endings.
- [ ] `neo4j-cli/aura/internal/skill/bundle/SKILL.md` regenerated with the renamed heading (no other content changes).
- [ ] Any `common/skill/render/testdata/**` goldens that pinned `## Gotchas` are updated.
- [ ] `.changes/unreleased/neo4j-cli-Patch-*.yaml` entry added describing the skill enrichment.
- [ ] `make test`, `make fmt-check`, `make lint`, `make generate-check` all clean.
- [ ] Manual eyeball of `bundle/SKILL.md` and `bundle/query-additions.md`: rendering is clean, code fences intact, no source-form artifacts.

## Out of Scope

- Restructuring `render.Options` to support per-command additions or a generic `ExtraFiles` slot.
- Editing cobra `Short`/`Long` strings on the `query`, `query :schema`, or `query :embed` commands.
- Adding new commands or flags to the `query` subtree.
- Splitting aura standalone bundle additions similarly.
- Porting source SKILL.md's Ollama/HuggingFace env-var examples or `:schema` `defaultCypherVersion` references.
- Documentation updates outside the agent skill bundle (README, CONTRIBUTING, `.agents/*.md`).

## Open Questions

- Does `TestGenerator_RoundTrip` walk `render.Bundle()`'s map only, or compare the full `bundle/` tree against a freshly regenerated tree? If the former, the gen-side-only `query-additions.md` won't be covered by the test directly — we'd rely on `make generate-check` for drift detection, or extend the test. Confirm during implementation.
- Should the new bullet linking to `query-additions.md` be the FIRST bullet (highest visibility) or grouped with the existing query-related gotchas (#7–#12)? Plan picks "first" for visibility, but if reviewers prefer thematic grouping, move it.
- `.gitattributes` already covers `**/internal/skill/bundle/**` and `**/internal/skill/additions.md`; verify whether `**/internal/skill/query-additions.md` matches an existing glob or needs an explicit entry to stay LF on Windows checkouts.
