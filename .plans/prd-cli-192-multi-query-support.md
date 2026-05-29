# PRD: Multi-query support for `neo4j-cli query`

Linear: [CLI-192](https://linear.app/neo4j/issue/CLI-192)

## Overview

`neo4j-cli query` accepts exactly one Cypher statement per invocation today.
Agents and users repeatedly hit this — e.g. wanting to send several `GRANT ...;`
statements in one call (the originating Slack thread shows Claude resorting to a
side Python script). Cypher-shell users naturally separate statements with `;`.

This feature lets `query` accept a single string containing multiple statements,
split on a **semicolon that ends a line** (`;` + EOL, per the issue), and execute
them in order. Default execution is **fail-fast, one transaction per statement**
(matches the current one-session-per-statement architecture and the GRANT use
case). A new `--atomic` flag wraps all statements in a **single transaction that
rolls back on any failure**. Output for multiple result sets is a **JSON array of
result envelopes** (`--format json`) or **stacked blocks** (table/toon).

Single-statement behaviour and output remain byte-identical to today.

## Goals

- Accept multiple Cypher statements in one `query` invocation, separated by `;` at
  line end.
- Execute statements sequentially in source order.
- Default: each statement runs in its own transaction; fail-fast on first error
  (already-committed statements stay committed).
- Add `--atomic`: run all statements in a single transaction; roll back on any
  failure.
- Render multiple result sets as a JSON array (`--format json`) or sequentially
  stacked blocks (`--format table` / `--format toon`).
- Keep single-statement behaviour and output unchanged.

## Non-Goals

- Splitting on a `;` that appears mid-line (only `;` at end of line is a split
  point — the documented, string-literal-safe rule from the issue).
- A full Cypher lexer / string-literal-aware semicolon parser.
- Multi-statement support for the `:schema` or `:embed` sub-leaves (they have their
  own RunE and do not flow through `runQuery`).
- Surfacing Bolt-summary counters (e.g. "nodes created") in output — out of scope,
  consistent with current behaviour.
- `--continue-on-error` mode (collect-all-errors). Default is fail-fast; only
  fail-fast and `--atomic` are in scope.
- Cross-statement parameter chaining or referencing one statement's result in the
  next.

## Requirements

### Functional Requirements

- REQ-F-001: A single positional/stdin Cypher string is split into statements on a
  `;` at end of line (`;` followed by optional trailing whitespace then `\n`, or
  `;` at end of input).
- REQ-F-002: A `;` that is **not** at end of line is not a split point and is kept
  verbatim within the statement.
- REQ-F-003: Each split statement has its terminating `;` stripped before being
  sent to the driver (Bolt `tx.Run` rejects a trailing `;`).
- REQ-F-004: Statements are executed in source order.
- REQ-F-005: Empty fragments (e.g. from a trailing `;`, blank lines, or
  whitespace-only segments) are dropped; non-empty input always yields ≥1
  statement.
- REQ-F-006: CRLF (`\r\n`) line endings are handled identically to LF.
- REQ-F-007: A single statement (with or without a trailing `;`) produces output
  byte-identical to today's single-statement output (single JSON object / single
  table).
- REQ-F-008: **Default mode** (no `--atomic`): each statement runs in its own
  transaction (current architecture). Execution stops at the first error and
  returns it; statements already executed remain committed (fail-fast).
- REQ-F-009: **Default mode** without `--rw`: each statement is independently
  classified via the existing EXPLAIN preflight (`rejectWriteCypher`); a statement
  classified as a write is blocked with the existing `this command writes; pass
  --rw to allow it` usage error.
- REQ-F-010: **`--atomic` mode**: all statements run inside a single managed
  transaction. Any error aborts the transaction and the driver rolls it back (no
  statement persists).
- REQ-F-011: `--atomic` without `--rw` is allowed (read-only batch runs in a single
  read transaction); the per-statement write-guard preflight still applies.
- REQ-F-012: `--atomic` with `--rw` runs all statements in a single write
  transaction.
- REQ-F-013: `--param` parameters (including `:embed` modifiers) are resolved once
  and shared across all statements.
- REQ-F-014: Multiple result sets in `--format json` render as a JSON array of the
  existing `{columns, rows, truncated, arrays_truncated}` envelopes, one per
  statement, in order.
- REQ-F-015: Multiple result sets in `--format table` render as sequential table
  blocks separated by a blank line; in `--format toon` as the array form.
- REQ-F-016: Per-statement row/array truncation (`--max-rows`,
  `--truncate-arrays-over`) applies to each result independently; truncation
  warnings are prefixed with `statement N:` only when more than one statement ran.
- REQ-F-017: A new persistent `--atomic` bool flag is registered on the `query`
  command, default `false`, with help text describing single-transaction /
  rollback semantics.
- REQ-F-018: The `query` command's `Long` and `Example` text documents
  multi-statement splitting and `--atomic` (multi-statement read example +
  `--rw --atomic` multi-write example, flush-left per bundle-rendering rules).
- REQ-F-019: A `changie` Minor changelog entry is added under
  `.changes/unreleased/`.

### Non-Functional Requirements

- REQ-NF-001: No regression on existing query tests (`go test
  ./neo4j-cli/query/...` and `make test` pass).
- REQ-NF-002: `make fmt-check` and `make lint` clean.
- REQ-NF-003: `make generate-check` / `TestGenerator_RoundTrip` clean — skill
  bundle regenerated (`go generate ./neo4j-cli/internal/skill/...`) and committed
  alongside the `Long`/`Example`/flag changes.
- REQ-NF-004: `TestAllLeafCommands_HaveExamples` passes (query examples remain
  valid: ≥2 invocations, comments, `--rw` on writes, `--format json` on reads).
- REQ-NF-005: Any new file carries the Neo4j copyright header (`make
  license-check` / CI `addlicense`).
- REQ-NF-006: New tests follow the table-driven idiom and per-command test-file
  naming used in `neo4j-cli/query/*_test.go`.

## Technical Considerations

### Statement splitting

New `splitStatements(cypher string) []string` (in `neo4j-cli/query/run.go` or a
new `split.go`). Line-walk implementation (avoids regex edge cases, handles CRLF):
accumulate lines; when a right-trimmed line ends with `;`, strip the `;`, flush the
accumulated buffer as one `strings.TrimSpace`d statement, drop empties; flush any
remainder at end.

### `runQuery` refactor — `neo4j-cli/query/run.go`

After `resolveCypher`, compute `statements := splitStatements(cypher)`. Keep
one-time setup (params, embeds, conn, password, `openDriver`) exactly as-is —
embeds resolve once into the shared `params` map. Read `--atomic` via
`cmd.Flag("atomic")` (mirrors how `--rw` is read). Branch:

- **Default:** loop statements; per statement run `rejectWriteCypher` when
  `!allowWrite`, then `runStatement` / `runStatementWrite`; fail-fast; apply
  `truncateValues`/`capRows`; collect a `renderResult` each.
- **`--atomic`:** run the per-statement EXPLAIN preflight loop first when
  `!allowWrite` (read-only, outside the tx) to preserve the write-guard; then run
  the whole batch in one transaction.

Replace the single `renderRows(...)` tail with `renderResults(cmd, cfg, results)`.

### Batch (atomic) execution — `neo4j-cli/query/connect.go`

Mirror the single-statement seam pattern:

- `var runStatementsResponseFn = runStatementsResponseImpl` — batch test seam.
- `runStatementsResponse(ctx, c, statements []string, params, readOnly bool)
  ([]*queryResponse, error)` — dispatch through the seam; wrap errors via
  `categorizeBoltError` (single boundary, same as `runStatementResponse`).
- `runStatementsResponseImpl` — open **one** session, run **one**
  `ExecuteRead`/`ExecuteWrite` whose work callback loops `tx.Run → Collect →
  Consume` per statement, appending a `*queryResponse` each; reuse
  `coerceDriverValue`. Any error returned from the callback aborts → managed tx
  rolls back automatically.
- `runStatementsWithMode(...) ([]*queryResult, error)` unwraps the batch, mirroring
  `runStatementWithMode`.

### Multi-result rendering — `neo4j-cli/query/output.go` + `common/output/output.go`

`renderResults(cmd, cfg, results []renderResult)`:

- `len == 1` → existing single path (`commonoutput.PrintBodyMap(cmd, cfg,
  results[0], results[0].columns)`) — output byte-identical to today.
- `len > 1` → new `commonoutput.PrintBodyMaps(cmd, cfg, items []ResponseData,
  fields [][]string)`:
  - `json`: `json.MarshalIndent(items, ...)` → array (each elem via
    `renderResult.MarshalJSON`).
  - `toon`: marshal the slice through the existing toon path.
  - `table`: loop `printTable(cmd, item, fields[i])`, blank line between blocks.

`PrintBodyMaps` reuses `ResolveOutput`/`printTable`/`printToon` so format
resolution and TTY auto-detection stay centralised.

### Flag / help / docs / gates

- `neo4j-cli/query/query.go`: register `--atomic` persistent bool; extend
  `Long` + `Example`.
- `go generate ./neo4j-cli/internal/skill/...` → commit regenerated
  `neo4j-cli/internal/skill/bundle/**` (query `Long`/`Example`/flags feed
  `references/query.md`).
- Update `README.md` query section and `neo4j-cli/internal/skill/additions.md`
  (verify it documents `query` before editing): multi-statement splitting rule,
  `--atomic`, multi-result output shape.
- `changie new --projects neo4j-cli --kind Minor --body "query: accept multiple
  statements separated by ; (line end); --atomic runs them in one transaction"`.

### Potential challenges

- **Seam wiring in `--atomic` tests:** the EXPLAIN preflight uses the existing
  per-statement seam (`runStatementResponseFn`) while the atomic execution uses the
  new batch seam (`runStatementsResponseFn`); atomic non-`--rw` tests must install
  both.
- **Rollback is driver-side:** unit tests can only assert the batch seam was
  invoked once and that an error surfaces; true rollback is verified in the live-DB
  manual verification step.
- **Backward compatibility:** the `len == 1` branch must not change a single byte
  of existing output — guarded by reusing the current `PrintBodyMap` call verbatim
  and by an explicit "single statement, output identical" test.

## Acceptance Criteria

- [ ] `splitStatements` splits on `;` at line end only, strips terminating `;`,
      drops empties, handles CRLF, and is covered by table-driven tests.
- [ ] Default multi-statement: statements execute in order, fail-fast on first
      error, each in its own transaction.
- [ ] Non-`--rw` multi-statement blocks a write statement with the existing
      `--rw` usage error via per-statement EXPLAIN preflight.
- [ ] `--atomic` runs all statements in one transaction; an error surfaces and (on
      a live DB) rolls back all statements; allowed both with and without `--rw`.
- [ ] `--format json` with >1 statement emits a JSON array of result envelopes;
      `--format table` emits stacked blocks separated by a blank line; `--format
      toon` emits the array form.
- [ ] Single statement (with or without trailing `;`) produces output identical to
      today.
- [ ] Per-statement truncation warnings prefixed `statement N:` only when >1
      statement ran.
- [ ] `--param`/`:embed` resolved once and shared across statements.
- [ ] `--atomic` flag registered with help text; `Long`/`Example` updated; skill
      bundle regenerated and committed.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check`, and
      `make generate-check` all clean.

## Out of Scope

- Mid-line `;` splitting and a string-literal-aware Cypher lexer.
- `--continue-on-error` / collect-all-errors mode.
- Multi-statement support for `:schema` / `:embed` sub-leaves.
- Cross-statement parameter/result chaining.
- Surfacing Bolt-summary counters in output.

## Open Questions

None. (Resolved during planning: `--atomic` without `--rw` is allowed; the
`statement N:` warning prefix is accepted; README/additions.md placement
confirmed pending a check that additions.md documents `query`.)
