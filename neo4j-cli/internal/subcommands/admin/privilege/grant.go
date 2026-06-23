// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newGrantCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var roleName, action string
	var opts privilegeOpts

	cmd := &cobra.Command{
		Use:         "grant",
		Short:       "Grant a privilege to a role",
		Annotations: map[string]string{"write": "true"},
		Long: "Grant a privilege to a role via GRANT <privilege> TO <role> against the system database. " +
			"The action is supplied with --action (case-insensitive, underscores tolerated). " +
			"Resource scope is set with --on-graph, --on-database, or --on-dbms (mutually exclusive); " +
			"graph privileges may be qualified with --node-label, --relationship-type, or --property. " +
			"WRITE and ALL GRAPH PRIVILEGES accept no qualifiers. " +
			"--cidr scopes a LOAD privilege to a CIDR range (LOAD only). " +
			"After the grant, the role's updated privileges are printed.",
		Example: `# Grant READ on all graphs to the analyst role
neo4j-cli admin privilege grant --action read --on-graph * --role analyst --credential local --rw

# Grant CREATE ROLE (a DBMS privilege) to the admin role, output as JSON
neo4j-cli admin privilege grant --action create_role --on-dbms --role admin --credential local --rw --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrivilegeMutation(cmd, cfg, *conn, "GRANT", action, roleName, opts, "TO")
		},
	}

	addPrivilegeFlags(cmd, &action, &roleName, &opts, "grant")

	return cmd
}
