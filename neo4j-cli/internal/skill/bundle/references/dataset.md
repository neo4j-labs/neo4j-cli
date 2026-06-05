# neo4j-cli dataset

Discover example Neo4j datasets

Discover example Neo4j datasets you can load into a database.

`dataset list` prints a curated set of suggestions. Datasets are addressed by their GitHub `<owner>/<repo>` (e.g. neo4j-graph-examples/movies), and any repo carrying a relate.project-install.json manifest works — the suggestions are not a constraint.

Loading happens on each target's own command tree:
  neo4j-cli docker load <owner/repo>          — load into a local Docker container
  neo4j-cli desktop dbms load <owner/repo>    — load into a Neo4j Desktop DBMS
  neo4j-cli aura instance load <owner/repo>   — load into a new Aura instance

Usage: `neo4j-cli dataset`

Examples:

```
# Load the movies dataset into a local Docker container
neo4j-cli docker load neo4j-graph-examples/movies --name movies --rw

# ... into a Neo4j Desktop DBMS
neo4j-cli desktop dbms load neo4j-graph-examples/movies --name movies --password <pw> --rw

# ... into a new Aura instance
neo4j-cli aura instance load neo4j-graph-examples/movies --name movies --type free-db --rw
```

## neo4j-cli dataset list

List curated example dataset suggestions

List a curated set of suggested example datasets, each with a slug, title, description, and GitHub `<owner>/<repo>`. Pass a repo to one of the load verbs (`docker load`, `desktop dbms load`, `aura instance load`). The suggestions are not a constraint — any repo carrying a relate.project-install.json manifest works.

Usage: `neo4j-cli dataset list`

Examples:

```
# List dataset suggestions as a table
neo4j-cli dataset list

# Emit JSON for scripting (e.g. piping into jq)
neo4j-cli dataset list --format json

# Then load a suggested dataset into a local Docker container
neo4j-cli docker load neo4j-graph-examples/movies --name movies --rw
```

