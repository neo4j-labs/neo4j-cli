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

func appWithFlag(t *testing.T, on bool) bool {
	t.Helper()
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test")
	cfg.Flags.SetForTest("flag.feedback-command", on)
	for _, c := range cli.NewCmd(cfg).Commands() {
		if c.Name() == "feedback" {
			return true
		}
	}
	return false
}

func TestFeedbackGroupIsMountedOnlyWhenFlagIsOn(t *testing.T) {
	assert.False(t, appWithFlag(t, false), "feedback must not be registered while the flag is off")
	assert.True(t, appWithFlag(t, true), "feedback must be registered when the flag is on")
}
