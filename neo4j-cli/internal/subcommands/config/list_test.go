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
			name:    "list with default format auto-detects non-TTY and renders JSON",
			command: "config list",
			// "default" auto-detects: non-TTY test stdout → JSON rendering
			wantOut: `{
	"aura.auth-url": "https://api.neo4j.io/oauth/token",
	"aura.base-url": "https://api.neo4j.io",
	"aura.default-tenant": null,
	"format": "default"
}`,
		},
		{
			name: "list with format set to json and --format json flag renders JSON",
			configSetup: func(h *neo4jTestHelper) {
				h.setConfigValue("format", "json")
			},
			command: "config list --format json",
			wantOut: `{
	"aura.auth-url": "https://api.neo4j.io/oauth/token",
	"aura.base-url": "https://api.neo4j.io",
	"aura.default-tenant": null,
	"format": "json"
}`,
		},
		{
			name:    "list with --format table flag renders a table",
			command: "config list --format table",
			// go-pretty renders header row in uppercase with StyleLight.
			wantContains: []string{"KEY", "VALUE", "format"},
		},
		{
			name: "list when format config is table renders a table",
			configSetup: func(h *neo4jTestHelper) {
				h.setConfigValue("format", "table")
			},
			command: "config list",
			// Table rendering when the stored format config is "table".
			// go-pretty renders header row in uppercase with StyleLight.
			wantContains: []string{"KEY", "VALUE", "format", "table"},
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
