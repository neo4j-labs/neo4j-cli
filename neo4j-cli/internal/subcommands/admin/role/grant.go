// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func newGrantCmd(cfg *clicfg.Config, credential *string) *cobra.Command {
	var roleName string
	var userName string

	cmd := &cobra.Command{
		Use:         "grant",
		Short:       "Grant a role to a user (Enterprise only)",
		Annotations: map[string]string{"write": "true"},
		Long: "Grant a role to a user in the system database. " +
			"Executes GRANT ROLE $role TO $user against the system database. " +
			"Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error. " +
			"Uses the dbms credential named by --credential on the parent `admin` command.",
		Example: `# Grant the analyst role to a user
neo4j-cli admin role grant --role analyst --user alice --credential local --rw

# Grant the reader role to a user
neo4j-cli admin role grant --role reader --user bob --credential local --rw`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cred, err := resolveCredential(cfg, credential)
			if err != nil {
				return err
			}
			_, err = roleExecFn(cmd.Context(), cfg, cred, "GRANT ROLE $role TO $user", map[string]any{"role": roleName, "user": userName})
			return err
		},
	}

	cmd.Flags().StringVar(&roleName, "role", "", "Role name to grant (required)")
	cmd.Flags().StringVar(&userName, "user", "", "Username to grant the role to (required)")
	_ = cmd.MarkFlagRequired("role")
	_ = cmd.MarkFlagRequired("user")

	return cmd
}
