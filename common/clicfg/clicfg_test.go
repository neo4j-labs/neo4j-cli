// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicfg_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/credentials"
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
			name:          "global key format resolves to global namespace",
			key:           "format",
			scope:         clicfg.GlobalScope,
			wantNamespace: clicfg.GlobalScope,
			wantKey:       "format",
		},
		{
			name:          "aura-prefixed key resolves to aura namespace with prefix stripped",
			key:           "aura.default-context",
			scope:         clicfg.GlobalScope,
			wantNamespace: clicfg.AuraScope,
			wantKey:       "default-context",
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
			name:    "aura.format is rejected because format is a global-only key",
			key:     "aura.format",
			scope:   clicfg.GlobalScope,
			wantErr: `invalid config key: "aura.format" is a global key and cannot be addressed with the "aura." prefix`,
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
		"format": "json",
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

func TestDefaultTenant(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		want       string
	}{
		{
			name:       "returns project portion of aura.default-context when set",
			configJSON: `{"aura":{"default-context":"org-abc/proj-xyz"}}`,
			want:       "proj-xyz",
		},
		{
			name:       "returns project portion when context has extra slashes",
			configJSON: `{"aura":{"default-context":"org-abc/proj-xyz/extra"}}`,
			want:       "extra",
		},
		{
			name:       "falls back to aura.default-tenant when default-context is unset",
			configJSON: `{"aura":{"default-tenant":"legacy-tenant-id"}}`,
			want:       "legacy-tenant-id",
		},
		{
			name:       "returns empty string when neither is set",
			configJSON: `{}`,
			want:       "",
		},
		{
			name:       "default-context takes priority over default-tenant when both are set",
			configJSON: `{"aura":{"default-context":"org-abc/proj-xyz","default-tenant":"legacy-tenant-id"}}`,
			want:       "proj-xyz",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := testfs.GetTestFs(tc.configJSON, "{}")
			require.NoError(t, err)
			cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
			assert.Equal(t, tc.want, cfg.Aura.DefaultTenant())
		})
	}
}

func TestAuraConfigActiveCredential(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(cfg *clicfg.Config)
		wantNil  bool
		wantName string
	}{
		{
			name:    "returns nil when no active credential has been set",
			setup:   func(cfg *clicfg.Config) {},
			wantNil: true,
		},
		{
			name: "returns the credential after SetActiveCredential",
			setup: func(cfg *clicfg.Config) {
				cred := &credentials.AuraCredential{Name: "my-cred", ClientId: "id1", ClientSecret: "secret1"}
				cfg.Aura.SetActiveCredential(cred)
			},
			wantNil:  false,
			wantName: "my-cred",
		},
		{
			name: "overwrites previous active credential",
			setup: func(cfg *clicfg.Config) {
				first := &credentials.AuraCredential{Name: "first", ClientId: "id1", ClientSecret: "secret1"}
				cfg.Aura.SetActiveCredential(first)
				second := &credentials.AuraCredential{Name: "second", ClientId: "id2", ClientSecret: "secret2"}
				cfg.Aura.SetActiveCredential(second)
			},
			wantNil:  false,
			wantName: "second",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestConfig(t, clicfg.AuraScope)
			tc.setup(cfg)
			got := cfg.Aura.ActiveCredential()
			if tc.wantNil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tc.wantName, got.Name)
			}
		})
	}
}
