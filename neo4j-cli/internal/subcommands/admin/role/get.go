// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"sort"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

func newGetCmd(cfg *clicfg.Config, credential *string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <role>",
		Short: "Get privileges for a role",
		Long: "Get the full privileges record for a single role by name. " +
			"Executes SHOW ROLE $name PRIVILEGES against the system database. " +
			"Uses the dbms credential named by --credential on the parent `admin` command.",
		Example: `# Get privileges for a role as a table
neo4j-cli admin role get admin --credential local

# Get privileges for a role as JSON for scripting
neo4j-cli admin role get admin --credential local --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			cred, err := adminutil.ResolveCredential(cfg, credential)
			if err != nil {
				return err
			}
			rows, err := roleExecFn(cmd.Context(), cfg, cred, "SHOW ROLE $name PRIVILEGES", map[string]any{"name": name})
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return clierr.NewNotFoundError("role %q not found", name)
			}
			var fields []string
			for k := range rows[0] {
				fields = append(fields, k)
			}
			sort.Strings(fields)
			commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(rows), fields)
			return nil
		},
	}
}
