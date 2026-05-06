# PRD: Query --credential Flag

## Overview

Add a `--credential <name>` flag to `neo4j-cli query` that lets users explicitly select a named database credential from storage, bypassing the default credential resolution. This is the follow-up to the database credential storage feature (CLI-37 / `prd-db-credential-storage.md`).

## Goals

- Let users with multiple stored database credentials pick one by name at query time without changing the stored default.
- Keep the UX consistent: `--credential` is the only way to select a named credential; it cannot be mixed with per-flag connection details.
- Give a clear error message when the named credential does not exist.

## Non-Goals

- Changing how the default credential is selected or stored.
- Any changes to the `credential database` management commands.
- Support for credential names that contain spaces or special characters beyond what the shell already handles.

## Requirements

### Functional Requirements

- REQ-F-001: `neo4j-cli query` gains a new optional flag `--credential <name>` (no shorthand). When provided, the named database credential is loaded from `credentials.json` and used for the connection, ignoring both the stored default credential and any `.env` / env-var connection values.
- REQ-F-002: `--credential` is mutually exclusive with `--username`, `--password`, `--uri`, and `--database`. If any of those four flags is set at the same time as `--credential`, the command must error with a message identifying the conflict before making any connection attempt.
- REQ-F-003: If the named credential does not exist in `credentials.json`, the command must error with a message that names the missing credential and suggests running `neo4j-cli credential database list` to see available credentials.
- REQ-F-004: `--insecure` may be used alongside `--credential`. When provided it overrides the `insecure` field stored in the credential (same behaviour as when `--insecure` overrides a default credential's insecure value today).
- REQ-F-005: When `--credential` is not provided, existing connection resolution behaviour is unchanged: explicit flags/env vars take priority; otherwise the default stored credential is used; otherwise built-in defaults apply.

### Non-Functional Requirements

- REQ-NF-001: All new/modified Go source files carry the Neo4j copyright header.
- REQ-NF-002: `make test`, `make lint` and `make fmt-check` pass with no failures after all changes.
- REQ-NF-003: A changelog entry is created for `neo4j-cli` using `changie new`.
- REQ-NF-004: The skill bundle is regenerated (`go generate ./neo4j-cli/internal/skill/...`) since the query command's flag list changes.

## Technical Considerations

### Flag placement

Add `--credential` as a persistent flag on the `query` parent command in `neo4j-cli/query/query.go`, alongside the existing `--uri`, `--username`, `--password`, `--database` flags. It should be an optional string flag with an empty default.

### resolveConn integration

In `neo4j-cli/query/connect.go`, add a new resolution path at the top of `resolveConn()` (or immediately after flag parsing):

1. Check whether `--credential` flag is set (`cmd.Flag("credential").Changed`).
2. If set:
   a. Verify none of `--username`, `--password`, `--uri`, `--database` flags are also set (`f.Changed`); if any are, return a descriptive error.
   b. Look up the named credential via `cfg.Credentials.Database.Get(name)`.
   c. If not found, return an error: `credential "<name>" not found; run 'neo4j-cli credential database list' to see available credentials`.
   d. Populate `uri`, `username`, `password`, `database` from the credential.
   e. Apply the credential's `insecure` field as the base; if `--insecure` flag is also `Changed`, override with the flag's value.
   f. Skip the rest of the default-credential resolution.
3. If not set, existing logic is unchanged.

### Test coverage

Tests should live in `neo4j-cli/query/connect_test.go` (or the existing query test files), covering:
- `--credential <name>` loads the named credential successfully.
- `--credential <name>` where the credential doesn't exist → error with helpful message.
- `--credential <name>` combined with `--username` → conflict error.
- `--credential <name>` combined with `--insecure` → insecure overrides stored value.
- No `--credential` flag → existing behaviour unchanged.

## Acceptance Criteria

- [ ] `neo4j-cli query "RETURN 1" --credential mydb` connects using the stored `mydb` credential.
- [ ] `neo4j-cli query "RETURN 1" --credential unknown` errors with a message containing `"unknown"` and a hint to run `credential database list`.
- [ ] `neo4j-cli query "RETURN 1" --credential mydb --username neo4j` errors with a conflict message before any connection attempt.
- [ ] `neo4j-cli query "RETURN 1" --credential mydb --insecure` skips TLS regardless of the credential's stored `insecure` value.
- [ ] When `--credential` is not passed, the default credential (or fallback) behaviour is identical to before this change.
- [ ] `make test`, `make lint` and `make fmt-check` pass.
- [ ] Skill bundle is regenerated and `make generate-check` exits 0.
- [ ] Changelog entry present for `neo4j-cli`.

## Out of Scope

- Shorthand (`-c`) for `--credential`.
- `--credential` support in any command other than `neo4j-cli query`.
- Aura credential changes.

## Open Questions

- None.
