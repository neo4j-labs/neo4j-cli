// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newDenyCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "deny",
		Short:       "Deny a privilege to a role",
		Annotations: map[string]string{"write": "true"},
		Long: "Deny a privilege to a role via DENY <privilege> TO <role> against the system database. " +
			"The action is a positional argument on a per-category subcommand (property, entity, graph, " +
			"label, load, database, dbms); run `deny <category> --help` to see its actions and flags. " +
			"After the deny, the role's updated privileges are printed.",
		Example: `# Deny WRITE on all graphs to the readonly role
neo4j-cli admin privilege deny graph write --on-graph * --role readonly --credential local --rw

# Deny CREATE ROLE (a DBMS privilege) to the analyst role, output as JSON
neo4j-cli admin privilege deny dbms create-role --on-dbms --role analyst --credential local --rw --format json`,
	}

	for _, cat := range categoryOrder {
		cmd.AddCommand(newCategoryCmd(cfg, conn, "DENY", cat))
	}

	return cmd
}
