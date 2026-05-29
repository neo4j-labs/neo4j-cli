# PRD: Detect Graph Engine in `:schema`

## Overview

Extend `neo4j-cli query :schema` to detect and report the active graph engine (initially Aura "Virtual Graph") and the set of Cypher dialect versions supported by the server, both surfaced via additional rows of the existing `CALL dbms.components()` probe. Teach the `query` skill bundle which Cypher constructs to avoid when talking to a Virtual Graph instance so that LLM-driven agents (notably Text2Cypher) stop emitting unsupported syntax.

Tracking issue: [CLI-177](https://linear.app/neo4j/issue/CLI-177/detect-graph-engine-in-schema). Upstream context: GAIAGT-768 (Text2Cypher needs VG awareness).

## Goals

- Surface a `graph_engine` block in the `:schema` output JSON / TOON payload whenever the connected DBMS exposes a non-kernel engine row in `CALL dbms.components()`.
- Surface the supported Cypher dialect versions (e.g. `["5","25"]`) alongside the existing `default_language`, so the agent knows *which* dialects the server understands rather than only which one it picks by default.
- Document the current Virtual Graph Cypher restrictions in the `query` skill so agents can adapt generated Cypher.
- Keep the change regression-safe on plain Neo4j: no new fields appear in the payload unless the server reports them (`omitempty` everywhere).

## Non-Goals

- Refactoring the existing `:schema` rendering pipeline or table-mode output (table mode already excludes the `database` block — no change needed).
- Adding a Virtual-Graph-aware preflight that *blocks* unsupported Cypher inside `neo4j-cli query` (information-only for now; agents adapt).
- Surfacing graph-engine info anywhere outside `:schema` (e.g. `query --help`, `agent-context`).
- Live testing against an actual Virtual Graph instance (none available locally; unit tests pin behaviour).
- Updating the bundle frontmatter `description.txt` — schema introspection is already advertised at the top level and "Virtual Graph awareness" is a sub-capability.

## Requirements

### Functional Requirements

- **REQ-F-001**: `:schema` MUST iterate every row returned by `CALL dbms.components()` rather than reading only row 0. Rows are matched by their literal `name` field (not row index) so server-side ordering changes do not break detection.
- **REQ-F-002**: A row with `name == "Neo4j Kernel"` MUST populate `databaseInfo.Versions` and `databaseInfo.Edition` (preserves existing behaviour).
- **REQ-F-003**: A row with `name == "Cypher"` MUST populate a new `databaseInfo.CypherVersions []string` field from the row's `versions[]` value.
- **REQ-F-004**: Any other row (e.g. `"Virtual Graph"`) MUST populate a new `databaseInfo.GraphEngine *graphEngine` field with the row's `name` and `versions[]`. If multiple non-kernel / non-Cypher rows exist, the first one wins (deterministic by iteration order over the result set).
- **REQ-F-005**: All new fields MUST use `omitempty` JSON tags so the payload shape on a plain Neo4j (no VG row) is identical to today.
- **REQ-F-006**: `fetchDatabaseInfo` MUST keep its existing warn-and-continue error policy: a failed `dbms.components()` call (missing role, stripped-down server) leaves all four fields empty and does NOT fail the command.
- **REQ-F-007**: The `query` skill bundle MUST gain a new "Virtual Graphs / Graph Engine" section in `query-additions.md` documenting the current Cypher restrictions:
  - Forbidden statements: `WITH`, `CALL`, `UNWIND`, `range()`, var-length / Quantified Path Patterns, subquery expressions (`EXISTS { ... }`), all writes (`CREATE` / `MERGE` / `SET` / `INSERT` / `DELETE` / `REMOVE` / `DROP` / `ALTER` / `START` / `STOP` / `GRANT` / `REVOKE`), `apoc.*` procedures.
  - Detection rule: `database.graph_engine.name == "Virtual Graph"` ⇒ apply the restrictions.
  - Guidance: simplify queries rather than retry verbatim when a VG rejects.
- **REQ-F-008**: The "What `:schema` returns" bullet for `database` in `query-additions.md` MUST mention the new `cypher_versions` and `graph_engine` fields and link to the new section.
- **REQ-F-009**: The skill bundle MUST be regenerated via `go generate ./neo4j-cli/internal/skill/...` and committed in the same PR.
- **REQ-F-010**: A user-facing `Patch` changelog entry MUST be added under `.changes/unreleased/`.

### Non-Functional Requirements

- **REQ-NF-001**: All existing tests in `neo4j-cli/query/schema_test.go` MUST continue to pass with no behavioural drift on plain Neo4j (the `happySchemaSeam` is updated to emit both `Neo4j Kernel` and `Cypher` rows, asserting `GraphEngine == nil`).
- **REQ-NF-002**: Unit tests MUST cover (a) plain Neo4j (no VG row, `GraphEngine == nil`, `CypherVersions == ["5","25"]`), (b) VG row present, (c) row-order insensitivity (VG row first).
- **REQ-NF-003**: All CI gates MUST pass: `make test`, `make fmt-check`, `make lint`, `make license-check`, `make generate-check`.
- **REQ-NF-004**: New Go code MUST carry the Neo4j copyright header (`addlicense` gate).
- **REQ-NF-005**: The Go struct additions follow the existing one-file-per-leaf cobra layout — all edits stay in `neo4j-cli/query/schema.go` and `neo4j-cli/query/schema_test.go`.

## Technical Considerations

### Data model

`neo4j-cli/query/schema.go:35-40` — `databaseInfo` gains two fields:

```go
type databaseInfo struct {
    Name            string       `json:"name,omitempty"`
    Versions        []string     `json:"versions,omitempty"`
    Edition         string       `json:"edition,omitempty"`
    DefaultLanguage string       `json:"default_language,omitempty"`
    CypherVersions  []string     `json:"cypher_versions,omitempty"`
    GraphEngine     *graphEngine `json:"graph_engine,omitempty"`
}

type graphEngine struct {
    Name     string   `json:"name"`
    Versions []string `json:"versions,omitempty"`
}
```

### Detection logic

`neo4j-cli/query/schema.go:259-282` — replace the `len(res.Rows) > 0 ⇒ res.Rows[0]` shortcut with a switch over each row's `name` field. Verified shape against live Neo4j 2026.04.0 enterprise:

```json
{
  "columns": ["name", "versions", "edition"],
  "rows": [
    {"name": "Neo4j Kernel", "versions": ["2026.04.0"], "edition": "enterprise"},
    {"name": "Cypher",       "versions": ["5", "25"],   "edition": ""}
  ]
}
```

A Virtual Graph DBMS adds a third row (`name: "Virtual Graph"`) per GAIAGT-768.

### Render path

- JSON / TOON: `schemaResult` marshals directly via `commonoutput.PrintBodyMap` (`schema.go:298`) — new fields appear automatically with `omitempty` handling. No render-code changes.
- Table mode: `printSchemaTables` (`schema.go:309-344`) intentionally omits the `database` block ("no natural tabular shape for the single-record metadata payload"). No change.

### Skill bundle

- `neo4j-cli/internal/skill/query-additions.md` — copied verbatim into the bundle via `gen/main.go:99`. Adding the new section requires regeneration via `go generate ./neo4j-cli/internal/skill/...`.
- `description.txt` unchanged (schema introspection already advertised).
- `additions.md` unchanged.

### Edge cases

- Empty `dbms.components()` result → `fetchDatabaseInfo` still returns `nil` (existing behaviour).
- Driver returns rows with unexpected `name` values → ignored (only known names populate kernel / cypher fields; unknown rows feed `GraphEngine`).
- Multiple non-kernel / non-Cypher rows → first one wins; sufficient for the foreseeable future since Aura ships one engine flavour per instance.
- `versions` column missing from a row → `asStringSlice` returns `nil`, `omitempty` hides the field.

### Risk

- We cannot live-verify the VG positive path (no VG instance in dev). Mitigation: pin the assertion key to the literal row `name` ("Virtual Graph") in unit tests; the only realistic break would be a server-side rename, which we'd notice from the same fixture mismatch.

## Acceptance Criteria

- [ ] `databaseInfo` extended with `CypherVersions` and `GraphEngine` per REQ-F-001..F-005.
- [ ] `fetchDatabaseInfo` iterates all rows and matches by `name` (not by index).
- [ ] Running `:schema --format json` against plain Neo4j returns the existing payload shape; no new keys present (regression test passes).
- [ ] Unit test `TestSchema_HappyPath_JSON` updated to include both `Neo4j Kernel` and `Cypher` rows; asserts `Database.CypherVersions == ["5","25"]` and `Database.GraphEngine == nil`.
- [ ] New unit test `TestSchema_DetectsGraphEngine` covers a VG row; asserts `Database.GraphEngine.Name == "Virtual Graph"`, `Database.GraphEngine.Versions == ["1.2.3"]`, kernel + cypher fields still populate.
- [ ] New unit test `TestSchema_GraphEngine_KernelRowOrderInsensitive` covers VG row at position 0; same assertions as above.
- [ ] Existing `TestSchema_*` tests continue to pass without modification (aside from the happy-path multi-row fixture update).
- [ ] `query-additions.md` `database` bullet mentions `cypher_versions` and `graph_engine`; new "Virtual Graphs / Graph Engine" section added after "Cypher 25 vs Cypher 5 syntax".
- [ ] `go generate ./neo4j-cli/internal/skill/...` re-run; `bundle/` changes committed in the same commit.
- [ ] `Patch` changelog entry under `.changes/unreleased/neo4j-cli-Patch-*.yaml` mentioning CLI-177.
- [ ] `make test && make fmt-check && make lint && make license-check && make generate-check` all green.
- [ ] Manual smoke test against `docker create --rw --ephemeral --edition enterprise` confirms `graph_engine` is **absent** and `cypher_versions == ["5","25"]` on plain Neo4j.

## Out of Scope

- A `--rw`-style preflight that *blocks* unsupported Cypher when talking to a VG.
- Surfacing graph-engine info outside `:schema` (e.g. in `query --help`, `agent-context`, `credential` listings).
- Table-mode rendering of the `database` block (intentionally JSON/TOON-only).
- Updating the skill `description.txt` frontmatter.
- Detecting / supporting future engine names beyond what `dbms.components()` returns verbatim — the field stores the literal `name`, so renames carry through with no code change.
- Live verification against a Virtual Graph Aura instance.

## Open Questions

None. All three open questions from the planning phase (field naming, surfacing Cypher row versions, changelog kind) confirmed by the user:

1. Nested `graph_engine: {name, versions}` object — confirmed.
2. Surface `cypher_versions[]` alongside `default_language` — confirmed.
3. Changelog kind `Patch` (not `Minor`) — confirmed.
