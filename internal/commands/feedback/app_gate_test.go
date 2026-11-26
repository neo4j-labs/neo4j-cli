// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback_test

import (
	"testing"

	"github.com/neo4j/cli/internal/cli"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The feedback group is always mounted: the installers and registered agent
// tools call it without any flag.
func TestFeedbackGroupIsAlwaysMounted(t *testing.T) {
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test")
	mounted := false
	for _, c := range cli.NewCmd(cfg).Commands() {
		if c.Name() == "feedback" {
			mounted = true
		}
	}
	assert.True(t, mounted, "feedback must be registered on the root command")
}
