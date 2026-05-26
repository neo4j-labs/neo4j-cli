# PRD: Desktop doctor diagnostic leaf

## Overview

A new `neo4j-cli desktop doctor` cobra leaf that runs an ordered, structured health check against Neo4j Desktop 2 and reports the result on stdout (JSON / toon) or as a human-readable table. Replaces the all-or-nothing `canonicalUnreachable` error with a per-check breakdown so users and agents can diagnose exactly which stage of Desktop integration is broken: install missing, data directory missing, auth data unreadable, relate API unreachable, or authentication failing.

## Goals

- Give the user a single command that diagnoses whether Desktop is installed, configured, reachable, and authenticated.
- Surface structured JSON output suitable for agent consumption (each check is a named row with status + detail + optional hint).
- Recover gracefully when Desktop is on a non-default port — accept `--port` override and surface the user-supplied value in the summary.
- Work uniformly on macOS, Linux, and Windows by reusing the existing OS-aware seams (`goosFn`, install-detect helpers, data-dir resolver, salt loader).

## Non-Goals

- Process-listing-based port discovery (no `gopsutil` dependency, no `lsof` / `netstat` parsing). Users on a random ephemeral port must pass `--port` themselves.
- Modifying Desktop state — read-only command, no `--rw`.
- Auto-fixing problems (no "reset password", no "restart Desktop", no config writes).
- Replacing the canonical "Desktop not running" error elsewhere — `canonicalUnreachable` stays for direct-call failures; it just gains a one-line pointer to `desktop doctor`.

## Requirements

### Functional Requirements

- REQ-F-001: A new cobra leaf `neo4j-cli desktop doctor` registered under the existing `desktop` parent in `neo4j-cli/internal/subcommands/desktop/desktop.go`, following the repo's one-file-per-leaf convention.
- REQ-F-002: The leaf runs an ordered sequence of five health checks: (1) install present, (2) data directory present, (3) auth data readable, (4) standard port probe, (5) authenticated probe. Each check produces a `{name, status, detail, hint?}` record.
- REQ-F-003: When a check FAILs, every later check that depends on its output is rendered with status `skip` and a `(depends on …)` detail. Dependency order: probe depends on (nothing); auth-probe depends on probe; install / data-dir / auth-data are independent.
- REQ-F-004: The check sequence reuses existing helpers without modification: `detectInstalled` (`install_detect.go:134`), `ResolveDataDir` + `LoadSalt` (`desktopclient/discovery.go`), `ProbePort` (`desktopclient/discovery.go:118`), `NewClient` + a cheap authed read (`desktopclient/client.go:174`).
- REQ-F-005: The leaf accepts the `--port int` flag inherited from the desktop parent. When set, the standard-port-probe check tries only that port instead of the 44222..44232 range.
- REQ-F-006: When `--format json` (or `toon`) is selected, stdout receives a single `doctorReport` document with `{checks: [...], summary: {...}}`. The summary includes `reachable: bool`, `port: int?`, `standard_port_range: bool`, `next_step: string?`.
- REQ-F-007: When `--format table` (or default-TTY) is selected, stdout renders a multi-row human-readable layout with name, status keyword (`PASS` / `FAIL` / `SKIP` / `INFO`), and detail. A trailing one-line summary follows when applicable.
- REQ-F-008: The "Auth data readable" check is labelled generically in user-visible output — no mention of "secret", "JWT", "key", or "salt" in the row label. The JSON field name is also neutral (e.g. `auth_data_readable`).
- REQ-F-009: The leaf always exits 0, regardless of which checks FAIL. Gating consumers parse `summary.reachable` (or another structured field). Diagnostic decoupled from process exit semantics.
- REQ-F-010: `canonicalUnreachable` in `neo4j-cli/internal/desktopclient/client.go` is updated to append `or run 'neo4j-cli desktop doctor' to scan.` so users hitting an unreachable error are pointed to the new command.
- REQ-F-011: The leaf carries a non-empty flush-left `Example:` cobra field with ≥3 invocations (e.g. `neo4j-cli desktop doctor`, `neo4j-cli desktop doctor --port 44222`, `neo4j-cli desktop doctor --format json`) per the `TestAllLeafCommands_HaveExamples` gate in `neo4j-cli/internal/subcommands/agentcontext/agentcontext_test.go`.

### Non-Functional Requirements

- REQ-NF-001: All new `.go` files carry the standard Neo4j license header (`// Copyright (c) "Neo4j"` / `// Neo4j Sweden AB [http://neo4j.com]`) per `make license-check`.
- REQ-NF-002: New tests are colocated as `doctor_test.go`, table-driven, and use the existing `Set*FnForTest` seams (`SetGOOSFnForTest`, `SetDetectHomeDirFnForTest`, `SetHomeDirFnForTest`, `SetHTTPDoFnForTest`) so they remain hermetic.
- REQ-NF-003: Cross-platform unit tests cover macOS, Linux, and Windows install-detection branches via `SetGOOSFnForTest`. CI's existing OS matrix runs `make test` on all three.
- REQ-NF-004: `go generate ./neo4j-cli/internal/skill/...` is run as part of the change so the agent skill bundle reflects the new leaf; `TestGenerator_RoundTrip` passes.
- REQ-NF-005: No new module dependency added (`go.mod` and `go.sum` untouched). Sanity-check the built `bin/neo4j-cli` size is within ±0.5 MiB of the pre-change baseline.
- REQ-NF-006: Pure-Go cross-compile for Windows (`GOOS=windows GOARCH=amd64 go build`) still succeeds without CGO.

## Technical Considerations

- **File layout**: one new file `neo4j-cli/internal/subcommands/desktop/doctor.go` containing the leaf constructor (`newDoctorCmd(cfg *clicfg.Config) *cobra.Command`) + per-check helpers + output rendering. Tests in `doctor_test.go`. One-line `cmd.AddCommand(newDoctorCmd(cfg))` in `desktop.go` next to `newListCmd`.
- **Output formatting**: the report shape doesn't fit `output.PrintBodyMap`'s `ResponseData` row-array contract cleanly. Doctor writes JSON / toon via `json.MarshalIndent` / direct `toon-go` encoding, and renders the table format with `fmt.Fprintf` directly on `cmd.OutOrStdout()`. Format resolution still uses `output.StdoutIsTerminal` + `cfg.Global.Format()`.
- **`--port` semantics**: inherited from `desktop` parent's persistent flag (already exists, used by every leaf in the subtree). When 0 (default), `ProbePort` scans 44222..44232; when non-zero, it tries that port only.
- **Authenticated probe choice**: use a minimal authed call (e.g. `GetDbms` with a known-bad id) — 2xx OR an expected 404 both prove auth works. Avoid expensive calls (`ListDbms` does I/O against every DBMS, which might be slow).
- **Cross-link**: `canonicalUnreachable` is a single `const` in `client.go:80`. Updating it affects every code path that returns it (probe miss, connection refused, EOF). One unit-test addition asserts the doctor hint is present; existing assertions on `doesn't appear to be running` and `--port` continue to hold.
- **No process-list fallback**: explicitly deferred. If Desktop falls past port 44231 to a random ephemeral port, the standard probe FAILs and the summary tells the user to pass `--port` themselves.
- **Bundle regen**: adding a new leaf under `desktop` requires `go generate ./neo4j-cli/internal/skill/...` per AGENTS.md "Adding any new command to the neo4j-cli command tree". `TestGenerator_RoundTrip` is the gate.
- **Existing PRD overlap**: the parent feature CLI-32 (`prd-cli-32-local-desktop-management.md`) shipped the `desktop` subtree; this PRD is a follow-on within the same release cycle. Coordinate so the doctor leaf and the existing `desktop` leaves stay in style sync (Long format, Example format, --port semantics).

## Acceptance Criteria

- [ ] `neo4j-cli desktop doctor --help` lists the leaf with a non-empty `Long`, ≥3 flush-left examples, and the inherited `--port` + `--format` flags.
- [ ] With Desktop running on 44222: all five checks render PASS; summary reports `reachable=true`, `port=44222`, `standard_port_range=true`, no `next_step`.
- [ ] With Desktop stopped: install / data-dir / auth-data checks PASS, standard probe FAILs, authenticated probe is `SKIP`, summary reports `reachable=false`, `next_step` points to starting Desktop.
- [ ] With `--port 12345` (nothing listening): probe FAILs with detail naming 12345, auth-probe `SKIP`, summary mentions the user-supplied port.
- [ ] `neo4j-cli desktop doctor --format json` emits parseable JSON matching the schema described in REQ-F-006; output is byte-identical regardless of TTY presence on stdout.
- [ ] Existing `TestUnreachableError_MatchesCanonicalMessage` still passes; a new test asserts the doctor hint is appended.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check`, `make generate-check` are all clean.
- [ ] `neo4j-cli desktop doctor` exits 0 in every scenario, verified by unit tests asserting `cmd.Execute() == nil`.
- [ ] `bin/neo4j-cli` size within ±0.5 MiB of the pre-change baseline (sanity check: no accidental heavy dep pulled in).

## Out of Scope

- Process-listing / `gopsutil`-based port discovery for the random-ephemeral-port case.
- Auto-fixing detected problems (resetting passwords, restarting Desktop, writing config files).
- Replacing the existing `canonicalUnreachable` error — it stays for direct-call failures, just gains a one-line pointer to doctor.
- A separate changelog entry — the doctor leaf rolls into the existing CLI-32 minor entry already drafted (`.changes/unreleased/neo4j-cli-Minor-20260518-113428.yaml`); no new changie file.
- WSL detection / cross-host port hints — only relevant if process-list discovery is added; deferred with that.

## Open Questions

- None outstanding. Exit-code semantics resolved as "always exit 0" (REQ-F-009).
