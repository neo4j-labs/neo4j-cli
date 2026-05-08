// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/neo4j-cli/app"
	"github.com/spf13/afero"
)

// recoverPanic prints a redacted "unexpected error" line to w and re-panics.
// Extracted so the redaction format is unit-testable without invoking main().
func recoverPanic(w io.Writer, args []string, r any) {
	fmt.Fprintf(w, "Unexpected error running CLI with args %s, please report an issue in https://github.com/neo4j/cli\n\n", clievents.RedactArgs(args)) //nolint:errcheck // best-effort write before re-panic
	panic(r)
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			recoverPanic(os.Stdout, os.Args[1:], r)
		}
	}()

	cfg := clicfg.NewConfig(afero.NewOsFs(), app.Version, clicfg.GlobalScope)

	// This is fake command that we use to emit startup.
	// This event allows us to easily measure installation base

	clievents.Emit(cfg.Events, []string{"startup"}, true)

	cmd := app.NewCmd(cfg)
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	// cobra prints the error itself; we only add the hook for errors that bypassed
	// both RunE and HelpFunc (e.g. unknown top-level command via legacyArgs in Find).
	if err := cmd.Execute(); err != nil {
		clievents.Emit(cfg.Events, os.Args[1:], false)
	} else {
		clievents.Emit(cfg.Events, os.Args[1:], true)
	}
	cfg.Events.Flush() // Send out any remaining events

}
