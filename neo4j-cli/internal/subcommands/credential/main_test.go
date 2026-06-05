// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential_test

import (
	"os"
	"testing"

	commonoutput "github.com/neo4j/cli/common/output"
)

// TestMain seeds IsAgent=false because the dev/CI-under-Claude env sets
// CLAUDECODE, which agent.Detect() reads — left true it would flip the
// default-format resolution paths to toon.
func TestMain(m *testing.M) {
	commonoutput.IsAgent = func() bool { return false }
	os.Exit(m.Run())
}
