// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
)

func newListCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all users",
		Long: "List all users visible from the system database. " +
			"Renders an overview with user, roles, password_change_required, and suspended columns. " +
			"Use `get` for the full record of a single user.",
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
			for i, r := range rows {
				rows[i] = normalizeUserRow(r)
			}
			if commonoutput.ResolveOutput(cmd, cfg) == "json" {
				commonoutput.PrintBodyMap(cmd, cfg, adminutil.NewRows(rows, userFields), userFields)
				return nil
			}
			// table / toon: convert roles []any to comma-joined string
			commonoutput.PrintBodyMap(cmd, cfg, adminutil.NewRows(rolesJoined(rows), userFields), userFields)
			return nil
		},
	}
}

// rolesJoined returns a copy of rows with the "roles" field converted from
// []any to a comma-joined string, suitable for table and toon rendering.
func rolesJoined(rows []map[string]any) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		cp := make(map[string]any, len(r))
		for k, v := range r {
			cp[k] = v
		}
		cp["roles"] = joinRoles(cp["roles"])
		out[i] = cp
	}
	return out
}

// joinRoles converts a []any of role strings to a comma-separated string.
// A nil or empty slice becomes an empty string.
func joinRoles(v any) string {
	roles, ok := v.([]any)
	if !ok || len(roles) == 0 {
		return ""
	}
	parts := make([]string, 0, len(roles))
	for _, r := range roles {
		parts = append(parts, fmt.Sprintf("%v", r))
	}
	return strings.Join(parts, ",")
}
