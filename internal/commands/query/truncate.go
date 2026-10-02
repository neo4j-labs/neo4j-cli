// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

// truncateArrays recursively walks value, replacing any []any whose length
// exceeds max with an empty slice `[]any{}`. Maps (`map[string]any`) and
// slices (`[]any`) are descended into; all other types are returned
// unchanged. When max <= 0, the input is returned as-is (no truncation
// applied) and the returned count is zero.
//
// The empty-slice signal is type-safe (downstream JSON consumers see an
// array, not a string) and unambiguously marks the value as elided. The
// returned int is the number of distinct slices that were truncated:
// each over-limit `[]any` adds 1, and nested truncations count separately
// (so callers can surface an aggregate signal alongside the silent
// in-band shape change).
//
// The function is pure: it does not mutate its input. Slices and maps that
// require modification are reallocated; values that pass through untouched
// are returned by reference.
func truncateArrays(value any, max int) (any, int) {
	if max <= 0 {
		return value, 0
	}
	return truncate(value, max)
}

func truncate(value any, max int) (any, int) {
	switch v := value.(type) {
	case []any:
		if len(v) > max {
			return []any{}, 1
		}
		out := make([]any, len(v))
		count := 0
		for i, item := range v {
			truncated, c := truncate(item, max)
			out[i] = truncated
			count += c
		}
		return out, count
	case map[string]any:
		out := make(map[string]any, len(v))
		count := 0
		for k, item := range v {
			truncated, c := truncate(item, max)
			out[k] = truncated
			count += c
		}
		return out, count
	default:
		return v, 0
	}
}
