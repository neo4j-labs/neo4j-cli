// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user

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
		Use:         "drop <username>",
		Short:       "Drop a Neo4j user",
		Annotations: map[string]string{"write": "true"},
		Long: "Drop (delete) a user from the system database. " +
			"Without --yes the command prompts for confirmation on a TTY, or returns a usage error on non-TTY. " +
			"Uses the dbms credential named by --credential on the parent `admin` command.",
		Example: `# Drop a user with confirmation prompt (TTY only)
neo4j-cli admin user drop alice --credential local --rw

# Drop a user without prompting (required for scripts)
neo4j-cli admin user drop alice --yes --credential local --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]

			if !yes {
				if !stdinIsTTY() {
					return clierr.NewUsageError(
						"refusing to drop user %q without confirmation: pass --yes to proceed non-interactively",
						name,
					)
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Drop user %q? [y/N]: ", name)
				reader := bufio.NewReader(cmd.InOrStdin())
				line, _ := reader.ReadString('\n')
				answer := strings.ToLower(strings.TrimSpace(line))
				if answer != "y" && answer != "yes" {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "cancelled.")
					cmd.SilenceErrors = true
					cmd.SilenceUsage = true
					return nil
				}
			}

			cred, err := resolveCredential(cfg, credential)
			if err != nil {
				return err
			}

			_, err = userExecFn(cmd.Context(), cfg, cred, "DROP USER $name", map[string]any{"name": name})
			return err
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm the drop without an interactive prompt")

	return cmd
}
