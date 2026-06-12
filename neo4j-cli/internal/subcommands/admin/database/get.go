// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

var getFields = []string{"name", "type", "access", "current_status", "requested_status", "status_message", "address", "role", "writer", "default", "home", "database_id"}

func newGetCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Get details of a database",
		Long: "Get the full record for a single database by name. " +
			"Executes SHOW DATABASE $name against the system database. " +
			"Renders name, type, access, current_status, requested_status, status_message, address, role, writer, default, home, and database_id columns.",
		Example: `# Get a database record as a table
neo4j-cli admin database get neo4j --credential local

# Get a database record as JSON for scripting
neo4j-cli admin database get neo4j --credential local --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			rows, err := dbExecFn(cmd.Context(), cfg, *conn, "SHOW DATABASE $name", map[string]any{"name": name})
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return clierr.NewNotFoundError("database %q not found", name)
			}
			commonoutput.PrintBodyMap(cmd, cfg, adminutil.NewRow(rows[0], getFields), getFields)
			return nil
		},
	}
}
