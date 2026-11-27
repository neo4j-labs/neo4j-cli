// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/spf13/cobra"
)

// newDisableCmd is deliberately not annotated as a write, unlike enable and
// register: revoking permission must never need --rw, so an agent asked to turn
// feedback off can always do it.
func newDisableCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Revoke permission to send feedback",
		Long:  "Revoke permission to send feedback immediately. Anyone can run this. Local receipts and the other feedback settings are kept.",
		Example: `# Revoke feedback authorization
neo4j-cli feedback disable

# Revoke and print the resulting state as JSON
neo4j-cli feedback disable --format json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if err := setValues(cfg, [2]string{keyAuthorizedUntil, ""}); err != nil {
				return clierr.NewFatalError("could not revoke feedback, the config file is not writable: %s", err)
			}
			return writeStatus(cmd, cfg, currentStatus(cfg))
		},
	}
}
