# Query usage guidance

Required pre-reading before using `neo4j-cli query`. Covers the schema-first workflow, parameter handling, embeddings, and Cypher 25 vs Cypher 5 syntax so generated Cypher hits real labels, relationships, and properties on the first try.

## Schema-first workflow

**Before generating ANY Cypher yourself, ALWAYS run `:schema` first** to understand the database structure. Do not guess label names, relationship types, or property names — get them from the schema.

```bash
neo4j-cli query :schema -f toon
```

If the user supplies a Cypher query directly, just execute it — no need to fetch schema first.

Use the schema output to:

1. Know exactly which labels exist (don't guess `User` when it's `Person`).
2. Know property names and types (don't guess `username` when it's `name`).
3. Know relationship types and directions (don't guess `EMPLOYED_BY` when it's `WORKS_AT`, and know it goes `Person → Company` not the reverse).
4. Pick the right Cypher dialect via `database.default_language` (see "Cypher 25 vs Cypher 5 syntax" below).

## What `:schema` returns

`neo4j-cli query :schema` returns a single structured payload (TOON by default, JSON via `-f json`, table via `-f table`) with these top-level fields:

- **`database`** — optional metadata: `name`, `versions[]` (e.g. `["5.26.0"]`), `edition` (`community`/`enterprise`), `default_language` (`CYPHER 5` or `CYPHER 25`). Missing if the server / role can't run `dbms.components()` or `SHOW SETTINGS`.
- **`nodes[]`** — one row per (node-label-set, property) pair. Fields: `nodeType`, `nodeLabels[]`, `propertyName`, `propertyTypes[]`, `mandatory`. Flat shape: a single label with three properties appears as three rows.
- **`relationships[]`** — one row per (relType, property) pair. Fields: `relType`, `propertyName`, `propertyTypes[]`, `mandatory`. Flat, same as `nodes`.
- **`relationship_paths[]`** — one row per distinct `(from-labels)-[:relType]->(to-labels)` shape. Fields: `relType`, `from[]`, `to[]`. Use this to confirm direction and endpoint labels.
- **`indexes[]`** — every index. Fields: `name`, `type`, `entityType`, `labelsOrTypes`, `properties`, `state`, `owningConstraint`, `options`.
- **`constraints[]`** — every constraint. Fields: `name`, `type`, `entityType`, `labelsOrTypes`, `properties`, `ownedIndex`, `propertyType`.

`database.default_language` is the hint for Cypher dialect selection — use it to pick version-specific syntax (e.g. vector-index queries differ between Cypher 5 and Cypher 25).

## Running queries

Run Cypher with the positional argument:

```bash
neo4j-cli query "MATCH (n:Person) RETURN n.name, n.age LIMIT 10" -f toon
```

Pipe from stdin when no positional is supplied:

```bash
echo "MATCH (n) RETURN labels(n), count(*)" | neo4j-cli query -f toon
```

Use a stored dbms credential to avoid passing connection details every time:

```bash
neo4j-cli query --credential prod "MATCH (n:Person) RETURN count(n)" -f toon
```

Pass query parameters with repeatable `--param`:

```bash
neo4j-cli query --param name=Alice --param age=30 \
  'MATCH (n:Person {name: $name, age: $age}) RETURN n' -f toon
```

Always pass `-f toon` on read commands — TOON is ~40% smaller than JSON for the same data, so it's the agent-friendly default. Switch to `-f json` only when piping into a JSON-aware tool.

## Handling user requests

- **User supplies a Cypher query as `$ARGUMENTS`** — run it directly. Do NOT fetch schema first.
- **User asks a data question without Cypher** (e.g. "how many people work at Acme?") — run `neo4j-cli query :schema -f toon` first, use the schema to write the correct Cypher, then run the query.
- **User asks to write / modify / delete** — surface the `--rw` requirement and ASK before retrying. Do not add `--rw` on your own. `neo4j-cli query` runs an `EXPLAIN` preflight over Bolt to detect write Cypher and blocks the statement unless `--rw` is set.

## Parameters

Use `--param NAME=VALUE` (repeatable). Values that parse as JSON are bound with that JSON type; everything else is bound as a string:

```bash
# string param
neo4j-cli query --param name=Alice 'MATCH (n:Person {name:$name}) RETURN n' -f toon

# JSON-typed params (number, array, bool)
neo4j-cli query --param limit=10 --param tags='["sci-fi","ai"]' --param strict=true \
  'MATCH (m:Movie) WHERE any(t IN m.tags WHERE t IN $tags) RETURN m LIMIT $limit' -f toon
```

Prefer `--param` over string interpolation: it keeps the query plan cacheable and avoids quoting bugs.

The `--param NAME:embed=<text>` modifier replaces the text with an embedding vector before the query runs (see "Embeddings & vector search").

## Embeddings & vector search

Embedding support is **opt-in** — a provider must be configured before `:embed` modifiers or the `:embed` leaf will error.

Config resolves with the same precedence as connection config: flag (`--embed-provider`, `--embed-model`, `--embed-base-url`, `--embed-dimensions`, `--embed-credential`) > OS env (`NEO4J_EMBED_*`) > stored embed credential. API keys layer per provider: `OPENAI_API_KEY` / `HF_TOKEN` beats `NEO4J_EMBED_API_KEY` beats the stored credential. Ollama needs no API key.

Inline embedding inside a query — `:embed` modifier on `--param`:

```bash
neo4j-cli query \
  --param q:embed='science fiction movies about AI' \
  "CALL db.index.vector.queryNodes('movie_embeddings', 5, \$q)
   YIELD node, score RETURN node.title AS title, score" -f toon
```

Other `--param` values keep normal type coercion, so mix freely:

```bash
neo4j-cli query \
  --param k=5 \
  --param q:embed='sci-fi movies' \
  "CALL db.index.vector.queryNodes('movie_embeddings', \$k, \$q)
   YIELD node, score RETURN node.title, score" -f toon
```

Standalone preview without opening a Bolt connection — `:embed` leaf:

```bash
neo4j-cli query :embed "hello world" -f json
```

Useful for verifying provider config and the vector dimensions before running a vector query.

## Cypher 25 vs Cypher 5 syntax

Vector-index querying differs by Cypher version:

- **Cypher 5** (Neo4j 5.x):
  ```cypher
  CALL db.index.vector.queryNodes('name', k, $q) YIELD node, score
  ```
- **Cypher 25** (Neo4j 2025.x+, preferred):
  ```cypher
  SEARCH n IN (VECTOR INDEX name FOR n.embedding LIMIT k) SCORE AS score
  ```

Cypher 25 also supports multi-label vector indexes (`FOR (n:Movie|Actor) ON n.embedding`) and filterable `WITH` properties. The `CALL db.index.vector.queryNodes` form still works in Cypher 25 but is deprecated.

Read `database.default_language` from `:schema` to pick the right dialect. If unsure which version the DB supports, start with the Cypher 5 form — it works everywhere.

## Tips

- Run `neo4j-cli query :schema -f toon` before generating Cypher — never assume you know the schema.
- Default to `-f toon` on every read command; switch to `-f json` only when piping into a JSON-aware tool, and `-f table` only when the user asked for a human-readable table.
- Use `LIMIT` for exploratory queries to avoid pulling large result sets.
- Use `--param` for dynamic values instead of string interpolation — keeps the query plan cacheable and avoids quoting bugs.
- Relationship directions matter: check `relationship_paths.from` → `to` in the schema output before writing `MATCH`.
- Property types in `nodes[].propertyTypes` drive comparison choices (string vs numeric vs date).
- `--truncate-arrays-over N` recursively replaces arrays longer than N items (default 100, 0 disables). Useful for hiding embedding vectors from row output. TOON renders truncated arrays as `["<truncated: K items>"]`.
- `--max-rows N` caps the number of rows printed (default 100, 0 = unlimited). When capped, the CLI prints a stderr warning and sets `truncated=true` in JSON output.
