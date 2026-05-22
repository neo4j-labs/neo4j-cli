# PRD: Discover Desktop 2 data dir via `/info/app` (CLI-32 extension)

Linear: https://linear.app/neo4j/issue/CLI-32/local-neo4j-desktop-management-v10
Branch: `oskar/cli-32-desktop-management`
Extends: [`prd-cli-32-local-desktop-management.md`](prd-cli-32-local-desktop-management.md), [`prd-cli-32-desktop-remote-connections.md`](prd-cli-32-desktop-remote-connections.md)

> Implementation notes (relate endpoint paths, JWT auth, salt resolution) live in [`cli-32-local-desktop-management-impl.md`](cli-32-local-desktop-management-impl.md). This PRD touches only data-dir discovery and one new endpoint (`GET /fastify/api/info/app`).

## Overview

Replace the per-OS hardcoded fallback in `desktopclient.ResolveDataDir` with a query to Desktop's `GET /fastify/api/info/app` endpoint, which Desktop 2 now exposes unauthenticated and which returns the canonical `dataPath` (where `relate.secret.key` lives) along with `version`, `appPath`, `logsPath`, `cachePath`, `configPath`. The endpoint is asked once per CLI invocation, right after the existing port probe. The env-JSON and per-OS-default branches stay as fallbacks for older Desktop builds.

## Goals

- Stop guessing where Neo4j Desktop 2 keeps `relate.secret.key`. Ask Desktop.
- Make the CLI work against any Desktop 2 install (current and future), regardless of where Desktop chose to put its data dir — including the `detectExistingDataPath` legacy migration paths in `packages/electron/src/api/relate-env-setup.ts` that the hardcoded macOS default (`~/Library/Application Support/neo4j-desktop/Application/Data`) silently misses today.
- Surface useful Desktop diagnostics (`version`, `appPath`, `dataPath`) from the same `/info/app` reply inside `desktop doctor` so triage doesn't require digging into config files.
- Keep all existing override surfaces (`NEO4J_DESKTOP_DATA_PATH`, env-JSON `relateDataPath`) working so power-users and older Desktops are unaffected.

## Non-Goals

- Cross-invocation caching of `dataPath`. Re-fetched every invocation; same cadence as the port probe.
- Removing the env-JSON `relateDataPath` branch. Older Desktop builds (where `/info/app` still 401s) keep working.
- Removing the per-OS default. Kept as a best-effort last-resort.
- Touching the Desktop side. `/info/app` is already unauthenticated in current builds; this PRD does not include any Desktop-repo work. The min Desktop version that ships unauth `/info/app` is captured as an Open Question; the Desktop team owns it.
- Adding any new user-facing flag. The discovery change is transparent.
- New error UX for the "Desktop too old, no env-JSON, OS-default missing" worst case. The salt-not-found error stays as-is; no version-upgrade hint added in this round.
- Changing `LoadSalt`, the JWT signing path, the `httpDoFn` seam, or the wire types for any other Desktop endpoint.

## Requirements

### Functional Requirements

- REQ-F-201: A new exported function `desktopclient.FetchAppInfo(ctx context.Context, probe ProbeResult) (AppInfo, error)` issues an UNAUTHENTICATED `GET <probe.Origin>/fastify/api/info/app`, JSON-decodes the reply, and returns the parsed struct. No `X-Client-Id` / `X-API-Token` headers are sent. The 90-second `requestTimeout` constant is reused; same body cap (`maxResponseBodyBytes`) as authenticated calls; transport-failure mapping mirrors `Client.doRaw` (deadline-exceeded → canonical timeout error, everything else → canonical unreachable error).
- REQ-F-202: `AppInfo` wire type covers all fields Desktop returns:
  ```go
  type AppInfo struct {
      Platform   string `json:"platform"`
      Version    string `json:"version"`
      AppPath    string `json:"appPath"`
      LogsPath   string `json:"logsPath"`
      DataPath   string `json:"dataPath"`
      CachePath  string `json:"cachePath"`
      ConfigPath string `json:"configPath"`
  }
  ```
  Unknown future fields are ignored (default JSON decoder behaviour).
- REQ-F-203: `ResolveDataDir` signature changes from `(ctx, fs)` to `(ctx, fs, probe)` so the function can call `/info/app`. Callers that already ran `ProbePort` thread the result through. The new precedence is:
  1. `NEO4J_DESKTOP_DATA_PATH` env var (existing behaviour: joined with `Application/Data`).
  2. `FetchAppInfo(ctx, probe)` → `AppInfo.DataPath`. Empty string ⇒ treat as "endpoint returned a degenerate reply, fall through".
  3. `LoadEnvs(fs)` → active env JSON `relateDataPath` (existing behaviour).
  4. `defaultDataDir()` per-OS hardcoded path (existing behaviour, kept as last-resort).
- REQ-F-204: Any non-200 from `/info/app` (401 — older Desktop with the auth-gated endpoint, 5xx, timeout, transport error, decode error, empty `dataPath`) falls through to step 3 without surfacing the error to the user. The CLI must not error when `/info/app` is unavailable on an older Desktop that the env-JSON or OS default still covers.
- REQ-F-205: Discovery only consults `/info/app` when a `ProbeResult` is supplied. Callers that do not run a port probe (none in the current desktop tree, but the seam is preserved for tests) pass `ProbeResult{}` and skip step 2 entirely — the function falls through to step 3 immediately.
- REQ-F-206: `desktopclient.ProbePort` is unchanged. Discovery order is `ProbePort` → `ResolveDataDir(..., probe)` → `LoadSalt(fs, dataDir)` → `NewClient(probe, salt)`. All desktop leaves (`list`, `dbms create|delete|start|stop`, `connection create|update|delete`, `doctor`, plugin subtree) follow this order via `desktop/helpers.go`; the wiring change is mechanical because `ProbePort` already runs first.
- REQ-F-207: `desktop doctor` calls `FetchAppInfo` once and prints `version`, `appPath`, `dataPath` inline with the existing checks (NOT a separate `Desktop app:` block; squeezed into the current step list so the doctor output keeps its shape). Order: `Port probe → /info/app probe (version, appPath, dataPath) → Data dir resolution → Salt load → API auth check → DBMS list`. When `/info/app` fails or returns 401, doctor prints `/info/app: unavailable (older Desktop)` and continues; the subsequent steps still run.
- REQ-F-208: No new user-facing flag, no env var rename, no breaking change to any existing flag default. The `--port` flag still pins the port for both the probe AND the subsequent `/info/app` GET (same `probe.Origin`).
- REQ-F-209: Bundle regen — if any `Long`/`Example` text on a desktop leaf shifts as part of doctor's new line, `go generate ./neo4j-cli/internal/skill/...` is run and `TestGenerator_RoundTrip` is the gate. If the only change is in `doctor.go`'s `RunE` body (not `Long`/`Example`), no regen is needed.
- REQ-F-210: Changelog — single changie entry: `changie new --projects neo4j-cli --kind Minor --body "desktop: discover relate data dir via /info/app instead of OS-specific hardcoded paths"`.

### Non-Functional Requirements

- REQ-NF-201: Hermetic tests only. The new `FetchAppInfo` is exercised via the existing `httpDoFn` seam pattern (same seam used by `client_test.go`). `discovery_test.go` adds table-driven cases for the four precedence branches without touching the real network or filesystem (afero memFs for env-JSON fallback; mock `httpDoFn` for `/info/app`).
- REQ-NF-202: Body-size cap. `FetchAppInfo` shares the same `maxResponseBodyBytes` defence-in-depth cap as authenticated calls — a rogue process that wins the port race cannot exhaust CLI memory through `/info/app`.
- REQ-NF-203: No goroutine leak. `FetchAppInfo` uses `context.WithTimeout` and a single `defer cancel()` — same pattern as `Client.doRaw`.
- REQ-NF-204: Windows + Linux CI paths exercise the same fallback ladder. Existing `goosFn` / `homeDirFn` seams cover the OS-default branch.
- REQ-NF-205: `make fmt-check`, `make lint`, `make license-check`, `make test` all green. CI gate is unchanged.

## Technical Considerations

**Endpoint contract (source of truth):** `packages/electron/src/api/electron.routes.ts:47-59` defines the route, response schema in `packages/electron/src/api/electron.routes.ts:36-46`. Verified 200 OK unauthenticated against a running dev build returning all seven fields.

**Why `/info/app` is unauthenticated:** the apiTokenMiddleware (`packages/web/src/fastify/auth/api-token.middleware.ts:8-30`) currently exempts only `/fastify/api-docs`. The Desktop maintainer landed a route-level exemption for `/info/app` so a fresh CLI process — which by definition can't yet produce a valid JWT (it needs the salt, which lives in the dir we're discovering) — can bootstrap. Confirmed empirically; the route-level wiring on the Desktop side is the maintainer's commitment, not this PRD's.

**Backwards compat path:** older Desktop builds will continue to 401 the request. Step 4 of the precedence ladder handles that — env-JSON survives, OS-default survives. No user-visible regression.

**Doctor output line ordering:** the new `/info/app` step lands between the port probe and the data-dir resolution because (a) it needs the probed port, (b) its output (`dataPath`) is what step 3 in the ladder consumes. When the step succeeds, the subsequent "Data dir:" line will mirror its `dataPath` value — that's expected and informative.

**Race with `ProbePort`:** the port probe and `/info/app` are sequential; `ProbePort` returns first, `FetchAppInfo` uses its `Origin`. Probe's only job remains identifying which port answers; it does NOT verify `/info/app` itself (probing both would double the cold-start latency for no gain — `/info/app` runs on the chosen port for the actual data-dir resolution anyway).

**Test seam:** the test seam in `discovery_test.go` overrides `httpDoFn` for `/info/app` mocks. The existing `e2e_desktop_seams` build tag with `e2eDataDirOverride` is unchanged — `NEO4J_CLI_DESKTOP_E2E_DATA_DIR` still short-circuits the whole ladder when set.

**Concurrent calls:** `desktop list` already runs DBMS listing and connection listing in parallel (REQ-F-104). `FetchAppInfo` runs once before either of those, single-shot. No concurrency required for the new endpoint.

## Acceptance Criteria

- [ ] `desktopclient.FetchAppInfo(ctx, probe)` exists, issues unauth GET `<probe.Origin>/fastify/api/info/app`, decodes `AppInfo`, maps transport errors to canonical timeout/unreachable.
- [ ] `desktopclient.AppInfo` struct exposes all seven fields Desktop returns.
- [ ] `desktopclient.ResolveDataDir(ctx, fs, probe)` applies the four-step precedence in REQ-F-203 with the fallthrough semantics in REQ-F-204.
- [ ] `desktop/helpers.go` (and any other call site discovered during implementation) threads `ProbeResult` into `ResolveDataDir`.
- [ ] `discovery_test.go` covers:
  - `/info/app` 200 + valid `dataPath` wins over env-JSON + OS default.
  - `/info/app` 401 falls back to env-JSON; succeeds when env-JSON has a `relateDataPath`.
  - `/info/app` 401 + no env-JSON falls back to OS default.
  - `/info/app` 5xx falls back as above (same path as 401).
  - `/info/app` transport error / decode error / empty `dataPath` falls back as above.
  - `NEO4J_DESKTOP_DATA_PATH` set ⇒ `/info/app` is not called at all.
- [ ] `desktop doctor` prints `/info/app` lines (version, appPath, dataPath) inline with existing steps when the endpoint succeeds, and prints `/info/app: unavailable (older Desktop)` when it doesn't. Other doctor checks continue regardless.
- [ ] `doctor_test.go` exercises both success and failure branches via the `httpDoFn` seam, with a golden expectation block.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check` all pass.
- [ ] `go generate ./neo4j-cli/internal/skill/...` produces no diff (or, if `Long`/`Example` text shifted, the regenerated bundle is committed and `TestGenerator_RoundTrip` is green).
- [ ] End-to-end smoke against a running Desktop 2:
  - With NO `NEO4J_DESKTOP_DATA_PATH` set and no relevant env-JSON, `bin/neo4j-cli desktop list` succeeds purely via `/info/app`-derived `dataPath`.
  - `bin/neo4j-cli desktop doctor` is green; the `/info/app` lines match Desktop's reported `dataPath`.
  - Temporarily moving `~/Library/Application Support/com.Neo4j.Relate/Config/environments/*.json` aside still leaves `desktop list` working — proves env-JSON is no longer load-bearing on a current Desktop.
- [ ] Changelog entry filed via `changie new --projects neo4j-cli --kind Minor --body "..."`.

## Out of Scope

- Refactoring `Client.doRaw` to share code with `FetchAppInfo`. The auth/non-auth split is small enough that mirroring the body-cap + error-mapping pattern is cheaper than a shared helper.
- Adding `--json` output to `desktop doctor`. Doctor is human-readable today; that shape is preserved.
- Caching `AppInfo` to disk for cross-invocation reuse. The probe is cheap; cache invalidation is not.
- Removing the env-JSON fallback. Deferred until min Desktop version is pinned (Open Question).
- Surfacing `cachePath` / `configPath` / `logsPath` in doctor output. We pick the three fields actually useful for triage; the others would be noise.
- Desktop-side route-level `requiresAPIToken:false` work. Already shipped (verified); not part of this PRD.

## Open Questions

- Minimum Desktop version that ships unauth `/info/app` — TBD before merge. Whatever it is goes into AGENTS.md "Desktop Subsystem Notes" alongside the existing port-probe range. (Doesn't change the code; only documentation.)
- Whether to ALSO record `appPath` / `version` in the structured `desktop list --format json` output (not just doctor). Punted — separate scope, separate changelog entry, easy to add later if asked.
