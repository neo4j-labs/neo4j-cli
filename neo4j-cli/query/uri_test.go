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
		{
			name:        "bolt with 7687 → http with 7474",
			input:       "bolt://h:7687",
			wantOut:     "http://h:7474",
			wantRewrite: true,
			wantOrig:    "bolt://h:7687",
		},
		{
			name:        "bolt+s with 7687 → https with 7473",
			input:       "bolt+s://h:7687",
			wantOut:     "https://h:7473",
			wantRewrite: true,
			wantOrig:    "bolt+s://h:7687",
		},
		{
			name:        "bolt+ssc with 7687 → https with 7473",
			input:       "bolt+ssc://h:7687",
			wantOut:     "https://h:7473",
			wantRewrite: true,
			wantOrig:    "bolt+ssc://h:7687",
		},
		{
			name:        "neo4j with 7687 → https with 7473",
			input:       "neo4j://h:7687",
			wantOut:     "https://h:7473",
			wantRewrite: true,
			wantOrig:    "neo4j://h:7687",
		},
		{
			name:        "neo4j+s no port (Aura) → https no port",
			input:       "neo4j+s://abc.databases.neo4j.io",
			wantOut:     "https://abc.databases.neo4j.io",
			wantRewrite: true,
			wantOrig:    "neo4j+s://abc.databases.neo4j.io",
		},
		{
			name:        "neo4j+ssc with 7687 → https with 7473",
			input:       "neo4j+ssc://h:7687",
			wantOut:     "https://h:7473",
			wantRewrite: true,
			wantOrig:    "neo4j+ssc://h:7687",
		},
		{
			name:        "bolt with custom port preserved",
			input:       "bolt://host:9999",
			wantOut:     "http://host:9999",
			wantRewrite: true,
			wantOrig:    "bolt://host:9999",
		},
		{
			name:        "bolt+s with custom port preserved",
			input:       "bolt+s://host:9999",
			wantOut:     "https://host:9999",
			wantRewrite: true,
			wantOrig:    "bolt+s://host:9999",
		},
		{
			name:        "https passthrough no rewrite",
			input:       "https://host:7473",
			wantOut:     "https://host:7473",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "http passthrough no rewrite",
			input:       "http://host:7474",
			wantOut:     "http://host:7474",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "bolt with userinfo+path+query: userinfo preserved in rewrite, password redacted in orig",
			input:       "bolt://user:pass@host:7687/some/path?q=1",
			wantOut:     "http://user:pass@host:7474/some/path?q=1",
			wantRewrite: true,
			wantOrig:    "bolt://user:xxxxx@host:7687/some/path?q=1",
		},
		{
			name:        "gibberish (no scheme) passthrough",
			input:       "gibberish-not-a-url",
			wantOut:     "gibberish-not-a-url",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "unknown scheme passthrough",
			input:       "not-a-scheme://x:7687",
			wantOut:     "not-a-scheme://x:7687",
			wantRewrite: false,
			wantOrig:    "",
		},
		// Aura special case: *.neo4j.io always rewrites to https with no port.
		{
			name:        "Aura bolt no port → https no port",
			input:       "bolt://abc.databases.neo4j.io",
			wantOut:     "https://abc.databases.neo4j.io",
			wantRewrite: true,
			wantOrig:    "bolt://abc.databases.neo4j.io",
		},
		{
			name:        "Aura bolt with 7687 → https no port (drop bolt port)",
			input:       "bolt://abc.databases.neo4j.io:7687",
			wantOut:     "https://abc.databases.neo4j.io",
			wantRewrite: true,
			wantOrig:    "bolt://abc.databases.neo4j.io:7687",
		},
		{
			name:        "Aura neo4j no port → https no port",
			input:       "neo4j://abc.databases.neo4j.io",
			wantOut:     "https://abc.databases.neo4j.io",
			wantRewrite: true,
			wantOrig:    "neo4j://abc.databases.neo4j.io",
		},
		{
			name:        "Aura neo4j with 7687 → https no port (drop bolt port)",
			input:       "neo4j://abc.databases.neo4j.io:7687",
			wantOut:     "https://abc.databases.neo4j.io",
			wantRewrite: true,
			wantOrig:    "neo4j://abc.databases.neo4j.io:7687",
		},
		{
			name:        "Aura neo4j+s no port → https no port (regression)",
			input:       "neo4j+s://abc.databases.neo4j.io",
			wantOut:     "https://abc.databases.neo4j.io",
			wantRewrite: true,
			wantOrig:    "neo4j+s://abc.databases.neo4j.io",
		},
		{
			name:        "Aura neo4j+s with 7687 → https no port (drop bolt port)",
			input:       "neo4j+s://abc.databases.neo4j.io:7687",
			wantOut:     "https://abc.databases.neo4j.io",
			wantRewrite: true,
			wantOrig:    "neo4j+s://abc.databases.neo4j.io:7687",
		},
		{
			name:        "Aura bolt+s with 7687 → https no port (drop bolt port)",
			input:       "bolt+s://abc.databases.neo4j.io:7687",
			wantOut:     "https://abc.databases.neo4j.io",
			wantRewrite: true,
			wantOrig:    "bolt+s://abc.databases.neo4j.io:7687",
		},
		{
			name:        "Aura bolt+ssc with 7687 → https no port (drop bolt port)",
			input:       "bolt+ssc://abc.databases.neo4j.io:7687",
			wantOut:     "https://abc.databases.neo4j.io",
			wantRewrite: true,
			wantOrig:    "bolt+ssc://abc.databases.neo4j.io:7687",
		},
		{
			name:        "Aura https passthrough no rewrite",
			input:       "https://abc.databases.neo4j.io",
			wantOut:     "https://abc.databases.neo4j.io",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "Aura apex uppercase: case-insensitive suffix match, original case preserved",
			input:       "bolt://NEO4J.IO",
			wantOut:     "https://NEO4J.IO",
			wantRewrite: true,
			wantOrig:    "bolt://NEO4J.IO",
		},
		{
			name:        "Lookalike *.neo4j.io.evil.com NOT matched → falls through to generic 7687→7474",
			input:       "bolt://abc.databases.neo4j.io.evil.com:7687",
			wantOut:     "http://abc.databases.neo4j.io.evil.com:7474",
			wantRewrite: true,
			wantOrig:    "bolt://abc.databases.neo4j.io.evil.com:7687",
		},
		{
			name:        "Aura with custom port → https no port (Aura is always 443)",
			input:       "bolt://abc.databases.neo4j.io:9999",
			wantOut:     "https://abc.databases.neo4j.io",
			wantRewrite: true,
			wantOrig:    "bolt://abc.databases.neo4j.io:9999",
		},
		{
			name:        "Aura path/query preserved across rewrite",
			input:       "bolt://abc.databases.neo4j.io/path?q=1",
			wantOut:     "https://abc.databases.neo4j.io/path?q=1",
			wantRewrite: true,
			wantOrig:    "bolt://abc.databases.neo4j.io/path?q=1",
		},
		{
			name:        "Aura userinfo preserved on rewrite, password redacted in orig",
			input:       "bolt://user:pass@abc.databases.neo4j.io:7687",
			wantOut:     "https://user:pass@abc.databases.neo4j.io",
			wantRewrite: true,
			wantOrig:    "bolt://user:xxxxx@abc.databases.neo4j.io:7687",
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
