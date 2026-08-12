# PRD: `--database` override for `--credential` connections (CLI-212)

## Overview

`neo4j-cli query --credential <x>` currently rejects `--database` (mutual exclusivity), and Neo4j Desktop's local API never supplies a database name. After the #211 fix (PR #213), the desktop credential path leaves the database empty so the server resolves the home DB — but there is NO way to target a non-home database on a multi-db Desktop DBMS. This feature makes `--database` (and `NEO4J_DATABASE` OS env) combinable with ALL `--credential` forms, overriding the credential-supplied database.

Tracks GitHub issue neo4j-labs/neo4j-cli#212 / Linear CLI-212. Implementation plan pre-approved by user at `/Users/oskarhane/.claude/plans/look-into-https-linear-app-neo4j-issue-c-hidden-stream.md`.

## Goals

- Allow targeting a specific (non-home) database when connecting via any `--credential` form: `desktop`, `desktop-connection:<uuid>`, or a persisted dbms credential name.
- Keep `--uri`/`--username`/`--password` mutually exclusive with `--credential` (those ARE the credential).

## Non-Goals

- No dotenv support in the credential path (credential path continues to skip dotenv loading entirely).
- No change to Desktop API types (`DbmsInfo`/`Connection`/`Credentials` stay DB-less — the API has no DB field).
- No change to the non-credential resolution path (dotenv/env/flags/stored-default precedence stays as-is).

## Requirements

### Functional Requirements

- REQ-F-001: `query --credential <any form> --database <name>` succeeds; the session targets `<name>`. The conflict check in `resolveConn` (`neo4j-cli/query/connect.go`) shrinks from `{uri, username, password, database}` to `{uri, username, password}`; error message updated.
- REQ-F-002: Database override precedence with `--credential`: explicit `--database` flag (even explicitly empty, i.e. `f.Changed`) > `NEO4J_DATABASE` OS env (`os.Getenv(envDatabase)`, NOT dotenv) > credential-supplied database (persisted cred's stored `DatabaseName`; empty for desktop forms) > empty (server resolves home DB).
- REQ-F-003: Override applies uniformly to all three credential paths: desktop active (`finishDesktopMatch`), desktop-connection (`finishDesktopMatch`), persisted (`buildConnFromPersistedCred`).
- REQ-F-004: Help text updated: `--credential` flag usage notes combinability with `--database`/`NEO4J_DATABASE`; `--database` usage notes it also applies with `--credential`; `resolveConn` doc comment updated (currently says none of the four may be set alongside `--credential`).
- REQ-F-005: One new flush-left `Example:` line on the query command showing `--credential desktop --database <name>` (per Example gate rules: `# comment`, `neo4j-cli` prefix, flush-left).

### Non-Functional Requirements

- REQ-NF-001: Skill bundle regenerated (`go generate ./neo4j-cli/internal/skill/...`) — flag-usage/Example changes drift `references/query.md`; bundle committed with source (gate: `TestGenerator_RoundTrip`).
- REQ-NF-002: User-facing changelog entry: `changie new --projects neo4j-cli --kind Minor --body '...'`.
- REQ-NF-003: Tests follow existing patterns: `SetResolveDesktopActiveDbmsCredentialFnForTest` / `SetResolveDesktopConnectionCredentialFnForTest` seams, `testfs.GetTestFs` (NEVER `afero.NewOsFs()` in query tests), `t.Setenv` for env cases.
- REQ-NF-004: Branch off `origin/main` (local `main` is behind; the #211 fix `7a675369` is required as the base).

## Technical Considerations

- Implementation point: in `resolveConn`'s `--credential` branch, apply a single database-override step to the conn returned by each of the three paths:
  ```go
  // flag (even explicit empty) > NEO4J_DATABASE env > credential-supplied
  if f := cmd.Flag("database"); f != nil && f.Changed {
      c.database = f.Value.String()
  } else if v := os.Getenv(envDatabase); v != "" {
      c.database = v
  }
  ```
  Small helper func or inline after each path.
- An empty `c.database` leaves `SessionConfig.DatabaseName` unset → server resolves home DB (post-#211 semantics; see `runStatementsResponseImpl`).
- Files touched: `neo4j-cli/query/connect.go`, `neo4j-cli/query/query.go`, `neo4j-cli/query/connect_test.go`, `neo4j-cli/query/desktop_test.go`, regenerated `neo4j-cli/internal/skill/bundle/references/query.md`, new `.changes/unreleased/` entry.

## Acceptance Criteria

- [ ] `--credential desktop --database movies` → `conn.database == "movies"` (test in `desktop_test.go`).
- [ ] `NEO4J_DATABASE=foo` env + `--credential desktop` → `conn.database == "foo"`; flag beats env; no override → `""` (home).
- [ ] Persisted cred: `--database` overrides stored `DatabaseName`; env overrides stored; no override keeps stored (tests in `connect_test.go`).
- [ ] `--uri`/`--username`/`--password` still rejected alongside `--credential`; `--database` no longer listed in the conflict error.
- [ ] `make test`, `make fmt-check`, `make lint` all pass (final gates).
- [ ] Bundle regenerated and committed; changelog entry present.

## Out of Scope

- Persisting a database name for desktop credentials.
- `NEO4J_DATABASE` via dotenv in the credential path.
- PR creation (`/hone:pr` owns that; PR should be titled `CLI-212: ...` with body `Fixes neo4j-labs/neo4j-cli#212`).

## Open Questions

None — all design decisions pre-confirmed with user (scope: all credential forms; env: flag + OS env only; precedence: flag > env > stored > empty).
