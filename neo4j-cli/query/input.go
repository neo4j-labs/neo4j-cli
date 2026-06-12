// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

// readPositionalOrStdin returns text supplied either as a single positional
// argument or via piped stdin. Precedence: a non-empty positional arg always
// wins; otherwise the function falls back to reading the stdin pipe through
// the package-level stdinReader seam and dbconn.StdinIsTTY. A TTY-attached stdin with
// no positional arg returns a usage error referencing the supplied input
// label so callers can produce consistent diagnostics across the package.
//
// The label is interpolated into both the empty-arg-on-TTY error ("no <label>
// provided: ...") and the empty-stdin-on-pipe error ("no <label> provided on
// stdin"), keeping the error wording stable across `query` (label="Cypher")
// and `query :embed` (label="text").
func readPositionalOrStdin(_ *cobra.Command, args []string, label string) (string, error) {
	if len(args) == 1 {
		s := strings.TrimSpace(args[0])
		if s == "" {
			return "", clierr.NewUsageError("%s is empty", label)
		}
		return s, nil
	}
	if dbconn.StdinIsTTY() {
		return "", clierr.NewUsageError(
			"no %s provided: pass a positional argument or pipe a value on stdin", label)
	}
	b, err := io.ReadAll(stdinReader())
	if err != nil {
		return "", fmt.Errorf("query: read stdin: %w", err)
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", clierr.NewUsageError("no %s provided on stdin", label)
	}
	return s, nil
}
