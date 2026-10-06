// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
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

	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	configPath, ok := a.MCPConfigPath()
	require.True(t, ok)
	require.NoError(t, fs.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, afero.WriteFile(fs, configPath, data, 0644))

	// Install neo4j-cli
	require.NoError(t, InstallMCPConfig(fs, a, "/usr/local/bin/neo4j-cli", noGates()))

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

	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	configPath, ok := a.MCPConfigPath()
	require.True(t, ok)
	require.NoError(t, fs.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, afero.WriteFile(fs, configPath, []byte("{}"), 0600))

	// Capture the original mode
	info, err := fs.Stat(configPath)
	require.NoError(t, err)
	origMode := info.Mode()

	require.NoError(t, InstallMCPConfig(fs, a, "/usr/local/bin/neo4j-cli", noGates()))

	info, err = fs.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, origMode, info.Mode(), "file mode must be preserved after atomic write")
}

func TestWriteMCPConfig_NewFileCreated(t *testing.T) {
	fs := afero.NewMemMapFs()
	t.Setenv("HOME", "/Users/test")

	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	configPath, ok := a.MCPConfigPath()
	require.True(t, ok)
	require.NoError(t, fs.MkdirAll(filepath.Dir(configPath), 0755))

	// No config file exists yet
	require.NoError(t, InstallMCPConfig(fs, a, "/usr/local/bin/neo4j-cli", noGates()))

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

	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	configPath, ok := a.MCPConfigPath()
	require.True(t, ok)
	require.NoError(t, fs.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, afero.WriteFile(fs, configPath, data, 0644))

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
	a := &Agent{
		Name:      "claude-desktop",
		DetectDir: "$APP_SUPPORT/Claude",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	detectDir, ok := a.DetectPath()
	require.True(t, ok)
	require.NoError(t, fs.MkdirAll(detectDir, 0755))

	require.NoError(t, InstallMCPConfig(fs, a, "/usr/local/bin/neo4j-cli", noGates()))

	installs := MCPList(fs)
	require.Len(t, installs, 1, "only claude-desktop is MCP-capable")
	assert.Equal(t, "claude-desktop", installs[0].Agent.Name)
	assert.True(t, installs[0].Detected)
	assert.True(t, installs[0].Installed)
	assert.Equal(t, "/usr/local/bin/neo4j-cli", installs[0].InstalledVersion)
	assert.True(t, installs[0].InstalledHasMCPManifest, "fresh install must have manifest env marker")
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

	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	configPath, ok := a.MCPConfigPath()
	require.True(t, ok)
	require.NoError(t, fs.MkdirAll(filepath.Dir(configPath), 0755))
	require.NoError(t, afero.WriteFile(fs, configPath, data, 0644))

	require.NoError(t, InstallMCPConfig(fs, a, "/opt/homebrew/bin/neo4j-cli", noGates()))

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
	assertFullEnv(t, entry, false, false, false)
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

// TestWriteMCPConfig_CrossPlatform proves the surgical merge works on
// darwin, windows and linux by exercising the currentGOOS seam. The actual
// path resolution per platform is independently tested by
// TestClaudeDesktopPathsPerPlatform; this test proves the merge logic itself
// is portable.
func TestWriteMCPConfig_CrossPlatform(t *testing.T) {
	for _, tc := range []struct {
		name string
		goos string
		env  map[string]string
	}{
		{
			name: "darwin",
			goos: "darwin",
			env:  map[string]string{"HOME": "/Users/test", "APPDATA": ""},
		},
		{
			name: "windows",
			goos: "windows",
			env: map[string]string{
				"HOME":    filepath.FromSlash("C:/Users/test"),
				"APPDATA": filepath.FromSlash("C:/Users/test/AppData/Roaming"),
			},
		},
		{
			name: "linux",
			goos: "linux",
			env:  map[string]string{"HOME": "/home/test", "APPDATA": ""},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setGOOSForTest(t, tc.goos)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			t.Setenv("XDG_CONFIG_HOME", "")

			fs := afero.NewMemMapFs()

			// Realistic fixture with preferences, globalShortcut and a
			// pre-existing mcpServers entry.
			existing := map[string]any{
				"globalShortcut": "Option+Space",
				"preferences": map[string]any{
					"theme":  "dark",
					"locale": "en-US",
				},
				"mcpServers": map[string]any{
					"neo4j-data-modeling": map[string]any{
						"command": "/some/other/tool",
						"args":    []any{"--port", "8080"},
					},
				},
			}
			data, err := json.MarshalIndent(existing, "", "  ")
			require.NoError(t, err)

			a := &Agent{
				Name:      "claude-desktop",
				MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
			}
			configPath, ok := a.MCPConfigPath()
			require.True(t, ok, "MCPConfigPath must resolve for %s", tc.goos)
			require.NoError(t, fs.MkdirAll(filepath.Dir(configPath), 0755))
			require.NoError(t, afero.WriteFile(fs, configPath, data, 0644))

			require.NoError(t, InstallMCPConfig(fs, a, "/usr/local/bin/neo4j-cli", noGates()))

			raw, err := afero.ReadFile(fs, configPath)
			require.NoError(t, err)

			var result map[string]any
			require.NoError(t, json.Unmarshal(raw, &result))

			// Verify surgical merge: preferences, globalShortcut and
			// pre-existing mcpServers all survive.
			prefs, ok := result["preferences"].(map[string]any)
			require.True(t, ok, "preferences must survive")
			assert.Equal(t, "dark", prefs["theme"])
			assert.Equal(t, "en-US", prefs["locale"])
			assert.Equal(t, "Option+Space", result["globalShortcut"])

			servers, ok := result["mcpServers"].(map[string]any)
			require.True(t, ok)
			_, ok = servers["neo4j-data-modeling"]
			require.True(t, ok, "pre-existing mcpServers entry must survive")

			entry, ok := servers["neo4j-cli"].(map[string]any)
			require.True(t, ok, "neo4j-cli entry must be present")
			assert.Equal(t, "/usr/local/bin/neo4j-cli", entry["command"])
			args, ok := entry["args"].([]any)
			require.True(t, ok)
			assert.Equal(t, []any{"mcp", "serve"}, args)
			assertFullEnv(t, entry, false, false, false)
		})
	}
}

func TestReadMCPConfigEnv(t *testing.T) {
	fs := afero.NewMemMapFs()
	t.Setenv("HOME", "/Users/test")

	a := &Agent{
		Name:      "claude-desktop",
		MCPConfig: "$APP_SUPPORT/Claude/claude_desktop_config.json",
	}
	configPath, ok := a.MCPConfigPath()
	require.True(t, ok)
	require.NoError(t, fs.MkdirAll(filepath.Dir(configPath), 0755))

	t.Run("no config file", func(t *testing.T) {
		assert.False(t, readMCPConfigEnv(fs, a), "absent file -> false")
	})

	// Write config with neo4j-cli entry but no env block
	t.Run("no env block", func(t *testing.T) {
		fs2 := afero.NewMemMapFs()
		t.Setenv("HOME", "/Users/test")
		cp, ok := a.MCPConfigPath()
		require.True(t, ok)
		require.NoError(t, fs2.MkdirAll(filepath.Dir(cp), 0755))
		config := map[string]any{
			"mcpServers": map[string]any{
				"neo4j-cli": map[string]any{
					"command": "/usr/local/bin/neo4j-cli",
					"args":    []any{"mcp", "serve"},
				},
			},
		}
		data, err := json.MarshalIndent(config, "", "  ")
		require.NoError(t, err)
		require.NoError(t, afero.WriteFile(fs2, cp, data, 0644))
		assert.False(t, readMCPConfigEnv(fs2, a), "entry without env -> false")
	})

	t.Run("env block missing manifest marker", func(t *testing.T) {
		fs2 := afero.NewMemMapFs()
		t.Setenv("HOME", "/Users/test")
		cp, ok := a.MCPConfigPath()
		require.True(t, ok)
		require.NoError(t, fs2.MkdirAll(filepath.Dir(cp), 0755))
		config := map[string]any{
			"mcpServers": map[string]any{
				"neo4j-cli": map[string]any{
					"command": "/usr/local/bin/neo4j-cli",
					"args":    []any{"mcp", "serve"},
					"env": map[string]any{
						"NEO4J_CLI_FLAG_MCP_SERVER": "1",
					},
				},
			},
		}
		data, err := json.MarshalIndent(config, "", "  ")
		require.NoError(t, err)
		require.NoError(t, afero.WriteFile(fs2, cp, data, 0644))
		assert.False(t, readMCPConfigEnv(fs2, a), "env without manifest marker -> false")
	})

	t.Run("env block with manifest marker", func(t *testing.T) {
		fs2 := afero.NewMemMapFs()
		t.Setenv("HOME", "/Users/test")
		cp, ok := a.MCPConfigPath()
		require.True(t, ok)
		require.NoError(t, fs2.MkdirAll(filepath.Dir(cp), 0755))
		config := map[string]any{
			"mcpServers": map[string]any{
				"neo4j-cli": map[string]any{
					"command": "/usr/local/bin/neo4j-cli",
					"args":    []any{"mcp", "serve"},
					"env": map[string]any{
						EnvMCPManifest: "1",
					},
				},
			},
		}
		data, err := json.MarshalIndent(config, "", "  ")
		require.NoError(t, err)
		require.NoError(t, afero.WriteFile(fs2, cp, data, 0644))
		assert.True(t, readMCPConfigEnv(fs2, a), "env with manifest marker -> true")
	})

	t.Run("entry absent", func(t *testing.T) {
		fs2 := afero.NewMemMapFs()
		t.Setenv("HOME", "/Users/test")
		cp, ok := a.MCPConfigPath()
		require.True(t, ok)
		require.NoError(t, fs2.MkdirAll(filepath.Dir(cp), 0755))
		config := map[string]any{
			"mcpServers": map[string]any{"other-tool": map[string]any{"command": "/x"}},
		}
		data, err := json.MarshalIndent(config, "", "  ")
		require.NoError(t, err)
		require.NoError(t, afero.WriteFile(fs2, cp, data, 0644))
		assert.False(t, readMCPConfigEnv(fs2, a), "no neo4j-cli entry -> false")
	})
}

// noGates returns the default all-off gate set for test brevity.
func noGates() MCPGates { return MCPGates{} }

// assertFullEnv verifies that the neo4j-cli entry's env block has all five
// keys with the expected gate values and the invariant "1" keys.
func assertFullEnv(t *testing.T, entry map[string]any, wantWrites, wantAura, wantCredentialWrite bool) {
	t.Helper()
	env, ok := entry["env"].(map[string]any)
	require.True(t, ok, "neo4j-cli entry must have an env block")

	assert.Equal(t, "1", env[EnvMCPFeatureFlag], "%s must always be 1", EnvMCPFeatureFlag)
	assert.Equal(t, "1", env[EnvMCPManifest], "%s must always be 1", EnvMCPManifest)

	wantWritesStr := "false"
	if wantWrites {
		wantWritesStr = "true"
	}
	assert.Equal(t, wantWritesStr, env[EnvMCPAllowWrites], "%s", EnvMCPAllowWrites)

	wantAuraStr := "false"
	if wantAura {
		wantAuraStr = "true"
	}
	assert.Equal(t, wantAuraStr, env[EnvMCPAllowAura], "%s", EnvMCPAllowAura)

	wantCredentialStr := "false"
	if wantCredentialWrite {
		wantCredentialStr = "true"
	}
	assert.Equal(t, wantCredentialStr, env[EnvMCPAllowCredentialWrite], "%s", EnvMCPAllowCredentialWrite)
}
