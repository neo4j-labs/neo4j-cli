# PRD: Add `--debug` to Desktop Commands

## Overview

Add a `--debug` flag to the `neo4j-cli desktop` command tree so operators can see
the underlying Neo4j Desktop 2 relate-API activity — discovery (port-probe scan,
mDNS/dns-sd, the unauthenticated `/info/app` fetch) **and** the authenticated
request/response wire. This mirrors the merged aura `--debug` (CLI-197) and docker
`--debug` (CLI-198), all of which share `common/debug.Resolve` for
`--debug`/`NEO4J_DEBUG` semantics and `common/debug.Scrub` for redaction. Debug
output goes to **stderr only** and redacts secrets; stdout (the command's real
result) is byte-for-byte unaffected.

This is CLI-199.

## Goals

- Give desktop users visibility into why a desktop command fails — especially the
  #1 problem, "Desktop unreachable": which ports were probed, whether mDNS
  answered, and the relate-API request/response wire for DBMS/connection/plugin
  operations — without enabling shell/HTTP tracing.
- Reuse the established `--debug`/`NEO4J_DEBUG` resolution shared by aura, docker,
  and query (`common/debug.Resolve`, strict env value `"1"`, flag overrides env)
  and the shared `common/debug.Scrub` redactor.
- Centralize tracing in the one package where all desktop HTTP lives
  (`neo4j-cli/internal/desktopclient/`), so a single seam covers discovery and the
  authenticated wire with no signature churn across the ~16 leaves / 4 client
  wrappers.
- Never leak secrets: request bodies and headers are scrubbed; the `X-API-Token`
  JWT and write-command passwords (DBMS create, connection create/update) are
  redacted.
- Also wire `query --desktop --debug` into the same seam so the query path's
  desktop discovery + credential fetch is traced too.

## Non-Goals

- Changing aura/docker/query debug behavior beyond query's desktop-resolution path
  calling `desktopclient.SetDebug(...)` (no aura/docker test/behavior changes).
- Adding a `DesktopConfig` to `common/clicfg` (none exists; desktop borrows
  `cfg.Aura.Fs()` — a new config type is overkill for one boolean).
- Threading a `debug bool` through `NewClient`, the 4 `newDesktopClient` wrappers,
  or the ~16 leaf call sites (rejected: churny and misses the free-function
  discovery path).
- Debugging the Bolt layer of `query` (covered by query's existing `--debug`).
- Persisting debug state to disk.

## Requirements

### Functional Requirements

- REQ-F-001: A persistent `--debug` boolean flag is registered on the `desktop`
  parent command (`NewCmd`), inherited by every leaf across `dbms`, `dbms/plugin`,
  `connection`, and the top-level `list`/`doctor`/`install`.
- REQ-F-002: Resolution uses `common/debug.Resolve(cmd)`: explicit `--debug`
  (Changed) wins over env; otherwise enabled iff `NEO4J_DEBUG == "1"` (strict).
  `--debug=false` overrides `NEO4J_DEBUG=1`. Resolved **once** in the `desktop`
  root's `PersistentPreRunE` via `desktopclient.SetDebug(debug.Resolve(cmd))`. The
  neo4j-cli root's `cobra.EnableTraverseRunHooks = true` ensures it runs for every
  nested leaf; no leaf/subtree defines a conflicting `PersistentPreRunE` (verified).
- REQ-F-003: The `desktopclient` package owns a debug seam (new `debug.go`):
  package-level `var debugW io.Writer = os.Stderr` (writer seam), `var debugEnabled
  bool` set by exported `SetDebug(bool)`, prefixes `[desktop-debug] > ` (request) /
  `[desktop-debug] < ` (response) / `[desktop-debug] ` (info), `scrub` delegating to
  `common/debug.Scrub`, and emit helpers `debugRequest`/`debugResponse`/`debugInfo`
  plus a `writeHeaders` helper. Each emit helper early-returns when
  `!debugEnabled`, so call sites stay unconditional.
- REQ-F-004: Authenticated wire — `Client.doRaw` emits, when enabled:
  `[desktop-debug] > <METHOD> <url>` + sorted scrubbed headers + scrubbed body
  before `httpDoFn`; on success `[desktop-debug] < <status> ...` + headers + body +
  `elapsed`; on transport error a `[desktop-debug]` line with the error + elapsed
  (before the canonical unreachable/timeout mapping).
- REQ-F-005: Discovery wire — `probeOne` emits a `[desktop-debug]` line per port
  probe (target + result); `DiscoverViaMDNS`/`advertisedPort` emit the mDNS/dns-sd
  outcome and which `Discover` tier resolved; `FetchAppInfo` emits
  request/response lines around its `httpDoFn` call.
- REQ-F-006: Secrets never leak. `NewClient` registers the signed JWT via
  `clievents.RegisterSecretValue(token)` (a bare JWT isn't caught by shape-based
  `RedactText`). Write leaves register the password before the client call:
  `dbms/create.go` (body key `credentials` — NOT caught by `RedactText`),
  `connection/create.go`, and `connection/update.go` (when password set).
- REQ-F-007: `query --desktop --debug` traces desktop activity: the query
  desktop-resolution path (caller of `newDesktopFallthroughClient`, where `cmd` is
  available, in `query/connect.go`) calls
  `desktopclient.SetDebug(resolveDebug(cmd))` once before resolving.

### Non-Functional Requirements

- REQ-NF-001: Secrets never appear in debug output — bodies/headers pass through
  `Scrub`; the JWT and passwords are registered as secret values so any appearance
  is redacted.
- REQ-NF-002: Control/ANSI bytes are neutralized via `StripControl` (inside
  `Scrub`) before any string reaches the terminal.
- REQ-NF-003: stdout bytes are byte-for-byte identical with and without `--debug`
  (debug is stderr-only via the `debugW` seam), across `json`/`table`/`toon`.
- REQ-NF-004: The writer is a package-level `var debugW io.Writer = os.Stderr`
  seam overridable in tests (`SetDebugWriterForTest`); `debugEnabled` is set per
  invocation and resettable in tests (`SetDebugForTest`), both restoring via
  `t.Cleanup` — consistent with the package's existing `httpDoFn`/`uuidNewFn`/etc.
  seams.
- REQ-NF-005: No import cycle — `desktopclient` may import `common/debug`,
  `common/clievents`, `common/output` (none import `desktopclient`).

## Technical Considerations

- **Single package funnel**: every desktop HTTP call (discovery free functions +
  `Client.doRaw`) lives in `neo4j-cli/internal/desktopclient/`, so a package-level
  `debugEnabled` resolved once at the root covers all of it with zero signature
  changes to the 6 `NewClient` construction sites or the leaves. This is the
  desktop-specific "centralize where it makes sense" choice (aura hangs debug on
  `cfg.Aura`; docker on the `execClient`; desktop has no config carrier and HTTP in
  free functions, so the package itself is the carrier).
- **`doRaw` body retention**: `doRaw` currently wraps the marshalled body in a
  reader and discards the bytes; retain `rawBody []byte` to pass to `debugRequest`.
- **Redaction gaps to close**: `RedactText` is shape-based — it catches the
  `"password"` JSON key (connections) but NOT the `"credentials"` key CreateDbms
  uses, and not a bare JWT header value. Hence the explicit
  `RegisterSecretValue` calls in REQ-F-006.
- **Generated-artifact drift**: the new persistent flag changes desktop command
  help → `bundle/references/*.md` drift (gated by `TestGenerator_RoundTrip` /
  `make generate-check`). Run `go generate ./neo4j-cli/internal/skill/...` and
  commit the regenerated bundle with the source.
- **Flag wording**: mirror aura/docker style incl. the
  `[env: NEO4J_DEBUG (set to 1 to enable)]` suffix; note discovery is in scope.
- **Test harness**: drive a leaf end-to-end by mounting the desktop tree under a
  stub `neo4j-cli` root with `cobra.EnableTraverseRunHooks=true` (mirror `app.go`)
  so the root `PersistentPreRunE` resolves `--debug`; mock HTTP via
  `SetHTTPDoFnForTest`, capture via `SetDebugWriterForTest`.

## Acceptance Criteria

- [ ] `neo4j-cli desktop <leaf> --debug ...` prints `[desktop-debug] >`/`<`/info
      lines to stderr for discovery (probe/mDNS/`/info/app`) and the relate-API
      wire.
- [ ] `NEO4J_DEBUG=1` enables debug without the flag; `--debug=false` overrides it;
      `NEO4J_DEBUG=true` leaves it off.
- [ ] stdout is byte-identical with and without `--debug` across json/table/toon.
- [ ] The `X-API-Token` JWT and write-command passwords (incl. CreateDbms
      `credentials` body) never appear in debug output.
- [ ] `query --desktop --debug` emits `[desktop-debug]` discovery lines.
- [ ] Unit tests cover the emit helpers (body+JWT redaction, control-strip,
      off-path silent); an end-to-end test asserts the wire reaches the seam and
      stdout invariance; a root flag/env precedence table; a discovery-trace test;
      a create-redaction test.
- [ ] `make generate-check`, `make test`, `make fmt-check`, `make lint`,
      `make license-check` pass; regenerated bundle committed.
- [ ] Changelog entry added (`neo4j-cli`, kind Minor).

## Out of Scope

- aura/docker/query debug behavior changes (beyond query's desktop-path
  `SetDebug` wiring).
- A new `DesktopConfig` type.
- Bolt-layer debug output for `query`.
- Persisting debug state.

## Open Questions

None.
