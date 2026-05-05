// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseParams converts a slice of `key=value` entries (from `--param`) into a
// map of Cypher query parameters. For each value, parseParams first attempts to
// decode it as JSON; on decode failure, the raw string is used verbatim. This
// matches the documented `--param` behaviour: numbers, booleans, null, arrays,
// and objects come through as their JSON-decoded Go types, while plain text
// values (e.g. `name=alice`) fall back to strings.
//
// Entries missing `=` or with an empty key return an error referencing the
// offending entry.
func parseParams(raw []string) (map[string]any, error) {
	out := make(map[string]any, len(raw))
	for _, entry := range raw {
		idx := strings.Index(entry, "=")
		if idx < 0 {
			return nil, fmt.Errorf("invalid --param %q: expected key=value", entry)
		}
		key := entry[:idx]
		if key == "" {
			return nil, fmt.Errorf("invalid --param %q: empty key", entry)
		}
		value := entry[idx+1:]

		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			out[key] = value
			continue
		}
		out[key] = decoded
	}
	return out, nil
}
