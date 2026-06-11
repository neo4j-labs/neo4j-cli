// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newSuspendCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return &cobra.Command{
		Use:         "suspend <username>",
		Short:       "Suspend a Neo4j user (Enterprise edition only)",
		Annotations: map[string]string{"write": "true"},
		Long: "Suspend an existing user in the system database, preventing them from logging in. " +
			"Requires Enterprise edition (Community edition returns an error).",
		Example: `# Suspend a user
neo4j-cli admin user suspend alice --credential local --rw

# Suspend a user and verify status
neo4j-cli admin user suspend bob --credential local --rw && neo4j-cli admin user get bob --credential local --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]

			if _, err := userExecFn(cmd.Context(), cfg, *conn,
				"ALTER USER $name SET STATUS SUSPENDED",
				map[string]any{"name": name},
			); err != nil {
				return err
			}
			return outputUser(cmd, cfg, *conn, name)
		},
	}
}
