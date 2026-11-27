// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build cgo && relate

package relate

import (
	"testing"
	"time"

	relateclient "github.com/relatedb/relate/clients/go"
	"github.com/stretchr/testify/assert"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
)

type namedEmbedding []float32 // stands in for the query package's embedVector

func TestToClientValue_MapsDriverTypesOntoClientKinds(t *testing.T) {
	utc := time.Date(2020, 6, 1, 10, 30, 15, 7, time.UTC)
	stockholm, err := time.LoadLocation("Europe/Stockholm")
	assert.NoError(t, err)
	fixed := time.Date(2020, 6, 1, 10, 0, 0, 0, time.FixedZone("", 3600))

	tests := []struct {
		name string
		in   any
		want any
	}{
		{"date", dbtype.Date(utc), relateclient.Date{Year: 2020, Month: 6, Day: 1}},
		{"local time", dbtype.LocalTime(utc), relateclient.LocalTime{Hour: 10, Minute: 30, Second: 15, Nanosecond: 7}},
		{"time keeps its offset", dbtype.Time(fixed), relateclient.Time{Hour: 10, OffsetSeconds: 3600}},
		{"local datetime", dbtype.LocalDateTime(utc), relateclient.LocalDateTime{Year: 2020, Month: 6, Day: 1, Hour: 10, Minute: 30, Second: 15, Nanosecond: 7}},
		{"utc datetime has no zone id", utc, relateclient.DateTime{Year: 2020, Month: 6, Day: 1, Hour: 10, Minute: 30, Second: 15, Nanosecond: 7}},
		{"fixed offset datetime has no zone id", fixed, relateclient.DateTime{Year: 2020, Month: 6, Day: 1, Hour: 10, OffsetSeconds: 3600}},
		{"named zone datetime binds by zone", time.Date(2020, 6, 1, 10, 0, 0, 0, stockholm),
			relateclient.DateTime{Year: 2020, Month: 6, Day: 1, Hour: 10, OffsetSeconds: 7200, Zone: "Europe/Stockholm"}},
		{"duration", dbtype.Duration{Months: 1, Days: 2, Seconds: 3, Nanos: 4}, relateclient.Duration{Months: 1, Days: 2, Seconds: 3, Nanos: 4}},
		{"2d point", dbtype.Point2D{SpatialRefId: 7203, X: 1, Y: 2}, relateclient.Point{SRID: 7203, Dimension: 2, X: 1, Y: 2}},
		{"3d point", dbtype.Point3D{SpatialRefId: 4979, X: 1, Y: 2, Z: 3}, relateclient.Point{SRID: 4979, Dimension: 3, X: 1, Y: 2, Z: 3}},
		{"float64 vector", dbtype.Vector[float64]{Elems: []float64{1, 2}}, relateclient.Vector{DType: "FLOAT", Floats: []float64{1, 2}}},
		{"float32 vector", dbtype.Vector[float32]{Elems: []float32{0.5}}, relateclient.Vector{DType: "FLOAT32", Floats: []float64{0.5}}},
		{"int64 vector", dbtype.Vector[int64]{Elems: []int64{1, 2}}, relateclient.Vector{DType: "INTEGER", Ints: []int64{1, 2}}},
		{"int8 vector", dbtype.Vector[int8]{Elems: []int8{-1}}, relateclient.Vector{DType: "INTEGER8", Ints: []int64{-1}}},
		{"an embedding is a float32 vector, named type or not", namedEmbedding{0.25, 0.5}, relateclient.Vector{DType: "FLOAT32", Floats: []float64{0.25, 0.5}}},
		{"scalars pass through", int64(7), int64(7)},
		{"nil passes through", nil, nil},
		{"a plain string passes through", "x", "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, toClientValue(tt.in))
		})
	}
}

func TestToClientValue_RecursesIntoContainers(t *testing.T) {
	in := map[string]any{
		"rows": []any{
			map[string]any{"props": map[string]any{"born": dbtype.Date(time.Date(1985, 3, 31, 0, 0, 0, 0, time.UTC))}},
		},
		"n": int64(1),
	}
	got := toClientParams(in)
	row := got["rows"].([]any)[0].(map[string]any)["props"].(map[string]any)
	assert.Equal(t, relateclient.Date{Year: 1985, Month: 3, Day: 31}, row["born"])
	assert.Equal(t, int64(1), got["n"])
	assert.Nil(t, toClientParams(nil))
}
