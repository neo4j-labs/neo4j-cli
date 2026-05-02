// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"bytes"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/config"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestNeo4jAuraConfigNoLongerExists tests that "neo4j aura config list" no longer
// executes the config list functionality, since config was removed from the aura
// subcommand tree. Cobra's legacy-args behavior means it shows the aura
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

			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			cobra.EnableTraverseRunHooks = true

			// Build a full neo4j root command with both aura and config subcommands.
			// aura.NewCmd does NOT include config (removed previously), so
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

// TestNeo4jConfigAuraNoLongerExists tests that "neo4j config aura" is no longer
// a valid subcommand path since the "aura" group was removed from neo4j config.
// Cobra's legacyArgs: child commands show the parent's help for unknown subcommands
// rather than returning an "unknown command" error. We assert the config help is
// shown and "aura" does not appear as a listed subcommand.
func TestNeo4jConfigAuraNoLongerExists(t *testing.T) {
	tests := []struct {
		name               string
		command            string
		wantOutContains    string
		wantOutNotContains string
	}{
		{
			name:               "neo4j config aura get shows config help (aura no longer a subcommand)",
			command:            "config aura get auth-url",
			wantOutContains:    "neo4j-cli config",
			wantOutNotContains: "aura",
		},
		{
			name:               "neo4j config aura list shows config help (aura no longer a subcommand)",
			command:            "config aura list",
			wantOutContains:    "neo4j-cli config",
			wantOutNotContains: "aura",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args, err := shlex.Split(tc.command)
			assert.Nil(t, err)

			fs, err := testfs.GetTestFs(`{}`, `{}`)
			assert.Nil(t, err)

			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			cobra.EnableTraverseRunHooks = true

			rootCmd := &cobra.Command{
				Use: "neo4j-cli",
			}
			aura.RegisterOutputFlag(rootCmd, cfg)
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

// TestNeo4jConfigProjectBetaGated tests that "neo4j config project" is available
// when beta is enabled, and absent when beta is disabled.
func TestNeo4jConfigProjectBetaGated(t *testing.T) {
	t.Run("config project list available when beta enabled", func(t *testing.T) {
		h := newNeo4jTestHelper(t)
		h.setConfigValue("aura.beta-enabled", true)
		h.setConfigValue("output", "json")
		h.executeCommand("config project list")
		// With no projects configured, the output should succeed with no error
		h.assertErr("")
	})

	t.Run("config project not available when beta disabled", func(t *testing.T) {
		args, err := shlex.Split("config project list")
		assert.Nil(t, err)

		fs, err := testfs.GetTestFs(`{}`, `{}`)
		assert.Nil(t, err)

		cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
		cobra.EnableTraverseRunHooks = true

		rootCmd := &cobra.Command{
			Use: "neo4j-cli",
		}
		aura.RegisterOutputFlag(rootCmd, cfg)
		rootCmd.AddCommand(config.NewCmd(cfg))

		outBuf := bytes.NewBufferString("")
		rootCmd.SetArgs(args)
		rootCmd.SetOut(outBuf)
		rootCmd.SetErr(bytes.NewBufferString(""))

		rootCmd.Execute() //nolint:errcheck // cobra prints errors itself; assertions check output

		// When beta is disabled, project is not added; cobra shows config help.
		// "project" must not appear as a listed subcommand.
		outStr := outBuf.String()
		assert.Contains(t, outStr, "neo4j-cli config")
		assert.NotContains(t, outStr, "project")
	})
}
