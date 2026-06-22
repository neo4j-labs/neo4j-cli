// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newGrantCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var roleName string
	var userName string

	cmd := &cobra.Command{
		Use:         "grant",
		Short:       "Grant a role to a user",
		Annotations: map[string]string{"write": "true"},
		Long: "Grant a role to a user via GRANT ROLE $role TO $user against the system database. " +
			"After the grant, the updated user record is printed with the current role membership.",
		Example: `# Grant the analyst role to alice
neo4j-cli admin role grant --role analyst --user alice --credential local --rw

# Grant a role and output the updated user record as JSON
neo4j-cli admin role grant --role analyst --user alice --credential local --rw --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if roleName == "" {
				return clierr.NewUsageError("--role is required")
			}
			if userName == "" {
				return clierr.NewUsageError("--user is required")
			}
			cmd.SilenceUsage = true
			if _, err := roleExecFn(cmd.Context(), cfg, *conn, "GRANT ROLE $role TO $user", map[string]any{"role": roleName, "user": userName}); err != nil {
				return err
			}
			return outputUserAfterRoleChange(cmd, cfg, *conn, userName)
		},
	}

	cmd.Flags().StringVar(&roleName, "role", "", "Name of the role to grant")
	cmd.Flags().StringVar(&userName, "user", "", "Name of the user to grant the role to")

	return cmd
}
