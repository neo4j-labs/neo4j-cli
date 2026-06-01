// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/neo4j-cli/app"
	"github.com/spf13/afero"
)

// recoverPanic prints a redacted "unexpected error" line to w.
// Extracted so the redaction format is unit-testable without invoking main().
//
// When the recovered value implements `error`, its `Error()` text is written
// on its own line BEFORE the fallback line. This surfaces panic diagnostics
// (e.g. unhandled API status codes that reach `response.go`'s default panic)
// without requiring a rebuild. Non-error panic values keep the single-line
// behaviour.
func recoverPanic(w io.Writer, args []string, r any) {
	if err, ok := r.(error); ok {
		fmt.Fprintf(w, "%s\n", err.Error()) //nolint:errcheck // best-effort write before re-panic
	}
	fmt.Fprintf(w, "Unexpected error running CLI with args %s, please report an issue in %s\n\n", clievents.RedactArgs(args), clierr.IssuesURL) //nolint:errcheck // best-effort write before re-panic
}

// exitCodeFor maps an error returned by cmd.Execute to a process exit code.
// Typed *clierr.CLIError values pass their Code through (via errors.As, so the
// code still surfaces through arbitrary fmt.Errorf("...: %w", ce) wrapping);
// any other non-nil error falls back to 1. A nil error returns 0.
// Extracted from main so it can be unit-tested without invoking the binary.
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

// resolveFormatForRender chooses the --format value used to render an error.
//
// When cobra's flag-parse step fails (e.g. `--bad-flag`), PersistentPreRunE
// never runs and viper's "format" key keeps its "default" seed even if the
// user passed `--format=json` on the command line. To keep the JSON envelope
// contract honest for failed parses, fall back to a raw scan of args when
// the bound format is empty or "default". The returned value is one of
// clicfg.ValidFormatValues or, when no usable hint is found, the original
// bound value.
func resolveFormatForRender(args []string, bound string) string {
	if bound != "" && bound != "default" {
		return bound
	}
	if peeked := peekFormatFromArgs(args); peeked != "" {
		return peeked
	}
	return bound
}

// peekFormatFromArgs scans args for `--format=<v>` or `--format <v>` and
// returns the value when it matches clicfg.ValidFormatValues. Used only as
// the flag-parse-failure fallback in resolveFormatForRender; the canonical
// binding happens in PersistentPreRunE via flags.BindFormatFromFlag.
func peekFormatFromArgs(args []string) string {
	for i, arg := range args {
		var val string
		switch {
		case strings.HasPrefix(arg, "--format="):
			val = strings.TrimPrefix(arg, "--format=")
		case arg == "--format" && i+1 < len(args):
			val = args[i+1]
		default:
			continue
		}
		for _, v := range clicfg.ValidFormatValues {
			if val == v {
				return val
			}
		}
	}
	return ""
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
	err := cmd.Execute()
	if err != nil {
		clievents.Emit(cfg.Events, os.Args[1:], false)
	} else {
		clievents.Emit(cfg.Events, os.Args[1:], true)
	}
	cfg.Events.Flush() // Send out any remaining events

	if err != nil {
		// Intercept confirm.ErrCancelled before render: cancellation is exit-0,
		// not exit-1, and the helper already wrote "cancelled." to stderr.
		if errors.Is(err, confirm.ErrCancelled) {
			os.Exit(0)
		}
		format := resolveFormatForRender(os.Args[1:], cfg.Global.Format())
		clierr.Render(err, os.Stdout, os.Stderr, format)
		os.Exit(exitCodeFor(err))
	}
}
