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

func newListCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	var userFilter string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all roles and their members",
		Long: "List all roles and their members via SHOW ROLES WITH USERS. " +
			"Use --user to filter results to only roles that contain a specific user.",
		Example: `# List all roles and their members
neo4j-cli admin role list --credential local

# List all roles and members as JSON
neo4j-cli admin role list --credential local --format json

# List only the roles that user alice belongs to
neo4j-cli admin role list --credential local --user alice --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			rows, err := roleExecFn(cmd.Context(), cfg, *conn, "SHOW ROLES WITH USERS", nil)
			if err != nil {
				return err
			}
			for i, row := range rows {
				rows[i] = normalizeRoleRow(row)
			}
			if userFilter != "" {
				filtered := rows[:0]
				for _, row := range rows {
					if member, ok := row["member"].(string); ok && member == userFilter {
						filtered = append(filtered, row)
					}
				}
				rows = filtered
			}
			commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(rows), roleFields)
			return nil
		},
	}

	cmd.Flags().StringVar(&userFilter, "user", "", "Filter results to only roles that contain this user")

	return cmd
}
