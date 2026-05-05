// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const skillNameForTests = "neo4j-cli"

// fixtureInstallerBundle returns a fs.FS shaped like a real per-binary
// bundle: SKILL.md with the {{VERSION}} placeholder + two reference files
// (which must NOT be substituted).
func fixtureInstallerBundle() fs.FS {
	return fstest.MapFS{
		"SKILL.md": {Data: []byte(`---
name: neo4j-cli
description: test desc
version: {{VERSION}}
---

# neo4j-cli

body
`)},
		"references/aura.md":  {Data: []byte("# aura\n{{VERSION}} stays here\n")},
		"references/skill.md": {Data: []byte("# skill\n")},
	}
}

// setupHomeWithAgents creates HOME/.claude (claude-code) and HOME/.cursor
// (cursor) so DetectAgents returns those two. Returns the configured fs.
func setupHomeWithAgents(t *testing.T, home string, names ...string) afero.Fs {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	memFs := afero.NewMemMapFs()
	for _, n := range names {
		a := FindAgent(n)
		require.NotNil(t, a, "fixture agent %q must be in catalog", n)
		dp, ok := a.DetectPath()
		require.True(t, ok)
		require.NoError(t, memFs.MkdirAll(dp, 0755))
	}
	return memFs
}

func TestInstallNoAgentsDetected(t *testing.T) {
	t.Setenv("HOME", "/home/alice")
	t.Setenv("XDG_CONFIG_HOME", "")

	memFs := afero.NewMemMapFs()
	_, err := Install(memFs, fixtureInstallerBundle(), skillNameForTests, "1.0.0", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoAgentsDetected)
}

func TestInstallAllDetected(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code", "cursor")

	got, err := Install(memFs, fixtureInstallerBundle(), skillNameForTests, "1.2.3", "")
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Both agents got the bundle.
	for _, agentName := range []string{"claude-code", "cursor"} {
		a := FindAgent(agentName)
		require.NotNil(t, a)
		sp, ok := a.SkillsPath()
		require.True(t, ok)

		skillFile := filepath.Join(sp, skillNameForTests, "SKILL.md")
		data, err := afero.ReadFile(memFs, skillFile)
		require.NoError(t, err, "SKILL.md missing for %s at %s", agentName, skillFile)
		assert.Contains(t, string(data), "version: 1.2.3", "{{VERSION}} not substituted in %s SKILL.md", agentName)
		assert.NotContains(t, string(data), versionPlaceholder, "placeholder should be replaced for %s", agentName)

		refFile := filepath.Join(sp, skillNameForTests, "references", "aura.md")
		refData, err := afero.ReadFile(memFs, refFile)
		require.NoError(t, err, "references/aura.md missing for %s", agentName)
		// References must NOT be substituted — only SKILL.md.
		assert.Contains(t, string(refData), "{{VERSION}} stays here", "references should not be substituted")
	}
}

func TestInstallSingleAgentByName(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code", "cursor")

	got, err := Install(memFs, fixtureInstallerBundle(), skillNameForTests, "9.9.9", "Claude-Code")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "claude-code", got[0].Name)

	// Cursor must NOT have been touched.
	cursor := FindAgent("cursor")
	cursorSkill, _ := cursor.SkillsPath()
	exists, _ := afero.Exists(memFs, filepath.Join(cursorSkill, skillNameForTests, "SKILL.md"))
	assert.False(t, exists, "cursor was not the target — should not be installed")
}

func TestInstallUnknownAgent(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code")

	_, err := Install(memFs, fixtureInstallerBundle(), skillNameForTests, "1", "vscode")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownAgent)
}

func TestInstallSingleAgentNotDetected(t *testing.T) {
	t.Setenv("HOME", "/home/alice")
	t.Setenv("XDG_CONFIG_HOME", "")
	memFs := afero.NewMemMapFs() // no agent dirs created

	_, err := Install(memFs, fixtureInstallerBundle(), skillNameForTests, "1", "claude-code")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotDetected)
}

func TestInstallOverwritesExisting(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code")

	a := FindAgent("claude-code")
	sp, _ := a.SkillsPath()
	dst := filepath.Join(sp, skillNameForTests)
	stale := filepath.Join(dst, "references", "stale.md")
	require.NoError(t, memFs.MkdirAll(filepath.Dir(stale), 0755))
	require.NoError(t, afero.WriteFile(memFs, stale, []byte("stale"), 0600))

	_, err := Install(memFs, fixtureInstallerBundle(), skillNameForTests, "2", "claude-code")
	require.NoError(t, err)

	// Stale ref must be gone — Install cleans before copying.
	exists, _ := afero.Exists(memFs, stale)
	assert.False(t, exists, "stale reference should be removed by reinstall")

	// Fresh references present.
	freshRef := filepath.Join(dst, "references", "aura.md")
	exists, _ = afero.Exists(memFs, freshRef)
	assert.True(t, exists)
}

func TestInstallEmptySkillName(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code")
	_, err := Install(memFs, fixtureInstallerBundle(), "", "1", "claude-code")
	require.Error(t, err)
}

func TestInstallNilBundle(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code")
	_, err := Install(memFs, nil, skillNameForTests, "1", "claude-code")
	require.Error(t, err)
}

func TestRemoveAllDetected(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code", "cursor")
	_, err := Install(memFs, fixtureInstallerBundle(), skillNameForTests, "1", "")
	require.NoError(t, err)

	got, err := Remove(memFs, skillNameForTests, "")
	require.NoError(t, err)
	assert.Len(t, got, 2)

	for _, n := range []string{"claude-code", "cursor"} {
		a := FindAgent(n)
		sp, _ := a.SkillsPath()
		exists, _ := afero.DirExists(memFs, filepath.Join(sp, skillNameForTests))
		assert.False(t, exists, "skill dir should be gone for %s", n)
	}
}

func TestRemoveIdempotent(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code")

	// Never installed — first remove succeeds.
	_, err := Remove(memFs, skillNameForTests, "claude-code")
	require.NoError(t, err)

	// Second remove also succeeds.
	_, err = Remove(memFs, skillNameForTests, "claude-code")
	require.NoError(t, err)
}

func TestRemoveUnknownAgent(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code")
	_, err := Remove(memFs, skillNameForTests, "vscode")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownAgent)
}

func TestRemoveEmptySkillName(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code")
	_, err := Remove(memFs, "", "claude-code")
	require.Error(t, err)
}

func TestList(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code", "cursor")

	// Install only into claude-code, leaving cursor detected-but-uninstalled.
	_, err := Install(memFs, fixtureInstallerBundle(), skillNameForTests, "1.7.0", "claude-code")
	require.NoError(t, err)

	rows, err := List(memFs, skillNameForTests)
	require.NoError(t, err)
	require.Len(t, rows, len(AGENTS))

	idx := func(name string) int {
		for i, r := range rows {
			if r.Agent.Name == name {
				return i
			}
		}
		t.Fatalf("agent %q not in rows", name)
		return -1
	}

	// claude-code: detected + installed at 1.7.0
	cc := rows[idx("claude-code")]
	assert.True(t, cc.Detected)
	assert.True(t, cc.Installed)
	assert.Equal(t, "1.7.0", cc.InstalledVersion)

	// cursor: detected but not installed
	cu := rows[idx("cursor")]
	assert.True(t, cu.Detected)
	assert.False(t, cu.Installed)
	assert.Equal(t, "", cu.InstalledVersion)

	// windsurf (untouched): not detected, not installed
	w := rows[idx("windsurf")]
	assert.False(t, w.Detected)
	assert.False(t, w.Installed)
}

func TestCheckNoDrift(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code")
	_, err := Install(memFs, fixtureInstallerBundle(), skillNameForTests, "1.0.0", "claude-code")
	require.NoError(t, err)

	rows, drift, err := Check(memFs, skillNameForTests, "1.0.0")
	require.NoError(t, err)
	assert.False(t, drift)
	require.Len(t, rows, 1)
	assert.Equal(t, "ok", rows[0].Status)
	assert.Equal(t, "1.0.0", rows[0].InstalledVersion)
	assert.Equal(t, "1.0.0", rows[0].CurrentVersion)
}

func TestCheckDrift(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code")
	_, err := Install(memFs, fixtureInstallerBundle(), skillNameForTests, "0.9.0", "claude-code")
	require.NoError(t, err)

	rows, drift, err := Check(memFs, skillNameForTests, "1.0.0")
	require.NoError(t, err)
	assert.True(t, drift)
	require.Len(t, rows, 1)
	assert.Equal(t, "drift", rows[0].Status)
	assert.Equal(t, "0.9.0", rows[0].InstalledVersion)
}

func TestCheckMissing(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code")
	// Nothing installed.

	rows, drift, err := Check(memFs, skillNameForTests, "1.0.0")
	require.NoError(t, err)
	assert.False(t, drift)
	assert.Empty(t, rows, "Check returns rows only for installed agents")
}

func TestCheckUnknownVersion(t *testing.T) {
	memFs := setupHomeWithAgents(t, "/home/alice", "claude-code")
	a := FindAgent("claude-code")
	sp, _ := a.SkillsPath()
	skillFile := filepath.Join(sp, skillNameForTests, "SKILL.md")
	require.NoError(t, memFs.MkdirAll(filepath.Dir(skillFile), 0755))
	require.NoError(t, afero.WriteFile(memFs, skillFile, []byte("no frontmatter here\n"), 0600))

	rows, drift, err := Check(memFs, skillNameForTests, "1.0.0")
	require.NoError(t, err)
	assert.True(t, drift)
	require.Len(t, rows, 1)
	assert.Equal(t, "unknown-version", rows[0].Status)
	assert.Equal(t, "", rows[0].InstalledVersion)
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"standard frontmatter", "---\nname: x\nversion: 1.2.3\n---\n", "1.2.3"},
		{"placeholder still present", "---\nversion: {{VERSION}}\n---\n", "{{VERSION}}"},
		{"leading whitespace tolerated", "---\n  version:   2.0.0\n---\n", "2.0.0"},
		{"missing version", "---\nname: x\n---\n", ""},
		{"empty value", "---\nversion:\n---\n", ""},
		{"empty doc", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseVersion([]byte(tc.in))
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSubstituteVersion(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		version string
		want    string
	}{
		{"replaces placeholder", "version: {{VERSION}}\n", "1.0.0", "version: 1.0.0\n"},
		{"empty version is no-op", "version: {{VERSION}}\n", "", "version: {{VERSION}}\n"},
		{"replaces multiple occurrences", "{{VERSION}} and {{VERSION}}", "1", "1 and 1"},
		{"no placeholder unchanged", "no placeholder here", "1.0.0", "no placeholder here"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := substituteVersion([]byte(tc.in), tc.version)
			assert.Equal(t, tc.want, string(got))
		})
	}
}
