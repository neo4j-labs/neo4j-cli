# PRD: Render Neo4j temporal values correctly in `neo4j-cli query` output

Linear: [CLI-182](https://linear.app/neo4j/issue/CLI-182)

## Overview

Any Cypher `RETURN` that surfaces a Neo4j temporal value (`Date`, `LocalDateTime`, `LocalTime`, `Time`, `Duration`) currently renders as `{}` (empty JSON object) in both `--format json` and `--format table`. The value is silently dropped — no error, no warning. `DateTime` (zoned) arrives as native `time.Time` and serialises correctly in JSON via stdlib, but renders inconsistently in table mode (quoted RFC3339 string), so we coerce it too for a single canonical format.

The fix adds a small recursive value-coercion step on the driver-response boundary that replaces each driver-native temporal with its ISO-8601 string form before serialization. The recursion walks lists, maps, and `Node`/`Relationship`/`Path` `Props` so nested temporals are also covered.

## Goals

- Stop silent data loss for `Date`, `LocalDateTime`, `LocalTime`, `Time`, `Duration` in `neo4j-cli query` output.
- Render each temporal as a stable ISO-8601 string in both `--format json` and `--format table`.
- Cover nested cases (list of dates, map with date value, node/relationship `Props` containing temporals, path entities).
- Preserve current behaviour for all non-temporal types.

## Non-Goals

- Spatial types (`dbtype.Point2D`, `dbtype.Point3D`) — currently marshal verbosely but preserve data; not a data-loss bug.
- `dbtype.Vector` — same: verbose JSON, but data preserved.
- Bolt-summary metadata surfacing in JSON output (intentional design choice).
- Graph Engine `Summary.QueryType` mis-reporting on pure reads (upstream server-side issue, separate ticket).

## Requirements

### Functional Requirements

- REQ-F-001: `dbtype.Date` values serialize to their ISO-8601 form (`2026-05-25`) in both `--format json` and `--format table`.
- REQ-F-002: `dbtype.LocalDateTime`, `dbtype.LocalTime`, `dbtype.Time` values serialize to their driver-supplied ISO-8601 form in both formats.
- REQ-F-003: `dbtype.Duration` values serialize to their ISO-8601 duration form (`P3D`, `PT1H30M`, etc.) in both formats.
- REQ-F-004: Native `time.Time` (Cypher `DateTime`) values serialize via `time.RFC3339Nano` in both formats.
- REQ-F-005: Temporal values nested inside `[]any` (list) are coerced recursively.
- REQ-F-006: Temporal values nested inside `map[string]any` are coerced recursively.
- REQ-F-007: Temporal values inside `dbtype.Node.Props`, `dbtype.Relationship.Props`, and `dbtype.Path.Nodes[].Props` / `dbtype.Path.Relationships[].Props` are coerced recursively.
- REQ-F-008: Non-temporal scalars (`bool`, `int64`, `float64`, `string`, `nil`, `[]byte`) pass through unchanged.
- REQ-F-009: A `changie` Patch changelog entry is added under `.changes/unreleased/`.

### Non-Functional Requirements

- REQ-NF-001: No regression on existing query tests (`go test ./neo4j-cli/query/...` and `make test` pass).
- REQ-NF-002: `make fmt-check` and `make lint` clean.
- REQ-NF-003: `make generate-check` clean (no skill-bundle drift).
- REQ-NF-004: New file carries the Neo4j copyright header (required by `make license-check` and CI `addlicense`).
- REQ-NF-005: New tests follow the table-driven idiom used elsewhere in `neo4j-cli/query/*_test.go`.

## Technical Considerations

### Architecture

The coercion lives at the response-builder boundary, immediately after `result.Collect` returns driver records:

- **New file `neo4j-cli/query/coerce.go`** — defines `coerceDriverValue(v any) any`. Switch on each driver-native temporal type and replace with its `String()` form; recurse into `[]any`, `map[string]any`, and `dbtype.Node`/`Relationship`/`Path` `Props` (shared map mutation is safe — records aren't reused after `Collect`).
- **Call site `neo4j-cli/query/connect.go:573-577`** — replace the bulk `copy(row, rec.Values)` with a per-cell loop calling `coerceDriverValue`. Single seam fixes both JSON and table output because `renderResult.MarshalJSON` (JSON path) and `renderResult.AsArray` → `formatCell` (table path) both read from `resp.Data.Values`. `formatCell` is left untouched — coerced strings flow through its existing `case string` branch.

### Type mapping

| Driver type             | Coerced to                       |
| ----------------------- | -------------------------------- |
| `dbtype.Date`           | `t.String()` → `2026-05-25`      |
| `dbtype.LocalDateTime`  | `t.String()`                     |
| `dbtype.LocalTime`      | `t.String()`                     |
| `dbtype.Time`           | `t.String()` (zoned)             |
| `dbtype.Duration`       | `t.String()` → `P3D`             |
| `time.Time` (DateTime)  | `t.Format(time.RFC3339Nano)`     |
| `[]any`                 | recurse element-wise (in place)  |
| `map[string]any`        | recurse value-wise (in place)    |
| `dbtype.Node`           | recurse `Props` (shared map)     |
| `dbtype.Relationship`   | recurse `Props` (shared map)     |
| `dbtype.Path`           | recurse Node + Relationship Props|
| anything else           | passthrough                      |

### Integration points

- Vendored driver: `github.com/neo4j/neo4j-go-driver/v6 v6.0.0` — `dbtype` package provides `String()` on every temporal type; no driver changes.
- No changes to cobra `Long`/`Example`/flag-Usage text → skill bundle stays in sync (no regen needed; verify with `make generate-check`).
- No changes to the JSON envelope schema (`columns`, `rows`, `truncated`, `arrays_truncated`).

### Risks / Mitigations

- **In-place mutation of `Props` maps** — safe because records aren't reused after `result.Collect`; documented at the helper. Test covers Node/Relationship/Path cases.
- **Unknown temporal types in future driver versions** — passthrough default means new types fall through to existing `json.Marshal` behaviour (verbose or `{}`); we'd need a follow-up to extend the switch. Low impact, easy to ship.

## Acceptance Criteria

- [ ] `neo4j-cli/query/coerce.go` exists with `coerceDriverValue(v any) any` covering all five `dbtype` temporal types, native `time.Time`, lists, maps, and `Node`/`Relationship`/`Path` `Props`.
- [ ] `neo4j-cli/query/connect.go` per-cell coerce loop replaces the bulk `copy` at lines 573-577.
- [ ] `neo4j-cli/query/coerce_test.go` table-driven tests cover: each temporal type, list of temporals, map containing a temporal, `Node` / `Relationship` / `Path` with temporal `Props`, scalar passthroughs (`int64`, `float64`, `string`, `bool`, `nil`).
- [ ] `neo4j-cli/query/output_test.go` extended to lock the string-passthrough contract (already-coerced ISO string renders identically in JSON and table).
- [ ] `.changes/unreleased/neo4j-cli-Patch-*.yaml` changelog entry added via `changie` (or hand-authored YAML per AGENTS.md if changie isn't installed).
- [ ] `make test`, `make fmt-check`, `make lint`, `make generate-check`, `make license-check` all clean.
- [ ] Manual smoke (Docker): `RETURN date('2026-05-25') AS d, datetime() AS dt, duration({days: 3}) AS dur` renders `"d":"2026-05-25"`, `"dt":"2026-..."`, `"dur":"P3D"` in JSON; bare ISO strings (no surrounding quotes, no `{}`) in table mode.

## Out of Scope

- Spatial Point2D/Point3D rendering improvements (cosmetic).
- Vector rendering improvements (cosmetic).
- Bolt summary metadata surfacing.
- Graph Engine QueryType classification bug.

## Open Questions

None — all design decisions resolved in plan phase:

- DateTime (`time.Time`) is coerced for canonical formatting consistency across JSON and table.
- Recursion includes `Node`/`Relationship`/`Path` `Props`.
- Scope is temporal-only; Spatial and Vector deferred to follow-up tickets if requested.
