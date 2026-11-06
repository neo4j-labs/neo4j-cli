// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMajorMinor(t *testing.T) {
	for _, tc := range []struct {
		in    string
		major int
		minor int
		ok    bool
		note  string
	}{
		{in: "v1.35.2", major: 1, minor: 35, ok: true},
		{in: "1.35", major: 1, minor: 35, ok: true},
		{in: "v1.36.0+k3s1", major: 1, minor: 36, ok: true, note: "build metadata is cut"},
		{in: "v3.16.4+g8e8f4e2", major: 3, minor: 16, ok: true},
		{in: "v1.35.2-rc.1", major: 1, minor: 35, ok: true, note: "pre-release suffix parses"},
		{in: " v1.35.2 ", major: 1, minor: 35, ok: true, note: "surrounding spaces are trimmed"},
		{in: "v1", ok: false},
		{in: "unknown", ok: false},
		{in: "", ok: false},
		{in: "vX.Y", ok: false},
	} {
		major, minor, ok := ParseMajorMinor(tc.in)
		assert.Equal(t, tc.ok, ok, "input %q", tc.in)
		if tc.ok {
			assert.Equal(t, tc.major, major, "input %q", tc.in)
			assert.Equal(t, tc.minor, minor, "input %q", tc.in)
		}
	}
}

func TestMajorMinorAtLeast(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{in: "v1.22.0", want: true},
		{in: "v1.23.4", want: true},
		{in: "v1.21.9", want: false},
		{in: "v2.0.0", want: true},
		{in: "garbage", want: false},
	} {
		assert.Equal(t, tc.want, MajorMinorAtLeast(tc.in, 1, 22), "input %q", tc.in)
	}
}

func TestMinorSkew(t *testing.T) {
	skew, ok := MinorSkew("v1.36.0", "v1.35.3")
	assert.True(t, ok)
	assert.Equal(t, 1, skew, "client one minor newer than server")

	skew, ok = MinorSkew("v1.34.1", "v1.35.0")
	assert.True(t, ok)
	assert.Equal(t, -1, skew, "client one minor older than server")

	_, ok = MinorSkew("v1.36.0", "v2.0.0")
	assert.False(t, ok, "a cross-major pair has no skew policy")

	_, ok = MinorSkew("v1.36.0", "")
	assert.False(t, ok, "an unparseable server version cannot be compared")
}
