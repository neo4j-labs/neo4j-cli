// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"os"
	"testing"

	commonoutput "github.com/neo4j/cli/internal/output"
)

// TestMain seeds IsAgent=false because the dev/CI-under-Claude env sets
// CLAUDECODE, which agent.Detect() reads. Left true, it flips the
// default-format resolution paths the doctor render tests exercise to toon.
func TestMain(m *testing.M) {
	commonoutput.IsAgent = func() bool { return false }
	os.Exit(m.Run())
}
