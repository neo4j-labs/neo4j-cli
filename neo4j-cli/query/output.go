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
// The `stats`, `plan`, `profile`, and `error` keys are additive (omitempty): a
// successful read stays byte-identical, an EXPLAIN/PROFILE run appends its
// operator tree under `plan`/`profile` (mutually exclusive, so never both), and
// an error-placeholder (--continue-on-error) keeps its positional slot and
// surfaces the failure under `error`.
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
		Plan            *planNode        `json:"plan,omitempty"`
		Profile         *planNode        `json:"profile,omitempty"`
		Error           string           `json:"error,omitempty"`
	}{
		Columns:         cols,
		Rows:            rows,
		Truncated:       r.truncated,
		ArraysTruncated: r.arraysTruncated,
		Stats:           r.stats,
		Plan:            r.plan,
		Profile:         r.profile,
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
		renderSideChannels(cmd, cfg, results)
		return
	}
	items := make([]commonoutput.ResponseData, len(results))
	fields := make([][]string, len(results))
	for i, r := range results {
		items[i] = r
		fields[i] = r.columns
	}
	commonoutput.PrintBodyMaps(cmd, cfg, items, fields)
	renderSideChannels(cmd, cfg, results)
}

// renderSideChannels writes the table-only supplementary lines (write counters
// then the EXPLAIN/PROFILE operator trees) that follow the result table. It
// guards the table-mode check once and computes the multi-statement prefix once,
// then delegates per-result line emission to the stats and plan helpers.
func renderSideChannels(cmd *cobra.Command, cfg *clicfg.Config, results []renderResult) {
	if commonoutput.ResolveOutput(cmd, cfg) != "table" {
		return
	}
	multi := len(results) > 1
	renderStatsLines(cmd, results, multi)
	renderPlanLines(cmd, results, multi)
}

// renderPlanLines writes the EXPLAIN/PROFILE operator trees to stdout. It only
// emits in table mode — the table-mode check and the multi-statement flag were
// already resolved by the caller (renderSideChannels), so they are not repeated
// here (JSON/TOON carry the plan inside the envelope instead, see
// renderResult.MarshalJSON). A result with no plan tree produces no line, so
// ordinary reads stay byte-identical to the pre-plan output. With more than one
// statement the root line of each tree is prefixed "statement N: " to match
// renderStatsLines' and truncateResult's warning convention; the rest of the
// tree is left unprefixed.
func renderPlanLines(cmd *cobra.Command, results []renderResult, multi bool) {
	for i, r := range results {
		tree, profiled := r.planInfo()
		if tree == nil {
			continue
		}
		lines := formatPlanTree(tree, profiled, 0)
		if multi {
			lines[0] = fmt.Sprintf("statement %d: %s", i+1, lines[0])
		}
		for _, line := range lines {
			cmd.Println(line)
		}
	}
}

// planInfo returns the plan tree to render and whether it came from a PROFILE
// run (in which case formatPlanTree appends per-operator metrics). plan and
// profile are mutually exclusive by construction: planFromResponse in connect.go
// is the single place that decides precedence (profile wins when both driver
// fields are non-nil), so here we just return whichever field is set. An EXPLAIN
// run reports only Plan(); an ordinary run reports neither. MarshalJSON is not a
// consumer of this accessor — it emits whichever raw field is set under its own
// key, so it reads the fields directly.
func (r renderResult) planInfo() (tree *planNode, profiled bool) {
	if r.profile != nil {
		return r.profile, true
	}
	return r.plan, false
}

// formatPlanTree renders an operator tree as one line per node, depth-first,
// indented two spaces per depth level. A node with identifiers is written
// "<Operator> => ident1, ident2"; one without identifiers is just the operator
// name. When profiled (the tree came from a PROFILE run) every line additionally
// carries its gathered metrics as " (rows: N, dbHits: M, time: Tµs)"; an EXPLAIN
// tree prints none. The profiled flag rides along from which renderResult field
// the tree came from rather than sniffing whether the metrics are zero, so a
// legitimately-zero-cost profiled operator still prints its metrics. There is no
// depth cap: the whole tree is rendered.
//
// Operator and identifier text is passed through commonoutput.StripControl for
// the same reason formatCell does it: these are server-supplied strings headed
// for a terminal, and a backtick-quoted Cypher identifier can carry an ANSI
// escape or other C0 byte. The metrics are integers, so they need no stripping.
func formatPlanTree(root *planNode, profiled bool, depth int) []string {
	if root == nil {
		return nil
	}
	line := strings.Repeat("  ", depth) + commonoutput.StripControl(root.Operator)
	if len(root.Identifiers) > 0 {
		idents := make([]string, len(root.Identifiers))
		for i, ident := range root.Identifiers {
			idents[i] = commonoutput.StripControl(ident)
		}
		line += " => " + strings.Join(idents, ", ")
	}
	if profiled {
		// planNode.Time is the server's raw nanosecond count (the driver
		// copies it off the wire with no conversion — verified against a
		// live PROFILE). Render it as microseconds so a typical operator's
		// tens-of-millions value reads as a human-scale number rather than
		// raw ns. The JSON envelope keeps the raw ns int for machine
		// consumers; only this terminal rendering converts.
		line += fmt.Sprintf(" (rows: %d, dbHits: %d, time: %dµs)", root.Rows, root.DbHits, root.Time/1000)
	}
	lines := []string{line}
	for i := range root.Children {
		lines = append(lines, formatPlanTree(&root.Children[i], profiled, depth+1)...)
	}
	return lines
}

// renderStatsLines writes a per-statement write-summary line to stdout. It only
// emits in table mode — the table-mode check and the multi-statement flag were
// already resolved by the caller (renderSideChannels), so they are not repeated
// here (JSON/TOON carry the stats inside the envelope instead). A result with no
// mutations (nil stats) produces no line, so reads stay byte-identical. With
// more than one statement each line is prefixed "statement N: " to match
// truncateResult's warning convention.
func renderStatsLines(cmd *cobra.Command, results []renderResult, multi bool) {
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
