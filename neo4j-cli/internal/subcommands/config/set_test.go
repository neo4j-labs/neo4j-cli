// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigSet(t *testing.T) {
	tests := []struct {
		name             string
		command          string
		wantConfigKey    string
		wantConfigValue  string
		wantErr          string
		wantErrSubstring string
	}{
		{
			name:            "set format to json writes json at root",
			command:         "config set format json",
			wantConfigKey:   "format",
			wantConfigValue: "json",
		},
		{
			name:            "set format to table writes table at root",
			command:         "config set format table",
			wantConfigKey:   "format",
			wantConfigValue: "table",
		},
		{
			name:            "set format to default writes default at root",
			command:         "config set format default",
			wantConfigKey:   "format",
			wantConfigValue: "default",
		},
		{
			name:    "set format to invalid value returns error",
			command: "config set format invalid",
			wantErr: "Error: invalid value for 'format': invalid (valid values: default, json, table)",
		},
		{
			name:    "set unknown key returns error",
			command: "config set unknown-key value",
			wantErr: `Error: invalid config key: "unknown-key"`,
		},
		{
			name:             "set with missing value returns error",
			command:          "config set format",
			wantErrSubstring: "Error",
		},
		// Dot-notation aura keys
		{
			name:            "set aura.base-url writes to aura.base-url",
			command:         "config set aura.base-url https://example.com",
			wantConfigKey:   "aura.base-url",
			wantConfigValue: "https://example.com",
		},
		{
			name:            "set aura.default-tenant writes to aura.default-tenant",
			command:         "config set aura.default-tenant my-tenant",
			wantConfigKey:   "aura.default-tenant",
			wantConfigValue: "my-tenant",
		},
		{
			name:    "set aura.format returns error (global-only key)",
			command: "config set aura.format json",
			wantErr: `Error: invalid config key: "aura.format" is a global key and cannot be addressed with the "aura." prefix`,
		},
		{
			name:    "set aura.unknown returns error",
			command: "config set aura.unknown value",
			wantErr: `Error: invalid config key: "aura.unknown"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newNeo4jTestHelper(t)

			h.executeCommand(tc.command)

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}
			if tc.wantErrSubstring != "" {
				errOut, err := io.ReadAll(h.err)
				assert.Nil(t, err)
				assert.Contains(t, string(errOut), tc.wantErrSubstring)
				return
			}

			h.assertErr("")
			h.assertConfigValue(tc.wantConfigKey, tc.wantConfigValue)
		})
	}
}
