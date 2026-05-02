// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/config/project"
	"github.com/spf13/cobra"
)

// NewAuraProjectCmd returns the project subcommand, suitable for mounting
// directly under the neo4j config command.
func NewAuraProjectCmd(cfg *clicfg.Config) *cobra.Command {
	return project.NewCmd(cfg)
}
