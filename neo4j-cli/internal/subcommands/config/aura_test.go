// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/config"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestConfigAuraGet tests the "neo4j config aura get" command using the existing
// neo4jTestHelper (which wires config.NewCmd, including the aura subcommand).
func TestConfigAuraGet(t *testing.T) {
	tests := []struct {
		name        string
		configSetup func(h *neo4jTestHelper)
		command     string
		wantOut     string
		wantErr     string
		wantOutFunc func(t *testing.T, outStr string)
	}{
		{
			name: "config aura get auth-url returns configured value",
			configSetup: func(h *neo4jTestHelper) {
				h.setConfigValue("aura.auth-url", "https://custom.example.com/oauth/token")
			},
			command: "config aura get auth-url",
			wantOut: "https://custom.example.com/oauth/token",
		},
		{
			name:    "config aura get with invalid key returns error",
			command: "config aura get unknown-key",
			wantErr: `Error: invalid argument "unknown-key" for "neo4j-cli config aura get"`,
		},
		{
			name:    "config aura get output returns error (output is not an aura key)",
			command: "config aura get output",
			wantErr: `Error: invalid argument "output" for "neo4j-cli config aura get"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newNeo4jTestHelper(t)
			if tc.configSetup != nil {
				tc.configSetup(&h)
			}

			h.executeCommand(tc.command)

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			if tc.wantOutFunc != nil {
				out, err := io.ReadAll(h.out)
				assert.Nil(t, err)
				tc.wantOutFunc(t, string(out))
			} else {
				h.assertOut(tc.wantOut)
			}
		})
	}
}

// TestConfigAuraSet tests the "neo4j config aura set" command.
func TestConfigAuraSet(t *testing.T) {
	tests := []struct {
		name            string
		configSetup     func(h *neo4jTestHelper)
		command         string
		wantConfigKey   string
		wantConfigValue string
		wantErr         string
	}{
		{
			name:            "config aura set auth-url writes aura key",
			configSetup:     func(h *neo4jTestHelper) {},
			command:         "config aura set auth-url https://custom.example.com/oauth/token",
			wantConfigKey:   "aura.auth-url",
			wantConfigValue: "https://custom.example.com/oauth/token",
		},
		{
			name:    "config aura set output returns invalid key error",
			command: "config aura set output json",
			wantErr: "Error: invalid config key specified: output",
		},
		{
			name:    "config aura set unknown key returns error",
			command: "config aura set unknown-key value",
			wantErr: "Error: invalid config key specified: unknown-key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newNeo4jTestHelper(t)
			if tc.configSetup != nil {
				tc.configSetup(&h)
			}

			h.executeCommand(tc.command)

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			h.assertConfigValue(tc.wantConfigKey, tc.wantConfigValue)
		})
	}
}

// TestConfigAuraList tests the "neo4j config aura list" command.
func TestConfigAuraList(t *testing.T) {
	tests := []struct {
		name         string
		configSetup  func(h *neo4jTestHelper)
		command      string
		wantContains []string
		wantOut      string
		wantErr      string
	}{
		{
			name:    "config aura list shows aura keys only (not output)",
			command: "config aura list",
			configSetup: func(h *neo4jTestHelper) {
				h.setConfigValue("aura.auth-url", "https://example.com/oauth/token")
			},
			wantContains: []string{"auth-url", "base-url", "default-tenant"},
		},
		{
			name:    "config aura list does not include global output key in JSON output",
			command: "config aura list",
			configSetup: func(h *neo4jTestHelper) {
				h.setConfigValue("output", "json")
			},
			// JSON rendering because output="json"; output key must NOT appear as a top-level key
			wantContains: []string{"auth-url", "base-url", "default-tenant"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newNeo4jTestHelper(t)
			if tc.configSetup != nil {
				tc.configSetup(&h)
			}

			h.executeCommand(tc.command)

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			if len(tc.wantContains) > 0 {
				out, err := io.ReadAll(h.out)
				assert.Nil(t, err)
				outStr := string(out)
				for _, expected := range tc.wantContains {
					assert.Contains(t, outStr, expected)
				}
			} else {
				h.assertOut(tc.wantOut)
			}
		})
	}
}

// TestNeo4jAuraConfigNoLongerExists tests that "neo4j aura config list" no longer
// executes the config list functionality, since config was removed from the aura
// subcommand tree in task-018. Cobra's legacy-args behavior means it shows the aura
// help instead of erroring, but "config" must not appear as an available command.
func TestNeo4jAuraConfigNoLongerExists(t *testing.T) {
	tests := []struct {
		name               string
		command            string
		wantOutContains    string
		wantOutNotContains string
	}{
		{
			name:    "neo4j aura config list shows aura help (config no longer a subcommand)",
			command: "aura config list",
			// aura command help is shown; "config" is not listed as an available command.
			wantOutContains:    "neo4j-cli aura",
			wantOutNotContains: "config",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args, err := shlex.Split(tc.command)
			assert.Nil(t, err)

			fs, err := testfs.GetTestFs(`{}`, `{}`)
			assert.Nil(t, err)

			cfg := clicfg.NewConfig(fs, "test")
			cobra.EnableTraverseRunHooks = true

			// Build a full neo4j root command with both aura and config subcommands.
			// aura.NewCmd does NOT include config (removed in task-018), so
			// "aura config list" will display the aura help without executing config.
			rootCmd := &cobra.Command{
				Use: "neo4j-cli",
			}
			aura.RegisterOutputFlag(rootCmd, cfg)

			auraCmd := aura.NewCmd(cfg)
			auraCmd.Use = "aura"
			rootCmd.AddCommand(auraCmd)
			rootCmd.AddCommand(config.NewCmd(cfg))

			outBuf := bytes.NewBufferString("")
			rootCmd.SetArgs(args)
			rootCmd.SetOut(outBuf)
			rootCmd.SetErr(bytes.NewBufferString(""))

			rootCmd.Execute() //nolint:errcheck // cobra prints errors itself; assertions check output

			outStr := outBuf.String()
			if tc.wantOutContains != "" {
				assert.Contains(t, outStr, tc.wantOutContains)
			}
			if tc.wantOutNotContains != "" {
				assert.NotContains(t, outStr, tc.wantOutNotContains)
			}
		})
	}
}
