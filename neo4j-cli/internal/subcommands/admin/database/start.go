// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newStartCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var wait bool

	cmd := &cobra.Command{
		Use:         "start <name>",
		Short:       "Start a database",
		Annotations: map[string]string{"write": "true"},
		Long: "Start a database via START DATABASE <name> against the system database. " +
			"Pass --wait to block until the database status is online (polls every 1 second, 60-second timeout).",
		Example: `# Start a database
neo4j-cli admin database start mydb --credential local --rw

# Start a database and wait until it is online before returning
neo4j-cli admin database start mydb --credential local --wait --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			if _, err := dbExecFn(cmd.Context(), cfg, *conn, "START DATABASE $name", map[string]any{"name": name}); err != nil {
				return err
			}
			if wait {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Waiting for database to come online...\n")
				if err := pollDatabaseStatus(cmd, cfg, *conn, name, "online"); err != nil {
					return err
				}
			}
			return nil
		},
	}

	flags.RegisterWait(cmd, &wait, "Wait until the database is online before returning (polls every 1 second, 60-second timeout).")
	return cmd
}
