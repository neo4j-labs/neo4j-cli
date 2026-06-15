// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

func newActivateCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return &cobra.Command{
		Use:         "activate <name>",
		Short:       "Activate a user",
		Annotations: map[string]string{"write": "true"},
		Long: "Activate a previously suspended user by setting their status to ACTIVE via " +
			"ALTER USER $name SET STATUS ACTIVE against the system database. " +
			"An active user can log in normally. Returns the updated user record on success.",
		Example: `# Activate a user
neo4j-cli admin user activate alice --credential local --rw

# Activate a user and display the result as JSON
neo4j-cli admin user activate alice --credential local --rw --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			if _, err := userExecFn(cmd.Context(), cfg, *conn, "ALTER USER $name SET STATUS ACTIVE", map[string]any{"name": name}); err != nil {
				return err
			}
			return outputUser(cmd, cfg, *conn, name)
		},
	}
}
