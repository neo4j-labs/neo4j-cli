// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package output renders the records the Aura client returns. Commands hand it a
// record (or a list of records) and the columns to show; it wraps them in the
// {"data": ...} envelope Aura output uses and prints them in the format the user
// asked for.
package output

import (
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/output"
)

// single is the {"data": {...}} envelope of one record.
type single struct {
	Data map[string]any `json:"data"`
}

func (d single) AsArray() []map[string]any { return []map[string]any{d.Data} }

// list is the {"data": [...]} envelope of several records.
type list struct {
	Data []map[string]any `json:"data"`
}

func (d list) AsArray() []map[string]any { return d.Data }

// PrintRecord prints one record. JSON output is the whole record; fields are the
// columns of the table form.
func PrintRecord(cmd *cobra.Command, cfg *clicfg.Config, record map[string]any, fields []string) {
	output.PrintBodyMap(cmd, cfg, single{Data: record}, fields)
}

// PrintRecords prints a list of records.
func PrintRecords(cmd *cobra.Command, cfg *clicfg.Config, records []map[string]any, fields []string) {
	output.PrintBodyMap(cmd, cfg, list{Data: records}, fields)
}

// PrintBodyMap prints any value that satisfies output.ResponseData, for the few
// commands whose result is not an Aura record (for example the workspace list).
func PrintBodyMap(cmd *cobra.Command, cfg *clicfg.Config, values output.ResponseData, fields []string) {
	output.PrintBodyMap(cmd, cfg, values, fields)
}
