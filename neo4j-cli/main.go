// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"fmt"
	"os"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/app"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Unexpected error running CLI with args %s, please report an issue in https://github.com/neo4j/cli\n\n", os.Args[1:])

			panic(r)
		}
	}()

	cfg := clicfg.NewConfig(afero.NewOsFs(), app.Version, clicfg.GlobalScope)

	cmd := app.NewCmd(cfg)
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	origHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		// add metrics callback for help here
		origHelp(c, args)
	})

	cobra.EnableTraverseRunHooks = true

	if err := cmd.Execute(); err != nil {
		// add metrics callback for fail here
		os.Exit(1)
	}
	// add metrics callback for success here
}
