// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"os"
	"testing"

	"github.com/neo4j/cli/internal/output"
)

// TestMain keeps output resolution independent of where the tests run: without
// it, a developer terminal or an agent harness (CLAUDECODE, AI_AGENT) would
// change the default format and break every test that parses JSON.
func TestMain(m *testing.M) {
	output.IsAgent = func() bool { return false }
	output.StdoutIsTerminal = func() bool { return false }
	os.Exit(m.Run())
}
