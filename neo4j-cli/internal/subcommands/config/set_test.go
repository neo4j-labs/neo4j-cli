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
		name              string
		command           string
		wantConfigKey     string
		wantConfigValue   string
		wantErr           string
		wantErrSubstring  string
	}{
		{
			name:            "set output to json writes json at root",
			command:         "config set output json",
			wantConfigKey:   "output",
			wantConfigValue: "json",
		},
		{
			name:            "set output to table writes table at root",
			command:         "config set output table",
			wantConfigKey:   "output",
			wantConfigValue: "table",
		},
		{
			name:            "set output to default writes default at root",
			command:         "config set output default",
			wantConfigKey:   "output",
			wantConfigValue: "default",
		},
		{
			name:    "set output to invalid value returns error",
			command: "config set output invalid",
			wantErr: "Error: invalid value for 'output': invalid (valid values: default, json, table)",
		},
		{
			name:    "set unknown key returns error",
			command: "config set unknown-key value",
			wantErr: "Error: invalid config key specified: unknown-key",
		},
		{
			name:            "set with missing value returns error",
			command:         "config set output",
			wantErrSubstring: "Error",
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
