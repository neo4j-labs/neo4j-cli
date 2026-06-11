// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newRevokeCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var actionFlag string
	var roleFlag string
	var revokeType string
	var flags privilegeFlags

	cmd := &cobra.Command{
		Use:         "revoke",
		Short:       "Revoke a privilege from a role (Enterprise only)",
		Annotations: map[string]string{"write": "true"},
		Long: "Revoke a privilege from a role in the system database. " +
			"Executes REVOKE [GRANT|DENY] <action> ON <resource> FROM <role>. " +
			"Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error. " +
			"Use --revoke-type grant to revoke only a GRANT, --revoke-type deny to revoke only a DENY, " +
			"or omit --revoke-type to revoke both. " +
			"The --action flag accepts Neo4j privilege keywords (case-insensitive; use _ or space as word separator). " +
			"Resource target is controlled by --on-graph (default), --on-database, or --on-dbms (mutually exclusive). " +
			"Graph-scoped qualifiers --node-label, --relationship-type, and --property are only valid with --on-graph.",
		Example: `# Revoke READ on all graphs from the analyst role (revokes both GRANT and DENY)
neo4j-cli admin privilege revoke --action read --on-graph '*' --role analyst --credential local --rw

# Revoke only a GRANT of WRITE from a role
neo4j-cli admin privilege revoke --action write --on-graph '*' --role analyst --revoke-type grant --credential local --rw

# Revoke only a DENY of ACCESS from a role on a specific database
neo4j-cli admin privilege revoke --action access --on-database neo4j --role readonly --revoke-type deny --credential local --rw

# Revoke a DBMS-level privilege from a role
neo4j-cli admin privilege revoke --action create_role --on-dbms --role limited --credential local --rw`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			if revokeType != "" {
				rt := strings.ToLower(revokeType)
				if rt != "grant" && rt != "deny" {
					return clierr.NewUsageError("--revoke-type must be %q or %q, got %q", "grant", "deny", revokeType)
				}
				revokeType = rt
			}

			normalized, ok := normalizeAction(actionFlag)
			if !ok {
				return clierr.NewUsageError("unknown --action %q; valid actions: %s", actionFlag, validActionsHelp())
			}

			clause, err := buildPrivilegeCypher(normalized, flags)
			if err != nil {
				return err
			}

			var verb string
			switch revokeType {
			case "grant":
				verb = "REVOKE GRANT"
			case "deny":
				verb = "REVOKE DENY"
			default:
				verb = "REVOKE"
			}

			cypher := fmt.Sprintf("%s %s FROM $role", verb, clause)
			_, err = privilegeExecFn(cmd.Context(), cfg, *conn, cypher, map[string]any{"role": roleFlag})
			if err != nil {
				return err
			}
			return emitRolePrivileges(cmd, cfg, *conn, roleFlag)
		},
	}

	cmd.Flags().StringVar(&actionFlag, "action", "", "Privilege action keyword (required; e.g. read, write, access, create_role)")
	cmd.Flags().StringVar(&roleFlag, "role", "", "Role name to revoke the privilege from (required)")
	cmd.Flags().StringVar(&revokeType, "revoke-type", "", "Revoke only a GRANT or DENY: grant|deny (default: revoke both)")
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
