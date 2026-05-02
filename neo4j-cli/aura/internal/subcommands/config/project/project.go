// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

var projectPrintFields = []string{"name", "organizationId", "projectId", "default"}

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage and view Aura project configurations.",
		Long:  "Manage and view Aura project configurations. Configurable values include organization and project id. Set the project you want to work with as the default project with `config project use <name>`",
	}

	cmd.AddCommand(NewAddCmd(cfg))
	cmd.AddCommand(NewUseCmd(cfg))
	cmd.AddCommand(NewListCmd(cfg))
	cmd.AddCommand(NewRemoveCmd(cfg))

	return cmd
}
