// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package role

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/confirm"
	"github.com/spf13/cobra"
)

func newDropCmd(cfg *clicfg.Config, credential *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "drop <name>",
		Short:       "Drop a role (Enterprise only)",
		Annotations: map[string]string{"write": "true"},
		Long: "Drop an existing role from the system database. " +
			"Executes DROP ROLE $name against the system database. " +
			"Enterprise edition only: Community edition returns an UnsupportedAdministrationCommand error. " +
			"Destructive: requires --yes --force (or a y answer at the TTY prompt) when invoked non-interactively. " +
			"Uses the dbms credential named by --credential on the parent `admin` command.",
		Example: `# Drop a role (prompts on a TTY)
neo4j-cli admin role drop analyst --credential local --rw

# Drop a role without prompting (for scripts and non-TTY callers)
neo4j-cli admin role drop analyst --credential local --rw --yes --force`,
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
			_, err = roleExecFn(cmd.Context(), cfg, cred, "DROP ROLE $name", map[string]any{"name": name})
			return err
		},
	}

	confirm.Register(cmd)

	return cmd
}
