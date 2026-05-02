// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestConfig(t *testing.T, scope clicfg.ConfigScope) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	return clicfg.NewConfig(fs, "test", scope)
}

func TestResolveConfigKey(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		scope         clicfg.ConfigScope
		wantNamespace clicfg.ConfigScope
		wantKey       string
		wantErr       string
	}{
		{
			name:          "global key output resolves to global namespace",
			key:           "output",
			scope:         clicfg.GlobalScope,
			wantNamespace: clicfg.GlobalScope,
			wantKey:       "output",
		},
		{
			name:          "aura-prefixed key resolves to aura namespace with prefix stripped",
			key:           "aura.default-tenant",
			scope:         clicfg.GlobalScope,
			wantNamespace: clicfg.AuraScope,
			wantKey:       "default-tenant",
		},
		{
			name:          "aura.base-url resolves to aura namespace",
			key:           "aura.base-url",
			scope:         clicfg.GlobalScope,
			wantNamespace: clicfg.AuraScope,
			wantKey:       "base-url",
		},
		{
			name:          "aura.auth-url resolves to aura namespace",
			key:           "aura.auth-url",
			scope:         clicfg.GlobalScope,
			wantNamespace: clicfg.AuraScope,
			wantKey:       "auth-url",
		},
		{
			name:    "aura.output is rejected because output is a global-only key",
			key:     "aura.output",
			scope:   clicfg.GlobalScope,
			wantErr: `invalid config key: "aura.output" is a global key and cannot be addressed with the "aura." prefix`,
		},
		{
			name:    "aura.unknown is rejected as an unrecognised aura key",
			key:     "aura.unknown",
			scope:   clicfg.GlobalScope,
			wantErr: `invalid config key: "aura.unknown"`,
		},
		{
			name:    "unknown bare key is rejected as unrecognised",
			key:     "unknown",
			scope:   clicfg.GlobalScope,
			wantErr: `invalid config key: "unknown"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestConfig(t, tc.scope)
			gotNamespace, gotKey, err := clicfg.ResolveConfigKey(tc.key, cfg)

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantNamespace, gotNamespace)
			assert.Equal(t, tc.wantKey, gotKey)
		})
	}
}

func TestGetAuraBaseUrlConfigRemovesTrailingPath(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	cfgStr := fmt.Sprintf(`{
		"output": "json",
		"aura": {
			"auth-url": "%s/oauth/token",
			"base-url": "%s/v1"
			}
		}`, server.URL, server.URL)

	credentialsStr := `{
		"aura": {
			"credentials": [{
				"name": "test-cred",
				"access-token": "dsa",
				"token-expiry": 123
			}],
			"default-credential": "test-cred"
			}
		}`

	fs, err := testfs.GetTestFs(cfgStr, credentialsStr)
	assert.Nil(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	//The path parameter will be removed from GET base url
	assert.Equal(t, server.URL, cfg.Aura.BaseUrl())
}
