// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package envfile

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		path           string
		content        string
		writeFile      bool
		wantErrSubstr  string
		wantVals       map[string]string
		wantPresent    map[string]bool
		notPresentKeys []string
	}{
		{
			name:      "happy path",
			path:      "/aura.txt",
			writeFile: true,
			content: `NEO4J_URI=neo4j+s://abc.databases.neo4j.io
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=secret
AURA_INSTANCENAME=my-instance
`,
			wantVals: map[string]string{
				"NEO4J_URI":         "neo4j+s://abc.databases.neo4j.io",
				"NEO4J_USERNAME":    "neo4j",
				"NEO4J_PASSWORD":    "secret",
				"AURA_INSTANCENAME": "my-instance",
			},
			wantPresent: map[string]bool{
				"NEO4J_URI":         true,
				"NEO4J_USERNAME":    true,
				"NEO4J_PASSWORD":    true,
				"AURA_INSTANCENAME": true,
			},
		},
		{
			name:      "comments and blank lines",
			path:      "/aura.txt",
			writeFile: true,
			content: `# This is a comment
NEO4J_URI=neo4j+s://abc.databases.neo4j.io

# Another comment
NEO4J_USERNAME=neo4j
`,
			wantVals: map[string]string{
				"NEO4J_URI":      "neo4j+s://abc.databases.neo4j.io",
				"NEO4J_USERNAME": "neo4j",
			},
			wantPresent: map[string]bool{
				"NEO4J_URI":      true,
				"NEO4J_USERNAME": true,
			},
		},
		{
			name:      "empty value — presence true, value empty",
			path:      "/aura.txt",
			writeFile: true,
			content: `NEO4J_URI=
NEO4J_USERNAME=neo4j
`,
			wantVals: map[string]string{
				"NEO4J_URI":      "",
				"NEO4J_USERNAME": "neo4j",
			},
			wantPresent: map[string]bool{
				"NEO4J_URI":      true,
				"NEO4J_USERNAME": true,
			},
		},
		{
			name:          "missing path returns wrapped usage error",
			path:          "/does-not-exist.txt",
			writeFile:     false,
			wantErrSubstr: `--file "/does-not-exist.txt":`,
		},
		{
			name:      "unrecognised keys are returned — caller filters",
			path:      "/aura.txt",
			writeFile: true,
			content: `NEO4J_URI=neo4j+s://abc.databases.neo4j.io
SOME_OTHER_KEY=value
AURA_INSTANCEID=abc123
`,
			wantVals: map[string]string{
				"NEO4J_URI":       "neo4j+s://abc.databases.neo4j.io",
				"SOME_OTHER_KEY":  "value",
				"AURA_INSTANCEID": "abc123",
			},
			wantPresent: map[string]bool{
				"NEO4J_URI":       true,
				"SOME_OTHER_KEY":  true,
				"AURA_INSTANCEID": true,
			},
		},
		{
			name:      "key NOT in file → present[key] is false",
			path:      "/aura.txt",
			writeFile: true,
			content:   `NEO4J_URI=neo4j+s://abc.databases.neo4j.io`,
			wantVals: map[string]string{
				"NEO4J_URI": "neo4j+s://abc.databases.neo4j.io",
			},
			wantPresent: map[string]bool{
				"NEO4J_URI": true,
			},
			notPresentKeys: []string{"NEO4J_USERNAME", "NEO4J_PASSWORD", "AURA_INSTANCENAME"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := afero.NewMemMapFs()
			if tc.writeFile {
				if err := afero.WriteFile(fs, tc.path, []byte(tc.content), 0o644); err != nil {
					t.Fatalf("failed to seed fs: %v", err)
				}
			}

			vals, present, err := Parse(fs, tc.path)
			if tc.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for k, want := range tc.wantVals {
				if got, ok := vals[k]; !ok {
					t.Errorf("vals[%q] missing", k)
				} else if got != want {
					t.Errorf("vals[%q] = %q, want %q", k, got, want)
				}
			}
			for k, want := range tc.wantPresent {
				if got := present[k]; got != want {
					t.Errorf("present[%q] = %v, want %v", k, got, want)
				}
			}
			for _, k := range tc.notPresentKeys {
				if present[k] {
					t.Errorf("present[%q] = true, want false (key not in file)", k)
				}
			}
		})
	}
}
