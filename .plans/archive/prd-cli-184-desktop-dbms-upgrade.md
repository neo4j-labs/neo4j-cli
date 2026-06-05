# PRD: Desktop DBMS upgrade (with plugin-upgrade-mode)

## Overview

Add a new leaf command `neo4j-cli desktop dbms upgrade <id>` that upgrades a
Neo4j Desktop 2-managed DBMS to a newer Neo4j version via Desktop's local relate
API (`POST /dbmss/:dbmsId/upgrade`).

This reframes CLI-184. The ticket originally asked for `desktop dbms plugin
upgrade`, but investigation of the Desktop 2 source (`../neo4j-desktop-2`) showed
**there is no per-plugin upgrade endpoint** — the only plugin routes are
`available`, `installed`, `install`, `uninstall`. Plugin upgrades in Desktop
happen *only* as a side-effect of a full DBMS version upgrade, governed by a
`pluginUpgradeMode` option (`ALL` / `NONE` / `UPGRADABLE`, default `UPGRADABLE`).
A standalone `plugin upgrade` would be a misleading fake. The CLI is also missing
`desktop dbms upgrade` entirely. So the real feature is the DBMS upgrade command,
which carries `--plugin-upgrade-mode` (default `upgradable`) — the genuine home of
plugin upgrades.

## Goals

- Let users upgrade a Desktop-managed DBMS to a newer Neo4j version from the CLI.
- Expose plugin-carry-over behavior via `--plugin-upgrade-mode` (default `upgradable`).
- Match existing `desktop dbms` ergonomics: `--version` auto-pick, `--force`
  to stop a running target, `--rw` write gate, `--format` output, stderr
  breadcrumbs/hints.
- Reuse existing dbms-package scaffolding rather than introducing new plumbing.

## Non-Goals

- A per-plugin `plugin upgrade` command (no Desktop API support; misleading).
- Auto-starting the DBMS after upgrade (it ends stopped; user starts it).
- A `--wait`/poll loop (the upgrade POST is synchronous over HTTP and returns the
  upgraded `DbmsInfo` once complete).
- Exposing `options.noCache`.
- Downgrade or cross-edition changes (Desktop 2 is enterprise-only).

## Requirements

### Functional Requirements

- REQ-F-001: Add leaf `neo4j-cli desktop dbms upgrade <id>` under
  `neo4j-cli/internal/subcommands/desktop/dbms/upgrade.go`, mirroring the
  one-file-per-leaf cobra layout of `create.go`/`start.go`. `Args:
  cobra.ExactArgs(1)`; `Annotations: {"write":"true"}` (requires `--rw`).
- REQ-F-002: Mount the leaf in `dbms.go` (`cmd.AddCommand(newUpgradeCmd(cfg))`)
  and update the parent `Short`/`Long` to list `upgrade` and note it needs `--rw`.
- REQ-F-003: `--version` flag is optional. When omitted, query Desktop's catalog
  (`client.ListDbmsVersions`) and auto-pick the highest stable enterprise version
  via the existing `pickLatestStableEnterprise`, emitting a stderr breadcrumf
  `Using Neo4j enterprise <v> (<origin>)`.
- REQ-F-004: `--plugin-upgrade-mode` flag, default `upgradable`. Accept
  `all|none|upgradable` case-insensitively; map to the uppercase wire value
  (`ALL|NONE|UPGRADABLE`); reject any other value with a
  `clierr.NewUsageError` that lists the valid values.
- REQ-F-005: `--no-migrate` bool flag (default false). When set, send
  `options.migrate=false` (server default is `true`).
- REQ-F-006: `--backup` bool flag (default `true` = keep the pre-upgrade backup).
  Send `options.backup`; `--backup=false` instructs Desktop to delete the backup.
- REQ-F-007: Upgrade requires the target DBMS to be stopped. Fetch the target's
  status (`client.GetDbms`). If it is `started`:
  - without `--force`: refuse with a fatal error hinting to stop it
    (`neo4j-cli desktop dbms stop <id> --rw`) or pass `--force`.
  - with `--force`: stop the target (`client.StopDbms`), poll until `stopped`
    (`pollUntilStatus`), then proceed.
- REQ-F-008: Add `desktopclient.UpgradeDbms(ctx, id, version string, opts
  UpgradeDbmsOptions) (*DbmsInfo, error)` calling `POST
  /dbmss/<id>/upgrade` with body `{version, options?}`, where `options` contains
  only the keys actually set. Use a dedicated 30-minute timeout.
- REQ-F-009: On success, render the upgraded `DbmsInfo` via
  `output.PrintBodyMap` using the shared field set
  `{id,name,version,status,connectionUri}`, then print a stderr hint that the
  DBMS is stopped and how to start it (`neo4j-cli desktop dbms start <id> --rw`).
- REQ-F-010: Provide a flush-left `Example:` block with ≥2 invocations, each with
  `--rw`, at least one using `--format json`
  (`TestAllLeafCommands_HaveExamples` gate).
- REQ-F-011: Regenerate the skill bundle (`go generate
  ./neo4j-cli/internal/skill/...`) so the new leaf is reflected
  (`TestGenerator_RoundTrip` gate).
- REQ-F-012: Add a single changie changelog entry (kind `Minor`) describing the
  new command.

### Non-Functional Requirements

- REQ-NF-001: `desktopclient.UpgradeDbms` uses a dedicated
  `dbmsUpgradeTimeout = 30 * time.Minute` ceiling (mirrors the long-timeout
  pattern of `dbmsCreateTimeout`); a client-side timeout surfaces the existing
  canonical "may still be running in Desktop, check the UI" message.
- REQ-NF-002: No new shared plumbing — reuse `newDesktopClientFn`, `portFlag`,
  `pickLatestStableEnterprise`, `pollUntilStatus`, `dbmsStatusStarted/Stopped`,
  `formatDbmsRef`, `output.PrintBodyMap` from the `dbms` package.
- REQ-NF-003: All `.go` files carry the Neo4j copyright header; code passes
  `make fmt-check`, `make lint`, `make license-check`.
- REQ-NF-004: Tests use the in-memory FS / httptest pattern; no real-FS access in
  the query package; never touch the dev machine's credentials.

## Technical Considerations

- **Endpoint**: `POST /dbmss/:dbmsId/upgrade`, body `{ version: string, options?:
  { backup?, migrate?, noCache?, pluginUpgradeMode? } }`, reply `DbmsInfo`. The
  POST resolves *after* the upgrade completes (download + unpack + config upgrade
  + optional data migration + plugin handling + install swap) and returns the
  upgraded `DbmsInfo` (left **stopped**). Server requires the DBMS stopped
  (`Can only upgrade stopped dbms`) and defaults `pluginUpgradeMode` to
  `UPGRADABLE`, `migrate` to `true`.
- **Wire mapping**: `--plugin-upgrade-mode` lowercase → uppercase enum;
  send `migrate`/`backup` as explicit booleans (pointers in
  `UpgradeDbmsOptions`, only attach set keys); `pluginUpgradeMode` always sent
  since the flag has a default.
- **New types** (`desktopclient/types.go`):
  `UpgradeDbmsOptions{ Backup *bool; Migrate *bool; PluginUpgradeMode string }`.
- **Existing `DbmsInfo`** already covers the reply shape.
- **One-DBMS-at-a-time**: upgrade is offline and does not need port 7687, so only
  the target itself must be stopped (distinct from `create`/`start`'s
  `resolveConflicting`, which stops *other* running DBMSes).

## Acceptance Criteria

- [ ] `neo4j-cli desktop dbms upgrade <id> --version <v> --rw` upgrades a stopped
      DBMS and prints the upgraded `DbmsInfo` plus a "start it" hint.
- [ ] Omitting `--version` auto-picks the latest stable enterprise version and
      logs the breadcrumb.
- [ ] Running target without `--force` is refused (no upgrade POST issued); with
      `--force` it is stopped (Stop → poll stopped) then upgraded.
- [ ] `--plugin-upgrade-mode all|none|upgradable` is sent uppercased; an invalid
      value yields a usage error listing valid values.
- [ ] `--no-migrate` sends `options.migrate=false`; `--backup=false` sends
      `options.backup=false`; defaults send `migrate=true`, `backup=true`,
      `pluginUpgradeMode=UPGRADABLE`.
- [ ] `--format json` emits the full `DbmsInfo`.
- [ ] Without `--rw` the command is blocked by the write gate.
- [ ] `make test`, `make fmt-check`, `make lint`, `make license-check` pass;
      `TestGenerator_RoundTrip` and `TestAllLeafCommands_HaveExamples` pass.
- [ ] A single `Minor` changie entry is added.

## Out of Scope

- Per-plugin `plugin upgrade` command.
- Auto-start after upgrade; `--wait`; `--no-cache`.
- Downgrade / edition changes.

## Open Questions

None — all design decisions resolved in the source plan
(`/Users/oskarhane/.claude/plans/time-to-take-on-happy-boole.md`):
version auto-pick when omitted; refuse + `--force` for running targets;
flags `--plugin-upgrade-mode` (default `upgradable`), `--no-migrate`, `--backup`
(default keep); 30-minute upgrade timeout.
