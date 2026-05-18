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
			name:        "get format returns default when no config set",
			configSetup: func(h *neo4jTestHelper) {},
			command:     "config get format",
			// "default" auto-detects: non-TTY test stdout → JSON rendering
			wantOut: `{
	"format": "default"
}`,
		},
		{
			name: "get format returns JSON when format configured to json",
			configSetup: func(h *neo4jTestHelper) {
				h.setConfigValue("format", "json")
			},
			command: "config get format",
			// format config is "json" so rendering format is JSON and value reported is "json"
			wantOut: `{
	"format": "json"
}`,
		},
		{
			name:    "get format with --format json flag renders JSON and reports json value",
			command: "config get format --format json",
			// --format json flag binds viper "format" to "json", so both the rendered
			// format and the reported value become "json".
			wantOut: `{
	"format": "json"
}`,
		},
		{
			name:    "get format with --format table flag renders a table",
			command: "config get format --format table",
			// --format table overrides rendering; go-pretty renders header in uppercase with StyleLight.
			// Flag binding also sets the viper "format" key to "table", so the displayed value is "table".
			wantOutFunc: func(t *testing.T, outStr string) {
				assert.Contains(t, outStr, "KEY")
				assert.Contains(t, outStr, "VALUE")
				assert.Contains(t, outStr, "format")
				assert.Contains(t, outStr, "table")
			},
		},
		{
			name:    "get with invalid key returns error",
			command: `config get invalid-key`,
			wantErr: `Error: invalid config key: "invalid-key"`,
		},
		{
			name:    "get aura.default-workspace returns default (null) value as JSON",
			command: "config get aura.default-workspace --format json",
			wantOut: `{
	"aura.default-workspace": null
}`,
		},
		{
			name: "get aura.default-workspace returns configured value as JSON",
			configSetup: func(h *neo4jTestHelper) {
				h.setConfigValue("aura.default-workspace", "my-org/my-project")
			},
			command: "config get aura.default-workspace --format json",
			wantOut: `{
	"aura.default-workspace": "my-org/my-project"
}`,
		},
		{
			name:    "get aura.default-workspace renders as table",
			command: "config get aura.default-workspace --format table",
			wantOutFunc: func(t *testing.T, outStr string) {
				assert.Contains(t, outStr, "KEY")
				assert.Contains(t, outStr, "VALUE")
				assert.Contains(t, outStr, "aura.default-workspace")
			},
		},
		{
			name:    "get aura.format returns error (format is global-only)",
			command: "config get aura.format",
			wantErr: `Error: invalid config key: "aura.format" is a global key and cannot be addressed with the "aura." prefix`,
		},
		{
			name:    "get aura.base-url returns default value as JSON",
			command: "config get aura.base-url --format json",
			wantOut: `{
	"aura.base-url": "https://api.neo4j.io"
}`,
		},
		// Feature-flag scope (flag.*)
		{
			name:    "get flag.aura-beta returns default false as JSON",
			command: "config get flag.aura-beta --format json",
			wantOut: `{
	"flag.aura-beta": false
}`,
		},
		{
			name: "get flag.aura-beta returns configured true as JSON",
			configSetup: func(h *neo4jTestHelper) {
				h.setConfigValue("flag.aura-beta", true)
			},
			command: "config get flag.aura-beta --format json",
			wantOut: `{
	"flag.aura-beta": true
}`,
		},
		{
			name:    "config list does not include any flag.* keys",
			command: "config list --format json",
			wantOutFunc: func(t *testing.T, outStr string) {
				assert.NotContains(t, outStr, "flag.")
			},
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
