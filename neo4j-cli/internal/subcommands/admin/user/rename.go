// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newRenameCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "rename <old-name>",
		Short:       "Rename a Neo4j user",
		Annotations: map[string]string{"write": "true"},
		Long: "Rename an existing user in the system database. " +
			"Not supported on Aura connections (Aura uses a non-native authentication provider).",
		Example: `# Rename a user
neo4j-cli admin user rename alice --new-name alice2 --credential local --rw

# Rename a user and verify the change
neo4j-cli admin user rename bob --new-name bob-renamed --credential local --rw && neo4j-cli admin user get bob-renamed --credential local --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			oldName := args[0]
			newName, err := cmd.Flags().GetString("new-name")
			if err != nil {
				return err
			}

			if _, err = userExecFn(cmd.Context(), cfg, *conn,
				"RENAME USER $oldName TO $newName",
				map[string]any{"oldName": oldName, "newName": newName},
			); err != nil {
				return err
			}
			return outputUser(cmd, cfg, *conn, newName)
		},
	}
	cmd.Flags().String("new-name", "", "The new name for the user")
	_ = cmd.MarkFlagRequired("new-name")
	return cmd
}
