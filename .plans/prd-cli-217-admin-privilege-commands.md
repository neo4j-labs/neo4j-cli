# PRD: CLI-217 — Admin Privilege Commands (Enterprise)

Linear: [CLI-217](https://linear.app/neo4j/issue/CLI-217/pr-4-admin-privilege-commands-enterprise)
Parent: [CLI-128](https://linear.app/neo4j/issue/CLI-128/add-admin-commands-for-userdatabase-management)
Depends on: CLI-214 ✅ (merged PR #222), CLI-215 ✅ (merged PR #225), CLI-216 ✅ (merged PR #226)

---

## Overview

Fourth and final staged PR completing the `neo4j-cli admin` command tree (CLI-128). Implements
`neo4j-cli admin privilege list/grant/deny/revoke` for fine-grained RBAC privilege management
targeting Neo4j Enterprise. All commands execute Cypher against the `system` database via the
shared `adminutil.ExecFn` seam introduced in CLI-214.

The principal technical challenge is **action-category-aware Cypher construction**: each privilege
action belongs to one category (propertyBearer, graphOnly, graphWhole, load, labelScoped, database,
dbms) and each category has different valid resource scopes, qualifier flags, and Cypher templates.
A `buildPrivilegeCypher` helper in `privilege/helpers.go` centralises this logic and is directly
unit-tested. **The unit tests assert the generated string only — they do not execute it — so they
cannot catch a category that emits Cypher a real server rejects; CLI-217 QA found three such defects
(see "Bug fixes from QA").**

PR #226 (CLI-216, role commands) introduced two infrastructure changes that directly affect
privilege implementation: (1) `run.go`'s `translateAdminError` now takes a `uri string` parameter
and handles `Neo.ClientError.Security.Forbidden` with an Aura tier hint — privilege commands
inherit this automatically via `RunAdminStatement`; (2) the `NewCmd` convention now takes
`execFn adminutil.ExecFn` as a third parameter injected from `admin.go`, instead of resolving it
internally. Both patterns must be followed.

---

## Goals

1. Complete the admin command surface with Enterprise privilege management (`list`, `grant`, `deny`, `revoke`).
2. Implement correct per-action-category Cypher construction: the right ON clause, property clause, and entity clause per action type.
3. Emit the role's updated privilege list after every mutation (same output path as `privilege list --role <name>`).
4. Provide clear usage errors for incompatible flag combinations before ever hitting the database.
5. Pass all final gates: `make test`, `make fmt-check`, `make lint`, `TestGenerator_RoundTrip` (skill bundle regenerated with all admin commands).
6. Emit Cypher that a live Neo4j 2025.x server actually accepts for **every** advertised action — no action category may produce a guaranteed `SyntaxError`, and no qualifier flag may be silently dropped (added after CLI-217 QA found three such defects; see "Bug fixes from QA" below).

---

## Non-Goals

- Role CRUD (`admin role list/get/create/drop/grant/revoke`) — that is CLI-216, worked on in parallel.
- Aura-specific privilege guard: commands execute against Aura connections and surface natural errors (Forbidden, UnsupportedAdministrationCommand) without a pre-execution check.
- Privilege inheritance / effective-privilege resolution.
- Wildcard role targeting (granting to multiple roles at once).
- neo4j-admin binary integration.

---

## Requirements

### Functional Requirements

**`privilege list`**

- REQ-F-001: `neo4j-cli admin privilege list` — executes `CYPHER 25 SHOW PRIVILEGES`. Optional
  mutually exclusive filters: `--role <name>` executes `CYPHER 25 SHOW ROLE $name PRIVILEGES`;
  `--user <name>` executes `CYPHER 25 SHOW USER $name PRIVILEGES`. Specifying both is a usage
  error. Supports `--format json|table|toon`. No `--rw` required.

- REQ-F-002: Output fields for all `privilege list` variants: `access`, `action`, `resource`,
  `segment`, `role`. Field names are the post-`camelToSnake` form of the Bolt column names
  returned by `SHOW PRIVILEGES` (these are already snake_case from Neo4j so no transform needed).

**`privilege grant`**

- REQ-F-003: `neo4j-cli admin privilege grant` — executes
  `CYPHER 25 GRANT <ACTION> [<propClause>] ON <resourceClause> [<entityClause>] TO <role>`.
  Required flags: `--action <action>`, `--role <name>`. Resource scope flags (mutually exclusive,
  at most one): `--on-graph <name>`, `--on-database <name>`, `--on-dbms` (boolean). Requires
  `--rw`. Enterprise-only.

- REQ-F-004: On success, `privilege grant` emits the target role's updated privilege list using the
  same output path as `privilege list --role <name>` (`CYPHER 25 SHOW ROLE $name PRIVILEGES` via
  the same `privilegeExecFn` seam).

**`privilege deny`**

- REQ-F-005: `neo4j-cli admin privilege deny` — executes
  `CYPHER 25 DENY <ACTION> [<propClause>] ON <resourceClause> [<entityClause>] TO <role>`.
  Same flag surface as `privilege grant` (REQ-F-003). Requires `--rw`. Enterprise-only.

- REQ-F-006: On success, `privilege deny` emits the target role's updated privilege list (same as
  REQ-F-004).

**`privilege revoke`**

- REQ-F-007: `neo4j-cli admin privilege revoke` — executes
  `CYPHER 25 REVOKE [GRANT|DENY] <ACTION> [<propClause>] ON <resourceClause> [<entityClause>] FROM <role>`.
  Same resource and action flags as `privilege grant`. Additional optional flag:
  `--revoke-type grant|deny` — when set, prefixes the REVOKE with `GRANT` or `DENY`; when absent,
  emits plain `REVOKE` (revokes both grant and deny). Requires `--rw`. Enterprise-only.

- REQ-F-008: On success, `privilege revoke` emits the target role's updated privilege list (same as
  REQ-F-004).

**`--action` validation and Cypher construction**

- REQ-F-009: `--action` accepts privilege keyword strings case-insensitively, with `_` accepted
  as a word separator (e.g. `all_graph_privileges` or `ALL GRAPH PRIVILEGES` both normalise to
  `ALL GRAPH PRIVILEGES`). An unrecognised action value returns a usage error listing the valid
  action keywords.

- REQ-F-010: Action category classification — every valid action belongs to exactly one category,
  which controls what resource scope and qualifier flags are valid and what Cypher template is emitted:

  | Category | Actions | Property clause | Entity clause | Valid resource scope |
  |---|---|---|---|---|
  | `propertyBearer` | READ, MATCH, SET PROPERTY, MERGE | `{*}` or `{p1, p2}` | ELEMENTS * / NODES / RELATIONSHIPS | `--on-graph` only |
  | `graphOnly` | TRAVERSE, CREATE, DELETE | none | ELEMENTS * / NODES / RELATIONSHIPS | `--on-graph` only |
  | `graphWhole` | WRITE, ALL GRAPH PRIVILEGES | none | none (qualifiers rejected) | `--on-graph` only |
  | `load` | LOAD | none | none | none — `ON ALL DATA` (default) or `ON CIDR "<cidr>"` via `--cidr` |
  | `labelScoped` | SET LABEL, REMOVE LABEL | none | `<label1>, <label2>, …` (all `--node-label` values) | `--on-graph` only |
  | `database` | ACCESS, START, STOP, CREATE INDEX, DROP INDEX, SHOW INDEX, INDEX MANAGEMENT, CREATE CONSTRAINT, DROP CONSTRAINT, SHOW CONSTRAINT, CONSTRAINT MANAGEMENT, CREATE NEW NODE LABEL, CREATE NEW RELATIONSHIP TYPE, CREATE NEW PROPERTY NAME, NAME MANAGEMENT, ALL DATABASE PRIVILEGES, SHOW TRANSACTION, TERMINATE TRANSACTION, TRANSACTION MANAGEMENT | none | none | `--on-database` only (default `*`) |
  | `dbms` | CREATE ROLE, DROP ROLE, ASSIGN ROLE, REMOVE ROLE, SHOW ROLE, ROLE MANAGEMENT, CREATE USER, DROP USER, SHOW USER, SET USER STATUS, SET USER HOME DATABASE, ALTER USER, USER MANAGEMENT, CREATE DATABASE, DROP DATABASE, DATABASE MANAGEMENT, SHOW PRIVILEGE, PRIVILEGE MANAGEMENT, ALL DBMS PRIVILEGES | none | none | `--on-dbms` only (required) |

- REQ-F-011: Client-side usage errors for flag/category mismatches (checked before sending to the
  database):
  - `graphOnly`, `graphWhole`, or `propertyBearer` action with `--on-database` or `--on-dbms` → error
  - `database` action with `--on-graph` or `--on-dbms` → error
  - `dbms` action without `--on-dbms` → error: `"action <ACTION> requires --on-dbms"`
  - `dbms` action with `--on-graph` or `--on-database` → error
  - `labelScoped` (`SET LABEL`/`REMOVE LABEL`) without `--node-label` → error: `"action SET/REMOVE LABEL requires --node-label"`
  - any non-`propertyBearer` action with `--property` → error: `"<ACTION> does not accept a property qualifier"`
  - `graphWhole` (`WRITE`/`ALL GRAPH PRIVILEGES`) with `--node-label` or `--relationship-type` → error (REQ-F-024)
  - `load` (`LOAD`) with any scope or qualifier flag → error; `--cidr` with any non-`load` action → error (REQ-F-025)
  - `--node-label` and `--relationship-type` both set → error: `"--node-label and --relationship-type are mutually exclusive"`
  - `--on-graph`, `--on-database`, and `--on-dbms` more than one set simultaneously → error

- REQ-F-012: Cypher template rules per category:

  **propertyBearer** (`READ`, `MATCH`, `SET PROPERTY`, `MERGE`):
  ```
  GRANT READ {*} ON GRAPH * ELEMENTS * TO analyst
  GRANT READ {name} ON GRAPH neo4j NODES Person TO analyst
  ```
  Property clause: `{*}` when no `--property`; `{p1, p2}` when one or more `--property` values.
  Entity clause: `ELEMENTS *` when neither `--node-label` nor `--relationship-type`; `NODES l1, l2`
  when only `--node-label`; `RELATIONSHIPS t1, t2` when only `--relationship-type`.
  Resource: `ON GRAPH <name>` — default name is `*` when `--on-graph` is absent.

  **graphOnly** (`TRAVERSE`, `CREATE`, `DELETE`):
  ```
  GRANT TRAVERSE ON GRAPH * ELEMENTS * TO analyst
  GRANT TRAVERSE ON GRAPH neo4j NODES Movie TO analyst
  ```
  No property clause. Entity clause same as propertyBearer. Resource: `ON GRAPH <name>`.

  **graphWhole** (`WRITE`, `ALL GRAPH PRIVILEGES`) — REQ-F-023/024:
  ```
  GRANT WRITE ON GRAPH * TO analyst
  GRANT ALL GRAPH PRIVILEGES ON GRAPH neo4j TO analyst
  ```
  No property clause AND no entity clause (`ELEMENTS *` / `NODES` / `RELATIONSHIPS` is invalid Cypher
  for these actions). Resource: `ON GRAPH <name>` — default `*`. `--node-label`,
  `--relationship-type`, and `--property` are usage errors.

  **load** (`LOAD`) — REQ-F-025:
  ```
  GRANT LOAD ON ALL DATA TO analyst
  GRANT LOAD ON CIDR "127.0.0.1/32" TO analyst
  ```
  No scope/entity/property flags. `ON ALL DATA` by default; `ON CIDR "<cidr>"` when `--cidr` is set
  (CIDR value is a double-quoted string literal, not a backtick identifier). All of `--on-graph`,
  `--on-database`, `--on-dbms`, `--node-label`, `--relationship-type`, `--property` are usage errors;
  `--cidr` on any non-LOAD action is a usage error.

  **labelScoped** (`SET LABEL`, `REMOVE LABEL`) — REQ-F-026:
  ```
  GRANT SET LABEL Person ON GRAPH neo4j TO analyst
  GRANT SET LABEL Person, Movie ON GRAPH neo4j TO analyst
  ```
  Emits `SET LABEL <l1>, <l2>, …` using **all** `--node-label` values (each backtick-escaped),
  comma-joined; no entity clause. Resource: `ON GRAPH <name>`. `--relationship-type` and `--property`
  are invalid.

  **database** scope:
  ```
  GRANT ACCESS ON DATABASE neo4j TO analyst
  GRANT ALL DATABASE PRIVILEGES ON DATABASE * TO analyst
  ```
  No property or entity clause. Resource: `ON DATABASE <name>` — default name is `*` when
  `--on-database` is absent (and no other scope flag is provided).

  **dbms** scope:
  ```
  GRANT CREATE ROLE ON DBMS TO analyst
  GRANT ALL DBMS PRIVILEGES ON DBMS TO analyst
  ```
  No property or entity clause. Resource: `ON DBMS`.

- REQ-F-013: `FIND` is not a valid action keyword. It must not appear in `validActions`.

**Required flag validation**

- REQ-F-021: `--action` and `--role` are required for `privilege grant`, `privilege deny`, and
  `privilege revoke`. Do NOT use `cmd.MarkFlagRequired()`. Instead, add explicit pre-validation in
  `RunE` before `cmd.SilenceUsage = true`:
  ```go
  if action == "" {
      return clierr.NewUsageError("--action is required")
  }
  if roleName == "" {
      return clierr.NewUsageError("--role is required")
  }
  cmd.SilenceUsage = true
  ```
  This ensures missing required flags exit with code 2 (`usage_error`), not code 1 (`fatal_error`
  from cobra's built-in required-flag check). Cobra prints usage automatically when `SilenceUsage`
  is still false at the time the error is returned. Rationale: established by PR #226 (CLI-216)
  QA finding REQ-F-027; applied consistently across all admin write commands.

**Aura `Security.Forbidden` handling**

- REQ-F-022: `run.go` (updated by PR #226) already handles `Neo.ClientError.Security.Forbidden`
  via `translateAdminError(uri, err)` — when the connection URI matches `*.neo4j.io`, it returns a
  `validation_error` with an actionable Aura BC tier hint. Privilege commands inherit this
  automatically through `RunAdminStatement`. No per-command Forbidden handling is needed in the
  privilege package.

**Cross-cutting**

- REQ-F-014: All four `privilege` leaf commands are Enterprise-only. `UnsupportedAdministrationCommand`
  errors are translated by the existing `translateAdminError` in `admin/run.go` with the hint
  `"(requires Enterprise edition)"`.

- REQ-F-015: All four `privilege` leaves receive `--format json|table|toon` from the parent
  `admin` persistent flags (no per-leaf registration needed).

- REQ-F-016: `privilege grant`, `privilege deny`, `privilege revoke` are write commands and require
  `--rw` (consistent with all other admin write commands).

- REQ-F-017: Every `privilege` leaf command has a flush-left `Example:` field with ≥ 2 invocations,
  each with a `# comment` line and `neo4j-cli` prefix. Write commands include `--rw`. At least one
  read example per read command uses `--format json`. Gate: `TestAllLeafCommands_HaveExamples`.

- REQ-F-018: `admin.go` is updated with:
  1. Import: `"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/privilege"`
  2. `cmd.AddCommand(privilege.NewCmd(cfg, &adminConn, RunAdminStatement))` after the role `AddCommand`
  3. `Long` string: append `, \`privilege\` (list, grant, deny, revoke).` to the current Long
     (current Long ends with `` `role` (list, get, create, drop, grant, revoke).``)
  4. `Short`: update from `"Manage Neo4j databases, users, and roles"` to
     `"Manage Neo4j databases, users, roles, and privileges"`
  5. Package comment: update from `"managing Neo4j databases, users, and roles via the system
     database over Bolt"` to include `"and privileges"`

- REQ-F-019: `go generate ./neo4j-cli/internal/skill/...` is run after wiring `privilege` into the
  cobra tree. `TestGenerator_RoundTrip` must pass. This is the final skill bundle regeneration for
  the admin command tree.

- REQ-F-020: A Minor changelog entry is added via `changie new --projects neo4j-cli --kind Minor
  --body "..."` describing the new `admin privilege` commands.

**Bug fixes from QA (CLI-217 live testing against Neo4j 2025.10 Enterprise + Aura Free/Professional/Business Critical)**

These requirements correct three defects found during end-to-end QA. All three were *masked* by the
unit suite because the leaf/`buildPrivilegeCypher` tests assert the generated Cypher *string* and
never execute it against a server — two of them even assert the broken output as the expected value
(see REQ-F-028). Every fix below MUST be accompanied by a test whose expected Cypher is valid on a
real Neo4j 2025.x server.

- REQ-F-023: **`WRITE` and `ALL GRAPH PRIVILEGES` must not emit an entity clause.** These two actions
  are currently in the `graphOnly` category, which unconditionally appends `entityClause(...)`
  (defaulting to `ELEMENTS *`). Neo4j rejects this — `GRANT WRITE ON GRAPH * ELEMENTS * TO role`
  fails with `Neo.ClientError.Statement.SyntaxError (Invalid input 'ELEMENTS')` — so **no invocation
  of `WRITE` or `ALL GRAPH PRIVILEGES` can ever succeed** today. Introduce a new action category
  (e.g. `graphWhole`) for these two actions that emits `<ACTION> ON GRAPH <name>` with **no property
  clause and no entity clause**. Default graph name is `*` when `--on-graph` is absent. The correct
  forms are `GRANT WRITE ON GRAPH * TO role` and `GRANT ALL GRAPH PRIVILEGES ON GRAPH neo4j TO role`
  (both verified live). `TRAVERSE`, `CREATE`, `DELETE` remain in `graphOnly` (they correctly accept
  `ELEMENTS *` / `NODES` / `RELATIONSHIPS`, verified live).

- REQ-F-024: **`graphWhole` actions reject entity and property qualifiers with a usage error.** Because
  `WRITE` / `ALL GRAPH PRIVILEGES` take no segment, passing `--node-label`, `--relationship-type`, or
  `--property` with one of them is a client-side usage error (checked before hitting the DB), e.g.
  `"WRITE does not accept node-label, relationship-type, or property qualifiers"`. They still accept
  `--on-graph` (and only `--on-graph` — `--on-database` / `--on-dbms` remain errors, same as
  `graphOnly`).

- REQ-F-025: **`LOAD` is re-categorised to its real syntax.** `LOAD` is not a graph privilege; its
  grammar is `GRANT LOAD ON ALL DATA TO role` or `GRANT LOAD ON CIDR "<cidr>" TO role` (both verified
  live — `LOAD ON GRAPH *` fails with a `SyntaxError`). Move `LOAD` out of `graphOnly` into a new
  `load` category that emits:
  - `LOAD ON ALL DATA` when `--cidr` is absent (default), and
  - `LOAD ON CIDR "<cidr>"` when `--cidr <value>` is supplied (the CIDR value is rendered as a
    double-quoted string literal, not a backtick identifier).
  Add a new `--cidr <value>` flag to the shared privilege flag set (only meaningful for `LOAD`). A
  `load` action rejects `--on-graph`, `--on-database`, `--on-dbms`, `--node-label`,
  `--relationship-type`, and `--property` with a usage error; `--cidr` on any non-`LOAD` action is
  likewise a usage error.

- REQ-F-026: **`SET LABEL` / `REMOVE LABEL` emit all `--node-label` values, not just the first.** The
  `labelScoped` arm currently inlines only `opts.nodeLabels[0]`, silently dropping any additional
  `--node-label` values with no error or warning. Neo4j accepts a comma list:
  `GRANT SET LABEL Person, Movie ON GRAPH neo4j TO role` (verified live). Emit every `--node-label`
  value, comma-joined, each backtick-escaped via the existing `cypherIdentifier` helper (REQ-F-011's
  `--node-label`/`--relationship-type` mutual-exclusion still applies; `--relationship-type` remains
  invalid for label actions).

- REQ-F-027: **The `validActions` category map and the per-category Cypher templates are updated** to
  reflect REQ-F-023/024/025: `WRITE` and `ALL GRAPH PRIVILEGES` → `graphWhole`; `LOAD` → `load`;
  `TRAVERSE`, `CREATE`, `DELETE` stay `graphOnly`. The category-classification table in REQ-F-010 and
  the Cypher-template rules in REQ-F-012 are corrected accordingly (they currently mis-state that
  `WRITE`, `LOAD`, and `ALL GRAPH PRIVILEGES` take an `ELEMENTS *` clause).

- REQ-F-028: **The bug-encoding test assertions are corrected.** `privilege_helpers_test.go` asserts
  `"GRANT ALL GRAPH PRIVILEGES ON GRAPH neo4j ELEMENTS *"` (the invalid output), and the original
  acceptance criteria in this PRD encoded `GRANT WRITE ON GRAPH * ELEMENTS * TO analyst` and
  `GRANT ALL GRAPH PRIVILEGES ON GRAPH neo4j ELEMENTS * TO analyst` as expected. These expectations
  are replaced with the corrected (server-valid) Cypher, and the `graphOnly` happy-path test for
  `WRITE`/`ALL GRAPH PRIVILEGES` (if any) is moved to the new `graphWhole` cases.

### Non-Functional Requirements

- REQ-NF-001: Unit tests for every leaf command using the `privilegeExecFn` package-level seam
  (same pattern as `dbExecFn` in `database/` and `userExecFn` in `user/`). No live Bolt connection.

- REQ-NF-002: Direct unit tests for `buildPrivilegeCypher` in `privilege_helpers_test.go` covering
  one case per action category and all client-side flag validation error paths.

- REQ-NF-003: All new `.go` files carry the Neo4j copyright header.

- REQ-NF-004: `make test`, `make fmt-check`, `make lint` all pass.

- REQ-NF-005: For each action category — `propertyBearer`, `graphOnly`, `graphWhole`, `labelScoped`,
  `database`, `dbms`, and `load` — at least one `buildPrivilegeCypher` unit case asserts Cypher that
  is **known-valid on a real Neo4j 2025.x server** (a regression guard against the string-only tests
  re-encoding invalid output). The expected fragments for the QA-confirmed cases (before the leaf
  appends ` TO $role` / ` FROM $role`) are:

  ```
  GRANT WRITE ON GRAPH *
  GRANT ALL GRAPH PRIVILEGES ON GRAPH `neo4j`
  GRANT LOAD ON ALL DATA
  GRANT LOAD ON CIDR "127.0.0.1/32"
  GRANT SET LABEL `Person`, `Movie` ON GRAPH `neo4j`
  ```

---

## Technical Considerations

### Command Tree Layout

```
neo4j-cli/internal/subcommands/admin/privilege/
  privilege.go               # NewCmd; registers list/grant/deny/revoke; sets privilegeExecFn
  list.go                    # SHOW [ROLE <x>|USER <x>] PRIVILEGES
  list_test.go
  grant.go                   # GRANT <action> ... TO <role>
  grant_test.go
  deny.go                    # DENY <action> ... TO <role>
  deny_test.go
  revoke.go                  # REVOKE [GRANT|DENY] <action> ... FROM <role>
  revoke_test.go
  helpers.go                 # buildPrivilegeCypher, validActions, actionCategory, outputPrivileges
  privilege_helpers_test.go  # unit tests for buildPrivilegeCypher
```

Additions to existing files:
- `admin/admin.go`: one `cmd.AddCommand(privilege.NewCmd(cfg, conn))` line + Long update

### Package-Level Seam

Mirror the `role.NewCmd` pattern from PR #226 exactly — `execFn` is injected as a parameter,
not resolved internally:

```go
// privilege.go
var privilegeExecFn adminutil.ExecFn

// NewCmd returns the `admin privilege` parent cobra command. execFn is the Cypher
// execution function injected by the parent (admin.RunAdminStatement in production);
// passing it here avoids an import cycle. conn is a pointer to the connection
// resolved by admin's PersistentPreRunE and shared with all leaves.
func NewCmd(cfg *clicfg.Config, conn **dbconn.Conn, execFn adminutil.ExecFn) *cobra.Command {
    privilegeExecFn = execFn
    // ...
}
```

In `admin.go`:
```go
cmd.AddCommand(privilege.NewCmd(cfg, &adminConn, RunAdminStatement))
```

Tests override `privilegeExecFn` directly:
```go
privilegeExecFn = func(ctx context.Context, cfg *clicfg.Config, conn *dbconn.Conn, cypher string, params map[string]any) ([]map[string]any, error) {
    // assert cypher + params; return fake rows
}
```

### `buildPrivilegeCypher` Helper

Central function in `helpers.go`. Signature:

```go
func buildPrivilegeCypher(verb, action string, opts privilegeOpts) (string, map[string]any, error)
```

Where `verb` is `"GRANT"`, `"DENY"`, or `"REVOKE [GRANT|DENY]"` and `opts` captures all flag
values. Returns:
- The full Cypher string (without the `CYPHER 25 ` prefix — that is prepended by `run.go`'s `RunAdminStatement`)
- A params map (only `$role` is parameterised; action and resource are always inlined as keywords
  to match how Neo4j parses privilege Cypher)
- A usage error for any invalid flag combination (checked before the caller sends anything to the DB)

Because `RunAdminStatement` prepends `CYPHER 25 `, `buildPrivilegeCypher` should NOT include it.

### Output Fields and `outputPrivileges` Helper

The output helper in `helpers.go` (uses `adminutil.NewRows` for field filtering, unlike role's
`adminutil.Rows` — see "Output: `adminutil.NewRows` vs `adminutil.Rows`" above):

```go
var privilegeFields = []string{"access", "action", "resource", "segment", "role"}

func outputPrivileges(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, roleName string) error {
    rows, err := privilegeExecFn(cmd.Context(), cfg, conn, "SHOW ROLE $name PRIVILEGES", map[string]any{"name": roleName})
    if err != nil {
        return err
    }
    commonoutput.PrintBodyMap(cmd, cfg, adminutil.NewRows(rows, privilegeFields), privilegeFields)
    return nil
}
```

Grant, deny, and revoke all call `outputPrivileges` after a successful mutation. This uses the same
`privilegeExecFn` seam so tests can assert both the mutation Cypher and the follow-up query.

### Test Approach

Each leaf test file (`list_test.go`, `grant_test.go`, etc.) follows the pattern confirmed by the
merged `role/` package:
1. Override `privilegeExecFn` at test setup (no `TestMain` needed — just set before each test case)
2. Table-driven test cases covering the happy path, Enterprise-only error translation, and flag
   validation errors
3. For write commands: use a `withSequencedPrivilegeExecFn` helper (mirrors `withSequencedExecFn`
   in `role/role_helpers_test.go`) to return different results for the mutation call vs the
   follow-up `SHOW ROLE $name PRIVILEGES` call
4. Missing required flags test cases assert `ce.Code == 2` and message containing
   `"--action is required"` / `"--role is required"` — NOT cobra's built-in required-flag message

`privilege_helpers_test.go` — shared test infrastructure (mirrors `role/role_helpers_test.go`
exactly, scoped to `privilegeExecFn`):
```go
func testConn() *dbconn.Conn { ... }
func withFakePrivilegeExecFn(t *testing.T, rows []map[string]any, err error) { ... }
func withSequencedPrivilegeExecFn(t *testing.T, responses []struct{ rows ...; err error }) { ... }
```

`privilege_helpers_test.go` also contains direct unit tests for `buildPrivilegeCypher` — one table
per action category, all error paths — without needing a cobra command.

Note: Aura `Security.Forbidden` translation is already covered by `run_test.go`; no
privilege-specific test is needed for it.

### Output: `adminutil.NewRows` vs `adminutil.Rows`

The `role/helpers.go` output helpers use `adminutil.Rows(rows)` directly (the role/member columns
from `SHOW ROLES WITH USERS` are already lowercase and the desired full set). For privilege, use
`adminutil.NewRows(rows, privilegeFields)` instead — `SHOW PRIVILEGES` also returns an `immutable`
column that should be excluded from default output, and `NewRows` applies camelToSnake
normalization and field filtering in one step. This is consistent with the `database/` package.

### `run.go` Signature (Post-PR #226)

`translateAdminError` and `translateNeo4jError` in `run.go` now take a `uri string` parameter.
`RunAdminStatement` passes `conn.URI` internally. Privilege commands never call these functions
directly — they go through `RunAdminStatement` / the `privilegeExecFn` seam. No changes to
`run.go` are expected for this PR.

### Dependency on CLI-216 (Role Commands)

CLI-216 merged as PR #226 — `main` now has the complete `admin database`, `admin user`, and
`admin role` trees. Branch directly from `main`. The `privilege/` package has no code-level import
of the `role/` package; the only coupling is that `admin.go` wiring must happen after the
privilege package compiles.

### Skill Bundle

After wiring `privilege.NewCmd` into `admin.go`, run:
```
go generate ./neo4j-cli/internal/skill/...
```
This produces the final skill bundle with all admin subcommands. Commit the source files and the
regenerated bundle together.

### QA Bug-Fix Implementation Notes (REQ-F-023..028, REQ-NF-005)

- **New categories in `helpers.go`**: add `graphWhole` and `load` to the `actionCategory` enum. In
  `validActions`, re-map `WRITE` and `ALL GRAPH PRIVILEGES` from `graphOnly` → `graphWhole`, and
  `LOAD` from `graphOnly` → `load`. Leave `TRAVERSE`, `CREATE`, `DELETE` on `graphOnly`.
- **`buildPrivilegeCypher` switch arms**:
  - `graphWhole`: reject `--on-database`/`--on-dbms` (as `graphOnly` does) AND reject
    `--node-label`/`--relationship-type`/`--property`. Emit
    `normalized + " ON GRAPH " + cypherIdentifier(graph)` with no property/entity clause.
  - `load`: reject every scope flag (`--on-graph`/`--on-database`/`--on-dbms`) and every qualifier
    (`--node-label`/`--relationship-type`/`--property`). Emit `LOAD ON ALL DATA`, or
    `LOAD ON CIDR "<cidr>"` when `opts.cidr != ""`. The CIDR value is wrapped in double quotes as a
    Cypher string literal — NOT `cypherIdentifier` (which uses backticks). Consider escaping embedded
    `"`/backslash in the CIDR value for safety, consistent with task-011's injection-hardening.
  - `labelScoped`: change `opts.nodeLabels[0]` to `strings.Join(escapeIdentifiers(opts.nodeLabels), ", ")`
    so every label is emitted.
- **New flag**: add `--cidr` to `addPrivilegeFlags` (or register it only where it applies); add
  `cidr string` to `privilegeOpts`. Guard: `opts.cidr != ""` with `category != load` → usage error
  (mirror the existing `--property` non-propertyBearer guard at the top of `buildPrivilegeCypher`).
- **The `--property` guard** currently fires for any `category != propertyBearer`; verify it still
  produces a sensible message for `graphWhole`/`load` (REQ-F-024 wants a combined entity+property
  message for `graphWhole`, so order the new `graphWhole` qualifier check before or alongside it).
- **Tests (REQ-NF-005)**: update the `graphOnly` table in `privilege_helpers_test.go` to drop
  `WRITE`/`ALL GRAPH PRIVILEGES`/`LOAD`; add `graphWhole`, `load`, and multi-label `labelScoped`
  cases with the server-valid expected fragments listed in REQ-NF-005; add the new usage-error cases
  (REQ-F-024/025). The corrected expectations replace the previously bug-encoding assertions
  (REQ-F-028). No live Bolt connection — but the expected strings must be ones a human has confirmed
  valid on Neo4j 2025.x (they were, during CLI-217 QA).
- **Skill bundle + Long/help**: `--cidr` and the new behaviour change `grant`/`deny`/`revoke` flag
  help, so re-run `go generate ./neo4j-cli/internal/skill/...` and commit the regenerated bundle
  (`TestGenerator_RoundTrip`). Mention `--cidr` (LOAD-only) and that `WRITE`/`ALL GRAPH PRIVILEGES`
  take no qualifiers in the relevant `Long` strings.

---

## Acceptance Criteria

**Privilege list**
- [ ] `neo4j-cli admin privilege list --credential local` executes `SHOW PRIVILEGES` and renders all privilege records.
- [ ] `neo4j-cli admin privilege list --role analyst --credential local` executes `SHOW ROLE analyst PRIVILEGES`.
- [ ] `neo4j-cli admin privilege list --user alice --credential local` executes `SHOW USER alice PRIVILEGES`.
- [ ] `--role` and `--user` together return a usage error.
- [ ] All privilege commands return `"(requires Enterprise edition)"` on Community.

**Privilege grant / deny — Cypher construction (propertyBearer)**
- [ ] `--action read --on-graph * --role analyst --rw` → `GRANT READ {*} ON GRAPH * ELEMENTS * TO analyst`
- [ ] `--action read --on-graph neo4j --node-label Person --property name --role analyst --rw` → `GRANT READ {name} ON GRAPH neo4j NODES Person TO analyst`
- [ ] `--action match --on-graph neo4j --relationship-type KNOWS --property weight --role analyst --rw` → `GRANT MATCH {weight} ON GRAPH neo4j RELATIONSHIPS KNOWS TO analyst`
- [ ] `--action read --node-label Person --role analyst --rw` (no explicit `--on-graph`) → `GRANT READ {*} ON GRAPH * NODES Person TO analyst` (default graph `*`)

**Privilege grant / deny — Cypher construction (graphOnly: TRAVERSE, CREATE, DELETE)**
- [ ] `--action traverse --on-graph * --role analyst --rw` → `GRANT TRAVERSE ON GRAPH * ELEMENTS * TO analyst` (no `{*}`)
- [ ] `--action traverse --on-graph neo4j --node-label Movie --role analyst --rw` → ``GRANT TRAVERSE ON GRAPH `neo4j` NODES `Movie` TO analyst``

**Privilege grant / deny — Cypher construction (graphWhole: WRITE, ALL GRAPH PRIVILEGES) — REQ-F-023/024**
- [ ] `--action write --on-graph * --role analyst --rw` → `GRANT WRITE ON GRAPH * TO analyst` (no `ELEMENTS *`; executes successfully on a live server)
- [ ] `--action all_graph_privileges --on-graph neo4j --role analyst --rw` → ``GRANT ALL GRAPH PRIVILEGES ON GRAPH `neo4j` TO analyst`` (no `ELEMENTS *`)
- [ ] `--action write --on-graph * --node-label Person --role analyst --rw` → usage error (WRITE does not accept entity/property qualifiers)
- [ ] `--action write --on-graph * --property name --role analyst --rw` → usage error

**Privilege grant / deny — Cypher construction (load: LOAD) — REQ-F-025**
- [ ] `--action load --role analyst --rw` → `GRANT LOAD ON ALL DATA TO analyst` (executes successfully on a live server)
- [ ] `--action load --cidr 127.0.0.1/32 --role analyst --rw` → `GRANT LOAD ON CIDR "127.0.0.1/32" TO analyst`
- [ ] `--action load --on-graph * --role analyst --rw` → usage error (LOAD takes no scope/entity flags)
- [ ] `--cidr 127.0.0.1/32` on any non-LOAD action → usage error

**Privilege grant / deny — Cypher construction (labelScoped: SET LABEL / REMOVE LABEL) — REQ-F-026**
- [ ] `--action set_label --node-label Person --on-graph neo4j --role analyst --rw` → ``GRANT SET LABEL `Person` ON GRAPH `neo4j` TO analyst``
- [ ] `--action set_label --node-label Person --node-label Movie --on-graph neo4j --role analyst --rw` → ``GRANT SET LABEL `Person`, `Movie` ON GRAPH `neo4j` TO analyst`` (all labels emitted, none dropped)
- [ ] `--action remove_label --node-label Person --on-graph neo4j --role analyst --rw` → ``GRANT REMOVE LABEL `Person` ON GRAPH `neo4j` TO analyst``
- [ ] `--action set_label --role analyst --rw` (missing `--node-label`) → usage error

**Privilege grant / deny — Cypher construction (database scope)**
- [ ] `--action access --on-database neo4j --role analyst --rw` → `GRANT ACCESS ON DATABASE neo4j TO analyst`
- [ ] `--action access --role analyst --rw` (no scope flag) → `GRANT ACCESS ON DATABASE * TO analyst` (default `*`)
- [ ] `--action all_database_privileges --on-database * --role analyst --rw` → `GRANT ALL DATABASE PRIVILEGES ON DATABASE * TO analyst`

**Privilege grant / deny — Cypher construction (dbms scope)**
- [ ] `--action create_role --on-dbms --role analyst --rw` → `GRANT CREATE ROLE ON DBMS TO analyst`
- [ ] `--action all_dbms_privileges --on-dbms --role analyst --rw` → `GRANT ALL DBMS PRIVILEGES ON DBMS TO analyst`
- [ ] `--action create_role --role analyst --rw` (missing `--on-dbms`) → usage error `"action CREATE ROLE requires --on-dbms"`

**Privilege revoke**
- [ ] `--action read --on-graph * --role analyst --rw` → `REVOKE READ {*} ON GRAPH * ELEMENTS * FROM analyst`
- [ ] `--action read --on-graph * --role analyst --revoke-type grant --rw` → `REVOKE GRANT READ {*} ON GRAPH * ELEMENTS * FROM analyst`
- [ ] `--action read --on-graph * --role analyst --revoke-type deny --rw` → `REVOKE DENY READ {*} ON GRAPH * ELEMENTS * FROM analyst`

**Flag conflict validation**
- [ ] `--action traverse --property name --role analyst --rw` → usage error (`TRAVERSE` does not accept a property qualifier)
- [ ] `--action find --role analyst --rw` → unknown action usage error (`FIND` is not a valid keyword)
- [ ] `--action access --on-graph neo4j --role analyst --rw` → usage error (database-scope action with graph scope)
- [ ] `--action create_role --on-graph neo4j --role analyst --rw` → usage error (dbms action with graph scope)
- [ ] `--on-graph *` and `--on-database neo4j` together → usage error (mutually exclusive scopes)
- [ ] `--node-label Person --relationship-type KNOWS` together → usage error (mutually exclusive qualifiers)

**Mutation output**
- [ ] `privilege grant --action read --on-graph * --role analyst --rw` emits analyst's updated privilege list on success.
- [ ] `privilege deny --action write --on-graph * --role readonly --rw` emits readonly's updated privilege list on success.
- [ ] `privilege revoke --action read --on-graph * --role analyst --rw` emits analyst's updated privilege list on success.

**Required flag validation (exit code 2)**
- [ ] `privilege grant --role analyst --on-graph * --credential local --rw` (missing `--action`) returns exit code 2 and prints usage.
- [ ] `privilege grant --action read --on-graph * --credential local --rw` (missing `--role`) returns exit code 2 and prints usage.
- [ ] `privilege deny --role analyst --on-graph * --credential local --rw` (missing `--action`) returns exit code 2 and prints usage.
- [ ] `privilege revoke --role analyst --on-graph * --credential local --rw` (missing `--action`) returns exit code 2 and prints usage.
- [ ] Unit tests for the above assert `ce.Code == 2` and message content (`"--action is required"`, `"--role is required"`), not cobra's built-in required-flag message.

**Aura Forbidden (handled by run.go, inherited automatically)**
- [ ] `admin privilege list` against an Aura Free or Professional instance that returns `Security.Forbidden` produces a `validation_error` with the Aura BC tier hint (verified via `run_test.go` existing coverage — no new privilege-specific test needed).
- [ ] `admin.go` `Short` updated to "Manage Neo4j databases, users, roles, and privileges".
- [ ] `admin.go` `Long` description ends with `` `privilege` (list, grant, deny, revoke). ``.

**General**
- [ ] All four privilege leaf commands have non-empty flush-left `Example:` fields (passes `TestAllLeafCommands_HaveExamples`).
- [ ] `make test`, `make fmt-check`, `make lint` all pass.
- [ ] `go generate ./neo4j-cli/internal/skill/...` produces no diff (passes `TestGenerator_RoundTrip`).
- [ ] Changelog entry added.

---

## Out of Scope

- Role CRUD — CLI-216 (parallel).
- Aura-specific privilege checks beyond natural error surfacing.
- Privilege filtering by label, relationship type, or property in `privilege list` (only --role and --user filters).
- `DENY` removal semantics subtleties (handled by Neo4j; CLI just emits the correct verb).

---

## Open Questions

1. **`--on-graph` default for database-scope actions**: The current spec (REQ-F-010) says
   `database` actions default to `ON DATABASE *` when no scope flag is set. This means omitting
   all scope flags is valid for database actions. For `graphOnly` / `propertyBearer` actions, the
   default is also `ON GRAPH *`. Only `dbms` actions require an explicit flag (`--on-dbms`). This
   is intentional (matches `GRANT ... ON GRAPH * / DATABASE *` Cypher defaults) but is worth a
   final confirmation during review.

2. **`immutable` column**: `SHOW PRIVILEGES` returns an `immutable` boolean column on some Neo4j
   versions (for built-in privileges that cannot be revoked). The current `privilegeFields` list
   excludes it for brevity. Add it to the fields list if it proves useful during live testing.
