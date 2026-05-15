// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// TestClassifyInspectError exercises the stderr-substring classifier that
// execClient.Inspect uses to distinguish "container does not exist" from
// operational docker failures (daemon down, permission denied, rootless
// misconfig, …). The classifier is pulled out into its own function so we
// can drive it with crafted error strings without needing a real docker
// binary; the substring contract documented on classifyInspectError must
// match docker's stable stderr wording for missing containers across modern
// daemon versions.
func TestClassifyInspectError(t *testing.T) {
	cases := []struct {
		name           string
		in             error
		wantNotFound   bool
		wantContainsIn bool // assert returned message contains the name (only on ErrNotFound)
		wantSame       bool // assert returned error == input (operational pass-through)
	}{
		{
			name:           "nil input returns nil",
			in:             nil,
			wantNotFound:   false,
			wantContainsIn: false,
			wantSame:       false, // nil case is asserted separately
		},
		{
			name:           "modern docker 'No such object' stderr",
			in:             errors.New("docker inspect ghost: Error: No such object: ghost"),
			wantNotFound:   true,
			wantContainsIn: true,
		},
		{
			name:           "legacy docker 'No such container' stderr",
			in:             errors.New("docker inspect ghost: Error: No such container: ghost"),
			wantNotFound:   true,
			wantContainsIn: true,
		},
		{
			name:         "daemon down error preserved verbatim",
			in:           errors.New("docker inspect dev: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"),
			wantNotFound: false,
			wantSame:     true,
		},
		{
			name:         "permission denied error preserved verbatim",
			in:           errors.New("docker inspect dev: permission denied while trying to connect to the Docker daemon socket"),
			wantNotFound: false,
			wantSame:     true,
		},
		{
			name:         "rootless misconfig error preserved verbatim",
			in:           errors.New("docker inspect dev: Got permission denied while trying to connect to the Docker daemon socket at unix:///run/user/1000/docker.sock"),
			wantNotFound: false,
			wantSame:     true,
		},
		{
			name:         "context deadline preserved verbatim",
			in:           errors.New("docker inspect dev: signal: killed"),
			wantNotFound: false,
			wantSame:     true,
		},
		{
			name:         "arbitrary unknown error preserved verbatim",
			in:           errors.New("something else entirely"),
			wantNotFound: false,
			wantSame:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyInspectError(tc.in, "ghost")

			if tc.in == nil {
				assert.NoError(t, got, "nil input must return nil")
				return
			}

			require.Error(t, got)
			if tc.wantNotFound {
				assert.True(t, errors.Is(got, ErrNotFound),
					"expected errors.Is(_, ErrNotFound) for missing-container stderr, got %v", got)
				if tc.wantContainsIn {
					assert.Contains(t, got.Error(), "ghost",
						"ErrNotFound wrap should mention the container name")
				}
			} else {
				assert.False(t, errors.Is(got, ErrNotFound),
					"operational error must NOT match ErrNotFound, got %v", got)
				if tc.wantSame {
					// Operational errors must propagate verbatim — same
					// underlying value so the stderr the operator needs to
					// read is preserved exactly.
					assert.Equal(t, tc.in, got,
						"operational error must be returned verbatim (no wrap)")
					assert.Equal(t, tc.in.Error(), got.Error(),
						"error message must be unchanged on operational pass-through")
				}
			}
		})
	}
}
