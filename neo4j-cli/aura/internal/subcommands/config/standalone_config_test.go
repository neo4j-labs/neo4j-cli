// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"fmt"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/stretchr/testify/assert"
)

// TestStandaloneConfigGet covers the flat standalone aura-cli "config get" command,
// which is wired to NewStandaloneCmd via AuraTestHelper.
func TestStandaloneConfigGet(t *testing.T) {
	tests := []struct {
		name         string
		configSetup  func(h *testutils.AuraTestHelper)
		command      string
		wantOut      string
		wantErr      string
		wantContains []string
	}{
		{
			name: "get format returns current global value",
			configSetup: func(h *testutils.AuraTestHelper) {
				h.SetConfigValue("format", "json")
			},
			command: "config get format",
			// format is "json" so rendering is JSON
			wantOut: `{
	"format": "json"
}`,
		},
		{
			name:    "get format returns default when no config set",
			command: "config get format",
			configSetup: func(h *testutils.AuraTestHelper) {
				h.OverwriteConfig("{}")
			},
			// "default" auto-detects: non-TTY test stdout → JSON rendering
			wantOut: `{
	"format": "default"
}`,
		},
		{
			name: "get auth-url returns aura value",
			configSetup: func(h *testutils.AuraTestHelper) {
				h.SetConfigValue("aura.auth-url", "https://example.com/oauth/token")
			},
			command: "config get auth-url",
			// default format is "json" in test helper so JSON rendering
			wantOut: `{
	"auth-url": "https://example.com/oauth/token"
}`,
		},
		{
			name:        "get unknown key returns error",
			configSetup: func(h *testutils.AuraTestHelper) {},
			command:     "config get unknown-key",
			wantErr:     `Error: invalid argument "unknown-key" for "aura-cli config get"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			if tc.configSetup != nil {
				tc.configSetup(&helper)
			}

			helper.ExecuteCommand(tc.command)

			if tc.wantErr != "" {
				helper.AssertErr(tc.wantErr)
				return
			}

			helper.AssertErr("")
			if len(tc.wantContains) > 0 {
				outStr := helper.PrintOut()
				for _, expected := range tc.wantContains {
					assert.Contains(t, outStr, expected)
				}
			} else {
				helper.AssertOut(tc.wantOut)
			}
		})
	}
}

// TestStandaloneConfigSet covers the flat standalone aura-cli "config set" command.
func TestStandaloneConfigSet(t *testing.T) {
	tests := []struct {
		name            string
		configSetup     func(h *testutils.AuraTestHelper)
		command         string
		wantConfigKey   string
		wantConfigValue string
		wantErr         string
	}{
		{
			name:            "set format json writes global key at JSON root",
			configSetup:     func(h *testutils.AuraTestHelper) {},
			command:         "config set format json",
			wantConfigKey:   "format",
			wantConfigValue: "json",
		},
		{
			name:            "set auth-url routes to aura config",
			configSetup:     func(h *testutils.AuraTestHelper) {},
			command:         "config set auth-url https://custom.example.com/oauth/token",
			wantConfigKey:   "aura.auth-url",
			wantConfigValue: "https://custom.example.com/oauth/token",
		},
		{
			name:        "set unknown key returns error",
			configSetup: func(h *testutils.AuraTestHelper) {},
			command:     "config set unknown-key value",
			wantErr:     "Error: invalid config key specified: unknown-key",
		},
		{
			name:        "set format to invalid value returns error",
			configSetup: func(h *testutils.AuraTestHelper) {},
			command:     "config set format invalid-value",
			wantErr:     "Error: invalid value for 'format': invalid-value (valid values: default, json, table, toon)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			if tc.configSetup != nil {
				tc.configSetup(&helper)
			}

			helper.ExecuteCommand(tc.command)

			if tc.wantErr != "" {
				helper.AssertErr(tc.wantErr)
				return
			}

			helper.AssertErr("")
			helper.AssertConfigValue(tc.wantConfigKey, tc.wantConfigValue)
		})
	}
}

// TestStandaloneConfigList covers the flat standalone aura-cli "config list" command,
// which should include both global keys (format) and aura-scoped keys.
func TestStandaloneConfigList(t *testing.T) {
	tests := []struct {
		name         string
		configSetup  func(h *testutils.AuraTestHelper)
		command      string
		wantContains []string
		wantOut      string
		wantErr      string
	}{
		{
			name: "list includes both format and aura keys",
			configSetup: func(h *testutils.AuraTestHelper) {
				h.OverwriteConfig(fmt.Sprintf(`{
					"format": "json",
					"aura": {
						"auth-url": "%s",
						"base-url": "%s"
					}
				}`, clicfg.DefaultAuraAuthUrl, clicfg.DefaultAuraBaseUrl))
			},
			command: "config list",
			// format is "json" → JSON rendering showing all keys
			wantOut: fmt.Sprintf(`{
	"auth-url": "%s",
	"base-url": "%s",
	"default-tenant": null,
	"format": "json"
}`, clicfg.DefaultAuraAuthUrl, clicfg.DefaultAuraBaseUrl),
		},
		{
			name: "list with table format renders all keys in table",
			configSetup: func(h *testutils.AuraTestHelper) {
				h.SetConfigValue("format", "table")
			},
			command:      "config list",
			wantContains: []string{"KEY", "VALUE", "format", "auth-url", "base-url"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			if tc.configSetup != nil {
				tc.configSetup(&helper)
			}

			helper.ExecuteCommand(tc.command)

			if tc.wantErr != "" {
				helper.AssertErr(tc.wantErr)
				return
			}

			helper.AssertErr("")
			if len(tc.wantContains) > 0 {
				outStr := helper.PrintOut()
				for _, expected := range tc.wantContains {
					assert.Contains(t, outStr, expected)
				}
			} else {
				helper.AssertOut(tc.wantOut)
			}
		})
	}
}
