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

func TestGlobalConfigTeeEnabled(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		want       bool
	}{
		{
			name:       "defaults to true when absent",
			configJSON: `{}`,
			want:       true,
		},
		{
			name:       "reflects explicit false override",
			configJSON: `{"tee-enabled":false}`,
			want:       false,
		},
		{
			name:       "reflects explicit true override",
			configJSON: `{"tee-enabled":true}`,
			want:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := testfs.GetTestFs(tc.configJSON, "{}")
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			assert.Equal(t, tc.want, cfg.Global.TeeEnabled())
		})
	}
}

func TestGlobalConfigTeeLimit(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		want       int
	}{
		{
			name:       "defaults to 20 when absent",
			configJSON: `{}`,
			want:       20,
		},
		{
			name:       "reflects explicit override",
			configJSON: `{"tee-limit":5}`,
			want:       5,
		},
		{
			name:       "zero disables retention",
			configJSON: `{"tee-limit":0}`,
			want:       0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := testfs.GetTestFs(tc.configJSON, "{}")
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			assert.Equal(t, tc.want, cfg.Global.TeeLimit())
		})
	}
}

func TestGlobalConfigTeeKeysResolveAndSet(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr string
	}{
		{name: "tee-enabled resolves as global key", key: "tee-enabled", value: "false"},
		{name: "tee-limit resolves as global key", key: "tee-limit", value: "10"},
		{name: "tee-enabled rejects non-bool", key: "tee-enabled", value: "nope", wantErr: "invalid value for 'tee-enabled'"},
		{name: "tee-limit rejects non-integer", key: "tee-limit", value: "abc", wantErr: "invalid value for 'tee-limit'"},
		{name: "tee-limit rejects negative", key: "tee-limit", value: "-1", wantErr: "invalid value for 'tee-limit'"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := testfs.GetTestFs(`{}`, "{}")
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

			scope, resolvedKey, err := clicfg.ResolveConfigKey(tc.key, cfg)
			require.NoError(t, err)
			assert.Equal(t, clicfg.GlobalScope, scope)
			assert.Equal(t, tc.key, resolvedKey)

			err = cfg.Global.Set(tc.key, tc.value)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestTeeKeysAppearInPrintable(t *testing.T) {
	fs, err := testfs.GetTestFs(`{}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	keys := make(map[string]bool)
	for _, e := range cfg.Printable() {
		keys[e.Key] = true
	}
	assert.True(t, keys["tee-enabled"], "tee-enabled should appear in config list")
	assert.True(t, keys["tee-limit"], "tee-limit should appear in config list")
}
