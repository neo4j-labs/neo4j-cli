// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newCreateCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return &cobra.Command{
		Use:         "create <name>",
		Short:       "Create a role (Enterprise only)",
		Annotations: map[string]string{"write": "true"},
		Long: "Create a new role in the system database. " +
			"Executes CREATE ROLE $name against the system database. " +
			"Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error.",
		Example: `# Create a new role
neo4j-cli admin role create analyst --credential local --rw

# Create a role and confirm it exists
neo4j-cli admin role create analyst --credential local --rw && neo4j-cli admin role list --credential local --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			if _, err := roleExecFn(cmd.Context(), cfg, *conn, "CREATE ROLE $name", map[string]any{"name": name}); err != nil {
				return err
			}
			return outputRoleMembers(cmd, cfg, *conn, name)
		},
	}
}
