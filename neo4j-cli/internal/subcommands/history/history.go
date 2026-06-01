// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package history

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// NewCmd builds the `neo4j-cli history` command tree. The parent is a
// non-runnable group that prints help; the runnable work lives in the `list`
// and `clear` leaves.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "View and manage the local command history log",
		Long: "View and manage the local, best-effort log of neo4j-cli commands you have run. " +
			"Each command is recorded as one redacted JSON line in a history file alongside config.json. " +
			"Recording is controlled by the `history-enabled` and `history-limit` config keys.",
	}

	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(newClearCmd(cfg))

	return cmd
}
