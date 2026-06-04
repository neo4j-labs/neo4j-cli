// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

func newRenameCmd(cfg *clicfg.Config, credential *string) *cobra.Command {
	return &cobra.Command{
		Use:         "rename <old-name> <new-name>",
		Short:       "Rename a Neo4j user",
		Annotations: map[string]string{"write": "true"},
		Long: "Rename an existing user in the system database. " +
			"Not supported on Aura connections (Aura uses a non-native authentication provider). " +
			"Uses the dbms credential named by --credential on the parent `admin` command.",
		Example: `# Rename a user
neo4j-cli admin user rename alice alice2 --credential local --rw

# Rename a user and verify the change
neo4j-cli admin user rename bob bob-renamed --credential local --rw && neo4j-cli admin user get bob-renamed --credential local --format json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			oldName := args[0]
			newName := args[1]

			cred, err := adminutil.ResolveCredential(cfg, credential)
			if err != nil {
				return err
			}

			_, err = userExecFn(cmd.Context(), cfg, cred,
				"RENAME USER $oldName TO $newName",
				map[string]any{"oldName": oldName, "newName": newName},
			)
			return err
		},
	}
}
