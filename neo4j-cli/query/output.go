// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"encoding/json"
	"fmt"
	"strings"

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
	stats           *writeStats
	// plan and profile are the driver-free EXPLAIN/PROFILE plan trees (mutually
	// exclusive), carried through truncateResult so the downstream renderer can
	// emit them. Both are nil for a non-EXPLAIN/PROFILE run.
	plan    *planNode
	profile *planNode
	// errMsg is set only for an error-placeholder result produced under
	// --continue-on-error: the statement failed, so it has no columns/rows but
	// keeps its positional slot in the rendered array. Empty for successes.
	errMsg string
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
// The `stats` and `error` keys are additive (omitempty): a successful read
// stays byte-identical, while an error-placeholder (--continue-on-error) keeps
// its positional slot and surfaces the failure under `error`.
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
		Stats           *writeStats      `json:"stats,omitempty"`
		Error           string           `json:"error,omitempty"`
	}{
		Columns:         cols,
		Rows:            rows,
		Truncated:       r.truncated,
		ArraysTruncated: r.arraysTruncated,
		Stats:           r.stats,
		Error:           r.errMsg,
	})
}

// renderRows writes the query result to cmd's stdout via PrintBodyMap, which
// delegates to ResolveOutput for TTY auto-detection. Explicit --format
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

// renderResults writes one or more query results to cmd's stdout. A single
// result takes the existing PrintBodyMap path so its output is byte-identical
// to the single-statement case; multiple results delegate to PrintBodyMaps,
// which emits a JSON array, stacked tables, or the TOON array form depending on
// the resolved format. Each result carries its own column ordering.
func renderResults(cmd *cobra.Command, cfg *clicfg.Config, results []renderResult) {
	if len(results) == 1 {
		commonoutput.PrintBodyMap(cmd, cfg, results[0], results[0].columns)
		renderStatsLines(cmd, cfg, results)
		return
	}
	items := make([]commonoutput.ResponseData, len(results))
	fields := make([][]string, len(results))
	for i, r := range results {
		items[i] = r
		fields[i] = r.columns
	}
	commonoutput.PrintBodyMaps(cmd, cfg, items, fields)
	renderStatsLines(cmd, cfg, results)
}

// renderStatsLines writes a per-statement write-summary line to stdout, but only
// in table mode — JSON/TOON carry the stats inside the envelope instead. A
// result with no mutations (nil stats) produces no line, so reads stay
// byte-identical. With more than one statement each line is prefixed
// "statement N: " to match truncateResult's warning convention.
func renderStatsLines(cmd *cobra.Command, cfg *clicfg.Config, results []renderResult) {
	if commonoutput.ResolveOutput(cmd, cfg) != "table" {
		return
	}
	multi := len(results) > 1
	for i, r := range results {
		if r.stats == nil {
			continue
		}
		prefix := ""
		if multi {
			prefix = fmt.Sprintf("statement %d: ", i+1)
		}
		cmd.Println(prefix + formatStatsLine(r.stats))
	}
}

// formatStatsLine renders a write-summary as a comma-separated list of only the
// non-zero counters (cypher-shell style), e.g.
// "2 nodes created, 1 relationship created, 5 properties set". A writeStats is
// only ever non-nil when at least one counter is set, so the slice is never
// empty.
func formatStatsLine(s *writeStats) string {
	parts := make([]string, 0, 12)
	add := func(n int, singular, plural string) {
		if n == 0 {
			return
		}
		noun := plural
		if n == 1 {
			noun = singular
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, noun))
	}
	add(s.NodesCreated, "node created", "nodes created")
	add(s.NodesDeleted, "node deleted", "nodes deleted")
	add(s.RelationshipsCreated, "relationship created", "relationships created")
	add(s.RelationshipsDeleted, "relationship deleted", "relationships deleted")
	add(s.PropertiesSet, "property set", "properties set")
	add(s.LabelsAdded, "label added", "labels added")
	add(s.LabelsRemoved, "label removed", "labels removed")
	add(s.IndexesAdded, "index added", "indexes added")
	add(s.IndexesRemoved, "index removed", "indexes removed")
	add(s.ConstraintsAdded, "constraint added", "constraints added")
	add(s.ConstraintsRemoved, "constraint removed", "constraints removed")
	add(s.SystemUpdates, "system update", "system updates")
	return strings.Join(parts, ", ")
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
// remain unambiguous. String cells are passed through commonoutput.StripControl
// so an embedded ANSI escape or other C0 control byte cannot corrupt the
// terminal table output. The JSON-marshal branch is left untouched: encoding/json
// already escapes control bytes, so a double-strip would mutate legitimate data.
func formatCell(v any) string {
	switch val := v.(type) {
	case nil:
		return "null"
	case string:
		return commonoutput.StripControl(val)
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
