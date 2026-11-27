// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package token

import (
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/deployment/internal/depflag"
	"github.com/spf13/cobra"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Relates to deployment tokens",
	}

	cmd.AddCommand(newCreateCmd(cfg))
	cmd.AddCommand(newUpdateCmd(cfg))
	cmd.AddCommand(newDeleteCmd(cfg))

	depflag.Register(cmd)

	return cmd
}
