// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/spf13/cobra"
)

func newCreateCmd(cfg *clicfg.Config, conn **dbconn.Conn) *cobra.Command {
	return &cobra.Command{
		Use:         "create <name>",
		Short:       "Create a role",
		Annotations: map[string]string{"write": "true"},
		Long: "Create a role via CREATE ROLE $name IF NOT EXISTS against the system database. " +
			"The command is idempotent — running it twice does not return an error. " +
			"After creation the current member list for the role is printed.",
		Example: `# Create a role named analyst
neo4j-cli admin role create analyst --credential local --rw

# Create a role and output the member list as JSON
neo4j-cli admin role create analyst --credential local --rw --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			if _, err := roleExecFn(cmd.Context(), cfg, *conn, "CREATE ROLE $name IF NOT EXISTS", map[string]any{"name": name}); err != nil {
				return err
			}
			return outputRoleMembers(cmd, cfg, *conn, name)
		},
	}
}
