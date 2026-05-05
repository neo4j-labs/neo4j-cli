// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	commonoutput "github.com/neo4j/cli/common/output"
)

// renderResult holds the columns, rows, and truncation metadata for a single
// query execution. It implements commonoutput.ResponseData so it can be
// rendered by PrintBodyMap in either table or JSON mode.
//
// MarshalJSON emits the consumer-facing envelope:
//
//	{"columns": [...], "rows": [...], "truncated": bool, "arrays_truncated": N}
//
// AsArray returns the rows as a slice of column-keyed maps, which PrintBodyMap
// uses for table rendering (column order is preserved by the fields slice).
type renderResult struct {
	columns         []string
	rows            []map[string]any
	truncated       bool
	arraysTruncated int
}

// AsArray implements commonoutput.ResponseData. Each row is returned as a
// column-name → pre-formatted-string map so that common/output.printTable can
// render them correctly. Each cell is formatted by formatCell: strings are
// emitted as-is, nil as "null", and everything else as compact JSON. Column
// ordering for table rendering is controlled by the fields slice passed to
// PrintBodyMap.
func (r renderResult) AsArray() []map[string]any {
	if r.rows == nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, len(r.rows))
	for i, row := range r.rows {
		formatted := make(map[string]any, len(row))
		for k, v := range row {
			formatted[k] = formatCell(v)
		}
		out[i] = formatted
	}
	return out
}

// MarshalJSON preserves the existing consumer-facing JSON schema:
//
//	{columns, rows, truncated, arrays_truncated}
//
// Field order is fixed via struct field order so encoding/json preserves it.
func (r renderResult) MarshalJSON() ([]byte, error) {
	cols := r.columns
	if cols == nil {
		cols = []string{}
	}
	rows := r.rows
	if rows == nil {
		rows = []map[string]any{}
	}
	return json.Marshal(struct {
		Columns         []string         `json:"columns"`
		Rows            []map[string]any `json:"rows"`
		Truncated       bool             `json:"truncated"`
		ArraysTruncated int              `json:"arrays_truncated"`
	}{
		Columns:         cols,
		Rows:            rows,
		Truncated:       r.truncated,
		ArraysTruncated: r.arraysTruncated,
	})
}

// renderRows writes the query result to cmd's stdout via PrintBodyMap, which
// delegates to ResolveOutput for TTY auto-detection. Explicit --output
// table|json always wins; "default" or "" auto-detects: TTY → table, non-TTY
// → JSON.
func renderRows(cmd *cobra.Command, cfg *clicfg.Config, columns []string, rows []map[string]any, truncated bool, arraysTruncated int) {
	result := renderResult{
		columns:         columns,
		rows:            rows,
		truncated:       truncated,
		arraysTruncated: arraysTruncated,
	}
	commonoutput.PrintBodyMap(cmd, cfg, result, columns)
}

// rowsFromValues converts the API's positional values (one []any per row, in
// column order) into {column: value} maps preserving the column ordering. If
// a row has fewer values than columns, missing positions are filled with nil;
// extra positional values are dropped.
func rowsFromValues(columns []string, values [][]any) []map[string]any {
	rows := make([]map[string]any, 0, len(values))
	for _, vs := range values {
		row := make(map[string]any, len(columns))
		for i, col := range columns {
			if i < len(vs) {
				row[col] = vs[i]
			} else {
				row[col] = nil
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// formatCell renders a single cell value as text. Strings are emitted as-is
// (no surrounding quotes) so the table reads naturally; everything else is
// JSON-stringified so nested objects, arrays, numbers, booleans, and nil
// remain unambiguous.
func formatCell(v any) string {
	switch val := v.(type) {
	case nil:
		return "null"
	case string:
		return val
	default:
		bytes, err := json.Marshal(val)
		if err != nil {
			// json.Marshal of a Go value parsed from JSON cannot fail;
			// surface the value via fmt as a last resort.
			return fmt.Sprintf("%v", val)
		}
		return string(bytes)
	}
}
