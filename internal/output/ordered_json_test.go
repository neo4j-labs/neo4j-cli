// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	toon "github.com/toon-format/toon-go"
)

func TestDecodeOrderedJSON_PreservesKeyOrderAtEveryDepth(t *testing.T) {
	v, err := decodeOrderedJSON([]byte(`{"z":1,"a":{"n":true,"b":[2,"x"]},"m":null}`))
	require.NoError(t, err)

	obj, ok := v.(toon.Object)
	require.True(t, ok, "top level must decode to toon.Object, got %T", v)
	assert.Equal(t, []string{"z", "a", "m"}, fieldKeys(obj))

	inner, ok := obj.Fields[1].Value.(toon.Object)
	require.True(t, ok, "nested object must decode to toon.Object, got %T", obj.Fields[1].Value)
	assert.Equal(t, []string{"n", "b"}, fieldKeys(inner))

	arr, ok := inner.Fields[1].Value.([]any)
	require.True(t, ok, "nested array must decode to []any, got %T", inner.Fields[1].Value)
	assert.Equal(t, []any{float64(2), "x"}, arr)
}

func TestDecodeOrderedJSON_ScalarShapesMatchUnmarshal(t *testing.T) {
	v, err := decodeOrderedJSON([]byte(`{"i":1,"f":1.5,"b":true,"n":null,"s":"x","e":{},"ea":[]}`))
	require.NoError(t, err)
	obj := v.(toon.Object)
	byKey := map[string]any{}
	for _, f := range obj.Fields {
		byKey[f.Key] = f.Value
	}
	// Numbers become float64 exactly like json.Unmarshal-into-any, so the
	// toon scalar formatting of the old map round-trip is unchanged.
	assert.Equal(t, float64(1), byKey["i"])
	assert.Equal(t, float64(1.5), byKey["f"])
	assert.Equal(t, true, byKey["b"])
	assert.Nil(t, byKey["n"])
	assert.Equal(t, "x", byKey["s"])
	empty, ok := byKey["e"].(toon.Object)
	require.True(t, ok)
	assert.True(t, empty.IsEmpty())
	emptyArr, ok := byKey["ea"].([]any)
	require.True(t, ok)
	assert.Empty(t, emptyArr)
	assert.NotNil(t, emptyArr, "empty array must be []any{}, not nil")
}

func TestDecodeOrderedJSON_RejectsTrailingData(t *testing.T) {
	_, err := decodeOrderedJSON([]byte(`{} 1`))
	assert.Error(t, err)
}

func TestWalkStrings_ToonObjectTransformsKeysAndValuesInPlace(t *testing.T) {
	in := toon.NewObject(
		toon.Field{Key: "k\x1b", Value: toon.NewObject(
			toon.Field{Key: "inner", Value: "v\x1b"},
		)},
	)
	out := WalkStrings(in, StripControl).(toon.Object)
	assert.Equal(t, []string{"k?"}, fieldKeys(out))
	inner := out.Fields[0].Value.(toon.Object)
	assert.Equal(t, "v?", inner.Fields[0].Value)
}

// declarationOrdered is a ResponseData whose MarshalJSON emits keys in a
// non-alphabetical declaration order — the shape whose TOON rendering used to
// be silently re-alphabetized by the map round-trip.
type declarationOrdered struct{}

func (declarationOrdered) AsArray() []map[string]any {
	return []map[string]any{{"zebra": "z", "alpha": "a"}}
}

func (declarationOrdered) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Zebra string `json:"zebra"`
		Alpha string `json:"alpha"`
	}{"z", "a"})
}

func TestPrintToon_PreservesJSONDeclarationOrder(t *testing.T) {
	cmd, cfg, stdout := newOutputCmd(t, "toon")
	PrintBodyMap(cmd, cfg, declarationOrdered{}, []string{"zebra", "alpha"})
	out := stdout.String()

	// The JSON path has always emitted declaration order; the TOON path must
	// agree with it instead of re-alphabetizing the keys.
	assert.Less(t, strings.Index(out, "zebra"), strings.Index(out, "alpha"),
		"toon key order must match the JSON declaration order, got: %s", out)
}

func fieldKeys(obj toon.Object) []string {
	keys := make([]string, len(obj.Fields))
	for i, f := range obj.Fields {
		keys[i] = f.Key
	}
	return keys
}
