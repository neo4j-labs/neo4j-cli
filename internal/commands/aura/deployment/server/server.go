// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/deployment/internal/depflag"
	"github.com/neo4j/cli/internal/commands/aura/deployment/server/database"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Relates to deployment servers",
	}

	cmd.AddCommand(newListCmd(cfg))
	cmd.AddCommand(database.NewCmd(cfg))

	depflag.Register(cmd)

	return cmd
}
