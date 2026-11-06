# neo4j-cli relate

## Contents

- [neo4j-cli relate create](#neo4j-cli-relate-create)
- [neo4j-cli relate delete](#neo4j-cli-relate-delete)
- [neo4j-cli relate info](#neo4j-cli-relate-info)
- [neo4j-cli relate list](#neo4j-cli-relate-list)
- [neo4j-cli relate pull](#neo4j-cli-relate-pull)
- [neo4j-cli relate push](#neo4j-cli-relate-push)

Manage local embedded Relate graph stores

Manage local Relate graph stores — the embedded openCypher engine (formerly CypherLite). A store is a directory (or .rl bundle) holding one graph database; no server, ports or credentials are involved. Stores are created under the CLI's managed directory when you pass a bare name, or at any path you give. Query a store with `neo4j-cli query "..." --relate <name-or-path>` (writes need --rw). `create` and `info` need the engine compiled in (build with -tags relate); `list` and `delete` work in every build.

Usage: `neo4j-cli relate`

Examples:

```
# Create a store under the managed directory, then query it
neo4j-cli relate create movies
neo4j-cli query "CREATE (m:Movie {title: 'The Matrix'}) RETURN m" --relate movies --rw

# Create a store at an explicit path
neo4j-cli relate create ./mydb

# List managed stores; inspect one (engine, counts, labels)
neo4j-cli relate list
neo4j-cli relate info movies

# Delete a store (destructive: --yes --force required when scripted)
neo4j-cli relate delete movies --yes --force
```

## neo4j-cli relate create

Create a local Relate graph store

Create an empty local Relate graph store. A bare name is created under the CLI's managed store directory (see `relate list`); a path is used verbatim. The store is created empty — populate and query it with `neo4j-cli query --relate <name-or-path>` (writes need --rw). An existing store is refused unless --force, which keeps the existing data untouched. A path that exists but is neither a store nor an empty directory is always refused. Requires a build with the Relate engine (-tags relate).

Usage: `neo4j-cli relate create <name-or-path> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Keep an existing store instead of refusing (its data is untouched) |

Examples:

```
# Create a store under the managed directory
neo4j-cli relate create movies

# Create a store at an explicit path
neo4j-cli relate create ./mydb

# Accept an existing store instead of refusing
neo4j-cli relate create movies --force

# Then populate it
neo4j-cli query "CREATE (:Movie {title: 'The Matrix'})" --relate movies --rw
```

## neo4j-cli relate delete

Delete a local Relate graph store and all its data

Delete a local Relate graph store — the whole store directory (or .rl bundle) and every graph it holds; this cannot be undone. Only paths that are Relate stores can be deleted; anything else is refused. Destructive: requires `--yes --force` (or a `y` answer at the TTY prompt) when invoked non-interactively. A store currently open by another process (e.g. a running `relate serve`) keeps its lock files; removal still proceeds on platforms that allow it.

Usage: `neo4j-cli relate delete <name-or-path> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Delete a managed store; prompts on a TTY
neo4j-cli relate delete movies

# Skip the prompt (required for scripts / non-TTY callers)
neo4j-cli relate delete movies --yes --force

# Delete a store at an explicit path
neo4j-cli relate delete ./mydb --yes --force
```

## neo4j-cli relate info

Show a Relate graph store's engine, counts and labels

Inspect a local Relate graph store: engine name and version (CALL dbms.components), node and relationship counts, labels, relationship types, index and constraint counts, and the store's on-disk size and last modification. The store is opened read-only — a missing store is an error, never created. Works with .rl bundles and directories alike. Requires a build with the Relate engine (-tags relate).

Usage: `neo4j-cli relate info <name-or-path>`

Examples:

```
# Inspect a managed store
neo4j-cli relate info movies

# Inspect a store at an explicit path, JSON for scripting
neo4j-cli relate info ./mydb --format json
```

## neo4j-cli relate list

List Relate graph stores under the managed directory

List every Relate graph store under the CLI's managed store directory, one row per store: name (the bare name `query --relate <name>` resolves), kind (directory or .rl bundle), size in bytes, and last modified. Stores created at explicit paths are not listed. An empty result renders as an empty table or empty JSON array (exit 0).

Usage: `neo4j-cli relate list`

Examples:

```
# List managed stores as a table
neo4j-cli relate list

# Emit JSON for scripting
neo4j-cli relate list --format json
```

## neo4j-cli relate pull

Pull a subgraph from a remote Neo4j into a local Relate store

Pull a subgraph from a remote Neo4j database into a local embedded Relate store, over Bolt. The remote is selected with the same connection flags as `query` (--uri, --username, --password, --database for the remote database name, --credential, --env); the first argument is the LOCAL store (a bare name under the managed directory, or a path — created when missing). --query is the remote Cypher; it must return graph entities — nodes, relationships or paths (e.g. `MATCH (n:Person) RETURN n` or `MATCH (a)-[r:KNOWS]->(b) RETURN a, r, b`); scalar-only results are refused. --param binds remote query parameters (key=value, JSON-typed). Pulled entities are upserted by source identity: a second pull from the same source updates the entities in place instead of duplicating them (properties are replaced wholesale). Relationships whose endpoints are not part of the pulled result are skipped and reported. --batch-size bounds the rows per write statement (default 500; property values are inlined as literals, so smaller batches mean smaller statements). The store is checkpointed after a successful pull. Requires a build with the Relate engine (-tags relate).

Usage: `neo4j-cli relate pull <name-or-path> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--batch-size` | int | 500 | Rows per write statement into the local store (property values are inlined as literals, so smaller batches mean smaller statements) |
| `-c, --credential` | string | - | Credential to use for the remote connection. Forms: 'desktop' (the single running Neo4j Desktop 2 DBMS), 'desktop-connection:<uuid>', or '<name>' (a persisted dbms credential; see 'neo4j-cli credential dbms list') |
| `-d, --database` | string | - | Remote database name; defaults to the connecting user's home database when unset |
| `--env` | string | - | Path to a .env file (auto-discovered by walking up from cwd if unset) |
| `--param` | stringArray | [] | Remote query parameter as key=value (repeatable); JSON-typed when value parses as JSON, otherwise a string |
| `-p, --password` | string | - | Neo4j password of the remote; prompted on TTY if unset |
| `--query` | string | - | Remote Cypher to pull; must return nodes, relationships or paths (not scalars) |
| `--uri` | string | - | Neo4j Bolt URI of the remote to pull from. http://<host>[:p][/...] is auto-rewritten to neo4j://<host>:7687; https://<host>[:p][/...] is auto-rewritten to neo4j+s://<host>:7687. (default "neo4j://localhost:7687") |
| `-u, --username` | string | - | Neo4j username of the remote (default "neo4j") |

Examples:

```
# Pull the Person subgraph from a local Neo4j into a managed store
neo4j-cli relate pull people --rw --uri neo4j://localhost:7687 -u neo4j -p pw \\
  --query "MATCH (n:Person) RETURN n"

# Pull a labeled neighborhood with relationships, using a stored credential
neo4j-cli relate pull movies --rw --credential local \\
  --query "MATCH (a:Person)-[r:ACTED_IN]->(m:Movie) RETURN a, r, m"

# Pull with parameters and a smaller write batch
neo4j-cli relate pull recent --rw --uri neo4j://localhost:7687 -u neo4j -p pw \\
  --query "MATCH (n:Event) WHERE n.date > $since RETURN n" --param since=\"2026-01-01\" --batch-size 100
```

## neo4j-cli relate push

Push a local Relate store's subgraph into a remote Neo4j

Push a local embedded Relate store's subgraph into a remote Neo4j database, over Bolt. The remote is selected with the same connection flags as `query`; the first argument is the LOCAL store. Nodes MERGE on their label's identity key: the remote's single-property uniqueness constraint (SHOW CONSTRAINTS) or an explicit `--merge-key Label.property` override (repeatable). A label in scope with no key is refused — create a uniqueness constraint on the remote or pass --merge-key. Relationships MATCH their endpoints by those keys and MERGE on the (start, type, end) pattern — pass `--rel-key property` to key the MERGE on a relationship property instead (for multiple rels of one type between the same pair). Properties are replaced wholesale; the push is additive: nothing is deleted. Scope with --labels and --types (comma-separated; default: the whole store). --batch-size bounds rows per remote statement (default 500). --dry-run prints the plan — labels, keys, counts, batches — without touching the remote. Requires a build with the Relate engine (-tags relate).

Usage: `neo4j-cli relate push <name-or-path> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--batch-size` | int | 500 | Rows per remote write statement |
| `-c, --credential` | string | - | Credential to use for the remote connection. Forms: 'desktop' (the single running Neo4j Desktop 2 DBMS), 'desktop-connection:<uuid>', or '<name>' (a persisted dbms credential; see 'neo4j-cli credential dbms list') |
| `-d, --database` | string | - | Remote database name; defaults to the connecting user's home database when unset |
| `--dry-run` | bool | false | Print the push plan (labels, keys, counts, batches) without touching the remote |
| `--env` | string | - | Path to a .env file (auto-discovered by walking up from cwd if unset) |
| `--labels` | stringArray | [] | Comma-separated node labels to push (default: every label in the store) |
| `--merge-key` | stringArray | [] | Identity key override as Label.property (repeatable); default is the remote's uniqueness constraint per label |
| `-p, --password` | string | - | Neo4j password of the remote; prompted on TTY if unset |
| `--rel-key` | string | - | Relationship property to key the remote MERGE on (for multiple rels of one type between the same pair) |
| `--types` | stringArray | [] | Comma-separated relationship types to push (default: every type in the store) |
| `--uri` | string | - | Neo4j Bolt URI of the remote to push to. http://<host>[:p][/...] is auto-rewritten to neo4j://<host>:7687; https://<host>[:p][/...] is auto-rewritten to neo4j+s://<host>:7687. (default "neo4j://localhost:7687") |
| `-u, --username` | string | - | Neo4j username of the remote (default "neo4j") |

Examples:

```
# Push the whole store into a local Neo4j (nodes MERGE on remote unique constraints)
neo4j-cli relate push movies --rw --uri neo4j://localhost:7687 -u neo4j -p pw

# Push one label with an explicit merge key
neo4j-cli relate push movies --rw --uri neo4j://localhost:7687 -u neo4j -p pw \\
  --labels Person --merge-key Person.email

# Preview what a push would do
neo4j-cli relate push movies --rw --uri neo4j://localhost:7687 -u neo4j -p pw --dry-run

# Push only some labels and relationship types, keyed rels
neo4j-cli relate push movies --rw --credential local \\
  --labels Person,Movie --types ACTED_IN --rel-key roles
```

