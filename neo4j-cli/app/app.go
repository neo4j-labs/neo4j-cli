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
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/common/skill"
	"github.com/neo4j/cli/neo4j-cli/aura"
	binskill "github.com/neo4j/cli/neo4j-cli/internal/skill"
	"github.com/neo4j/cli/neo4j-cli/internal/skillrefresh"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/agentcontext"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/config"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/credential"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/update"
	"github.com/neo4j/cli/neo4j-cli/internal/versioncheck"
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
		Long:    "Allows you to manage Neo4j resources. Write operations require --rw.",
		Version: Version,
	}

	flags.RegisterOutputFlag(cmd, cfg)
	flags.RegisterRwFlag(cmd)

	// Wrap cobra's flag-parse errors (unknown flag, missing value, bad type)
	// into a typed *clierr.CLIError with exit code 2. Cobra walks up to the
	// root for FlagErrorFunc, so one registration covers every subcommand.
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clierr.NewUsageError("%v", err)
	})

	// Compose the root PersistentPreRunE: bind --format, enforce --rw,
	// then schedule the silent background version-check (5% sample, cached
	// in version-check.json under cfg.Aura.Fs()) and print the
	// stderr nag if the cache shows a newer stable. Both versioncheck
	// surfaces are no-ops when NEO4J_CLI_NO_UPDATE_NAG is set; the dice
	// roll can also short-circuit before any network call. None of this
	// is allowed to fail the foreground command — if any of it errors,
	// versioncheck swallows silently.
	formatAndRw := flags.ComposeRootPersistentPreRunE(cfg)
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if err := formatAndRw(cmd, args); err != nil {
			return err
		}
		versioncheck.MaybeHint(cmd, cfg, Version)
		versioncheck.Schedule(cmd.Context(), cfg, Version)
		skillrefresh.MaybeRefresh(cmd.Context(), cmd, cfg, binskill.Bundle, "neo4j-cli")
		return nil
	}

	auraCmd := aura.NewCmd(cfg)
	auraCmd.Use = "aura"
	cmd.AddCommand(auraCmd)
	cmd.AddCommand(credential.NewCredentialCmd(cfg))
	cmd.AddCommand(config.NewCmd(cfg))
	cmd.AddCommand(query.NewCmd(cfg))
	cmd.AddCommand(skill.NewCmd(cfg, binskill.Bundle, "neo4j-cli"))
	cmd.AddCommand(update.NewCmd(cfg, binskill.Bundle, "neo4j-cli"))
	cmd.AddCommand(agentcontext.NewCmd(cfg, Version))

	cobra.EnableTraverseRunHooks = true

	return cmd
}
