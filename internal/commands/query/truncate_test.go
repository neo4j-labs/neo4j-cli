// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateArrays(t *testing.T) {
	// makeRange returns a []any of the given length, populated with float64
	// counts (matching the JSON-decoded shape of API row values).
	makeRange := func(n int) []any {
		out := make([]any, n)
		for i := 0; i < n; i++ {
			out[i] = float64(i)
		}
		return out
	}

	tests := []struct {
		name      string
		in        any
		max       int
		want      any
		wantCount int
	}{
		{
			name:      "scalar string passthrough",
			in:        "hello",
			max:       100,
			want:      "hello",
			wantCount: 0,
		},
		{
			name:      "scalar number passthrough",
			in:        float64(42),
			max:       100,
			want:      float64(42),
			wantCount: 0,
		},
		{
			name:      "scalar bool passthrough",
			in:        true,
			max:       100,
			want:      true,
			wantCount: 0,
		},
		{
			name:      "scalar nil passthrough",
			in:        nil,
			max:       100,
			want:      nil,
			wantCount: 0,
		},
		{
			name:      "short array passthrough (len equals max)",
			in:        []any{float64(1), float64(2), float64(3)},
			max:       3,
			want:      []any{float64(1), float64(2), float64(3)},
			wantCount: 0,
		},
		{
			name:      "short array passthrough (len below max)",
			in:        []any{"a", "b"},
			max:       10,
			want:      []any{"a", "b"},
			wantCount: 0,
		},
		{
			name:      "long top-level array truncates to empty slice",
			in:        makeRange(200),
			max:       100,
			want:      []any{},
			wantCount: 1,
		},
		{
			name: "array nested in map truncates; sibling scalar untouched",
			in: map[string]any{
				"name":  "alice",
				"items": makeRange(50),
			},
			max: 10,
			want: map[string]any{
				"name":  "alice",
				"items": []any{},
			},
			wantCount: 1,
		},
		{
			name:      "array nested in array truncates inner only",
			in:        []any{"a", makeRange(20), "b"},
			max:       5,
			want:      []any{"a", []any{}, "b"},
			wantCount: 1,
		},
		{
			name: "mixed nesting truncates at every level",
			in: map[string]any{
				"shallow": makeRange(150),
				"deep": map[string]any{
					"nested": []any{"x", makeRange(30)},
					"keep":   []any{float64(1), float64(2)},
				},
			},
			max: 10,
			want: map[string]any{
				"shallow": []any{},
				"deep": map[string]any{
					"nested": []any{"x", []any{}},
					"keep":   []any{float64(1), float64(2)},
				},
			},
			wantCount: 2,
		},
		{
			name:      "max == 0 disables truncation entirely",
			in:        makeRange(500),
			max:       0,
			want:      makeRange(500),
			wantCount: 0,
		},
		{
			name:      "max < 0 disables truncation entirely",
			in:        makeRange(500),
			max:       -1,
			want:      makeRange(500),
			wantCount: 0,
		},
		{
			name:      "empty array passthrough",
			in:        []any{},
			max:       10,
			want:      []any{},
			wantCount: 0,
		},
		{
			name:      "empty map passthrough",
			in:        map[string]any{},
			max:       10,
			want:      map[string]any{},
			wantCount: 0,
		},
		{
			name: "two distinct over-limit arrays count separately",
			in: map[string]any{
				"a": makeRange(20),
				"b": makeRange(30),
			},
			max: 5,
			want: map[string]any{
				"a": []any{},
				"b": []any{},
			},
			wantCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, count := truncateArrays(tc.in, tc.max)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantCount, count, "truncation count mismatch")
		})
	}
}

// TestTruncateArrays_DoesNotMutateInput confirms the pure-function contract:
// truncating a value must not modify the original slice / map.
func TestTruncateArrays_DoesNotMutateInput(t *testing.T) {
	original := map[string]any{
		"items": []any{float64(1), float64(2), float64(3), float64(4)},
		"name":  "alice",
	}
	// Snapshot the original via a deep-equal copy reference.
	innerOrig := original["items"].([]any)
	innerLen := len(innerOrig)

	_, _ = truncateArrays(original, 2)

	assert.Equal(t, "alice", original["name"], "sibling scalar must remain")
	assert.Len(t, original["items"], innerLen, "input slice must not be mutated")
	assert.Equal(t, float64(1), innerOrig[0], "input slice elements must not be mutated")
}
