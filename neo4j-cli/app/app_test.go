// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package app

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdEnablesTraverseRunHooks(t *testing.T) {
	cobra.EnableTraverseRunHooks = false
	t.Cleanup(func() { cobra.EnableTraverseRunHooks = true })

	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	NewCmd(cfg)

	assert.True(t, cobra.EnableTraverseRunHooks,
		"EnableTraverseRunHooks must be true so PersistentPreRunE hooks on root "+
			"(e.g. format flag binding) are not shadowed by hooks on child commands")
}
