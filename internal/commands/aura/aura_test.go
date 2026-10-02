// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package aura

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdDoesNotRegisterRwFlag(t *testing.T) {
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	cmd := NewCmd(cfg)

	assert.Nil(t, cmd.PersistentFlags().Lookup("rw"))
}

func TestNewCmd_RejectsRemovedTenantCommand(t *testing.T) {
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	cmd := NewCmd(cfg)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"tenant", "list"})

	execErr := cmd.Execute()
	require.Error(t, execErr)
	assert.Contains(t, strings.ToLower(execErr.Error()), "unknown command")
}
