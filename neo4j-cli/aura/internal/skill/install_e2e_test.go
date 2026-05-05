// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonskill "github.com/neo4j/cli/common/skill"
	binskill "github.com/neo4j/cli/neo4j-cli/aura/internal/skill"
)

// TestBundleWalkAtRoot locks the contract that `Bundle` is rooted at the
// bundle/ contents — `fs.WalkDir(Bundle, ".")` must yield `SKILL.md`
// (not `bundle/SKILL.md`).
func TestBundleWalkAtRoot(t *testing.T) {
	var seen []string
	err := fs.WalkDir(binskill.Bundle, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		seen = append(seen, p)
		return nil
	})
	require.NoError(t, err)

	require.Contains(t, seen, "SKILL.md",
		"Bundle should expose SKILL.md at the root; got %v", seen)
	for _, p := range seen {
		assert.NotContains(t, p, "bundle/",
			"Bundle paths must NOT include a leading bundle/ segment; got %q", p)
	}
}

// TestInstallE2E exercises the real exported Bundle through the installer
// against an afero.MemMapFs. Regression test for the bundle-root mismatch
// bug.
func TestInstallE2E(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))

	memFs := afero.NewMemMapFs()
	claudeCode := commonskill.FindAgent("claude-code")
	require.NotNil(t, claudeCode)
	detectDir, ok := claudeCode.DetectPath()
	require.True(t, ok)
	require.NoError(t, memFs.MkdirAll(detectDir, 0755))

	const version = "v9.9.9-test"
	targets, err := commonskill.Install(memFs, binskill.Bundle, "aura-cli", version, "claude-code")
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, "claude-code", targets[0].Name)

	skillsDir, ok := claudeCode.SkillsPath()
	require.True(t, ok)
	skillRoot := filepath.Join(skillsDir, "aura-cli")

	// (a) SKILL.md lands at <skillsDir>/<skillName>/SKILL.md — no stray bundle/ segment.
	skillFile := filepath.Join(skillRoot, "SKILL.md")
	exists, err := afero.Exists(memFs, skillFile)
	require.NoError(t, err)
	require.True(t, exists, "SKILL.md should exist at %s (no bundle/ segment)", skillFile)

	buggy := filepath.Join(skillRoot, "bundle", "SKILL.md")
	exists, err = afero.Exists(memFs, buggy)
	require.NoError(t, err)
	assert.False(t, exists, "buggy nested path %s must not exist", buggy)

	// (b) {{VERSION}} placeholder is replaced.
	data, err := afero.ReadFile(memFs, skillFile)
	require.NoError(t, err)
	body := string(data)
	assert.Contains(t, body, "version: "+version,
		"version frontmatter should be substituted; SKILL.md head:\n%s", head(body, 10))
	assert.NotContains(t, body, "{{VERSION}}",
		"{{VERSION}} placeholder should be gone after install")

	// (c) at least one references/<sub>.md file is written.
	refsDir := filepath.Join(skillRoot, "references")
	refExists, err := afero.DirExists(memFs, refsDir)
	require.NoError(t, err)
	require.True(t, refExists, "references/ dir should exist under %s", skillRoot)

	var refFiles []string
	err = afero.Walk(memFs, refsDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".md") {
			refFiles = append(refFiles, p)
		}
		return nil
	})
	require.NoError(t, err)
	assert.NotEmpty(t, refFiles, "expected at least one references/*.md file")
}

func head(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
