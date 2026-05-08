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
		// wantWarnSubstrs: each non-empty substring must appear in the warning.
		// Use nil/empty to assert the warning is "".
		wantWarnSubstrs []string
		// wantNoWarnSubstrs: each substring must NOT appear in the warning.
		// Useful for asserting password redaction.
		wantNoWarnSubstrs []string
	}{
		// http → neo4j (port forced to 7687). The rewrite produces a
		// cleartext neo4j:// to a non-loopback host so a warning is also
		// expected unless host is loopback.
		{
			name:            "http no port → neo4j:7687 (non-loopback warns)",
			input:           "http://host",
			wantOut:         "neo4j://host:7687",
			wantRewrite:     true,
			wantOrig:        "http://host",
			wantWarnSubstrs: []string{"warning:", "cleartext", "neo4j://host:7687", "neo4j+s://"},
		},
		{
			name:        "http to localhost → neo4j:7687 (silent)",
			input:       "http://localhost:7474",
			wantOut:     "neo4j://localhost:7687",
			wantRewrite: true,
			wantOrig:    "http://localhost:7474",
		},
		{
			name:        "http to 127.0.0.1 → neo4j:7687 (silent)",
			input:       "http://127.0.0.1:9999",
			wantOut:     "neo4j://127.0.0.1:7687",
			wantRewrite: true,
			wantOrig:    "http://127.0.0.1:9999",
		},
		{
			name:            "http to non-loopback with custom port → warn",
			input:           "http://host:9999",
			wantOut:         "neo4j://host:7687",
			wantRewrite:     true,
			wantOrig:        "http://host:9999",
			wantWarnSubstrs: []string{"warning:", "cleartext"},
		},
		{
			name:            "http with path/query stripped (warn)",
			input:           "http://host:7474/some/path?q=1",
			wantOut:         "neo4j://host:7687",
			wantRewrite:     true,
			wantOrig:        "http://host:7474/some/path?q=1",
			wantWarnSubstrs: []string{"warning:"},
		},
		// https → neo4j+s (port forced to 7687); never produces a cleartext warning.
		{
			name:        "https no port → neo4j+s:7687 (silent — encrypted)",
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
		// Userinfo preserved on rewrite, password redacted in displayOrig
		// AND in any warning text.
		{
			name:              "http with userinfo+path: warning masks password",
			input:             "http://user:pass@host:7474/db?x=1",
			wantOut:           "neo4j://user:pass@host:7687",
			wantRewrite:       true,
			wantOrig:          "http://user:xxxxx@host:7474/db?x=1",
			wantWarnSubstrs:   []string{"warning:", "cleartext", "user:xxxxx@host:7687"},
			wantNoWarnSubstrs: []string{"pass"},
		},
		{
			name:        "https with userinfo: userinfo preserved, password redacted in orig (silent)",
			input:       "https://user:pass@host:7473",
			wantOut:     "neo4j+s://user:pass@host:7687",
			wantRewrite: true,
			wantOrig:    "https://user:xxxxx@host:7473",
		},
		// Bolt-family schemes pass through (already valid for the driver);
		// cleartext bolt/neo4j to non-loopback hosts produce a warning.
		{
			name:            "bolt to non-loopback host warns",
			input:           "bolt://prod.example",
			wantOut:         "bolt://prod.example",
			wantRewrite:     false,
			wantOrig:        "",
			wantWarnSubstrs: []string{"warning:", "cleartext", "bolt://prod.example", "bolt+s://"},
		},
		{
			name:            "neo4j to non-loopback host warns",
			input:           "neo4j://prod.example",
			wantOut:         "neo4j://prod.example",
			wantRewrite:     false,
			wantOrig:        "",
			wantWarnSubstrs: []string{"warning:", "cleartext", "neo4j://prod.example", "neo4j+s://"},
		},
		{
			name:            "bolt to non-loopback redacts userinfo password",
			input:           "bolt://user:secret@prod.example:7687",
			wantOut:         "bolt://user:secret@prod.example:7687",
			wantRewrite:     false,
			wantOrig:        "",
			wantWarnSubstrs: []string{"warning:", "user:xxxxx@prod.example"},
			// The literal password value must not leak into the warning.
			wantNoWarnSubstrs: []string{"secret"},
		},
		{
			name:        "bolt to localhost is silent",
			input:       "bolt://localhost",
			wantOut:     "bolt://localhost",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "bolt to 127.0.0.1 is silent",
			input:       "bolt://127.0.0.1",
			wantOut:     "bolt://127.0.0.1",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "bolt to ::1 is silent",
			input:       "bolt://[::1]:7687",
			wantOut:     "bolt://[::1]:7687",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "neo4j to localhost is silent",
			input:       "neo4j://localhost:7687",
			wantOut:     "neo4j://localhost:7687",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "neo4j+s to non-loopback is silent (encrypted)",
			input:       "neo4j+s://abc.databases.neo4j.io",
			wantOut:     "neo4j+s://abc.databases.neo4j.io",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "neo4j+ssc passthrough silent",
			input:       "neo4j+ssc://host:7687",
			wantOut:     "neo4j+ssc://host:7687",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "bolt+s passthrough silent",
			input:       "bolt+s://prod.example:7687",
			wantOut:     "bolt+s://prod.example:7687",
			wantRewrite: false,
			wantOrig:    "",
		},
		{
			name:        "bolt+ssc passthrough silent",
			input:       "bolt+ssc://prod.example:7687",
			wantOut:     "bolt+ssc://prod.example:7687",
			wantRewrite: false,
			wantOrig:    "",
		},
		// Garbage / unknown schemes pass through with no warning.
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
			name:            "uppercase HTTP → neo4j:7687 (non-loopback warns)",
			input:           "HTTP://HOST:7474",
			wantOut:         "neo4j://HOST:7687",
			wantRewrite:     true,
			wantOrig:        "http://HOST:7474",
			wantWarnSubstrs: []string{"warning:", "cleartext"},
		},
		{
			name:        "uppercase HTTPS → neo4j+s:7687 (silent)",
			input:       "HTTPS://Host",
			wantOut:     "neo4j+s://Host:7687",
			wantRewrite: true,
			wantOrig:    "https://Host",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, did, orig, warn := normalizeURI(tc.input)
			assert.Equal(t, tc.wantOut, out, "rewritten URI")
			assert.Equal(t, tc.wantRewrite, did, "didRewrite")
			assert.Equal(t, tc.wantOrig, orig, "displayOrig")
			if len(tc.wantWarnSubstrs) == 0 {
				assert.Empty(t, warn, "expected no warning")
			} else {
				for _, sub := range tc.wantWarnSubstrs {
					assert.Contains(t, warn, sub, "warning missing expected substring")
				}
			}
			for _, sub := range tc.wantNoWarnSubstrs {
				assert.NotContains(t, warn, sub, "warning unexpectedly contains substring")
			}
		})
	}
}
