// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigGet(t *testing.T) {
	tests := []struct {
		name        string
		configSetup func(h *neo4jTestHelper)
		command     string
		wantOut     string
		wantErr     string
		wantOutFunc func(t *testing.T, outStr string)
	}{
		{
			name:        "get output returns default when no config set",
			configSetup: func(h *neo4jTestHelper) {},
			command:     "config get output",
			// "default" output mode renders as a table via PrintBodyMap
			wantOutFunc: func(t *testing.T, outStr string) {
				assert.Contains(t, outStr, "KEY")
				assert.Contains(t, outStr, "VALUE")
				assert.Contains(t, outStr, "output")
				assert.Contains(t, outStr, "default")
			},
		},
		{
			name: "get output returns JSON when output configured to json",
			configSetup: func(h *neo4jTestHelper) {
				h.setConfigValue("output", "json")
			},
			command: "config get output",
			// Output config is "json" so rendering format is JSON and value reported is "json"
			wantOut: `{
	"Key": "output",
	"Value": "json"
}`,
		},
		{
			name:    "get output with --output json flag renders JSON and reports json value",
			command: "config get output --output json",
			// --output json flag binds viper "output" to "json", so both the rendered
			// format and the reported value become "json".
			wantOut: `{
	"Key": "output",
	"Value": "json"
}`,
		},
		{
			name:    "get output with --output table flag renders a table",
			command: "config get output --output table",
			// --output table overrides rendering; go-pretty renders header in uppercase with StyleLight.
			// Flag binding also sets the viper "output" key to "table", so the displayed value is "table".
			wantOutFunc: func(t *testing.T, outStr string) {
				assert.Contains(t, outStr, "KEY")
				assert.Contains(t, outStr, "VALUE")
				assert.Contains(t, outStr, "output")
				assert.Contains(t, outStr, "table")
			},
		},
		{
			name:    "get with invalid key returns error",
			command: `config get invalid-key`,
			wantErr: `Error: invalid argument "invalid-key" for "neo4j-cli config get"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newNeo4jTestHelper(t)
			if tc.configSetup != nil {
				tc.configSetup(&h)
			}

			h.executeCommand(tc.command)

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			if tc.wantOutFunc != nil {
				out, err := io.ReadAll(h.out)
				assert.Nil(t, err)
				tc.wantOutFunc(t, string(out))
			} else {
				h.assertOut(tc.wantOut)
			}
		})
	}
}
