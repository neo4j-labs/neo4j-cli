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

func (h *neo4jTestHelper) overwriteConfig(cfg string) {
	h.cfg = cfg
}

func (h *neo4jTestHelper) executeCommand(command string) {
	h.out = bytes.NewBufferString("")
	h.err = bytes.NewBufferString("")

	args, err := shlex.Split(command)
	assert.Nil(h.t, err)

	fs, err := testfs.GetTestFs(h.cfg, `{}`)
	assert.Nil(h.t, err)
	h.fs = fs

	cfg := clicfg.NewConfig(fs, "test")

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

// --- Tests ---

func TestConfigGetOutputDefault(t *testing.T) {
	h := newNeo4jTestHelper(t)

	h.executeCommand("config get output")

	// With an empty config the default value is "default"
	h.assertOut("default")
	h.assertErr("")
}

func TestConfigGetOutputWithValueSet(t *testing.T) {
	h := newNeo4jTestHelper(t)
	h.setConfigValue("output", "json")

	h.executeCommand("config get output")

	// Output flag in config is "json" so the rendered output is JSON
	h.assertOut(`{
	"output": "json"
}`)
	h.assertErr("")
}

func TestConfigGetOutputWithOutputFlagJson(t *testing.T) {
	h := newNeo4jTestHelper(t)

	h.executeCommand("config get output --output json")

	// --output json flag binds viper "output" to "json", so both the rendered
	// format and the reported value become "json".
	h.assertOut(`{
	"output": "json"
}`)
	h.assertErr("")
}

func TestConfigGetOutputWithOutputFlagTable(t *testing.T) {
	h := newNeo4jTestHelper(t)

	h.executeCommand("config get output --output table")

	// --output table flag overrides rendering to a table.
	// go-pretty renders header row in uppercase with StyleLight.
	out, err := io.ReadAll(h.out)
	assert.Nil(t, err)
	outStr := string(out)
	assert.Contains(t, outStr, "KEY")
	assert.Contains(t, outStr, "VALUE")
	assert.Contains(t, outStr, "output")
	// The flag binding sets the viper "output" key to "table", so the value shown is "table".
	assert.Contains(t, outStr, "table")
	h.assertErr("")
}

func TestConfigGetInvalidKey(t *testing.T) {
	h := newNeo4jTestHelper(t)

	h.executeCommand("config get invalid-key")

	h.assertErr(`Error: invalid argument "invalid-key" for "neo4j-cli config get"`)
}

func TestConfigSetOutputJson(t *testing.T) {
	h := newNeo4jTestHelper(t)

	h.executeCommand("config set output json")

	h.assertConfigValue("output", "json")
	h.assertErr("")
}

func TestConfigSetOutputTable(t *testing.T) {
	h := newNeo4jTestHelper(t)

	h.executeCommand("config set output table")

	h.assertConfigValue("output", "table")
	h.assertErr("")
}

func TestConfigSetOutputDefault(t *testing.T) {
	h := newNeo4jTestHelper(t)

	h.executeCommand("config set output default")

	h.assertConfigValue("output", "default")
	h.assertErr("")
}

func TestConfigSetOutputInvalidValue(t *testing.T) {
	h := newNeo4jTestHelper(t)

	h.executeCommand("config set output invalid")

	h.assertErr("Error: invalid value for 'output': invalid (valid values: default, json, table)")
}

func TestConfigSetUnknownKey(t *testing.T) {
	h := newNeo4jTestHelper(t)

	h.executeCommand("config set unknown-key value")

	h.assertErr("Error: invalid config key specified: unknown-key")
}

func TestConfigSetMissingArgs(t *testing.T) {
	h := newNeo4jTestHelper(t)

	h.executeCommand("config set output")

	// Only one arg — should get usage error about needing 2 args
	errOut, err := io.ReadAll(h.err)
	assert.Nil(t, err)
	assert.Contains(t, string(errOut), "Error")
}

func TestConfigListDefaultOutput(t *testing.T) {
	h := newNeo4jTestHelper(t)

	h.executeCommand("config list")

	// Default output mode: JSON (since default falls through to JSON in list)
	h.assertOut(`{
	"output": "default"
}`)
	h.assertErr("")
}

func TestConfigListWithOutputFlagJson(t *testing.T) {
	h := newNeo4jTestHelper(t)
	h.setConfigValue("output", "json")

	h.executeCommand("config list --output json")

	h.assertOut(`{
	"output": "json"
}`)
	h.assertErr("")
}

func TestConfigListWithOutputFlagTable(t *testing.T) {
	h := newNeo4jTestHelper(t)

	h.executeCommand("config list --output table")

	out, err := io.ReadAll(h.out)
	assert.Nil(t, err)
	outStr := string(out)
	// go-pretty renders header row in uppercase with StyleLight.
	assert.Contains(t, outStr, "KEY")
	assert.Contains(t, outStr, "VALUE")
	assert.Contains(t, outStr, "output")
	h.assertErr("")
}

func TestConfigListWithTableConfigSetting(t *testing.T) {
	h := newNeo4jTestHelper(t)
	h.setConfigValue("output", "table")

	h.executeCommand("config list")

	out, err := io.ReadAll(h.out)
	assert.Nil(t, err)
	outStr := string(out)
	// Table rendering when the stored output config is "table".
	// go-pretty renders header row in uppercase with StyleLight.
	assert.Contains(t, outStr, "KEY")
	assert.Contains(t, outStr, "VALUE")
	assert.Contains(t, outStr, "output")
	assert.Contains(t, outStr, "table")
	h.assertErr("")
}

func TestConfigMigrationAuraOutputToTopLevel(t *testing.T) {
	// Simulate an old config with "aura.output" set (the pre-migration format).
	// NewConfig should silently migrate it to "output" at the top level.
	h := newNeo4jTestHelper(t)
	h.overwriteConfig(`{"aura": {"output": "json"}}`)

	h.executeCommand("config get output")

	// After migration, the value "json" is at the top-level "output" key.
	// cfg.Global.Output() == "json" so the rendering format is JSON,
	// and cfg.Global.Get("output") == "json" so the shown value is "json".
	h.assertOut(`{
	"output": "json"
}`)
	h.assertErr("")
}

func TestConfigMigrationAuraOutputStoredAtTopLevel(t *testing.T) {
	// After migration, the config file should have "output" at the root.
	h := newNeo4jTestHelper(t)
	h.overwriteConfig(`{"aura": {"output": "table"}}`)

	h.executeCommand("config get output")

	// Confirm the migrated value in the config file
	h.assertConfigValue("output", "table")

	// "aura.output" should be gone
	file, err := h.fs.Open(filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "config.json"))
	assert.Nil(t, err)
	defer file.Close() //nolint:errcheck // in-memory FS close error is not actionable in a defer

	raw, err := io.ReadAll(file)
	assert.Nil(t, err)

	auraOutput := gjson.GetBytes(raw, "aura.output")
	assert.False(t, auraOutput.Exists(), "aura.output should have been removed after migration")
}
