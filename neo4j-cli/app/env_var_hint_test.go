// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/test/utils/testfs"
	gokeyring "github.com/zalando/go-keyring"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const envVarHintText = "hint: credential env vars detected but accept-env-vars is not set"

// envVarHintSuggestion is the actionable remedy embedded in the hint. The --rw
// flag is required because the hint most often surfaces in non-interactive/agent
// contexts where `config set` rejects a write without it (REQ-F-016).
const envVarHintSuggestion = "run 'neo4j-cli config set accept-env-vars true --rw' or set NEO4J_CLI_ACCEPT_ENV_VARS=1"

// TestMaybeEmitEnvVarHint covers the discovery-hint matrix: it fires only when
// accept-env-vars has never been explicitly set AND a credential env var is
// present.
func TestMaybeEmitEnvVarHint(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		envName    string
		envValue   string
		wantHint   bool
	}{
		{
			name:       "unset config + NEO4J_URI present: hint",
			configJSON: `{}`,
			envName:    credentials.EnvURI,
			envValue:   "neo4j://localhost",
			wantHint:   true,
		},
		{
			name:       "explicit false + NEO4J_URI present: no hint",
			configJSON: `{"accept-env-vars":false}`,
			envName:    credentials.EnvURI,
			envValue:   "neo4j://localhost",
			wantHint:   false,
		},
		{
			name:       "explicit true + NEO4J_URI present: no hint",
			configJSON: `{"accept-env-vars":true}`,
			envName:    credentials.EnvURI,
			envValue:   "neo4j://localhost",
			wantHint:   false,
		},
		{
			name:       "unset config + no relevant env var: no hint",
			configJSON: `{}`,
			envName:    "SOME_UNRELATED_VAR",
			envValue:   "x",
			wantHint:   false,
		},
		{
			name:       "unset config + Aura client id present: hint",
			configJSON: `{}`,
			envName:    credentials.EnvAuraClientID,
			envValue:   "abc",
			wantHint:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Clear any sentinel that might leak in from the host environment.
			for _, name := range []string{credentials.EnvURI, credentials.EnvAuraClientID, credentials.EnvEmbedProvider} {
				t.Setenv(name, "")
			}
			t.Setenv(tc.envName, tc.envValue)

			fs, err := testfs.GetTestFs(tc.configJSON, "{}")
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

			var stderr bytes.Buffer
			maybeEmitEnvVarHint(cfg, &stderr)

			if tc.wantHint {
				assert.Contains(t, stderr.String(), envVarHintText)
				assert.Contains(t, stderr.String(), envVarHintSuggestion)
			} else {
				assert.Empty(t, stderr.String())
			}
		})
	}
}

// TestMaybeEmitEnvVarHint_FiresAtMostOncePerCall asserts a single call emits
// exactly one hint line (the root PersistentPreRunE runs once per process, so
// one emit per call == once per invocation).
func TestMaybeEmitEnvVarHint_FiresAtMostOnce(t *testing.T) {
	t.Setenv(credentials.EnvURI, "neo4j://localhost")

	fs, err := testfs.GetTestFs(`{}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	var stderr bytes.Buffer
	maybeEmitEnvVarHint(cfg, &stderr)

	got := strings.Count(stderr.String(), envVarHintText)
	assert.Equal(t, 1, got, "hint must appear exactly once")
}

// TestEnvVarHint_SurfacesRegardlessOfCommand drives the full root command tree
// through Execute() and asserts the hint surfaces on stderr no matter which
// command triggered the root PersistentPreRunE.
func TestEnvVarHint_SurfacesRegardlessOfCommand(t *testing.T) {
	commands := [][]string{
		{"config", "get", "accept-env-vars"},
		{"credential", "dbms", "list"},
	}

	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			gokeyring.MockInit()
			t.Setenv("NEO4J_CLI_NO_UPDATE_NAG", "1")
			t.Setenv(credentials.EnvURI, "neo4j://localhost")

			fs, err := testfs.GetTestFs(`{"credential-storage":"keyring"}`, "{}")
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

			cmd := NewCmd(cfg)
			var stderr bytes.Buffer
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&stderr)
			cmd.SetArgs(args)

			_ = cmd.Execute()

			assert.Contains(t, stderr.String(), envVarHintText,
				"hint should surface for command %q", strings.Join(args, " "))
		})
	}
}
