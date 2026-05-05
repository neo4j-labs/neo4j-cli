// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/neo4j/cli/common/clicfg"
)

// stdoutIsTerminal is the test seam for terminal detection on the writer the
// renderer is about to print to. Production asserts the writer is *os.File and
// calls term.IsTerminal on its fd; non-*os.File (e.g. a *bytes.Buffer in tests)
// returns false. Tests substitute this seam to simulate either a TTY or a
// piped/redirected stdout. Mirrors stdinIsTTY at run.go.
var stdoutIsTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// resolveOutput returns the effective output mode ("json" or "table") for the
// current invocation. When cfg.Aura.Output() is "default" or "" the mode is
// auto-detected from cmd.OutOrStdout(): TTY → "table", non-TTY → "json".
// Any other configured value (e.g. "json", "table") passes through unchanged
// — explicit --output always wins.
func resolveOutput(cmd *cobra.Command, cfg *clicfg.Config) string {
	v := cfg.Aura.Output()
	if v != "default" && v != "" {
		return v
	}
	if stdoutIsTerminal(cmd.OutOrStdout()) {
		return "table"
	}
	return "json"
}

// jsonRowsResult is the JSON shape emitted in JSON output mode. Field order
// (columns, rows, truncated, arrays_truncated) is fixed via struct field
// order so encoding/json preserves it for downstream consumers. The
// arrays_truncated field always emits (zero value when nothing was elided)
// so the schema is stable for jq-style consumers.
type jsonRowsResult struct {
	Columns         []string         `json:"columns"`
	Rows            []map[string]any `json:"rows"`
	Truncated       bool             `json:"truncated"`
	ArraysTruncated int              `json:"arrays_truncated"`
}

// renderRows writes the query result to cmd's stdout in either JSON or table
// form, branching on resolveOutput(cmd, cfg). When --output is "default" (the
// implicit value), the renderer auto-detects: TTY stdout → table, piped or
// redirected stdout → JSON. Explicit --output table|json always wins. Rows
// must already be shaped as {column: value} maps — use rowsFromValues to
// convert raw positional API values. The truncated flag is propagated to the
// JSON output but does not itself emit a warning; the caller (runQuery)
// prints any stderr warning. arraysTruncated is the aggregate count of slices
// elided by --truncate-arrays-over and is always emitted in the JSON envelope.
func renderRows(cmd *cobra.Command, cfg *clicfg.Config, columns []string, rows []map[string]any, truncated bool, arraysTruncated int) {
	if rows == nil {
		rows = []map[string]any{}
	}

	if resolveOutput(cmd, cfg) == "json" {
		printJSONRows(cmd, columns, rows, truncated, arraysTruncated)
		return
	}

	printTableRows(cmd, columns, rows)
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

func printJSONRows(cmd *cobra.Command, columns []string, rows []map[string]any, truncated bool, arraysTruncated int) {
	if columns == nil {
		columns = []string{}
	}
	out := jsonRowsResult{
		Columns:         columns,
		Rows:            rows,
		Truncated:       truncated,
		ArraysTruncated: arraysTruncated,
	}
	bytes, err := json.MarshalIndent(out, "", "\t")
	if err != nil {
		// Encoding our own struct cannot fail in practice; mirror the existing
		// output package's posture (panic on impossible-state).
		panic(err)
	}
	cmd.Println(string(bytes))
}

func printTableRows(cmd *cobra.Command, columns []string, rows []map[string]any) {
	t := table.NewWriter()

	header := make(table.Row, 0, len(columns))
	for _, c := range columns {
		header = append(header, c)
	}
	t.AppendHeader(header)

	for _, r := range rows {
		row := make(table.Row, 0, len(columns))
		for _, c := range columns {
			row = append(row, formatCell(r[c]))
		}
		t.AppendRow(row)
	}

	t.SetStyle(table.StyleLight)
	cmd.Println(t.Render())
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
