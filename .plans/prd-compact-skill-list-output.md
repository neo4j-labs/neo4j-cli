# PRD: Compact `neo4j-cli skill list` output

## Overview

Today `neo4j-cli skill list` renders one row per `(skill × agent)`. With 11 supported agents and ~28 curated catalog skills, that's ~319 rows in table view — unscannable on a normal terminal. The embedded self-skill row IS meaningfully per-agent (install state and version varies by agent). Every catalog row, by contrast, is the same agent-agnostic markdown duplicated 11 times.

This feature collapses the human-readable views (table + toon) into two stacked sections:

1. **Self-skill matrix** — one row per agent for the embedded self-skill (unchanged information density; just isolated visually).
2. **Catalog summary** — one row per catalog skill with an aggregated `installed_in` column naming which agents hold it.

JSON output stays flat and unchanged (`--format json | jq` consumers untouched). Only `--format table` (default) and `--format toon` get the two-section render.

## Goals

- Make `skill list` scannable on a normal terminal — ~39 rows (11 self + ~28 catalog) instead of ~319.
- Preserve every datum surfaced today; the aggregation is presentational, not informational loss.
- Zero contract break for scripts consuming `--format json`.
- Minimal diff: reuse `BuildInventory` and `statusFor` from `inventory.go`; only render-time changes plus a small pure aggregator.

## Non-Goals

- No new flag (`--full`, `--shape`, `--by-agent`, `--agent <name>`). The flat matrix is already reachable via `--format json` — humans don't need it.
- No change to `check.go`. It already filters to installed rows (typically 1–5 rows) and is not a UX problem.
- No change to `BuildInventory` signature, `InventoryRow` struct, or the per-row status classifier. Compaction is render-time only.
- No new structured-output renderer infrastructure (no multi-table abstraction in `common/output/`). Two `PrintBodyMap` calls with a heading line between them are enough.
- No change to JSON output shape — flat array of per-agent rows, exactly as today.

## Requirements

### Functional Requirements

- REQ-F-001: `runList` branches on `commonoutput.ResolveOutput(cmd, cfg)`. `json` → unchanged single `PrintBodyMap` over the flat row set. `table` / `toon` → the new two-section render.
- REQ-F-002: The self-skill section prints first. Columns: `agent, detected, installed, installed_version, available_version, status`. One row per agent in `AGENTS` (all 11 — keeps the "this agent doesn't have it" cell visible). The `skill` and `source` columns drop — the section heading carries that context.
- REQ-F-003: The catalog section prints second. Columns: `skill, available_version, status, installed_in`. One row per catalog skill present in the cached `plugin.json`, in plugin.json order. Reserved-name collisions (self / binary name) are still skipped.
- REQ-F-004: `installed_in` cell format:
  - `"—"` when zero agents have the skill installed.
  - `"N/11 (agent-a, agent-b, ...)"` when 1 ≤ N < 11. Agent names comma-separated in `AGENTS` catalog order.
  - `"11/11"` when every agent has it (omit the parenthetical to keep the line short).
  - Denominator is `len(AGENTS)` — stable across hosts regardless of which agents are detected.
- REQ-F-005: Aggregated catalog `status` follows a **worst-wins priority** when folding the 11 per-agent rows for one skill:
  1. `drift` — at least one installed copy whose version disagrees with `available_version`.
  2. `unknown-version` — at least one installed copy whose frontmatter lacks a parseable `version:`.
  3. `partial` — at least one agent has it AND at least one agent doesn't (and nothing above triggered).
  4. `installed` — every agent has it at the matching version.
  5. `not-installed` — zero agents have it.
- REQ-F-006: Section headings — two short title lines printed via `cmd.Println`, e.g.:
  - `Self-skill:` (blank line) `<table>` (blank line) `Catalog:` (blank line) `<table>`.
  Plain text; no banner / no ASCII art. Tone matches existing CLI emissions (compare with `install.go`'s `"No agents to install."` cmd.Printf).
- REQ-F-007: The existing cold-cache hint (`skill catalog cache is empty — run '... skill refresh' ...`) still fires when `catalogLoad.Cat == nil`. In that case the catalog section is omitted entirely; only the self-skill section renders.
- REQ-F-008: The existing warn-on-network-failure-with-cache message (via `catalogLoad.PrintWarn`) still fires before the rendered output.
- REQ-F-009: `--format json` output is byte-identical to today's output for the same inputs. Test assertions in `list_test.go` that check JSON shape and per-row count remain valid without modification.
- REQ-F-010: `--refresh` flag behaviour is unchanged — forces a network fetch before rendering, regardless of which section view is selected.

### Non-Functional Requirements

- REQ-NF-001: New aggregator function (`aggregateCatalog` or similar) lives in `common/skill/inventory.go`, is a pure function over `[]InventoryRow`, and has table-driven test coverage in `inventory_test.go` (worst-wins priority cases + edge cases: all-installed, zero-installed, single-installed-with-drift, mixed unknown-version + partial).
- REQ-NF-002: `BuildInventory` signature and `InventoryRow` struct unchanged. `statusFor` unchanged. `check.go` consumers unaffected.
- REQ-NF-003: All gates pass: `make test`, `make fmt-check`, `make lint`, `make license-check`, `make generate-check` (regen the skill bundle after `runList`'s Long text picks up a one-line note about the compact default).
- REQ-NF-004: No new third-party dependency. No new file under `common/output/`. The only new code lives in `common/skill/list.go` (render branch) and `common/skill/inventory.go` (aggregator) plus their colocated tests.

## Technical Considerations

**Where to branch.** `runList` already calls `loadOrRefreshCatalog`, builds inventory rows, then calls `renderListResult` for the unified render. Branching there on `commonoutput.ResolveOutput(cmd, cfg) == "json"` keeps the diff localised. The JSON branch keeps today's code path verbatim — same `listResultRow` JSON struct, same `MarshalJSON`. The table/toon branch is new logic that:

1. Splits the `[]InventoryRow` into `self` rows (where `Source == sourceEmbedded`) and `catalog` rows (everything else).
2. Calls `PrintBodyMap` over the self rows with columns `agent, detected, installed, installed_version, available_version, status`.
3. For each catalog skill, folds its per-agent rows via `aggregateCatalog` → a `catalogSummary` struct, then calls `PrintBodyMap` over the resulting summary rows with columns `skill, available_version, status, installed_in`.

**Aggregator shape (preview):**

```go
type catalogSummary struct {
    Skill            string
    AvailableVersion string
    Status           string  // worst-wins
    InstalledCount   int     // 0..len(AGENTS)
    InstalledAgents  []string // catalog order
}

func aggregateCatalog(rows []InventoryRow) catalogSummary
```

`installed_in` text is composed from `InstalledCount + InstalledAgents` at render time, not stored — this keeps the struct minimal and lets the renderer pick the "—" / "N/M (names)" / "N/M" formatting.

**Reuse:**
- `BuildInventory(fs, binaryName, version, cat)` — single source of per-agent rows; unchanged.
- `statusFor(installed, installedVersion, availableVersion)` — per-row classifier; the aggregator composes its results into the worst-wins fold.
- `boolStr(b bool) string` — already used for `detected`/`installed` cells in the self section. Reuse, don't reinvent.
- `commonoutput.PrintBodyMap(cmd, cfg, data, columns)` — called twice (once per section) for table/toon, once for JSON.
- `commonoutput.ResolveOutput(cmd, cfg)` — already imported in `list.go`; the branch is one extra `if` at the top of the renderer.
- `catalogLoad.PrintWarn` and `catalogLoad.PrintColdCacheHint` — unchanged surfaces; still fire at the same point relative to the table render.

**JSON shape unchanged.** This is the load-bearing constraint. Tests in `list_test.go` unmarshal the JSON output into `[]map[string]any` and grep for specific `(skill, agent)` tuples. Those assertions must keep passing without modification. Achieved by leaving the JSON path identical to today.

**Toon collapses with table.** Toon's value prop is token efficiency for LLM consumers; rendering 319 rows defeats that. Both table and toon share the new two-section path. Toon consumers that need the full matrix go through `--format json`.

**Skill-bundle regeneration.** The `Long` text on the list leaf picks up one sentence describing the compact default and pointing at `--format json` for the flat matrix. After source edits, run `go generate ./neo4j-cli/internal/skill/...` and commit the bundle delta. `make generate-check` is the CI gate.

**Test brittleness.** Today's `list_test.go` asserts `expectedRowCount(n) = (1 + n_catalog) * len(AGENTS)` against the table output and also greps for column headers (`skill`, `source`, `agent`, ...). Those table-mode assertions need to change to match the new shape: row count for table is `len(AGENTS) + n_catalog` (one self row per agent + one catalog row per skill), columns split across two sections. JSON-mode assertions stay untouched.

## Acceptance Criteria

- [ ] `neo4j-cli skill list` (default = table) prints two sections separated by a blank line. Self section has 11 rows; catalog section has ~28 rows (one per catalog entry in `plugin.json`). Total visible rows ≈ 39, not 319.
- [ ] Self section columns: `agent, detected, installed, installed_version, available_version, status`. No `skill` or `source` columns in this section.
- [ ] Catalog section columns: `skill, available_version, status, installed_in`. `installed_in` reads `"—"` / `"N/11 (a, b, ...)"` / `"11/11"` per the rules in REQ-F-004.
- [ ] Aggregated `status` honours worst-wins priority: `drift` > `unknown-version` > `partial` > `installed` > `not-installed`. Unit tests cover each transition.
- [ ] `neo4j-cli skill list --format toon` renders the same two-section shape as table (token-efficient compact form for LLM consumers).
- [ ] `neo4j-cli skill list --format json` is byte-identical to today's output for the same inputs. `jq 'length'` returns the same count it did before the change.
- [ ] Cold-cache run still shows only the self section + the existing stderr `skill catalog cache is empty — run '... skill refresh' ...` hint.
- [ ] Network-failure-with-cache run still emits the existing `warning: skill catalog refresh failed, using cached content: ...` stderr message before rendering.
- [ ] `neo4j-cli skill check` is unchanged — same code path, same output (still filters to installed rows, still exits non-zero on drift).
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check`, `make generate-check` all clean.
- [ ] Skill bundle (`neo4j-cli/internal/skill/bundle/**`) regenerated to reflect the new Long-text sentence about the compact default.

## Out of Scope

- No `--full` / `--shape` / `--by-agent` flag. JSON IS the matrix view; that's documented in the Long text.
- No `--agent <name>` filter on `skill list`. Could be a future task if requested.
- No change to `skill check` output. It's already concise.
- No change to JSON output shape. Future PRs that want a `{"self": [...], "catalog": [...]}` shape can do so in a separate breaking change.
- No new multi-section renderer abstraction in `common/output/`. Two `PrintBodyMap` calls are sufficient.
- No change to `BuildInventory`, `InventoryRow`, or `statusFor`.
- No changelog entry — this is a UX refinement of an in-flight feature on the same branch, not a separate release surface. Consolidate with the catalog feature's existing Major entry.

## Open Questions

(none — clarifications captured: toon collapses with table; denominator is `len(AGENTS)`; no `--full` flag; JSON unchanged; catalog row carries skill / available_version / status / installed_in; aggregator status priority drift > unknown-version > partial > installed > not-installed)
