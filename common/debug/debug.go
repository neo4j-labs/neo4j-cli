// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package debug provides the single source of truth for resolving the
// `--debug` flag / `NEO4J_DEBUG` env var semantics shared by the query and
// aura command trees.
package debug

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/common/output"
)

// EnvVar is the environment variable consulted when `--debug` was not
// explicitly set on the command line.
const EnvVar = "NEO4J_DEBUG"

// Resolve merges the `--debug` flag with the NEO4J_DEBUG env var. When
// `--debug` was explicitly set on the command line (flag Changed), its boolean
// value wins (so `--debug=false` overrides NEO4J_DEBUG=1). Otherwise debug is
// enabled iff NEO4J_DEBUG == "1" (strict — any other value, including "true" /
// "yes" / "on" / "0", leaves debug OFF). Dotenv is intentionally not consulted;
// only os.Getenv is read.
func Resolve(cmd *cobra.Command) bool {
	if f := cmd.Flag("debug"); f != nil && f.Changed {
		return f.Value.String() == "true"
	}
	return os.Getenv(EnvVar) == "1"
}

// Scrub redacts secrets (RedactText) then neutralises terminal control/ANSI
// escapes (StripControl) before any string is written to the operator's
// terminal. Order matters: redact first, strip on the result. It is the shared
// primitive for the aura and docker --debug diagnostics.
func Scrub(s string) string {
	return output.StripControl(clievents.RedactText(s))
}
