// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

var listFields = []string{"user", "roles", "passwordChangeRequired", "suspended"}

func newListCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all users",
		Long: "List all users visible from the system database. " +
			"Renders user, roles, passwordChangeRequired, and suspended columns.",
		Example: `# List all users as a table
neo4j-cli admin user list --credential local

# List all users as JSON for scripting
neo4j-cli admin user list --credential local --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			rows, err := userExecFn(cmd.Context(), cfg, *conn, "SHOW USERS", nil)
			if err != nil {
				return err
			}
			commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(rows), listFields)
			return nil
		},
	}
}
