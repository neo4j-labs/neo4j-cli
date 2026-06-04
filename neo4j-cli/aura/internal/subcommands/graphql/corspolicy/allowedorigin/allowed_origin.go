// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package allowedorigin

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "allowed-origin",
		Short: "Allows you to manage Cross-Origin Resource Sharing (CORS) allowed origins for a specific GraphQL Data API",
	}

	cmd.AddCommand(NewAddCmd(cfg))
	cmd.AddCommand(NewRemoveCmd(cfg))

	return cmd
}
