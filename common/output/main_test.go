// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"os"
	"testing"
)

// TestMain seeds IsAgent=false because the dev/CI-under-Claude env sets
// CLAUDECODE, which agent.Detect() reads — left true it would flip the
// default-format resolution tests to toon.
func TestMain(m *testing.M) {
	IsAgent = func() bool { return false }
	os.Exit(m.Run())
}
