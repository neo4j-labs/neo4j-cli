// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBundleCommand_ProducesFile verifies that `mcp bundle --out <path>`
// produces the .mcpb file at the given path.
func TestBundleCommand_ProducesFile(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "test.mcpb")

	stdout, stderr, err := runApp(t, true, "mcp", "bundle", "--out", outPath, "--rw")
	require.NoError(t, err, "stderr=%s", stderr.String())

	stat, err := os.Stat(outPath)
	require.NoError(t, err, "bundle file must exist at %s", outPath)
	assert.True(t, stat.Size() > 0, "bundle must not be empty")

	// Output mentions the path
	assert.Contains(t, stdout.String(), outPath)
}

// TestBundleCommand_ErrorsWithoutOut ensures --out is required.
func TestBundleCommand_ErrorsWithoutOut(t *testing.T) {
	_, _, err := runApp(t, true, "mcp", "bundle", "--rw")
	require.Error(t, err)
	// With SilenceErrors=true, cobra returns the required-flag error via
	// cmd.Execute() rather than printing it to stderr.
	assert.Contains(t, err.Error(), "\"out\"")
}

// TestBundleCommand_FlagOffGroupAbsent ensures the group is absent
// when flag.mcp-server is off.
func TestBundleCommand_FlagOffGroupAbsent(t *testing.T) {
	_, _, err := runApp(t, false, "mcp", "bundle")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown command")
}

// TestBundleCommand_Registered verifies the bundle leaf is registered
// when the flag is on.
func TestBundleCommand_Registered(t *testing.T) {
	root := newAppCmd(t, true)
	group := findSubcommand(root, "mcp")
	require.NotNil(t, group)

	bundle := findSubcommand(group, "bundle")
	require.NotNil(t, bundle)
	assert.False(t, bundle.Hidden, "bundle must not be hidden")
	assert.NotEmpty(t, bundle.Short)
	assert.NotEmpty(t, bundle.Long)
	assert.NotEmpty(t, bundle.Example)
	assert.NotNil(t, bundle.RunE)
}
