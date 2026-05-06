// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeURI(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantOut     string
		wantRewrite bool
		wantOrig    string
	}{
		// http → neo4j (port forced to 7687).
		{
			name:        "http no port → neo4j:7687",
			input:       "http://host",
			wantOut:     "neo4j://host:7687",
			wantRewrite: true,
			wantOrig:    "http://host",
		},
		{
			name:        "http with default 7474 → neo4j:7687",
			input:       "http://host:7474",
			wantOut:     "neo4j://host:7687",
			wantRewrite: true,
			wantOrig:    "http://host:7474",
		},
		{
			name:        "http with custom port → neo4j:7687 (port forced)",
			input:       "http://host:9999",
			wantOut:     "neo4j://host:7687",
			wantRewrite: true,
			wantOrig:    "http://host:9999",
		},
		{
			name:        "http with path/query stripped",
			input:       "http://host:7474/some/path?q=1",
			wantOut:     "neo4j://host:7687",
			wantRewrite: true,
			wantOrig:    "http://host:7474/some/path?q=1",
		},
		// https → neo4j+s (port forced to 7687).
		{
			name:        "https no port → neo4j+s:7687",
			input:       "https://host",
			wantOut:     "neo4j+s://host:7687",
			wantRewrite: true,
			wantOrig:    "https://host",
		},
		{
			name:        "https with default 7473 → neo4j+s:7687",
			input:       "https://host:7473",
			wantOut:     "neo4j+s://host:7687",
			wantRewrite: true,
			wantOrig:    "https://host:7473",
		},
		{
			name:        "https with custom port → neo4j+s:7687 (port forced)",
			input:       "https://host:9999",
			wantOut:     "neo4j+s://host:7687",
			wantRewrite: true,
			wantOrig:    "https://host:9999",
		},
		{
			name:        "https with path/query stripped",
			input:       "https://host:7473/api?token=abc",
			wantOut:     "neo4j+s://host:7687",
			wantRewrite: true,
			wantOrig:    "https://host:7473/api?token=abc",
		},
		// Userinfo preserved on rewrite, password redacted in displayOrig.
		{
			name:        "http with userinfo+path: userinfo preserved, password redacted in orig",
			input:       "http://user:pass@host:7474/db?x=1",
			wantOut:     "neo4j://user:pass@host:7687",
			wantRewrite: true,
			wantOrig:    "http://user:xxxxx@host:7474/db?x=1",
		},
		{
			name:        "https with userinfo: userinfo preserved, password redacted in orig",
			input:       "https://user:pass@host:7473",
			wantOut:     "neo4j+s://user:pass@host:7687",
			wantRewrite: true,
			wantOrig:    "https://user:xxxxx@host:7473",
		},
		// Bolt-family schemes pass through (already valid for the driver).
		{
			name:        "bolt passthrough",
			input:       "bolt://host:7687",
			wantOut:     "bolt://host:7687",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "neo4j passthrough",
			input:       "neo4j://host:7687",
			wantOut:     "neo4j://host:7687",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "neo4j+s passthrough",
			input:       "neo4j+s://abc.databases.neo4j.io",
			wantOut:     "neo4j+s://abc.databases.neo4j.io",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "neo4j+ssc passthrough",
			input:       "neo4j+ssc://host:7687",
			wantOut:     "neo4j+ssc://host:7687",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "bolt+s passthrough",
			input:       "bolt+s://host:7687",
			wantOut:     "bolt+s://host:7687",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "bolt+ssc passthrough",
			input:       "bolt+ssc://host:7687",
			wantOut:     "bolt+ssc://host:7687",
			wantRewrite: false,
			wantOrig:    "",
		},
		// Garbage / unknown schemes pass through.
		{
			name:        "gibberish (no scheme) passthrough",
			input:       "gibberish-not-a-url",
			wantOut:     "gibberish-not-a-url",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "unknown scheme passthrough",
			input:       "ftp://host:21",
			wantOut:     "ftp://host:21",
			wantRewrite: false,
			wantOrig:    "",
		},
		// Case-insensitive scheme match.
		{
			name:        "uppercase HTTP → neo4j:7687",
			input:       "HTTP://HOST:7474",
			wantOut:     "neo4j://HOST:7687",
			wantRewrite: true,
			wantOrig:    "http://HOST:7474",
		},
		{
			name:        "uppercase HTTPS → neo4j+s:7687",
			input:       "HTTPS://Host",
			wantOut:     "neo4j+s://Host:7687",
			wantRewrite: true,
			wantOrig:    "https://Host",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, did, orig := normalizeURI(tc.input)
			assert.Equal(t, tc.wantOut, out, "rewritten URI")
			assert.Equal(t, tc.wantRewrite, did, "didRewrite")
			assert.Equal(t, tc.wantOrig, orig, "displayOrig")
		})
	}
}
