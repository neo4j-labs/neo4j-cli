# PRD: Desktop remote connections + prefixed `query -c` dispatch (CLI-32 extension)

Linear: https://linear.app/neo4j/issue/CLI-32/local-neo4j-desktop-management-v10
Branch: `oskar/cli-32-desktop-management`
Extends: [`prd-cli-32-local-desktop-management.md`](prd-cli-32-local-desktop-management.md)

> Builds on the original CLI-32 PRD. Implementation specifics (relate endpoint paths, key namespaces, JWT auth, salt resolution) continue to live in [`cli-32-local-desktop-management-impl.md`](cli-32-local-desktop-management-impl.md).

## Overview

Add a `neo4j-cli desktop connection` subcommand subtree that manages Desktop 2's saved remote DB connections (Aura URIs / remote Neo4j endpoints) via the `/connections` route on Desktop's local relate API, and migrate `neo4j-cli query --credential` to an explicit prefix-based dispatch. `-c desktop` picks the currently-running Desktop DBMS; `-c desktop-connection:<uuid>` picks a specific saved connection; unprefixed values are persisted-store-only (the implicit Desktop fallthrough from the original CLI-32 implementation is removed). The unified `desktop list` grows a Remote connections section alongside the existing Local DBMSes section so users can discover connection UUIDs.

## Goals

- Let users CRUD saved remote DB connections (Aura, remote Neo4j) from the terminal without opening the Desktop GUI.
- Make `neo4j-cli query` connect to ANY saved Desktop connection via an unambiguous prefix, not just the currently-running DBMS.
- Replace the implicit name-match Desktop fallthrough (REQ-F-026 from the original PRD) with an explicit `-c desktop` / `-c desktop-connection:<uuid>` scheme so connections and DBMSes can never collide.
- Surface remote connections in `desktop list` so users discover their UUIDs.
- Keep Desktop as the source of truth for connection state and credentials — no caching in neo4j-cli's `credentials.json`.

## Non-Goals

- Persisting Desktop connection credentials in `credentials.json`. Read-only at query time, same model as REQ-F-025.
- Exposing Desktop's `tags`, `project`, or `metadata` fields on connections. Plain `name / uri / username / password / description` only this round.
- A separate `desktop dbms list` leaf — DBMS lifecycle commands stay flat on `desktop` (existing shape). The unified `desktop list` shows both kinds.
- A separate `desktop connection list` / `desktop connection get` leaf — connection reads go through `desktop list` (mirrors the DBMS side, which also has no `list` / `get` outside the parent's `list`).
- Importing or syncing connections between neo4j-cli's persisted `credential dbms ...` store and Desktop's connection store. Two independent stores.
- Connection lifecycle verbs (start/stop). Saved connections are inert profiles, not running processes.
- Falling back from `-c desktop` to a saved connection when no DBMS is running. Each prefix means exactly one thing.

## Requirements

### Functional Requirements

- REQ-F-101: `neo4j-cli query --credential desktop` resolves to the currently-running Desktop DBMS — filter `GET /dbmss` by `status == "started"`, use the single match. Zero matches → `clierr.NewFatalError("No running DBMS in Neo4j Desktop 2. Start one with 'neo4j-cli desktop start <id>'.")`. Defensive >1 → `clierr.NewFatalError("Multiple running DBMSes reported by Neo4j Desktop 2 (<ids>). Stop all but one, or pick a saved connection with --credential desktop-connection:<id>.")` (relate's design guarantees ≤1; the branch is defensive).
- REQ-F-102: `neo4j-cli query --credential desktop-connection:<uuid>` resolves to a specific saved connection. The value after the colon MUST be a syntactically valid UUID — malformed input rejects with `clierr.NewUsageError("--credential desktop-connection:<id> requires a UUID; got '<raw>'. Run 'neo4j-cli desktop list' to see connection ids.")`. Unknown UUID rejects with `clierr.NewFatalError("Neo4j Desktop 2 has no connection with id <uuid>. Run 'neo4j-cli desktop list' to see saved connections.")`. The connection URI for the Bolt driver comes from `connection.connectionUri`; username/password from `GET /credentials/connection:<uuid>`.
- REQ-F-103: Unprefixed `--credential <name>` resolves against the persisted credential store ONLY. No Desktop fallthrough. The original REQ-F-026 (implicit name-match fallthrough) and REQ-F-027 (zero-config single-DBMS auto-select) are removed. Their tests in `neo4j-cli/query/desktop_test.go` are deleted; a regression test asserts that unprefixed values never touch the Desktop client.
- REQ-F-104: `neo4j-cli desktop list` produces a unified output covering Local DBMSes AND Remote connections. Table format renders two labelled sections (`Local DBMSes`, `Remote connections`), each with its own column set; an empty section prints `(none)`. `--format json` returns `{"dbmss": [...DbmsInfo...], "connections": [...Connection...]}` — BREAKING vs the current `[]DbmsInfo` array shape; the branch hasn't shipped yet, so a clean break is taken and the change is called out in the changelog. `--format toon` mirrors the JSON shape. Both lookups run in parallel; existing concurrent DBMS enrichment is preserved.
- REQ-F-105: `neo4j-cli desktop connection create --name <n> --uri <u> --username <user> [--password <p>] [--description <d>]` creates a connection via `POST /connections`. `--name`, `--uri`, `--username` are required. `--password` is optional on the flag surface but mandatory at runtime: when omitted on a TTY, prompt via stdin; when omitted on a non-TTY, fail with `clierr.NewUsageError("--password is required when stdin is not a TTY")`. On success, render the returned Connection in the requested `--format`. Write command — `Annotations{"write":"true"}`, requires `--rw`. Reuses the REQ-F-028 stdin prompt helper.
- REQ-F-106: `neo4j-cli desktop connection update <id> [--name --uri --username --password --description]` updates a connection via `PATCH /connections/:id`. The `<id>` positional is UUID-only (connection names are user-supplied free text and unreliable as identifiers; mirrors `desktop delete|start|stop <id>`). At least one mutating flag must be supplied; zero mutations → `clierr.NewUsageError("'desktop connection update' requires at least one of --name, --uri, --username, --password, --description")`. The PATCH body contains only the supplied subset (partial update). `--password` accepts an empty flag value to trigger the stdin prompt on a TTY. Write command, requires `--rw`.
- REQ-F-107: `neo4j-cli desktop connection delete <id> [--yes]` deletes a connection via `DELETE /connections/:id`. UUID-only positional. Without `--yes` on a TTY, prompt `Delete connection '<name>' (<id>)? [y/N]`. Without `--yes` on a non-TTY, error. Print a confirmation line on success (mirror task-021 desktop delete shape). Write command, requires `--rw`.
- REQ-F-108: `desktopclient` gains four endpoints and one rename:
  - `ListConnections(ctx) ([]Connection, error)` → `GET /connections`
  - `CreateConnection(ctx, args ConnectionCreateArgs) (*Connection, error)` → `POST /connections`
  - `UpdateConnection(ctx, id string, args ConnectionUpdateArgs) (*Connection, error)` → `PATCH /connections/:id`
  - `DeleteConnection(ctx, id string) (*Connection, error)` → `DELETE /connections/:id`
  - `GetCredentials(ctx, dbmsId)` renamed to `GetCredentialsByKey(ctx, key string)` so the caller passes the full key (`"dbms:<id>"` or `"connection:<id>"`).
- REQ-F-109: `Connection` wire type mirrors relate's `Connection` shape (`id, name, description, tags, project, metadata, connectionUri, createdAt?, manifestPath?`), `omitempty` on optionals. CLI-side `ConnectionCreateArgs` exposes only `{name, connectionUri, username, password, description?}`; `ConnectionUpdateArgs` exposes the same fields as optionals. `tags`, `project`, `metadata` are NOT plumbed through the CLI flag surface this round.
- REQ-F-110: The query resolver gains two functions sharing the existing probe + client setup:
  - `resolveDesktopActiveDbmsCredential` — filter by `status == "started"`, fetch creds via `GetCredentialsByKey("dbms:<id>")`. Null-creds → REQ-F-028 path unchanged.
  - `resolveDesktopConnectionCredential(uuid)` — validate UUID, list connections (no GET-by-id route), match `connection.id == uuid`, fetch creds via `GetCredentialsByKey("connection:<uuid>")`. Null-creds → REQ-F-028 path unchanged.
  Both reject malformed input early. The legacy `resolveDesktopCredential` name-match path is deleted (no longer reachable).
- REQ-F-111: `neo4j-cli/query/query.go` `--credential` flag description is rewritten to document the two prefix forms and the persisted-store-only fallback. Example: `"Credential to use. Forms: 'desktop' (running Desktop DBMS), 'desktop-connection:<uuid>' (saved Desktop connection), or '<name>' (persisted credential from 'credential dbms list')."`
- REQ-F-112: Skill bundle regenerated (`go generate ./neo4j-cli/internal/skill/...`); `TestGenerator_RoundTrip` is the gate. Reference docs updated:
  - `references/query.md` — rewrite the `-c` row in the flag table; add Desktop-prefix examples.
  - `references/credential.md` — short note that the `desktop` and `desktop-connection:` forms are runtime-resolved and not stored in `credentials.json`.
  - `references/desktop.md` — document the `connection` subtree, the unified `desktop list` shape, and the `query -c desktop` / `desktop-connection:` flow.
- REQ-F-113: Every new leaf carries a flush-left `Example:` with ≥3 invocations, `# comment` lines, `--rw` on writes, at least one `--format json` on reads. Enforced by `TestAllLeafCommands_HaveExamples`.
- REQ-F-114: Changelog — three changie entries (`changie new --projects neo4j-cli --kind Minor --body "..."`):
  1. `desktop connection create/update/delete` subtree.
  2. `query --credential` prefix dispatch (`desktop`, `desktop-connection:<uuid>`); removal of implicit Desktop fallthrough and zero-config single-DBMS auto-select.
  3. `desktop list` unified output (BREAKING: JSON shape changed from `[]DbmsInfo` to `{dbmss, connections}`).

### Non-Functional Requirements

- REQ-NF-101: All Desktop API calls hermetic in tests — `httptest.NewServer` for relate, `afero.NewMemMapFs` / `testfs.GetTestFs` for env metadata and salt. No real Desktop, no real filesystem. Reuses the existing `desktopclient` testutils pattern.
- REQ-NF-102: Final gates `make test && make fmt-check && make lint && make license-check && make generate-check` pass on the full CI matrix (Linux, macOS, Windows).
- REQ-NF-103: One-file-per-leaf layout per AGENTS.md. New leaves live under `neo4j-cli/internal/subcommands/desktop/connection/{connection.go, create.go, update.go, delete.go}` with colocated `*_test.go`.
- REQ-NF-104: Re-use, don't duplicate. The new connection resolver shares the probe + client setup with the DBMS resolver via a private helper in `neo4j-cli/query/desktop.go`. The stdin password prompt reuses the REQ-F-028 helper.

## Technical Considerations

- **Prefix parsing seam.** `resolveConn` in `neo4j-cli/query/connect.go` sniffs the `--credential` value first. Two literal prefixes recognised: `desktop` (exact match) and `desktop-connection:` (prefix). Anything else flows into the existing persisted-store code path. The sniff happens BEFORE the persisted-store lookup so `desktop` as a literal persisted credential name becomes inaccessible — this is a deliberate naming claim (called out in the changelog).
- **UUID validation.** Use `github.com/google/uuid.Parse` (already a transitive dep via Aura) on `desktop-connection:<value>` and on the `<id>` positional for `update`/`delete`. Reject early with a usage error.
- **No GET-by-id for connections.** Both the query resolver and the connection leaves must `ListConnections` + filter locally. List size is bounded by what a user has saved (small).
- **PATCH body construction.** Build a `map[string]any` containing only flags the user set (cobra `Changed()` check), serialise to JSON. Avoid `omitempty` on the request struct because empty-string is a valid update for fields like `description`.
- **Breaking JSON shape.** `desktop list --format json` switches from `[]DbmsInfo` to `{dbmss: [], connections: []}`. The branch has not shipped, so no migration shim is added. Documented in the changelog and in `references/desktop.md`.
- **Removal of REQ-F-026 / REQ-F-027.** Existing tests at `neo4j-cli/query/desktop_test.go` covering these branches are deleted in the same commit that removes the code paths. A new regression test asserts that an unprefixed `--credential` value never instantiates the desktop client.
- **Password input.** `--password` accepts both `--password=<value>` (flag-set, value used as-is) and `--password ""` / `--password` with no value followed by a positional → trigger stdin prompt on TTY. Implementation reuses `term.ReadPassword` already wired for REQ-F-028.
- **Connection name as free text.** Connection names are user-supplied with no validation in relate (could be `"Aura - prod / EU-west"` or `"   "`). All identifier-shaped CLI inputs (`-c` prefix, `update`/`delete` positionals) demand the UUID for unambiguity.

## Acceptance Criteria

- [ ] `bin/neo4j-cli query --credential desktop "RETURN 1"` connects to the running Desktop DBMS when exactly one is started.
- [ ] `bin/neo4j-cli query --credential desktop "RETURN 1"` fails with the canonical "no running DBMS" message when zero are started.
- [ ] `bin/neo4j-cli query --credential desktop-connection:<uuid> "RETURN 1"` connects to the saved connection identified by `<uuid>`.
- [ ] `bin/neo4j-cli query --credential desktop-connection:not-a-uuid` rejects with a usage error pointing at `desktop list`.
- [ ] `bin/neo4j-cli query --credential desktop-connection:<unknown-uuid>` rejects with a "no connection with id" error.
- [ ] `bin/neo4j-cli query --credential <random-name>` does NOT call the Desktop client (regression gate for removed implicit fallthrough).
- [ ] `bin/neo4j-cli desktop list` shows two labelled sections; `--format json` returns `{dbmss, connections}` with both keys populated when both exist.
- [ ] `bin/neo4j-cli desktop connection create --name X --uri neo4j+s://... --username neo4j --password P --rw` creates a connection and prints the returned struct.
- [ ] `bin/neo4j-cli desktop connection create --name X --uri ... --username neo4j --rw` on a TTY prompts for password; on a non-TTY errors.
- [ ] `bin/neo4j-cli desktop connection update <uuid> --description "x" --rw` patches only the description.
- [ ] `bin/neo4j-cli desktop connection update <uuid> --rw` (no mutating flag) errors with a usage hint.
- [ ] `bin/neo4j-cli desktop connection delete <uuid> --rw` prompts on a TTY; `--yes` skips the prompt.
- [ ] `make test && make fmt-check && make lint && make license-check && make generate-check` all pass.
- [ ] Skill bundle round-trip test (`TestGenerator_RoundTrip`) passes after `go generate ./neo4j-cli/internal/skill/...`.
- [ ] `TestAllLeafCommands_HaveExamples` passes — every new leaf has a flush-left `Example:`.
- [ ] Three changie entries land under `.changes/unreleased/`.

## Out of Scope

- `tags`, `project`, `metadata` flags on `desktop connection create/update`. Add when there's user demand and corresponding CLI commands for those resources.
- Persistent caching of Desktop connections / credentials in neo4j-cli's local store.
- Auto-start of a Desktop DBMS when `-c desktop` finds zero running.
- Friendly name → UUID resolution in `desktop connection update/delete` and in the `-c desktop-connection:` prefix.
- A `desktop connection list` / `desktop connection get` leaf. Reads go through `desktop list`.
- Bulk operations (delete multiple connections, batch create from a file).
- Connection import/export between Desktop's store and `credential dbms` persisted store.

## Open Questions

(none — all design questions resolved during planning)
