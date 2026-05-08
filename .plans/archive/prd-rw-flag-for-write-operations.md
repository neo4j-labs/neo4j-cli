# PRD: `--rw` flag for write operations

Linear: https://linear.app/neo4j/issue/CLI-29
Plan: `~/.claude/plans/i-want-to-implement-precious-rabbit.md`

## Overview

Add a global persistent `--rw` boolean flag to both shipped binaries
(`neo4j-cli` super-CLI and standalone `aura-cli`) that gates **every**
state-mutating command. Writes refuse to execute without `--rw`; reads
remain frictionless. Today every leaf can mutate state with no opt-in,
so an autocomplete / fat-finger / agent-driven `aura instance delete
<id>` runs immediately. `--rw` is a guard rail that costs one keystroke
and prevents an entire class of accidents.

For `query run`, where the cypher itself determines read-vs-write, the
CLI does a preflight `EXPLAIN <cypher>` round trip and inspects the
resulting query type. Only non-read-only queries require `--rw`.

Use cobra `Annotations["write"] = "true"` on every write leaf and a
single root `PersistentPreRunE` that walks the annotation and rejects
the call when the flag is unset. No per-leaf branching.

**Bolt migration (extension).** The HTTP Query API has no `queryType`
field on EXPLAIN responses (verified against `neo4j:5.26` and
`neo4j:2026.04` — only `data` / `queryPlan` / `bookmarks`). The
official Neo4j Go driver v6 returns `ResultSummary.StatementType()`
(`r` / `rw` / `w` / `s`) but is Bolt-only. To get a single-field
classifier instead of an operator-tree walk, the `query` package
switches transport to Bolt via `github.com/neo4j/neo4j-go-driver/v6`.
HTTP transport, the `--insecure` flag, and the operator-tree
classifier are removed. Query execution uses the driver's
auto-managed transaction wrappers `session.ExecuteRead` /
`session.ExecuteWrite` — no manual `BeginTransaction`, no commit/
rollback bookkeeping. The EXPLAIN preflight runs inside
`ExecuteRead` (it never mutates), the real query runs inside
`ExecuteRead` when classified read-only and `ExecuteWrite`
otherwise.

## Goals

- Block accidental mutations across the Aura API, local config,
  credentials, skill install/remove, and write cypher.
- Keep all reads frictionless — no flag changes for `list`, `get`,
  `query schema`, `query run` of read-only cypher.
- One uniform mechanism (cobra Annotations + composed root hook) so
  future write leaves only add a one-line annotation.
- Surface the requirement in help text, both `SKILL.md` bundles
  (auto-generated), the `additions.md` gotchas, the README, and the
  npm user-facing README.
- Ship as a dual-project `Minor` changie entry (aura-cli + neo4j-cli).
- Replace the bespoke HTTP transport with the official Neo4j Go
  driver v6 so write detection collapses to a single field
  (`summary.StatementType()`) and `query run` becomes idiomatic
  driver code.

## Non-Goals

- No env-var alternative (`NEO4J_CLI_RW=1`). Flag-only by user
  decision — explicit per-invocation, harder to leave on by accident.
- No detection of write cypher by string parsing — the preflight
  EXPLAIN is the only classifier.
- No granular gating (e.g. separate flags for "API" vs "local config").
  All writes are gated uniformly.
- No `--force` / `--yes` to combine with `--rw`. `--rw` is the single
  opt-in; existing `--await` and other flags are unaffected.
- No interactive confirmation prompt. The flag is the confirmation.
- No allowlist exception for "harmless" local writes like
  `aura config set format json` — gated for consistency.
- No manual transaction management. The driver's auto-managed
  `session.ExecuteRead` / `session.ExecuteWrite` are the only
  paths; no explicit `BeginTransaction`, no manual commit/
  rollback. (The wrappers are simpler than `session.Run` for our
  use case: they handle retries on transient errors and close
  the tx automatically on success or failure.)
- No backwards-compat handling for credentials stored with `http://`
  URIs. The release is experimental; nobody has stored creds yet.
- No driver tuning (pool size, routing tweaks, custom resolvers).
  Driver defaults only.

## Requirements

### Functional Requirements

- REQ-F-001: `common/flags/flags.go` MUST export
  `RegisterRwFlag(cmd *cobra.Command)` that registers a persistent
  boolean flag `--rw` (no shorthand, default `false`) on `cmd`. Help
  text: `"Allow write operations. Required for any command that
  mutates state (Aura API, local config, credentials, skills, write
  cypher)."`.
- REQ-F-002: The two existing root commands MUST mount the flag:
  - `neo4j-cli/app/app.go::NewCmd` after the existing
    `RegisterOutputFlag` call.
  - `neo4j-cli/aura/aura.go::NewStandaloneCmd` (the standalone
    aura-cli root). `NewCmd` (when aura is mounted as a subcommand
    under `neo4j-cli`) MUST NOT register the flag separately — it
    inherits the super-CLI's persistent flag.
- REQ-F-003: Every write leaf listed below MUST set
  `cmd.Annotations = map[string]string{"write": "true"}` inside its
  `newXxxCmd` constructor (33 leaves):
  - `neo4j-cli/aura/internal/subcommands/instance/`: create, delete,
    update, pause, resume, overwrite
  - `neo4j-cli/aura/internal/subcommands/instance/snapshot/`: create
  - `neo4j-cli/aura/internal/subcommands/customermanagedkey/`: create,
    delete
  - `neo4j-cli/aura/internal/subcommands/deployment/`: create, delete
  - `neo4j-cli/aura/internal/subcommands/deployment/token/`: create,
    delete, update
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/`: create,
    delete, update, pause, resume
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/authprovider/`:
    create, delete
  - `neo4j-cli/aura/internal/subcommands/dataapi/graphql/corspolicy/allowedorigin/`:
    add, remove
  - `neo4j-cli/aura/internal/subcommands/graphanalytics/session/`:
    create, delete
  - `neo4j-cli/aura/internal/subcommands/import/job/`: create, cancel
  - `neo4j-cli/aura/internal/subcommands/credential/`: add, remove,
    use
  - `neo4j-cli/aura/internal/subcommands/config/`: set
  - `neo4j-cli/aura/internal/subcommands/config/project/`: add,
    remove, use
  - `neo4j-cli/internal/subcommands/config/`: set
  - `neo4j-cli/internal/subcommands/credential/database/`: add,
    remove, use
  - `common/skill/`: install.go, remove.go
- REQ-F-004: A single root `PersistentPreRunE` MUST compose the
  existing format-binding logic (currently inside `RegisterOutputFlag`)
  with a new write-gate check. Refactor `RegisterOutputFlag` to expose
  its body as a callable helper rather than directly setting
  `PersistentPreRunE`; the root then composes both helpers in one
  hook.
- REQ-F-005: The write-gate check MUST evaluate `cmd.Annotations["write"]
  == "true"` and the resolved value of `--rw`. If write and `--rw` is
  false, return
  `clierr.NewUsageError("this command writes; pass --rw to allow it")`.
- REQ-F-006: `cobra.EnableTraverseRunHooks = true` is already set in
  `neo4j-cli/app/app.go:45` and the `aura-cli` `cmd/main.go`. Tests
  MUST cover that the root hook still fires for deeply nested writes
  (e.g. `dataapi graphql corspolicy allowedorigin add`).
- REQ-F-007: `neo4j-cli/query/run.go::runQuery` MUST run a preflight
  EXPLAIN before the real `runStatement` call, but ONLY when `--rw` is
  not set. When `--rw` is already true, skip the preflight (saves a
  round trip; the user has already opted in).
- REQ-F-008: The preflight MUST send `"EXPLAIN " + cypher` with the
  user's params via the existing `runStatement` helper using the same
  resolved `conn` (so auth, TLS, database, user-agent are reused).
- REQ-F-009: `neo4j-cli/query/connect.go::queryResponse` MUST be
  extended to capture the query-type field returned by the Neo4j HTTP
  Query API v2 EXPLAIN response. The exact JSON path and enum values
  MUST be confirmed against a docker `neo4j:latest` (mirror the boot
  pattern in `neo4j-cli/query/query_https_smoke_test.go`); only after
  empirical confirmation is the struct field written.
- REQ-F-010: When the EXPLAIN response classifies the cypher as
  anything other than read-only, `runQuery` MUST return
  `clierr.NewUsageError("this command writes; pass --rw to allow it")`
  before the real `runStatement` runs.
- REQ-F-011: When EXPLAIN itself errors (cypher syntax error, network
  error, etc.), the error MUST surface as-is — the user gets the same
  error they would have seen without the preflight, just earlier.
- REQ-F-012: Both root commands MUST gain a `Long` description (or
  extend `Short`) referencing the `--rw` requirement. Locations:
  `neo4j-cli/app/app.go:28-32` and `neo4j-cli/aura/aura.go:23-27`.
- REQ-F-013: `neo4j-cli/internal/skill/additions.md` and
  `neo4j-cli/aura/internal/skill/additions.md` MUST gain a "Write
  operations" gotcha block: agents MUST pass `--rw` for any mutating
  command; for `query run`, the CLI runs EXPLAIN to detect write
  cypher and refuses without `--rw`.
- REQ-F-014: `make generate` MUST be re-run so the regenerated
  `bundle/SKILL.md` files in both
  `neo4j-cli/internal/skill/bundle/` and
  `neo4j-cli/aura/internal/skill/bundle/` reflect `--rw` in their
  "Global Flags" section (this happens automatically via
  `common/skill/render/render.go:130-135` since `--rw` is a persistent
  root flag).
- REQ-F-015: `README.md` (repo root) MUST gain a short "Write
  operations require `--rw`" section near the existing usage examples,
  with one example each for an aura write, a config write, and a
  query write.
- REQ-F-016: `distribution/npm/cli/README.md` (the user-facing tarball
  README) MUST mention `--rw` if it currently documents flags or
  shows examples of mutating commands.
- REQ-F-017: `CONTRIBUTING.md` MUST gain a one-liner under the
  cobra-tree convention section: every new write leaf must set
  `Annotations["write"] = "true"` in its constructor.
- REQ-F-018: Two changie YAMLs MUST land in `.changes/unreleased/`,
  both `kind: Minor`:
  - `project: aura-cli` — body: `Require --rw flag for any write
    operation (instance create/delete/update, config set, credential
    add/remove/use, etc.)`
  - `project: neo4j-cli` — body: `Require --rw flag for any write
    operation; query run uses EXPLAIN to detect write cypher`
- REQ-F-019: All new `.go` files MUST begin with the Neo4j copyright
  header (enforced by `make license-check`).

### Functional Requirements — Bolt Migration

These requirements supersede the HTTP-transport pieces of REQ-F-007
through REQ-F-011 once implemented; the surface contract (writes
require `--rw`, reads frictionless, EXPLAIN preflight) is unchanged.

- REQ-F-020: `go.mod` MUST gain a direct dependency on
  `github.com/neo4j/neo4j-go-driver/v6` at `v6.0.0` (or the latest
  `v6.x` at implementation time). This is the ONLY new dependency
  permitted; supersedes REQ-NF-006.
- REQ-F-021: `neo4j-cli/query/connect.go` MUST replace the entire
  HTTP transport (`httpDoer`, `runStatement`,
  `runStatementResponse`, `newHTTPClient`, `parseBool`, the
  `queryResponse` / `queryPlan` / `queryError` structs, the
  `application/json` request building, and the `/db/<db>/query/v2`
  URL composition) with a single `neo4j.DriverWithContext`
  constructed via `neo4j.NewDriverWithContext(uri,
  neo4j.BasicAuth(username, password, ""))`. The `conn` struct
  retains only the fields needed by callers: `uri`, `database`,
  `userAgent`, and the driver handle. The driver is closed by the
  caller (defer `driver.Close(ctx)` at the leaf RunE level).
- REQ-F-022: The `--insecure` flag and the `NEO4J_INSECURE` env var
  MUST be removed entirely. Targets: the flag registration in
  `neo4j-cli/query/query.go`, every reference in
  `neo4j-cli/query/connect.go`, the `Insecure bool` field on
  `DbmsCredentials`, its inclusion in
  `PrintableDbmsCredentials.AsArray()` / `MarshalJSON()` and the
  add/use commands that set it, every test that exercises
  `--insecure` or `NEO4J_INSECURE`, and every doc/skill/README
  mention. TLS is selected by URI scheme only
  (`neo4j+s://` for verified TLS, `neo4j+ssc://` for self-signed).
- REQ-F-023: URI rewrite — when the resolved URI begins with
  `http://<host>[:<port>][/...]`, rewrite to
  `neo4j://<host>:7687`. When `https://<host>[:<port>][/...]`,
  rewrite to `neo4j+s://<host>:7687`. The host segment is
  preserved; any path/query is stripped; the port is forced to
  7687. The existing one-line `info: rewrote URI ...` notice on
  stderr stays so the user sees the change. Default URI when
  none is supplied: `neo4j://localhost:7687`. The default
  database stays `neo4j`. The default username stays `neo4j`.
- REQ-F-024: Query execution MUST use the driver's auto-managed
  transaction wrappers — `session.ExecuteRead(ctx, work)` for
  read-only cypher (and the EXPLAIN preflight) and
  `session.ExecuteWrite(ctx, work)` for cypher classified as
  write/schema. Inside the work function, the body is the
  simplest possible:
  ```go
  res, err := tx.Run(ctx, cypher, params)
  if err != nil { return nil, err }
  records, err := res.Collect(ctx)        // for execution
  // or
  summary, err := res.Consume(ctx)        // for EXPLAIN preflight
  ```
  No manual `BeginTransaction`, no commit/rollback. The session
  is created with `driver.NewSession(ctx, neo4j.SessionConfig{
  DatabaseName: c.database})` and closed via
  `defer session.Close(ctx)`.
- REQ-F-025: The `--rw` write classifier MUST read
  `summary.StatementType()` from the EXPLAIN preflight (the
  preflight runs inside `session.ExecuteRead` since EXPLAIN never
  mutates state). When the value is `neo4j.StatementTypeReadOnly`,
  proceed by running the real cypher inside `ExecuteRead`. Anything
  else (`StatementTypeReadWrite`, `StatementTypeWriteOnly`,
  `StatementTypeSchemaWrite`, `StatementTypeUnknown`) returns the
  existing `clierr.NewUsageError("this command writes; pass --rw
  to allow it")` before any execution. When `--rw` is set the
  preflight is skipped and the real cypher runs inside
  `ExecuteWrite`. No operator-tree walking.
- REQ-F-026: Drop the `queryPlan` struct and any code path that
  inspected operator types from EXPLAIN. Detection is single
  source: `summary.StatementType()`.
- REQ-F-027: Delete `neo4j-cli/query/query_https_smoke_test.go`
  entirely (cert generation, container boot, HTTPS readiness
  poller, `testExplainResponseShape`, `runQueryCmd`,
  `postQueryV2`, etc.). Replace with one env-gated Bolt smoke
  test at `neo4j-cli/query/query_bolt_smoke_test.go` that boots
  `neo4j:latest`, exposes the Bolt port on a random local port,
  and asserts: read EXPLAIN → `StatementTypeReadOnly`; write
  EXPLAIN (`CREATE (n:T)`) → `StatementTypeReadWrite`; schema
  EXPLAIN (`CREATE INDEX ...`) → `StatementTypeSchemaWrite`. The
  test stays Unix-only (`//go:build !windows`) and gated on
  `NEO4J_BOLT_TEST=1`.
- REQ-F-028: Update existing query unit tests
  (`neo4j-cli/query/run_test.go`, `connect_test.go`, etc.) to
  swap the HTTP fake (`httpDoer` / `httptest.Server`) for a
  driver-level fake. Keep coverage parity for the existing
  cases: read with no `--rw` succeeds (preflight returns
  ReadOnly), write with no `--rw` errors before the real call,
  write with `--rw` skips preflight and runs once, EXPLAIN
  syntax errors surface verbatim, partial-credential resolution
  rules unchanged.
- REQ-F-029: Update changie entry bodies (REQ-F-018 already
  landed) — replace the existing two `.changes/unreleased/`
  YAMLs with bodies that reflect both changes:
  - `project: aura-cli` body: `Require --rw flag for any write
    operation (instance create/delete/update, config set,
    credential add/remove/use, etc.)` (unchanged for aura-cli).
  - `project: neo4j-cli` body: `Require --rw flag for any write
    operation; query run now uses the Neo4j Bolt driver
    (--insecure is removed; use neo4j+ssc:// for self-signed
    certs)`.
- REQ-F-030: Update `neo4j-cli/internal/skill/additions.md` (and
  the aura one if applicable) and the regenerated `bundle/`
  files: drop every mention of `--insecure` and the HTTP Query
  API; the "Write operations" gotcha block stays, with the
  description updated to "the CLI runs EXPLAIN over Bolt to
  detect write cypher". Run `make generate` after the edit so
  the bundle diff is part of the PR.
- REQ-F-031: Update `README.md` and
  `distribution/npm/cli/README.md` to drop every `--insecure`
  reference. URI examples MUST use `neo4j://` form
  (`http://...` examples are auto-rewritten at runtime, but the
  documented form is the native scheme).

### Non-Functional Requirements

- REQ-NF-001: `make test` passes on linux, windows, macos.
- REQ-NF-002: `make fmt-check` passes (gofmt clean).
- REQ-NF-003: `make lint` passes (golangci-lint v2).
- REQ-NF-004: `make license-check` passes.
- REQ-NF-005: `make generate-check` passes (CI gate; the bundle diff
  catches stale generated output).
- REQ-NF-006: ~~No new dependencies in `go.mod`.~~ Superseded by
  REQ-F-020: `github.com/neo4j/neo4j-go-driver/v6` is the single
  permitted addition.
- REQ-NF-007: For read-only `query run` invocations the preflight
  costs exactly one extra Bolt round trip. Acceptable; EXPLAIN is
  cheap and the cost vanishes as soon as the user passes `--rw`.
- REQ-NF-008: The Bolt smoke test
  (`neo4j-cli/query/query_bolt_smoke_test.go`) is gated on
  `NEO4J_BOLT_TEST=1` so `go test ./...` and CI default runs
  remain docker-free.

## Technical Considerations

**Read vs write inventory** (full classification — used for choosing
which leaves to annotate):

- **Aura API writes (24)** — instance×6 + snapshot.create + cmk×2 +
  deployment×2 + deployment.token×3 + graphql×5 + authprovider×2 +
  cors.allowedorigin×2 + session×2 + import.job×2.
- **Local writes (9)** — aura.config.set + aura.config.project×3 +
  aura.credential×3 + neo4j.config.set + neo4j.credential.database×3.
  (= 11; revise if exact list shifts.)
- **Skill writes (2)** — install, remove.
- **Query** — runtime classification via EXPLAIN.
- **Reads (everything else)** — list/get/check/list across all the
  above, plus `query schema` and read-only `query run`.

**Hook composition strategy.** `RegisterOutputFlag` today both
registers `--format` AND sets `cmd.PersistentPreRunE` for binding +
validation (`common/flags/flags.go:17-45`). Adding a second
registration that also sets `PersistentPreRunE` would clobber the
first. Refactor:

1. Extract the format-binding body of `RegisterOutputFlag` into a
   private helper `bindFormatFromFlag(cmd, cfg) error`.
2. Add `RegisterRwFlag(cmd)` that just registers the `--rw` flag (no
   hook).
3. Add `enforceWriteGate(cmd) error` that walks
   `cmd.Annotations["write"]` and the `--rw` value.
4. The root commands set `PersistentPreRunE` exactly once, calling
   both helpers in order: format binding first (so format errors
   surface even on writes without `--rw`), then write-gate check.

**Annotation lookup.** Inside `PersistentPreRunE`, `cmd` is the
*invoked leaf*, not the root. `cmd.Annotations` is a per-cmd map
populated at construction time, so reading `cmd.Annotations["write"]`
on the leaf is the correct check.

**Flag access from leaf.** `cmd.Flag("rw").Value.String()` (the
`Flag()` form, not `Flags().GetBool()`) reads the persistent flag
through the parent chain even when only the local flagset has been
merged. Convert with `strconv.ParseBool`. This pattern is documented
in `CLAUDE.md` under "Cobra Flag Access Notes".

**EXPLAIN response shape — empirical findings (2026-05-06).**
Verified live against `neo4j:5.26` and `neo4j:2026.04`:

| | HTTP Query API v2 | Bolt (Go driver v5.28 / v6.0.0) |
|---|---|---|
| Top-level fields | `data`, `queryPlan`, `bookmarks` | summary exposes `StatementType()` |
| Single-field classifier | absent (no `summary.queryType`) | present — `r`/`rw`/`w`/`s` |
| Read EXPLAIN | operator tree only | `StatementTypeReadOnly` |
| Write EXPLAIN | operator tree only | `StatementTypeReadWrite` |
| Schema EXPLAIN | operator tree only | `StatementTypeSchemaWrite` |
| Driver scheme support | n/a | bolt-only — `http://`/`https://` rejected |

This is the empirical basis for the migration: HTTP cannot give us a
single-field classifier without walking the operator tree, while Bolt
does it natively. The driver itself does not support HTTP transport
(verified against v5.28 and v6.0.0; allow-list is `bolt`,
`bolt+unix`, `bolt+s`, `bolt+ssc`, `neo4j`, `neo4j+s`, `neo4j+ssc`).

**Bolt migration plan.**

1. Replace `httpDoer`-based `runStatement` with a thin helper that
   takes `(ctx, *conn, statement, params)`, opens a session via
   `driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName:
   c.database})`, runs the work inside `session.ExecuteRead` /
   `ExecuteWrite`, and returns `(records, summary, err)`.
2. `runQuery` (in `run.go`) chooses the wrapper:
   - When `--rw` is unset: `ExecuteRead` for the EXPLAIN preflight,
     read `summary.StatementType()`. If `ReadOnly`, run the real
     cypher inside `ExecuteRead`. Otherwise return the usage error.
   - When `--rw` is set: skip preflight, run the real cypher inside
     `ExecuteWrite`.
3. Convert `[]*neo4j.Record` to the existing `queryResult` shape:
   `Columns = records[0].Keys` (or the result's `Keys()` when no
   records returned), `Rows[i] = records[i].Values`. Output
   rendering stays identical.

**`ExecuteRead`/`ExecuteWrite` over `session.Run`.** The managed
wrappers handle transient-error retries, automatic commit on
callback success, and rollback on failure. The body inside the
callback is one `tx.Run` + one `Collect` (or `Consume`) — no more
verbose than `session.Run` and substantially less error-prone.

**Files touched (Bolt-migration delta).**

- `go.mod` / `go.sum` — add `github.com/neo4j/neo4j-go-driver/v6`.
- `neo4j-cli/query/connect.go` — replace HTTP transport, drop
  `httpDoer`, `runStatement*`, `newHTTPClient`, `parseBool`, the
  `queryResponse` / `queryPlan` / `queryError` structs, and any
  `--insecure` references.
- `neo4j-cli/query/run.go` — switch preflight + execution to the
  driver wrappers; classifier reads `summary.StatementType()`.
- `neo4j-cli/query/query.go` — drop the `--insecure` flag
  registration.
- `neo4j-cli/query/uri.go` (and tests) — replace HTTP↔Bolt URL
  rewrite with `http://X[:Y]/...` → `neo4j://X:7687`, `https://`
  → `neo4j+s://X:7687`. Default `neo4j://localhost:7687`.
- `neo4j-cli/query/connect_test.go` / `run_test.go` — rewrite
  fakes against the driver. Drop `--insecure` cases.
- `neo4j-cli/query/query_https_smoke_test.go` — delete.
- `neo4j-cli/query/query_bolt_smoke_test.go` — new, env-gated.
- `common/clicfg/credentials/dbms.go` and `aura.go`-shaped
  printable types — drop `Insecure bool` from `DbmsCredentials`,
  remove from `AsArray()`/`MarshalJSON()`, drop the add/use
  flag that sets it.
- `neo4j-cli/internal/skill/additions.md` and bundle regen.
- `README.md`, `distribution/npm/cli/README.md` — drop
  `--insecure`, update URI examples to `neo4j://`.
- `.changes/unreleased/neo4j-cli-Minor-*.yaml` — replace body.

**Preflight reuses connection.** The existing `conn` struct
(`neo4j-cli/query/connect.go:49-57`) holds uri, basic auth, database,
TLS, user-agent. Calling `runStatement(ctx, c, "EXPLAIN "+cypher,
params)` with the same `c` reuses everything. No new HTTP client.

**Persistent flag inheritance.** The super-CLI mounts aura via
`aura.NewCmd(cfg)`; aura's `NewCmd` does NOT register `--rw` because
it inherits the super-CLI's persistent flag automatically. Only
`NewStandaloneCmd` (used by the aura-cli binary) registers it. This
mirrors how `RegisterOutputFlag` is already invoked.

**Generated bundle drift.** Adding a persistent root flag changes
the "Global Flags" table in `bundle/SKILL.md` for both binaries. Any
edit to the cobra tree without re-running `go generate` fails the
CI gate (`TestGenerator_RoundTrip` and `make generate-check`). Run
`make generate` and commit the regenerated bundles in the same PR.

**Files touched (summary):**

- `common/flags/flags.go` *(refactor + add `RegisterRwFlag`)*
- `common/flags/flags_test.go` *(new)*
- `neo4j-cli/app/app.go` *(call `RegisterRwFlag`, extend `Long`,
  compose hook)*
- `neo4j-cli/aura/aura.go` *(call `RegisterRwFlag` in
  `NewStandaloneCmd`, extend `Long`)*
- 33 write leaves *(one-line annotation each)*
- `neo4j-cli/query/run.go` *(preflight EXPLAIN call when `!rw`)*
- `neo4j-cli/query/connect.go` *(extend `queryResponse` with
  query-type field after docker-confirmed shape)*
- `neo4j-cli/query/run_test.go` *(read/write/with-rw cases)*
- `neo4j-cli/internal/skill/additions.md`
- `neo4j-cli/aura/internal/skill/additions.md`
- `README.md`
- `distribution/npm/cli/README.md`
- `CONTRIBUTING.md`
- `.changes/unreleased/aura-cli-Minor-<ts>.yaml`
- `.changes/unreleased/neo4j-cli-Minor-<ts>.yaml`
- Regenerated `neo4j-cli/internal/skill/bundle/**` and
  `neo4j-cli/aura/internal/skill/bundle/**`

**Risks:**

- Forgetting to annotate a write leaf — that write would silently run
  without `--rw`. Mitigation: per-resource test that asserts the
  command rejects the call without `--rw` (one positive + one
  negative case per resource as a tripwire).
- EXPLAIN preflight masks real-execution errors — only if EXPLAIN
  succeeds and real execution then fails. The preflight runs
  EXPLAIN, not the real query, so this is rare; surface real errors
  unchanged.
- `make generate-check` failure if the developer forgets to
  regenerate bundles after adding `--rw`. Already covered by CI.

## Acceptance Criteria

- [ ] `common/flags/flags.go` exports `RegisterRwFlag` and a refactored
      composable format-binding helper; existing `RegisterOutputFlag`
      callers continue to work.
- [ ] `--rw` appears in `--help` output of both `neo4j-cli` and
      `aura-cli` roots, with the documented help text.
- [ ] Every write leaf listed in REQ-F-003 sets
      `Annotations["write"] = "true"`.
- [ ] `bin/neo4j-cli aura instance list` succeeds with no `--rw`.
- [ ] `bin/neo4j-cli aura instance delete <id>` exits non-zero with
      message `"this command writes; pass --rw to allow it"`.
- [ ] `bin/neo4j-cli aura instance delete <id> --rw` proceeds (against
      mock or test API).
- [ ] `bin/neo4j-cli config set foo bar` errors without `--rw`,
      succeeds with `--rw`.
- [ ] `bin/neo4j-cli skill install claude-code` errors without `--rw`,
      succeeds with `--rw`.
- [ ] `bin/neo4j-cli query run "MATCH (n) RETURN count(n)"` succeeds
      with no `--rw` (EXPLAIN classifies as read-only).
- [ ] `bin/neo4j-cli query run "CREATE (n:Test)"` errors with
      `"this command writes; pass --rw to allow it"` BEFORE any
      mutation reaches the database.
- [ ] `bin/neo4j-cli query run "CREATE (n:Test)" --rw` succeeds and
      skips the preflight (single Bolt round trip, not two).
- [ ] EXPLAIN failure (e.g. cypher syntax error) surfaces the original
      error, not a generic preflight wrapper.
- [ ] Standalone `bin/aura-cli` repeats all the above for its scope.
- [ ] Both `additions.md` files contain the new "Write operations"
      gotcha block.
- [ ] `bundle/SKILL.md` "Global Flags" section in both binaries lists
      `--rw` after `make generate`.
- [ ] `README.md` mentions `--rw` with at least three examples (aura
      write, config write, query write).
- [ ] `distribution/npm/cli/README.md` mentions `--rw` if it shows
      mutating examples.
- [ ] `CONTRIBUTING.md` documents the `Annotations["write"] = "true"`
      convention for new write leaves.
- [ ] Two changie YAMLs in `.changes/unreleased/`, project keys
      `aura-cli` and `neo4j-cli`, kind `Minor`, body per REQ-F-018.
- [ ] `make test`, `make fmt-check`, `make lint`, `make
      license-check`, `make generate-check` all pass.

### Acceptance Criteria — Bolt Migration

- [ ] `go.mod` contains exactly one new direct dep:
      `github.com/neo4j/neo4j-go-driver/v6` (REQ-F-020).
- [ ] No `httpDoer`, `runStatement`, `runStatementResponse`,
      `newHTTPClient`, `parseBool`, `queryResponse`, `queryPlan`,
      or `queryError` symbols remain in the `query` package.
- [ ] `--insecure` is gone: no flag registration, no `NEO4J_INSECURE`
      env-var read, no `Insecure` field on `DbmsCredentials`, no
      mention in any test, doc, README, skill, or bundle.
- [ ] `bin/neo4j-cli query run "RETURN 1"` against a Bolt-only
      Neo4j instance (`neo4j:latest` in docker) succeeds with the
      default URI.
- [ ] `bin/neo4j-cli query run "RETURN 1" --uri http://example:7474`
      logs the rewrite line and connects to `neo4j://example:7687`.
- [ ] Same with `--uri https://example:7473` rewriting to
      `neo4j+s://example:7687`.
- [ ] `query_https_smoke_test.go` is deleted; a Bolt smoke test
      exists, env-gated on `NEO4J_BOLT_TEST=1`, asserting the
      three statement-type cases (read / write / schema).
- [ ] EXPLAIN preflight runs inside `session.ExecuteRead`; the
      real query runs inside `ExecuteRead` (read-only) or
      `ExecuteWrite` (when `--rw` was passed). No code path opens
      a manual `BeginTransaction`.
- [ ] `summary.StatementType()` is the sole classifier; no
      operator-tree walking remains.

## Out of Scope

- `NEO4J_CLI_RW=1` environment variable alternative.
- A separate `--force`/`--yes` flag.
- Granular gating (e.g. distinct flags for API vs. local).
- Detection of write cypher by string parsing (no regex on the cypher
  body — EXPLAIN is the only classifier).
- Interactive confirmation prompts.
- Allowlist exceptions for "harmless" local writes.
- Migrating any existing flag default behavior; the only change is the
  addition of `--rw`.
- Changes to `query schema`'s read-only behavior.

## Open Questions

None — the resolved decisions block in the source plan covers them
all (EXPLAIN response shape verified at implementation time via
docker, error wording fixed, preflight skipped when `--rw` already
set, local config writes gated for consistency).
