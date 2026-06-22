// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package query implements the `neo4j-cli query` command tree, which executes
// Cypher against a Neo4j database via the Bolt protocol.
package query

import (
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
)

// NewCmd returns the `query` parent cobra command, with all persistent flags
// registered and the `:schema` subcommand mounted. The parent's RunE runs an
// ad-hoc Cypher statement against the connected Neo4j database.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query [cypher]",
		Short: "Run Cypher, inspect the database schema (:schema), lint Cypher offline (:lint), and embed text against a Neo4j database via the Bolt protocol",
		Long: "Use the :schema subcommand to introspect labels, relationship types, and properties before writing Cypher — never guess the schema. " +
			"Run a Cypher statement against a Neo4j database via the Bolt " +
			"protocol. Cypher is taken from the positional argument, or from " +
			"stdin when no argument is provided and stdin is piped. " +
			"Use `--param NAME:embed=<text>` to inject an embedding vector inline " +
			"(text is sent to the configured embedding provider, the resulting vector " +
			"is bound to $NAME for both EXPLAIN preflight and the real run). " +
			"The sibling `query :embed [text]` leaf computes a vector standalone " +
			"without opening a Bolt connection. " +
			"The sibling `query :lint [cypher]` leaf checks Cypher for syntax and " +
			"semantic errors offline, also without opening a Bolt connection. " +
			"Multiple statements may be passed in a single string: they are split on " +
			"a `;` at the end of a line (a mid-line `;` is kept verbatim; the " +
			"terminating `;` is stripped). By default each statement runs in its own " +
			"transaction, in order, failing fast on the first error; pass `--atomic` " +
			"to run them all in one transaction that rolls back if any statement " +
			"fails, or `--continue-on-error` (non-atomic only) to report each failure " +
			"and keep going, exiting non-zero at the end. " +
			"Multiple result sets render as a JSON array with `--format json` " +
			"or as stacked blocks with `--format table`/`toon`. " +
			"Write operations require `--rw`; without `--rw`, an EXPLAIN preflight " +
			"runs first and statements classified as writes are blocked.",
		Example: `# Introspect the schema before writing Cypher (always do this first)
neo4j-cli query :schema --format toon

# Run inline Cypher (read-only — no --rw needed)
neo4j-cli query "MATCH (n) RETURN count(n) AS n" --format json

# Pipe Cypher from stdin
echo "MATCH (n) RETURN n LIMIT 5" | neo4j-cli query --format json

# Pass typed parameters with --param (repeatable; JSON values are auto-typed)
neo4j-cli query "MATCH (p:Person {name: $name}) RETURN p" --param name=Alice --format json

# Embed text inline as a vector parameter via the :embed modifier
neo4j-cli query "CALL db.index.vector.queryNodes('idx', 5, $v) YIELD node RETURN node" --param v:embed="hello world" --format json

# Route to the single running Neo4j Desktop 2 DBMS at runtime (no persisted credential)
neo4j-cli query "MATCH (n) RETURN count(n)" --credential desktop --format json

# Route to a saved Neo4j Desktop 2 remote connection by uuid (see 'neo4j-cli desktop list')
neo4j-cli query "MATCH (n) RETURN count(n)" --credential desktop-connection:f4e2f3c0-1111-2222-3333-444455556666 --format json

# Target a specific database on the running Neo4j Desktop 2 DBMS
neo4j-cli query "MATCH (n) RETURN count(n)" --credential desktop --database movies --format json

# Use a persisted dbms credential by name (see 'neo4j-cli credential dbms list')
neo4j-cli query "MATCH (n) RETURN count(n)" --credential local --format json

# Write Cypher requires --rw (opt-in)
neo4j-cli query "CREATE (n:Person {name: \"Alice\"}) RETURN n" --rw --format json

# Run multiple read statements in one call (split on ; at end of line); results render as a JSON array
neo4j-cli query "MATCH (n:Person) RETURN count(n) AS people; MATCH (m:Movie) RETURN count(m) AS movies" --format json

# Run multiple write statements atomically — all in one transaction, rolled back if any fails
neo4j-cli query "CREATE (:Person {name: \"Alice\"}); CREATE (:Person {name: \"Bob\"})" --rw --atomic --format json

# Import many write statements, skipping over any that fail (reports each failure, exits non-zero)
neo4j-cli query "CREATE (:Person {name: \"Alice\"}); CREATE (:Person {name: \"Bob\"})" --rw --continue-on-error --format json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuery(cmd, args, cfg)
		},
	}

	cmd.PersistentFlags().String("uri", "", "Neo4j Bolt URI [env: NEO4J_URI]. http://<host>[:p][/...] is auto-rewritten to neo4j://<host>:7687; https://<host>[:p][/...] is auto-rewritten to neo4j+s://<host>:7687. (default \"neo4j://localhost:7687\")")
	cmd.PersistentFlags().StringP("username", "u", "", "Neo4j username [env: NEO4J_USERNAME] (default \"neo4j\")")
	cmd.PersistentFlags().StringP("password", "p", "", "Neo4j password [env: NEO4J_PASSWORD]; prompted on TTY if unset")
	cmd.PersistentFlags().StringP("database", "d", "", "Target database name; defaults to the connecting user's home database when unset - typically \"neo4j\", but can vary by deployment (e.g. the instance DBID on Aura Free). Also applies with --credential, overriding the credential-supplied database [env: NEO4J_DATABASE]")
	cmd.PersistentFlags().String("env", "", "Path to a .env file (auto-discovered by walking up from cwd if unset)")
	cmd.PersistentFlags().StringArray("param", nil, "Query parameter as key=value (repeatable); JSON-typed when value parses as JSON, otherwise treated as a string. Use `key:embed=<text>` to embed text via the configured provider and bind the resulting vector to $key (see `query :embed`).")
	cmd.PersistentFlags().Int("max-rows", 100, "Maximum rows to print (0 = unlimited); when capped, prints a stderr warning and sets truncated=true in JSON")
	cmd.PersistentFlags().Int("truncate-arrays-over", 100, "Recursively truncate any array longer than N inside row values (0 = off); rendered as [\"<truncated: K items>\"]")
	cmd.PersistentFlags().StringP("credential", "c", "", "Credential to use for the connection. Forms: 'desktop' (the single running Neo4j Desktop 2 DBMS), 'desktop-connection:<uuid>' (a saved Neo4j Desktop 2 connection; see 'neo4j-cli desktop list'), or '<name>' (a persisted dbms credential; see 'neo4j-cli credential dbms list'). Combine with --database/NEO4J_DATABASE to target a specific database")
	cmd.PersistentFlags().Bool("atomic", false, "Run all statements in a single transaction; roll back on any failure (default: each statement in its own transaction, fail-fast)")
	cmd.PersistentFlags().Bool("continue-on-error", false, "Keep running after a statement fails: report each failure and execute the rest, then exit non-zero (non-atomic only; mutually exclusive with --atomic)")

	cmd.PersistentFlags().String("embed-credential", "", "Name of a stored embed credential to seed embedding config (see 'neo4j-cli credential embed list')")
	cmd.PersistentFlags().String("embed-provider", "", "Embedding provider: openai | ollama | huggingface | gemini | vertex [env: NEO4J_EMBED_PROVIDER]")
	cmd.PersistentFlags().String("embed-model", "", "Embedding model name [env: NEO4J_EMBED_MODEL]")
	cmd.PersistentFlags().String("embed-base-url", "", "Embedding provider base URL [env: NEO4J_EMBED_BASE_URL]")
	cmd.PersistentFlags().Int("embed-dimensions", 0, "Embedding output dimensions (provider-dependent; ignored by Ollama) [env: NEO4J_EMBED_DIMENSIONS]")

	cmd.PersistentFlags().Bool("debug", false, "Route Neo4j driver activity (connection, auth, routing, retries) to stderr at DEBUG level; stdout is unaffected [env: NEO4J_DEBUG (set to 1 to enable)]")

	flags.RegisterOutputFlag(cmd, cfg)

	cmd.AddCommand(newSchemaCmd(cfg))
	cmd.AddCommand(newEmbedCmd(cfg))
	cmd.AddCommand(newLintCmd(cfg))

	return cmd
}
