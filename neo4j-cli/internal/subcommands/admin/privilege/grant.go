// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
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
			"After the grant, the role's updated privileges are printed.",
		Example: `# Grant READ on all graphs to the analyst role
neo4j-cli admin privilege grant --action read --on-graph * --role analyst --credential local --rw

# Grant CREATE ROLE (a DBMS privilege) to the admin role, output as JSON
neo4j-cli admin privilege grant --action create_role --on-dbms --role admin --credential local --rw --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if action == "" {
				return clierr.NewUsageError("--action is required")
			}
			if roleName == "" {
				return clierr.NewUsageError("--role is required")
			}

			cypher, params, err := buildPrivilegeCypher("GRANT", action, opts)
			if err != nil {
				return err
			}
			cmd.SilenceUsage = true

			params["role"] = roleName
			if _, err := privilegeExecFn(cmd.Context(), cfg, *conn, cypher+" TO $role", params); err != nil {
				return err
			}
			return outputPrivileges(cmd, cfg, *conn, roleName)
		},
	}

	cmd.Flags().StringVar(&action, "action", "", "Privilege action to grant (e.g. read, traverse, create_role)")
	cmd.Flags().StringVar(&roleName, "role", "", "Name of the role to grant the privilege to")
	cmd.Flags().StringVar(&opts.onGraph, "on-graph", "", "Scope the privilege to a graph (use * for all)")
	cmd.Flags().StringVar(&opts.onDatabase, "on-database", "", "Scope the privilege to a database (use * for all)")
	cmd.Flags().BoolVar(&opts.onDbms, "on-dbms", false, "Scope the privilege to the DBMS")
	cmd.Flags().StringArrayVar(&opts.nodeLabels, "node-label", nil, "Restrict a graph privilege to node labels")
	cmd.Flags().StringArrayVar(&opts.relTypes, "relationship-type", nil, "Restrict a graph privilege to relationship types")
	cmd.Flags().StringArrayVar(&opts.properties, "property", nil, "Restrict a property privilege to properties")

	return cmd
}
