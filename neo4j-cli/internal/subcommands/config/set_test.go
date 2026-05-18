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
		wantOutEmpty     bool
	}{
		{
			name:         "set format to json without rw errors",
			command:      "config set format json",
			wantErr:      "Error: this command writes; pass --rw to allow it",
			wantOutEmpty: true,
		},
		{
			name:            "set format to json with rw writes json at root",
			command:         "config set --rw format json",
			wantConfigKey:   "format",
			wantConfigValue: "json",
		},
		{
			name:            "set format to table with rw writes table at root",
			command:         "config set --rw format table",
			wantConfigKey:   "format",
			wantConfigValue: "table",
		},
		{
			name:            "set format to default with rw writes default at root",
			command:         "config set --rw format default",
			wantConfigKey:   "format",
			wantConfigValue: "default",
		},
		{
			name:         "set format to invalid value with rw returns error",
			command:      "config set --rw format invalid",
			wantErr:      "Error: invalid value for 'format': invalid (valid values: default, json, table, toon)",
			wantOutEmpty: true,
		},
		{
			name:    "set unknown key with rw returns error",
			command: "config set --rw unknown-key value",
			wantErr: `Error: invalid config key: "unknown-key"`,
		},
		{
			name:             "set with missing value and rw returns error",
			command:          "config set --rw format",
			wantErrSubstring: "Error",
		},
		// Dot-notation aura keys
		{
			name:            "set aura.base-url with rw writes to aura.base-url",
			command:         "config set --rw aura.base-url https://example.com",
			wantConfigKey:   "aura.base-url",
			wantConfigValue: "https://example.com",
		},
		{
			name:    "set aura.format with rw returns error (global-only key)",
			command: "config set --rw aura.format json",
			wantErr: `Error: invalid config key: "aura.format" is a global key and cannot be addressed with the "aura." prefix`,
		},
		{
			name:    "set aura.unknown with rw returns error",
			command: "config set --rw aura.unknown value",
			wantErr: `Error: invalid config key: "aura.unknown"`,
		},
		// Telemetry opt-out
		{
			name:            "set telemetry to false with rw writes false at root",
			command:         "config set --rw telemetry false",
			wantConfigKey:   "telemetry",
			wantConfigValue: "false",
		},
		{
			name:            "set telemetry to true with rw writes true at root",
			command:         "config set --rw telemetry true",
			wantConfigKey:   "telemetry",
			wantConfigValue: "true",
		},
		{
			name:    "set telemetry to invalid value with rw returns error",
			command: "config set --rw telemetry maybe",
			wantErr: "Error: invalid value for 'telemetry': maybe (valid values: true, false)",
		},
		{
			name:         "set telemetry to false without rw errors",
			command:      "config set telemetry false",
			wantErr:      "Error: this command writes; pass --rw to allow it",
			wantOutEmpty: true,
		},
		// Feature-flag scope (flag.*)
		{
			name:    "set flag.aura-beta to true with rw writes bool true at flag.aura-beta",
			command: "config set --rw flag.aura-beta true",
			// dot in key is escaped on read so gjson treats it as a literal flat key
			wantConfigKey:   `flag\.aura-beta`,
			wantConfigValue: "true",
		},
		{
			name:    "set flag.aura-beta to invalid value with rw returns error",
			command: "config set --rw flag.aura-beta maybe",
			wantErr: `Error: invalid value for "flag.aura-beta": maybe (valid values: true, false)`,
		},
		{
			name:    "set flag.unknown-thing with rw returns error",
			command: "config set --rw flag.unknown-thing true",
			wantErr: `Error: invalid config key: "flag.unknown-thing"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newNeo4jTestHelper(t)

			h.executeCommand(tc.command)

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				if tc.wantOutEmpty {
					h.assertOut("")
				}
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
