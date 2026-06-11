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

func newGrantCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var actionFlag string
	var roleFlag string
	var flags privilegeFlags

	cmd := &cobra.Command{
		Use:         "grant",
		Short:       "Grant a privilege to a role (Enterprise only)",
		Annotations: map[string]string{"write": "true"},
		Long: "Grant a privilege to a role in the system database. " +
			"Executes GRANT <action> ON <resource> TO <role>. " +
			"Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error. " +
			"The --action flag accepts Neo4j privilege keywords (case-insensitive; use _ or space as word separator). " +
			"Resource target is controlled by --on-graph (default), --on-database, or --on-dbms (mutually exclusive). " +
			"Graph-scoped qualifiers --node-label, --relationship-type, and --property are only valid with --on-graph.",
		Example: `# Grant READ on all graphs to the analyst role
neo4j-cli admin privilege grant --action read --on-graph '*' --role analyst --credential local --rw

# Grant ACCESS on a specific database to a role
neo4j-cli admin privilege grant --action access --on-database neo4j --role analyst --credential local --rw

# Grant a DBMS-level privilege to a role
neo4j-cli admin privilege grant --action create_role --on-dbms --role admin --credential local --rw

# Grant READ on specific nodes and property to a role
neo4j-cli admin privilege grant --action read --on-graph neo4j --node-label Person --property name --role analyst --credential local --rw`,
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

			cypher := fmt.Sprintf("GRANT %s TO $role", clause)
			_, err = privilegeExecFn(cmd.Context(), cfg, *conn, cypher, map[string]any{"role": roleFlag})
			return err
		},
	}

	cmd.Flags().StringVar(&actionFlag, "action", "", "Privilege action keyword (required; e.g. read, write, access, create_role)")
	cmd.Flags().StringVar(&roleFlag, "role", "", "Role name to grant the privilege to (required)")
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
