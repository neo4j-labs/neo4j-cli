// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSuggestionsForTypos drives the live tree built by app.NewCmd and checks
// that typos at every nesting level — root, nested parents, and aura-rooted
// parents — produce a "Did you mean" hint and a non-nil error. Covers
// REQ-F-006 through REQ-F-010 from the CLI-70 PRD.
func TestSuggestionsForTypos(t *testing.T) {
	testCases := []struct {
		name        string
		args        []string
		typo        string
		suggestions []string
	}{
		{
			// REQ-F-006 — root-level regression: `configg` -> `config`.
			name:        "root typo configg list",
			args:        []string{"configg", "list"},
			typo:        "configg",
			suggestions: []string{"config"},
		},
		{
			// REQ-F-007 — root-level regression: `updaet` -> `update`.
			name:        "root typo updaet",
			args:        []string{"updaet"},
			typo:        "updaet",
			suggestions: []string{"update"},
		},
		{
			// REQ-F-008 — nested typo under `config`.
			name:        "config lsit",
			args:        []string{"config", "lsit"},
			typo:        "lsit",
			suggestions: []string{"list"},
		},
		{
			// REQ-F-009 — nested typo under `aura instance`.
			name:        "aura insance list",
			args:        []string{"aura", "insance", "list"},
			typo:        "insance",
			suggestions: []string{"instance"},
		},
		{
			// REQ-F-010 — nested typo under `credential`.
			name:        "credential aura-clent add",
			args:        []string{"credential", "aura-clent", "add"},
			typo:        "aura-clent",
			suggestions: []string{"aura-client"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
			require.NoError(t, err)

			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			cmd := NewCmd(cfg)

			var outBuf, errBuf bytes.Buffer
			cmd.SetOut(&outBuf)
			cmd.SetErr(&errBuf)
			cmd.SetArgs(tc.args)

			execErr := cmd.Execute()
			require.Error(t, execErr, "expected typo %q to produce an error", tc.typo)

			combined := execErr.Error() + "\n" + errBuf.String()
			assert.Contains(t, combined, "Did you mean",
				"expected 'Did you mean' hint for typo %q; got: %q", tc.typo, combined)
			for _, want := range tc.suggestions {
				assert.Contains(t, combined, want,
					"expected suggestion %q for typo %q; got: %q", want, tc.typo, combined)
			}
			assert.True(t,
				strings.Contains(combined, `unknown command "`+tc.typo+`"`) ||
					strings.Contains(combined, `unknown command `+tc.typo),
				"expected the typo %q to appear as the unknown-command target; got: %q", tc.typo, combined)
		})
	}
}

// TestSuggestionsDoNotShadowRunnableCommands spot-checks that commands with
// their own RunE or explicit Args validators are unaffected by the walker.
// Picking three representative leaves: `update` (Run-bearing), `query`
// (runnable parent with explicit Args), and `config set` (RunE + explicit
// Args). For each we assert `--help` resolves and exits without error.
func TestSuggestionsDoNotShadowRunnableCommands(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{name: "update --help", args: []string{"update", "--help"}},
		{name: "query --help", args: []string{"query", "--help"}},
		{name: "config set --help", args: []string{"config", "set", "--help"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
			require.NoError(t, err)

			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			cmd := NewCmd(cfg)

			var outBuf, errBuf bytes.Buffer
			cmd.SetOut(&outBuf)
			cmd.SetErr(&errBuf)
			cmd.SetArgs(tc.args)

			require.NoError(t, cmd.Execute(),
				"expected %v to resolve without error; stderr=%q", tc.args, errBuf.String())
		})
	}
}
