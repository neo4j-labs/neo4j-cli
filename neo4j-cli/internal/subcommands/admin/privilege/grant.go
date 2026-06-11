// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newGrantCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return newPrivilegeMutationCmd(
		cfg, conn,
		"GRANT",
		"Grant a privilege to a role (Enterprise only)",
		"Grant a privilege to a role in the system database. "+
			"Executes GRANT <action> ON <resource> TO <role>. "+
			"Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error. "+
			"The --action flag accepts Neo4j privilege keywords (case-insensitive; use _ or space as word separator). "+
			"Resource target is controlled by --on-graph (default), --on-database, or --on-dbms (mutually exclusive). "+
			"Graph-scoped qualifiers --node-label, --relationship-type, and --property are only valid with --on-graph.",
		`# Grant READ on all graphs to the analyst role
neo4j-cli admin privilege grant --action read --on-graph '*' --role analyst --credential local --rw

# Grant ACCESS on a specific database to a role
neo4j-cli admin privilege grant --action access --on-database neo4j --role analyst --credential local --rw

# Grant a DBMS-level privilege to a role
neo4j-cli admin privilege grant --action create_role --on-dbms --role admin --credential local --rw

# Grant READ on specific nodes and property to a role
neo4j-cli admin privilege grant --action read --on-graph neo4j --node-label Person --property name --role analyst --credential local --rw`,
	)
}
