// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package app builds the neo4j-cli cobra command tree.
//
// It is split out of package main so generators (e.g. the per-binary skill
// bundle generator) can import the tree without pulling in main's entrypoint
// side-effects.
package app

import (
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/common/skill"
	"github.com/neo4j/cli/neo4j-cli/aura"
	binskill "github.com/neo4j/cli/neo4j-cli/internal/skill"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/config"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/credential"
	"github.com/neo4j/cli/neo4j-cli/query"
	"github.com/spf13/cobra"
)

// Version is the neo4j-cli binary version. It is overridden at release time
// via -ldflags "-X github.com/neo4j/cli/neo4j-cli/app.Version=<tag>".
var Version = "dev"

// NewCmd returns the neo4j-cli root cobra command with all subcommands wired.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "neo4j-cli",
		Short:   "Allows you to manage Neo4j resources",
		Version: Version,
	}

	flags.RegisterOutputFlag(cmd, cfg)

	auraCmd := aura.NewCmd(cfg)
	auraCmd.Use = "aura"
	cmd.AddCommand(auraCmd)
	cmd.AddCommand(credential.NewCredentialCmd(cfg))
	cmd.AddCommand(config.NewCmd(cfg))
	cmd.AddCommand(query.NewCmd(cfg))
	cmd.AddCommand(skill.NewCmd(cfg, binskill.Bundle, "neo4j-cli"))

	cobra.EnableTraverseRunHooks = true

	return cmd
}
