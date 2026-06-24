// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newGrantCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "grant",
		Short:       "Grant a privilege to a role",
		Annotations: map[string]string{"write": "true"},
		Long: "Grant a privilege to a role via GRANT <privilege> TO <role> against the system database. " +
			"The action is a positional argument on a per-category subcommand (property, entity, graph, " +
			"label, load, database, dbms); run `grant <category> --help` to see its actions and flags. " +
			"After the grant, the role's updated privileges are printed.",
		Example: `# Grant READ on all graphs to the analyst role
neo4j-cli admin privilege grant property read --on-graph * --role analyst --credential local --rw

# Grant CREATE ROLE (a DBMS privilege) to the admin role, output as JSON
neo4j-cli admin privilege grant dbms create-role --on-dbms --role admin --credential local --rw --format json`,
	}

	for _, cat := range categoryOrder {
		cmd.AddCommand(newCategoryCmd(cfg, conn, "GRANT", cat))
	}

	return cmd
}
