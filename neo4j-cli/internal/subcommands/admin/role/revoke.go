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
		Long: "Revoke a role from a user via REVOKE ROLE $role FROM $user against the system database. " +
			"After the revoke, the updated user record is printed with the current role membership.",
		Example: `# Revoke the analyst role from alice
neo4j-cli admin role revoke --role analyst --user alice --credential local --rw

# Revoke a role and output the updated user record as JSON
neo4j-cli admin role revoke --role analyst --user alice --credential local --rw --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if _, err := roleExecFn(cmd.Context(), cfg, *conn, "REVOKE ROLE $role FROM $user", map[string]any{"role": roleName, "user": userName}); err != nil {
				return err
			}
			return outputUserAfterRoleChange(cmd, cfg, *conn, userName)
		},
	}

	cmd.Flags().StringVar(&roleName, "role", "", "Name of the role to revoke")
	cmd.Flags().StringVar(&userName, "user", "", "Name of the user to revoke the role from")
	_ = cmd.MarkFlagRequired("role")
	_ = cmd.MarkFlagRequired("user")

	return cmd
}
