// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server_test

import (
	"os"
	"testing"

	commonoutput "github.com/neo4j/cli/common/output"
)

func TestMain(m *testing.M) {
	commonoutput.IsAgent = func() bool { return false }
	os.Exit(m.Run())
}
