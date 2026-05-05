# PRD: neo4j-cli query command

## Overview

Add a top-level `query` command to the `neo4j-cli` super-CLI that executes Cypher against a Neo4j database via the HTTP Query API (`POST /db/{database}/query/v2`). Inspired by `oskarhane/homebrew-neo4j-query` and tracked in Linear CLI-21. The command supports auth via flags / env / `.env`, a `:schema` introspection subcommand, JSON-typed parameters, recursive array truncation, and a result-row cap. Mounted only on `neo4j-cli` for now; not built as a standalone binary.

## Goals

- Let users run ad-hoc Cypher from the shell without spinning up `cypher-shell` or a notebook.
- Cover the common workflows of the homebrew tool with the same flag surface so users can swap in `neo4j-cli query` with minimal muscle-memory cost.
- Stay stdlib-only on the transport — no bolt driver dependency. Use the Neo4j HTTP Query API.
- Match existing `neo4j-cli` conventions (cobra one-file-per-leaf, `--output json|table|default`, `clicfg.Config` plumbing) so the surface feels native to the CLI.
- Keep the change tight enough to ship in one PR.

## Non-Goals

- TOON output format.
- `--param` modifiers (e.g. `:embed`).
- Embeddings / providers / models.
- Named DB credential profiles in `credentials.json`.
- A standalone `query-cli` / `cypher` binary, separate skill template, separate npm package.
- Bolt protocol support, transactions, multi-statement requests.
- `--param @file.json` file-loading shorthand (deferred to v2).

## Requirements

### Functional Requirements

- REQ-F-001: A new top-level command `neo4j-cli query [flags] [cypher]` runs Cypher and prints rows.
- REQ-F-002: Cypher input source order: positional argument first; if absent and stdin is not a TTY, read from stdin; otherwise error with a clear "no cypher provided" message.
- REQ-F-003: A `:schema` subcommand (`neo4j-cli query :schema`) introspects the connected database and emits database info plus node/relationship properties, relationship paths, indexes, and constraints. Mirrors `oskarhane/homebrew-neo4j-query/src/main.rs` L292–565.
- REQ-F-004: Persistent connection flags on the `query` parent: `--uri` (`NEO4J_URI`, default `http://localhost:7474`; bolt-family URIs auto-rewritten per REQ-F-017), `-u`/`--username` (`NEO4J_USERNAME`, default `neo4j`), `-p`/`--password` (`NEO4J_PASSWORD`), `-d`/`--database` (`NEO4J_DATABASE`, default `neo4j`), `--env <file>` (explicit `.env` path; auto-walk-up otherwise), `--insecure` (`NEO4J_INSECURE`, skip TLS verification).
- REQ-F-005: Resolution precedence (lowest → highest): `.env` < OS env (`NEO4J_*`) < command-line flags.
- REQ-F-006: When the resolved password is empty AND stdin is a TTY, prompt the user securely (no echo) via `golang.org/x/term`. When stdin is not a TTY, fail with `password required (set --password, NEO4J_PASSWORD, or .env NEO4J_PASSWORD=...)`.
- REQ-F-007: `--param key=value` (repeatable). Value parsing: try `json.Unmarshal`; on failure, treat as a raw string. Supports number, bool, null, JSON array, JSON object, plain string.
- REQ-F-008: `--max-rows N` caps printed rows (default `100`; `0` = unlimited). When the API returns more than `N` rows, drop the extras, set `truncated: true` in JSON output, and print `warning: truncated to <N> rows (use --max-rows 0 for unlimited)` to stderr regardless of output mode.
- REQ-F-009: `--truncate-arrays-over N` recursively replaces arrays with `len > N` inside row values (default `100`; `0` = off). Truncated arrays render as an empty array (`[]`) — a type-safe signal that the value was elided without polluting downstream JSON consumers with a placeholder string. Applied before row-limit handling. Because the in-band shape change is silent, the command emits a dual-signal aggregate warning when at least one array is elided: (a) exactly one stderr line `warning: truncated N arrays larger than M items (use --truncate-arrays-over 0 to disable)` (where N is the count of distinct slices truncated and M is the configured threshold) — suppressed when N is zero or `--truncate-arrays-over 0`; (b) an `arrays_truncated: <int>` field in the JSON envelope, always emitted (zero when nothing was elided) so downstream JSON consumers can rely on a stable schema. Table output body is unchanged — the stderr warning is the only signal in table mode.
- REQ-F-010: `--output json|table|default` (validated against `clicfg.ValidOutputValues`, bound via `cfg.Aura.BindOutput`). Default → `table`.
- REQ-F-011: Table output uses `github.com/jedib0t/go-pretty/v6/table`, header row = column names in result order (preserve user's `RETURN` ordering), each cell is the JSON-stringified value. Style `table.StyleLight`.
- REQ-F-012: JSON output shape: `{"columns": [...], "rows": [{"col": value, ...}, ...], "truncated": <bool>, "arrays_truncated": <int>}`. Rows are `col → value` objects (not positional arrays) for jq ergonomics. `arrays_truncated` is always present (zero when no arrays were elided) per REQ-F-009.
- REQ-F-013: `:schema` honors `--output`. JSON default emits one structured object containing `database`, `nodes`, `relationships`, `relationship_paths`, `indexes`, `constraints`. Table mode prints five stacked sub-tables with H2 separators (one per section). `--max-rows` does not apply.
- REQ-F-014: Server errors (non-2xx, JSON `errors[]`) are surfaced as cobra usage/command errors with the upstream `code` and `message` visible.
- REQ-F-015: A single-line entry `cmd.AddCommand(query.NewCmd(cfg))` is added to `neo4j-cli/app/app.go`. The skill bundle is regenerated via `make generate` so the new command appears in `neo4j-cli/internal/skill/bundle/`.
- REQ-F-016: A user-facing changelog entry is added under `.changes/unreleased/` for the `neo4j-cli` project (changie kind `Minor`).
- REQ-F-017: The resolved `--uri` value is auto-rewritten when it uses a bolt-family scheme so users coming from drivers / `cypher-shell` are not blocked by a confusing transport error. Rewrite table: `bolt://` → `http://`; `bolt+s://`, `bolt+ssc://`, `neo4j://`, `neo4j+s://`, `neo4j+ssc://` → `https://`; `http://` and `https://` pass through unchanged. Port mapping: source port `7687` is replaced with `7473` (when rewriting to `https://`) or `7474` (when rewriting to `http://`); a missing port (e.g. Aura `neo4j+s://abc.databases.neo4j.io`) stays missing (URL library defaults to 443/80); any other custom port is preserved. **Aura special case**: when the host (without port) ends in `.neo4j.io` (case-insensitive; apex `neo4j.io` matches too) AND the scheme is bolt-family, the URI is forced to `https://<host>` with NO port — Aura always serves HTTPS on 443 regardless of input scheme or port (so even custom ports like `:9999` are dropped). Suffix matching is strict: `*.neo4j.io.evil.com` does NOT match and falls through to the generic 7687→7474 rule. Already-`https://` Aura URLs pass through unchanged. Userinfo on the input URI is preserved on the rewritten URI but masked via `(*url.URL).Redacted()` (`xxxxx`) before it appears in the stderr notice. Notice format (one line per rewrite, written to stderr before any other output): `info: rewrote URI '<orig>' to '<new>' (the query command uses Neo4j's HTTP Query API; pass --uri https://... to silence)`. Already-correct `http(s)://` URIs and unrecognised schemes pass through silently — downstream HTTP errors surface naturally for genuine garbage. The `--uri` flag's help string documents the rewrite (including the Aura special case).

### Non-Functional Requirements

- REQ-NF-001: No new third-party dependencies beyond what is already in `go.mod`. Transport is stdlib `net/http` + `encoding/json` + `encoding/base64`. Password prompt uses `golang.org/x/term` (already transitively available; verify before relying on it — if missing, add).
- REQ-NF-002: Strict cobra one-file-per-leaf layout per `AGENTS.md` §"Cobra Command Layout": parent `query.go` ≤ 80 lines; each leaf and helper in its own file (`run.go`, `schema.go`, `connect.go`, `params.go`, `truncate.go`, `output.go`); colocated `*_test.go`; shared fixtures in `query_helpers_test.go`.
- REQ-NF-003: Every new `.go` file starts with the Neo4j copyright header (CI enforces via `addlicense`).
- REQ-NF-004: All gates pass before merge: `make test`, `make fmt-check`, `make lint`, `make license-check`, `make generate-check`.
- REQ-NF-005: Tests are colocated and prefer table-driven style (`for _, tc := range []struct{...}{...}`) per `AGENTS.md` §"Testing Framework".
- REQ-NF-006: Tests are hermetic — no live Neo4j required for unit/CI runs. Use `httptest.NewServer` for transport-level tests, `afero.NewMemMapFs` + `t.Setenv("HOME", ...)` for `.env` walk-up tests. A `query_smoke_test.go` skipped unless `NEO4J_TEST_URI` is set provides a real-server check for local development.
- REQ-NF-007: Cross-platform clean — header assertions use `strings.ToLower(...)` (go-pretty upper-cases headers); any path expectations build via `filepath.Join` / `filepath.FromSlash` (Windows CI).
- REQ-NF-008: TLS verification is on by default. `--insecure` (or `NEO4J_INSECURE=true`) sets `tls.Config{InsecureSkipVerify: true}` on the HTTP client. Help text flags it as a development-only escape hatch.

## Technical Considerations

**Mount point.** `query` lives at `neo4j-cli/query/` and is mounted in `neo4j-cli/app/app.go` next to `aura`, `credential`, `skill`. It is NOT mounted inside `aura.NewCmd` or `aura.NewStandaloneCmd` — `aura-cli` is the Aura control-plane CLI; direct DB queries belong on the super-CLI only.

**Transport.** The Neo4j HTTP Query API endpoint is `POST {uri}/db/{database}/query/v2` with HTTP Basic auth, `Content-Type: application/json`, body `{"statement": "...", "parameters": {...}}`, and a response of `{"data": {"fields": [...], "values": [[...]]}, "errors": [...]}`. No transactional endpoint, no bolt routing — single auto-commit per request.

**Cobra layout.** Following `AGENTS.md` §"Cobra Command Layout" and the precedent of `neo4j-cli/aura/internal/subcommands/instance/`:

```
neo4j-cli/query/
  query.go              # NewCmd(cfg) — parent, persistent flags, AddCommand(:schema), RunE delegates to runQuery
  run.go                # runQuery(...) default cypher execution
  schema.go             # newSchemaCmd(cfg) — :schema leaf
  connect.go            # resolveConn + runStatement; .env walk-up; flag/env merge; HTTP client (with --insecure)
  params.go             # parseParams([]string) (map[string]any, error)
  truncate.go           # truncateArrays(value any, max int) any — recursive
  output.go             # renderRows / renderSchema (json + table)
  *_test.go             # one per source
  query_helpers_test.go # shared fixtures
```

The parent has `RunE` (cobra supports parent-with-RunE alongside subcommands); the body lives in `run.go` to keep `query.go` small.

**Reuse vs. add.** `aura/internal/output/output.go::PrintBodyMap` is too API-shaped for arbitrary record columns — write fresh render helpers in `query/output.go`. `clicfg.Config`, `clicfg.ValidOutputValues`, and `cfg.Aura.BindOutput` are reused for `--output` plumbing. `go-pretty/v6/table` and `afero` are existing deps.

**`:schema` queries.** Five Cypher calls plus an optional database-info pair, run sequentially:
1. `CALL db.schema.nodeTypeProperties() YIELD nodeType, nodeLabels, propertyName, propertyTypes, mandatory`
2. `CALL db.schema.relTypeProperties() YIELD relType, propertyName, propertyTypes, mandatory`
3. Per relType: ``MATCH (n)-[r:`<type>`]->(m) WITH DISTINCT labels(n) AS from, labels(m) AS to RETURN from, to``
4. `SHOW INDEXES YIELD name, type, entityType, labelsOrTypes, properties, state, owningConstraint, options`
5. `SHOW CONSTRAINTS YIELD name, type, entityType, labelsOrTypes, properties, ownedIndex, propertyType`
6. Optional, errors swallowed: `CALL dbms.components()` and `SHOW SETTINGS YIELD name, value WHERE name = 'db.query.default_language'`.

**Test seam.** `connect.go` exposes a small `httpDoer interface{ Do(*http.Request) (*http.Response, error) }` so `httptest.Server` URLs can be wired into the command via `--uri`; no monkey-patching, no testcontainers needed for unit tests.

**Skill bundle.** Adding a top-level command requires regenerating `neo4j-cli/internal/skill/bundle/` via `make generate`. The generator walks the cobra tree from `app.NewCmd`, so the new `query` row and `references/query.md` appear automatically. CI's `make generate-check` enforces commit hygiene.

**Risks / open items.** TLS bypass (`--insecure`) is a footgun — flagged in `--help`. Password prompt requires `golang.org/x/term`; if not transitively available, adding it is a small extra dep. The HTTP Query API requires Neo4j 5+ with the `query` API enabled (default in 5.x).

## Acceptance Criteria

- [ ] `neo4j-cli query --password testtest "RETURN 1 AS n"` prints a table with column `n`, value `1`, against a local Neo4j 5 (`docker run -d --rm -p 7474:7474 -e NEO4J_AUTH=neo4j/testtest neo4j:5`).
- [ ] `--output json` produces `{"columns":["n"],"rows":[{"n":1}],"truncated":false}`.
- [ ] `--param k=5 "RETURN $k AS x"` sends `5` as an integer; `--param name=alice "RETURN $name AS x"` sends `"alice"` as a string (string fallback).
- [ ] `--max-rows 2 "UNWIND range(1,10) AS i RETURN i"` returns 2 rows, sets `truncated:true` in JSON, and prints the stderr warning.
- [ ] `--truncate-arrays-over 3 "RETURN range(1,10) AS xs"` produces a row whose `xs` value is an empty array (`[]`).
- [ ] `neo4j-cli query :schema` emits the expected structured object (JSON default) and 5 stacked tables under `--output table`.
- [ ] `echo "RETURN 1" | neo4j-cli query --password testtest` reads from stdin successfully.
- [ ] `NEO4J_PASSWORD=testtest neo4j-cli query "RETURN 1"` succeeds via env.
- [ ] A `.env` file containing `NEO4J_PASSWORD=testtest` placed in cwd is auto-discovered.
- [ ] Running `neo4j-cli query "RETURN 1"` interactively (TTY, no password set) prompts for a password without echoing.
- [ ] Running `neo4j-cli query "RETURN 1"` non-interactively (no password set) errors with the documented message.
- [ ] `--insecure` allows connecting to a self-signed HTTPS Neo4j; without the flag, the same connection fails with a TLS error.
- [ ] A server-side error (e.g. invalid Cypher) surfaces the upstream `code` and `message` and exits non-zero.
- [ ] `neo4j-cli query` is listed in the `neo4j-cli/internal/skill/bundle/SKILL.md` Subcommands table after `make generate`.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check`, `make generate-check` all pass.
- [ ] A changelog entry exists at `.changes/unreleased/neo4j-cli-Minor-<ts>.yaml`.

## Out of Scope

- TOON output format.
- `--param` modifiers (`:embed`, etc.).
- Embedding generation, model providers, model selection.
- Named Neo4j-DB credential profiles in `credentials.json`.
- Standalone `query-cli` / `cypher` binary, separate npm package, separate skill template.
- Bolt protocol, transactions, multi-statement requests, streaming results.
- `--param @file.json` file-loading shorthand.
- Aura control-plane integration on the `query` command (use `aura` for that).

## Open Questions

- Does `golang.org/x/term` need to be added explicitly to `go.mod`, or is it already available transitively? (Confirm at implementation time; the change is trivial either way.)
- Should the `--insecure` flag print a one-line stderr warning ("WARNING: TLS verification disabled") on every invocation, or stay silent and let `--help` carry the warning?
- Truncation signal — resolved during implementation: over-limit arrays are replaced with an empty array (`[]`). Type-safe and consumer-friendly; no placeholder string in JSON output.
- For the `:schema` table layout, confirm the section header format (Markdown-style `## Nodes` lines vs. blank-line separators) once we see it rendered.
