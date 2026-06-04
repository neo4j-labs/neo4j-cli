// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package authprovider

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "auth-provider",
		Short: "Allows you to programmatically manage Authentication providers for a specific GraphQL Data API",
	}

	cmd.AddCommand(NewListCmd(cfg))
	cmd.AddCommand(NewGetCmd(cfg))
	cmd.AddCommand(NewCreateCmd(cfg))
	cmd.AddCommand(NewDeleteCmd(cfg))

	return cmd
}
