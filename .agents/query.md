# Query Subsystem Notes

Notes and patterns for working on the `neo4j-cli/query` package — Bolt driver integration, query execution, credential wiring, and local smoke testing.

## Local Verification Scripts

- `TestBolt_Smoke` (`neo4j-cli/query/query_bolt_smoke_test.go`) — real-Neo4j Bolt smoke for the `query` command's EXPLAIN classifier. Boots `neo4j:latest` via `os/exec`, maps Bolt 7687 to a random free host port, polls `Driver.VerifyConnectivity` until ready, then asserts `runStatementResponse(... readOnly=true)` returns `QueryTypeReadOnly` / `QueryTypeReadWrite` / `QueryTypeSchemaWrite` for read / write / schema EXPLAIN cypher. Build constraint `//go:build !windows`. Gated by `NEO4J_BOLT_TEST=1`; run via `NEO4J_BOLT_TEST=1 go test -run TestBolt_Smoke -v ./neo4j-cli/query/...`. Requires `docker`. Skipped by default in `go test ./...`.

## query Bolt Driver Notes

- neo4j-go-driver/v6 deprecates the `*WithContext` aliases (`Driver`, `NewDriver` are the canonical names in v6); staticcheck/SA1019 flags `DriverWithContext`/`NewDriverWithContext`. Use the unsuffixed names — the bare `neo4j.Driver` interface IS context-aware in v6.
- `result.Consume(ctx)` returns `ResultSummary`; `summary.Plan()` is non-nil only for EXPLAIN/PROFILE. The `Plan` interface exposes `Operator() string` (not `OperatorType`) and `Children() []Plan` — recursive walk maps cleanly into our internal `queryPlan{OperatorType, Children}` shape via a `convertPlan` helper.
- For driver-backed query tests: use a package-level `runStatementResponseFn` test seam (and a paired `driverOpener` seam returning a no-op driver) so tests inject canned `*queryResponse` envelopes without a live Bolt server. The no-op driver embeds `neo4j.Driver` (nil interface) and overrides only `Close(ctx) error`; type satisfaction is automatic, and any unrouted method call panics — which is the desired loud-fail signal that the seam was bypassed.
- `c.openDriver()` is idempotent; production callers defer `c.driver.Close(ctx)` immediately after openDriver succeeds. The driver is opened LAZILY (after password prompt) because BasicAuth is bound at driver creation time and we cannot know the password during `resolveConn`.
- `make generate-check` is `git diff --exit-code` after `go generate ./...`; on a working tree with uncommitted source changes it WILL fail (it's checking that nothing changed AT ALL during regenerate). The gate is meaningful only against a clean working tree (CI scenario). Locally, commit your source changes first OR ignore the false positive when the only reported diff is your own edits, not bundles.

## query/connect.go Credential Integration Notes

- `resolveConn` integrates stored dbms credentials: when no params are set via flags/env/dotenv, the stored default credential is used; when a stored credential exists and only 1–3 of the 4 params are explicitly set, an all-or-nothing error is returned; when all 4 are set explicitly, the stored credential is bypassed entirely. When no stored credential exists, the original behavior (partial params + built-in defaults) applies unchanged.
- Use `cmd.Flag("name").Changed` (not `flagString(cmd, "name") != ""`) to detect explicit flag-setting — `Changed` is the only reliable indicator that the user set the flag, versus just reading the default value.
- `insecureExplicit` pattern: read `cmd.Flag("insecure").Changed` AFTER applying the insecure value, then gate credential's insecure on `!insecureExplicit` — ensures the explicit `--insecure=false` overrides the stored credential's `insecure:true`.

## query Bolt Execution Notes

- `runStatementResponseFn` seam takes a `readOnly bool` arg; production routes `readOnly=true` to `session.ExecuteRead` and `readOnly=false` to `session.ExecuteWrite`. Tests must forward this arg in their seam handlers (`func(_ ctx, _ *conn, stmt, params, readOnly bool) (*queryResponse, error)`).
- Two entry points wrap the seam: `runStatement` (defaults to ExecuteRead) and `runStatementWrite` (forces ExecuteWrite). EXPLAIN preflight calls `runStatementResponse(..., readOnly=true)` directly because EXPLAIN never mutates.
- TLS is selected exclusively by URI scheme (`neo4j+s://` for verified, `neo4j+ssc://` for self-signed). The `--insecure` flag, `NEO4J_INSECURE` env var, and `DbmsCredentials.Insecure` field have been removed. `driverOpener` takes 4 args (target, username, password, userAgent) — tests must match this signature in their seam handlers.
- Removing a column from `dbmsCredentialFields` in `internal/subcommands/credential/dbms/list.go` is the only step needed to drop a column from `credential dbms list` table output; `PrintBodyMap` reads the field list and renders accordingly. JSON output is driven by `PrintableDbmsCredentials.AsArray()` / `MarshalJSON` so both must be kept in sync with the struct shape.
- Inside the managed-transaction work callback the order is `tx.Run` → `result.Collect(ctx)` → `result.Consume(ctx)`. Wrap errors once at the outer ExecuteRead/ExecuteWrite return (not per step) so retry semantics get the raw error and user output keeps a single `query: ...` prefix.
- `seamRouter.readOnlyCalls map[string]bool` lets `run_test.go` assert the routing by statement (e.g. `assert.True(t, r.readOnlyCalls["EXPLAIN ..."])`).
- v6 driver deprecates `StatementType()` / `StatementType*` constants in favor of `QueryType()` / `QueryType*` — staticcheck (SA1019) flags any use. They share the same underlying int alias, so the rename is purely API hygiene; CLI code stores `resp.QueryType neo4j.QueryType` and compares against `neo4j.QueryTypeReadOnly` for the --rw classifier.
- `normalizeURI` rewrites `http://<host>[:p][/...]` → `neo4j://<host>:7687` and `https://...` → `neo4j+s://<host>:7687`; path/query/fragment are stripped, userinfo is preserved on the rewritten URI, and the displayOrig form is `(*url.URL).Redacted()` so passwords are masked on stderr. Bolt-family schemes (bolt, bolt+s, bolt+ssc, neo4j, neo4j+s, neo4j+ssc) pass through unchanged. Default URI is `neo4j://localhost:7687`.
