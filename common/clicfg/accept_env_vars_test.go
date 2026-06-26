// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg_test

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalConfigAcceptEnvVars(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		want       bool
		wantIsSet  bool
	}{
		{
			name:       "defaults to false when absent",
			configJSON: `{}`,
			want:       false,
			wantIsSet:  false,
		},
		{
			name:       "reflects explicit true override",
			configJSON: `{"accept-env-vars":true}`,
			want:       true,
			wantIsSet:  true,
		},
		{
			name:       "reflects explicit false override",
			configJSON: `{"accept-env-vars":false}`,
			want:       false,
			wantIsSet:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := testfs.GetTestFs(tc.configJSON, "{}")
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			assert.Equal(t, tc.want, cfg.Global.AcceptEnvVars())
			assert.Equal(t, tc.wantIsSet, cfg.Global.AcceptEnvVarsIsSet())
		})
	}
}

func TestGlobalConfigAcceptEnvVarsFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		envValue  string
		wantValue bool
	}{
		{name: "truthy env enables", envValue: "1", wantValue: true},
		{name: "falsy env still counts as set", envValue: "false", wantValue: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NEO4J_CLI_ACCEPT_ENV_VARS", tc.envValue)
			fs, err := testfs.GetTestFs(`{}`, "{}")
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

			assert.Equal(t, tc.wantValue, cfg.Global.AcceptEnvVars())
			assert.True(t, cfg.Global.AcceptEnvVarsIsSet(), "env var should count as explicitly set")
		})
	}
}

func TestGlobalConfigAcceptEnvVarsKeyResolvesAndSets(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr string
	}{
		{name: "accepts true", value: "true"},
		{name: "accepts false", value: "false"},
		{name: "rejects non-bool", value: "notabool", wantErr: "invalid value for 'accept-env-vars'"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := testfs.GetTestFs(`{}`, "{}")
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

			scope, resolvedKey, err := clicfg.ResolveConfigKey("accept-env-vars", cfg)
			require.NoError(t, err)
			assert.Equal(t, clicfg.GlobalScope, scope)
			assert.Equal(t, "accept-env-vars", resolvedKey)

			err = cfg.Global.Set("accept-env-vars", tc.value)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.value == "true", cfg.Global.AcceptEnvVars())
		})
	}
}

func TestGatedGetenv(t *testing.T) {
	const name = "NEO4J_GATED_GETENV_TEST"

	t.Run("nil config returns empty", func(t *testing.T) {
		t.Setenv(name, "value")
		var cfg *clicfg.Config
		assert.Equal(t, "", cfg.GatedGetenv(name))
	})

	t.Run("gate off ignores the env var", func(t *testing.T) {
		t.Setenv(name, "value")
		fs, err := testfs.GetTestFs(`{}`, "{}")
		require.NoError(t, err)
		cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
		assert.Equal(t, "", cfg.GatedGetenv(name))
	})

	t.Run("gate on reads the env var", func(t *testing.T) {
		t.Setenv(name, "value")
		fs, err := testfs.GetTestFs(`{"accept-env-vars":true}`, "{}")
		require.NoError(t, err)
		cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
		assert.Equal(t, "value", cfg.GatedGetenv(name))
	})

	t.Run("gate on with unset var returns empty", func(t *testing.T) {
		fs, err := testfs.GetTestFs(`{"accept-env-vars":true}`, "{}")
		require.NoError(t, err)
		cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
		assert.Equal(t, "", cfg.GatedGetenv("NEO4J_GATED_GETENV_UNSET"))
	})
}

func TestAcceptEnvVarsDisplaysAsBool(t *testing.T) {
	t.Run("unset renders as nil (null)", func(t *testing.T) {
		fs, err := testfs.GetTestFs(`{}`, "{}")
		require.NoError(t, err)
		cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
		assert.Nil(t, cfg.Global.Get("accept-env-vars"))
	})

	t.Run("config-set string renders as bool", func(t *testing.T) {
		fs, err := testfs.GetTestFs(`{}`, "{}")
		require.NoError(t, err)
		cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
		require.NoError(t, cfg.Global.Set("accept-env-vars", "true"))
		assert.Equal(t, true, cfg.Global.Get("accept-env-vars"))
		assert.Equal(t, true, cfg.Global.GetPrintable("accept-env-vars").Value)
	})

	t.Run("env bootstrap renders as bool, never the raw literal", func(t *testing.T) {
		for _, tc := range []struct {
			env  string
			want bool
		}{{"1", true}, {"0", false}} {
			t.Setenv("NEO4J_CLI_ACCEPT_ENV_VARS", tc.env)
			fs, err := testfs.GetTestFs(`{}`, "{}")
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			assert.Equal(t, tc.want, cfg.Global.Get("accept-env-vars"))
		}
	})
}

func TestAcceptEnvVarsAppearsInPrintable(t *testing.T) {
	fs, err := testfs.GetTestFs(`{}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	keys := make(map[string]bool)
	for _, e := range cfg.Printable() {
		keys[e.Key] = true
	}
	assert.True(t, keys["accept-env-vars"], "accept-env-vars should appear in config list")
}
