// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/neo4j/cli/common/agent"
	"github.com/neo4j/cli/common/clicfg"
	toon "github.com/toon-format/toon-go"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// StripControl replaces C0 control characters (runes < 0x20) and DEL (0x7F)
// with "?", except for the whitespace runes "\t", "\n", and "\r" which are
// preserved. This sanitises raw user data before rendering into a terminal
// table cell so that an embedded ANSI escape (e.g. "\x1b[31m") cannot inject
// styling, move the cursor, or otherwise corrupt the output. The JSON-marshal
// path already escapes these bytes via encoding/json so it does NOT need this
// helper.
func StripControl(s string) string {
	if s == "" {
		return s
	}
	// Fast path: scan for any rune that needs replacing before allocating.
	needs := false
	for _, r := range s {
		if (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7F {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7F {
			b.WriteByte('?')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// StdoutIsTerminal is the package-level test seam for terminal detection. It
// reads the real os.Stdout file descriptor directly so that wrapping the
// command's writer (e.g. the tee io.MultiWriter installed in main.go) cannot
// affect format resolution (CLI-210). Mirrors common/flags.stdoutIsTerminal.
// Tests may replace this var and restore it via t.Cleanup.
var StdoutIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// IsAgent is the package-level test seam for agent-harness detection. When it
// reports true and no explicit --format is given, output defaults to toon.
// Defaults to common/agent.Detect.
var IsAgent = agent.Detect

// ResolveOutput returns the effective output mode ("json", "table", or "toon")
// for the current invocation. When cfg.Global.Format() is "json", "table", or
// "toon" that value is returned unchanged — an explicit --format flag always
// wins. Otherwise the mode is auto-detected: an agent harness yields "toon", a
// TTY stdout yields "table", and a non-TTY (piped/redirected) stdout yields
// "json". The cmd parameter is retained for call-site compatibility.
func ResolveOutput(cmd *cobra.Command, cfg *clicfg.Config) string {
	v := cfg.Global.Format()
	if v == "json" || v == "table" || v == "toon" {
		return v
	}
	if IsAgent() {
		return "toon"
	}
	if StdoutIsTerminal() {
		return "table"
	}
	return "json"
}

// ResponseData is the interface that all API response types must satisfy to be
// rendered by PrintBodyMap.
type ResponseData interface {
	AsArray() []map[string]any
}

// PrintBodyMap renders values to the command output in the format resolved by
// ResolveOutput (explicit "json"/"table"/"toon" config wins; otherwise TTY-detected).
func PrintBodyMap(cmd *cobra.Command, cfg *clicfg.Config, values ResponseData, fields []string) {
	switch ResolveOutput(cmd, cfg) {
	case "json":
		bytes, err := json.MarshalIndent(values, "", "\t")
		if err != nil {
			panic(err)
		}
		cmd.Println(string(bytes))
	case "toon":
		printToon(cmd, values)
	default:
		printTable(cmd, values, fields)
	}
}

// PrintBodyMaps renders multiple result envelopes to the command output in the
// format resolved by ResolveOutput. Each item carries its own column ordering
// in the matching fields entry (items[i] uses fields[i]):
//
//   - json: a single JSON array of envelopes (each element via its own MarshalJSON).
//   - toon: the slice marshalled through the TOON path (array form).
//   - table: one table block per item, separated by a blank line.
func PrintBodyMaps(cmd *cobra.Command, cfg *clicfg.Config, items []ResponseData, fields [][]string) {
	switch ResolveOutput(cmd, cfg) {
	case "json":
		bytes, err := json.MarshalIndent(items, "", "\t")
		if err != nil {
			panic(err)
		}
		cmd.Println(string(bytes))
	case "toon":
		printToonValue(cmd, items)
	default:
		for i, item := range items {
			if i > 0 {
				cmd.Println()
			}
			printTable(cmd, item, fields[i])
		}
	}
}

// rawRows adapts a decoded JSON array of objects to the ResponseData interface
// printTable consumes, so a passthrough response needs no json-tagged output
// struct of its own.
type rawRows []map[string]any

func (r rawRows) AsArray() []map[string]any { return r }

// PrintPassthrough renders a raw HTTP response body in the format resolved by
// ResolveOutput without imposing any envelope on it:
//
//   - json: the body byte-for-byte, with a single trailing newline, so `| jq`
//     sees exactly what the server sent.
//   - toon: the body decoded, control-stripped and re-encoded as TOON.
//   - table: rows derived from the body shape — a `data` array of objects, a
//     bare array of objects, or a bare object as a single row.
//
// An empty body writes nothing. Any shape it cannot render as a table or TOON —
// a scalar, a null, an array of non-objects, or invalid JSON — falls back to the
// body itself, so an unmodelled or non-JSON upstream response is still shown.
// Unlike api.ParseBody / api.ParseRawBody it never panics on a body shape.
func PrintPassthrough(cmd *cobra.Command, cfg *clicfg.Config, body []byte) {
	if len(body) == 0 {
		return
	}
	switch ResolveOutput(cmd, cfg) {
	case "json":
		writePassthrough(cmd, body)
	case "toon":
		printPassthroughToon(cmd, body)
	default:
		printPassthroughTable(cmd, body)
	}
}

func writePassthrough(cmd *cobra.Command, b []byte) {
	w := cmd.OutOrStdout()
	_, _ = w.Write(b)
	if !bytes.HasSuffix(b, []byte("\n")) {
		_, _ = io.WriteString(w, "\n")
	}
}

// writeUnrenderable writes a body the table and toon branches could not render.
// Those formats resolve for a terminal or an agent harness, and only the json
// branch's byte-for-byte contract forbids rewriting, so control bytes are
// stripped here — an unparseable upstream body is attacker-influenced text.
func writeUnrenderable(cmd *cobra.Command, body []byte) {
	writePassthrough(cmd, []byte(StripControl(string(body))))
}

func printPassthroughToon(cmd *cobra.Command, body []byte) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		writeUnrenderable(cmd, body)
		return
	}
	toonBytes, err := toon.Marshal(stripControlDeep(v), toon.WithLengthMarkers(true))
	if err != nil {
		writeUnrenderable(cmd, body)
		return
	}
	writePassthrough(cmd, toonBytes)
}

func printPassthroughTable(cmd *cobra.Command, body []byte) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		writeUnrenderable(cmd, body)
		return
	}
	// Strip before deriving columns: stripControlDeep rewrites map keys as well
	// as string values, so the header cells and the lookup keys stay in sync.
	rows, ok := passthroughRows(stripControlDeep(v))
	if !ok {
		writeUnrenderable(cmd, body)
		return
	}
	fields := passthroughFields(rows)
	// printTable reads ":" in a field as a nested-key path. Every other caller
	// hand-writes its fields, but these are response keys, so a key carrying a
	// colon would address a sub-key that does not exist and render an empty cell.
	if len(fields) == 0 || slices.ContainsFunc(fields, func(f string) bool { return strings.Contains(f, ":") }) {
		writeUnrenderable(cmd, body)
		return
	}
	printTable(cmd, rawRows(rows), fields)
}

// passthroughRows maps a decoded response body to table rows. A top-level object
// carrying a `data` array of objects contributes that array, a bare array of
// objects contributes itself, and any other object is a single row. Every other
// shape reports false so the caller can fall back to the verbatim body.
func passthroughRows(v any) ([]map[string]any, bool) {
	switch val := v.(type) {
	case map[string]any:
		if rows, ok := objectArray(val["data"]); ok {
			return rows, true
		}
		return []map[string]any{val}, true
	case []any:
		return objectArray(val)
	default:
		return nil, false
	}
}

// objectArray returns v as a slice of objects, reporting false unless v is an
// array whose every element is a JSON object.
func objectArray(v any) ([]map[string]any, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		rows = append(rows, row)
	}
	return rows, true
}

// passthroughFields returns the union of the rows' keys, sorted within each row
// and unioned in row order. encoding/json discards the response's own object key
// order, so sorting is what makes the column order deterministic.
func passthroughFields(rows []map[string]any) []string {
	seen := make(map[string]bool)
	fields := []string{}
	for _, row := range rows {
		keys := make([]string, 0, len(row))
		for k := range row {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if seen[k] {
				continue
			}
			seen[k] = true
			fields = append(fields, k)
		}
	}
	return fields
}

// printToon renders a single ResponseData as a TOON document.
func printToon(cmd *cobra.Command, values ResponseData) {
	printToonValue(cmd, values)
}

// printToonValue renders an arbitrary value as a TOON document. It first
// marshals to canonical JSON (honouring any MarshalJSON implementations),
// unmarshals to any to obtain a plain Go value, then encodes with toon.Marshal.
func printToonValue(cmd *cobra.Command, values any) {
	jsonBytes, err := json.Marshal(values)
	if err != nil {
		panic(err)
	}
	var v any
	if err := json.Unmarshal(jsonBytes, &v); err != nil {
		panic(err)
	}
	// toon.Marshal rejects C0 control bytes (e.g. ESC, BEL) in strings — the
	// json -> any round-trip decodes JSON-escaped control bytes back into raw
	// bytes, so attacker-stored data would otherwise panic. StripControl
	// neutralises every control byte toon rejects while preserving the \t \n \r
	// that toon accepts. The JSON fallback handles any future toon rejection
	// without crashing on a data-driven condition.
	v = stripControlDeep(v)
	toonBytes, err := toon.Marshal(v, toon.WithLengthMarkers(true))
	if err != nil {
		cmd.Println(string(jsonBytes))
		return
	}
	cmd.Println(string(toonBytes))
}

// stripControlDeep recursively applies StripControl to every string value and
// map key in the shapes produced by encoding/json unmarshal-to-any. Numbers,
// bools and nil are returned unchanged.
func stripControlDeep(v any) any {
	switch val := v.(type) {
	case string:
		return StripControl(val)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[StripControl(k)] = stripControlDeep(item)
		}
		return out
	case []any:
		for i, item := range val {
			val[i] = stripControlDeep(item)
		}
		return val
	default:
		return v
	}
}

func getNestedField(v map[string]any, subFields []string) string {
	if len(subFields) == 1 {
		value := v[subFields[0]]
		if value == nil {
			return ""
		}
		if s, ok := value.(fmt.Stringer); ok {
			return s.String()
		}
		if reflect.TypeOf(value).Kind() == reflect.Slice {
			marshaledSlice, _ := json.MarshalIndent(value, "", "  ")
			return string(marshaledSlice)
		}
		// Strip control bytes from strings only — the json branch above already
		// escapes them, and numbers/bools carry none.
		if s, ok := value.(string); ok {
			return StripControl(s)
		}
		// encoding/json decodes every JSON number into float64, and %v renders a
		// large one in scientific notation — an int64-typed API field such as
		// maximum_bytes_billed would print as "1e+12" instead of its digits. Format
		// without an exponent so the table cell shows the same value the JSON
		// output does. 'f'/-1 keeps fractional values exact ("0.36" stays "0.36").
		if f, ok := value.(float64); ok {
			return strconv.FormatFloat(f, 'f', -1, 64)
		}
		return fmt.Sprintf("%+v", value)
	}
	switch val := v[subFields[0]].(type) {
	case map[string]any:
		return getNestedField(val, subFields[1:])
	default:
		//The field is no longer nested, so we can't proceed in the next level
		return ""
	}
}

func printTable(cmd *cobra.Command, responseData ResponseData, fields []string) {
	t := table.NewWriter()

	header := table.Row{}
	for _, f := range fields {
		header = append(header, f)
	}

	t.AppendHeader(header)
	for _, v := range responseData.AsArray() {
		row := table.Row{}
		for _, f := range fields {
			subfields := strings.Split(f, ":")
			formattedValue := getNestedField(v, subfields)

			row = append(row, formattedValue)
		}
		t.AppendRow(row)
	}

	t.SetStyle(table.StyleLight)
	cmd.Println(t.Render())
}
