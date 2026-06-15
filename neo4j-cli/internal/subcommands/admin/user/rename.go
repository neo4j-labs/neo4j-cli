// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

func newRenameCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var newName string

	cmd := &cobra.Command{
		Use:         "rename <name>",
		Short:       "Rename a user",
		Annotations: map[string]string{"write": "true"},
		Long: "Rename a user via RENAME USER $oldName TO $newName against the system database. " +
			"On success, emits the updated user record for the new name. " +
			"On Aura, renaming a non-native user is rejected by the server.",
		Example: `# Rename a user
neo4j-cli admin user rename alice --new-name bob --credential local --rw

# Rename a user and view the result as JSON
neo4j-cli admin user rename alice --new-name bob --credential local --rw --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			oldName := args[0]
			if _, err := userExecFn(cmd.Context(), cfg, *conn, "RENAME USER $oldName TO $newName", map[string]any{
				"oldName": oldName,
				"newName": newName,
			}); err != nil {
				return err
			}
			return outputUser(cmd, cfg, *conn, newName)
		},
	}

	cmd.Flags().StringVar(&newName, "new-name", "", "New name for the user (required)")
	_ = cmd.MarkFlagRequired("new-name")

	return cmd
}
