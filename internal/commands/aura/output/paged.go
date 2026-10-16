// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"fmt"

	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

// PrintPagedRecords prints the records a paginated list walked and narrates on
// stderr when the walk stopped before the end. noun names the resources in the
// truncation note ("virtual graphs"), limit is the --limit the user passed,
// and the two flags are the page's stop reasons. A complete walk prints the
// records and says nothing, so any note on stderr is the signal that the list
// on stdout is not the whole set.
func PrintPagedRecords(cmd *cobra.Command, cfg *clicfg.Config, records []map[string]any, fields []string, noun string, limit int, limitReached, pageCapReached bool) {
	PrintRecords(cmd, cfg, records, fields)

	if limitReached {
		fmt.Fprintf(cmd.ErrOrStderr(), "Showing the first %d %s; more are available. Raise or omit --limit to see them all.\n", limit, noun) //nolint:errcheck // narration to stderr; write errors are not actionable
	}
	if pageCapReached {
		fmt.Fprintf(cmd.ErrOrStderr(), "Stopped after %d pages, so this list may be incomplete.\n", auraclient.MaxListPages) //nolint:errcheck // narration to stderr; write errors are not actionable
	}
}
