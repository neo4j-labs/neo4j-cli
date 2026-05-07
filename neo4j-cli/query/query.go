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
		Short: "Run Cypher against a Neo4j database via the Bolt protocol",
		Long: "Run a Cypher statement against a Neo4j database via the Bolt " +
			"protocol. Cypher is taken from the positional argument, or from " +
			"stdin when no argument is provided and stdin is piped.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQuery(cmd, args, cfg)
		},
	}

	cmd.PersistentFlags().String("uri", "", "Neo4j Bolt URI [env: NEO4J_URI]. http://<host>[:p][/...] is auto-rewritten to neo4j://<host>:7687; https://<host>[:p][/...] is auto-rewritten to neo4j+s://<host>:7687. (default \"neo4j://localhost:7687\")")
	cmd.PersistentFlags().StringP("username", "u", "", "Neo4j username [env: NEO4J_USERNAME] (default \"neo4j\")")
	cmd.PersistentFlags().StringP("password", "p", "", "Neo4j password [env: NEO4J_PASSWORD]; prompted on TTY if unset")
	cmd.PersistentFlags().StringP("database", "d", "", "Target database name [env: NEO4J_DATABASE] (default \"neo4j\")")
	cmd.PersistentFlags().String("env", "", "Path to a .env file (auto-discovered by walking up from cwd if unset)")
	cmd.PersistentFlags().StringArray("param", nil, "Query parameter as key=value (repeatable); JSON-typed when value parses as JSON, otherwise treated as a string")
	cmd.PersistentFlags().Int("max-rows", 100, "Maximum rows to print (0 = unlimited); when capped, prints a stderr warning and sets truncated=true in JSON")
	cmd.PersistentFlags().Int("truncate-arrays-over", 100, "Recursively truncate any array longer than N inside row values (0 = off); rendered as [\"<truncated: K items>\"]")
	cmd.PersistentFlags().Bool("insecure", false, "Skip TLS certificate verification [env: NEO4J_INSECURE] (development only)")
	cmd.PersistentFlags().StringP("credential", "c", "", "Name of a stored dbms credential to use for the connection (see 'neo4j-cli credential dbms list')")

	flags.RegisterOutputFlag(cmd, cfg)

	cmd.AddCommand(newSchemaCmd(cfg))

	return cmd
}
