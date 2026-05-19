// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"bytes"
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
)

// PrintBodyMap is a shim that delegates to common/output.PrintBodyMap so that
// all existing call sites in subcommands/ continue to compile without import changes.
func PrintBodyMap(cmd *cobra.Command, cfg *clicfg.Config, values output.ResponseData, fields []string) {
	output.PrintBodyMap(cmd, cfg, values, fields)
}

// PrintBody parses the raw response body and then calls PrintBodyMap.
func PrintBody(cmd *cobra.Command, cfg *clicfg.Config, body []byte, fields []string) {
	if len(body) == 0 {
		return
	}
	values := api.ParseBody(body)

	PrintBodyMap(cmd, cfg, values, fields)
}

// PrintRawBody prints a bare-JSON response body (no `{"data": ...}` envelope).
// Used for endpoints (e.g. the Aura Agents API) whose response shape is a bare
// array or a bare object at the top level. In JSON output mode the raw body is
// re-indented and printed verbatim, preserving fidelity. In any other mode the
// body is parsed via `api.ParseRawBody` and rendered through `PrintBodyMap`.
func PrintRawBody(cmd *cobra.Command, cfg *clicfg.Config, body []byte, fields []string) {
	if len(body) == 0 {
		return
	}
	if output.ResolveOutput(cmd, cfg) == "json" {
		var buf bytes.Buffer
		if err := json.Indent(&buf, body, "", "\t"); err != nil {
			panic(err)
		}
		cmd.Println(buf.String())
		return
	}
	PrintBodyMap(cmd, cfg, api.ParseRawBody(body), fields)
}
