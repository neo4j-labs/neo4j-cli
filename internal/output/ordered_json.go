// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	toon "github.com/toon-format/toon-go"
)

// decodeOrderedJSON parses a JSON document into the same value shapes
// json.Unmarshal(..., &any) produces — nil, bool, float64, string, []any —
// except every object becomes a toon.Object instead of a map[string]any.
// A toon.Object preserves the document's key order, so a downstream
// toon.Marshal emits keys in the order the JSON document declared them
// instead of the alphabetical order a Go map imposes.
//
// This is what keeps the TOON rendering of a ResponseData faithful to its
// JSON rendering: a MarshalJSON that emits ordered keys (e.g. query rows in
// server column order) keeps that order through the round-trip
// printToonValue performs. Numbers are converted to float64 exactly like
// json.Unmarshal-into-any, so scalar formatting is unchanged.
//
// Duplicate object keys (which json.Marshal never produces) are kept as
// duplicate fields rather than last-wins; callers only feed it documents
// produced by encoding/json.
func decodeOrderedJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeOrderedValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("ordered json: trailing data after top-level value")
	}
	return v, nil
}

// decodeOrderedValue reads the next value from dec, recursing into objects
// and arrays so nested key order is preserved at every depth.
func decodeOrderedValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			return decodeOrderedObject(dec)
		case '[':
			return decodeOrderedArray(dec)
		default:
			return nil, fmt.Errorf("ordered json: unexpected delimiter %v", t)
		}
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return nil, fmt.Errorf("ordered json: number %s: %w", t.String(), err)
		}
		return f, nil
	default:
		// string, bool, or nil — already the shape json.Unmarshal yields.
		return tok, nil
	}
}

// decodeOrderedObject consumes an object body whose opening delimiter has
// already been read, producing an order-preserving toon.Object.
func decodeOrderedObject(dec *json.Decoder) (any, error) {
	var fields []toon.Field
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("ordered json: object key is not a string")
		}
		value, err := decodeOrderedValue(dec)
		if err != nil {
			return nil, err
		}
		fields = append(fields, toon.Field{Key: key, Value: value})
	}
	if _, err := dec.Token(); err != nil { // closing '}'
		return nil, err
	}
	return toon.NewObject(fields...), nil
}

// decodeOrderedArray consumes an array body whose opening delimiter has
// already been read. An empty array decodes to []any{} (not nil), matching
// json.Unmarshal-into-any.
func decodeOrderedArray(dec *json.Decoder) (any, error) {
	items := []any{}
	for dec.More() {
		item, err := decodeOrderedValue(dec)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if _, err := dec.Token(); err != nil { // closing ']'
		return nil, err
	}
	return items, nil
}
