// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/spf13/cobra"
)

var listFields = []string{"user", "roles", "passwordChangeRequired", "suspended"}

func newListCmd(cfg *clicfg.Config, credential *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all users",
		Long: "List all users visible from the system database. " +
			"Renders user, roles, passwordChangeRequired, and suspended columns. " +
			"Uses the dbms credential named by --credential on the parent `admin` command.",
		Example: `# List all users as a table
neo4j-cli admin user list --credential local

# List all users as JSON for scripting
neo4j-cli admin user list --credential local --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cred, err := resolveCredential(cfg, credential)
			if err != nil {
				return err
			}
			rows, err := userExecFn(cmd.Context(), cfg, cred, "SHOW USERS", nil)
			if err != nil {
				return err
			}
			commonoutput.PrintBodyMap(cmd, cfg, userRows(rows), listFields)
			return nil
		},
	}
}
