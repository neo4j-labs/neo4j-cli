// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/spf13/cobra"
)

func newDropCmd(cfg *clicfg.Config, credential *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "drop <username>",
		Short:       "Drop a Neo4j user",
		Annotations: map[string]string{"write": "true"},
		Long: "Drop (delete) a user from the system database. " +
			"Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively. " +
			"Uses the dbms credential named by --credential on the parent `admin` command.",
		Example: `# Drop a user with confirmation prompt (TTY only)
neo4j-cli admin user drop alice --credential local --rw

# Drop a user without prompting (required for scripts)
neo4j-cli admin user drop alice --credential local --rw --yes --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			if err := confirm.Require(cmd, name); err != nil {
				return err
			}
			cred, err := resolveCredential(cfg, credential)
			if err != nil {
				return err
			}
			_, err = userExecFn(cmd.Context(), cfg, cred, "DROP USER $name", map[string]any{"name": name})
			return err
		},
	}

	confirm.Register(cmd)

	return cmd
}
