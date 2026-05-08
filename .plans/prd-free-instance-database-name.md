# PRD: Free Aura Instance — Correct Database Name in Stored Credentials

## Overview

When `aura instance create` stores DBMS credentials after a successful instance creation, it hardcodes the database name as `"neo4j"`. For free Aura instances the default database is not `neo4j` — it matches the username (which equals the instance ID). This means any subsequent `neo4j query` invocation using the stored credential targets the wrong database and fails.

## Goals

- Store the correct database name when creating a free Aura instance.
- Ensure the stored username always comes from the API response with no silent fallback.

## Non-Goals

- Changing the credential storage format or file schema.
- Fetching database name from the Aura API (it is not returned).
- Altering credential storage behaviour for any command other than `aura instance create`.
- Changing the database name logic for any non-free instance type.

## Requirements

### Functional Requirements

- **REQ-F-001**: When `--type free-db` is provided AND the API response `username` is not `"neo4j"`, store `username` as the database name instead of `"neo4j"`.
- **REQ-F-002**: When `--type` is any value other than `free-db`, always store `"neo4j"` as the database name (existing behaviour unchanged).
- **REQ-F-003**: When `--type free-db` is provided but the API response `username` IS `"neo4j"`, store `"neo4j"` as the database name (no change from current behaviour — both conditions must be true).
- **REQ-F-004**: The username stored in the credential must come from the API response; there must be no hardcoded or implicit fallback to `"neo4j"` for the username field.

### Non-Functional Requirements

- **REQ-NF-001**: No change to the on-disk credential JSON schema or the `DbmsCredentials.Add` method signature.
- **REQ-NF-002**: The fix must be covered by table-driven tests in `create_test.go` that assert the stored `databaseName` for the free-db (username ≠ "neo4j"), free-db (username == "neo4j"), and non-free paths.

## Technical Considerations

- The relevant call site is `neo4j-cli/aura/internal/subcommands/instance/create.go` around line 209:
  ```go
  cfg.Credentials.Dbms.Add(resolvedName, username, password, "neo4j", uri)
  ```
  Replace the hardcoded `"neo4j"` with a helper that applies the detection logic.
- Username is already extracted from the API response on line 201 (`username, _ := instance["username"].(string)`), and instance type is available from the CLI flag — both are at hand at the call site without any additional API calls.
- Detection logic (suggest a small private helper `databaseName(instanceType, username string) string`):
  - If `instanceType == "free-db"` AND `username != "neo4j"` → return `username`
  - Otherwise → return `"neo4j"`
- `instanceType` can be read from the CLI flag value (already bound to the command) or from `instance["type"]` in the API response map — use whichever is simpler at the call site.
- Existing tests in `create_test.go` mock the API response; update the free-db test fixture to return a non-`"neo4j"` username and assert the stored credential carries the correct database name via `helper.AssertCredentialsValue`.

## Acceptance Criteria

- [ ] `aura instance create --type free-db` with a non-`"neo4j"` API username stores the database name equal to that username.
- [ ] `aura instance create --type free-db` with a `"neo4j"` API username stores `"neo4j"` as the database name.
- [ ] `aura instance create --type professional-db` (and all other non-free types) always stores `"neo4j"` as the database name, regardless of what the API username is.
- [ ] Table-driven unit tests cover all three cases above.
- [ ] `make test`, `make lint`, and `make fmt-check` all pass.

## Out of Scope

- Updating stored credentials for instances created before this fix.
- Exposing a `--database` flag on `instance create` for manual override.
- Changes to the `aura credential dbms` subcommands.
- Any username-based inference outside the `free-db` type.

## Open Questions

None.
