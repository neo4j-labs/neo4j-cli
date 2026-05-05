// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	toon "github.com/toon-format/toon-go"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// StdoutIsTerminal is the package-level test seam for terminal detection.
// Production initialisation checks whether the writer is an *os.File and, if
// so, calls term.IsTerminal on its file descriptor. Non-*os.File writers (e.g.
// a *bytes.Buffer in tests) always return false. Tests may replace this var and
// restore it via t.Cleanup.
var StdoutIsTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// ResolveOutput returns the effective output mode ("json", "table", or "toon")
// for the current invocation. When cfg.Global.Format() is "json", "table", or
// "toon" that value is returned unchanged — an explicit --format flag always
// wins. For any other value ("default", "", or an unknown value) the mode is
// auto-detected from cmd.OutOrStdout(): a TTY stdout yields "table", a
// non-TTY (piped/redirected) stdout yields "json".
func ResolveOutput(cmd *cobra.Command, cfg *clicfg.Config) string {
	v := cfg.Global.Format()
	if v == "json" || v == "table" || v == "toon" {
		return v
	}
	if StdoutIsTerminal(cmd.OutOrStdout()) {
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

// printToon renders values as a TOON document. It first marshals values to
// canonical JSON (honouring any MarshalJSON implementations), unmarshals to
// any to obtain a plain Go value, then encodes with toon.Marshal.
func printToon(cmd *cobra.Command, values ResponseData) {
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
