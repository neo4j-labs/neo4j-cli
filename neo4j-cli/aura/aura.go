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
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/common/skill"
	binskill "github.com/neo4j/cli/neo4j-cli/aura/internal/skill"
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
		Long:    "Allows you to programmatically provision and manage your Aura resources. Write operations require --rw. `query run` under neo4j-cli runs EXPLAIN first when --rw is not set and blocks statements classified as writes.",
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
	flags.RegisterRwFlag(cmd)
	// Wrap cobra's flag-parse errors (unknown flag, missing value, bad type)
	// into a typed *clierr.CLIError with exit code 2. Cobra walks up to the
	// root for FlagErrorFunc, so one registration covers every subcommand
	// under the standalone tree.
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clierr.NewUsageError("%v", err)
	})
	cmd.AddCommand(config.NewCmd(cfg))
	cmd.AddCommand(credential.NewCmd(cfg))
	cmd.AddCommand(skill.NewCmd(cfg, binskill.Bundle, "aura-cli"))
	return cmd
}
