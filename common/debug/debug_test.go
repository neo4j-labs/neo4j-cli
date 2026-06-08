// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package debug

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolve_FlagAndEnvPrecedence locks parity with query's resolveDebug
// semantics: an explicit --debug flag wins outright (so --debug=false beats
// NEO4J_DEBUG=1); otherwise debug is ON iff NEO4J_DEBUG == "1" strictly (any
// other value, including true/yes/on/0, leaves debug OFF).
func TestResolve_FlagAndEnvPrecedence(t *testing.T) {
	tests := []struct {
		name      string
		flagArgs  []string
		envValue  string // empty string means env unset
		wantDebug bool
	}{
		{name: "flag on, env unset", flagArgs: []string{"--debug"}, envValue: "", wantDebug: true},
		{name: "env=1, no flag", flagArgs: nil, envValue: "1", wantDebug: true},
		{name: "flag on, env=1", flagArgs: []string{"--debug"}, envValue: "1", wantDebug: true},
		{name: "both off", flagArgs: nil, envValue: "", wantDebug: false},
		{name: "env=true (not '1') leaves debug off", flagArgs: nil, envValue: "true", wantDebug: false},
		{name: "env=yes leaves debug off", flagArgs: nil, envValue: "yes", wantDebug: false},
		{name: "env=on leaves debug off", flagArgs: nil, envValue: "on", wantDebug: false},
		{name: "env=0 leaves debug off", flagArgs: nil, envValue: "0", wantDebug: false},
		{name: "explicit --debug=false overrides env=1", flagArgs: []string{"--debug=false"}, envValue: "1", wantDebug: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvVar, tc.envValue)

			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().Bool("debug", false, "")
			require.NoError(t, cmd.ParseFlags(tc.flagArgs))

			assert.Equal(t, tc.wantDebug, Resolve(cmd))
		})
	}
}

// TestResolve_NoDebugFlagDefined returns false when no --debug flag exists and
// the env is unset, so callers without the flag registered don't panic.
func TestResolve_NoDebugFlagDefined(t *testing.T) {
	t.Setenv(EnvVar, "")
	cmd := &cobra.Command{Use: "test"}
	assert.False(t, Resolve(cmd))
}
