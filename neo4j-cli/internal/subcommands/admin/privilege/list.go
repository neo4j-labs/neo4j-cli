// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"sort"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

func newListCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var roleFilter string
	var userFilter string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List privileges (Enterprise only)",
		Long: "List privileges from the system database (Enterprise edition only). " +
			"Executes SHOW PRIVILEGES by default. " +
			"Use --role to show privileges for a specific role or --user to show privileges for a specific user. " +
			"--role and --user are mutually exclusive.",
		Example: `# List all privileges as a table
neo4j-cli admin privilege list --credential local

# List all privileges as JSON for scripting
neo4j-cli admin privilege list --credential local --format json

# List privileges for a specific role
neo4j-cli admin privilege list --credential local --role analyst --format json

# List privileges for a specific user
neo4j-cli admin privilege list --credential local --user alice`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			if roleFilter != "" && userFilter != "" {
				return clierr.NewUsageError("--role and --user are mutually exclusive")
			}

			var cypher string
			var params map[string]any

			switch {
			case roleFilter != "":
				cypher = "SHOW ROLE $name PRIVILEGES"
				params = map[string]any{"name": roleFilter}
			case userFilter != "":
				cypher = "SHOW USER $name PRIVILEGES"
				params = map[string]any{"name": userFilter}
			default:
				cypher = "SHOW PRIVILEGES"
			}

			rows, err := privilegeExecFn(cmd.Context(), cfg, *conn, cypher, params)
			if err != nil {
				return err
			}

			fields := privilegeFields(rows)
			commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(rows), fields)
			return nil
		},
	}

	cmd.Flags().StringVar(&roleFilter, "role", "", "Show privileges for the specified role name (mutually exclusive with --user)")
	cmd.Flags().StringVar(&userFilter, "user", "", "Show privileges for the specified user name (mutually exclusive with --role)")

	return cmd
}

// privilegeFields derives a stable column list from the first row of results.
// SHOW PRIVILEGES returns a variable column set; columns are sorted for
// consistent output.
func privilegeFields(rows []map[string]any) []string {
	if len(rows) == 0 {
		return []string{"access", "action", "resource", "graph", "segment", "role"}
	}
	fields := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	return fields
}
