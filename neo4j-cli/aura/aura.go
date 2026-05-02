// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/config"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/deployment"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/graphanalytics"
	_import "github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/import"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/credential"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/customermanagedkey"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/dataapi"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/instance"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/tenant"
)

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "aura-cli",
		Short:   "Allows you to programmatically provision and manage your Aura resources",
		Version: cfg.Version,
	}

	cmd.AddCommand(customermanagedkey.NewCmd(cfg))
	cmd.AddCommand(instance.NewCmd(cfg))
	cmd.AddCommand(tenant.NewCmd(cfg))
	cmd.AddCommand(graphanalytics.NewCmd(cfg))
	if cfg.Aura.AuraBetaEnabled() {
		cmd.AddCommand(dataapi.NewCmd(cfg))
		cmd.AddCommand(_import.NewCmd(cfg))
		cmd.AddCommand(deployment.NewCmd(cfg))
	}

	return cmd
}

func NewStandaloneCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := NewCmd(cfg)
	cmd.AddCommand(config.NewCmd(cfg))
	cmd.AddCommand(credential.NewCmd(cfg))
	return cmd
}
