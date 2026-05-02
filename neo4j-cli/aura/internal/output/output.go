// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
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
