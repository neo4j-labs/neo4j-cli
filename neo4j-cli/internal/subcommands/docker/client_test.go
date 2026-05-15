// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRedactArgs verifies that the argv-redaction helper used by execClient.run
// on the error-message path (REQ-NF-004) masks credential-bearing env values
// while preserving every other argv element verbatim, and never mutates its
// input. The helper exists because a non-zero docker exit echoes the full argv
// — including `-e NEO4J_AUTH=neo4j/<password>` — back to the user's terminal
// and any captured shell/CI logs.
func TestRedactArgs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "neo4j auth env masked",
			in:   []string{"run", "-d", "-e", "NEO4J_AUTH=neo4j/hunter2", "neo4j:latest"},
			want: []string{"run", "-d", "-e", "NEO4J_AUTH=<redacted>", "neo4j:latest"},
		},
		{
			name: "license env preserved (no AUTH or PASSWORD substring)",
			in:   []string{"run", "-d", "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval", "neo4j:latest-enterprise"},
			want: []string{"run", "-d", "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval", "neo4j:latest-enterprise"},
		},
		{
			name: "arbitrary password env masked via PASSWORD substring",
			in:   []string{"run", "-e", "MY_PASSWORD=hunter2"},
			want: []string{"run", "-e", "MY_PASSWORD=<redacted>"},
		},
		{
			name: "lowercase auth still masked",
			in:   []string{"run", "-e", "neo4j_auth=neo4j/x"},
			want: []string{"run", "-e", "neo4j_auth=<redacted>"},
		},
		{
			name: "non-env arg with equals is preserved (no LHS letters before =)",
			in:   []string{"run", "=oddly-shaped"},
			want: []string{"run", "=oddly-shaped"},
		},
		{
			name: "label assignments preserved (no AUTH/PASSWORD)",
			in:   []string{"--label", "org.neo4j.cli.managed=true"},
			want: []string{"--label", "org.neo4j.cli.managed=true"},
		},
		{
			name: "empty slice returns empty slice",
			in:   []string{},
			want: []string{},
		},
		{
			name: "nil returns nil",
			in:   nil,
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Snapshot the input so we can assert non-mutation after the call.
			var inCopy []string
			if tc.in != nil {
				inCopy = make([]string, len(tc.in))
				copy(inCopy, tc.in)
			}

			got := redactArgs(tc.in)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, inCopy, tc.in, "redactArgs must not mutate its input slice")
		})
	}
}
