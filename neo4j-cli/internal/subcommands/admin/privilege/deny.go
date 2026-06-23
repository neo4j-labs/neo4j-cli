// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newDenyCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var roleName, action string
	var opts privilegeOpts

	cmd := &cobra.Command{
		Use:         "deny",
		Short:       "Deny a privilege to a role",
		Annotations: map[string]string{"write": "true"},
		Long: "Deny a privilege to a role via DENY <privilege> TO <role> against the system database. " +
			"The action is supplied with --action (case-insensitive, underscores tolerated). " +
			"Resource scope is set with --on-graph, --on-database, or --on-dbms (mutually exclusive); " +
			"graph privileges may be qualified with --node-label, --relationship-type, or --property. " +
			"WRITE and ALL GRAPH PRIVILEGES accept no qualifiers. " +
			"--cidr scopes a LOAD privilege to a CIDR range (LOAD only). " +
			"After the deny, the role's updated privileges are printed.",
		Example: `# Deny WRITE on all graphs to the readonly role
neo4j-cli admin privilege deny --action write --on-graph * --role readonly --credential local --rw

# Deny CREATE ROLE (a DBMS privilege) to the analyst role, output as JSON
neo4j-cli admin privilege deny --action create_role --on-dbms --role analyst --credential local --rw --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrivilegeMutation(cmd, cfg, *conn, "DENY", action, roleName, opts, "TO")
		},
	}

	addPrivilegeFlags(cmd, &action, &roleName, &opts, "deny")

	return cmd
}
