# PRD: Rename `database` Credential Type to `dbms`

## Overview

Rename the `database` credential type (introduced in the unreleased `add-db-credential-storage` branch) to `dbms` across the full stack: CLI command surface, Go package, Go type names, and the JSON storage key in `credentials.json`. Since the feature has never been released, no migration is required.

## Goals

- Surface the credential subcommand as `neo4j-cli credential dbms add/list/use/remove`.
- Rename all internal Go identifiers (`DatabaseCredentials` → `DbmsCredentials`, etc.) to match.
- Rename the JSON key in `credentials.json` from `"database"` to `"dbms"`.
- Update all error messages and help text that reference `credential database` to reference `credential dbms`.
- Keep the Aura credential type entirely untouched.

## Non-Goals

- Functional changes to how credentials are stored or resolved.
- Migration logic for any previous storage format (the feature is unreleased).
- Changes to `aura-cli` or the Aura credential type.
- Changes to the `--credential` flag semantics on `neo4j-cli query`.

## Requirements

### Functional Requirements

- REQ-F-001: The CLI subcommand `neo4j-cli credential database` is renamed to `neo4j-cli credential dbms`. All four leaf commands (`add`, `list`, `use`, `remove`) retain identical flags, arguments, and behaviour.
- REQ-F-002: The JSON key used to store dbms credentials in `credentials.json` changes from `"database"` to `"dbms"` (i.e. `{"dbms": {"default-credential": "...", "credentials": [...]}}`).
- REQ-F-003: The error message produced when a named credential is not found (used by both `resolveConn` in `connect.go` and the `use`/`remove` commands) references `credential dbms list` instead of `credential database list`.
- REQ-F-004: All Go source files, test files, and package paths use `dbms` in place of `database` for this credential type. Specifically:
  - `common/clicfg/credentials/database.go` → `dbms.go`; types `DatabaseCredentials` → `DbmsCredentials`, `DatabaseCredential` → `DbmsCredential`, `PrintableDatabaseCredentials` → `PrintableDbmsCredentials`
  - `common/clicfg/credentials/database_test.go` → `dbms_test.go`
  - `neo4j-cli/internal/subcommands/credential/database/` → `neo4j-cli/internal/subcommands/credential/dbms/`; package name `database` → `dbms`
  - `cfg.Credentials.Database` field → `cfg.Credentials.Dbms` everywhere it is referenced

### Non-Functional Requirements

- REQ-NF-001: All modified Go source files retain the Neo4j copyright header.
- REQ-NF-002: `make test`, `make lint`, and `make fmt-check` pass with no failures.
- REQ-NF-003: The skill bundle is regenerated (`go generate ./neo4j-cli/internal/skill/...`) since command `Use` strings change; `make generate-check` exits 0.
- REQ-NF-004: The existing unreleased changelog entry at `.changes/unreleased/neo4j-cli-Minor-20260506-070000.yaml` is edited (not replaced) to update its `body` to cover both the `credential dbms` commands and the `--credential` flag on `neo4j-cli query`. No new changelog file is created.
- REQ-NF-005: `AGENTS.md` is updated to replace stale `database`/`Database` references introduced by this branch. Specifically: the `Makefile Notes` mention of `credential/database/` becomes `credential/dbms/`; the `Credentials Storage Notes` section updates type names (`DatabaseCredentials` → `DbmsCredentials`, `PrintableDatabaseCredentials` → `PrintableDbmsCredentials`) and field name (`Database` → `Dbms`); the `query/connect.go Credential Integration Notes` section updates any reference to "database credentials" to "dbms credentials".

## Technical Considerations

### Rename touch-points

| Location | Old name | New name |
|---|---|---|
| `common/clicfg/credentials/credentials.go` field | `Database *DatabaseCredentials` (JSON `"database"`) | `Dbms *DbmsCredentials` (JSON `"dbms"`) |
| `common/clicfg/credentials/database.go` | file + all types | `dbms.go` + `DbmsCredentials`, `DbmsCredential`, `PrintableDbmsCredentials` |
| `common/clicfg/credentials/database_test.go` | file + all references | `dbms_test.go` |
| `neo4j-cli/internal/subcommands/credential/database/` | directory + package | `dbms/` + `package dbms` |
| `neo4j-cli/internal/subcommands/credential/credential.go` | import `database`, `database.NewCmd(cfg)` | import `dbms`, `dbms.NewCmd(cfg)` |
| `neo4j-cli/query/connect.go` | `cfg.Credentials.Database.*`, error text `credential database list` | `cfg.Credentials.Dbms.*`, error text `credential dbms list` |
| All `*_test.go` files referencing the above | update imports + identifiers | — |

### Credential `onUpdate` wiring

`credentials.go` re-wires `onUpdate` after JSON decode for the field name `Database` — this must be updated to `Dbms` at the same time.

### Cobra `Use` field

The parent command's `Use` field changes from `"database"` to `"dbms"`. This is what drives the CLI surface and is also what gets embedded in the skill bundle, so bundle regen is required.

### Auto-generated docs

`neo4j-cli/internal/skill/bundle/references/credential.md` and `neo4j-cli/internal/skill/bundle/references/query.md` both contain stale `credential database` references. These files are **auto-generated** by `go generate ./neo4j-cli/internal/skill/...` — do not edit them by hand. They will be corrected automatically when REQ-NF-003's `go generate` step runs. The `--credential` flag's usage string in `query.go` also references `credential database list`; updating that string before running `go generate` ensures the regenerated `query.md` is correct too.

### No migration

The `credentials.json` key changes from `"database"` to `"dbms"`. Since the feature is on an unreleased branch, there are no production users. No migration code is needed.

## Acceptance Criteria

- [ ] `neo4j-cli credential dbms add --name local --username neo4j --password s --uri http://localhost:7474` succeeds.
- [ ] `neo4j-cli credential dbms list` shows stored credentials.
- [ ] `neo4j-cli credential dbms use local` sets the default.
- [ ] `neo4j-cli credential dbms remove local` deletes the credential.
- [ ] `neo4j-cli credential database` no longer exists as a subcommand.
- [ ] `credentials.json` uses key `"dbms"` (not `"database"`) for stored credentials.
- [ ] `neo4j-cli query "RETURN 1" --credential unknown` error message references `credential dbms list`.
- [ ] `make test`, `make lint`, `make fmt-check` pass.
- [ ] `make generate-check` exits 0 (skill bundle up to date).
- [ ] `AGENTS.md` updated: `credential/database/` → `credential/dbms/`, type names and field names reflect `Dbms`.
- [ ] Existing changelog entry `.changes/unreleased/neo4j-cli-Minor-20260506-070000.yaml` body is updated to mention both `credential dbms` commands and the `--credential` flag on `neo4j-cli query`.

## Out of Scope

- Functional changes to credential storage or query resolution.
- Migration for existing `"database"` keys in credentials.json.
- Aura credential changes.

## Open Questions

- None.
