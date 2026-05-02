// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/config"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// neo4jTestHelper is a minimal test helper for the neo4j root command,
// analogous to AuraTestHelper but wired to the neo4j config package.
type neo4jTestHelper struct {
	out *bytes.Buffer
	err *bytes.Buffer
	cfg string
	fs  afero.Fs
	t   *testing.T
}

func newNeo4jTestHelper(t *testing.T) neo4jTestHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true

	return neo4jTestHelper{
		t:   t,
		out: bytes.NewBufferString(""),
		err: bytes.NewBufferString(""),
		cfg: `{}`,
	}
}

func (h *neo4jTestHelper) setConfigValue(key string, value interface{}) {
	updated, err := sjson.Set(h.cfg, key, value)
	assert.Nil(h.t, err)
	h.cfg = updated
}

func (h *neo4jTestHelper) executeCommand(command string) {
	h.out = bytes.NewBufferString("")
	h.err = bytes.NewBufferString("")

	args, err := shlex.Split(command)
	assert.Nil(h.t, err)

	fs, err := testfs.GetTestFs(h.cfg, `{}`)
	assert.Nil(h.t, err)
	h.fs = fs

	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	// Build a minimal neo4j root command with just the config subcommand
	rootCmd := &cobra.Command{
		Use: "neo4j-cli",
	}
	aura.RegisterOutputFlag(rootCmd, cfg)
	rootCmd.AddCommand(config.NewCmd(cfg))

	rootCmd.SetArgs(args)
	rootCmd.SetOut(h.out)
	rootCmd.SetErr(h.err)

	rootCmd.Execute() //nolint:errcheck // cobra prints the error itself; test assertions check output
}

func (h *neo4jTestHelper) assertOut(expected string) {
	out, err := io.ReadAll(h.out)
	assert.Nil(h.t, err)
	assert.Equal(h.t, strings.TrimSpace(expected), strings.TrimSpace(string(out)))
}

func (h *neo4jTestHelper) assertErr(expected string) {
	out, err := io.ReadAll(h.err)
	assert.Nil(h.t, err)
	assert.Equal(h.t, strings.TrimSpace(expected), strings.TrimSpace(string(out)))
}

func (h *neo4jTestHelper) assertConfigValue(key string, expected string) {
	file, err := h.fs.Open(filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "config.json"))
	assert.Nil(h.t, err)
	defer file.Close() //nolint:errcheck // in-memory FS close error is not actionable in a defer

	raw, err := io.ReadAll(file)
	assert.Nil(h.t, err)

	actual := gjson.GetBytes(raw, key).String()
	assert.Equal(h.t, expected, actual)
}
