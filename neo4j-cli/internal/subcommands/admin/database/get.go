// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

func newGetCmd(cfg *clicfg.Config, credential *string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Get details of a database",
		Long: "Get the full record for a single database by name. " +
			"Executes SHOW DATABASE $name against the system database. " +
			"Uses the dbms credential named by --credential on the parent `admin` command.",
		Example: `# Get a database record as a table
neo4j-cli admin database get neo4j --credential local

# Get a database record as JSON for scripting
neo4j-cli admin database get neo4j --credential local --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			cred, err := adminutil.ResolveCredential(cfg, credential)
			if err != nil {
				return err
			}
			rows, err := dbExecFn(cmd.Context(), cfg, cred, "SHOW DATABASE $name", map[string]any{"name": name})
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return clierr.NewNotFoundError("database %q not found", name)
			}
			commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(rows), listFields)
			return nil
		},
	}
}
