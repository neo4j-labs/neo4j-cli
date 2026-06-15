// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

func newSuspendCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return &cobra.Command{
		Use:         "suspend <name>",
		Short:       "Suspend a user",
		Annotations: map[string]string{"write": "true"},
		Long: "Suspend a user by setting their status to SUSPENDED via " +
			"ALTER USER $name SET STATUS SUSPENDED against the system database. " +
			"A suspended user cannot log in. Returns the updated user record on success.",
		Example: `# Suspend a user
neo4j-cli admin user suspend alice --credential local --rw

# Suspend a user and display the result as JSON
neo4j-cli admin user suspend alice --credential local --rw --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			if _, err := userExecFn(cmd.Context(), cfg, *conn, "ALTER USER $name SET STATUS SUSPENDED", map[string]any{"name": name}); err != nil {
				return translateUserNotFoundError(err, name)
			}
			return outputUser(cmd, cfg, *conn, name)
		},
	}
}
