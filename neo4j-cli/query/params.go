// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"encoding/json"
	"strings"

	"github.com/neo4j/cli/common/clierr"
)

// EmbedJob describes a single `--param NAME:embed=TEXT` entry produced by
// parseParams. The Cypher parameter named Name is populated at run time with
// the embedding vector produced for Text by the resolved embed provider.
type EmbedJob struct {
	Name string
	Text string
}

// parseParams converts a slice of `key=value` entries (from `--param`) into a
// map of Cypher query parameters plus a slice of pending embed jobs.
//
// Without a modifier, each value is JSON-decoded; on decode failure the raw
// string is used verbatim. This matches the documented `--param` behaviour:
// numbers, booleans, null, arrays, and objects come through as their JSON
// types, while plain text values fall back to strings.
//
// A key may carry exactly one modifier separated by `:` (e.g.
// `q:embed=hello`). Only `embed` is recognised; any other modifier is a
// usage error. With `:embed`, JSON typing is disabled (text passes through
// verbatim) and the entry is collected as an EmbedJob instead of being
// added to the params map. As a guardrail, a `:embed` value that parses as
// a JSON array is rejected — text was almost certainly intended.
//
// Entries missing `=`, with an empty key, or with an empty modifier name
// after `:` return a usage error referencing the offending entry.
func parseParams(raw []string) (map[string]any, []EmbedJob, error) {
	out := make(map[string]any, len(raw))
	var embeds []EmbedJob
	for _, entry := range raw {
		idx := strings.Index(entry, "=")
		if idx < 0 {
			return nil, nil, clierr.NewUsageError("invalid --param %q: expected key=value", entry)
		}
		keyPart := entry[:idx]
		value := entry[idx+1:]

		name, modifier, hasModifier := splitKeyModifier(keyPart)
		if name == "" {
			return nil, nil, clierr.NewUsageError("invalid --param %q: empty key", entry)
		}

		if !hasModifier {
			out[name] = decodeValue(value)
			continue
		}

		if modifier != "embed" {
			return nil, nil, clierr.NewUsageError("invalid --param %q: unknown modifier %q", entry, modifier)
		}

		// `:embed` — value is treated as raw text, except a JSON array is
		// rejected (caller almost certainly meant text and is shadowing the
		// modifier with a literal vector).
		if looksLikeJSONArray(value) {
			return nil, nil, clierr.NewUsageError("--param %q: :embed expects text, got JSON array", entry)
		}
		embeds = append(embeds, EmbedJob{Name: name, Text: value})
	}
	return out, embeds, nil
}

// splitKeyModifier splits "name:modifier" into ("name", "modifier", true);
// for "name" it returns ("name", "", false). The split is on the first `:`.
func splitKeyModifier(key string) (name, modifier string, hasModifier bool) {
	idx := strings.Index(key, ":")
	if idx < 0 {
		return key, "", false
	}
	return key[:idx], key[idx+1:], true
}

// decodeValue mirrors the legacy literal-param behaviour: JSON-decode if
// possible, otherwise fall back to the raw string.
func decodeValue(value string) any {
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return value
	}
	return decoded
}

// looksLikeJSONArray reports whether value parses as a JSON array. Used
// only to reject `:embed=[...]` entries — any other JSON shape is fine
// (text containing `{` or numerals is just text under `:embed`).
func looksLikeJSONArray(value string) bool {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "[") {
		return false
	}
	var arr []any
	return json.Unmarshal([]byte(trimmed), &arr) == nil
}
