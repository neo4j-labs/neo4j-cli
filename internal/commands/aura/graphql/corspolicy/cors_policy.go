// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package corspolicy

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/graphql/corspolicy/allowedorigin"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "cors-policy",
		Short: "Allows you to manage the Cross-Origin Resource Sharing (CORS) policy for a specific GraphQL Data API",
	}

	cmd.AddCommand(allowedorigin.NewCmd(cfg))

	return cmd
}
