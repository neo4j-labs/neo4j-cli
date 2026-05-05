# PRD: User-Agent header + tx metadata for `query` and `:schema`

## Overview

`neo4j-cli query` (and `query :schema`) currently POST to `/db/<db>/query/v2` with no client identity: no `User-Agent` header, no `txMetadata` in the request body. Server-side ops cannot tell from `query.log` or `SHOW TRANSACTIONS` which client issued a query.

The original plan was to add both a `User-Agent: neo4j-cli/v<version>` header and a `txMetadata` map (`{app: "neo4j-cli", type: <type>}`) to the request body. **End-to-end verification against real Neo4j servers (5.x and 2025.x) showed the v2 endpoint rejects requests carrying `txMetadata` with HTTP 400 — that body field is only accepted from Neo4j 2026.04 onward.** To stay compatible with current production servers, this feature ships only the User-Agent half. The plumbing for `txMetadata` is removed; a comment on `runStatement` records the deferral and the minimum server version so a future iteration can re-add it gated behind a server-version probe. Tracks Linear [CLI-28](https://linear.app/neo4j/issue/CLI-28).

Scope is the `query` package only. Aura subcommand HTTP code paths are untouched.

## Goals

- Identify `neo4j-cli` to the database via the HTTP `User-Agent` header so server-side ops can attribute traffic.
- Reuse existing `cfg.Version` plumbing.
- Single HTTP call site change (`runStatement`) covers both `query` and `:schema`.
- Leave a code-level note about the `txMetadata` deferral so a future re-add lands cleanly.

## Non-Goals

- **`txMetadata` body field — deferred.** Rejected by Neo4j 5.x and 2025.x with HTTP 400; only accepted from Neo4j 2026.04. A future task will re-add it gated behind a server-version probe.
- OS / arch suffix on the User-Agent (`neo4j-cli/v1.2.3 (darwin; arm64)`) — kept bare.
- Aura subcommand HTTP requests (`neo4j-cli/aura/internal/api/`) — already send `Neo4jCLI/%s`; not unified here.
- Configurability of the UA via flags or env — value is a constant of the build version.
- Bolt user-agent (the v2 HTTP API is the only transport `query` uses).

## Requirements

### Functional Requirements

- **REQ-F-001**: Add a `userAgent string` field to the `conn` struct in `neo4j-cli/query/connect.go`.
- **REQ-F-002**: In `resolveConn`, populate `conn.userAgent` from `cfg.Version` as `"neo4j-cli/v" + cfg.Version`. When `cfg.Version` is empty, fall back to `"neo4j-cli/vdev"` (matches the `Version = "dev"` default at `neo4j-cli/app/app.go:22`).
- **REQ-F-003**: In `runStatement`, after `req.SetBasicAuth(...)`, set `req.Header.Set("User-Agent", c.userAgent)`. Skip the `Set` when `c.userAgent` is empty so Go's default UA is not clobbered.
- **REQ-F-004**: `runStatement` keeps its original four-arg signature `(ctx, c, statement, params)`. **It does NOT add a `txMetadata` body field.** A doc comment on `runStatement` records that `txMetadata` would be desirable, names the minimum server version (Neo4j 2026.04+), and points future implementers at a server-version probe.
- **REQ-F-005**: Add a changelog entry: `changie new --projects neo4j-cli --kind Minor --body "query/:schema: send a User-Agent header (neo4j-cli/v<version>) so server logs identify the CLI"`.

### Non-Functional Requirements

- **REQ-NF-001**: No new module dependencies. The implementation uses only the existing `net/http` + `encoding/json` imports already in `connect.go`.
- **REQ-NF-002**: No import of `neo4j-cli/app` from the `query` package — would create a cycle. Version flows via `cfg.Version`, which `query.NewCmd(cfg)` already receives.
- **REQ-NF-003**: All existing query-package tests stay green; no behavioural regressions.
- **REQ-NF-004**: New unit tests cover (a) the resolved User-Agent string for a populated `cfg.Version`, (b) the fallback for empty `cfg.Version`, (c) the User-Agent header on the wire (via `httptest`), (d) that `txMetadata` is NOT in the request body (locks in the deferral).
- **REQ-NF-005**: `make test`, `make fmt-check`, `make lint`, and `make license-check` all pass before merge (per AGENTS.md gate).
- **REQ-NF-006**: No file under `neo4j-cli/aura/` is modified by this change.

## Technical Considerations

**Single call site.** `runStatement` (`neo4j-cli/query/connect.go`) is the only place the `query` package issues HTTP requests. Patching it once covers both leaf commands.

**txMetadata deferred.** Real-server verification against `neo4j:5`, `neo4j:5.20`, `neo4j:2025.x`, and `neo4j:latest` returned HTTP 400 the moment a `txMetadata` field was included in the v2 body. The Neo4j docs source repo labels the field `new-2026.04`. To stay compatible with current production servers, the implementation does not send the field. The `runStatement` doc comment records this so a future iteration can re-enable the field once the server version is probed (the existing `CALL dbms.components()` call in `schema.go` can be promoted into a connection-time probe and stashed on `conn`).

**User-Agent format.** `neo4j-cli/v<version>` mirrors neo4j-browser's `neo4j-browser/v${version}` shape. Bare — no platform suffix. Distinct from Aura's `Neo4jCLI/%s` (used in `neo4j-cli/aura/internal/api/api.go:17`) which targets a different API; we deliberately do not reuse it so log filtering can distinguish DB-level from Aura-control-plane traffic.

**Version plumbing.** `cfg.Version` is already populated end-to-end (`common/clicfg/clicfg.go:34,72` ← `Version` in `neo4j-cli/main.go` and `neo4j-cli/aura/cmd/main.go`). `query.NewCmd(cfg)` already receives `cfg`. No new wiring required.

**Files touched.**
- `neo4j-cli/query/connect.go` — add `userAgent` field; populate in `resolveConn`; set User-Agent header in `runStatement`; doc comment on `runStatement` recording the txMetadata deferral and minimum server version.
- `neo4j-cli/query/connect_test.go` — assert User-Agent header on the wire; assert the body does NOT contain `txMetadata`; populate `userAgent` on every `&conn{...}` literal; new `TestResolveConn_UserAgent` (table-driven).
- `neo4j-cli/query/schema_test.go` — populate `userAgent` on the single `&conn{...}` literal.
- `.changes/unreleased/neo4j-cli-Minor-*.yaml` — changelog entry (User-Agent only).

**Files explicitly NOT touched.** `neo4j-cli/aura/**`, `common/clicfg/**`, `neo4j-cli/app/app.go`, the `:schema` probe Cypher itself, the `query_https_smoke_test.go` test.

## Acceptance Criteria

- [ ] `bin/neo4j-cli query --uri ... "RETURN 1 AS n"` against any Neo4j 5+ server — works (no HTTP 400) and the request's `User-Agent` is `neo4j-cli/v<version>`.
- [ ] `bin/neo4j-cli query :schema --uri ...` — same: works and carries the User-Agent.
- [ ] `TestRunStatement_HappyPath` asserts `User-Agent == "neo4j-cli/vtest"` and asserts the request body does NOT contain `txMetadata`.
- [ ] `TestResolveConn_UserAgent` (table-driven) asserts `c.userAgent == "neo4j-cli/v1.2.3"` for `cfg.Version == "1.2.3"` and `c.userAgent == "neo4j-cli/vdev"` for `cfg.Version == ""`.
- [ ] `runStatement` doc comment names Neo4j 2026.04 as the minimum server version for `txMetadata` and points at a server-version probe as the gating mechanism.
- [ ] `make test` green.
- [ ] `make fmt-check` green (no gofmt drift).
- [ ] `make lint` green.
- [ ] `make license-check` green.
- [ ] `.changes/unreleased/neo4j-cli-Minor-*.yaml` exists with the User-Agent-only body.
- [ ] No file under `neo4j-cli/aura/` is modified.

## Out of Scope

- OS / arch suffix on User-Agent.
- Aura subcommand HTTP code paths.
- Bolt-protocol user-agent (CLI doesn't use Bolt).
- Configurable UA / tx metadata via flag or env.
- Additional `txMetadata` fields (e.g. `user`, `host`, `cwd`, request id).
- Changes to `:schema` probe query set or output formatting.
- Refactor of `runStatement` into a struct-based options API.

## Open Questions

(none — all clarifying questions resolved during planning.)
