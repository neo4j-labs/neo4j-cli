// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/testutil/testfs"
)

func configFilePath() string {
	return filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "config.json")
}

func TestLoad_CorruptConfigFileIsAnErrorNamingTheFile(t *testing.T) {
	fs, err := testfs.GetTestFs(`{"format": "json"`, `{}`) // unterminated JSON
	require.NoError(t, err)

	var cfg *clicfg.Config
	require.NotPanics(t, func() { cfg, err = clicfg.Load(fs, "test") })

	require.Error(t, err)
	assert.Nil(t, cfg)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "want a typed error, got %T", err)
	assert.Equal(t, 1, ce.Code)
	assert.Contains(t, ce.Message, configFilePath(), "the message names the file to fix")
	assert.Contains(t, ce.Message, "Fix or remove the file")
}

func TestLoad_CreatesTheConfigFileOnFirstRun(t *testing.T) {
	fs := afero.NewMemMapFs()

	cfg, err := clicfg.Load(fs, "test")

	require.NoError(t, err)
	require.NotNil(t, cfg)
	exists, err := afero.Exists(fs, configFilePath())
	require.NoError(t, err)
	assert.True(t, exists, "first run writes the default config")
}

func TestNewConfig_PanicsWhereLoadReturnsAnError(t *testing.T) {
	fs, err := testfs.GetTestFs(`not json`, `{}`)
	require.NoError(t, err)

	assert.Panics(t, func() { clicfg.NewConfig(fs, "test") })
}
