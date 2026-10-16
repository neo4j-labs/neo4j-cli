// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package feedback implements the `neo4j-cli feedback` command group, behind
// flag.feedback-command: coding agents and people send short, locally
// validated reports of friction with neo4j-cli to the Neo4j team, only after a
// person has agreed to it.
package feedback

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

// NewCmd builds the `neo4j-cli feedback` tree.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	short := groupShort
	cmd := &cobra.Command{
		Use:   "feedback",
		Short: short,
		Long: short + ". Reports are short, structured and checked locally before anything is sent; " +
			"see `neo4j-cli feedback submit --help`. Sending is only possible after a person enables it, with `neo4j-cli feedback enable` or by accepting the question the feedback tool asks the first time an agent reports.",
	}
	cmd.AddCommand(newSubmitCmd(cfg))
	cmd.AddCommand(newEnableCmd(cfg))
	cmd.AddCommand(newDisableCmd(cfg))
	cmd.AddCommand(newStatusCmd(cfg))
	cmd.AddCommand(newServeCmd(cfg))
	cmd.AddCommand(newRegisterCmd(cfg))
	return cmd
}
