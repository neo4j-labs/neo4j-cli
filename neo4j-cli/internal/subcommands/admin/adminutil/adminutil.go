// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package adminutil provides shared types and helpers used by all admin
// sub-packages (database, user, role).
package adminutil

import (
	"context"
	"encoding/json"
	"unicode"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

// ExecFn is the Cypher execution function signature shared by all admin
// sub-packages. It is set by each package's NewCmd from the injected
// admin.RunAdminStatement and replaced by tests to avoid real Bolt connections.
type ExecFn func(ctx context.Context, cfg *clicfg.Config, conn *dbconn.Conn, cypher string, params map[string]any) ([]map[string]any, error)

// Rows adapts a []map[string]any into commonoutput.ResponseData.
type Rows []map[string]any

func (r Rows) AsArray() []map[string]any {
	if r == nil {
		return []map[string]any{}
	}
	return r
}

func (r Rows) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.AsArray())
}

// Row adapts a single map[string]any into commonoutput.ResponseData.
// MarshalJSON produces a JSON object (not an array), suitable for get and
// write-command follow-up output where there is always exactly one result.
type Row map[string]any

func (r Row) AsArray() []map[string]any { return []map[string]any{r} }

func (r Row) MarshalJSON() ([]byte, error) { return json.Marshal(map[string]any(r)) }

// NewRow normalizes the keys of a single Bolt result row from camelCase to
// snake_case, retains only the keys named in fields, and returns the result as
// a Row. The fields list also drives table rendering, so JSON and table output
// are always consistent.
func NewRow(m map[string]any, fields []string) Row {
	normalized := normalizeMap(m)
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		out[f] = normalized[f]
	}
	return Row(out)
}

// NewRows normalizes and filters each row in a Bolt result set to exactly the
// keys named in fields. JSON and table output are always consistent.
func NewRows(ms []map[string]any, fields []string) Rows {
	out := make(Rows, len(ms))
	for i, m := range ms {
		normalized := normalizeMap(m)
		row := make(map[string]any, len(fields))
		for _, f := range fields {
			row[f] = normalized[f]
		}
		out[i] = row
	}
	return out
}

// normalizeMap converts all keys of m from camelCase to snake_case.
func normalizeMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[camelToSnake(k)] = v
	}
	return out
}

// camelToSnake converts a camelCase or PascalCase identifier to snake_case.
// Consecutive uppercase sequences such as "ID" are treated as a single word,
// so "databaseID" → "database_id" and "currentStatus" → "current_status".
func camelToSnake(s string) string {
	runes := []rune(s)
	var b []rune
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 {
			prev := runes[i-1]
			var next rune
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			if unicode.IsLower(prev) || (next != 0 && unicode.IsLower(next)) {
				b = append(b, '_')
			}
		}
		b = append(b, unicode.ToLower(r))
	}
	return string(b)
}
