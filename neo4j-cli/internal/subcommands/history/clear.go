// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package history

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
)

func newClearCmd(cfg *clicfg.Config) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:         "clear",
		Short:       "Empty the local command history log",
		Annotations: map[string]string{"write": "true"},
		Long: "Empty the local command history log. This is destructive and irreversible, so it requires --force. " +
			"Recording of future commands is unaffected (controlled by the `history-enabled` config key).",
		Example: `# Clearing requires --force; this errors with guidance
neo4j-cli history clear

# Empty the history log
neo4j-cli history clear --force`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if !force {
				return clierr.NewUsageError("refusing to clear history without confirmation; re-run with --force")
			}
			if err := Clear(cfg); err != nil {
				return err
			}
			cmd.Println("History cleared.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Confirm clearing the history log")

	return cmd
}
