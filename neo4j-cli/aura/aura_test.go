// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStandaloneCmdRegistersRwFlag(t *testing.T) {
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	cmd := NewStandaloneCmd(cfg)

	flag := cmd.PersistentFlags().Lookup("rw")
	require.NotNil(t, flag)
	assert.Equal(t, "false", flag.DefValue)
	assert.Contains(t, flag.Usage, "Allow write operations")
}

func TestNewCmdDoesNotRegisterRwFlag(t *testing.T) {
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	cmd := NewCmd(cfg)

	assert.Nil(t, cmd.PersistentFlags().Lookup("rw"))
}
