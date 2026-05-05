// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseParams(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want map[string]any
	}{
		{
			name: "empty input returns empty map",
			in:   nil,
			want: map[string]any{},
		},
		{
			name: "integer parses as JSON number (float64)",
			in:   []string{"k=5"},
			want: map[string]any{"k": float64(5)},
		},
		{
			name: "float parses as JSON number",
			in:   []string{"x=3.14"},
			want: map[string]any{"x": float64(3.14)},
		},
		{
			name: "true parses as bool",
			in:   []string{"flag=true"},
			want: map[string]any{"flag": true},
		},
		{
			name: "false parses as bool",
			in:   []string{"flag=false"},
			want: map[string]any{"flag": false},
		},
		{
			name: "null parses as nil",
			in:   []string{"v=null"},
			want: map[string]any{"v": nil},
		},
		{
			name: "JSON array parses as slice",
			in:   []string{"xs=[1,2,3]"},
			want: map[string]any{"xs": []any{float64(1), float64(2), float64(3)}},
		},
		{
			name: "JSON object parses as map",
			in:   []string{`obj={"a":1}`},
			want: map[string]any{"obj": map[string]any{"a": float64(1)}},
		},
		{
			name: "plain string falls back to string",
			in:   []string{"name=alice"},
			want: map[string]any{"name": "alice"},
		},
		{
			name: "JSON-quoted string unwraps to plain string",
			in:   []string{`name="bob"`},
			want: map[string]any{"name": "bob"},
		},
		{
			name: "malformed JSON falls back to raw string",
			in:   []string{"v={broken"},
			want: map[string]any{"v": "{broken"},
		},
		{
			name: "empty value parses as empty string fallback",
			in:   []string{"k="},
			want: map[string]any{"k": ""},
		},
		{
			name: "value containing equals keeps full RHS",
			in:   []string{"eq=a=b"},
			want: map[string]any{"eq": "a=b"},
		},
		{
			name: "multiple params accumulate into one map",
			in:   []string{"a=1", "b=hello", "c=[true,false]"},
			want: map[string]any{
				"a": float64(1),
				"b": "hello",
				"c": []any{true, false},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseParams(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseParams_Errors(t *testing.T) {
	tests := []struct {
		name      string
		in        []string
		errSubstr string
	}{
		{
			name:      "missing equals returns error",
			in:        []string{"noEquals"},
			errSubstr: "noEquals",
		},
		{
			name:      "empty key returns error",
			in:        []string{"=value"},
			errSubstr: "empty key",
		},
		{
			name:      "missing equals error mentions key=value form",
			in:        []string{"justAKey"},
			errSubstr: "key=value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseParams(tc.in)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errSubstr)
		})
	}
}
