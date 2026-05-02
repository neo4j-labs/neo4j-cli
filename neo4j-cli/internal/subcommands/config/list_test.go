// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package config_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigList(t *testing.T) {
	tests := []struct {
		name         string
		configSetup  func(h *neo4jTestHelper)
		command      string
		wantOut      string
		wantErr      string
		wantContains []string
	}{
		{
			name:    "list with default output renders a table",
			command: "config list",
			// "default" output mode now renders as a table via PrintBodyMap
			wantContains: []string{"KEY", "VALUE", "output", "default"},
		},
		{
			name: "list with output set to json and --output json flag renders JSON",
			configSetup: func(h *neo4jTestHelper) {
				h.setConfigValue("output", "json")
			},
			command: "config list --output json",
			wantOut: `{
	"output": "json"
}`,
		},
		{
			name:    "list with --output table flag renders a table",
			command: "config list --output table",
			// go-pretty renders header row in uppercase with StyleLight.
			wantContains: []string{"KEY", "VALUE", "output"},
		},
		{
			name: "list when output config is table renders a table",
			configSetup: func(h *neo4jTestHelper) {
				h.setConfigValue("output", "table")
			},
			command: "config list",
			// Table rendering when the stored output config is "table".
			// go-pretty renders header row in uppercase with StyleLight.
			wantContains: []string{"KEY", "VALUE", "output", "table"},
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
			if len(tc.wantContains) > 0 {
				out, err := io.ReadAll(h.out)
				assert.Nil(t, err)
				outStr := string(out)
				for _, expected := range tc.wantContains {
					assert.Contains(t, outStr, expected)
				}
			} else {
				h.assertOut(tc.wantOut)
			}
		})
	}
}
