# PRD: CLI-128 — Admin Commands for User/Database Management

## Overview

Add a `neo4j-cli admin` command tree that exposes Neo4j administration operations — database lifecycle, user management, and role/permission assignment — as first-class CLI verbs. All commands execute Cypher against the `system` database via stored dbms credentials, making them work uniformly across self-managed deployments (Docker, Desktop, on-prem) and Aura Pro connections (where the platform permits). Database lifecycle operations are not supported on Aura and remain out of scope pending the Aura v2 multi-DB API (see investigation findings below).

Linear: [CLI-128](https://linear.app/neo4j/issue/CLI-128/add-admin-commands-for-userdatabase-management)
Related: [CLI-190](https://linear.app/neo4j/issue/CLI-190/) (DBMS credential changes)

---

## Investigation Findings

Tests run against `neo4j:latest` (Community 2026.04.0) and `neo4j:latest` Enterprise (2025.07.1) using `neo4j-cli docker create` + `neo4j-cli query`.

### Key Finding: Admin Cypher Requires Write Transaction Semantics

All system-database Cypher commands — including read-only ones like `SHOW DATABASES` and `SHOW USERS` — are classified as writes by the Bolt driver's EXPLAIN preflight. The `neo4j-cli query` command therefore requires `--rw` even for list/get operations. The new `admin` commands must bypass this by always running with `ExecuteWrite` semantics internally, with no `--rw` requirement surfaced to the user.

### Community Edition Capabilities

| Command group | Supported | Notes |
|---|---|---|
| `SHOW DATABASES` | ✅ | Shows `neo4j` + `system` only |
| `CREATE DATABASE` | ❌ | `UnsupportedAdministrationCommand` |
| `STOP / START DATABASE` | ❌ | `UnsupportedAdministrationCommand` |
| `DROP DATABASE` | ❌ | `UnsupportedAdministrationCommand` |
| `SHOW USERS` | ✅ | `roles` field is `null` (no RBAC) |
| `CREATE USER` | ✅ | |
| `DROP USER` | ✅ | |
| `RENAME USER` | ✅ | |
| `ALTER USER SET PASSWORD` | ✅ | |
| `ALTER USER SET STATUS SUSPENDED` | ❌ | "'SET STATUS' is not available in community edition" |
| `ALTER USER SET HOME DATABASE` | ❌ | "'HOME DATABASE' is not available in community edition" |
| `SHOW ROLES` | ❌ | `UnsupportedAdministrationCommand` |
| `SHOW PRIVILEGES` | ❌ | `UnsupportedAdministrationCommand` |
| `GRANT / REVOKE ROLE TO user` | ❌ | `UnsupportedAdministrationCommand` |
| `GRANT / REVOKE privilege TO role` | ❌ | `UnsupportedAdministrationCommand` |

**Community summary:** Only basic user CRUD is supported. Database management and all role/permission operations require Enterprise. Error messages from the DB are descriptive and can be surfaced directly with a prefixed hint ("requires Enterprise edition").

### Enterprise Edition Capabilities

All proposed commands work:
- **Databases:** `SHOW DATABASES`, `CREATE DATABASE`, `DROP DATABASE`, `STOP DATABASE`, `START DATABASE`, `ALTER DATABASE SET ACCESS READ ONLY/READ WRITE`, `SHOW DATABASE <name>`
- **Users:** `SHOW USERS`, `CREATE USER`, `DROP USER`, `RENAME USER`, `ALTER USER` (password, status, home database)
- **Roles:** `SHOW ROLES`, `CREATE ROLE`, `DROP ROLE`, `GRANT ROLE TO user`, `REVOKE ROLE FROM user`, `SHOW ROLES WITH USERS`
- **Privileges:** `SHOW PRIVILEGES`, `SHOW ROLE <x> PRIVILEGES`, `SHOW USER <x> PRIVILEGES`, `GRANT <privilege> ON <graph> TO <role>`, `REVOKE <privilege> ON <graph> FROM <role>`

### Edition Detection

`CALL dbms.components() YIELD edition` returns `"community"` or `"enterprise"`. This can be used as a preflight to give targeted error messages before attempting unsupported operations, though it adds a round-trip. Alternative: attempt the operation and translate `UnsupportedAdministrationCommand` errors inline.

Recommended approach: **translate errors inline** (no preflight round-trip). The Neo4j error code `Neo.ClientError.Statement.UnsupportedAdministrationCommand` reliably signals Community-edition limitations; the error message already names the unsupported clause. Surface these with an added hint: `"(requires Enterprise edition)"`.

### Aura Pro (professional-db) — Live Test Results

Tests run against a `professional-db` instance (`neo4j+s://6a5428d6.databases.neo4j.io`) created via `neo4j-cli aura instance create` on 2026-06-04. The `neo4j` user carries the Aura-provisioned `console_admin_pro_*` role.

| Command group | Supported | Notes |
|---|---|---|
| `SHOW DATABASES` | ✅ | Returns `neo4j` + `system` |
| `SHOW DATABASE <name>` | ✅ | Full record including all Yield fields |
| `CREATE DATABASE` | ❌ | `UnsupportedAdministrationCommand` with Aura KB link |
| `DROP DATABASE` | ❌ | `UnsupportedAdministrationCommand` with Aura KB link |
| `STOP DATABASE` | ❌ | `UnsupportedAdministrationCommand` with Aura KB link |
| `START DATABASE` | ❌ | Expected same; not tested |
| `SHOW USERS` | ✅ | Users have Aura-provisioned roles (e.g. `console_admin_pro_*`, `PUBLIC`) |
| `CREATE USER` | ✅ | |
| `DROP USER` | ✅ | |
| `RENAME USER` | ❌ | `Neo.ClientError.Statement.ArgumentError`: "Changing username is not supported when using an authentication or authentication provider apart from native." — Aura uses a non-native auth provider |
| `ALTER USER SET PASSWORD` | ✅ | |
| `ALTER USER SET STATUS SUSPENDED/ACTIVE` | ✅ | No Community-edition restriction on Aura |
| `ALTER USER SET HOME DATABASE` | ✅ | |
| `SHOW ROLES` | ✅ | Returns all Aura-provisioned roles |
| `SHOW ROLES WITH USERS` | ✅ | |
| `SHOW USER <x> PRIVILEGES` | ✅ | |
| `SHOW PRIVILEGES` | ❌ | `Neo.ClientError.Security.Forbidden` — the default `neo4j` user's `console_admin_pro_*` role does not include `show_privilege`; may succeed with a more-privileged user |
| `SHOW ROLE <x> PRIVILEGES` | ❌ | Same Forbidden — not granted to the default admin role |
| `CREATE ROLE` | ❌ | `Security.Forbidden` — `create_role` not granted to `console_admin_pro_*` |
| `DROP ROLE` | ❌ | `Security.Forbidden` — `drop_role` not granted |
| `GRANT ROLE <role> TO <user>` | ✅ | Works for any of the Aura-provisioned roles |
| `REVOKE ROLE <role> FROM <user>` | ✅ | |

**Aura Pro summary:** Many admin commands work on Aura Pro, including all user CRUD (except rename), SHOW DATABASES/USERS/ROLES, and GRANT/REVOKE role. Database lifecycle operations (CREATE/DROP/STOP/START DATABASE) are blocked by the Aura platform. Role creation/deletion and privilege inspection are blocked by the default admin user's privilege set (may be a user limitation, not a platform restriction).

### Aura Free (free-db) — Live Test Results

Tests run against a `free-db` instance (`neo4j+s://c533afa0.databases.neo4j.io`) on 2026-06-04. Notable: username and default database name are both the instance ID (e.g. `c533afa0`), not `neo4j`. The free-tier user carries `console_admin_free_*` role which has only data-access privileges (no `user_management`, `show_role`, `assign_role`, `database_management`).

| Command group | Supported | Notes |
|---|---|---|
| `SHOW DATABASES` | ✅ | Returns instance DB + `system`; shared infrastructure means multiple `system` rows appear (one primary + replicas) |
| `SHOW DATABASE <name>` | ✅ | Works |
| `CREATE DATABASE` | ❌ | `UnsupportedAdministrationCommand` with Aura KB link |
| `SHOW USERS` | ❌ | `Security.Forbidden` — `console_admin_free_*` lacks `user_management` privilege |
| `CREATE USER` | ❌ | `Security.Forbidden` — same |
| All other user commands | ❌ | `Security.Forbidden` — same |
| `SHOW ROLES` | ❌ | `Security.Forbidden` — `console_admin_free_*` lacks `show_role` privilege |
| All role commands | ❌ | `Security.Forbidden` — same |

**Aura Free summary:** Only `database list` and `database get` work. The default admin user on Free has no user or role management privileges. `Security.Forbidden` is the error for all user/role commands. The `database list` output may show duplicate `system` entries due to shared infrastructure — the CLI should present these as-is (no deduplication needed; accurate reflection of what the DB reports).

**Decision:** Remove the blanket Aura pre-execution guard. Commands that work should work; commands that fail will surface Aura's natural error messages. Add targeted translation only for the `RENAME USER` `ArgumentError` (a known Aura platform restriction). Database lifecycle commands remain out of scope pending the Aura v2 multi-DB API.

---

## Goals

1. Expose database lifecycle, user management, and role/permission management as named admin subcommands rather than raw Cypher.
2. Work uniformly against self-managed Neo4j deployments (Docker, Desktop, on-prem) via stored dbms credentials.
3. Work against Aura Pro connections for operations the platform supports (user management, database read, role assignment).
4. Provide clear, actionable error messages for unsupported operations (Community edition, Aura platform restrictions).
5. Follow the repo's one-file-per-leaf cobra layout and credential/output conventions.

## Non-Goals

- Aura database lifecycle management (`database create/drop/start/stop`) — blocked by the Aura platform; planned for the Aura v2 multi-DB API when available.
- Fine-grained privilege management beyond role assignment (e.g. `GRANT READ {prop} ON GRAPH * NODE Label`). The `role` subtree covers role CRUD and role-to-user assignment only.
- neo4j-admin binary integration (backup, restore, import — separate concern).
- Desktop-specific integration beyond standard Bolt credential resolution.

---

## Requirements

### Functional Requirements

**Database subcommand (Enterprise-only operations)**

- REQ-F-001: `neo4j-cli admin database list` — execute `SHOW DATABASES`, output name/type/status/access/default columns. Supports `--format json|table|toon`.
- REQ-F-002: `neo4j-cli admin database get <name>` — execute `SHOW DATABASE <name>`, output full record. Supports `--format`.
- REQ-F-003: `neo4j-cli admin database create <name>` — execute `CREATE DATABASE <name> IF NOT EXISTS`. Flag: `--wait` to block until `currentStatus = online`. Requires `--rw`.
- REQ-F-004: `neo4j-cli admin database drop <name>` — execute `DROP DATABASE <name>`. Requires `--rw` and `--yes` for confirmation (same pattern as `docker delete`).
- REQ-F-005: `neo4j-cli admin database start <name>` — execute `START DATABASE <name>`. Flag: `--wait` to block until `currentStatus = online`. Requires `--rw`.
- REQ-F-006: `neo4j-cli admin database stop <name>` — execute `STOP DATABASE <name>`. Flag: `--wait` to block until `currentStatus = offline`. Requires `--rw`.

**User subcommand (Community + Enterprise, with stated limitations)**

- REQ-F-010: `neo4j-cli admin user list` — execute `SHOW USERS`. Supports `--format`.
- REQ-F-011: `neo4j-cli admin user get <name>` — execute `SHOW USERS WHERE user = $name`. Supports `--format`.
- REQ-F-012: `neo4j-cli admin user create <name>` — execute `CREATE USER <name> SET PASSWORD $pwd SET PASSWORD CHANGE [NOT] REQUIRED`. Flags: `--password` (prompted if absent, never echoed), `--password-change-required` (default true). Enterprise-only flag: `--home-database`. Requires `--rw`.
- REQ-F-013: `neo4j-cli admin user drop <name>` — execute `DROP USER <name>`. Requires `--rw` and `--yes`.
- REQ-F-014: `neo4j-cli admin user rename <old-name> <new-name>` — execute `RENAME USER <old> TO <new>`. Requires `--rw`.
- REQ-F-015: `neo4j-cli admin user set-password <name>` — execute `ALTER USER <name> SET PASSWORD $pwd SET PASSWORD CHANGE [NOT] REQUIRED`. Flags: `--password` (prompted if absent), `--password-change-required` (default false). Requires `--rw`.
- REQ-F-016: `neo4j-cli admin user suspend <name>` — execute `ALTER USER <name> SET STATUS SUSPENDED`. Enterprise-only (Community returns clear error). Requires `--rw`.
- REQ-F-017: `neo4j-cli admin user activate <name>` — execute `ALTER USER <name> SET STATUS ACTIVE`. Enterprise-only. Requires `--rw`.

**Role subcommand (Enterprise-only — role CRUD + role assignment)**

- REQ-F-020: `neo4j-cli admin role list` — execute `SHOW ROLES WITH USERS`, output role/member pairs. Supports `--format`. Optional `--role <name>` or `--user <name>` filter.
- REQ-F-021: `neo4j-cli admin role get <role-name>` — execute `SHOW ROLE <name> PRIVILEGES`, output privilege records. Supports `--format`.
- REQ-F-022: `neo4j-cli admin role create <role-name>` — execute `CREATE ROLE <name>`. Requires `--rw`.
- REQ-F-023: `neo4j-cli admin role drop <role-name>` — execute `DROP ROLE <name>`. Requires `--rw` and `--yes`.
- REQ-F-024: `neo4j-cli admin role grant --role <role> --user <user>` — execute `GRANT ROLE <role> TO <user>`. Requires `--rw`.
- REQ-F-025: `neo4j-cli admin role revoke --role <role> --user <user>` — execute `REVOKE ROLE <role> FROM <user>`. Requires `--rw`.

**Cross-cutting**

- REQ-F-030: All `admin` subcommands accept `--credential <name>` (same as `query`) to select the target DBMS. The value is dispatched on its prefix: `"desktop"` resolves the running Desktop DBMS, `"desktop-connection:<uuid>"` resolves a saved Desktop connection, any other value is a persisted-store lookup (see REQ-F-038 / REQ-F-039).
- REQ-F-031: ~~Blanket Aura block — removed.~~ Individual commands execute against Aura connections without a pre-execution guard; errors from the Aura platform surface naturally per REQ-F-037.
- REQ-F-032: `UnsupportedAdministrationCommand` errors from Neo4j are caught and re-surfaced with an appended hint: `(requires Enterprise edition)`. Exception: if the error message contains Aura-specific text (e.g. `"not supported, for more info see https://support.neo4j.com"`), append `(not supported on Aura — use the Aura Console or API)` instead.
- REQ-F-033: Errors containing `'SET STATUS' is not available in community edition` or `'HOME DATABASE' is not available in community edition` are caught and re-surfaced with a clear Community-limitation message.
- REQ-F-034: All admin Cypher executes with write-transaction semantics internally. All mutating commands require `--rw` from the user, consistent with every other write-capable command in the CLI. Read-only commands (`list`, `get`) do not require `--rw`.
- REQ-F-035: All list/get commands support `--format json|table|toon`.
- REQ-F-036: All mutating commands that affect critical resources (`drop`, `delete`) require `--yes` to skip the interactive confirmation prompt.
- REQ-F-037: `user rename` translates `Neo.ClientError.Statement.ArgumentError` with text "Changing username is not supported when using an authentication or authentication provider apart from native" to: `"renaming users is not supported on Aura connections (Aura uses a non-native authentication provider)"`. Other `ArgumentError` variants from `RENAME USER` are surfaced as-is.
- REQ-F-038: `admin --credential desktop` resolves the single running Desktop DBMS (via the Desktop local API), using the same lookup logic as `query --credential desktop`. Errors (no Desktop running, multiple running DBMSes, Desktop unreachable) surface the same messages as `query`.
- REQ-F-039: `admin --credential desktop-connection:<uuid>` resolves a saved Desktop connection by UUID, using the same lookup logic as `query --credential desktop-connection:<uuid>`. A non-UUID value or unknown UUID surfaces a clear error matching `query`'s behavior.
- REQ-F-040: When a Desktop DBMS or connection match has no stored credentials (null-creds case — legacy DBMS / safeStorage unavailable), admin commands prompt for a password when stdin is a TTY (printing `"Password: "` to stderr, no echo), and fatal with the 3-option hint ("Pass --password explicitly, run 'credential dbms add' to register a connection, or open Desktop and use 'Reset password'") when stdin is not a TTY. This mirrors `query`'s `finishDesktopMatch` behavior exactly.
- REQ-F-041: Admin registers `--uri` (env: `NEO4J_URI`), `--username/-u` (env: `NEO4J_USERNAME`), and `--password/-p` (env: `NEO4J_PASSWORD`) as persistent flags on the `admin` parent command, with identical semantics to the same flags on `query`. These constitute the "direct connection" path and are mutually exclusive with `--credential`.
- REQ-F-042: Admin registers `--env` as a persistent flag and supports dotenv auto-discovery (walking up from the current working directory) when `--credential` is not set — same logic as `query`. Dotenv values have lower precedence than OS env vars and explicit flags. Dotenv is skipped entirely when `--credential` is set.
- REQ-F-043: Admin connection resolution follows the same full precedence chain as `query`'s `resolveConn`: dotenv < OS env var < explicit flag; stored default credential when no params are explicitly provided; built-in defaults (`neo4j://localhost:7687`, user `neo4j`) when nothing else resolves; URI normalization (rewriting `http://` → `neo4j://` and `https://` → `neo4j+s://` with an info message to stderr). Partial-override rejection (some but not all of `--uri/--username/--password` supplied without `--credential`) mirrors `query`.
- REQ-F-044: Passing `--uri`, `--username`, or `--password` alongside `--credential` is an error on admin, matching `query`'s conflict check. `--database` and `NEO4J_DATABASE` are not exposed on admin — admin always targets the `system` database regardless of env content.
- REQ-F-045: When the resolved password is empty after all sources (no flag, no env var, no dotenv, no stored credential), admin prompts for a password on TTY (printing `"Password: "` to stderr, no echo) and returns a usage error on non-TTY — same as `query`'s post-resolution password check in `runQuery`.
- REQ-F-046: Admin registers `--debug` (env: `NEO4J_DEBUG=1`) as a persistent flag on the `admin` parent, routing Bolt driver wire activity to stderr at DEBUG level — same as `query`.

### Non-Functional Requirements

- REQ-NF-001: Unit tests for every leaf command with a `fakeQueryRunner` test seam (analogous to `fakeDockerClient` in the docker subsystem).
- REQ-NF-002: No live database required for tests — all network calls are mocked.
- REQ-NF-003: Commands follow the repo's `--format` and `--credential` flag conventions.
- REQ-NF-004: Run `go generate ./neo4j-cli/internal/skill/...` after adding the command tree to keep the skill bundle current.
- REQ-NF-005: All new `.go` files carry the Neo4j copyright header.
- REQ-NF-006: Changelog entry via `changie new --projects neo4j-cli --kind Minor --body "..."`.
- REQ-NF-007: The full connection resolution logic (credential dispatch, Desktop prefix handling, dotenv loading, env var/flag merging, default credential fallback, URI normalization) must not be reimplemented from scratch in admin. Extract `resolveConn` (or the subset admin needs) into a shared `neo4j-cli/internal/dbconn/` package that both `query` and `admin` import. Admin's connection surface omits `--database` / `NEO4J_DATABASE`; all other behavior is identical to `query`.

---

## Technical Considerations

### Command Tree Layout

Following the one-file-per-leaf convention:

```
neo4j-cli/internal/subcommands/admin/
  admin.go              # NewCmd; registers database/user/role subcommands
  database/
    database.go         # parent; registers list/get/create/drop/start/stop
    list.go             # SHOW DATABASES
    get.go              # SHOW DATABASE <name>
    create.go           # CREATE DATABASE
    drop.go             # DROP DATABASE
    start.go            # START DATABASE
    stop.go             # STOP DATABASE
    *_test.go           # per-leaf tests
    helpers_test.go     # shared fakeRunner + helpers
  user/
    user.go             # parent
    list.go get.go create.go drop.go rename.go set_password.go suspend.go activate.go
    *_test.go helpers_test.go
  role/
    role.go             # parent
    list.go get.go create.go drop.go grant.go revoke.go
    *_test.go helpers_test.go
```

### Execution Model

Admin commands need a `runAdminStatement(ctx, cred, cypher, params)` helper that:
1. Resolves a Bolt connection using the existing `connect.go` credential resolution path.
2. Executes the Cypher via `ExecuteWrite` in an explicit session targeting the `system` database.
3. Returns `[]map[string]any` rows (for list/get) or nothing (for mutating commands).
4. Translates errors per REQ-F-032, REQ-F-033, and REQ-F-037 (Community-limitation messages, Aura-specific `UnsupportedAdministrationCommand` variants, and RENAME USER `ArgumentError` on Aura).

Note: no pre-execution Aura guard — commands execute against Aura connections and surface errors naturally.

The helper lives in `neo4j-cli/internal/subcommands/admin/run.go` with a `queryRunner` interface as the test seam (injectable in tests, defaulting to the real Bolt runner).

### `--wait` Implementation for Databases

`database create --wait` and `database start --wait` poll `SHOW DATABASE <name> YIELD currentStatus` in a loop (1s interval, 60s timeout) until `currentStatus = "online"`. `database stop --wait` polls until `currentStatus = "offline"`. This mirrors the Docker subsystem's `WaitForBolt` pattern.

### Password Handling

Copy the pattern from `neo4j-cli/query/run.go` exactly:

- A `passwordReader` package-level variable wraps `term.ReadPassword(int(os.Stdin.Fd()))` from `golang.org/x/term` — this is the test seam for injecting a stub in unit tests.
- A `stdinIsTTY` package-level variable wraps `term.IsTerminal(int(os.Stdin.Fd()))` — same test seam pattern.
- A `promptPassword(cmd)` helper: if `stdinIsTTY()` is false, return a `clierr.UsageError` ("password is required; pass --password or run interactively"); otherwise print `"Password: "` to `cmd.ErrOrStderr()`, call `passwordReader()`, print a newline, and return the result.
- `user create` and `user set-password` call `promptPassword` when `--password` is not set.

Both seam variables (`passwordReader`, `stdinIsTTY`) live at the top of `admin/user/helpers.go` (or similar) so every leaf in the user subtree can share them. Tests override them directly.

### `--yes` Confirmation

`database drop`, `user drop`, `role drop` gates behind an interactive `y/N` prompt unless `--yes` is passed. Error if neither `--yes` is passed nor a TTY is available (same pattern as `docker delete`).

### Skill Bundle Regeneration

Adding `admin` to the cobra tree requires running:
```
go generate ./neo4j-cli/internal/skill/...
```
This is gated by `TestGenerator_RoundTrip`. Run it after the full command tree is wired.

### Full Connection Parity with `query`

Admin commands must have the same connection surface as `query`, minus `--database`. The implementation approach is to extract `resolveConn` (and `desktop.go`) from `neo4j-cli/query/` into a new shared `neo4j-cli/internal/dbconn/` package, then have both `query` and `admin` import from it.

**Recommended extraction target: `neo4j-cli/internal/dbconn/`**

Move from `query/` into `dbconn/`:
- `resolveConn` → `dbconn.ResolveConn(cmd, cfg, opts)` where `opts` controls whether `--database` is active (query: yes; admin: no)
- `desktopMatch`, `resolveDesktopActiveDbmsCredential`, `resolveDesktopConnectionCredential`, `newDesktopFallthroughClient`, `buildConnFromDesktopMatch`, related helpers → `dbconn` package with the same exported test seam setters
- `conn` struct → `dbconn.Conn` (exported), with `URI`, `Username`, `Password`, `Database`, `UserAgent`, `Debug` fields
- `openDriver`, `driverOpener` test seam, URI normalization → `dbconn`
- `loadEnvFile`, `overlay`, `flagString` helpers → `dbconn`

`query/connect.go` becomes a thin wrapper: imports `dbconn`, registers flags, calls `dbconn.ResolveConn`, opens the driver. `query/desktop.go` is either removed or becomes re-exports.

**Admin wiring:**

`admin.go` registers the same persistent flags as `query` minus `--database`:
- `--uri`, `--username/-u`, `--password/-p`, `--env`, `--credential/-c`, `--debug`

After resolving the connection via `dbconn.ResolveConn`, admin extracts `(URI, Username, Password)` to open its own Bolt session targeting `system`. The `Database` field from `dbconn.Conn` is intentionally ignored.

**Connection model in admin:**

`boltAdminRunner.run()` and the `queryRunner` interface currently take `*credentials.DbmsCredential`. Replace with `*dbconn.Conn` (or a minimal `AdminConnParams` struct in `adminutil` mirroring `dbconn.Conn`'s URI/Username/Password). All leaf callers and `fakeQueryRunner` fakes must be updated.

**Password prompt at connection time:**

After `dbconn.ResolveConn`, if `conn.Password == ""`:
- On TTY: print `"Password: "` to stderr, read without echo, store on `conn`
- On non-TTY: return a usage error

The `stdinIsTTY` and `passwordReader` test seams used by `admin/user/helpers.go` (for user-create/set-password prompts) should live in a common location (e.g., `dbconn` or `adminutil`) so they can also be overridden in connection-level tests. Alternatively, define separate seam vars at the admin-root level.

**`--database` / `NEO4J_DATABASE`:**

Admin does not register `--database`. `dbconn.ResolveConn` should accept an option to skip database resolution entirely; admin passes this option so `NEO4J_DATABASE` is never consulted. The returned `conn.Database` is ignored by admin regardless.

### Example Fields

Every leaf command must have a non-empty flush-left `Example:` field with ≥2 invocations (enforced by `TestAllLeafCommands_HaveExamples`). Mutating commands (`create`, `drop`, `rename`, `set-password`, `suspend`, `activate`, `grant`, `revoke`) must include `--rw` in their examples, consistent with all other write-capable commands in the CLI. Read-only commands (`list`, `get`) do not include `--rw`.

---

## Acceptance Criteria

- [ ] `neo4j-cli admin database list/get/create/drop/start/stop` work against Enterprise, with `--wait` functional on create/start/stop.
- [ ] `neo4j-cli admin database create/start/stop/drop` return clear `UnsupportedAdministrationCommand (requires Enterprise edition)` error on Community.
- [ ] `neo4j-cli admin database list` returns correctly formatted output (json/table/toon).
- [ ] `neo4j-cli admin user list/get/create/drop/rename/set-password` work on both Community and Enterprise.
- [ ] `neo4j-cli admin user suspend/activate` return clear Community-limitation error on Community.
- [ ] `neo4j-cli admin user create` prompts for password when `--password` is not supplied.
- [ ] `neo4j-cli admin role list/get/create/drop/grant/revoke` work on Enterprise.
- [ ] `neo4j-cli admin role *` returns clear `UnsupportedAdministrationCommand (requires Enterprise edition)` error on Community.
- [ ] `neo4j-cli admin database list/get` work against Aura Pro (professional-db) credentials.
- [ ] `neo4j-cli admin database create/drop/start/stop` return a clear Aura-unsupported message (with Aura Console/API hint) when run against an Aura credential.
- [ ] `neo4j-cli admin user list/get/create/drop/set-password/suspend/activate` work against Aura Pro credentials.
- [ ] `neo4j-cli admin user rename` returns `"renaming users is not supported on Aura connections"` when run against an Aura credential.
- [ ] `neo4j-cli admin role list/grant/revoke` work against Aura Pro credentials.
- [ ] `database drop`, `user drop`, `role drop` require `--yes` when not interactive.
- [ ] All leaf commands have non-empty `Example:` fields (passes `TestAllLeafCommands_HaveExamples`).
- [ ] `make test`, `make fmt-check`, `make lint` all pass clean.
- [ ] `go generate ./neo4j-cli/internal/skill/...` produces no diff (passes `TestGenerator_RoundTrip`).
- [ ] Changelog entry added.
- [ ] `neo4j-cli admin user list --credential desktop` resolves the active Desktop DBMS and executes `SHOW USERS`.
- [ ] `neo4j-cli admin user list --credential desktop-connection:<uuid>` resolves the named Desktop connection.
- [ ] When the Desktop DBMS has no stored credentials on a TTY, admin prompts for a password and proceeds.
- [ ] When the Desktop DBMS has no stored credentials on a non-TTY, admin fatals with the 3-option hint.
- [ ] Passing `--credential desktop` when no Desktop DBMS is running produces the same error message as `query --credential desktop`.
- [ ] Unit tests cover the `desktop` and `desktop-connection:` paths in admin credential resolution (no live Desktop required — resolver seams are mocked).
- [ ] All previously-passing admin tests continue to pass after the connection-model refactor.
- [ ] `neo4j-cli admin user list --uri neo4j://localhost:7687 --username neo4j --password pass` connects without `--credential`.
- [ ] `NEO4J_URI` / `NEO4J_USERNAME` / `NEO4J_PASSWORD` env vars resolve for admin commands.
- [ ] A `.env` file in or above the current directory is loaded by admin when `--credential` is not set.
- [ ] `neo4j-cli admin user list --uri ... --credential mydb` returns a conflict error.
- [ ] Admin with no flags, no env vars, and no default stored credential connects to `neo4j://localhost:7687` with user `neo4j` (same built-in defaults as `query`).
- [ ] When resolved password is empty and stdin is a TTY, admin prompts for a password before opening the Bolt connection.
- [ ] `--debug` on admin routes driver wire activity to stderr (same as `query --debug`).
- [ ] `--database` is not a recognized flag on `neo4j-cli admin`; `NEO4J_DATABASE` env var is ignored.

---

## Out of Scope

- Aura database lifecycle management (`database create/drop/start/stop`) — planned for the Aura v2 multi-DB API when available. User management and role assignment work on Aura Pro via Cypher today.
- Fine-grained privilege management (e.g. `GRANT READ {prop} ON GRAPH * NODE Label`) — too complex for MVP; roles cover the most common access-control use cases.
- `DENY` privilege (Enterprise-only nuance; defer to a follow-on).
- Database alias management (`CREATE ALIAS`).
- Composite database management (`CREATE COMPOSITE DATABASE`).
- neo4j-admin operations (backup, restore, import, copy).

---

## Open Questions

1. **Password prompting library:** ✅ Resolved — copy the `passwordReader`/`stdinIsTTY`/`promptPassword` pattern from `neo4j-cli/query/run.go` (uses `golang.org/x/term`). See Technical Considerations > Password Handling.
2. **`--wait` polling:** ✅ `--wait` is confirmed for `database create/start/stop` (the only async operations — they wait for `currentStatus` to transition). All user and role commands complete synchronously and do not offer `--wait`. Follow `docker create --wait` for progress output style (emit status to stderr).
3. **`admin role` naming:** ✅ Resolved — renamed from `permission` to `role` to match the Neo4j concept. Subcommand is `neo4j-cli admin role`.
4. **Aura live testing:** ✅ Tested against both Aura Pro (professional-db) and Aura Free (free-db) on 2026-06-04. See Investigation Findings for full capability tables. Key difference: Free tier admin user lacks `user_management` and `show_role` privileges — only `database list/get` work on Free.
5. **`--rw` on admin commands:** ✅ Resolved — all mutating admin commands require `--rw`, consistent with every other write-capable command in the CLI (REQ-F-034 updated).
