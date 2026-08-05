// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"

	commonoutput "github.com/neo4j/cli/common/output"
	auraapi "github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/spf13/cobra"
)

// outputFlags holds the flags that shape what is written about the response
// rather than what is requested.
type outputFlags struct {
	include bool
	silent  bool
}

func registerOutputFlags(cmd *cobra.Command, f *outputFlags) {
	cmd.Flags().BoolVarP(&f.include, "include", "i", false,
		"Print the HTTP status line and the response headers before the body. Makes stdout no longer a single JSON document, and is skipped on a failing status so the error envelope stays the only document there.")
	cmd.Flags().BoolVar(&f.silent, "silent", false,
		"Do not print the response body; with --include the headers are still printed.")
}

// mergeQuery overlays the --field-derived params on the endpoint's inline query,
// so an explicit flag wins over a value copied along with the path.
func mergeQuery(inline, fields url.Values) url.Values {
	merged := make(url.Values, len(inline)+len(fields))
	maps.Copy(merged, inline)
	maps.Copy(merged, fields)

	return merged
}

// printResponseMeta writes the status line and the response headers, sorted by
// name and control-stripped: both are upstream-controlled text reaching a
// terminal, and net/http filters neither the reason phrase nor a header value.
func printResponseMeta(cmd *cobra.Command, res *auraapi.RawResponse) {
	w := cmd.OutOrStdout()

	statusLine := strings.TrimSpace(fmt.Sprintf("%s %s", res.Proto, res.Status))
	_, _ = fmt.Fprintln(w, commonoutput.StripControl(statusLine))

	for _, name := range slices.Sorted(maps.Keys(res.Header)) {
		for _, value := range res.Header[name] {
			_, _ = fmt.Fprintf(w, "%s: %s\n", commonoutput.StripControl(name), commonoutput.StripControl(value))
		}
	}
	_, _ = fmt.Fprintln(w)
}
