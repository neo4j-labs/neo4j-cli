// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAGENTSCatalog(t *testing.T) {
	// Catalog must match the Rust reference (10 agents, in this order).
	// Locking order means stable `skill list` output across releases.
	expected := []string{
		"claude-code", "cursor", "windsurf", "copilot", "gemini-cli",
		"cline", "codex", "pi", "opencode", "junie",
	}
	require.Len(t, AGENTS, len(expected))
	for i, want := range expected {
		assert.Equal(t, want, AGENTS[i].Name, "agent at index %d", i)
		assert.NotEmpty(t, AGENTS[i].DisplayName)
		assert.NotEmpty(t, AGENTS[i].DetectDir)
		assert.NotEmpty(t, AGENTS[i].SkillsDir)
	}
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		home     string
		xdg      string
		xdgUnset bool
		want     string
		wantOk   bool
	}{
		{
			name:   "bare tilde expands to HOME",
			path:   "~",
			home:   "/home/alice",
			want:   "/home/alice",
			wantOk: true,
		},
		{
			name:   "tilde slash prefix expands to HOME/sub",
			path:   "~/.claude",
			home:   "/home/alice",
			want:   filepath.Join("/home/alice", ".claude"),
			wantOk: true,
		},
		{
			name:   "tilde slash deeper path",
			path:   "~/.codeium/windsurf",
			home:   "/home/alice",
			want:   filepath.Join("/home/alice", ".codeium/windsurf"),
			wantOk: true,
		},
		{
			name:   "bare tilde with no HOME returns ok=false",
			path:   "~",
			home:   "",
			wantOk: false,
		},
		{
			name:   "tilde slash with no HOME returns ok=false",
			path:   "~/.claude",
			home:   "",
			wantOk: false,
		},
		{
			name: "XDG set substitutes verbatim",
			path: "$XDG_CONFIG_HOME/opencode",
			home: "/home/alice",
			xdg:  "/home/alice/xdg",
			// Result is OS-native: forward slashes on Unix, backslashes on
			// Windows. expandPath runs the substitution through
			// filepath.FromSlash so the catalog's portable forward-slash
			// path doesn't produce mixed separators on Windows.
			want:   filepath.FromSlash("/home/alice/xdg/opencode"),
			wantOk: true,
		},
		{
			name:     "XDG unset falls back to HOME/.config",
			path:     "$XDG_CONFIG_HOME/opencode",
			home:     "/home/alice",
			xdgUnset: true,
			want:     filepath.Join("/home/alice", ".config", "opencode"),
			wantOk:   true,
		},
		{
			name:     "XDG empty string falls back to HOME/.config",
			path:     "$XDG_CONFIG_HOME/opencode",
			home:     "/home/alice",
			xdg:      "",
			xdgUnset: false,
			want:     filepath.Join("/home/alice", ".config", "opencode"),
			wantOk:   true,
		},
		{
			name:     "XDG fallback with no HOME returns ok=false",
			path:     "$XDG_CONFIG_HOME/opencode",
			home:     "",
			xdgUnset: true,
			wantOk:   false,
		},
		{
			// Regression for the Windows CI failure: the OpenCode catalog
			// entry uses forward slashes, but on Windows xdg resolves to
			// `C:\…\.config` (backslashes). Without filepath.FromSlash the
			// substitution would yield mixed separators
			// (`C:\…\.config/opencode`). Build `xdg` with the OS separator
			// so the test exercises the same shape on every OS, then assert
			// the result contains zero forward slashes on Windows.
			name:   "OpenCode-style mixed-slash input is normalised to OS-native",
			path:   "$XDG_CONFIG_HOME/opencode/skills",
			home:   "/home/alice",
			xdg:    filepath.Join(string(filepath.Separator)+"some", "native", "config"),
			want:   filepath.Join(string(filepath.Separator)+"some", "native", "config", "opencode", "skills"),
			wantOk: true,
		},
		{
			name:   "absolute path unchanged",
			path:   "/etc/agent/skills",
			home:   "/home/alice",
			want:   "/etc/agent/skills",
			wantOk: true,
		},
		{
			name:   "relative path unchanged",
			path:   "relative/path",
			home:   "/home/alice",
			want:   "relative/path",
			wantOk: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", tc.home)
			// Setting XDG_CONFIG_HOME="" is equivalent to unset for our
			// purposes — expandPath treats empty the same as missing.
			if tc.xdgUnset {
				t.Setenv("XDG_CONFIG_HOME", "")
			} else {
				t.Setenv("XDG_CONFIG_HOME", tc.xdg)
			}

			got, ok := expandPath(tc.path)
			assert.Equal(t, tc.wantOk, ok)
			if tc.wantOk {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

// TestExpandPathOpenCodeMixedSlashIsOSNative is an explicit regression
// guard for the Windows CI failure where `expandPath` produced mixed
// separators (`C:\…\.config/opencode`) for OpenCode's catalog entry.
// The catalog entry uses forward slashes (portable convention) but the
// substituted XDG value can be OS-native (backslashes on Windows). Fix:
// run the result through `filepath.FromSlash` so the entire path uses
// the OS separator. On Unix this is a no-op (forward slashes are
// native); on Windows it ensures backslash-only output.
func TestExpandPathOpenCodeMixedSlashIsOSNative(t *testing.T) {
	t.Setenv("HOME", filepath.FromSlash("/home/alice"))
	// Force the OS-native form so the post-substitution path inherits
	// the OS separator.
	t.Setenv("XDG_CONFIG_HOME", filepath.FromSlash("/home/alice/.config"))

	got, ok := expandPath("$XDG_CONFIG_HOME/opencode/skills")
	require.True(t, ok)

	want := filepath.Join(filepath.FromSlash("/home/alice/.config"), "opencode", "skills")
	assert.Equal(t, want, got)

	// On Windows os.PathSeparator is `\`, so the result must contain no
	// `/`. On Unix os.PathSeparator is `/` and this assertion is vacuous.
	if os.PathSeparator != '/' {
		assert.NotContains(t, got, "/",
			"expandPath produced mixed separators (%q) — must run substitution through filepath.FromSlash", got)
	}
}

func TestFindAgent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string // empty means nil
	}{
		{name: "exact lowercase match", input: "claude-code", wantName: "claude-code"},
		{name: "mixed case match", input: "Claude-Code", wantName: "claude-code"},
		{name: "uppercase match", input: "CURSOR", wantName: "cursor"},
		{name: "unknown returns nil", input: "vscode", wantName: ""},
		{name: "empty returns nil", input: "", wantName: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FindAgent(tc.input)
			if tc.wantName == "" {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tc.wantName, got.Name)
		})
	}
}

func TestDetectAgents(t *testing.T) {
	t.Setenv("HOME", "/home/alice")
	t.Setenv("XDG_CONFIG_HOME", "/home/alice/xdg")

	fs := afero.NewMemMapFs()
	// Fake two agent install markers. Build the marker paths the same
	// way expandPath does so MemMapFs lookups match on every OS — XDG
	// substitution runs through filepath.FromSlash so the path is
	// OS-native.
	require.NoError(t, fs.MkdirAll(filepath.Join("/home/alice", ".claude"), 0755))
	require.NoError(t, fs.MkdirAll(filepath.FromSlash("/home/alice/xdg/opencode"), 0755))

	got := DetectAgents(fs)
	require.Len(t, got, 2)
	assert.Equal(t, "claude-code", got[0].Name)
	assert.Equal(t, "opencode", got[1].Name)
}

func TestDetectAgentsEmpty(t *testing.T) {
	t.Setenv("HOME", "/home/alice")
	t.Setenv("XDG_CONFIG_HOME", "/home/alice/xdg")

	fs := afero.NewMemMapFs()
	got := DetectAgents(fs)
	assert.Empty(t, got)
}

func TestDetectAgentsHomeUnset(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	fs := afero.NewMemMapFs()
	got := DetectAgents(fs)
	// All AGENTS reference HOME (directly or via XDG fallback) — unresolvable
	// paths are skipped, so the result is empty.
	assert.Empty(t, got)
}

func TestDetectAgentsIgnoresFile(t *testing.T) {
	// DetectDir must be a directory, not a file. afero.DirExists returns
	// false for files, so a file at the marker path doesn't count as
	// detected.
	t.Setenv("HOME", "/home/alice")
	fs := afero.NewMemMapFs()
	require.NoError(t, afero.WriteFile(fs, filepath.Join("/home/alice", ".claude"), []byte("not a dir"), 0644))

	got := DetectAgents(fs)
	assert.Empty(t, got)
}

func TestAgentDetectAndSkillsPath(t *testing.T) {
	t.Setenv("HOME", "/home/alice")
	t.Setenv("XDG_CONFIG_HOME", "/home/alice/xdg")

	a := FindAgent("claude-code")
	require.NotNil(t, a)

	dp, ok := a.DetectPath()
	require.True(t, ok)
	assert.Equal(t, filepath.Join("/home/alice", ".claude"), dp)

	sp, ok := a.SkillsPath()
	require.True(t, ok)
	assert.Equal(t, filepath.Join("/home/alice", ".claude/skills"), sp)
}
