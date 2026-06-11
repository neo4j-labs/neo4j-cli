// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

func newGetCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return &cobra.Command{
		Use:   "get <username>",
		Short: "Get details of a user",
		Long: "Get the full record for a single user by name. " +
			"Executes 'SHOW USERS WHERE user = $name' against the system database.",
		Example: `# Get a user record as a table
neo4j-cli admin user get neo4j --credential local

# Get a user record as JSON for scripting
neo4j-cli admin user get neo4j --credential local --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			rows, err := userExecFn(cmd.Context(), cfg, *conn, "SHOW USERS WHERE user = $name", map[string]any{"name": name})
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return clierr.NewNotFoundError("user %q not found", name)
			}
			normalized := make([]map[string]any, len(rows))
			for i, row := range rows {
				normalized[i] = normalizeUserRow(row)
			}
			commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(normalized), listFields)
			return nil
		},
	}
}
