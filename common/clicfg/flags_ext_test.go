// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// The Registry tests live in the EXTERNAL clicfg_test package (rather than
// alongside flags_test.go) because they resolve overrides through a real
// clicfg.NewConfig, which needs test/utils/testfs — and testfs imports clicfg,
// so an internal test file would be an import cycle.
//
// These tests read the PRODUCTION Registry, while flags_test.go's
// registerTestFlag and clicfg_test.go's TestResolveConfigKey both REPLACE that
// global with a sentinel-only map. That is safe only because no test in this
// package calls t.Parallel(); parallelising either side would make the
// registration lookups below flake.

package clicfg_test

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_EntriesCarryFullMetadata(t *testing.T) {
	require.NotEmpty(t, clicfg.Registry)

	for key, spec := range clicfg.Registry {
		t.Run(key, func(t *testing.T) {
			assert.Equal(t, key, spec.Name, "map key must equal Flag.Name")
			assert.Regexp(t, `^flag\.[a-z0-9]+(-[a-z0-9]+)+$`, spec.Name)
			assert.False(t, spec.Default, "flags are opt-in only; no default-true kill switches")
			assert.NotEmpty(t, spec.Owner)
			assert.NotEmpty(t, spec.Gates)
			assert.NotEmpty(t, spec.IntroducedIn)
			assert.NotEmpty(t, spec.RemovalCondition)
		})
	}
}

func TestRegistry_MCPServerFlag(t *testing.T) {
	spec, ok := clicfg.Registry["flag.mcp-server"]
	require.True(t, ok, "flag.mcp-server must be registered")
	assert.False(t, spec.Default)
	assert.Empty(t, spec.LegacyKey, "a newly introduced flag has no legacy alias")
	assert.Equal(t, "NEO4J_CLI_FLAG_MCP_SERVER", clicfg.FlagNameToEnv(spec.Name))
}

func TestRegistry_MCPServerFlag_Overrides(t *testing.T) {
	tests := []struct {
		name       string
		configJSON string
		env        string
		want       bool
	}{
		{
			name:       "default is off",
			configJSON: `{}`,
			want:       false,
		},
		{
			name:       "config file true enables",
			configJSON: `{"flag.mcp-server":true}`,
			want:       true,
		},
		{
			name:       "config file false keeps it off",
			configJSON: `{"flag.mcp-server":false}`,
			want:       false,
		},
		{
			name:       "env var 1 enables",
			configJSON: `{}`,
			env:        "1",
			want:       true,
		},
		{
			name:       "env var true enables",
			configJSON: `{}`,
			env:        "true",
			want:       true,
		},
		{
			name:       "env var wins over config file false",
			configJSON: `{"flag.mcp-server":false}`,
			env:        "1",
			want:       true,
		},
		{
			name:       "env var 0 wins over config file true",
			configJSON: `{"flag.mcp-server":true}`,
			env:        "0",
			want:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("NEO4J_CLI_FLAG_MCP_SERVER", tc.env)
			}
			cfg := newTestConfig(t, clicfg.GlobalScope, tc.configJSON)
			assert.Equal(t, tc.want, cfg.Flags.Enabled("flag.mcp-server"))
		})
	}
}
