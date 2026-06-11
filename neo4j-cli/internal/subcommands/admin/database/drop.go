// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newDropCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "drop <name>",
		Short:       "Drop a database",
		Annotations: map[string]string{"write": "true"},
		Long: "Drop a database via DROP DATABASE <name> against the system database. " +
			"Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively.",
		Example: `# Drop a database, prompting for confirmation on a TTY
neo4j-cli admin database drop mydb --credential local --rw

# Drop a database without prompting (required for scripts and non-TTY callers)
neo4j-cli admin database drop mydb --credential local --rw --yes --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			if err := confirm.Require(cmd, name); err != nil {
				return err
			}
			if _, err := dbExecFn(cmd.Context(), cfg, *conn, "DROP DATABASE $name", map[string]any{"name": name}); err != nil {
				return err
			}
			return nil
		},
	}

	confirm.Register(cmd)

	return cmd
}
