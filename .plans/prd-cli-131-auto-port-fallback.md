# PRD: Auto-port fallback for `docker create`

Linear: [CLI-131](https://linear.app/neo4j/issue/CLI-131/handle-random-port-allocation-for-parallel-containers)
Source plan: `/Users/oskarhane/.claude/plans/agile-floating-lobster.md`
Suggested branch: `oskar/cli-131-auto-port-fallback`

## Overview

`neo4j-cli docker create` currently hard-errors when `--bolt-port` (default 7687) or `--http-port` (default 7474) is in use, telling the operator to pass a different value (`create.go:498-505`). That breaks the common case of running multiple Neo4j containers in parallel — every integration-test runner has to do its own port allocation.

Testcontainers solves this by allocating free ports and reporting the resolved values in its connection URLs. CLI-131 brings the same ergonomics to `docker create`: when the requested host ports are taken, walk **both** ports forward by the same offset starting from the requested pair — `(7687,7474) → (7688,7475) → (7689,7476) → …` — until both are free. The resolved pair is reported in stderr narration and the existing structured output (`bolt-port`, `http-port`, `uri`), the persisted dbms credential, and the container labels (`org.neo4j.cli.bolt-port`, `org.neo4j.cli.http-port`).

Auto-resolve is the default behaviour. It applies regardless of whether `--bolt-port` / `--http-port` were explicit or left at defaults. No opt-in / opt-out flag in v1.

## Goals

- Replace the hard-error "port in use" pre-flight (`create.go:498-505`) with a soft fallback loop that walks both ports forward by the same offset until both are free.
- Preserve the equal-ports guard (`boltPort == httpPort`) as a fast-fail BEFORE iteration — a misconfigured invocation surfaces immediately and the loop never has to consider equal pairs.
- Emit one stderr `info:` line when the resolved pair differs from the requested pair, matching the existing narration style (`create.go:227, 334, 353, 630`).
- No output schema change: `bolt-port`, `http-port`, `uri`, the stored dbms credential, and the container labels (`LabelBoltPort`, `LabelHTTPPort` in `labels.go:17-18`) already key off the chosen ports.
- Reuse the existing `listenerFactory` test seam (`create.go:55-57`) and `stubListenerFactory` test helper (`create_test.go:55-77`) — no new injection surface.
- Update all documentation surfaces (README.md, `additions.md`, regenerated `bundle/SKILL.md` and `bundle/references/docker.md`, cobra `Long:` / flag help, changelog entry).

## Non-Goals

- **No `--strict-port` flag in v1.** Soft fallback is unconditional. Operators who need strict behaviour can read stderr or assert on the JSON `bolt-port` field. Add a flag later if anyone asks.
- **No `--auto-port` opt-in flag.** Behaviour is on by default; no toggle.
- **No ephemeral port allocation via `net.Listen("tcp", ":0")`.** The user picked a deterministic walk so the chosen ports stay predictable and grouped near the defaults.
- **No separate stride for bolt vs http.** The offset is shared, preserving any operator-supplied gap between the two (e.g. `--bolt-port 8000 --http-port 9000` taken → `(8001, 9001)`).
- **No iteration cap beyond 100 attempts** (mirrors `maxNameSuffix` in `create.go:39`). On exhaustion we surface a `clierr.UsageError`.
- **No race-elimination between probe and `docker run`.** Same best-effort window as the existing `checkPortFree` — documented behaviour, not a regression.
- **No changes to `docker start` / `docker stop` / other leaves.** Only the create-time port pre-flight changes.
- **No retroactive re-probing of resolved labels.** The container's `org.neo4j.cli.bolt-port` / `org.neo4j.cli.http-port` labels already record the actually-used ports; no migration needed.
- **No website (`gh-pages`) edits in this PR.** The site is prompt-driven and rolled out separately.

## Requirements

### Functional Requirements

#### Pre-flight loop

- **REQ-F-001:** Replace `checkPortFree(boltPort, ...)` + `checkPortFree(httpPort, ...)` in `create.go` `RunE` with a single call to a new `findFreePortPair(boltStart, httpStart int) (bolt, http int, err error)` helper. The helper iterates `offset = 0 … maxPortOffset-1` and returns the first pair `(boltStart+offset, httpStart+offset)` where BOTH ports probe-free.
- **REQ-F-002:** Add a private `portFree(port int) bool` helper that wraps `listenerFactory(port)` + `ln.Close()`, returning `true` when the listener bind succeeds. The existing `listenerFactory` seam (`create.go:55-57`) is the single point of network I/O — production binds an ephemeral TCP listener, tests swap in `stubListenerFactory`.
- **REQ-F-003:** Add a package-level `const maxPortOffset = 100` placed next to `maxNameSuffix` (`create.go:39`) for symmetry. The comment cites parity with `maxNameSuffix=99` so future readers see the intent.
- **REQ-F-004:** On exhaustion (all 100 candidate pairs busy), return `clierr.NewUsageError("could not find a free port pair starting at %d/%d after %d attempts; pass --bolt-port / --http-port", boltStart, httpStart, maxPortOffset)`. Same error type as the current `checkPortFree` so the caller-side `clierr` handling is unchanged.
- **REQ-F-005:** The equal-ports guard `if boltPort == httpPort { return clierr.NewUsageError(...) }` (`create.go:206-208`) STAYS in place and STAYS BEFORE the new loop. It guarantees the loop never has to consider equal pairs.
- **REQ-F-006:** Auto-resolve fires regardless of whether `--bolt-port` / `--http-port` were explicit or left at defaults. No `cmd.Flags().Changed("bolt-port")` check is introduced. The loop starts at whatever pair was requested.
- **REQ-F-007:** When the loop succeeds, reassign the local `boltPort` / `httpPort` variables in `RunE` to the resolved values BEFORE the rest of the function runs. Downstream consumers (argv builder, container labels, credential URI, output row) read these variables and automatically pick up the resolved pair.

#### Stderr narration

- **REQ-F-010:** When the resolved pair differs from the requested pair, emit ONE line on stderr:

  ```
  info: ports 7687/7474 in use; using 7689/7476 (bolt/http)
  ```

  Use the existing `_, _ = fmt.Fprintf(cmd.ErrOrStderr(), ...)` idiom to match other `info:` lines (`create.go:227, 334, 353, 630`). Numbers reference the requested pair (left of `using`) and the resolved pair (right of `using`).
- **REQ-F-011:** When the resolved pair equals the requested pair (happy path, no fallback needed), no narration is emitted.

#### Output / persistence

- **REQ-F-020:** No schema change to the rendered table / JSON / TOON output of `create`. The `bolt-port`, `http-port`, and `uri` fields automatically reflect the resolved pair because they read from the (now-reassigned) `boltPort` / `httpPort` / locally-built `uri` variables.
- **REQ-F-021:** The dbms credential persisted via `cfg.Credentials.Dbms.Add(chosenName, "neo4j", resolvedPassword, "neo4j", uri)` (`create.go:320`) automatically uses the resolved Bolt port because `uri` is built from `boltPort` after reassignment.
- **REQ-F-022:** The `--label org.neo4j.cli.bolt-port=<port>` / `org.neo4j.cli.http-port=<port>` labels (`create.go:297-298`, constants in `labels.go:17-18`) automatically use the resolved ports for the same reason.
- **REQ-F-023:** The `--ephemeral` env-file blob (`renderEnvFile` in `create.go:402-410`) automatically uses the resolved Bolt port via the same `uri` variable.

#### Documentation

- **REQ-F-030:** Update `--bolt-port` flag help (`create.go:384`) to read approximately: `Host port to publish for Bolt (container 7687). Auto-incremented along with --http-port if taken.`
- **REQ-F-031:** Mirror on `--http-port` flag help (`create.go:385`): `Host port to publish for the HTTP browser (container 7474). Auto-incremented along with --bolt-port if taken.`
- **REQ-F-032:** Append one sentence to the cobra `Long:` paragraph for `create` (`create.go:118-136`) so `--help` surfaces the fallback behaviour: "When the requested `--bolt-port` and `--http-port` pair is taken, both ports are auto-incremented by the same offset (up to 100 attempts) and the chosen pair is reported on stderr."
- **REQ-F-033:** Update `neo4j-cli/internal/skill/additions.md:17` (currently: `Host ports default to 7474 (HTTP) and 7687 (Bolt); override with --http-port / --bolt-port.`) to mention auto-increment fallback.
- **REQ-F-034:** Update `README.md:111` (same line) to match.
- **REQ-F-035:** Regenerate the skill bundle via `go generate ./neo4j-cli/internal/skill/...` so `bundle/SKILL.md` (line 51) and `bundle/references/docker.md` (flag table lines 31, 36) pick up the new wording. `TestGenerator_RoundTrip` is the gate.
- **REQ-F-036:** Add a changelog entry: `changie new --projects neo4j-cli --kind Minor --body "docker create: auto-increment host ports when defaults are taken (CLI-131)"`.

#### Tests

- **REQ-F-040:** Update existing `TestCreate_PortPreflight` subtests in `create_test.go:351-383`:
  - `"bolt port occupied surfaces --bolt-port hint"` — flip from asserting `clierr.UsageError` to asserting the fallback resolved pair lands on `(7688, 7475)` and the stderr `info:` line fires.
  - `"http port occupied surfaces --http-port hint"` — same flip, asserting `(7688, 7475)` when http=7474 is busy.
  - `"custom bolt port occupied is named correctly"` — flip to assert `(9000, 9001)`-style resolution starting from the user-supplied pair.
  - `"equal ports rejected before any Listen"` STAYS UNCHANGED — the equal-ports guard still fires up front.
- **REQ-F-041:** Add `TestCreate_AutoPortFallback_DefaultsTaken` — mark 7687 + 7474 busy, expect resolved `(7688, 7475)`, assert (a) the `info:` line on stderr names both pairs, (b) the argv built for `docker run` carries `-p 7475:7474 -p 7688:7687`, (c) the rendered output row has `bolt-port=7688 http-port=7475 uri=neo4j://localhost:7688`.
- **REQ-F-042:** Add `TestCreate_AutoPortFallback_PreservesOffset` — `--bolt-port 8000 --http-port 9000`, mark 8000 busy, expect resolved `(8001, 9001)`. Verifies the shared-offset invariant.
- **REQ-F-043:** Add `TestCreate_AutoPortFallback_Exhausted` — mark every port in `[7687, 7786]` AND `[7474, 7573]` busy (full 100-step range), expect `clierr.UsageError` whose message names the start pair `7687/7474` and the cap `100`. Verifies the exhaustion error path.
- **REQ-F-044:** Add `TestCreate_AutoPortFallback_NoNarrationOnHappyPath` — defaults free, assert NO `info:` stderr line about ports fires (only existing narration permitted).
- **REQ-F-045:** All new tests use the existing `stubListenerFactory` seam (`create_test.go:55-77`). No new injection points are introduced.

### Non-Functional Requirements

- **REQ-NF-001:** Cross-platform parity. The new code path uses only `net.Listen` (already cross-platform via existing `listenerFactory`) and `fmt.Fprintf(cmd.ErrOrStderr(), …)`. CI matrix (ubuntu, windows, macos) keeps the existing gates green.
- **REQ-NF-002:** No new dependencies. The change touches only `create.go` and tests; no new imports beyond what's already in scope.
- **REQ-NF-003:** Performance — the 100-attempt cap with two `net.Listen` calls each (~µs in the happy/contended case) keeps create-time pre-flight ≤ ~10 ms even at exhaustion. No measurable user-visible latency increase.
- **REQ-NF-004:** Backwards compatibility for scripted consumers. The output schema (`bolt-port`, `http-port`, `uri`) is unchanged; only the VALUES may differ from what the operator requested. Scripts that parse JSON output continue to work. Scripts that hard-coded "exit 1 on port-in-use" no longer trigger that exit and instead see a success with possibly different ports — flagged in the changelog body as a behaviour change.
- **REQ-NF-005:** Format gates. `make test`, `make fmt-check`, `make lint`, `make generate-check` must all pass before merge. The `go generate ./neo4j-cli/internal/skill/...` step is mandatory because the bundle includes the cobra `Long:` and flag help.
- **REQ-NF-006:** No silent re-allocation of ports beyond 65535. Implicit cap from `net.Listen` returning an error for out-of-range ports — the loop will surface those as "port not free" and continue, then exhaust. Acceptable behaviour for v1; operators starting near 65535 should pick a saner pair.

## Technical Considerations

**Implementation seams.** The change is intentionally contained to `neo4j-cli/internal/subcommands/docker/create.go` and its test file. The new `portFree` / `findFreePortPair` helpers sit next to the existing `checkPortFree` (which becomes unused and is deleted). The existing `listenerFactory` package var is the only network seam and remains unchanged.

**Equal-ports guard ordering.** The current `if boltPort == httpPort` check fires before any `net.Listen` call (`create.go:206-208`). Preserve this ordering: it short-circuits a misconfigured invocation before the loop, so the loop's invariant "both ports start unequal and shift by the same offset → resolved ports remain unequal" holds automatically.

**Race window.** `checkPortFree` already has a TOCTOU window between `ln.Close()` and `docker run` claiming the port. The new loop preserves that window (it has to — `docker run` happens after the pre-flight). Acceptable because (a) the window is sub-second, (b) `docker run` will fail loudly if it loses the race, (c) the existing label-driven discovery means a half-created container can be cleaned up with `docker delete`. No mitigation in v1.

**Cobra `Long:` regeneration cascade.** Touching `create.go`'s `Long:` field or flag Usage strings triggers `TestGenerator_RoundTrip` failure unless `go generate ./neo4j-cli/internal/skill/...` runs (see CLAUDE.md "Cobra Help / Skill Bundle Rendering Notes"). The implementation MUST regenerate and commit the bundle in the same change.

**Test ergonomics.** `stubListenerFactory` (`create_test.go:55-77`) returns a `*[]int` of probed ports. The new exhaustion test will probe 200 ports (100 bolt + 100 http) — assert on the count and on the last-probed pair, not on the full slice contents.

## Acceptance Criteria

- [ ] `neo4j-cli docker create --name a --rw` succeeds on 7687/7474 when free, output row shows those ports, no `info:` stderr line about ports.
- [ ] A second invocation `neo4j-cli docker create --name b --rw` (with `a` already holding 7687/7474) succeeds on `(7688, 7475)`, emits `info: ports 7687/7474 in use; using 7688/7475 (bolt/http)` on stderr, output row shows `bolt-port=7688 http-port=7475 uri=neo4j://localhost:7688`, the persisted dbms credential for `b` targets `neo4j://localhost:7688`, and the container labels `org.neo4j.cli.bolt-port=7688` / `org.neo4j.cli.http-port=7475` reflect the resolved pair.
- [ ] `neo4j-cli docker create --name c --bolt-port 8000 --http-port 9000 --rw` with 8000 busy lands on `(8001, 9001)`. The offset is preserved.
- [ ] When all 100 candidate pairs are busy, the command exits with a `clierr.UsageError` whose message names the start pair and the cap.
- [ ] `neo4j-cli docker create --name a --bolt-port 7687 --http-port 7687` still errors with the existing equal-ports message — the guard fires before the loop.
- [ ] `make test`, `make fmt-check`, `make lint`, `make generate-check` all pass.
- [ ] `bundle/SKILL.md` and `bundle/references/docker.md` reflect the updated flag help and `Long:` paragraph (regenerated via `go generate`).
- [ ] `README.md:111` and `additions.md:17` mention the auto-increment fallback.
- [ ] A `Minor` changelog entry is added via `changie new --projects neo4j-cli --kind Minor --body "..."`.

## Out of Scope

- `--strict-port` opt-out flag.
- `--auto-port` opt-in flag.
- Ephemeral port allocation via `net.Listen("tcp", ":0")`.
- Per-port stride (bolt-only or http-only auto-resolve).
- Iteration cap configurability.
- Retroactive port reassignment for already-created containers.
- Changes to `docker start` / `docker stop` / other leaves' port handling.
- Website (`gh-pages`) updates.

## Open Questions

None — the design is fully pinned by the source plan (`Resolved decisions` section).
