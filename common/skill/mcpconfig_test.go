// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteMCPConfig_SurgicalMerge(t *testing.T) {
	fs := afero.NewMemMapFs()
	t.Setenv("HOME", "/Users/test")

	// Simulate a realistic claude_desktop_config.json with preferences,
	// globalShortcut and a pre-existing mcpServers entry.
	existing := map[string]any{
		"globalShortcut": "Option+Space",
		"preferences": map[string]any{
			"theme":  "dark",
			"locale": "en-US",
		},
		"coworkUserFilesPath": "/Users/test/.claude/cowork",
		"mcpServers": map[string]any{
			"neo4j-data-modeling": map[string]any{
				"command": "/some/other/tool",
				"args":    []any{"--port", "8080"},
			},
		},
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	require.NoError(t, err)

	claudeDir := "/Users/test/Library/Application Support/Claude"
	configPath := claudeDir + "/claude_desktop_config.json"
	require.NoError(t, fs.MkdirAll(claudeDir, 0755))
	require.NoError(t, afero.WriteFile(fs, configPath, data, 0644))

	// Install neo4j-cli
	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	require.NoError(t, InstallMCPConfig(fs, a, "/usr/local/bin/neo4j-cli"))

	// Read back and verify surgical merge
	raw, err := afero.ReadFile(fs, configPath)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(raw, &result))

	// Preferences and globalShortcut are preserved
	prefs, ok := result["preferences"].(map[string]any)
	require.True(t, ok, "preferences must survive")
	assert.Equal(t, "dark", prefs["theme"])
	assert.Equal(t, "en-US", prefs["locale"])
	assert.Equal(t, "Option+Space", result["globalShortcut"])
	assert.Equal(t, "/Users/test/.claude/cowork", result["coworkUserFilesPath"])

	// Pre-existing mcpServers entry is preserved
	servers, ok := result["mcpServers"].(map[string]any)
	require.True(t, ok)
	dataModeling, ok := servers["neo4j-data-modeling"].(map[string]any)
	require.True(t, ok, "neo4j-data-modeling entry must survive")
	assert.Equal(t, "/some/other/tool", dataModeling["command"])

	// neo4j-cli entry is present
	neo4jCli, ok := servers["neo4j-cli"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/usr/local/bin/neo4j-cli", neo4jCli["command"])
	args, ok := neo4jCli["args"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"mcp", "serve"}, args)
}

func TestWriteMCPConfig_AtomicWritePreservesMode(t *testing.T) {
	fs := afero.NewMemMapFs()
	t.Setenv("HOME", "/Users/test")

	claudeDir := "/Users/test/Library/Application Support/Claude"
	configPath := claudeDir + "/claude_desktop_config.json"
	require.NoError(t, fs.MkdirAll(claudeDir, 0755))
	require.NoError(t, afero.WriteFile(fs, configPath, []byte("{}"), 0600))

	// Capture the original mode
	info, err := fs.Stat(configPath)
	require.NoError(t, err)
	origMode := info.Mode()

	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	require.NoError(t, InstallMCPConfig(fs, a, "/usr/local/bin/neo4j-cli"))

	info, err = fs.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, origMode, info.Mode(), "file mode must be preserved after atomic write")
}

func TestWriteMCPConfig_NewFileCreated(t *testing.T) {
	fs := afero.NewMemMapFs()
	t.Setenv("HOME", "/Users/test")

	claudeDir := "/Users/test/Library/Application Support/Claude"
	configPath := claudeDir + "/claude_desktop_config.json"
	require.NoError(t, fs.MkdirAll(claudeDir, 0755))

	// No config file exists yet
	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	require.NoError(t, InstallMCPConfig(fs, a, "/usr/local/bin/neo4j-cli"))

	raw, err := afero.ReadFile(fs, configPath)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(raw, &result))

	servers, ok := result["mcpServers"].(map[string]any)
	require.True(t, ok)
	entry, ok := servers["neo4j-cli"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/usr/local/bin/neo4j-cli", entry["command"])
}

func TestRemoveMCPConfig_Idempotent(t *testing.T) {
	fs := afero.NewMemMapFs()

	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}

	// Remove when nothing exists — idempotent, no error
	assert.NoError(t, RemoveMCPConfig(fs, a))

	// Remove after removing — still idempotent
	assert.NoError(t, RemoveMCPConfig(fs, a))

	// Remove from an agent that doesn't support MCP
	skillAgent := &Agent{Name: "claude-code", SkillsDir: "~/.claude/skills"}
	assert.NoError(t, RemoveMCPConfig(fs, skillAgent))
}

func TestRemoveMCPConfig_RemovesEntry(t *testing.T) {
	fs := afero.NewMemMapFs()
	t.Setenv("HOME", "/Users/test")

	config := map[string]any{
		"preferences": map[string]any{"theme": "light"},
		"mcpServers": map[string]any{
			"neo4j-cli": map[string]any{
				"command": "/usr/local/bin/neo4j-cli",
				"args":    []any{"mcp", "serve"},
			},
		},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	require.NoError(t, err)

	claudeDir := "/Users/test/Library/Application Support/Claude"
	configPath := claudeDir + "/claude_desktop_config.json"
	require.NoError(t, fs.MkdirAll(claudeDir, 0755))
	require.NoError(t, afero.WriteFile(fs, configPath, data, 0644))

	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	require.NoError(t, RemoveMCPConfig(fs, a))

	raw, err := afero.ReadFile(fs, configPath)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(raw, &result))

	// Preferences must survive
	prefs, ok := result["preferences"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "light", prefs["theme"])

	// neo4j-cli entry must be gone
	servers, _ := result["mcpServers"].(map[string]any)
	if servers != nil {
		_, exists := servers["neo4j-cli"]
		assert.False(t, exists, "neo4j-cli entry must be removed")
	}
}

func TestMCPList_OrderAndDetection(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Set HOME and create a detect dir for claude-desktop to emulate detection
	t.Setenv("HOME", "/Users/test")
	claudeDir := "/Users/test/Library/Application Support/Claude"
	require.NoError(t, fs.MkdirAll(claudeDir, 0755))

	// Install neo4j-cli
	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	require.NoError(t, InstallMCPConfig(fs, a, "/usr/local/bin/neo4j-cli"))

	installs := MCPList(fs)
	require.Len(t, installs, 1, "only claude-desktop is MCP-capable")
	assert.Equal(t, "claude-desktop", installs[0].Agent.Name)
	assert.True(t, installs[0].Detected)
	assert.True(t, installs[0].Installed)
	assert.Equal(t, "/usr/local/bin/neo4j-cli", installs[0].InstalledVersion)
}

func TestMCPList_NotDetected(t *testing.T) {
	fs := afero.NewMemMapFs()
	t.Setenv("HOME", "/Users/test")

	installs := MCPList(fs)
	require.Len(t, installs, 1, "only claude-desktop is MCP-capable")
	assert.Equal(t, "claude-desktop", installs[0].Agent.Name)
	assert.False(t, installs[0].Detected, "no detect dir -> not detected")
	assert.False(t, installs[0].Installed, "no config -> not installed")
}

// Test golden: merging into a realistic claude_desktop_config.json preserves
// preferences, globalShortcut and a pre-existing mcpServers entry.
// This is the acceptance-criteria golden test referenced in the task.
func TestMCPConfig_SurgicalMergeGolden(t *testing.T) {
	fs := afero.NewMemMapFs()
	t.Setenv("HOME", "/Users/test")

	// Realistic config with all the things that must survive
	existing := map[string]any{
		"globalShortcut":      "Ctrl+Shift+Space",
		"coworkUserFilesPath": "/Users/test/.claude/cowork",
		"preferences": map[string]any{
			"theme":          "light",
			"locale":         "en-US",
			"fontSize":       float64(14),
			"notifications":  true,
			"autoUpdate":     true,
			"customCommands": []any{},
		},
		"mcpServers": map[string]any{
			"neo4j-data-modeling": map[string]any{
				"command": "/path/to/neo4j-data-modeling",
				"args":    []any{},
			},
		},
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	require.NoError(t, err)

	claudeDir := "/Users/test/Library/Application Support/Claude"
	configPath := claudeDir + "/claude_desktop_config.json"
	require.NoError(t, fs.MkdirAll(claudeDir, 0755))
	require.NoError(t, afero.WriteFile(fs, configPath, data, 0644))

	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	require.NoError(t, InstallMCPConfig(fs, a, "/opt/homebrew/bin/neo4j-cli"))

	raw, err := afero.ReadFile(fs, configPath)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(raw, &result))

	// Assert preferences survive
	prefs, ok := result["preferences"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "light", prefs["theme"])
	assert.Equal(t, "en-US", prefs["locale"])
	assert.Equal(t, float64(14), prefs["fontSize"])
	assert.Equal(t, true, prefs["notifications"])
	assert.Equal(t, true, prefs["autoUpdate"])

	// Assert globalShortcut survives
	assert.Equal(t, "Ctrl+Shift+Space", result["globalShortcut"])

	// Assert co-worker path survives
	assert.Equal(t, "/Users/test/.claude/cowork", result["coworkUserFilesPath"])

	// Assert pre-existing mcpServers entry survives
	servers, ok := result["mcpServers"].(map[string]any)
	require.True(t, ok)
	dataModeling, ok := servers["neo4j-data-modeling"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/path/to/neo4j-data-modeling", dataModeling["command"])

	// Assert neo4j-cli entry is present
	entry, ok := servers["neo4j-cli"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/opt/homebrew/bin/neo4j-cli", entry["command"])
	args, ok := entry["args"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"mcp", "serve"}, args)
}

// TestExtractAgentName exercises extracting the agent name from error messages.
func TestExtractAgentName(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{msg: `skill: unknown agent: "claude-desktop"`, want: "claude-desktop"},
		{msg: `skill: unknown agent: "nope"`, want: "nope"},
		{msg: "skill: unknown agent: no-quotes", want: "no-quotes"},
		{msg: "some other error", want: ""},
	}
	for _, tc := range tests {
		err := os.NewSyscallError("test", nil) // just something to wrap
		wrapped := &mockErr{msg: tc.msg, err: err}
		got := extractAgentName(wrapped)
		assert.Equal(t, tc.want, got, "extractAgentName(%q)", tc.msg)
	}
}

type mockErr struct {
	msg string
	err error
}

func (e *mockErr) Error() string        { return e.msg }
func (e *mockErr) Unwrap() error        { return e.err }
func (e *mockErr) Is(target error) bool { return target == e.err }
