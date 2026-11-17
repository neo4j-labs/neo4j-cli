// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveOutput_JSONLResolvesToJSON(t *testing.T) {
	cmd, cfg, _ := newOutputCmd(t, "jsonl")
	assert.Equal(t, "json", ResolveOutput(cmd, cfg))
	assert.True(t, IsJSONL(cfg))
}

func TestIsJSONL_FalseForOtherFormats(t *testing.T) {
	for _, format := range []string{"json", "table", "toon", "default"} {
		_, cfg, _ := newOutputCmd(t, format)
		assert.False(t, IsJSONL(cfg), format)
	}
}

func TestPrintBodyMap_JSONL(t *testing.T) {
	tests := []struct {
		name string
		rows []map[string]any
		want []string
	}{
		{
			name: "list prints one object per line without an envelope",
			rows: []map[string]any{{"id": "1", "name": "a"}, {"id": "2", "name": "b"}},
			want: []string{`{"id":"1","name":"a"}`, `{"id":"2","name":"b"}`},
		},
		{
			name: "single record prints one line",
			rows: []map[string]any{{"id": "1"}},
			want: []string{`{"id":"1"}`},
		},
		{
			name: "empty list prints nothing",
			rows: []map[string]any{},
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, cfg, stdout := newOutputCmd(t, "jsonl")
			PrintBodyMap(cmd, cfg, simpleData{rows: tc.rows}, []string{"id"})

			out := strings.TrimSuffix(stdout.String(), "\n")
			if tc.want == nil {
				assert.Empty(t, out)
				return
			}
			lines := strings.Split(out, "\n")
			assert.Equal(t, tc.want, lines)
			for _, line := range lines {
				assert.True(t, json.Valid([]byte(line)), line)
			}
		})
	}
}

func TestPrintBodyMap_JSONUnchangedByJSONL(t *testing.T) {
	cmd, cfg, stdout := newOutputCmd(t, "json")
	PrintBodyMap(cmd, cfg, simpleData{rows: []map[string]any{{"id": "1"}}}, []string{"id"})
	assert.Contains(t, stdout.String(), `"data"`)
	assert.Contains(t, stdout.String(), "\n")
}

func TestPrintBodyMaps_JSONL(t *testing.T) {
	cmd, cfg, stdout := newOutputCmd(t, "jsonl")
	items := []ResponseData{
		simpleData{rows: []map[string]any{{"id": "1"}}},
		simpleData{rows: []map[string]any{{"id": "2"}, {"id": "3"}}},
	}
	PrintBodyMaps(cmd, cfg, items, [][]string{{"id"}, {"id"}})
	assert.Equal(t, "{\"id\":\"1\"}\n{\"id\":\"2\"}\n{\"id\":\"3\"}\n", stdout.String())
}

func TestPrintPassthrough_JSONL(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"data envelope unwraps to rows", `{"data":[{"id":"1"},{"id":"2"}]}`, "{\"id\":\"1\"}\n{\"id\":\"2\"}\n"},
		{"bare array", `[{"id":"1"}]`, "{\"id\":\"1\"}\n"},
		{"big integer keeps its digits", `{"data":[{"id":12345678901234567890}]}`, "{\"id\":12345678901234567890}\n"},
		{"single object", `{"id":"1"}`, "{\"id\":\"1\"}\n"},
		{"scalar falls back to the body", `42`, "42\n"},
		{"invalid json falls back to the body", `not json`, "not json\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, cfg, stdout := newOutputCmd(t, "jsonl")
			PrintPassthrough(cmd, cfg, []byte(tc.body))
			require.Equal(t, tc.want, stdout.String())
		})
	}
}
