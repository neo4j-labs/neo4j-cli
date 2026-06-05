# PRD: mDNS/DNS-SD discovery for the Neo4j Desktop 2 local API (CLI-179)

## Overview

`neo4j-cli` finds Neo4j Desktop 2's local "relate" management API by scanning a
hard-coded port range **44222..44232** (`neo4j-cli/internal/desktopclient/discovery.go`,
`ProbePort`), hitting `GET /fastify/api-docs` and treating a 200 as "present".
Desktop's own port picker (`detectPort(44222)`) walks up on conflict and, when
all 10 standard ports are taken, falls back to an **ephemeral** port (49152+)
the CLI can never discover.

The next Desktop release (neo4j-desktop-2 PR #381, branch `feature/multicast-dns`)
advertises the API over **mDNS/DNS-SD** so clients can locate it regardless of
port. This feature makes `neo4j-cli` discover the API via mDNS first, falling
back to the legacy port scan for current Desktop versions, so the CLI keeps
working across both Desktop generations.

The discovery result already flows through a single abstraction —
`ProbeResult{Port int; Origin string}` — consumed by `NewClient`/`signToken`
(JWT key + HTTP base URL) in `client.go`, the five `desktop` subcommand call
sites, `query --credential desktop`, and `desktop doctor`. mDNS slots in by
producing a `ProbeResult`; downstream code is essentially unchanged.

### Confirmed facts about the new Desktop advertisement (PR #381)
- DNS-SD service type **`_neo4j-desktop-2._tcp.local`**, instance name **`Neo4j Desktop 2`**.
- **SRV** → host `neo4j-desktop-2.local`, **port = the live API port** (authoritative).
- **A/AAAA**: `neo4j-desktop-2.local` → `127.0.0.1` / `::1`; server binds `127.0.0.1:<port>`.
- **TXT**: `api=http://neo4j-desktop-2.local:<port>`, `path=/`, `port=<port>`.

### Critical auth coupling (load-bearing)
The relate auth middleware verifies the CLI's JWT with HMAC secret
`<salt>-<hostName>-<clientId>`, where `hostName = resolveRequestOrigin(env, request.Origin)`.
The CLI sends **no `Origin` header**, so `hostName` falls back to
`environment.httpOrigin`. The **same PR** that adds mDNS changed
`environment.httpOrigin` from `http://localhost:<port>` (current) to
**`http://127.0.0.1:<port>`** (new). All loopback hostnames collapse to that
value (`originsEquivalent`), so the CLI cannot override it via an `Origin`
header. Therefore the `ProbeResult.Origin` — which is **both** the JWT key
component **and** the HTTP base URL — must be:
- mDNS-discovered (new Desktop) ⇒ `http://127.0.0.1:<port>`
- port-scanned (old Desktop) ⇒ `http://localhost:<port>` (unchanged)

This coupling is reliable because both shipped in the same Desktop release.

## Goals

- Discover the Desktop 2 API by service identity (mDNS/DNS-SD), not by a fixed
  port range — so the CLI finds Desktop even on a non-standard or ephemeral port.
- Preserve full backward compatibility with the current Desktop (no mDNS) via the
  existing port scan as a graceful fallback.
- Survive macOS Local Network privacy restrictions on a bare CLI binary by adding
  a `dns-sd` shell-out tier that runs inside the permission-holding `mDNSResponder`.
- Keep the correct JWT/HTTP origin (`127.0.0.1` for new Desktop, `localhost` for
  old) so authenticated calls don't 401.
- Surface discovery state in `desktop doctor`.

## Non-Goals

- Implementing mDNS **advertising** (server side) — that's Desktop's job.
- Changing the relate API surface, request shapes, or any `desktop`/`query`
  command UX beyond discovery + help text.
- Bundling the CLI as a macOS app or requesting entitlements to obtain the Local
  Network grant — out of scope for a CLI; `dns-sd` + port-scan + `--port` cover it.
- Adding a new user-facing flag for origin/host selection (the `--port` override
  plus auto-detection of origin suffices).

## Requirements

### Functional Requirements

- REQ-F-001: Add `DiscoverViaMDNS(ctx)` that browses `_neo4j-desktop-2._tcp.local`
  via `github.com/hashicorp/mdns`, takes the SRV **port** of the first responder,
  and returns `ProbeResult{Port, Origin:"http://127.0.0.1:<port>"}`.
- REQ-F-002: On macOS only, add a `dns-sd` shell-out tier
  (`dns-sd -L "Neo4j Desktop 2" _neo4j-desktop-2._tcp`) that resolves the port via
  `mDNSResponder`, used when the in-process multicast browse returns nothing.
- REQ-F-003: Add a high-level `Discover(ctx, pinned)` orchestrator with ordering:
  - `pinned != 0`: try the pinned port; if an mDNS responder reports that port →
    `127.0.0.1` origin, else fall back to `ProbePort(ctx, pinned)` (`localhost`).
  - `pinned == 0`: (1) mDNS multicast → (2) macOS `dns-sd` if (1) empty →
    (3) `ProbePort(ctx, 0)` legacy scan.
- REQ-F-004: **Every tier fails gracefully and falls through.** A miss, error,
  missing `dns-sd` on PATH, or timeout in any mDNS tier yields a soft
  "not found" and proceeds to the next tier. `Discover` returns `ErrNoDesktop`
  **only** after the legacy port scan also misses.
- REQ-F-005: Origin rule enforced: mDNS/`dns-sd` results use
  `http://127.0.0.1:<port>`; port-scan results keep `http://localhost:<port>`.
  `ProbeResult` and `client.go` consume it unchanged.
- REQ-F-006: Migrate the five `ProbePort` callers to `Discover`:
  `query/desktop.go`, `subcommands/desktop/helpers.go`, `dbms/helpers.go`,
  `connection/create.go`, `dbms/plugin/helpers.go`. `ErrNoDesktop` branching
  is preserved.
- REQ-F-007: `ProbePort` (legacy scan) remains intact and is still used as tier 3
  and by the e2e port override.
- REQ-F-008: Add a non-gating `mdns_discovery` check to `desktop doctor` (mirrors
  the `info_app` INFO/skip pattern): reports whether mDNS found a responder and on
  what port; on macOS, hints about the Local Network permission when nothing was
  found. Refresh doctor `Long`/`Example` and `checkStandardProbe` strings that say
  "probing 44222..44232" to reflect "mDNS first, then the 44222..44232 fallback".
- REQ-F-009: Update `canonicalUnreachable` (`client.go`) to mention mDNS, the macOS
  Local Network permission, and the `--port` escape hatch.
- REQ-F-010: Add an e2e seam `e2eMDNSPortOverride` driven by
  `NEO4J_CLI_DESKTOP_E2E_MDNS_PORT` (build-tag `e2e_desktop_seams`), mirroring
  `e2ePortOverride`; reuse `e2eOriginOverride` for the origin.
- REQ-F-011: Add a `mdnsBrowseFn` seam (and a `dnssdLookupFn` seam) with
  `SetXxxFnForTest` restorers, matching the package's existing test-seam pattern.
- REQ-F-012: Regenerate the skill bundle (`go generate ./neo4j-cli/internal/skill/...`)
  after doctor help-text edits and commit the result (CI gate `make generate-check`).
- REQ-F-013: Add a single `changie` entry, kind **Minor**, project `neo4j-cli`.

### Non-Functional Requirements

- REQ-NF-001: Discovery latency — common case (Desktop present + advertising)
  resolves in tier 1 in well under 1s; absent-Desktop worst case (all tiers miss)
  stays roughly ~3–4s. Tier timeouts: mDNS multicast ~750ms, `dns-sd` ~1.5s, plus
  the existing ~2.2s port scan.
- REQ-NF-002: Hermetic tests — unit tests drive all tiers through seams; **no real
  multicast or `dns-sd` invocation** in the default `make test` (must pass on
  ubuntu, windows, and macos runners).
- REQ-NF-003: New deps must be permissively licensed (MIT/BSD/Apache):
  `github.com/hashicorp/mdns` (MIT) + transitive `miekg/dns`, `golang.org/x/net`
  (BSD). All `.go` files carry the Neo4j copyright header (`make license-check`).
- REQ-NF-004: All mDNS library imports + the `dns-sd` exec are isolated in one new
  file (`discovery_mdns.go`) so the dependency surface is contained.
- REQ-NF-005: `make build`, `make test`, `make fmt-check`, `make lint`,
  `make license-check`, and `make generate-check` all pass.

## Technical Considerations

- **Library**: `github.com/hashicorp/mdns` (MIT, active, `QueryContext(ctx)` for
  clean timeouts). Lower-level than zeroconf — we parse `*ServiceEntry.Port`
  ourselves, which is all we need.
- **macOS Local Network gate**: macOS 15+ may silently drop a bare CLI's multicast
  on `224.0.0.251:5353` (no prompt, no error → zero entries). The `dns-sd` tier
  uses `mDNSResponder`, which already holds the grant, so it succeeds where the
  CLI's own socket is blocked. Rationale captured as a doc comment atop
  `discovery_mdns.go`.
- **`dns-sd` parsing**: `dns-sd -L` streams and never exits — run under a context
  deadline, read stdout, parse the first `… can be reached at <host>:<port>` line,
  then kill. Gate on `goosFn()=="darwin"` (existing seam) and `exec.LookPath`.
- **Port-only from mDNS**: ignore the SRV target/A/TXT host; always talk to
  `127.0.0.1:<port>` (server binds it, auth forces the origin).
- **Pinned-port ambiguity**: a `--port` value alone can't tell old vs new Desktop;
  resolve by an mDNS confirm (responder on that port ⇒ `127.0.0.1`, else `localhost`).
  No new flag.
- **e2e/test seams**: new vars declared in `discovery.go`, assigned only in
  `seams_e2e.go` (mirrors `e2ePortOverride`); `seams_default.go` unchanged.

### Files
- **Create**: `neo4j-cli/internal/desktopclient/discovery_mdns.go`,
  `…/discovery_mdns_test.go`, `.changes/unreleased/neo4j-cli-Minor-<ts>.yaml`.
- **Modify**: `discovery.go` (`Discover`, `discoverPinned`, `DiscoverViaMDNS`,
  `mdnsOrigin`, `e2eMDNSPortOverride`), `client.go` (`canonicalUnreachable`,
  field comment), `seams_e2e.go`, the 5 caller sites, `doctor.go`/`doctor_checks.go`/
  `doctor_orchestrator.go` (+ `mdns_discovery` check), affected `*_test.go`,
  `go.mod`/`go.sum`, generated `internal/skill/bundle/references/desktop.md`.

## Acceptance Criteria

- [ ] With the new mDNS-enabled Desktop running, `neo4j-cli desktop list` and
      `desktop doctor` discover it via mDNS (origin `127.0.0.1`), including when
      Desktop is on an ephemeral port outside 44222..44232.
- [ ] With the current (non-mDNS) Desktop running, discovery still succeeds via the
      44222..44232 scan (origin `localhost`) and authenticated calls work.
- [ ] On macOS, when in-process multicast is blocked, the `dns-sd` tier still finds
      Desktop; when `dns-sd` finds nothing, discovery falls back to the port scan.
- [ ] When no Desktop is running, `Discover` returns `ErrNoDesktop` and the CLI
      shows the updated unreachable message (mentions mDNS, macOS permission, `--port`).
- [ ] `desktop doctor` shows an `mdns_discovery` row (non-gating) in table and JSON
      output; existing checks/summary semantics unchanged.
- [ ] Unit tests cover every tier via seams and pass hermetically on ubuntu/windows/macos.
- [ ] `make build`, `make test`, `make fmt-check`, `make lint`, `make license-check`,
      `make generate-check` all pass; one Minor changie entry present.

## Out of Scope

- mDNS advertising / any Desktop-side change.
- Mutual auth / signed responses to close the local-bind-race residual risk.
- A standalone "discover Desktop" command (discovery stays internal to existing commands).
- Origin/host override flags beyond `--port`.

## Open Questions

- None blocking. (Instance name `Neo4j Desktop 2` and the ~3–4s worst-case timeout
  budget are confirmed; graceful per-tier fallback to the port scan is required.)
  If Desktop ever localizes/renames the mDNS instance, switch the `dns-sd` tier
  from `-L "Neo4j Desktop 2"` to a `-B` browse that discovers the instance first.
