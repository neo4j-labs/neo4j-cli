// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

var listFields = []string{"name", "type", "currentStatus", "access", "default"}

func newListCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all databases",
		Long: "List all databases visible from the system database. " +
			"Renders name, type, currentStatus, access, and default columns.",
		Example: `# List all databases as a table
neo4j-cli admin database list --credential local

# List all databases as JSON for scripting
neo4j-cli admin database list --credential local --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			rows, err := dbExecFn(cmd.Context(), cfg, *conn, "SHOW DATABASES", nil)
			if err != nil {
				return err
			}
			commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(rows), listFields)
			return nil
		},
	}
}

// dbExecFn is the package-level test seam. It must be set to a real
// implementation by the parent command (NewCmd) before any leaf runs.
// Tests replace it to inject fake results without opening a Bolt connection.
var dbExecFn adminutil.ExecFn
