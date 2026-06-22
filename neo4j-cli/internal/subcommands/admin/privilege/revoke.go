// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newRevokeCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var roleName, action, revokeType string
	var opts privilegeOpts

	cmd := &cobra.Command{
		Use:         "revoke",
		Short:       "Revoke a privilege from a role",
		Annotations: map[string]string{"write": "true"},
		Long: "Revoke a privilege from a role via REVOKE <privilege> FROM <role> against the system database. " +
			"The action is supplied with --action (case-insensitive, underscores tolerated). " +
			"Use --revoke-type grant or --revoke-type deny to revoke only a previously granted or denied " +
			"privilege; omit it to revoke both. " +
			"Resource scope is set with --on-graph, --on-database, or --on-dbms (mutually exclusive); " +
			"graph privileges may be qualified with --node-label, --relationship-type, or --property. " +
			"After the revoke, the role's updated privileges are printed.",
		Example: `# Revoke READ on all graphs from the analyst role
neo4j-cli admin privilege revoke --action read --on-graph * --role analyst --credential local --rw

# Revoke only a previously granted READ privilege, output as JSON
neo4j-cli admin privilege revoke --action read --on-graph * --role analyst --revoke-type grant --credential local --rw --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if action == "" {
				return clierr.NewUsageError("--action is required")
			}
			if roleName == "" {
				return clierr.NewUsageError("--role is required")
			}

			verb, err := revokeVerb(revokeType)
			if err != nil {
				return err
			}

			cypher, params, err := buildPrivilegeCypher(verb, action, opts)
			if err != nil {
				return err
			}
			cmd.SilenceUsage = true

			params["role"] = roleName
			if _, err := privilegeExecFn(cmd.Context(), cfg, *conn, cypher+" FROM $role", params); err != nil {
				return err
			}
			return outputPrivileges(cmd, cfg, *conn, roleName)
		},
	}

	cmd.Flags().StringVar(&action, "action", "", "Privilege action to revoke (e.g. read, traverse, create_role)")
	cmd.Flags().StringVar(&roleName, "role", "", "Name of the role to revoke the privilege from")
	cmd.Flags().StringVar(&revokeType, "revoke-type", "", "Restrict the revoke to grant or deny privileges (grant|deny); omit to revoke both")
	cmd.Flags().StringVar(&opts.onGraph, "on-graph", "", "Scope the privilege to a graph (use * for all)")
	cmd.Flags().StringVar(&opts.onDatabase, "on-database", "", "Scope the privilege to a database (use * for all)")
	cmd.Flags().BoolVar(&opts.onDbms, "on-dbms", false, "Scope the privilege to the DBMS")
	cmd.Flags().StringArrayVar(&opts.nodeLabels, "node-label", nil, "Restrict a graph privilege to node labels")
	cmd.Flags().StringArrayVar(&opts.relTypes, "relationship-type", nil, "Restrict a graph privilege to relationship types")
	cmd.Flags().StringArrayVar(&opts.properties, "property", nil, "Restrict a property privilege to properties")

	return cmd
}

// revokeVerb resolves the --revoke-type flag to the REVOKE verb: empty → REVOKE,
// grant → REVOKE GRANT, deny → REVOKE DENY. Any other value is a usage error.
func revokeVerb(revokeType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(revokeType)) {
	case "":
		return "REVOKE", nil
	case "grant":
		return "REVOKE GRANT", nil
	case "deny":
		return "REVOKE DENY", nil
	default:
		return "", clierr.NewUsageError("--revoke-type must be grant or deny")
	}
}
