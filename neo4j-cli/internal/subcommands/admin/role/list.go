// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

var listFields = []string{"role", "member"}

func newListCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var roleFilter string
	var userFilter string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all roles and their members",
		Long: "List all roles and their member users. " +
			"Executes SHOW ROLES WITH USERS against the system database. " +
			"Use --role to filter by role name or --user to filter by user name.",
		Example: `# List all roles and their members as a table
neo4j-cli admin role list --credential local

# List all roles and members as JSON for scripting
neo4j-cli admin role list --credential local --format json

# Filter to a specific role
neo4j-cli admin role list --credential local --role admin --format json

# Filter to roles a specific user belongs to
neo4j-cli admin role list --credential local --user alice`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			rows, err := roleExecFn(cmd.Context(), cfg, *conn, "SHOW ROLES WITH USERS", nil)
			if err != nil {
				return err
			}

			if roleFilter != "" {
				filtered := rows[:0]
				for _, row := range rows {
					if r, ok := row["role"].(string); ok && r == roleFilter {
						filtered = append(filtered, row)
					}
				}
				rows = filtered
			}

			if userFilter != "" {
				filtered := rows[:0]
				for _, row := range rows {
					if m, ok := row["member"].(string); ok && m == userFilter {
						filtered = append(filtered, row)
					}
				}
				rows = filtered
			}

			normalized := make([]map[string]any, len(rows))
			for i, row := range rows {
				normalized[i] = normalizeRoleRow(row)
			}
			commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(normalized), listFields)
			return nil
		},
	}

	cmd.Flags().StringVar(&roleFilter, "role", "", "Filter output to the specified role name")
	cmd.Flags().StringVar(&userFilter, "user", "", "Filter output to rows where the member matches the specified user name")

	return cmd
}
