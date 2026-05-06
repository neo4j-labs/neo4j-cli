# PRD: Database Credential Storage

## Overview

Add a `database` credential type to the neo4j-cli credential system so that users can store named connection profiles (uri, username, password, database name, insecure flag) and have `neo4j-cli query` use the default profile automatically when no connection flags or env vars are provided.

## Goals

- Eliminate the need to repeat connection details on every `neo4j-cli query` invocation.
- Follow the existing Aura credential pattern: named credentials, a settable default, and commands to add/list/use/remove.
- Keep the password out of all terminal output.
- Preserve full backward compatibility: users who pass flags or env vars today are unaffected.

## Non-Goals

- A `--credential <name>` flag on `neo4j-cli query` to select a non-default credential at query time (deferred to a follow-up).
- Encryption or secret-manager integration for stored passwords.
- Any changes to the Aura credential type.

## Requirements

### Functional Requirements

- REQ-F-001: A new `database` section is added to `~/.neo4j/cli/credentials.json` alongside the existing `aura` section. It stores a list of named database credentials and a `default-credential` key.
- REQ-F-002: Each database credential entry stores: `name` (friendly name), `username`, `password`, `database-name` (defaults to `"neo4j"` on add), `uri`, and `insecure` (bool).
- REQ-F-003: `neo4j-cli credential database add --name <name> --username <user> --password <pass> --uri <uri> [--database-name <db>] [--insecure]` adds a new named credential. All flags except `--database-name` and `--insecure` are required. `--database-name` defaults to `"neo4j"`. `--insecure` defaults to `false`; omitting it stores the credential with `insecure: false`. Adding a duplicate name is an error.
- REQ-F-004: `neo4j-cli credential database list` prints all stored database credentials as a table with columns `NAME`, `USERNAME`, `DATABASE-NAME`, `URI`, `INSECURE`, `DEFAULT`. The password is never included in any output.
- REQ-F-005: `neo4j-cli credential database use <name>` sets the named credential as the default in `credentials.json`. The name must refer to an existing credential; unknown names are an error.
- REQ-F-006: `neo4j-cli credential database remove <name>` deletes the named credential from storage. If the removed credential was the default, the `default-credential` key is cleared. Unknown names are an error.
- REQ-F-007: `neo4j-cli query` resolves connection parameters using the following precedence (highest wins):
  1. CLI flags / OS environment variables (existing behaviour)
  2. Default database credential from `credentials.json`
  3. Current built-in defaults (`http://localhost:7474`, username `neo4j`, database `neo4j`, TTY password prompt)
- REQ-F-008: If **any** of the four connection parameters (`--username` / `NEO4J_USERNAME`, `--password` / `NEO4J_PASSWORD`, `--uri` / `NEO4J_URI`, `--database` / `NEO4J_DATABASE`) is explicitly supplied (flag changed or env var non-empty), **all four** must be supplied. The command must error with a clear message if any of the four is missing in this case.
- REQ-F-009: When a default database credential is present and no connection flags/env vars are supplied, `neo4j-cli query` uses the credential's uri, username, password, database-name, and insecure values without prompting the user.
- REQ-F-010: The `--insecure` flag on `neo4j-cli query` continues to work as today when flags are used. When a stored credential's `insecure` field is `true`, TLS verification is skipped without requiring `--insecure` on the command line.

### Non-Functional Requirements

- REQ-NF-001: All new Go source files carry the Neo4j copyright header (enforced by `addlicense`).
- REQ-NF-002: New commands follow the one-file-per-leaf Cobra layout: `neo4j-cli/internal/subcommands/credential/database/database.go` (parent) plus `add.go`, `list.go`, `use.go`, `remove.go`, each with a colocated `*_test.go`.
- REQ-NF-003: `make test` and `make fmt-check` pass with no failures after all changes.
- REQ-NF-004: A changelog entry is created for both `aura-cli` and `neo4j-cli` using `changie new`.

## Technical Considerations

### Credential storage

`common/clicfg/credentials/credentials.go` defines `CredentialsFile`. Add a `Database *DatabaseCredentials` field (JSON key `"database"`, `omitempty`). Create `common/clicfg/credentials/database.go` mirroring the structure of `aura.go`:

```go
type DatabaseCredentials struct {
    Credentials       []DatabaseCredential `json:"credentials"`
    DefaultCredential string               `json:"default-credential"`
}

type DatabaseCredential struct {
    Name         string `json:"name"`
    Username     string `json:"username"`
    Password     string `json:"password"`
    DatabaseName string `json:"database-name"`
    URI          string `json:"uri"`
    Default      string `json:"default"`
    Insecure     bool   `json:"insecure"`
}
```

Methods needed: `Add`, `Remove`, `SetDefault`, `GetDefault`, `Get`, `List`.

### Command tree

Mount a new `database` subcommand under the existing `neo4j-cli credential` parent (`neo4j-cli/internal/subcommands/credential/credential.go`). The parent file calls `cmd.AddCommand(database.NewCmd(cfg))`.

Directory: `neo4j-cli/internal/subcommands/credential/database/`
- `database.go` — `NewCmd` registers the four leaf commands
- `add.go` / `add_test.go`
- `list.go` / `list_test.go`
- `use.go` / `use_test.go`
- `remove.go` / `remove_test.go`

### Query integration

In `neo4j-cli/query/connect.go`, after the existing `.env` / env-var / flag merge in `resolveConn()`:

1. Determine whether any connection parameter was explicitly set (flag `.Changed` or env var non-empty).
2. If yes and any of the four is still empty → return a descriptive error.
3. If none were set → load the default database credential via `cfg.Credentials.Database.GetDefault()` and overlay its values onto the `conn` struct (uri, username, password, database, insecure).
4. If no default credential exists → continue with current defaults / TTY prompt (unchanged).

The `cfg *clicfg.Config` value is already threaded through the `query` command; pass it into `resolveConn` or use it directly in `RunE`.

### Output

`list` uses `output.PrintBodyMap` with fields `["name", "username", "database-name", "uri", "insecure", "default"]` (no password). `--format json|table|toon` is honoured via the existing `--format` flag inherited from the root.

## Acceptance Criteria

- [ ] `neo4j-cli credential database add --name local --username neo4j --password secret --uri http://localhost:7474` succeeds and persists to `credentials.json`.
- [ ] `neo4j-cli credential database list` shows `local` with correct fields and no password column.
- [ ] `neo4j-cli credential database use local` sets `"default-credential": "local"` in `credentials.json`.
- [ ] After `use local`, `neo4j-cli query "RETURN 1"` (with no flags or env vars) connects using the stored credential.
- [ ] `neo4j-cli query "RETURN 1" --username neo4j` (only one flag set) errors with a message that all four connection params must be provided.
- [ ] `neo4j-cli query "RETURN 1" --username neo4j --password s --uri http://localhost:7474 --database neo4j` works as today (no change to existing behaviour).
- [ ] `neo4j-cli credential database remove local` deletes the entry; `list` shows empty.
- [ ] Removing the default credential clears `default-credential`; `query` falls back to defaults + TTY prompt.
- [ ] A credential stored with `--insecure` causes `query` to skip TLS verification without passing `--insecure` on the query command line.
- [ ] Password is absent from `list` output in both `table` and `json` format.
- [ ] `make test` and `make fmt-check` pass.
- [ ] Changelog entry present for `aura-cli` and `neo4j-cli`.

## Out of Scope

- `--credential <name>` flag on `neo4j-cli query` (future work).
- Encryption of stored passwords.
- Aura credential changes.
- Any changes to standalone `aura-cli` (database credentials are a neo4j-cli–only feature).

## Open Questions

- None.
