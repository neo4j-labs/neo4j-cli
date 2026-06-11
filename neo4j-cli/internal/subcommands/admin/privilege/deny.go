// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newDenyCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var actionFlag string
	var roleFlag string
	var flags privilegeFlags

	cmd := &cobra.Command{
		Use:         "deny",
		Short:       "Deny a privilege to a role (Enterprise only)",
		Annotations: map[string]string{"write": "true"},
		Long: "Deny a privilege to a role in the system database. " +
			"Executes DENY <action> ON <resource> TO <role>. " +
			"Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error. " +
			"The --action flag accepts Neo4j privilege keywords (case-insensitive; use _ or space as word separator). " +
			"Resource target is controlled by --on-graph (default), --on-database, or --on-dbms (mutually exclusive). " +
			"Graph-scoped qualifiers --node-label, --relationship-type, and --property are only valid with --on-graph.",
		Example: `# Deny WRITE on all graphs to the readonly role
neo4j-cli admin privilege deny --action write --on-graph '*' --role readonly --credential local --rw

# Deny ACCESS on a specific database to a role
neo4j-cli admin privilege deny --action access --on-database restricted --role readonly --credential local --rw

# Deny a DBMS-level privilege to a role
neo4j-cli admin privilege deny --action create_user --on-dbms --role limited --credential local --rw

# Deny READ on specific nodes to a role
neo4j-cli admin privilege deny --action read --on-graph neo4j --node-label Secret --role analyst --credential local --rw`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			normalized, ok := normalizeAction(actionFlag)
			if !ok {
				return clierr.NewUsageError("unknown --action %q; valid actions: %s", actionFlag, validActionsHelp())
			}

			clause, err := buildPrivilegeCypher(normalized, flags)
			if err != nil {
				return err
			}

			cypher := fmt.Sprintf("DENY %s TO $role", clause)
			_, err = privilegeExecFn(cmd.Context(), cfg, *conn, cypher, map[string]any{"role": roleFlag})
			return err
		},
	}

	cmd.Flags().StringVar(&actionFlag, "action", "", "Privilege action keyword (required; e.g. read, write, access, create_role)")
	cmd.Flags().StringVar(&roleFlag, "role", "", "Role name to deny the privilege to (required)")
	cmd.Flags().StringVar(&flags.onGraph, "on-graph", "", "Target a specific graph by name (default: * when no resource flag is set)")
	cmd.Flags().StringVar(&flags.onDatabase, "on-database", "", "Target a specific database by name")
	cmd.Flags().BoolVar(&flags.onDbms, "on-dbms", false, "Target the DBMS (for DBMS-level privileges)")
	cmd.Flags().StringArrayVar(&flags.nodeLabels, "node-label", nil, "Node label qualifier (repeatable; only valid with --on-graph)")
	cmd.Flags().StringArrayVar(&flags.relationshipTypes, "relationship-type", nil, "Relationship type qualifier (repeatable; only valid with --on-graph)")
	cmd.Flags().StringArrayVar(&flags.properties, "property", nil, "Property name qualifier (repeatable; only valid with --on-graph; default: all properties)")

	_ = cmd.MarkFlagRequired("action")
	_ = cmd.MarkFlagRequired("role")

	return cmd
}
