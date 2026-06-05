// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
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
	toonBytes, err := toon.Marshal(v, toon.WithLengthMarkers(true))
	if err != nil {
		panic(err)
	}
	cmd.Println(string(toonBytes))
}

func getNestedField(v map[string]any, subFields []string) string {
	if len(subFields) == 1 {
		value := v[subFields[0]]
		if value == nil {
			return ""
		}
		if reflect.TypeOf(value).Kind() == reflect.Slice {
			marshaledSlice, _ := json.MarshalIndent(value, "", "  ")
			return string(marshaledSlice)
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
