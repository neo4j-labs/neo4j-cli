# PRD: Add `--debug` to Docker Commands

## Overview

Add a `--debug` flag to the `neo4j-cli docker` command tree so operators can see
the underlying host `docker` CLI invocations the tool shells out to. This mirrors
the just-merged aura `--debug` (CLI-197) and the existing query `--debug`, all of
which share `common/debug.Resolve` for `--debug` / `NEO4J_DEBUG` semantics. Debug
output goes to stderr only and redacts secrets; stdout (the command's real result)
is unaffected.

This is CLI-198, a child of CLI-169.

## Goals

- Give docker users visibility into which `docker` commands run (diagnose
  create/load/start/stop failures) without enabling shell tracing.
- Reuse the established `--debug` / `NEO4J_DEBUG` resolution shared by aura and
  query (`common/debug.Resolve`, strict env value `"1"`, flag overrides env).
- Centralize the one duplicated redaction primitive (`StripControl(RedactText)`)
  into `common/debug.Scrub`, used by both docker and aura.
- Never leak secrets: argv is scrubbed; env var **names only** are emitted (never
  values — secrets travel via `cmd.Env`, e.g. `NEO4J_AUTH`).

## Non-Goals

- Changing query's or aura's debug behavior beyond aura's `scrub` delegating to
  the new `common/debug.Scrub` (no aura test/behavior changes).
- Debugging the Bolt-readiness probe (`WaitForBolt`) — it uses the neo4j driver,
  not the docker CLI, and is covered by query `--debug`.
- Dumping docker stdout bodies into debug output (concise emit only).
- Persisting debug state to disk.

## Requirements

### Functional Requirements

- REQ-F-001: A persistent `--debug` boolean flag is registered on the `docker`
  parent command (`NewCmd`), inherited by all 7 leaves (create, delete, get,
  list, load, start, stop).
- REQ-F-002: Resolution uses `common/debug.Resolve(cmd)`: explicit `--debug`
  (Changed) wins over env; otherwise enabled iff `NEO4J_DEBUG == "1"` (strict).
  `--debug=false` overrides `NEO4J_DEBUG=1`.
- REQ-F-003: When debug is on, each `docker` invocation through
  `execClient.runEnv` emits to stderr, before exec:
  `[docker-debug] > docker <scrubbed, redacted argv>` followed by one line listing
  the injected env **variable names only** (never values).
- REQ-F-004: After exec, emit `[docker-debug] < exit <code> elapsed <duration>`.
  The exit code is extracted via `errors.As(err, *exec.ExitError)` → `.ExitCode()`,
  falling back to `-1` when no exit code is available (e.g. signal/ctx cancel).
- REQ-F-005: The resolved debug bool is injected into the client at construction:
  `newClient(debug bool)`; the package `clientFactory` seam becomes
  `func(debug bool) dockerClient`; each leaf calls
  `clientFactory(debug.Resolve(cmd))`.
- REQ-F-006: `NewDeployClient()` (external aura-deploy caller) keeps its no-arg
  signature and constructs a non-debug client (`newClient(false)`).
- REQ-F-007: `common/debug` exposes exported
  `Scrub(s) = output.StripControl(clievents.RedactText(s))`; aura's local `scrub`
  delegates to it.

### Non-Functional Requirements

- REQ-NF-001: Secrets never appear in debug output — argv passes through the
  existing `redactArgs` plus `Scrub`; env values are never emitted, only names.
- REQ-NF-002: Control/ANSI bytes are neutralized via `StripControl` (inside
  `Scrub`) before any string reaches the terminal.
- REQ-NF-003: stdout bytes are byte-for-byte identical with and without `--debug`
  (debug is stderr-only).
- REQ-NF-004: Debug state lives on the `execClient` instance (no process-global
  mutable flag) so tests remain parallel-safe. The writer is a package-level
  `var debugW io.Writer = os.Stderr` seam (mirrors aura), overridable in tests.
- REQ-NF-005: No import cycle — `common/debug` may import `common/clievents` and
  `common/output` (neither imports `common/debug`).

## Technical Considerations

- **Single funnel**: all docker methods (`Run`, `Start`, `Stop`, `RemoveForce`,
  `PsAll`, `Inspect`, `Exec*`, `CopyTo`) pass through `execClient.runEnv`, so emit
  lives there once.
- **Testability**: `runEnv` shells to real docker (resolves `docker` via
  `exec.LookPath` directly, no seam), so unit tests use a fake `dockerClient` and
  never hit `runEnv`. Extract emit into standalone helpers writing to `debugW` so
  they can be unit-tested against a `bytes.Buffer`; the real-docker wiring is
  asserted in the `-tags=smoke` suite only.
- **Generated-artifact drift**: the new persistent flag changes the rendered
  `LocalFlags()` for docker and the inherited-flags listing in agent-context, so
  `bundle/references/docker.md` and agent-context output drift —
  `go generate ./...` must be run and the regenerated artifacts committed
  (`make generate-check` is a CI gate).
- **Flag wording**: mirror aura's flag description style, including the
  `[env: NEO4J_DEBUG (set to 1 to enable)]` suffix.

## Acceptance Criteria

- [ ] `neo4j-cli docker <leaf> --debug ...` prints `[docker-debug] >` / `<` lines
      to stderr for each docker invocation.
- [ ] Generated container password (and any env value) never appears in debug
      output; env lines show names only (e.g. `NEO4J_AUTH`).
- [ ] `NEO4J_DEBUG=1` enables debug without the flag; `--debug=false` overrides it.
- [ ] stdout is identical with and without `--debug`.
- [ ] `common/debug.Scrub` exists and aura's `scrub` delegates to it; aura tests
      unchanged and passing.
- [ ] Unit tests cover emit helpers (argv scrubbed, env keys-only/no-values,
      exit/elapsed shape, off-path silent); a smoke test asserts `[docker-debug]`
      reaches stderr against real docker.
- [ ] `make generate-check`, `make test`, `make fmt-check`, `make lint` pass;
      regenerated bundle/agent-context committed.
- [ ] Changelog entry added (`neo4j-cli`, kind Minor).

## Out of Scope

- query / aura debug behavior changes (beyond the `scrub` delegation).
- Bolt-readiness probe debug output.
- Dumping docker stdout/response bodies.

## Open Questions

None.
