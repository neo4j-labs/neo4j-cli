// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEphemeralKind(t *testing.T) {
	prev := tempDir
	t.Cleanup(func() { tempDir = prev })
	tempDir = func() string { return "/tmp" }

	for _, tc := range []struct {
		name, path, want string
	}{
		// ephemeral: seen in the route check (uvx), plus the same family
		{"uvx on Linux", "/home/u/.cache/uv/archive-v0/Mu39h33IX3EjBk_H/lib/python3.13/site-packages/neo4j_cli/bin/neo4j-cli", "the uv cache (what `uvx` runs from)"},
		{"uvx on macOS", "/Users/u/Library/Caches/uv/archive-v0/abc/lib/python3.13/site-packages/neo4j_cli/bin/neo4j-cli", "the uv cache (what `uvx` runs from)"},
		{"uvx on Windows", `C:\Users\u\AppData\Local\uv\cache\archive-v0\abc\Lib\site-packages\neo4j_cli\bin\neo4j-cli.exe`, "the uv cache (what `uvx` runs from)"},
		{"uv ephemeral environment", "/home/u/.cache/uv/environments-v2/abc/bin/neo4j-cli", "the uv cache (what `uvx` runs from)"},
		{"pipx run", "/home/u/.local/pipx/.cache/abc/lib/python3.13/site-packages/neo4j_cli/bin/neo4j-cli", "the pipx cache (what `pipx run` runs from)"},
		{"npx", "/home/u/.npm/_npx/123abc/node_modules/@neo4j-labs/cli-linux-arm64/bin/neo4j-cli", "the npx cache"},
		{"temporary folder", "/tmp/download/neo4j-cli", "a temporary folder"},
		// normal installs: must not warn
		{"uv tool install", "/home/u/.local/share/uv/tools/neo4j-cli/lib/python3.13/site-packages/neo4j_cli/bin/neo4j-cli", ""},
		{"uv tool on Windows", `C:\Users\u\AppData\Roaming\uv\tools\neo4j-cli\Lib\site-packages\neo4j_cli\bin\neo4j-cli.exe`, ""},
		{"pipx install", "/home/u/.local/pipx/venvs/neo4j-cli/lib/python3.13/site-packages/neo4j_cli/bin/neo4j-cli", ""},
		{"pip in a venv", "/home/u/venv/lib/python3.13/site-packages/neo4j_cli/bin/neo4j-cli", ""},
		{"global npm", "/home/u/.npm-global/lib/node_modules/@neo4j-labs/cli/node_modules/@neo4j-labs/cli-linux-arm64/bin/neo4j-cli", ""},
		{"homebrew link", "/opt/homebrew/bin/neo4j-cli", ""},
		{"install script", "/home/u/.local/bin/neo4j-cli", ""},
		{"a folder merely named like the temp folder", "/tmp2/neo4j-cli", ""},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ephemeralKind(tc.path))
		})
	}
}

// onlyClaude makes `claude` the only agent found on the PATH (fakeClaude finds every binary).
func onlyClaude(t *testing.T) {
	t.Helper()
	lookPath = func(name string) (string, error) {
		if name == claudeBinary {
			return "/usr/bin/claude", nil
		}
		return "", exec.ErrNotFound
	}
}

func TestEphemeralWarning_AvoidsPhrasesTheInstallScriptMatches(t *testing.T) {
	w := ephemeralWarning("/x/neo4j-cli", "the uv cache (what `uvx` runs from)")
	assert.NotContains(t, w, "registration failed")
	assert.NotContains(t, w, `"claude-code"`)
	assert.NotContains(t, w, `"codex"`)
	assert.Contains(t, w, "feedback register")
	assert.Contains(t, w, "uv tool install neo4j-cli")
}

func TestRegisterAgents_WarnsWhenTheBinaryIsInAnEphemeralPlaceAndStillRegisters(t *testing.T) {
	calls := fakeClaude(t, false, false)
	onlyClaude(t)
	executablePath = func() (string, error) {
		return "/home/u/.cache/uv/archive-v0/abc/lib/python3.13/site-packages/neo4j_cli/bin/neo4j-cli", nil
	}
	agents, notes := registerAgents(t.Context(), "basic")
	assert.Equal(t, []string{claudeCodeAgent}, agents, "a warning does not stop the registration")
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "uv cache")
	assert.Contains(t, notes[0], "/home/u/.cache/uv/archive-v0/abc/")
	assert.Contains(t, strings.Join(*calls, "\n"), "mcp add -s user neo4j-feedback", "the tool is still registered")
}

func TestRegisterAgents_NoWarningForANormalInstall(t *testing.T) {
	fakeClaude(t, false, false) // the fake binary lives at /opt/neo4j-cli
	onlyClaude(t)
	_, notes := registerAgents(t.Context(), "basic")
	assert.Empty(t, notes)
}

func TestRegisterAgents_WarnsOnceWhenBothAgentsAreRegistered(t *testing.T) {
	fakeClaude(t, false, false)
	prevCodex := execCodex
	t.Cleanup(func() { execCodex = prevCodex })
	execCodex = func(context.Context, ...string) error { return nil }
	lookPath = func(string) (string, error) { return "/usr/bin/agent", nil }
	executablePath = func() (string, error) { return "/home/u/.npm/_npx/1/node_modules/x/bin/neo4j-cli", nil }
	agents, notes := registerAgents(t.Context(), "basic")
	assert.ElementsMatch(t, []string{claudeCodeAgent, codexAgent}, agents)
	require.Len(t, notes, 1, "one warning, not one per agent")
	assert.Contains(t, notes[0], "npx cache")
}

func TestRegisterCmd_SurfacesTheWarningInItsOutput(t *testing.T) {
	withSeams(t, false, "")
	fakeClaude(t, false, false)
	onlyClaude(t)
	executablePath = func() (string, error) { return "/home/u/.cache/uv/archive-v0/abc/bin/neo4j-cli", nil }
	cfg := newTestConfig(t)
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "register", "--rw", "--endpoint", loopback)
	require.NoError(t, err)
	notes, _ := parseJSON(t, stdout)["notes"].([]any)
	joined := ""
	for _, n := range notes {
		joined += n.(string) + "\n"
	}
	assert.Contains(t, joined, "uv cache")
	assert.Contains(t, joined, "Not enabled yet")
}

func TestEphemeralKind_PnpmDlxAndBunx(t *testing.T) {
	assert.Contains(t, ephemeralKind("/home/u/.cache/pnpm/dlx/abc/node_modules/.bin/neo4j-cli"), "pnpm")
	assert.Contains(t, ephemeralKind("/home/u/.bun/install/cache/@neo4j-labs/cli/bin/neo4j-cli"), "bun")
	assert.Empty(t, ephemeralKind("/home/u/.bun/bin/neo4j-cli"), "a global bun install is permanent")
}

func TestUnderDir_IsCaseInsensitiveOnWindows(t *testing.T) {
	prev := goos
	t.Cleanup(func() { goos = prev })
	goos = "windows"
	sep := string(filepath.Separator)
	assert.True(t, underDir(sep+"Users"+sep+"U"+sep+"AppData"+sep+"Local"+sep+"Temp"+sep+"x", sep+"users"+sep+"u"+sep+"appdata"+sep+"local"+sep+"temp"))
}
