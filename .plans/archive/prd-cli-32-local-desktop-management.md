# PRD: Local Neo4j Desktop management (CLI-32)

Linear: https://linear.app/neo4j/issue/CLI-32/local-neo4j-desktop-management-v10
Branch: `cli-32-local-neo4j-desktop-management-v10`

> **Implementation specifics** (relate API endpoint paths, authentication scheme, key derivation, salt file location, credential key namespace, source-file references in `neo4j-desktop-2`) live in [`cli-32-local-desktop-management-impl.md`](cli-32-local-desktop-management-impl.md), kept out of the PRD so the PRD stays focused on user-facing behaviour.

## Overview

Add a `neo4j-cli desktop` subcommand tree that manages Neo4j DBMSes running under Neo4j Desktop 2 on the user's machine — list / create / delete / start / stop, plus `desktop install` to install the Desktop app itself when it's missing. The CLI integrates against Desktop's existing local authenticated management API rather than duplicating Desktop's filesystem layout or process management.

## Goals

- Give users one terminal command to inspect and lifecycle their local Desktop-managed DBMSes, matching the ergonomics of `neo4j-cli aura instance ...` where it makes sense.
- Avoid shadowing Desktop's storage. We do not write DBMS data ourselves, and we do not duplicate Desktop's credential storage.
- Surface relate environment context so users understand why DBMSes from a dormant env are invisible to the running Desktop.
- Let users install Neo4j Desktop 2 from the CLI when it isn't present, so the rest of the tree works out of the box.
- Make `neo4j-cli query` "just work" against any Desktop-managed DBMS (including GUI-created ones) by reading credentials from Desktop on demand.
- Manage Neo4j plugins (APOC, GDS, neo-semantics, …) on local Desktop DBMSes — install, uninstall, list installed/available — without dropping to the Desktop GUI. Auto-restart the DBMS on plugin changes when the DBMS is currently running so the plugin takes effect immediately.
- Cleanly factor the `desktop` subcommand tree as DBMSes (`desktop dbms ...`) and remote saved connections (`desktop connection ...`) now coexist as first-class resources, while keeping a composed `desktop list` view that shows both together.

## Non-Goals

- Re-implementing Neo4j process management or DBMS storage outside Desktop.
- Caching the relate port, salt, or token between invocations. Re-discover each call.
- Switching relate environments at runtime — relate has no such endpoint; switching requires `NEO4J_DESKTOP_ENV=<name>` plus a Desktop restart.
- **Duplicating Desktop's credential storage in neo4j-cli's `credentials.json`.** Desktop's OS-keychain-backed store, exposed via its authenticated local API, is the source of truth; we read on demand. No auto-register on `desktop create`, no prune on `desktop delete`.
- Launching Desktop GUI automatically (`open -a`) when it's not running. Fail fast with a hint instead.
- Headless / daemon mode for the relate API. Out of our control — Desktop's HTTP server runs inside the Electron main process.
- Backups, projects, connection-string management, schema introspection. All deferred. (Plugin management is in scope per REQ-F-035..040.)

## Requirements

### Functional Requirements

- REQ-F-001: `neo4j-cli desktop list` — lists DBMSes via Desktop's local API. Default table columns: `id, name, version, status, connectionUri`. `--format json|table|toon` honoured. Empty result renders an empty table (no hint).
- REQ-F-002: `neo4j-cli desktop create --name <n> [--version <v>] --password <p> [--wait]` — creates a DBMS via Desktop's local API, then starts it. By default returns as soon as Desktop's start call resolves; `--wait` blocks while the CLI polls every 1s for up to 30s for `status=started`. `--wait` matches the opt-in convention used by sibling lifecycle leaves (`dbms start --wait`, `dbms stop --wait`) and AGENTS.md "Flag conventions" ("async operations expose `--wait`"). `--version` is OPTIONAL; when omitted the CLI auto-selects the latest stable enterprise version per REQ-F-030. No `--edition` flag: Desktop 2 ships enterprise-only, and the create request MUST omit the `edition` field entirely (let Desktop apply its default). Write command → `Annotations{"write":"true"}`, requires `--rw`.
- REQ-F-003: `neo4j-cli desktop delete <id> [--yes]` — deletes via Desktop's local API. On a TTY without `--yes`, prompt `Delete DBMS '<name>' (<id>)? [y/N]`. Non-TTY without `--yes` errors out. Write, requires `--rw`.
- REQ-F-004: `neo4j-cli desktop start <id> [--wait]` — starts via Desktop's local API. `--wait` polls every 1s until `status=online` (or 30s timeout, exit code 1 with last-seen status). Write, requires `--rw`.
- REQ-F-005: `neo4j-cli desktop stop <id> [--wait]` — stops via Desktop's local API. `--wait` polls until `status=offline` (30s timeout). Write, requires `--rw`.
- REQ-F-007: `neo4j-cli desktop install` — install Neo4j Desktop 2 from the electron-builder publish feed. Already-installed detection (REQ-F-016) runs BEFORE any network call. On a clean system: fetch the per-OS YAML manifest (REQ-F-015), pick the platform artifact, download to a tempfile, verify base64 SHA-512 against the manifest entry, then dispatch to the per-OS install action (REQ-F-017–019). Write, requires `--rw`. Flags: `--force`, `--target-dir <path>`, `--dry-run` (REQ-F-020).
- REQ-F-008: **Unreachable Desktop API** — the dominant failure mode in practice is "user hasn't started Desktop". All paths converge on a single canonical message. Apply on:
  - **Probe-time** (REQ-F-009 walks the documented port range and nothing answers).
  - **Mid-request** (probe succeeded but the actual HTTP call returns a connection-refused / connection-reset / EOF; user Cmd+Q'd Desktop between probe and call).
  - **Request timeout** (REQ-F-023).

  All three surface `clierr.NewFatalError` with: `"Neo4j Desktop 2 doesn't appear to be running. Start Desktop (Cmd+Space → 'Neo4j Desktop 2') or pass --port if it's on a non-default port."` When the already-installed detection (REQ-F-016) finds nothing, append a second sentence: `"If you don't have Desktop installed yet, run 'neo4j-cli desktop install'."`

  Probe MUST validate an actual Desktop response — a raw TCP-open on the port doesn't count as "Desktop is here", because an unrelated service could be listening.
- REQ-F-009: `--port` flag on every leaf that calls the Desktop API. Skips probe; uses the supplied port verbatim.
- REQ-F-010: Auth on every Desktop API request uses Desktop's existing token scheme. On 401, surface a friendly hint: `"Auth failed against Neo4j Desktop 2 local API. The stored token state may be stale or out of sync — restart Neo4j Desktop 2 to regenerate."` (no silent retry).
- REQ-F-011: Data-dir resolution order: (1) `NEO4J_DESKTOP_DATA_PATH` env var; (2) walk relate env metadata, pick `active: true` (or the one named by `NEO4J_DESKTOP_ENV`); (3) per-OS Desktop-2 default. Resolution helper lives in `discovery.go`, used by auth + data-dir lookup.
- REQ-F-012: Cross-platform path handling. macOS, Linux, and Windows all supported in v1. Use `os.UserConfigDir()` / `os.UserHomeDir()` where possible.
- REQ-F-013: Skill bundle regeneration — `go generate ./neo4j-cli/internal/skill/...` runs after any cobra-tree change. `TestGenerator_RoundTrip` is the gate.
- REQ-F-014: Every leaf has a flush-left `Example:` field with ≥3 invocations, `# comment` lines, `--rw` on writes, at least one `--format json` on reads. Enforced by `TestAllLeafCommands_HaveExamples`.
- REQ-F-015: Manifest fetch + verify. Per OS:
  - macOS: `https://dist.neo4j.org/neo4j-desktop-2/mac/latest-mac.yml`
  - Linux: `https://dist.neo4j.org/neo4j-desktop-2/linux/latest-linux.yml`
  - Windows: `https://dist.neo4j.org/neo4j-desktop-2/win/latest.yml`
  Parse the YAML (`version`, `files`, `path`, top-level `sha512`, `releaseDate`). Resolve asset URLs against the manifest URL's directory. Hash the downloaded artifact with `crypto/sha512` and compare against the base64-decoded `sha512` from the chosen `files[]` entry; mismatch → `clierr.NewFatalError`, delete the tempfile.
- REQ-F-016: Already-installed detection — run before any HTTP call. macOS: stat `/Applications/Neo4j Desktop 2.app` or `~/Applications/Neo4j Desktop 2.app`, read `Contents/Info.plist` for `CFBundleShortVersionString`. Linux: glob `~/Applications/neo4j-desktop-*.AppImage` and extract the version from the filename. Windows: stat `%LOCALAPPDATA%\Programs\neo4j-desktop\` and, when available, read `DisplayVersion` from `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\<key>`. On hit: print `Neo4j Desktop 2 already installed at <path> (version <X>). Pass --force to re-install.` to stdout, exit 0. `--force` skips the detection.
- REQ-F-017: macOS install action — choose the universal `.dmg` file from `files[]` (NOT the `.zip`, which is Squirrel.Mac auto-update format). Run `hdiutil attach -nobrowse -noverify -noautoopen <dmg>`, `cp -R "/Volumes/<mountpoint>/Neo4j Desktop 2.app" <target>/`, `hdiutil detach "/Volumes/<mountpoint>" -quiet`. Default `<target>` is `/Applications`; on EACCES fall back to `~/Applications`. Notarized → Gatekeeper requires no extra steps.
- REQ-F-018: Linux install action — download the x86_64 AppImage to `~/Applications/` (create the dir if missing), `chmod +x`. Write a `.desktop` entry at `~/.local/share/applications/neo4j-desktop-2.desktop` with `Exec=<absolute path to .AppImage>` and `Name=Neo4j Desktop 2` so launchers pick it up. arm64 Linux: no upstream build — return `clierr.NewFatalError("Neo4j Desktop 2 does not publish an arm64 Linux build; visit https://neo4j.com/deployment-center/?desktop-gdb")`. Do NOT attempt emulation.
- REQ-F-019: Windows install action — download the NSIS `.exe`, run with `/S` for silent install. Default target `%LOCALAPPDATA%\Programs\neo4j-desktop\`. Authenticode signed by Neo4j; no extra trust steps.
- REQ-F-020: Install flags: `--force` (re-install regardless of REQ-F-016), `--target-dir <path>` (override the per-OS default — applies after dir resolution, before extract), `--dry-run` (fetch manifest, resolve URL, print what would happen, exit 0 — no download, no install). `--rw` required.
- REQ-F-021: `desktop install` does NOT prompt for license acceptance. macOS DMG and Windows NSIS silent install (`/S`) both proceed without a EULA dialog; we match that. Print a single post-install line: `Installed Neo4j Desktop 2 (version <X>) at <path>.` and exit 0.
- REQ-F-022: `desktop install` does NOT auto-launch Desktop after a successful install. Print a short next-step hint to stderr: `Run Neo4j Desktop 2 (Cmd+Space → 'Neo4j Desktop 2') to start using it.`
- REQ-F-023: HTTP request timeout for Desktop API calls: 30s flat, applied via `context.WithTimeout` on the `cmd.Context()` passed into the HTTP client. On timeout: treat as the unreachable case (REQ-F-008) — same message. The probe-time timeout is separate and unchanged.
- REQ-F-024: HTTP 5xx from Desktop (rare — implies Desktop's own state is corrupt): surface `clierr.NewFatalError` with `"Neo4j Desktop 2 local API returned <status>. The response body was: <truncated body, 200 chars max>. Try restarting Desktop; if the error persists, file a bug."` Don't retry automatically.
- REQ-F-025: **Desktop is the credential source of truth.** neo4j-cli reads DBMS credentials from Desktop's existing OS-keychain-backed store via Desktop's authenticated local API. neo4j-cli MUST NOT write Desktop DBMS credentials into its own `credentials.json` — no auto-register on `desktop create`, no prune on `desktop delete`. Desktop owns the lifecycle. (Endpoint paths, key namespace, and source-file references are in [`cli-32-local-desktop-management-impl.md`](cli-32-local-desktop-management-impl.md).)
- REQ-F-026: **`neo4j-cli query` falls through to Desktop.** Extend `resolveConn` (`neo4j-cli/query/connect.go:103-265`) with a Desktop fall-through that fires AFTER the persisted-credential lookup but BEFORE built-in defaults. When `--credential <name>` is passed and no persisted entry matches: locate the running Desktop instance, find the DBMS by `name` or `id`, fetch its credentials from Desktop, and synthesise an in-memory conn using the returned connection URI. Same `<name>` matching a persisted credential AND a Desktop DBMS: persisted wins (explicit user intent).
- REQ-F-027: **Zero-config auto-select.** When no `--credential` flag was passed AND no persisted default credential exists AND Desktop is reachable AND exactly ONE DBMS exists in the active env: auto-use that DBMS. Print a one-time stderr breadcrumb: `"Using Desktop DBMS '<name>' (<id>). Pass --credential to override."` More than one DBMS or zero DBMSes → fall through to existing resolveConn behaviour (built-in defaults / prompt).
- REQ-F-028: **No-stored-credentials fallback.** When Desktop has the DBMS but no credentials stored for it (a verified-live edge case for legacy DBMSes), neo4j-cli MUST NOT treat it as a generic error. Fallback chain: (1) one more try at persisted `Credentials.Dbms.Get(<name>)`; (2) on TTY, prompt for password with `neo4j` as default username; (3) non-TTY fatal: `"Neo4j Desktop 2 has no stored credentials for DBMS '<name>' (<id>). Pass --password (and optionally --username) explicitly, or run 'credential dbms add' to register a connection, or open Desktop and use 'Reset password' on this DBMS."`
- REQ-F-029: **Miss-everywhere error UX.** When `--credential <name>` matches neither persisted nor Desktop AND Desktop probe fails: list BOTH fallbacks in one error — `"No persisted credential <name>. Could not reach Desktop on the expected local port (or DBMS not found in the active env). Run 'credential dbms add' to register a connection, or start Neo4j Desktop 2."`
- REQ-F-030: **Default-to-latest version on `desktop create`.** When `--version` is omitted: call `GET /dbmss/versions` on the Desktop relate API, filter to entries where `edition == "enterprise"` AND the version is a stable release (no pre-release qualifiers like `-alpha`, `-beta`, `-rc`), then pick the highest-semver entry across BOTH the legacy `5.x.y` and new calendar `YYYY.MM.0` series (the calendar series outranks 5.x lexically and semantically, so a plain semver compare yields the right answer). Prefer cached over online when versions tie (`origin == "cached"` means no download wait; ties are unlikely with distinct versions but defensive). Emit a stderr breadcrumb: `Using Neo4j enterprise <picked-version> (cached|online)`. If the versions endpoint returns an empty list or only pre-releases, fall through to a fatal error pointing the user at `--version <vX>`. `--version` when supplied is honoured verbatim with no auto-selection. See [`cli-32-local-desktop-management-impl.md`](cli-32-local-desktop-management-impl.md) "Versions endpoint" for the wire shape and observed sorting behaviour.

#### Subcommand restructure (DBMS / connection split)

- REQ-F-031: **Introduce `desktop dbms` and reorganise lifecycle leaves.** All existing DBMS lifecycle leaves move from `desktop <action>` to `desktop dbms <action>` (hard rename, no aliases — the surface has not shipped a stable release):
  - `desktop create` → `desktop dbms create`
  - `desktop delete <id>` → `desktop dbms delete <id>`
  - `desktop start <id>` → `desktop dbms start <id>`
  - `desktop stop <id>` → `desktop dbms stop <id>`

  Skill bundle (`go generate ./neo4j-cli/internal/skill/...`) and `agent-context` regenerate in the same task. Existing tests update to call the new paths.
- REQ-F-032: **`desktop dbms list` leaf.** New read-only leaf that renders ONLY DBMSes (no connections section). Uses the existing enrichment fan-out (`ListDbmss` + bounded `GetDbms` fan-out, REQ-F-001 dormant-env hint logic) — the existing implementation of `desktop list`'s DBMS-section logic moves here unchanged. Default table columns: `id, name, version, status, connectionUri`. `--format json|table|toon` honoured. Read-only.
- REQ-F-033: **`desktop list` retained as the composed view.** `desktop list` continues to render BOTH `Local DBMSes` AND `Remote connections` in one two-section view (current behaviour from CLI-32 desktop-remote-connections work). It is the only leaf that emits both sections. `desktop dbms list` / `desktop connection list` are the single-resource alternatives. Skill bundle Long text and Example updated to call this out.
- REQ-F-034: **`desktop connection list` leaf.** New read-only leaf that renders ONLY saved remote connections (no DBMS section). Default table columns: `id, name, connectionUri`. `--format json|table|toon` honoured. Talks to `GET /fastify/api/connections`. Read-only.

#### Plugin management (`desktop dbms plugin ...`)

- REQ-F-035: **`desktop dbms plugin list <dbms-id>` leaf.** Read-only. Hits `GET /fastify/api/dbmss/:dbmsId/plugins/installed`. Renders the installed-plugin list. Default table columns: `name, version, pendingRestart, filePath`. `--format json|table|toon`. Resolves `<dbms-id>` against the DBMS list (`name` OR `id` accepted, mirroring `desktop dbms start/stop/delete`). Empty result is a clean empty table with no error.
- REQ-F-036: **`desktop dbms plugin available <dbms-id>` leaf.** Read-only. Hits `GET /fastify/api/dbmss/:dbmsId/plugins/available`. Renders the installable-plugin catalog (scans the DBMS's `products/` and `labs/` directories on the server side). Same default columns as REQ-F-035. Resolves `<dbms-id>` the same way. Useful to discover plugin names before `install`.
- REQ-F-037: **`desktop dbms plugin install <dbms-id> <plugin>` leaf.** Write, requires `--rw`. Hits `POST /fastify/api/dbmss/:dbmsId/plugins/install` with body `{"pluginName": "<plugin>"}`. The `<plugin>` positional is passed verbatim — relate's server-side `install()` (see `dbms-plugins.local.ts:91-105` in `../neo4j-desktop-2`) accepts EITHER a known plugin name (e.g., `apoc`, `gds`, present in `products/` or `labs/`) OR a filesystem path to a local `.jar`. CLI does no dispatch — relate decides. Renders the returned `DbmsPlugin` (`name, version, filePath, pendingRestart`). Auto-restart per REQ-F-039.
- REQ-F-038: **`desktop dbms plugin uninstall <dbms-id> <plugin>` leaf.** Write, requires `--rw`. Hits `POST /fastify/api/dbmss/:dbmsId/plugins/uninstall` with body `{"pluginName": "<plugin>"}`. Server returns `{"name": "<plugin>"}` even when the plugin wasn't installed (idempotent — relate still rolls back any installed config). CLI renders `{name: <plugin>, uninstalled: true}` for table/json/toon consistency with `desktop dbms delete`'s `{id, name, deleted: true}` shape. Auto-restart per REQ-F-039.
- REQ-F-039: **Auto-restart on plugin install/uninstall when the DBMS is running.** When the target DBMS has `status == "started"` at the time of the plugin operation:
  1. After the plugin POST returns 2xx, emit stderr: `Plugin change pending — restarting DBMS '<name>' (<id>) to apply…`
  2. Issue `StopDbms` + poll-until-`stopped` (30s ceiling, reuse `pollUntilStatus`).
  3. Issue `StartDbms` + poll-until-`started` (30s ceiling).
  4. Emit stderr on success: `DBMS restarted; plugin '<plugin>' is now active.`

  When the DBMS is `stopped` at plugin-op time: skip the restart, emit stderr breadcrumb `DBMS '<name>' is not running; plugin '<plugin>' will be active on next start.` (No-op — the JAR is already on disk; the next manual `start` picks it up.)

  `--no-restart` flag on both `install` and `uninstall` opts out of the auto-restart entirely (skipping the whole orchestration). When set AND the DBMS is `started`, emit stderr hint: `Run 'neo4j-cli desktop dbms stop <id> && neo4j-cli desktop dbms start <id> --wait --rw' to apply the change.`

  If the stop or start fails, the plugin op itself already succeeded — surface a non-fatal stderr warning naming the DBMS + failure mode, exit 0. (Don't roll back the plugin change; that's beyond CLI scope and the user can inspect / restart manually.)

  No `--restart` flag (auto is the default for the running case; `--no-restart` is the escape hatch).
- REQ-F-040: **Extended timeout for plugin install/uninstall.** Apply 120s `context.WithTimeout` to the install/uninstall POST (vs 30s for DBMS lifecycle calls, REQ-F-023). Relate's install copies a JAR file and updates `neo4j.conf` — fast on already-cached plugins, but the request can stack with relate's queueing for a freshly-cached catalog. 120s leaves headroom; on timeout, route through REQ-F-008 canonical unreachable text.
- REQ-F-041: **Plugin-not-found error.** Relate returns `NotFoundError` (`HTTP 404`) when the plugin name doesn't match a known available plugin AND the value isn't a valid path. CLI canonical error: `Plugin '<plugin>' not found on DBMS '<dbms-id>'. Run 'neo4j-cli desktop dbms plugin available <dbms-id>' to see installable plugins, or pass a path to a local .jar file.` Exit 1.
- REQ-F-042: **DBMS-not-found error on plugin commands.** When `<dbms-id>` doesn't resolve via the local `ListDbmss` lookup OR relate returns 404 on the plugin endpoint: canonical error `DBMS '<dbms-id>' not found. Run 'neo4j-cli desktop dbms list' to see managed DBMSes.` Exit 1. Same error shape across all four plugin leaves.

#### Plugin testing (fixture + e2e)

- REQ-F-043: **Fixture extension.** `test/e2e/desktop_fixture/` MUST grow plugin endpoints — handlers for the four relate routes, in-memory state under `state.go` (per-DBMS `available` + `installed` plugin lists, `pendingRestart` flag toggled by install/uninstall when the DBMS is `started`), and scenario presets exposing single-DBMS-with-plugins and multi-DBMS-mixed-plugin-state fixtures via `scenarioPutDbms` / a new `scenarioPutPlugin` injector. Install must reject names not in the available list (returning 404 like relate). Uninstall must be idempotent (returns `{name}` even when not installed).
- REQ-F-044: **E2E suite extension.** `test/e2e/desktop/desktop_test.go` MUST add scenarios covering ALL four plugin leaves:
  - `list` happy path + empty + DBMS-not-found.
  - `available` happy path + empty.
  - `install` happy path (DBMS stopped — no restart) + happy path (DBMS started — auto-restart sequence asserted via request-order recording) + `--no-restart` opt-out + plugin-not-found error + DBMS-not-running error.
  - `uninstall` happy path + idempotent (already-uninstalled) + auto-restart + `--no-restart`.

  Restart simulation in the fixture: when a started DBMS is stopped + restarted, the fixture flips `pendingRestart: false` on all its installed plugins. The e2e suite asserts this state via a follow-up `list` call.
- REQ-F-045: **Skill bundle regen for restructure + plugins.** Single bundle regeneration task at the end (`go generate ./neo4j-cli/internal/skill/...`); `TestGenerator_RoundTrip` is the gate. The bundle's `references/` will see: new `desktop_dbms.md` + `desktop_dbms_list.md` + `desktop_dbms_create.md` + `desktop_dbms_delete.md` + `desktop_dbms_start.md` + `desktop_dbms_stop.md` + `desktop_dbms_plugin.md` + `desktop_dbms_plugin_list.md` + `desktop_dbms_plugin_available.md` + `desktop_dbms_plugin_install.md` + `desktop_dbms_plugin_uninstall.md` + `desktop_connection_list.md`; removal of the standalone `desktop_create.md` / `desktop_delete.md` / `desktop_start.md` / `desktop_stop.md` (renamed under dbms). `agent-context` reflects all four plugin leaves automatically (no edits to `agentcontext/build.go`).

### Non-Functional Requirements

- REQ-NF-001: All operations hermetic in tests — `httptest.NewServer` for the Desktop API, `afero.NewMemMapFs` / `testfs.GetTestFs` for the relate env metadata and salt file. No real Desktop, no real filesystem.
- REQ-NF-002: Final gates `make test && make fmt-check && make lint` must pass on Linux, macOS, and Windows CI matrix.
- REQ-NF-003: Port probe latency budget: ≤200ms total when Desktop is on the default port (single TCP connect to the probe endpoint). Worst case (probing the full range) ≤2s with 200ms per-probe timeout.
- REQ-NF-004: License header on every new `.go` file (CI gate via `addlicense`).
- REQ-NF-005: Changelog entry via `changie new --projects neo4j-cli --kind Minor` describing user-visible commands.
- REQ-NF-006: New dependency `github.com/golang-jwt/jwt/v5` added to `go.mod`.

## Technical Considerations

### Architecture

```
neo4j-cli/internal/subcommands/desktop/
  desktop.go               # parent: NewCmd(cfg); mounts dbms, connection, list, install
  list.go                  # COMPOSED: DBMSes + Remote connections in one render
  install.go               # cobra wiring + orchestration: detect → fetch manifest → verify → dispatch
  installer_mac.go         # REQ-F-017: hdiutil attach / cp -R / hdiutil detach
  installer_linux.go       # REQ-F-018: AppImage + chmod + .desktop entry; arm64 hard-error
  installer_win.go         # REQ-F-019: NSIS .exe /S
  dbms/
    dbms.go                # parent
    list.go                # REQ-F-032: DBMS-only list (enriched fan-out, dormant-env hint)
    create.go              # was desktop/create.go (REQ-F-031 rename)
    delete.go              # was desktop/delete.go
    start.go               # was desktop/start.go
    stop.go                # was desktop/stop.go
    plugin/
      plugin.go            # parent for plugin subtree
      list.go              # REQ-F-035: GET /dbmss/:id/plugins/installed
      available.go         # REQ-F-036: GET /dbmss/:id/plugins/available
      install.go           # REQ-F-037 + auto-restart
      uninstall.go         # REQ-F-038 + auto-restart
      helpers.go           # autoRestartIfRunning + plugin-id resolution
  connection/
    connection.go          # parent (existing)
    list.go                # REQ-F-034: connection-only list (new)
    create.go              # existing
    update.go              # existing
    delete.go              # existing
  *_test.go                # colocated; one per leaf + helpers_test.go
```

Shared Desktop API client, imported by BOTH the `desktop` subcommand tree AND `neo4j-cli/query/connect.go` (for the REQ-F-026 fall-through):

```
neo4j-cli/internal/desktopclient/
  client.go          # auth + HTTP transport; methods for ListDbmss, GetDbms,
                     # CreateDbms, DeleteDbms, StartDbms, StopDbms, GetCredentialsByKey,
                     # ListConnections, CreateConnection, UpdateConnection, DeleteConnection,
                     # ListInstalledPlugins, ListAvailablePlugins, InstallPlugin, UninstallPlugin
  discovery.go       # port probe + active-env / data-dir resolution
  envconfig.go       # parse env metadata off disk for active-env / data-dir resolution
  types.go           # response structs mirroring Desktop's schemas (incl. DbmsPlugin)
  *_test.go
```

No reverse dependency — `desktop` and `query` both depend on `desktopclient`; `desktopclient` depends on nothing in either.

Single mount line in `neo4j-cli/app/app.go` next to existing internal-subcommand mounts. `agentcontext/build.go` requires no edits — it walks the cobra tree at runtime.

### Lifecycle

Desktop's local management API is part of the Electron main process's lifetime. Killed on `before-quit`. No daemon mode. Therefore: API is live iff Desktop is running. We do not attempt to `open -a` the GUI when it's missing.

### Relate environments

The Desktop API operates on **one** environment at a time — calls always target the default env set at Desktop startup. Selection order (replicated by `desktopclient/discovery.go`):
1. `NEO4J_DESKTOP_DATA_PATH` env var (custom data path)
2. `NEO4J_DESKTOP_ENV=<name>` (selects an existing on-disk env)
3. Disk env named `Neo4j_Desktop_2`
4. Disk env named `Neo4j_Desktop` (v1 legacy)
5. Per-OS default

There is no API endpoint to list, create, or switch envs. Electron reads them off disk; the CLI does the same internally for data-dir + active-env resolution.

### `desktop install` — download channel (locked in)

electron-builder publish feed (publicly served, not sensitive):

| OS | Manifest URL |
|---|---|
| macOS | `https://dist.neo4j.org/neo4j-desktop-2/mac/latest-mac.yml` |
| Linux | `https://dist.neo4j.org/neo4j-desktop-2/linux/latest-linux.yml` |
| Windows | `https://dist.neo4j.org/neo4j-desktop-2/win/latest.yml` |

Each manifest is electron-builder YAML with `version`, `files: [{url, sha512, size}]`, `path`, top-level `sha512`, `releaseNotes`, `releaseDate`. Asset URLs resolve against the manifest URL's directory. macOS uses the `.dmg` (NOT the `.zip` — Squirrel.Mac auto-update vehicle). Linux uses the `.AppImage`. Windows uses the `.exe`. Verification = base64-decode the `sha512` from the chosen `files[]` entry → compare against `crypto/sha512` of the tempfile.

Binaries are codesigned/notarized by Neo4j (macOS DMG, Windows Authenticode). AppImage is unsigned (Linux norm). No GPG/PGP files alongside; the manifest hash IS the verification.

Out of scope (follow-ups):
- `--channel canary` (canary publish feed exists)
- `--version <vX>` — manifest only exposes `latest`
- Linux `.deb` / `.rpm` — not published
- arm64 Linux — no upstream build

### Credential surface — Desktop owns it

Desktop's `credentialsManager` writes `{username, password}` per DBMS into an Electron `safeStorage`-encrypted store (OS-keychain-keyed) at every install. Desktop exposes a CRUD HTTP surface for those credentials on the same authenticated local API neo4j-cli already uses for DBMS CRUD. Endpoint paths, key namespace, and source-file references are in [`cli-32-local-desktop-management-impl.md`](cli-32-local-desktop-management-impl.md).

`null` is a documented return value for legacy DBMSes that pre-date `storePasswords` or for environments where `safeStorage` isn't available. REQ-F-028 covers that case.

### Reused existing utilities

- `common/clicfg` — config + afero filesystem
- `common/clierr` — `NewFatalError` / `NewNotFound` / `NewUsageError`
- `common/output.PrintBodyMap(cmd, cfg, data, fields)` — rendering for `--format json|table|toon`
- Cobra leaf template: `neo4j-cli/internal/subcommands/credential/dbms/list.go`
- TTY-prompt pattern: check `golang.org/x/term.IsTerminal(int(os.Stdin.Fd()))`; reuse existing prompt helpers if present, otherwise add a thin one.
- For `desktop install`, reuse patterns from `neo4j-cli/internal/subcommands/update/`:
  - `swap.go` — HTTP download → atomic-tempfile rename pattern. Adapt for SHA-512 base64 (manifest format) instead of SHA-256 hex.
  - Test seam pattern (package-level `var httpDoFn = ...`) for hermetic tests with `httptest.NewServer`.
  - Sudo-elevation logic from `swap.go` if `/Applications` ever isn't writable (rare; usually a per-user `~/Applications` fallback is enough).
  - `clierr` error types and exit-code conventions.
- NOT reused: GitHub Releases API lookup (`release.go` — Desktop publishes to `dist.neo4j.org`, not GitHub); the package-manager passthrough hint from `install_method.go` (irrelevant — we're installing a different app, not the running CLI).

## Acceptance Criteria

- [ ] `neo4j-cli desktop list` lists DBMSes from the running Desktop's active env on macOS, Linux, and Windows.
- [ ] `neo4j-cli desktop list` returns 0 DBMSes plus the env-hint on stderr when the active env is empty and other envs have DBMSes on disk.
- [ ] `neo4j-cli desktop create --name X --version 5 --password foobar1234 --rw` creates and starts a DBMS, blocking until `status=online`, then prints the rendered `DbmsInfo`.
- [ ] `neo4j-cli desktop create ...` returns immediately after the start call returns; `--wait` blocks until `status=started` (30s ceiling).
- [ ] `neo4j-cli desktop start <id> --rw --wait` blocks until online; 30s timeout exits non-zero with last status.
- [ ] `neo4j-cli desktop stop <id> --rw --wait` blocks until offline.
- [ ] `neo4j-cli desktop delete <id> --rw` prompts on TTY; `--yes` bypasses; non-TTY without `--yes` errors out.
- [ ] `neo4j-cli desktop install` does not prompt for license acceptance.
- [ ] `neo4j-cli desktop install` does not auto-launch Desktop on success; prints a stderr next-step hint instead.
- [ ] `neo4j-cli desktop install` installs Desktop 2 from `dist.neo4j.org` on macOS, Linux, and Windows; SHA-512 verified.
- [ ] `neo4j-cli desktop install` is idempotent — re-run on an installed system prints `already installed at <path> (version <X>). Pass --force to re-install.` and exits 0.
- [ ] `neo4j-cli desktop install --force` re-downloads and reinstalls regardless of detection.
- [ ] `neo4j-cli desktop install --dry-run` resolves the manifest + artifact URL, prints them, and exits without downloading.
- [ ] `neo4j-cli desktop install` on `runtime.GOOS == "linux" && runtime.GOARCH == "arm64"` errors with the deployment-center URL — never attempts emulation.
- [ ] `neo4j-cli desktop install` rejects an artifact whose computed `crypto/sha512` doesn't match the manifest's base64 `sha512`.
- [ ] When Desktop isn't running, every API-backed leaf fails fast with the canonical error referencing the `--port` escape hatch (REQ-F-008).
- [ ] Desktop quit mid-request produces the SAME canonical error as probe-time unreachable.
- [ ] When Desktop isn't installed at all, the canonical error appends the "run 'neo4j-cli desktop install'" sentence.
- [ ] A 30s HTTP request timeout fires on hung calls and is treated as unreachable (REQ-F-023).
- [ ] 401 from Desktop surfaces the restart hint (REQ-F-010), not the raw error.
- [ ] 5xx from Desktop surfaces the truncated body message (REQ-F-024).
- [ ] Port probe accepts only an actual Desktop response — a raw TCP-open on the port doesn't count.
- [ ] `neo4j-cli desktop create` does NOT write to `~/.neo4j/cli/credentials.json` (REQ-F-025).
- [ ] `neo4j-cli desktop delete` does NOT mutate `~/.neo4j/cli/credentials.json` (REQ-F-025).
- [ ] `neo4j-cli query --credential <dbms-name>` works against a Desktop DBMS that was never registered via `credential dbms add` (REQ-F-026), pulling creds from Desktop on the fly.
- [ ] With exactly one Desktop DBMS running and no persisted default, `neo4j-cli query` auto-selects it and prints the one-time stderr breadcrumb (REQ-F-027).
- [ ] When Desktop has the DBMS but no stored credentials, neo4j-cli prompts on TTY / errors with the "Reset password" hint on non-TTY (REQ-F-028).
- [ ] `--credential <name>` matching neither persisted nor Desktop (Desktop unreachable) produces the both-fallbacks error (REQ-F-029).
- [ ] `neo4j-cli query --credential <dbms-name>` works for a Desktop GUI-created DBMS the CLI has never seen before (zero pre-registration).
- [ ] `--port N` on any leaf skips the probe and hits localhost:N.
- [ ] All commands honour `--format json|table|toon`; JSON output includes all `DbmsInfo` fields.
- [ ] Skill bundle regenerated; `TestGenerator_RoundTrip` and `TestAllLeafCommands_HaveExamples` pass.
- [ ] `make test && make fmt-check && make lint` green on Linux/macOS/Windows CI.
- [ ] Changelog entry merged via `changie` describing the new commands.
- [ ] End-to-end smoke (manual, local): list → create → start → query via cypher-shell → stop → delete → list.

### Acceptance criteria — subcommand restructure (REQ-F-031..034)

- [ ] `neo4j-cli desktop create/delete/start/stop` no longer exist; the equivalents under `desktop dbms` work identically.
- [ ] `neo4j-cli desktop dbms list` renders ONLY the DBMS section with `id, name, version, status, connectionUri` (enriched fan-out preserved).
- [ ] `neo4j-cli desktop connection list` renders ONLY the connection section with `id, name, connectionUri`.
- [ ] `neo4j-cli desktop list` continues to render BOTH sections (Local DBMSes + Remote connections); no behaviour change from the desktop-remote-connections work.
- [ ] Bundle `references/desktop_dbms*.md` + `desktop_connection_list.md` regenerated; standalone `desktop_create/delete/start/stop.md` removed.
- [ ] `agent-context` output reflects the new tree automatically.

### Acceptance criteria — plugin management (REQ-F-035..042)

- [ ] `desktop dbms plugin list <dbms-id>` renders installed plugins; empty DBMS yields an empty table (no error).
- [ ] `desktop dbms plugin available <dbms-id>` renders the installable catalog from `products/` + `labs/`.
- [ ] `desktop dbms plugin install <dbms-id> apoc --rw` succeeds against the fixture; renders the returned `DbmsPlugin`.
- [ ] `desktop dbms plugin install <dbms-id> /path/to/local.jar --rw` passes the path verbatim to the relate endpoint (no CLI-side dispatch).
- [ ] `desktop dbms plugin uninstall <dbms-id> apoc --rw` succeeds and renders `{name: apoc, uninstalled: true}`.
- [ ] `desktop dbms plugin uninstall <dbms-id> not-installed --rw` is idempotent (exit 0, same rendered shape).
- [ ] Plugin install/uninstall on a `started` DBMS auto-restarts: stop-poll → start-poll, with the two stderr breadcrumbs (`Plugin change pending` + `DBMS restarted`); the post-restart `list` shows `pendingRestart: false`.
- [ ] Plugin install/uninstall on a `stopped` DBMS skips the restart and emits the `will be active on next start` breadcrumb.
- [ ] `--no-restart` flag skips the auto-restart and emits the manual-restart hint when the DBMS is `started`.
- [ ] Auto-restart stop/start failures emit a stderr warning + exit 0 (plugin change is not rolled back).
- [ ] 120s request timeout applies to `install` / `uninstall`; lifecycle leaves keep their 30s timeout.
- [ ] Plugin not found (relate 404 on a name that isn't a path either) yields the REQ-F-041 canonical error with the `plugin available` hint.
- [ ] DBMS not found yields the REQ-F-042 canonical error with the `dbms list` hint.

### Acceptance criteria — plugin testing (REQ-F-043..045)

- [ ] `test/e2e/desktop_fixture/` has handlers for all four plugin routes; in-memory state tracks `installed` / `available` per DBMS and toggles `pendingRestart` on the start/stop cycle.
- [ ] `test/e2e/desktop/desktop_test.go` covers the scenarios listed in REQ-F-044 (list happy/empty/dbms-not-found, available, install with-restart / without-restart / `--no-restart` / plugin-not-found, uninstall with-restart / idempotent / `--no-restart`).
- [ ] `make test` green with `-tags=e2e_desktop` and without.
- [ ] Skill bundle regenerated; `TestGenerator_RoundTrip` + `TestAllLeafCommands_HaveExamples` pass.

## Out of Scope

- `desktop versions` leaf (list installable Neo4j versions).
- Auto-launching Desktop via `open -a`.
- Backups, projects, manifest editing, clone.
- Plugin upgrade as a distinct leaf (`desktop dbms plugin upgrade`) — same JAR mtime/PID-mtime mechanism applies; user re-runs `install` over the existing plugin which relate copies in-place. Dedicated upgrade leaf deferred.
- Plugin pinning to a specific version (`--version` on `install`) — relate's `install` doesn't accept a version parameter; the version is bound to whatever `.jar` sits in the DBMS's `products/` or `labs/` directory.
- Headless/daemon mode for the Desktop local API.
- `desktop install --channel canary` — only `latest` in v1.
- `desktop install --version <vX>` — manifest only exposes `latest`.
- Linux `.deb` / `.rpm` packages — upstream publishes AppImage only.
- arm64 Linux Desktop install — no upstream build.
- Community edition support — Desktop 2 ships enterprise-only.

## Open Questions

_(none — all resolved during PRD iteration; see REQ-F-006/021/022/025–029)_
