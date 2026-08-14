// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/skill"
	"github.com/neo4j/cli/neo4j-cli/app"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck_RegistersLeaf(t *testing.T) {
	root := newAppCmdEveryFlagEnabled(t)
	mcpGroup := findSubcommand(root, "mcp")
	require.NotNil(t, mcpGroup)

	check := findSubcommand(mcpGroup, "check")
	require.NotNil(t, check, "mcp check must be registered")
	assert.False(t, check.Hidden)
}

func TestCheck_NoInstalledServers(t *testing.T) {
	// With no agents installed and a pipe stdout, the output is JSON.
	stdout, stderr, err := runMCPApp(t, false, "mcp", "check")
	require.NoError(t, err, "stderr=%s", stderr.String())
	// JSON array or friendly message depending on TTY; either is fine.
	t.Logf("check output: %s", stdout.String())
}

func TestCheck_JSONOutput(t *testing.T) {
	// JSON output should be a valid array.
	stdout, stderr, err := runMCPApp(t, false, "mcp", "check", "--format", "json")
	require.NoError(t, err, "stderr=%s", stderr.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &rows))
	_ = rows
}

func TestCheck_JSONFieldsAreSnakeCase(t *testing.T) {
	stdout, stderr, err := runMCPApp(t, true, "mcp", "check", "--format", "json")
	require.NoError(t, err, "stderr=%s", stderr.String())

	var rows []map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &rows))

	for _, row := range rows {
		for key := range row {
			assert.Regexp(t, `^[a-z][a-z0-9_]*$`, key, "JSON key %q must be snake_case", key)
		}
	}
}

func TestCheck_ExitInfo(t *testing.T) {
	// mcp check is not write-annotated, so it should work without --rw.
	stdout, stderr, err := runMCPApp(t, false, "mcp", "check")
	require.NoError(t, err, "check should not need --rw")
	_ = stdout
	_ = stderr
}

// writeCheckConfig writes a neo4j-cli entry to the agent config on fs, then
// runs "mcp check --format json" and decodes the result rows.
// Returns (rows, err) — callers that expect drift should not assert no error.
func writeCheckConfigAndRun(t *testing.T, fs afero.Fs, entry map[string]any) ([]map[string]any, error) {
	t.Helper()
	t.Setenv("HOME", "/Users/test")

	a := skill.FindAgent("claude-desktop")
	require.NotNil(t, a)
	claudeDir, ok := a.DetectPath()
	require.True(t, ok)
	require.NoError(t, fs.MkdirAll(claudeDir, 0755))

	configPath, ok := a.MCPConfigPath()
	require.True(t, ok)
	require.NoError(t, fs.MkdirAll(filepath.Dir(configPath), 0755))

	if entry != nil {
		// Override command to the actual test binary so the path comparison
		// in runCheckCmd doesn't produce a false-positive drift.
		binPath, err := os.Executable()
		require.NoError(t, err)
		entry["command"] = binPath

		cfg := map[string]any{
			"mcpServers": map[string]any{
				"neo4j-cli": entry,
			},
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		require.NoError(t, err)
		require.NoError(t, afero.WriteFile(fs, configPath, data, 0644))
	}

	// Build tree with all flags on so mcp subtree is visible.
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	for name := range clicfg.Registry {
		cfg.Flags.SetForTest(name, true)
	}
	root := app.NewCmd(cfg)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.SetArgs([]string{"mcp", "check", "--format", "json"})
	execErr := root.Execute()

	var rows []map[string]any
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &rows); unmarshalErr != nil {
		return rows, execErr
	}
	return rows, execErr
}

func TestCheck_EnvDrift_NoEnvBlock(t *testing.T) {
	fs := afero.NewMemMapFs()
	entry := map[string]any{
		"command": "/usr/local/bin/neo4j-cli",
		"args":    []any{"mcp", "serve"},
		// no env block
	}
	rows, err := writeCheckConfigAndRun(t, fs, entry)
	require.Error(t, err, "drift must cause non-zero exit")
	require.Len(t, rows, 1)
	assert.Equal(t, "drift", rows[0]["status"], "missing env block must report drift")
	assert.False(t, rows[0]["has_mcp_env_manifest"].(bool), "has_mcp_env_manifest must be false")
}

func TestCheck_EnvDrift_MissingManifestMarker(t *testing.T) {
	fs := afero.NewMemMapFs()
	entry := map[string]any{
		"command": "/usr/local/bin/neo4j-cli",
		"args":    []any{"mcp", "serve"},
		"env": map[string]any{
			"NEO4J_CLI_FLAG_MCP_SERVER": "1",
			// no NEO4J_CLI_MCP_MANIFEST
		},
	}
	rows, err := writeCheckConfigAndRun(t, fs, entry)
	require.Error(t, err, "drift must cause non-zero exit")
	require.Len(t, rows, 1)
	assert.Equal(t, "drift", rows[0]["status"], "env without manifest marker must report drift")
}

func TestCheck_EnvDrift_FreshInstallOK(t *testing.T) {
	fs := afero.NewMemMapFs()
	entry := map[string]any{
		"command": "/usr/local/bin/neo4j-cli",
		"args":    []any{"mcp", "serve"},
		"env": map[string]any{
			skill.EnvMCPManifest:             "1",
			skill.EnvMCPFeatureFlag:          "1",
			skill.EnvMCPAllowWrites:          "false",
			skill.EnvMCPAllowAura:            "false",
			skill.EnvMCPAllowCredentialWrite: "false",
		},
	}
	rows, err := writeCheckConfigAndRun(t, fs, entry)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "ok", rows[0]["status"], "entry with manifest marker must report ok")
	assert.True(t, rows[0]["has_mcp_env_manifest"].(bool), "has_mcp_env_manifest must be true")
}

func TestCheck_EnvDrift_ExitCode(t *testing.T) {
	// When drift is detected, mcp check exits non-zero via clierr.
	fs := afero.NewMemMapFs()
	t.Setenv("HOME", "/Users/test")

	a := skill.FindAgent("claude-desktop")
	require.NotNil(t, a)
	claudeDir, ok := a.DetectPath()
	require.True(t, ok)
	require.NoError(t, fs.MkdirAll(claudeDir, 0755))

	configPath, ok := a.MCPConfigPath()
	require.True(t, ok)
	require.NoError(t, fs.MkdirAll(filepath.Dir(configPath), 0755))

	cfg := map[string]any{
		"mcpServers": map[string]any{
			"neo4j-cli": map[string]any{
				"command": "/usr/local/bin/neo4j-cli",
				"args":    []any{"mcp", "serve"},
				// no env block
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	require.NoError(t, err)
	require.NoError(t, afero.WriteFile(fs, configPath, data, 0644))

	cfgObj := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	for name := range clicfg.Registry {
		cfgObj.Flags.SetForTest(name, true)
	}
	root := app.NewCmd(cfgObj)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"mcp", "check", "--format", "json"})

	err = root.Execute()
	require.Error(t, err, "drift must cause non-zero exit")
	_ = stdout
	_ = stderr
}
