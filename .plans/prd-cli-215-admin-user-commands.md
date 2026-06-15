# PRD: CLI-215 — Admin User Commands (PR-2)

## Overview

Add the `neo4j-cli admin user` subcommand tree to the existing `admin` command introduced in PR-1 (commit `c1254c7`). PR-1 shipped the `admin database` tree and all shared infrastructure (connection resolution in `admin.go`, Bolt runner in `run.go`, error translation, `adminutil` package, `dbconn` shared package). This PR builds directly on those foundations to add eight user-management leaves: `list`, `get`, `create`, `drop`, `rename`, `set-password`, `suspend`, `activate`. It also wires the new subtree into `admin.go` and regenerates the skill bundle.

Linear: [CLI-215](https://linear.app/neo4j/issue/CLI-215/pr-2-admin-user-commands)
Previous PRD: [CLI-128](.plans/prd-cli-128-admin-commands.md) (full context, investigation findings)
Foundation PR: [#222](https://github.com/neo4j-labs/neo4j-cli/pull/222) (merged PR-1 — database commands + all shared infrastructure)
Draft PR: [#207](https://github.com/neo4j-labs/neo4j-cli/pull/207) (full admin tree draft — user/role/privilege reference implementation)

---

## Goals

1. Expose Neo4j user management (CRUD + rename, suspend, activate, set-password) as named `admin user` subcommands.
2. Reuse all connection infrastructure already wired in PR-1 — no new connection flags or resolution logic.
3. Follow the exact one-file-per-leaf cobra layout established by `admin/database/`.
4. Provide clear, actionable error messages for Community-edition restrictions and Aura platform limits.

## Non-Goals

- `admin role` commands — separate PR-3.
- `admin privilege` commands — separate chunk after roles.
- Any new connection flags or resolver changes — all already in `admin.go`.
- `--home-database` on user create (Enterprise-only flag; defer to a later PR or add as optional enhancement).

---

## Requirements

### Functional Requirements

**User leaves**

- REQ-F-001: `neo4j-cli admin user list` — execute `SHOW USERS`. Output fields: `user`, `roles`, `password_change_required`, `suspended`. `roles` is rendered as a comma-joined string in table/toon output (e.g. `admin, PUBLIC`); JSON renders it as an array. Community edition returns `null` for `roles` and `suspended`; normalize `null` `roles` → `[]` and `null` `suspended` → `false`. Supports `--format json|table|toon`. No `--rw`.

- REQ-F-002: `neo4j-cli admin user get <name>` — execute `SHOW USERS WHERE user = $name`. Same output fields and null normalization as `list`. Returns `not_found` (exit 3) when zero rows are returned. No `--rw`.

- REQ-F-003: `neo4j-cli admin user create <name>` — execute `CREATE USER $name SET PASSWORD $password SET PASSWORD CHANGE [NOT] REQUIRED`. Flags:
  - `--set-password` (string, empty default) — the initial password. When absent and stdin is a TTY, prompt interactively (print `"Password: "` to stderr, no echo). When absent and stdin is not a TTY, return a usage error.
  - `--password-change-required` (bool, default `true`) — whether the user must change their password on first login.
  - On success, emit the created user record (same fields as `get`, via `SHOW USERS WHERE user = $name`).
  - Requires `--rw`.

- REQ-F-004: `neo4j-cli admin user drop <name>` — execute `DROP USER $name`. Requires `--rw` and `--yes` for confirmation (same `confirm.Register` / `confirm.Require` pattern as `database drop`). Translates `Neo.ClientError.Statement.ArgumentError` whose message contains `"does not exist"` to `clierr.NewNotFoundError("user %q not found", name)` (exit 3). Produces no output on success.

- REQ-F-005: `neo4j-cli admin user rename <old-name> --new-name <new-name>` — execute `RENAME USER $oldName TO $newName`. One positional arg (`<old-name>`), one required flag (`--new-name`). This satisfies the one-positional-max convention; the two-positional form (`rename alice alice2`) is not supported. Translates `Neo.ClientError.Statement.ArgumentError` containing `"non-native"` or `"authentication provider apart from native"` to `"renaming users is not supported on Aura connections (Aura uses a non-native authentication provider)"` — this is already handled by `translateAdminError` in `run.go`. On success, emit the renamed user record via `SHOW USERS WHERE user = $newName`. Requires `--rw`.

- REQ-F-006: `neo4j-cli admin user set-password <name>` — execute `ALTER USER $name SET PASSWORD $password SET PASSWORD CHANGE [NOT] REQUIRED`. Flags:
  - `--new-password` (string, empty default) — the new password. Same prompt-on-TTY / usage-error-on-non-TTY behavior as `create`.
  - `--password-change-required` (bool, default `false`) — whether the user must change their password again after this reset.
  - On success, emit the updated user record via `SHOW USERS WHERE user = $name`.
  - Requires `--rw`.

- REQ-F-007: `neo4j-cli admin user suspend <name>` — execute `ALTER USER $name SET STATUS SUSPENDED`. Enterprise-only on self-managed; works on Aura Pro. On Community, `translateAdminError` already catches `Neo.DatabaseError.Statement.ExecutionFailed` with `"not available in community edition"` and classifies it as `validation_error` (exit 6, `retryable: false`). On success, emit the updated user record via `SHOW USERS WHERE user = $name`. Requires `--rw`.

- REQ-F-008: `neo4j-cli admin user activate <name>` — execute `ALTER USER $name SET STATUS ACTIVE`. Same Community-edition behaviour as `suspend`. On success, emit the updated user record via `SHOW USERS WHERE user = $name`. Requires `--rw`.

**Wiring and bundle**

- REQ-F-009: Mount `user.NewCmd(cfg, &adminConn, RunAdminStatement)` in `admin.go` via `cmd.AddCommand(...)`, mirroring the `database.NewCmd(...)` call already present. Update `admin.go` `Long` description to mention `user` subcommands.

- REQ-F-010: Run `go generate ./neo4j-cli/internal/skill/...` after wiring to regenerate the skill bundle. `TestGenerator_RoundTrip` must pass after this.

### Non-Functional Requirements

- REQ-NF-001: One file per leaf under `neo4j-cli/internal/subcommands/admin/user/`: `user.go`, `list.go`, `get.go`, `create.go`, `drop.go`, `rename.go`, `set_password.go`, `suspend.go`, `activate.go`, `helpers.go`, and colocated `*_test.go` + `user_helpers_test.go`.
- REQ-NF-002: Package-level `userExecFn adminutil.ExecFn` test seam declared in `helpers.go`; set by `NewCmd` (in `user.go`) at wiring time, replaced by `withFakeExecFn` in tests — mirrors `dbExecFn` in the database package. Declaring `userExecFn` in `helpers.go` allows all leaf files to reference it and be tested independently before `user.go` and its `NewCmd` exist.
- REQ-NF-003: Password prompting in `user/helpers.go` must reuse `dbconn.StdinIsTTY` and `dbconn.PasswordReader` (both exported package-level vars introduced in PR-1's `neo4j-cli/internal/dbconn/helpers.go`) rather than declaring new seam vars. Tests override `dbconn.StdinIsTTY` and `dbconn.PasswordReader` directly.
- REQ-NF-004: All new `.go` files carry the Neo4j copyright header.
- REQ-NF-005: Table-driven tests for every leaf using `fakeExecFn` — no live Bolt connection required.
- REQ-NF-006: Every leaf has a flush-left `Example:` with ≥2 invocations (`# comment` per invocation, `neo4j-cli` prefix, `--rw` on writes, ≥1 `--format json` on reads). Gate: `TestAllLeafCommands_HaveExamples`.
- REQ-NF-007: Changelog entry via `changie new --projects neo4j-cli --kind Minor --body "add admin user management commands (list, get, create, drop, rename, set-password, suspend, activate)"`.
- REQ-NF-008: A dedicated `helpers_test.go` in `neo4j-cli/internal/subcommands/admin/user/` (package `user`, internal test) that directly unit-tests the three shared helper functions:
  - `normalizeUserRow`: null roles → `[]any{}`, null suspended → `false`, non-null values pass through unchanged.
  - `outputUser`: zero rows → no output and no error; one row → row is printed (using fakeExecFn); execFn error → error returned.
  - `promptUserPassword`: non-empty flag value → returned immediately; empty flag + TTY → "Password: " printed to stderr and reader called; empty flag + non-TTY → usage error; PasswordReader error → wrapped error returned.

---

## Technical Considerations

### File Layout

```
neo4j-cli/internal/subcommands/admin/user/
  user.go              # NewCmd; sets userExecFn; AddCommand for all 8 leaves
  helpers.go           # passwordReader + stdinIsTTY seams; promptPassword; outputUser; userFields
  list.go              # SHOW USERS
  get.go               # SHOW USERS WHERE user = $name
  create.go            # CREATE USER ... SET PASSWORD ...
  drop.go              # DROP USER ... (confirm gate)
  rename.go            # RENAME USER ... TO ... (--new-name flag)
  set_password.go      # ALTER USER ... SET PASSWORD ...
  suspend.go           # ALTER USER ... SET STATUS SUSPENDED
  activate.go          # ALTER USER ... SET STATUS ACTIVE
  list_test.go
  get_test.go
  create_test.go
  drop_test.go
  rename_test.go
  set_password_test.go
  suspend_test.go
  activate_test.go
  user_helpers_test.go  # withFakeExecFn, fakeExecFn helpers, stdinIsTTY/passwordReader overrides
```

### Output Fields and Null Normalization

`userFields = []string{"user", "roles", "password_change_required", "suspended"}`

In `outputUser` (called by write commands after mutation), and in `list`/`get` `RunE`, normalize the raw row before passing to `adminutil.NewRow` / `adminutil.NewRows`:

```go
func normalizeUserRow(m map[string]any) map[string]any {
    if m["roles"] == nil {
        m["roles"] = []any{}
    }
    if m["suspended"] == nil {
        m["suspended"] = false
    }
    return m
}
```

For table/toon rendering, join the `roles` slice as a comma-separated string before passing to `PrintBodyMap`. One approach: a custom `adminutil.Row` or a pre-processing step in `outputUser` that replaces `[]any` with a joined string. The JSON path must retain the array; only table/toon flatten it. Use `commonoutput.PrintBodyMap`'s `fields` parameter with a pre-processed row where `roles` has been converted to `strings.Join(...)` for non-JSON formats, or pass the raw row for JSON.

Simpler approach: keep a `userTableRow` helper that normalizes + joins `roles` into a string specifically for table/toon, and use the raw `adminutil.NewRow(normalizeUserRow(m), userFields)` for JSON.

### Password Prompting

PR-1 (`neo4j-cli/internal/dbconn/helpers.go`) already exports the test seams:

```go
var StdinIsTTY = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
var PasswordReader = func() (string, error) { ... }
func PromptPassword(cmd *cobra.Command) (string, error) { ... }
```

`dbconn.PromptPassword` always prints `"Password: "` and uses the fixed error message `"--password is required or run interactively"` — appropriate for the connection-level prompt in `admin.go`'s `PersistentPreRunE`.

For the user-operation password flags (`--set-password`, `--new-password`), `user/helpers.go` needs a flag-name-aware wrapper that reuses the existing seams:

```go
// promptUserPassword reads the operation password from flagName.
// Calls dbconn.StdinIsTTY / dbconn.PasswordReader (both test-overridable) so
// no new seam vars are declared here.
func promptUserPassword(cmd *cobra.Command, flagName string) (string, error) {
    pw, _ := cmd.Flags().GetString(flagName)
    if pw != "" {
        return pw, nil
    }
    if !dbconn.StdinIsTTY() {
        return "", clierr.NewUsageError("--%s is required or run interactively", flagName)
    }
    _, _ = fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
    pw, err := dbconn.PasswordReader()
    _, _ = fmt.Fprintln(cmd.ErrOrStderr())
    if err != nil {
        return "", fmt.Errorf("read password: %w", err)
    }
    return pw, nil
}
```

Tests override `dbconn.StdinIsTTY` and `dbconn.PasswordReader` directly (no local seam vars in `user/`).

### `--set-password` vs `--new-password` Flag Names

The parent `admin` command registers `--password` as a persistent connection flag. If a leaf command also registers a local `--password` flag, cobra resolves the local one first for `cmd.Flag("password")` — breaking `dbconn.ResolveConn`'s ability to read the connection password when `--credential` is not set alongside a user-operation password.

Solution: use distinct leaf-local flag names:
- `user create`: `--set-password` for the new user's initial password.
- `user set-password`: `--new-password` for the replacement password.

### `user drop` Not-Found Translation

After `dbExecFn` returns an error, add a local check in `drop.go`'s `RunE`:

```go
var ne *neo4j.Neo4jError
if errors.As(err, &ne) &&
    ne.Code == "Neo.ClientError.Statement.ArgumentError" &&
    strings.Contains(ne.Msg, "does not exist") {
    return clierr.NewNotFoundError("user %q not found", name)
}
return err
```

This mirrors the `DatabaseNotFound` check in `database/drop.go`.

### Write-Command Follow-up Output

All mutating user commands (create, rename, set-password, suspend, activate) call `outputUser` after a successful mutation:

```go
// outputUser fetches and prints the current record for userName.
func outputUser(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, userName string) error {
    rows, err := userExecFn(cmd.Context(), cfg, conn, "SHOW USERS WHERE user = $name", map[string]any{"name": userName})
    if err != nil {
        return err
    }
    if len(rows) == 0 {
        return nil
    }
    // ... normalize + print
}
```

`drop` produces no output on success (resource destroyed). This is consistent with `database drop`.

### Error Handling Already in Place

The following error cases are already handled by `translateAdminError` / `translateNeo4jError` in `run.go` — user commands need no local translation for these:

| Neo4j error | Translated to |
|---|---|
| `UnsupportedAdministrationCommand` (non-Aura) | `validation_error` + "(requires Enterprise edition)" |
| `UnsupportedAdministrationCommand` (Aura KB URL) | `validation_error` + "(not supported on Aura...)" |
| `ArgumentError` with "non-native" / "authentication provider" | `validation_error` "renaming users is not supported on Aura" |
| `ExecutionFailed` with "not available in community edition" | `validation_error` |
| `SyntaxError` with "Invalid input 'CYPHER'" | `validation_error` "admin commands require Neo4j 2025.x or later" |

`user drop`'s not-found case (`ArgumentError` + "does not exist") is NOT handled globally because not all `ArgumentError` variants should become `not_found` — it must be handled locally in `drop.go`.

### Skill Bundle Regeneration

After `user.NewCmd` is wired in `admin.go`, run:

```
go generate ./neo4j-cli/internal/skill/...
```

Commit the regenerated bundle alongside the source changes. `TestGenerator_RoundTrip` will fail if skipped.

---

## Acceptance Criteria

- [ ] `neo4j-cli admin user list --credential local --format json` returns all users as a JSON array with `user`, `roles`, `password_change_required`, `suspended` keys.
- [ ] `neo4j-cli admin user list --format table` renders the `roles` column as a comma-joined string, not multi-line JSON.
- [ ] `neo4j-cli admin user list --format json` on Community edition shows `"roles": []` and `"suspended": false` (not `null`).
- [ ] `neo4j-cli admin user get alice --credential local` returns alice's record (exit 0).
- [ ] `neo4j-cli admin user get nonexistent --credential local` returns `not_found` (exit 3).
- [ ] `neo4j-cli admin user create alice --set-password s3cr3t --credential local --rw` creates alice and emits her user record.
- [ ] `neo4j-cli admin user create alice --credential local --rw` (no `--set-password`, TTY) prompts `"Password: "` on stderr and reads without echo.
- [ ] `neo4j-cli admin user create alice --credential local --rw` on non-TTY without `--set-password` returns a usage error.
- [ ] `neo4j-cli admin user create alice --password s3cr3t --credential local --rw` returns "unknown flag: --password" (old form rejected; --password is the connection flag).
- [ ] `neo4j-cli admin user drop alice --credential local --rw --yes --force` drops alice and produces no output.
- [ ] `neo4j-cli admin user drop nonexistent --credential local --rw --yes --force` returns `not_found` (exit 3).
- [ ] `neo4j-cli admin user drop alice --credential local --rw` (no `--yes`) prompts for confirmation on TTY.
- [ ] `neo4j-cli admin user rename alice --new-name bob --credential local --rw` renames alice and emits bob's user record.
- [ ] `neo4j-cli admin user rename alice bob --credential local --rw` (two positional args) returns "accepts 1 arg(s), received 2".
- [ ] `neo4j-cli admin user rename alice --new-name bob` against an Aura credential returns "renaming users is not supported on Aura connections".
- [ ] `neo4j-cli admin user set-password alice --new-password newpass --credential local --rw` updates the password and emits alice's updated user record.
- [ ] `neo4j-cli admin user set-password alice --credential local --rw` (no `--new-password`, TTY) prompts for the new password.
- [ ] `neo4j-cli admin user suspend alice --credential local --rw` on Enterprise suspends alice and emits her updated record with `"suspended": true`.
- [ ] `neo4j-cli admin user suspend alice --credential local --rw` on Community returns `validation_error` (exit 6, `retryable: false`).
- [ ] `neo4j-cli admin user activate alice --credential local --rw` on Enterprise activates alice and emits her updated record with `"suspended": false`.
- [ ] `helpers_test.go` (`package user`) tests `normalizeUserRow`, `outputUser`, and `promptUserPassword` directly; all cases pass.
- [ ] All leaf commands have non-empty flush-left `Example:` fields (passes `TestAllLeafCommands_HaveExamples`).
- [ ] `make test`, `make fmt-check`, `make lint` all pass clean.
- [ ] `go generate ./neo4j-cli/internal/skill/...` produces no diff (passes `TestGenerator_RoundTrip`).
- [ ] Changelog entry added.

---

## Out of Scope

- `admin role` commands — planned for PR-3.
- `admin privilege` commands — planned for a later chunk.
- `--home-database` flag on `user create` — Enterprise-only; can be added in a follow-up.
- Any changes to `admin.go` connection flags or `dbconn` / `run.go` — these are complete from PR-1.
- Live database integration tests — unit tests only; `fakeExecFn` seam covers all cases.

---

## Open Questions

None — all design decisions resolved in the CLI-128 PRD investigation findings and the draft PR #207.
