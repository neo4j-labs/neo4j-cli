// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"os"
	"testing"

	"github.com/neo4j/cli/internal/flags"
	commonoutput "github.com/neo4j/cli/internal/output"
)

func TestMain(m *testing.M) {
	commonoutput.IsAgent = func() bool { return false }
	flags.ForceNonInteractive()
	os.Exit(m.Run())
}
