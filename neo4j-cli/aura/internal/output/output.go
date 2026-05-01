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
	"github.com/neo4j/cli/neo4j-cli/aura/internal/api"
)

func PrintBodyMap(cmd *cobra.Command, cfg *clicfg.Config, values api.ResponseData, fields []string) {
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

// Prints the response body, taking the output configuration into account. Only the defined fields will be printed in table mode. The full output will be printed in json
func PrintBody(cmd *cobra.Command, cfg *clicfg.Config, body []byte, fields []string) {
	if len(body) == 0 {
		return
	}
	values := api.ParseBody(body)

	PrintBodyMap(cmd, cfg, values, fields)
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

func printTable(cmd *cobra.Command, responseData api.ResponseData, fields []string) {
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
