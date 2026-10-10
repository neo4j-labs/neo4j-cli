// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalogcmd

import (
	"encoding/json"

	"github.com/jedib0t/go-pretty/v6/table"
	commonoutput "github.com/neo4j/cli/internal/output"
)

// listResult is the payload returned by a generated list leaf.
type listResult[T any] struct {
	items   []T
	rowJSON func(T) any
}

// AsArray satisfies commonoutput.ResponseData; table mode routes through the
// custom renderer so this returns nil.
func (r listResult[T]) AsArray() []map[string]any { return nil }

// MarshalJSON emits the RowJSON-projected array unwrapped. Empty marshals to
// `[]` (never `null`) so JSON consumers always see an array.
func (r listResult[T]) MarshalJSON() ([]byte, error) {
	out := make([]any, 0, len(r.items))
	for _, it := range r.items {
		out = append(out, r.rowJSON(it))
	}
	return json.Marshal(out)
}

// itemResult adapts a single catalog entry to `output.ResponseData` for the
// create/update leaves.
type itemResult[T any] struct {
	item      *T
	fields    []string
	rowJSON   func(T) any
	rowValues func(T) []string
}

// itemResult wraps one found entry with the spec's projections.
func (s Spec[T, S]) itemResult(item *T) itemResult[T] {
	return itemResult[T]{item: item, fields: s.Fields, rowJSON: s.RowJSON, rowValues: s.RowValues}
}

// AsArray zips Fields x RowValues into a single-row map; the values are the
// same strings the table renderer prints (printTable strips control bytes at
// render time).
func (r itemResult[T]) AsArray() []map[string]any {
	if r.item == nil {
		return nil
	}
	vals := r.rowValues(*r.item)
	row := make(map[string]any, len(r.fields))
	for i, f := range r.fields {
		row[f] = vals[i]
	}
	return []map[string]any{row}
}

// MarshalJSON emits the RowJSON projection; a nil item marshals to `null`.
// The fail-loud finder policy means leaves never reach this with a nil item.
func (r itemResult[T]) MarshalJSON() ([]byte, error) {
	if r.item == nil {
		return []byte("null"), nil
	}
	return json.Marshal(r.rowJSON(*r.item))
}

// renderTable emits the list table: a header row from fields, then one row per
// entry with control-stripped cells. Empty input yields a one-row `(none)`
// placeholder so the table is visually present.
func renderTable[T any](fields []string, items []T, rowValues func(T) []string) string {
	t := table.NewWriter()
	header := make(table.Row, 0, len(fields))
	for _, f := range fields {
		header = append(header, f)
	}
	t.AppendHeader(header)
	if len(items) == 0 {
		row := make(table.Row, len(fields))
		row[0] = "(none)"
		for i := 1; i < len(row); i++ {
			row[i] = ""
		}
		t.AppendRow(row)
	} else {
		for _, it := range items {
			vals := rowValues(it)
			row := make(table.Row, len(vals))
			for i, v := range vals {
				row[i] = commonoutput.StripControl(v)
			}
			t.AppendRow(row)
		}
	}
	t.SetStyle(table.StyleLight)
	return t.Render()
}
