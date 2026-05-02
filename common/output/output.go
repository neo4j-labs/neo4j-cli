// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
)

// ResponseData is the interface that all API response types must satisfy to be
// rendered by PrintBodyMap.
type ResponseData interface {
	AsArray() []map[string]any
	GetSingleOrError() (map[string]any, error)
}

// ConfigEntry represents a single configuration key-value pair.
type ConfigEntry struct {
	Key   string
	Value interface{}
}

// ConfigData is a slice of ConfigEntry that satisfies the ResponseData interface,
// enabling config commands to use PrintBodyMap for consistent rendering.
type ConfigData []ConfigEntry

// AsArray returns each entry as a {"key": k, "value": v} map for table rendering.
func (d ConfigData) AsArray() []map[string]any {
	result := make([]map[string]any, len(d))
	for i, e := range d {
		result[i] = map[string]any{
			"key":   e.Key,
			"value": e.Value,
		}
	}
	return result
}

// GetSingleOrError returns the single entry as a {"key": k, "value": v} map,
// or an error if the slice does not contain exactly one entry.
func (d ConfigData) GetSingleOrError() (map[string]any, error) {
	if len(d) != 1 {
		return nil, fmt.Errorf("expected exactly 1 config entry, got %d", len(d))
	}
	return map[string]any{
		"key":   d[0].Key,
		"value": d[0].Value,
	}, nil
}

// MarshalJSON renders ConfigData as a flat map {key: value, ...} so that
// PrintBodyMap JSON output is {"output": "json", ...} rather than an array.
func (d ConfigData) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{}, len(d))
	for _, e := range d {
		m[e.Key] = e.Value
	}
	return json.Marshal(m)
}

// PrintBodyMap renders values to the command output in the format selected by
// cfg.Global.Output().
func PrintBodyMap(cmd *cobra.Command, cfg *clicfg.Config, values ResponseData, fields []string) {
	outputType := cfg.Global.Output()

	switch output := outputType; output {
	case "json":
		bytes, err := json.MarshalIndent(values, "", "\t")
		if err != nil {
			panic(err)
		}
		cmd.Println(string(bytes))
	case "table", "default":
		printTable(cmd, values, fields)
	default:
		// This is in case the value is unknown
		cmd.Println(values)
	}
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
