// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/config"
	"github.com/spf13/cobra"
)

// NewAuraConfigCmd returns the aura config command mounted under the "aura"
// use-name, suitable for nesting under the neo4j root as "neo4j config aura".
func NewAuraConfigCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := config.NewCmd(cfg)
	cmd.Use = "aura"
	cmd.Short = "Manage and view Aura-specific configuration values"
	return cmd
}
