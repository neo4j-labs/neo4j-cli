// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newRevokeCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var roleName string
	var userName string

	cmd := &cobra.Command{
		Use:         "revoke",
		Short:       "Revoke a role from a user (Enterprise only)",
		Annotations: map[string]string{"write": "true"},
		Long: "Revoke a role from a user in the system database. " +
			"Executes REVOKE ROLE $role FROM $user against the system database. " +
			"Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error.",
		Example: `# Revoke the analyst role from a user
neo4j-cli admin role revoke --role analyst --user alice --credential local --rw

# Revoke the reader role from a user
neo4j-cli admin role revoke --role reader --user bob --credential local --rw`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if _, err := roleExecFn(cmd.Context(), cfg, *conn, "REVOKE ROLE $role FROM $user", map[string]any{"role": roleName, "user": userName}); err != nil {
				return err
			}
			return outputUserRoles(cmd, cfg, *conn, userName)
		},
	}

	cmd.Flags().StringVar(&roleName, "role", "", "Role name to revoke (required)")
	cmd.Flags().StringVar(&userName, "user", "", "Username to revoke the role from (required)")
	_ = cmd.MarkFlagRequired("role")
	_ = cmd.MarkFlagRequired("user")

	return cmd
}
