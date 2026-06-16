// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"sort"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

func newGetCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Get the privileges for a role",
		Long: "Get the privileges assigned to a role. " +
			"First checks the role exists via SHOW ROLES WITH USERS WHERE role = $name; " +
			"returns a not-found error if no rows are returned. " +
			"Then executes SHOW ROLE $name PRIVILEGES and returns the privilege list. " +
			"Returns an empty list if the role exists but has no privileges.",
		Example: `# Get the privileges for the admin role
neo4j-cli admin role get admin --credential local

# Get privileges as JSON for scripting
neo4j-cli admin role get admin --credential local --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]

			// Step 1: check the role exists.
			existRows, err := roleExecFn(cmd.Context(), cfg, *conn, "SHOW ROLES WITH USERS WHERE role = $name", map[string]any{"name": name})
			if err != nil {
				return err
			}
			if len(existRows) == 0 {
				return clierr.NewNotFoundError("role %q not found", name)
			}

			// Step 2: fetch privileges.
			privRows, err := roleExecFn(cmd.Context(), cfg, *conn, "SHOW ROLE $name PRIVILEGES", map[string]any{"name": name})
			if err != nil {
				return err
			}
			if len(privRows) == 0 {
				commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(nil), []string{})
				return nil
			}

			// Derive sorted field list from the first row.
			fields := make([]string, 0, len(privRows[0]))
			for k := range privRows[0] {
				fields = append(fields, k)
			}
			sort.Strings(fields)

			commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(privRows), fields)
			return nil
		},
	}
}
