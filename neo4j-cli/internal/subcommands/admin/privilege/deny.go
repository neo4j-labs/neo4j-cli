// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newDenyCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return newPrivilegeMutationCmd(
		cfg, conn,
		"DENY",
		"Deny a privilege to a role (Enterprise only)",
		"Deny a privilege to a role in the system database. "+
			"Executes DENY <action> ON <resource> TO <role>. "+
			"Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error. "+
			"The --action flag accepts Neo4j privilege keywords (case-insensitive; use _ or space as word separator). "+
			"Resource target is controlled by --on-graph (default), --on-database, or --on-dbms (mutually exclusive). "+
			"Graph-scoped qualifiers --node-label, --relationship-type, and --property are only valid with --on-graph.",
		`# Deny WRITE on all graphs to the readonly role
neo4j-cli admin privilege deny --action write --on-graph '*' --role readonly --credential local --rw

# Deny ACCESS on a specific database to a role
neo4j-cli admin privilege deny --action access --on-database restricted --role readonly --credential local --rw

# Deny a DBMS-level privilege to a role
neo4j-cli admin privilege deny --action create_user --on-dbms --role limited --credential local --rw

# Deny READ on specific nodes to a role
neo4j-cli admin privilege deny --action read --on-graph neo4j --node-label Secret --role analyst --credential local --rw`,
	)
}
