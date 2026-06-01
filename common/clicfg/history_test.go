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

func TestGlobalConfigHistoryEnabled(t *testing.T) {
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
			configJSON: `{"history-enabled":false}`,
			want:       false,
		},
		{
			name:       "reflects explicit true override",
			configJSON: `{"history-enabled":true}`,
			want:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := testfs.GetTestFs(tc.configJSON, "{}")
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			assert.Equal(t, tc.want, cfg.Global.HistoryEnabled())
		})
	}
}

func TestGlobalConfigHistoryLimit(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		want       int
	}{
		{
			name:       "defaults to 1000 when absent",
			configJSON: `{}`,
			want:       1000,
		},
		{
			name:       "reflects explicit override",
			configJSON: `{"history-limit":50}`,
			want:       50,
		},
		{
			name:       "zero disables retention",
			configJSON: `{"history-limit":0}`,
			want:       0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := testfs.GetTestFs(tc.configJSON, "{}")
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			assert.Equal(t, tc.want, cfg.Global.HistoryLimit())
		})
	}
}

func TestGlobalConfigHistoryKeysResolveAndSet(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr string
	}{
		{name: "history-enabled resolves as global key", key: "history-enabled", value: "false"},
		{name: "history-limit resolves as global key", key: "history-limit", value: "25"},
		{name: "history-enabled rejects non-bool", key: "history-enabled", value: "nope", wantErr: "invalid value for 'history-enabled'"},
		{name: "history-limit rejects non-integer", key: "history-limit", value: "abc", wantErr: "invalid value for 'history-limit'"},
		{name: "history-limit rejects negative", key: "history-limit", value: "-1", wantErr: "invalid value for 'history-limit'"},
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

func TestHistoryKeysAppearInPrintable(t *testing.T) {
	fs, err := testfs.GetTestFs(`{}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	keys := make(map[string]bool)
	for _, e := range cfg.Printable() {
		keys[e.Key] = true
	}
	assert.True(t, keys["history-enabled"], "history-enabled should appear in config list")
	assert.True(t, keys["history-limit"], "history-limit should appear in config list")
}
