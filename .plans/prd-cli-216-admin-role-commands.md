# PRD: CLI-216 — Admin Role Commands (PR 3 of 4)

## Overview

Third of four staged PRs implementing the `neo4j-cli admin` command tree (CLI-128). Adds the `admin role` subcommand tree (`list`, `get`, `create`, `drop`, `grant`, `revoke`) that exposes Neo4j role management via Cypher against the `system` database. All commands are Enterprise-only at the DB layer; Community returns a clear "requires Enterprise edition" error.

This PR stacks on top of CLI-215 / PR #225 (`admin user` commands). The branch `pr3/admin-role` targets `cli-215-admin-user-commands` and will be rebased onto `main` after PR #225 merges.

A draft implementation exists in `origin/feature/cli-128-admin-commands`, but PR 3 writes the role package **fresh from PR 2's patterns** with all known bugs fixed from the start — do not copy the draft directly.

Linear: [CLI-216](https://linear.app/neo4j/issue/CLI-216/pr-3-admin-role-commands-enterprise)
Parent: [CLI-128](https://linear.app/neo4j/issue/CLI-128/add-admin-commands-for-userdatabase-management)
Foundation: [CLI-214 / PR #222](https://github.com/neo4j-labs/neo4j-cli/pull/222) (merged)
Prereq: [CLI-215 / PR #225](https://github.com/neo4j-labs/neo4j-cli/pull/225) (in-review)

---

## Goals

1. Ship `neo4j-cli admin role list/get/create/drop/grant/revoke` following the exact structural patterns from PR 2 (`admin user`).
2. Fix all known bugs present in the draft branch before they reach review (wrong Cypher column name, wrong error code in drop, missing `IF NOT EXISTS`, null normalization gap, camelCase output field names).
3. Wire the `role` subtree into `admin.go`, update the admin Long description, regenerate the skill bundle, and add one changelog entry.
4. Keep each PR reviewable: this PR is role-only, no privilege commands.

## Non-Goals

- `neo4j-cli admin privilege` commands — that is CLI-217 (PR 4).
- Database alias management, composite databases, neo4j-admin binary integration.
- Any changes to the `admin database` or `admin user` packages.
- Changes to `dbconn`, `adminutil`, or `run.go` — the PR 1/2 foundation is complete.

---

## Requirements

### Functional Requirements

**`admin role list`**

- REQ-F-001: Execute `SHOW ROLES WITH USERS`. Output columns: `role`, `member`. Supports `--format json|table|toon`.
- REQ-F-002: Optional `--user <name>` flag — filter output rows in-process to those where `member == name`. This is the only command that answers "what roles does user X belong to".
- REQ-F-003: Normalize `null` `member` values to `""` (empty string) before output, in all formats. Community edition returns null for the member column on roles with no members.

**`admin role get`**

- REQ-F-004: First, execute `SHOW ROLES WITH USERS WHERE role = $name`. Column is `role`, **not** `name` (using `name` causes a Neo4j syntax error — this is the bug in the draft). Zero rows → `clierr.NewNotFoundError("role %q not found", name)`.
- REQ-F-005: If role exists, execute `SHOW ROLE $name PRIVILEGES`. Zero privilege rows → return empty list (exit 0, not not_found — a freshly created role has no privileges).
- REQ-F-006: Privilege output fields are dynamic: collect keys from `rows[0]`, sort them, pass sorted slice to `PrintBodyMap`. Supports `--format`.

**`admin role create`**

- REQ-F-007: Execute `CREATE ROLE $name IF NOT EXISTS` (idempotent; running twice must not return an error).
- REQ-F-008: After successful create, fetch and output `SHOW ROLES WITH USERS WHERE role = $name` via `outputRoleMembers`. Zero rows is valid (empty member list).
- REQ-F-009: Enterprise-only; `UnsupportedAdministrationCommand` translated per `translateAdminError` in `run.go`.
- REQ-F-010: Requires `--rw`.

**`admin role drop`**

- REQ-F-011: Execute `DROP ROLE $name`. Requires `--rw` and `--yes` (confirm gate, same pattern as `user drop` and `database drop`).
- REQ-F-012: Translate `Neo.ClientError.Statement.ArgumentError` with message containing `"does not exist"` → `clierr.NewNotFoundError("role %q not found", name)`. The correct error code is `Neo.ClientError.Statement.ArgumentError` — the draft incorrectly used `Neo.ClientError.Security.InvalidArguments`.
- REQ-F-013: No output on success (resource destroyed).
- REQ-F-014: Enterprise-only.

**`admin role grant`**

- REQ-F-015: Execute `GRANT ROLE $role TO $user`. Required flags: `--role <role>`, `--user <user>`.
- REQ-F-016: After successful grant, fetch and output the user's updated record via `outputUserAfterRoleChange(cmd, cfg, conn, userName)` — executes `SHOW USERS WHERE user = $name`, normalizes the row (null roles → `[]any{}`, null suspended → `false`), prints with fields `["user", "roles", "password_change_required", "suspended"]` (snake_case, matching PR 2's field names).
- REQ-F-017: Requires `--rw`. Enterprise-only.

**`admin role revoke`**

- REQ-F-018: Execute `REVOKE ROLE $role FROM $user`. Required flags: `--role <role>`, `--user <user>`.
- REQ-F-019: After successful revoke, fetch and output the user's updated record via `outputUserAfterRoleChange` (same as grant follow-up).
- REQ-F-020: Requires `--rw`. Enterprise-only.

**Cross-cutting**

- REQ-F-021: All role commands accept connection flags via the `admin` parent persistent flags (`--uri`, `--username`, `--password`, `--env`, `--credential`, `--debug`). No connection flags on the role subcommand tree itself.
- REQ-F-022: `UnsupportedAdministrationCommand` from Neo4j is translated to `"... (requires Enterprise edition)"` by `translateAdminError` in `run.go` — no per-command handling needed.
- REQ-F-023: All Cypher is automatically prefixed with `CYPHER 25` by `RunAdminStatement` in `run.go` — no per-command prefix needed.
- REQ-F-024: Every leaf must have a flush-left `Example:` field with ≥2 invocations, `# comment` per invocation, `neo4j-cli` prefix. Mutating commands include `--rw`. At least one read command per leaf uses `--format json`. Gate: `TestAllLeafCommands_HaveExamples`.

**QA-identified fixes (from live testing across Aura tiers)**

- REQ-F-025: When `Neo.ClientError.Security.Forbidden` is returned from a connection to an Aura-hosted instance (URI matches `*.neo4j.io`), `translateAdminError` must emit a clear, actionable hint explaining tier support — e.g.: `"<original message>. On Aura, role management support varies by tier: Business Critical supports all role commands; Professional supports list, grant, and revoke only; Free does not support role management."` — rather than Neo4j's raw "Permission has not been granted ... try GRANT/REVOKE" advice which is impossible to follow on Aura. For non-Aura connections the existing generic `Security.Forbidden` validation_error is returned unchanged. Implementation: add `isAuraURI(uri string) bool` helper in `run.go`; thread `conn.URI` from `RunAdminStatement` into `translateAdminError(uri, err)` and `translateNeo4jError(uri, ne)`.
- REQ-F-026: Remove `(Enterprise only)` from `revoke`'s `Short` description. It is the only command with this annotation; all six commands require Enterprise at the DB level. Consistency: none of the six should carry this annotation (the parent Long already documents the `--rw` requirement for write commands).
- REQ-F-027: Replace `cmd.MarkFlagRequired("role")` and `cmd.MarkFlagRequired("user")` in `grant.go` and `revoke.go` with explicit pre-validation at the top of `RunE` (before `cmd.SilenceUsage = true`) that returns `clierr.NewUsageError`. This ensures missing `--role` or `--user` exits with code 2 (`usage_error`) rather than code 1 (`fatal_error` from cobra's built-in required-flag check). Usage text must still be printed when either flag is absent (cobra prints usage whenever `RunE` returns an error and `SilenceUsage` is still false).

**Output shape refinement**

- REQ-F-028: `role list` and `outputRoleMembers` (called by `role create` follow-up) must group the flat `SHOW ROLES WITH USERS` rows by `role`, collecting all member values into a `members` list (plural). The output field `member` (singular) is renamed to `members` (plural). Null `member` values from the flat query are excluded from the collected `members` list, so a role with no members produces `"members": []`. The `--user` filter in `list` is adapted to retain only roles where the named user appears in the `members` list. The `roles` field on `SHOW USERS` output (used by `grant`/`revoke` follow-up via `outputUserAfterRoleChange`) is already returned as a list by Neo4j and is unaffected.
- REQ-F-029: `role get` must return a **single object** (not a flat list of privilege rows) with three fields: `role` (string), `members` (list), and `privileges` (list). This supersedes REQ-F-005 and REQ-F-006. The `members` list is extracted from the `SHOW ROLES WITH USERS WHERE role = $name` existence-check result using `groupRoleRows` — the same two-call structure is retained, but the first call now also feeds member data. The `privileges` list contains the rows from `SHOW ROLE $name PRIVILEGES` with the redundant `role` column removed (it is always the role name, already the key of the outer object) and the remaining fields sorted alphabetically (`access`, `action`, `resource`, `segment` for standard privileges). A role with no privileges yields `"privileges": []`. Output fields on the wrapping object: `["role", "members", "privileges"]`.
- REQ-F-030: **Format-dependent rendering of `members` in `role list` and `role create` follow-up.** In JSON and toon formats, `members` is emitted as a proper JSON list (the raw `[]any`). In table format, `members` is pre-converted to a comma-joined string (e.g., `"alice, bob"`) before passing to `PrintBodyMap`, keeping table rows compact and readable. Implementation: resolve the output format before printing (via `commonoutput.ResolveOutput(cmd, cfg)`) and convert the `members` value in each row to `strings.Join(...)` when format is `"table"`. Applies to both `list.go` and `outputRoleMembers` in `helpers.go`.
- REQ-F-031: **`role get` table format is best-effort.** The `members` and `privileges` fields are nested structures; in table format they render as multi-line indented JSON blobs inside table cells. This is accepted as a known limitation — `--format json` is the intended format for `role get`. The `Long` description of `role get` should note this: `"Use --format json for machine-readable output; table format renders nested fields as JSON."`

### Non-Functional Requirements

- REQ-NF-001: One file per leaf under `neo4j-cli/internal/subcommands/admin/role/`. Parent in `role.go`. Shared helpers + seam var in `helpers.go`. Test helpers in `role_helpers_test.go`.
- REQ-NF-002: `roleExecFn adminutil.ExecFn` as the package-level test seam, set by `NewCmd` and overridden by tests — identical pattern to `userExecFn` in PR 2.
- REQ-NF-003: Table-driven tests for every leaf. No live Bolt connection; all Cypher calls go through the seam.
- REQ-NF-004: Output field names are snake_case (e.g., `password_change_required`). Input identifiers (flag long names) are kebab-case.
- REQ-NF-005: All new `.go` files carry the Neo4j copyright header.
- REQ-NF-006: Run `go generate ./neo4j-cli/internal/skill/...` after wiring `role.NewCmd` into `admin.go`. `TestGenerator_RoundTrip` must pass.
- REQ-NF-007: Changelog entry: `changie new --projects neo4j-cli --kind Minor --body "Add admin role commands: list, get, create, drop, grant, revoke"`.
- REQ-NF-008: `make test`, `make fmt-check`, `make lint` all pass.
- REQ-NF-009: Unit tests for the Aura `Security.Forbidden` translation path in `run_test.go` — verify that `translateAdminError` with an Aura URI and a `Neo.ClientError.Security.Forbidden` Neo4jError produces a `validation_error` CLIError whose message contains the tier hint string; verify that the same error with a non-Aura URI does NOT include the hint.
- REQ-NF-010: Update tests in `grant_test.go` and `revoke_test.go` for missing `--role` and `--user` to assert exit code 2 (`usage_error`) rather than exit code 1, reflecting the switch from `MarkFlagRequired` to manual pre-validation.
- REQ-NF-011: In `helpers.go`: rename `roleFields` to `["role", "members"]`, remove `normalizeRoleRow`/`normalizeRoleRows`, and add `groupRoleRows(rows []map[string]any) []map[string]any` that groups by role (preserving SHOW output order) and collects non-nil member values into a `[]any` list. Update `outputRoleMembers` to call `groupRoleRows` and apply format-dependent member rendering (REQ-F-030). Update `list.go` to call `groupRoleRows`, adapt the `--user` filter, and apply the same format-dependent rendering before printing. Update `list_test.go` and `create_test.go` to assert the new grouped shape; include table-format assertions confirming comma-joined output.
- REQ-NF-012: Update `get.go` to assemble and output a single object `{"role": <name>, "members": <[]any from groupRoleRows>, "privileges": <[]any of privilege maps>}` using output fields `["role", "members", "privileges"]`. Delete the dynamic-field sort (`sort.Strings(fields)` over `rows[0]`). Remove the `role` key from each privilege row before assembling. Use `adminutil.NewRow` (single-row output) with `PrintBodyMap`. Update the `Long` description to note that `--format json` is intended for `role get`. Update `get_test.go`: update `existsResponse` to provide member data in the existence-check row, update all output assertions to the new shape, add a test for `TestGet_ExistingRole_NoPrivileges_ReturnsObjectWithEmptyPrivileges`.

---

## Technical Considerations

### File Layout

```
neo4j-cli/internal/subcommands/admin/role/
  role.go                  — NewCmd; sets roleExecFn; AddCommands added one per leaf task
  helpers.go               — roleExecFn seam, roleFields, normalizeRoleRow,
                             outputRoleMembers, outputUserAfterRoleChange
  list.go + list_test.go   — list_test.go owns runList helper
  get.go  + get_test.go    — get_test.go owns runGet + existsResponse/notFoundResponse
  create.go + create_test.go
  drop.go + drop_test.go
  grant.go + grant_test.go
  revoke.go + revoke_test.go
  role_helpers_test.go     — testConn(), withFakeExecFn, withSequencedExecFn only
                             (no runXxx helpers — those live in each leaf test file)
```

### Execution Seam Pattern (mirrors PR 2 exactly)

`helpers.go` contains:
```go
var roleExecFn adminutil.ExecFn

var roleFields = []string{"role", "member"}

func normalizeRoleRow(m map[string]any) map[string]any {
    if m["member"] == nil {
        m["member"] = ""
    }
    return m
}

func outputRoleMembers(cmd, cfg, conn, roleName) error {
    // SHOW ROLES WITH USERS WHERE role = $name
    // normalizeRoleRow each row
    // PrintBodyMap with roleFields
}

func outputUserAfterRoleChange(cmd, cfg, conn, userName) error {
    // SHOW USERS WHERE user = $name
    // normalize: roles nil→[]any{}, suspended nil→false
    // PrintBodyMap with ["user", "roles", "password_change_required", "suspended"]
}
```

`role.go` (skeleton at task-001 time, grown incrementally):
```go
func NewCmd(cfg *clicfg.Config, conn **dbconn.Conn, execFn adminutil.ExecFn) *cobra.Command {
    roleExecFn = execFn
    cmd := &cobra.Command{Use: "role", ...}
    // AddCommand calls are added one per leaf task (002–007).
    // Do NOT pre-wire leaves that don't compile yet.
    return cmd
}
```

Each leaf task (002–007) adds exactly one `cmd.AddCommand(newXxxCmd(cfg, conn))` line to `role.go` as part of that task's scope. `role.go` is never touched again after task-007 completes.

### `admin.go` Changes

Two lines change in `admin.go`:
1. Import `"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/role"` alongside database and user.
2. `cmd.AddCommand(role.NewCmd(cfg, &adminConn, RunAdminStatement))` after the user `AddCommand`.
3. Update `Long` to list `role` in the subcommands description.

### Critical Bug Fixes vs. Draft

| Bug | Draft code | Correct code |
|-----|-----------|-------------|
| `role get` existence check Cypher | `SHOW ROLES WHERE name = $name` | `SHOW ROLES WITH USERS WHERE role = $name` |
| `role drop` not-found error code | `Neo.ClientError.Security.InvalidArguments` | `Neo.ClientError.Statement.ArgumentError` |
| `role create` idempotency | `CREATE ROLE $name` | `CREATE ROLE $name IF NOT EXISTS` |
| `role list/create` null normalization | missing | `member: nil → ""` |
| `grant`/`revoke` user output field names | `"passwordChangeRequired"` (camelCase) | `"password_change_required"` (snake_case) |

### Test Pattern for `get` (two-call sequenced seam)

`get` calls `roleExecFn` twice (existence check + privileges). Tests must use a sequenced exec-fn that returns different results per call. See `withSequencedExecFn` in `role_helpers_test.go` — same pattern as `withSequencedExecFn` in PR 2's `user_helpers_test.go`.

The `existsResponse` / `notFoundResponse` helper functions in `role_helpers_test.go` pre-build the two-call response pairs, keeping test bodies readable.

### Test Pattern for `create`, `grant`, `revoke` (mutation + follow-up seam)

Each has a mutation call followed by a follow-up SHOW query. Use the same sequenced seam approach: first call = mutation (returns empty rows or error), second call = follow-up SHOW (returns rows to output). Tests assert both the error propagation path and the follow-up output path.

### Format-Dependent `members` Rendering (REQ-F-030)

The output package renders `[]any` values as multi-line indented JSON (`json.MarshalIndent`) in table cells. For `role list` and `role create` follow-up, this is too wide. Instead, resolve the format before calling `PrintBodyMap` and convert `members` to a comma-joined string when table:

```go
format := commonoutput.ResolveOutput(cmd, cfg)
for i, row := range rows {
    if format == "table" {
        members, _ := row["members"].([]any)
        strs := make([]string, len(members))
        for j, m := range members { strs[j] = fmt.Sprintf("%v", m) }
        rows[i]["members"] = strings.Join(strs, ", ")
    }
}
commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(rows), roleFields)
```

This runs in both `list.go` (after `groupRoleRows`) and `outputRoleMembers` (in `helpers.go`, after `groupRoleRows`). JSON and toon formats skip the conversion and receive the raw `[]any`.

### `role get` Single-Object Output (REQ-F-029)

The two-call structure of `get.go` is preserved, but the first call now feeds member data as well as the existence check:

```go
// Step 1: check existence and collect members.
existRows, err := roleExecFn(ctx, cfg, conn, "SHOW ROLES WITH USERS WHERE role = $name", ...)
if err != nil { return err }
if len(existRows) == 0 { return clierr.NewNotFoundError(...) }
members := groupRoleRows(existRows)[0]["members"]  // []any

// Step 2: fetch privileges.
privRows, err := roleExecFn(ctx, cfg, conn, "SHOW ROLE $name PRIVILEGES", ...)
if err != nil { return err }

// Drop the redundant "role" field from each privilege row.
privObjs := make([]any, len(privRows))
for i, row := range privRows {
    delete(row, "role")
    privObjs[i] = row
}

out := map[string]any{"role": name, "members": members, "privileges": privObjs}
commonoutput.PrintBodyMap(cmd, cfg, adminutil.NewRow(out, getFields), getFields)
```

`getFields = []string{"role", "members", "privileges"}` (logical order, not alphabetic — mirrors `userFields` in `helpers.go`).

For the no-privilege case `privObjs` is `[]any{}` and the output is a single object with `"privileges": []`. The `sort.Strings` dynamic-field logic is deleted; the four standard privilege fields (`access`, `action`, `resource`, `segment`) will appear naturally after the `role` deletion.

Table output renders `members` and `privileges` as JSON-marshaled cell values (same mechanism as `roles` in the user output from PR 2).

### groupRoleRows Helper (REQ-F-028)

`SHOW ROLES WITH USERS` emits one row per `(role, member)` pair. Roles with multiple members produce multiple rows; roles with no members produce one row with `member: nil`. `groupRoleRows` collapses these into one row per role:

```go
func groupRoleRows(rows []map[string]any) []map[string]any {
    type entry struct{ members []any }
    order := []string{}
    seen  := map[string]*entry{}
    for _, row := range rows {
        role := fmt.Sprintf("%v", row["role"])
        if _, ok := seen[role]; !ok {
            seen[role] = &entry{}
            order = append(order, role)
        }
        if m := row["member"]; m != nil {
            seen[role].members = append(seen[role].members, m)
        }
    }
    result := make([]map[string]any, len(order))
    for i, r := range order {
        members := seen[r].members
        if members == nil {
            members = []any{}
        }
        result[i] = map[string]any{"role": r, "members": members}
    }
    return result
}
```

`outputRoleMembers` replaces the `normalizeRoleRows` call with `groupRoleRows`. `list.go` replaces its `normalizeRoleRows` call with `groupRoleRows` and updates the `--user` filter to iterate over `row["members"].([]any)`.

`normalizeRoleRow` and `normalizeRoleRows` are deleted (their null-handling is subsumed by the nil-exclusion in `groupRoleRows`).

### Confirm Gate for `drop`

`drop.go` imports `"github.com/neo4j/cli/common/confirm"` and calls `confirm.Require(cmd, name)` before executing. `confirm.Register(cmd)` registers `--yes` and `--force` on the leaf. Tests use `confirmtest.AssertLeafGate` from `"github.com/neo4j/cli/common/confirm/confirmtest"` — exact pattern from `user drop` and `database drop` in PR 1/2.

### Not-Found Error Translation in `drop`

The not-found translation lives locally in `drop.go`'s `RunE` (after calling `roleExecFn`), not in `translateAdminError`, because the `ArgumentError` code is shared with other error conditions. Pattern:
```go
var ne *neo4j.Neo4jError
if errors.As(err, &ne) &&
    ne.Code == "Neo.ClientError.Statement.ArgumentError" &&
    strings.Contains(ne.Msg, "does not exist") {
    return clierr.NewNotFoundError("role %q not found", name)
}
```

### Aura `Security.Forbidden` Translation (REQ-F-025)

Live testing on Aura Free, Professional, and Business Critical revealed that role management commands do not return `UnsupportedAdministrationCommand` on restricted tiers — they return `Neo.ClientError.Security.Forbidden` with Neo4j's generic "Permission has not been granted … try GRANT/REVOKE" advice, which is impossible to follow on managed Aura instances.

**Tier support matrix (observed):**

| Command | Aura Free | Aura Pro | Aura BC |
|---------|-----------|----------|---------|
| `role list` | ❌ Forbidden | ✅ | ✅ |
| `role list --user` | ❌ Forbidden | ✅ | ✅ |
| `role grant` | ❌ Forbidden | ✅ | ✅ |
| `role revoke` | ❌ Forbidden | ✅ | ✅ |
| `role get` | ❌ Forbidden | ❌ Forbidden | ✅ |
| `role create` | ❌ Forbidden | ❌ Forbidden | ✅ |
| `role drop` | ❌ Forbidden | ❌ Forbidden | ✅ |

**Implementation:**

Add `isAuraURI(uri string) bool` in `run.go` that returns true when the URI domain ends in `.neo4j.io`:
```go
func isAuraURI(uri string) bool {
    return strings.Contains(strings.ToLower(uri), ".neo4j.io")
}
```

Change `translateAdminError(err error)` → `translateAdminError(uri string, err error)` and similarly thread `uri` into `translateNeo4jError`. Update `RunAdminStatement` to pass `conn.URI`.

In `translateNeo4jError`, handle the `Security.Forbidden` code:
```go
case "Neo.ClientError.Security.Forbidden":
    if isAuraURI(uri) {
        return clierr.NewValidationError(
            "%s\n\nOn Aura, role management support varies by tier: "+
            "Business Critical supports all role commands; "+
            "Professional supports list, grant, and revoke only; "+
            "Free does not support role management.",
            ne.Msg)
    }
    return clierr.NewValidationError("%w", ne)
```

`run_test.go` already tests `translateAdminError` via `RunAdminStatement` + `adminRunnerFn` seam. Add two new cases: one with a `neo4j+s://test.databases.neo4j.io` conn URI (hint fires) and one with `neo4j://localhost:7687` (hint does not fire).

### Required Flag Validation Exit Code (REQ-F-027)

Cobra's `MarkFlagRequired` fires a check _before_ `RunE`, generating a `fatal_error` (exit 1). To return `usage_error` (exit 2), remove `MarkFlagRequired` and add explicit checks in `RunE` before `cmd.SilenceUsage = true`:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    if roleName == "" {
        return clierr.NewUsageError("--role is required")
    }
    if userName == "" {
        return clierr.NewUsageError("--user is required")
    }
    cmd.SilenceUsage = true
    // ... rest of command
},
```

Because `SilenceUsage` is still false when these errors are returned, cobra will print the usage block automatically. Update existing tests in `grant_test.go` and `revoke_test.go` that asserted the cobra-style `"required flag(s) \"role\" not set"` message — they should now assert `ce.Code == 2` and a message containing `"--role is required"` / `"--user is required"`.

---

## Acceptance Criteria

- [ ] `neo4j-cli admin role list --credential local --format json` returns `[{"role":"...", "members":[...]}]` — one entry per role, members as a list
- [ ] `neo4j-cli admin role list --user alice --credential local` filters to roles where `alice` appears in the `members` list
- [ ] `role list` JSON output shows `"members": []` (not `null` or `[""]`) for roles with no members
- [ ] `neo4j-cli admin role get admin --credential local --format json` returns a single object `{"role":"admin","members":[...],"privileges":[...]}` — NOT a flat privilege list
- [ ] `neo4j-cli admin role get <newly-created-role> --credential local --format json` returns `{"role":"<name>","members":[],"privileges":[]}` (not `not_found`, not `[]`)
- [ ] `neo4j-cli admin role get nonexistent --credential local` returns exit code 3 (`not_found`)
- [ ] `role get` JSON output does NOT contain a `role` field inside each privilege object (the redundant column is dropped)
- [ ] `role get` existence check uses `SHOW ROLES WITH USERS WHERE role = $name` (verified via unit test asserting the Cypher string passed to the seam)
- [ ] `neo4j-cli admin role create analyst --credential local --rw` succeeds; emits the role's member record
- [ ] `neo4j-cli admin role create analyst --credential local --rw` succeeds on second invocation (`IF NOT EXISTS`)
- [ ] `neo4j-cli admin role create analyst --credential local --rw --format json` emits `[]` when the SHOW follow-up returns zero rows (Enterprise new role), or `[{"role":"analyst","members":[]}]` when it returns a null-member row (Community)
- [ ] `neo4j-cli admin role drop analyst --credential local --rw --yes --force` succeeds; no output
- [ ] `neo4j-cli admin role drop nonexistent --credential local --rw --yes --force` returns exit code 3 (`not_found`)
- [ ] `role drop` not-found test uses `Neo.ClientError.Statement.ArgumentError` with "does not exist" message
- [ ] `neo4j-cli admin role grant --role analyst --user alice --credential local --rw` succeeds; emits alice's updated user record
- [ ] `neo4j-cli admin role revoke --role analyst --user alice --credential local --rw` succeeds; emits alice's updated user record
- [ ] `grant`/`revoke` follow-up user output uses snake_case fields: `password_change_required` not `passwordChangeRequired`
- [ ] `--role` and `--user` flags are required for `grant` and `revoke`; missing either returns a usage error
- [ ] All role commands return `(requires Enterprise edition)` on Community edition
- [ ] All leaf commands have flush-left `Example:` fields — `TestAllLeafCommands_HaveExamples` passes
- [ ] `TestGenerator_RoundTrip` passes (skill bundle reflects database + user + role commands)
- [ ] `make test`, `make fmt-check`, `make lint` all pass
- [ ] Changelog entry added
- [ ] `role revoke --help` Short matches all other commands — no `(Enterprise only)` annotation
- [ ] `neo4j-cli admin role grant --user alice --credential local --rw` (missing `--role`) exits with code 2 and prints usage
- [ ] `neo4j-cli admin role revoke --role analyst --credential local --rw` (missing `--user`) exits with code 2 and prints usage
- [ ] `admin role list` against an Aura Free or Professional instance returns a `validation_error` whose message contains the tier hint (verified via `run_test.go` unit test with a `*.neo4j.io` URI)
- [ ] `admin role list` against a non-Aura instance with `Security.Forbidden` does NOT include the Aura tier hint in the message

---

## Out of Scope

- `admin privilege` commands (list, grant, deny, revoke) — CLI-217 (PR 4).
- Changes to `dbconn`, `adminutil`, or `admin.go`'s connection flag surface — foundation is complete from PR 1.
- Any changes to the `admin database` or `admin user` packages.
- Note: `run.go` IS touched by REQ-F-025 (Aura Forbidden hint); this is the only `run.go` change in this PR.

---

## Open Questions

None — requirements are fully specified by the parent PRD (CLI-128) and the patterns from PR 1 (CLI-214, merged) and PR 2 (CLI-215, in-review).
