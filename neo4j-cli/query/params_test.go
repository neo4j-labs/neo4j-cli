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
		name       string
		in         []string
		wantParams map[string]any
		wantEmbeds []EmbedJob
	}{
		{
			name:       "empty input returns empty map and nil embeds",
			in:         nil,
			wantParams: map[string]any{},
			wantEmbeds: nil,
		},
		{
			name:       "integer parses as JSON number (float64)",
			in:         []string{"k=5"},
			wantParams: map[string]any{"k": float64(5)},
		},
		{
			name:       "float parses as JSON number",
			in:         []string{"x=3.14"},
			wantParams: map[string]any{"x": float64(3.14)},
		},
		{
			name:       "true parses as bool",
			in:         []string{"flag=true"},
			wantParams: map[string]any{"flag": true},
		},
		{
			name:       "false parses as bool",
			in:         []string{"flag=false"},
			wantParams: map[string]any{"flag": false},
		},
		{
			name:       "null parses as nil",
			in:         []string{"v=null"},
			wantParams: map[string]any{"v": nil},
		},
		{
			name:       "JSON array parses as slice",
			in:         []string{"xs=[1,2,3]"},
			wantParams: map[string]any{"xs": []any{float64(1), float64(2), float64(3)}},
		},
		{
			name:       "JSON object parses as map",
			in:         []string{`obj={"a":1}`},
			wantParams: map[string]any{"obj": map[string]any{"a": float64(1)}},
		},
		{
			name:       "plain string falls back to string",
			in:         []string{"name=alice"},
			wantParams: map[string]any{"name": "alice"},
		},
		{
			name:       "JSON-quoted string unwraps to plain string",
			in:         []string{`name="bob"`},
			wantParams: map[string]any{"name": "bob"},
		},
		{
			name:       "malformed JSON falls back to raw string",
			in:         []string{"v={broken"},
			wantParams: map[string]any{"v": "{broken"},
		},
		{
			name:       "empty value parses as empty string fallback",
			in:         []string{"k="},
			wantParams: map[string]any{"k": ""},
		},
		{
			name:       "value containing equals keeps full RHS",
			in:         []string{"eq=a=b"},
			wantParams: map[string]any{"eq": "a=b"},
		},
		{
			name: "multiple params accumulate into one map",
			in:   []string{"a=1", "b=hello", "c=[true,false]"},
			wantParams: map[string]any{
				"a": float64(1),
				"b": "hello",
				"c": []any{true, false},
			},
		},
		{
			name:       ":embed produces an embed job and no params entry",
			in:         []string{"q:embed=hello"},
			wantParams: map[string]any{},
			wantEmbeds: []EmbedJob{{Name: "q", Text: "hello"}},
		},
		{
			name:       ":embed accepts empty text",
			in:         []string{"q:embed="},
			wantParams: map[string]any{},
			wantEmbeds: []EmbedJob{{Name: "q", Text: ""}},
		},
		{
			name:       ":embed value with `=` keeps full RHS",
			in:         []string{"q:embed=a=b=c"},
			wantParams: map[string]any{},
			wantEmbeds: []EmbedJob{{Name: "q", Text: "a=b=c"}},
		},
		{
			name:       ":embed value with JSON object passes through verbatim",
			in:         []string{`q:embed={"a":1}`},
			wantParams: map[string]any{},
			wantEmbeds: []EmbedJob{{Name: "q", Text: `{"a":1}`}},
		},
		{
			name: "literal and embed entries mix and preserve order",
			in:   []string{"name=alice", "q:embed=hello", "n=5", "title:embed=world"},
			wantParams: map[string]any{
				"name": "alice",
				"n":    float64(5),
			},
			wantEmbeds: []EmbedJob{
				{Name: "q", Text: "hello"},
				{Name: "title", Text: "world"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotParams, gotEmbeds, err := parseParams(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.wantParams, gotParams)
			assert.Equal(t, tc.wantEmbeds, gotEmbeds)
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
		{
			name:      "unknown modifier rejected",
			in:        []string{"q:bogus=x"},
			errSubstr: `unknown modifier "bogus"`,
		},
		{
			name:      "unknown modifier error references the entry",
			in:        []string{"q:bogus=x"},
			errSubstr: `"q:bogus=x"`,
		},
		{
			name:      "empty name with modifier rejected",
			in:        []string{":embed=x"},
			errSubstr: "empty key",
		},
		{
			name:      ":embed=[...] rejected as JSON array",
			in:        []string{"q:embed=[1,2,3]"},
			errSubstr: ":embed expects text, got JSON array",
		},
		{
			name:      ":embed JSON-array rejection references the entry",
			in:        []string{"q:embed=[1,2,3]"},
			errSubstr: `"q:embed=[1,2,3]"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseParams(tc.in)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errSubstr)
		})
	}
}
