# PRD: CLI-168 — Add `--debug` flag to `neo4j-cli query`

## Overview

A user reported a silent hang on:

```
neo4j-cli query \
  --uri 'neo4j+s://cad2fee7b350fbd7844314001bcec5ae.neo4jsandbox.com:443' \
  --username neo4j --database neo4j --password '<redacted>' \
  'match () return count(*);'
```

The neo4j-go-driver-default `SocketConnectTimeout = 5s` bounds only TCP
connect; TLS handshake, Bolt handshake, routing-table fetch
(triggered by `neo4j+s://`), HELLO, and RUN are all unbounded. Likely
the routing-table fetch is stuck (sandboxes are single-instance — a
`bolt+s://` URI would skip routing) or port 443 isn't actually Bolt
for that sandbox. We can't tell without a wire-level trace.

This PRD scopes the addition of a `--debug` persistent flag on the
`query` command tree, wired into the driver's
`config.Config.Log` so future hang reports include a trace that
localises the failure to TLS / routing / Bolt-handshake / HELLO.

Per-session `BoltLogger` (RUN/PULL/HELLO frames) is intentionally
out of scope: HELLO frames carry credentials, so enabling it
requires a redacting wrapper. Track separately if needed.

Linear:
[CLI-168 — investigate hanging neo4j-cli query and add debug flag](https://linear.app/neo4j/issue/CLI-168/investigate-hanging-neo4j-cli-query-and-add-debug-flag).

## Goals

- Give the next reporter of a `query` hang a one-flag way to produce
  a wire-level trace (`neo4j-cli query --debug ...`) that pinpoints
  whether the hang is TLS, routing, or Bolt-handshake.
- Make the flag inherit automatically to `query :schema` (which
  shares the driver path); make it harmless on `query :embed` (which
  never opens a driver).
- Preserve current behaviour exactly when `--debug` is off — no log
  surface, no extra overhead, `config.Config.Log == nil`.
- Keep machine-readable output (`--format json`, `--format toon`)
  uncorrupted: debug lines MUST go to stderr only.

## Non-Goals

- Per-session `BoltLogger`. HELLO frames carry credentials; safe
  exposure requires a redacting decorator. Tracked as a possible
  follow-up; not in this PRD.
- `--connect-timeout` / overall context deadline. Bounding the hang
  itself is the right next change but is a separate decision: this
  PRD only ships visibility.
- Multiple verbosity levels (`-v` / `-vv` / `--log-level`). DEBUG-only
  is enough to localise the reported hangs; multi-level wiring can
  follow if a real need emerges.
- Fixing the underlying sandbox-URL hang. The user's report is
  the trigger, not the deliverable; the deliverable is the
  diagnostic surface.
- Logging in the `query :embed` leaf. The leaf is provider-side and
  never calls `openDriver`; `--debug` is a documented no-op there.

## Requirements

### Functional Requirements

- **REQ-F-001**: A persistent boolean flag `--debug` MUST be
  registered on the `query` parent in `neo4j-cli/query/query.go`,
  default `false`. Persistence ensures `query :schema` (and any
  future driver-backed leaf) inherits it.
- **REQ-F-002**: The flag's help text MUST mention the env-var
  fallback in the existing `[env: NEO4J_VAR]` style used by `--uri`,
  `--username`, etc. — e.g.
  `"Trace driver activity to stderr [env: NEO4J_DEBUG (set to 1 to enable)]"`.
- **REQ-F-003**: `NEO4J_DEBUG` MUST be honoured **only** when its
  value is the literal `1`. Any other value — including `true`,
  `yes`, `on`, `0`, empty, or unset — leaves debug OFF. This strict
  acceptance MUST be documented in the flag help and asserted by
  test.
- **REQ-F-004**: Precedence: flag set explicitly wins over env.
  Specifically, `cmd.Flag("debug").Changed && value=="true"` OR
  `os.Getenv("NEO4J_DEBUG") == "1"` enables debug. (An explicit
  `--debug=false` overrides `NEO4J_DEBUG=1`.) Resolution MUST live in
  `resolveConn` (`neo4j-cli/query/connect.go`).
- **REQ-F-005**: A `debug bool` field MUST be added to the `conn`
  struct (`neo4j-cli/query/connect.go`), sitting next to
  `userAgent`. `resolveConn` populates it; downstream callers read
  it.
- **REQ-F-006**: The `driverOpener` test seam MUST extend from 4
  args `(target, username, password, userAgent)` to 5 args
  `(target, username, password, userAgent, debug)`. All call sites
  and stubs (including those in `neo4j-cli/query/run_test.go` and
  any other `*_test.go` that swaps the seam) MUST be updated to the
  new signature.
- **REQ-F-007**: When `debug == true`, `driverOpener`'s configurer
  closure MUST set `c.Log = newStderrLogger(log.DEBUG)` where
  `newStderrLogger` is a thin in-package adapter implementing the
  `neo4j/log.Logger` interface and writing all levels (DEBUG / INFO
  / WARN / ERROR) to `os.Stderr`. The driver-shipped
  `log.ToConsole(level)` MUST NOT be used: it writes DEBUG / INFO /
  WARN to stdout (see
  `github.com/neo4j/neo4j-go-driver/v6@v6.0.0/neo4j/log/console.go`),
  which would corrupt `--format json` and `--format toon` output.
- **REQ-F-008**: When `debug == false`, the configurer closure MUST
  NOT touch `c.Log`. Resulting `config.Config.Log` remains the
  driver's nil default. Asserted by test.
- **REQ-F-009**: `--debug` MUST NOT alter stdout in any way. The
  rendered query result on stdout MUST be byte-identical between a
  `--debug=false` run and a `--debug=true` run against the same
  seam-stubbed response. Asserted by test.
- **REQ-F-010**: The `query :embed` leaf is a no-op for `--debug`:
  it never opens a driver, so the flag inherits but has no
  observable effect. This is intentional; the help text on
  `--debug` does not need to caveat this (the user-facing copy stays
  generic and the leaf-specific behaviour is documented in
  `.agents/query.md`).

### Non-Functional Requirements

- **REQ-NF-001**: `make fmt-check`, `make lint`, and `make test`
  MUST be clean on the resulting branch (the AGENTS.md final-gate
  rule).
- **REQ-NF-002**: After source changes,
  `go generate ./neo4j-cli/internal/skill/...` MUST be run and the
  resulting bundle diff committed in the same change. The new
  persistent flag changes the parent's help text, which
  `TestGenerator_RoundTrip` watches via `references/query.md` (and
  `SKILL.md` "Global Flags" — see AGENTS.md "Repo Layout Notes").
- **REQ-NF-003**: A changelog entry MUST be added via
  `make changelog` (or hand-authored YAML under `.changes/unreleased/`)
  with `kind: Minor`, project `neo4j-cli`. The entry mentions
  `--debug` and `NEO4J_DEBUG=1`, and notes that per-frame Bolt
  tracing is a deliberate follow-up.
- **REQ-NF-004**: Any new `.go` file MUST start with the Neo4j
  copyright header (enforced by `make license-check`). If the
  stderr adapter lives inline in an existing file no new header is
  needed.
- **REQ-NF-005**: `.agents/query.md` MUST be updated: the line
  noting `driverOpener` "takes 4 args" becomes "takes 5 args" and
  briefly describes the new `debug bool` parameter.
- **REQ-NF-006**: No new external Go dependency. The stderr adapter
  is a few lines of in-repo code; reuse the existing `neo4j/log`
  package (already imported transitively via the driver).

## Technical Considerations

- **Why a custom stderr logger instead of `log.ToConsole`.** The
  driver-shipped `console` writes DEBUG / INFO / WARN to `os.Stdout`
  (only ERROR goes to `os.Stderr`). The query command writes its
  results to stdout via the rendering pipeline (JSON, toon, table),
  so driver debug lines on stdout would interleave into and
  invalidate the user's output stream. Required to route all four
  levels to stderr. Adapter is ~30 lines, implements `Logger` (5
  methods: `Error`, `Errorf`-equivalent via interface; `Infof`,
  `Warnf`, `Debugf`).

- **Where the adapter lives.** Inline in `neo4j-cli/query/connect.go`
  as a package-private type. Tiny, single-use, no need for its own
  file or package. If a second consumer appears, factor it out
  later. (Tracks AGENTS.md "Don't add abstractions beyond what the
  task requires.")

- **`driverOpener` signature change.** The seam is consumed by tests
  in `neo4j-cli/query/run_test.go` (and possibly elsewhere). All
  stub overrides MUST update to the 5-arg form. The schema-leaf and
  embed-leaf tests don't override `driverOpener` directly — they
  use higher-level helpers — but a grep MUST confirm every override
  site is migrated.

- **Bundle regen scope.** Changing a persistent flag on the `query`
  parent rewrites two bundle files: `references/query.md` (the
  flag table for the parent) and `SKILL.md` (Global Flags
  section, if applicable). `go generate ./neo4j-cli/internal/skill/...`
  handles both; `TestGenerator_RoundTrip` is the gate.

- **Help-text wording.** Persistent-flag annotation MUST follow the
  in-file pattern (`[env: NEO4J_VAR]`), with the strict-`1`
  acceptance noted parenthetically. Example: `"Trace driver
  activity to stderr (uses neo4j-go-driver log.DEBUG; lines prefixed
  by level + connection id) [env: NEO4J_DEBUG (set to 1 to enable)]"`.
  Cobra's help wrapper handles line-folding.

- **Test seams already in place.**
  `neo4j-cli/query/run_test.go` and `connect_test.go` use the
  package-level `driverOpener` and `runStatementResponseFn` seams
  per `.agents/query.md`. The new tests do NOT need new seams —
  they assert via:
  - a stub `driverOpener` that captures the `debug` argument; AND
  - a stub `driverOpener` whose configurer closure inspects the
    resulting `*config.Config` to assert `Log` (set or nil).

- **`os.Getenv` discipline.** Read `NEO4J_DEBUG` exactly once in
  `resolveConn`. Tests use `t.Setenv("NEO4J_DEBUG", "1")` /
  `t.Setenv("NEO4J_DEBUG", "true")` etc. to exercise the
  acceptance rule. No new dotenv plumbing — `--debug` is a
  diagnostic switch, not a connection parameter; it does NOT layer
  through `.env` (matches the existing scope of dotenv lookup).

- **Loud-fail check on stdout cleanliness.** The byte-identity
  assertion (REQ-F-009) protects against a future contributor
  accidentally swapping the stderr writer back to stdout.

## Acceptance Criteria

- [ ] `--debug` persistent flag is registered on the `query` parent
  in `neo4j-cli/query/query.go` with help text matching
  REQ-F-002 and REQ-F-003.
- [ ] `resolveConn` populates `conn.debug` from flag-or-env
  (REQ-F-003 + REQ-F-004); strict-`1` env acceptance is asserted.
- [ ] `driverOpener` takes 5 args; all in-tree call sites and test
  stubs are migrated; `make test` passes.
- [ ] When `--debug` is on, `config.Config.Log` is a non-nil
  in-package adapter that writes all log levels to `os.Stderr`;
  asserted via a stub `driverOpener` that captures the configurer
  output.
- [ ] When `--debug` is off, `config.Config.Log` remains nil;
  asserted by a separate test case.
- [ ] Stdout byte-identity between `--debug=false` and
  `--debug=true` runs against the same seam-stubbed response.
- [ ] Test table covers: flag-on, env-on (`NEO4J_DEBUG=1`), both-on,
  both-off, env-set-but-not-`1` (`NEO4J_DEBUG=true` → debug OFF),
  explicit `--debug=false` overriding `NEO4J_DEBUG=1`.
- [ ] `.agents/query.md` is updated: "4 args" → "5 args" with a
  one-line note on the new `debug bool` parameter.
- [ ] `go generate ./neo4j-cli/internal/skill/...` has been run and
  the resulting `references/query.md` / `SKILL.md` diff is
  committed in the same change.
- [ ] Changelog entry exists under `.changes/unreleased/` with
  `kind: Minor`, project `neo4j-cli`, body referencing
  `--debug` + `NEO4J_DEBUG=1` and noting BoltLogger follow-up.
- [ ] `make fmt-check`, `make lint`, `make test` are all clean.
- [ ] Manual repro (smoke, not committed): running
  `neo4j-cli query --debug --uri 'bolt+s://127.0.0.1:443' "RETURN 1"`
  against a TLS-only-not-Bolt port (e.g. a local nginx) emits
  driver-trace lines on stderr and the eventual error on stderr; no
  driver lines leak onto stdout. Confirms the flag is wired and the
  stderr-routing requirement holds end-to-end.

## Out of Scope

- Per-session `BoltLogger` (RUN/PULL/HELLO frame trace). Needs a
  credential-redacting wrapper before it can ship; tracked as a
  separate ticket.
- `--connect-timeout` flag or any context deadline applied to
  `cmd.Context()`. Solves "how to stop the hang" rather than "how to
  see it"; deliberately split out.
- A `--log-level` / multi-level verbosity surface. Single-purpose
  `--debug` is sufficient.
- Plumbing `--debug` through `.env`. Diagnostic switches do not
  layer through dotenv; only connection parameters do.
- Logging in `query :embed`. The leaf is provider-side and never
  opens a driver.
- Any change to other commands' debug surfaces (Aura HTTP client,
  config migration warnings, skill installer logs). Scoped to the
  `query` tree only.

## Open Questions

None. All design decisions (driver-log only; strict-`1` env;
stderr-only output; embed leaf no-op) were resolved interactively
prior to PRD generation.
