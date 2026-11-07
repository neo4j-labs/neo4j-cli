// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package operator

import (
	"os"
	"testing"

	"github.com/neo4j/cli/internal/flags"
	commonoutput "github.com/neo4j/cli/internal/output"
)

// TestMain seeds IsAgent=false because the dev/CI-under-Claude env sets
// CLAUDECODE, which agent.Detect() reads. Left true, it flips the
// default-format resolution paths the report render tests exercise to toon.
// ForceNonInteractive pins the --rw TTY probe off, so the write gate is
// deterministic whatever terminal the tests run in.
func TestMain(m *testing.M) {
	commonoutput.IsAgent = func() bool { return false }
	flags.ForceNonInteractive()
	os.Exit(m.Run())
}
