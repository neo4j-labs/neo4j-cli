// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"fmt"
	"os"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/config"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

var Version = "dev"

func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "neo4j-cli",
		Short:   "Allows you to manage Neo4j resources",
		Version: Version,
	}

	aura.RegisterOutputFlag(cmd, cfg)

	auraCmd := aura.NewCmd(cfg)
	auraCmd.Use = "aura"
	cmd.AddCommand(auraCmd)
	cmd.AddCommand(aura.NewCredentialCmd(cfg))
	cmd.AddCommand(config.NewCmd(cfg))
	return cmd
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Unexpected error running CLI with args %s, please report an issue in https://github.com/neo4j/cli\n\n", os.Args[1:])

			panic(r)
		}
	}()

	cfg := clicfg.NewConfig(afero.NewOsFs(), Version, clicfg.GlobalScope)

	cmd := NewCmd(cfg)
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	origHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		fmt.Printf("[neo4j-cli] help displayed: %s\n", c.CommandPath()) // TODO: remove this log in favour of real metrics on help displayed
		origHelp(c, args)
	})

	cobra.EnableTraverseRunHooks = true

	// cobra prints the error itself; we only add the hook for errors that bypassed
	// both RunE and HelpFunc (e.g. unknown top-level command via legacyArgs in Find).
	if err := cmd.Execute(); err != nil {
		fmt.Printf("[neo4j-cli] invalid command with args %s: %v\n", os.Args[1:], err) // TODO: remove this log in favour of real metrics in case of invalid command
	} else {
		fmt.Printf("[neo4j-cli] command executed successfully with args %s\n", os.Args[1:]) // TODO: remove this log in favour of real metrics on successful command execution
	}
}
