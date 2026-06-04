// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dataset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRangeMatches(t *testing.T) {
	for _, tc := range []struct {
		name      string
		expr      string
		target    string
		wantMatch bool
		wantLow   string
		wantErr   bool
	}{
		{name: "in range", expr: ">=5.0.0 <6.0.0", target: "v5.13.0", wantMatch: true, wantLow: "v5.0.0"},
		{name: "below lower", expr: ">=5.0.0 <6.0.0", target: "v4.4.0", wantMatch: false},
		{name: "at upper exclusive", expr: ">=5.0.0 <6.0.0", target: "v6.0.0", wantMatch: false},
		{name: "lower inclusive", expr: ">=5.0.0 <6.0.0", target: "v5.0.0", wantMatch: true, wantLow: "v5.0.0"},
		{name: "open lower bound", expr: ">=4.0.0", target: "v2025.1.0", wantMatch: true, wantLow: "v4.0.0"},
		{name: "exclusive lower", expr: ">3.5.0", target: "v3.5.0", wantMatch: false},
		{name: "exclusive lower hit", expr: ">3.5.0", target: "v3.5.1", wantMatch: true, wantLow: "v3.5.0"},
		{name: "le upper", expr: "<=4.4.0", target: "v4.4.0", wantMatch: true, wantLow: "v0.0.0"},
		{name: "bare equals", expr: "5.0.0", target: "v5.0.0", wantMatch: true, wantLow: "v5.0.0"},
		{name: "bare equals miss", expr: "5.0.0", target: "v5.1.0", wantMatch: false},
		{name: "empty", expr: "", target: "v5.0.0", wantErr: true},
		{name: "bad version", expr: ">=abc", target: "v5.0.0", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ok, low, err := rangeMatches(tc.expr, tc.target)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantMatch, ok)
			if tc.wantMatch {
				assert.Equal(t, tc.wantLow, low)
			}
		})
	}
}

func TestCanonicalVersion(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"5.13.0", "v5.13.0"},
		{"v5.13", "v5.13.0"},
		{"5", "v5.0.0"},
		{"  5.0.0  ", "v5.0.0"},
		{"", ""},
	} {
		assert.Equal(t, tc.want, canonicalVersion(tc.in), tc.in)
	}
}
