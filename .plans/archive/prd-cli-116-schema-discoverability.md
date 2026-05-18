# PRD: CLI-116 — Make `:schema` easier to find for agents

Linear: [CLI-116](https://linear.app/neo4j/issue/CLI-116/make-schema-easier-to-find-for-agents)
Source plan: `/Users/oskarhane/.claude/plans/in-the-skill-it-foamy-cupcake.md`

## Overview

The `neo4j-cli query :schema` subcommand introspects a Neo4j database (labels, relationship types, properties, indexes, constraints) and is the canonical way for an AI agent to learn the schema before generating Cypher. The functionality already works and is documented in the companion `query-additions.md`, but it is under-exposed in the skill bundle: agents asked to "look into the schema of neo4j" or "generate a data model from the schema" frequently fail to discover the `:schema` subcommand and instead guess or refuse.

This PRD covers a minimal, 4-file source edit (plus one regen) to surface `:schema` and the "always introspect before generating Cypher" rule in every surface an agent could plausibly land on: the skill's frontmatter `description`, the SKILL.md Subcommands table, the first Tips & Gotchas bullet, `query --help`, and `query :schema --help`.

## Goals

- Make `:schema` discoverable from a single read of `SKILL.md` — no need to follow the `query-additions.md` pointer to see the command.
- Strengthen the skill's trigger match on "schema" / "data model" phrasing by naming `:schema` explicitly in `description.txt`.
- Surface the schema-first rule literally (showing `neo4j-cli query :schema --format toon`) so an agent can act on it without a second hop.
- Land the rule across all `--help` surfaces an agent might consult (`query --help`, `query :schema --help`) by editing Cobra `Short` / `Long` / `Example` fields that flow into the generated reference docs.

## Non-Goals

- Rewriting `query-additions.md`. Its existing "ALWAYS run `:schema` first" content stays; this PRD's job is to surface the rule upstream, not to rewrite it.
- Adding a dedicated `## Schema-first workflow` section to SKILL.md. That would require teaching `common/skill/render/render.go` about a new section type and is explicitly rejected for this PRD.
- Touching the 21 existing `-f <format>` references in `query-additions.md` / `additions.md` / etc. The `-f` shorthand is being removed by in-flight PR [neo4j-labs/neo4j-cli#111](https://github.com/neo4j-labs/neo4j-cli/pull/111); that PR (or a follow-up) handles the cleanup. This PRD uses long-form `--format` in all NEW content only.
- Changes to the standalone `aura` skill bundle (`neo4j-cli/aura/internal/skill/`). `query` is not under `aura`, so its bundle is unaffected.
- Changes to behavior of `query` or `:schema` — text/help/skill-bundle content only.
- Changing the generator (`neo4j-cli/internal/skill/gen/main.go`) or the renderer (`common/skill/render/render.go`).

## Requirements

### Functional Requirements

- **REQ-F-001**: `neo4j-cli/internal/skill/description.txt` MUST explicitly mention `:schema` (or "neo4j-cli query :schema") as a way to inspect the database schema / data model. The phrase must remain a single paragraph, third-person, ≤1024 chars (skill description constraint).

- **REQ-F-002**: `neo4j-cli/internal/skill/additions.md` MUST have, as its FIRST bullet, a directive of the form:
  > **Before generating ANY Cypher: run `neo4j-cli query :schema --format toon` first to discover the real labels, relationship types, and properties. Do not guess the schema.** Read [query-additions.md](query-additions.md) for the full schema-first workflow, parameters, embeddings, and Cypher 25 vs 5.
  
  The bullet MUST show the literal command (`neo4j-cli query :schema --format toon`) and MUST retain a pointer to `query-additions.md`.

- **REQ-F-003**: `neo4j-cli/query/query.go`'s parent cobra command MUST have its `Short` rewritten to mention schema introspection, e.g. `"Run Cypher, inspect the database schema (:schema), and embed text against a Neo4j database via the Bolt protocol"`. The new `Short` MUST stay ≤120 characters so the SKILL.md Subcommands table row renders cleanly.

- **REQ-F-004**: `neo4j-cli/query/query.go`'s `Long` MUST be prefixed with: `"Use the :schema subcommand to introspect labels, relationship types, and properties before writing Cypher — never guess the schema."` The remainder of the existing `Long` text is preserved verbatim.

- **REQ-F-005**: `neo4j-cli/query/query.go`'s `Example` MUST have a new FIRST example:
  ```
  # Introspect the schema before writing Cypher (always do this first)
  neo4j-cli query :schema --format toon
  ```
  followed by the existing 5 examples in their current order. The block must remain flush-left (no leading indent on the first line) so the renderer at `common/skill/render/render.go:235` does not produce a ragged code block.

- **REQ-F-006**: `neo4j-cli/query/schema.go`'s `Long` MUST be prefixed with: `"Run this BEFORE generating Cypher to discover the database's actual labels, relationship types, and properties — never guess. "` (note trailing space). The remainder is preserved.

- **REQ-F-007**: `neo4j-cli/query/schema.go`'s `Example` MUST replace its first example with:
  ```
  # Discover labels, rel types, and properties (run this before writing Cypher)
  neo4j-cli query :schema --format toon
  ```
  followed by the existing JSON and `jq` examples in their current order.

- **REQ-F-008**: After source edits, `go generate ./neo4j-cli/internal/skill/...` MUST be run. The committed bundle (`neo4j-cli/internal/skill/bundle/SKILL.md` and `bundle/references/query.md`) MUST reflect the new content. No hand-edits to bundle files.

- **REQ-F-009**: A changelog entry MUST be added via `changie new --projects neo4j-cli --kind Minor --body "skill bundle: surface query :schema as the schema-first workflow on SKILL.md, query --help, and :schema --help (CLI-116)"`.

### Non-Functional Requirements

- **REQ-NF-001**: All NEW examples MUST use long-form `--format` (not `-f`). Stale `-f` references elsewhere in the repo are out of scope.

- **REQ-NF-002**: `TestGenerator_RoundTrip` MUST pass — the committed bundle matches what `go generate` produces from the post-edit cobra tree.

- **REQ-NF-003**: `TestAllLeafCommands_HaveExamples` MUST pass — every cobra leaf still has a non-empty flush-left `Example` field with ≥3 invocations, `#` comment lines before each invocation, blank-line separators, `neo4j-cli` prefix, and `--rw` on writes / `--format json` on at least one read.

- **REQ-NF-004**: All CLAUDE.md final gates MUST pass on the diff: `make fmt-check`, `make test`, `make lint`.

- **REQ-NF-005**: The skill bundle's frontmatter `description` MUST stay a single paragraph, third-person, ≤1024 chars.

- **REQ-NF-006**: The `Long` text on `query` MUST stay readable when rendered by `cobra --help`: avoid breaking the existing prose flow when prepending the new sentence.

## Technical Considerations

- **Generated content workflow**. The skill bundle under `neo4j-cli/internal/skill/bundle/` is generated by `neo4j-cli/internal/skill/gen/main.go` from cobra-tree text (`Short` / `Long` / `Example` fields) plus three hand-written inputs (`description.txt`, `additions.md`, `query-additions.md`). Edits land in source; `go generate ./neo4j-cli/internal/skill/...` produces the bundle. `TestGenerator_RoundTrip` is the gate.

- **Renderer quirk**. `common/skill/render/render.go:235` trims leading whitespace from the FIRST line of an `Example` block via `strings.TrimSpace` and leaves subsequent lines intact, producing a ragged code block when the source has a 2-space indent. The new Example contents must be flush-left in the Go source to render cleanly.

- **Subcommands-table source**. The SKILL.md Subcommands table row for `query` is sourced from the cobra `Short` field, NOT from `Long`. To put `:schema` in the table, edit `Short`.

- **`-f` removal in flight**. PR #111 removes the `-f` shorthand. This PRD uses long-form `--format` in new content so the diff doesn't conflict with #111 and doesn't ship a directive that will break the day #111 lands. Existing `-f` references are out of scope.

- **Aura side unaffected**. `query` is at the top of the neo4j-cli command tree, not under `aura`. Only `neo4j-cli/internal/skill/...` needs regen; `neo4j-cli/aura/internal/skill/...` is untouched.

- **`query-additions.md` is canonical for the rule**. The detailed workflow ("Schema-first workflow", "What `:schema` returns", "Handling user requests", Cypher 25 vs 5) lives in `query-additions.md`. This PRD does not duplicate that content upstream — the upstream surfaces show the command and a one-line directive, then link to the canonical file.

- **Subcommand position matters**. Cobra renders subcommands in the order they're declared. The Subcommands table on SKILL.md is rendered alphabetically by the bundle generator, but `query --help` shows subcommands in their declared order. No changes to subcommand order are needed; `:schema` already exists as a leaf.

## Acceptance Criteria

- [ ] `neo4j-cli/internal/skill/description.txt` mentions `:schema` (or `neo4j-cli query :schema`) explicitly. Description is a single paragraph, third-person, ≤1024 chars.
- [ ] `neo4j-cli/internal/skill/additions.md` has the schema-first directive as the FIRST bullet, with literal `neo4j-cli query :schema --format toon` shown and a pointer to `query-additions.md` retained.
- [ ] `neo4j-cli/query/query.go`'s `Short` is rewritten to name schema introspection (`:schema`), stays ≤120 chars.
- [ ] `neo4j-cli/query/query.go`'s `Long` is prefixed with the new `:schema` directive sentence; the rest is preserved.
- [ ] `neo4j-cli/query/query.go`'s `Example` has the `:schema --format toon` block as the new FIRST example, with the existing 5 examples following.
- [ ] `neo4j-cli/query/schema.go`'s `Long` is prefixed with the new "Run this BEFORE generating Cypher…" sentence; the rest is preserved.
- [ ] `neo4j-cli/query/schema.go`'s `Example` has the new `--format toon` first example; existing JSON and `jq` examples follow.
- [ ] `go generate ./neo4j-cli/internal/skill/...` has been run; `neo4j-cli/internal/skill/bundle/SKILL.md` and `bundle/references/query.md` reflect the new content.
- [ ] `bundle/SKILL.md` frontmatter `description` mentions `:schema`.
- [ ] `bundle/SKILL.md` Subcommands table `query` row signals `:schema` / schema introspection.
- [ ] `bundle/SKILL.md` first Tips & Gotchas bullet shows the literal `neo4j-cli query :schema --format toon` command.
- [ ] `bundle/references/query.md` Long lede and FIRST example are both `:schema`-centric.
- [ ] `bin/neo4j-cli query --help` smoke test: Long lede mentions `:schema` first; first example is `:schema`.
- [ ] `bin/neo4j-cli query :schema --help` smoke test: Long lede leads with "Run this BEFORE generating Cypher…"; first example uses `--format toon`.
- [ ] `make fmt-check` clean.
- [ ] `make test` passes (especially `TestGenerator_RoundTrip` and `TestAllLeafCommands_HaveExamples`).
- [ ] `make lint` clean.
- [ ] Changelog entry added via `changie new --projects neo4j-cli --kind Minor --body "skill bundle: surface query :schema as the schema-first workflow on SKILL.md, query --help, and :schema --help (CLI-116)"`.
- [ ] PR title and body reference `CLI-116`.

## Out of Scope

- Rewriting or restructuring `neo4j-cli/internal/skill/query-additions.md`.
- Adding a dedicated `## Schema-first workflow` section to `SKILL.md` (would require `common/skill/render/render.go` changes).
- Cleaning up the 21 stale `-f <format>` references across `query-additions.md`, `additions.md`, `common/skill/skill.go`, `test/e2e/check_json/main.go`, `CHANGELOG-*.md`. PR #111 owns the `-f` removal.
- Any changes to the `aura` skill bundle or commands.
- Any change to the JSON/TOON/table output shape of `:schema`.
- Any change to the cobra `Short` / `Long` / `Example` fields on commands other than `query` (parent) and `query :schema` (leaf).
- Updating the website (`gh-pages` branch) — that's a separate prompt-driven workflow.

## Open Questions

None. Linear ticket confirmed (CLI-116). `--format` long-form confirmed for new content. No dedicated SKILL.md section. 4-file minimal edit confirmed.
