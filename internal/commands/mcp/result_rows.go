// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import "encoding/json"

// asArrayRower is satisfied by any row type that can produce a
// map[string]any for output.ResponseData's AsArray.
type asArrayRower interface {
	asArrayRow() map[string]any
}

// resultRows is a generic wrapper around []T that implements
// output.ResponseData by delegating to each element's asArrayRow method.
type resultRows[T asArrayRower] []T

func (r resultRows[T]) AsArray() []map[string]any {
	out := make([]map[string]any, len(r))
	for i, row := range r {
		out[i] = row.asArrayRow()
	}
	return out
}

func (r resultRows[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal([]T(r))
}
