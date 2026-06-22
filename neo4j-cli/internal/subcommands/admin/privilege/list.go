// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

func newListCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var roleFilter, userFilter string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List privileges",
		Long: "List privileges via SHOW PRIVILEGES. " +
			"Use --role to scope to a single role's privileges (SHOW ROLE $name PRIVILEGES) " +
			"or --user to scope to a single user's privileges (SHOW USER $name PRIVILEGES). " +
			"--role and --user are mutually exclusive.",
		Example: `# List all privileges (read-only)
neo4j-cli admin privilege list --credential local

# List the privileges of role analyst as JSON
neo4j-cli admin privilege list --credential local --role analyst --format json

# List the privileges of user alice
neo4j-cli admin privilege list --credential local --user alice`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if roleFilter != "" && userFilter != "" {
				return clierr.NewUsageError("--role and --user are mutually exclusive")
			}
			cmd.SilenceUsage = true

			cypher := "SHOW PRIVILEGES"
			var params map[string]any
			switch {
			case roleFilter != "":
				cypher = "SHOW ROLE $name PRIVILEGES"
				params = map[string]any{"name": roleFilter}
			case userFilter != "":
				cypher = "SHOW USER $name PRIVILEGES"
				params = map[string]any{"name": userFilter}
			}

			rows, err := privilegeExecFn(cmd.Context(), cfg, *conn, cypher, params)
			if err != nil {
				return err
			}
			commonoutput.PrintBodyMap(cmd, cfg, adminutil.NewRows(rows, privilegeFields), privilegeFields)
			return nil
		},
	}

	cmd.Flags().StringVar(&roleFilter, "role", "", "Scope output to a single role's privileges")
	cmd.Flags().StringVar(&userFilter, "user", "", "Scope output to a single user's privileges")

	return cmd
}
