// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newActivateCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return &cobra.Command{
		Use:         "activate <username>",
		Short:       "Activate (unsuspend) a Neo4j user (Enterprise edition only)",
		Annotations: map[string]string{"write": "true"},
		Long: "Activate (unsuspend) an existing user in the system database, allowing them to log in again. " +
			"Requires Enterprise edition (Community edition returns an error).",
		Example: `# Activate a previously suspended user
neo4j-cli admin user activate alice --credential local --rw

# Activate a user and verify status
neo4j-cli admin user activate bob --credential local --rw && neo4j-cli admin user get bob --credential local --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]

			_, err := userExecFn(cmd.Context(), cfg, *conn,
				"ALTER USER $name SET STATUS ACTIVE",
				map[string]any{"name": name},
			)
			return err
		},
	}
}
