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

func newStopCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var wait bool

	cmd := &cobra.Command{
		Use:         "stop <name>",
		Short:       "Stop a database",
		Annotations: map[string]string{"write": "true"},
		Long: "Stop a database via STOP DATABASE <name> against the system database. " +
			"Pass --wait to block until the database status is offline (polls every 1 second, 60-second timeout).",
		Example: `# Stop a database
neo4j-cli admin database stop mydb --credential local --rw

# Stop a database and wait until it is offline before returning
neo4j-cli admin database stop mydb --credential local --wait --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			if _, err := dbExecFn(cmd.Context(), cfg, *conn, "STOP DATABASE $name", map[string]any{"name": name}); err != nil {
				return err
			}
			if wait {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Waiting for database to go offline...\n")
				if err := pollDatabaseStatus(cmd, cfg, *conn, name, "offline"); err != nil {
					return err
				}
			}
			return nil
		},
	}

	flags.RegisterWait(cmd, &wait, "Wait until the database is offline before returning (polls every 1 second, 60-second timeout).")
	return cmd
}
