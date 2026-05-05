// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill_test

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/skill"
	"github.com/neo4j/cli/test/utils/testfs"
)

const testSkillName = "neo4j-cli"

// fixtureBundle mirrors the shape used by installer_test.go.
func fixtureBundle() fs.FS {
	return fstest.MapFS{
		"SKILL.md": {Data: []byte(`---
name: neo4j-cli
description: test desc
version: {{VERSION}}
---

# neo4j-cli
body
`)},
		"references/aura.md": {Data: []byte("# aura\n")},
	}
}

// fixture is the per-test wiring: a `skill` cobra command, captured
// stdout/stderr buffers, and the in-memory afero.Fs carried by the config.
type fixture struct {
	cmd    *cobra.Command
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	fs     afero.Fs
}

// newFixture wires up a cfg + skill command, populating agent detect dirs
// for every name in agentNames. `output` becomes the `format` config
// value ("default", "json", or "table"). The test HOME is set to homeDir.
func newFixture(t *testing.T, homeDir, output string, agentNames ...string) *fixture {
	t.Helper()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, "xdg"))

	cfgJSON := `{"format":"` + output + `"}`
	memFs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)

	for _, n := range agentNames {
		a := skill.FindAgent(n)
		require.NotNil(t, a, "fixture agent %q must exist", n)
		dp, ok := a.DetectPath()
		require.True(t, ok)
		require.NoError(t, memFs.MkdirAll(dp, 0755))
	}

	cfg := clicfg.NewConfig(memFs, "1.7.0", clicfg.SkillsScope)
	cmd := skill.NewCmd(cfg, fixtureBundle(), testSkillName)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	return &fixture{cmd: cmd, stdout: stdout, stderr: stderr, fs: memFs}
}

// exec runs the skill command with the given argv split.
func (f *fixture) exec(t *testing.T, args ...string) error {
	t.Helper()
	f.cmd.SetArgs(args)
	return f.cmd.Execute()
}

// resetBuffers lets a single fixture run multiple commands.
func (f *fixture) resetBuffers() {
	f.stdout.Reset()
	f.stderr.Reset()
}
