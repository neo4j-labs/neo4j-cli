// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
)

func newDropCmd(cfg *clicfg.Config, credential *string) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:         "drop <name>",
		Short:       "Drop a database",
		Annotations: map[string]string{"write": "true"},
		Long: "Drop a database via DROP DATABASE <name> against the system database. " +
			"Uses the dbms credential named by --credential on the parent `admin` command. " +
			"Pass --yes to skip the confirmation prompt. " +
			"Without --yes: prompts interactively on a TTY or returns a usage error on non-TTY.",
		Example: `# Drop a database, prompting for confirmation on a TTY
neo4j-cli admin database drop mydb --credential local --rw

# Drop a database without prompting (required for scripts and non-TTY callers)
neo4j-cli admin database drop mydb --credential local --yes --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]
			if !yes {
				if !stdinIsTTY() {
					return clierr.NewUsageError(
						"refusing to drop database %q without confirmation: pass --yes to proceed non-interactively",
						name,
					)
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Drop database %q? [y/N]: ", name)
				reader := bufio.NewReader(cmd.InOrStdin())
				line, _ := reader.ReadString('\n')
				answer := strings.ToLower(strings.TrimSpace(line))
				if answer != "y" && answer != "yes" {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "cancelled.")
					cmd.SilenceErrors = true
					return clierr.NewUsageError("drop cancelled")
				}
			}
			cred, err := resolveCredential(cfg, credential)
			if err != nil {
				return err
			}
			if _, err := dbExecFn(cmd.Context(), cfg, cred, "DROP DATABASE $name", map[string]any{"name": name}); err != nil {
				return err
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the confirmation prompt and drop the database immediately.")
	return cmd
}
