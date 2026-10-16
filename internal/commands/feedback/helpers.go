// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"encoding/json"
	"os"
	"runtime"
	"time"

	"github.com/neo4j/cli/internal/agent"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/output"
	"github.com/spf13/cobra"
)

// Seams for tests.
var (
	now          = time.Now
	detectAgent  = agent.Detect
	harnessLabel = agent.Harness
	lookupEnv    = os.Getenv
	goos         = runtime.GOOS
	goarch       = runtime.GOARCH
	invokerLabel = agent.Invoker
)

// writeJSON prints v as one indented JSON document on the command's stdout,
// regardless of --format. Only `submit` uses it: its outcome is one document an
// agent parses, and stdout must stay a single document.
func writeJSON(cmd *cobra.Command, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = cmd.OutOrStdout().Write(append(b, '\n'))
	return err
}

// writeStatus prints the feedback state in the format --format resolves to:
// json and toon from the struct, table as field/value rows.
func writeStatus(cmd *cobra.Command, cfg *clicfg.Config, st statusObj) error {
	output.PrintBodyMap(cmd, cfg, st, statusFields)
	return nil
}
