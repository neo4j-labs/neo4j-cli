// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatus_Fields(t *testing.T) {
	cfg := newTestConfig(t)
	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "status")
	require.NoError(t, err)
	st := parseJSON(t, stdout)
	for _, k := range []string{"enabled", "endpoint", "authorized_until", "receipts_path"} {
		assert.Contains(t, st, k)
	}
	assert.Equal(t, false, st["enabled"])
	assert.Contains(t, stdout, `"enabled": false`)
}

func TestDisable_Revokes(t *testing.T) {
	withSeams(t, false, "")
	cfg := newTestConfig(t)
	_, _, err := runCmd(t, newTestApp(cfg), "feedback", "enable", "--rw", "--accept", "--endpoint", loopback)
	require.NoError(t, err)
	require.True(t, Authorized(cfg, now()))

	stdout, _, err := runCmd(t, newTestApp(cfg), "feedback", "disable")
	require.NoError(t, err)
	st := parseJSON(t, stdout)
	assert.Equal(t, false, st["enabled"])
	assert.False(t, Authorized(cfg, now()))
}

func TestDisable_AllowedUnderAgent(t *testing.T) {
	withSeams(t, true, "claude-code")
	cfg := newTestConfig(t)
	_, _, err := runCmd(t, newTestApp(cfg), "feedback", "disable")
	assert.NoError(t, err)
}

func TestLeaves_HaveFlushLeftExamplesWithJSON(t *testing.T) {
	cfg := newTestConfig(t)
	for _, c := range NewCmd(cfg).Commands() {
		assert.NotEmpty(t, c.Example, c.Name())
		assert.Contains(t, c.Example, "--format json", c.Name())
		assert.GreaterOrEqual(t, countComments(c.Example), 2, c.Name())
		assert.NotRegexp(t, `^\s`, c.Example, c.Name())
	}
}

func countComments(s string) (n int) {
	for _, l := range splitLines(s) {
		if len(l) > 1 && l[0] == '#' && l[1] == ' ' {
			n++
		}
	}
	return n
}

func splitLines(s string) (out []string) {
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}
