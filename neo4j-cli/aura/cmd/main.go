// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/aura"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

var Version = "dev"

// recoverPanic prints a redacted "unexpected error" line to w and re-panics.
// Extracted so the redaction format is unit-testable without invoking main().
func recoverPanic(w io.Writer, args []string, r any) {
	fmt.Fprintf(w, "Unexpected error running CLI with args %s, please report an issue in https://github.com/neo4j/cli\n\n", clievents.RedactArgs(args)) //nolint:errcheck // best-effort write before re-panic
	panic(r)
}

// exitCodeFor mirrors neo4j-cli/main.go's helper: typed *clierr.CLIError
// errors pass their Code through (via errors.As); other non-nil errors fall
// back to 1; nil returns 0.
func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	var ce *clierr.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return 1
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			recoverPanic(os.Stdout, os.Args[1:], r)
		}
	}()

	cfg := clicfg.NewConfig(afero.NewOsFs(), Version, clicfg.AuraScope)

	cmd := aura.NewStandaloneCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.PersistentPreRunE = flags.ComposeRootPersistentPreRunE(cfg)
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
		os.Exit(exitCodeFor(err))
	}
	// add metrics callback for success here
}
