// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build cgo && relate

package relate

import (
	"reflect"
	"time"

	relateclient "github.com/relatedb/relate/clients/go"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
)

// toClientParams maps statement parameters onto the Go client's typed values,
// the reverse of convertValue. The client binds its own Date/Time/Point/Vector
// kinds through the engine's typed putters; the driver-side dbtype values the
// CLI carries (a Bolt pull, --param :embed) would otherwise fall into the
// binder's generic struct arm and bind as maps of their fields.
func toClientParams(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}
	out := make(map[string]any, len(params))
	for k, v := range params {
		out[k] = toClientValue(v)
	}
	return out
}

func toClientValue(v any) any {
	switch val := v.(type) {
	case dbtype.Date:
		t := time.Time(val)
		return relateclient.Date{Year: int64(t.Year()), Month: int64(t.Month()), Day: int64(t.Day())}
	case dbtype.LocalTime:
		t := time.Time(val)
		return relateclient.LocalTime{Hour: int64(t.Hour()), Minute: int64(t.Minute()), Second: int64(t.Second()), Nanosecond: int64(t.Nanosecond())}
	case dbtype.Time:
		t := time.Time(val)
		_, off := t.Zone()
		return relateclient.Time{
			Hour: int64(t.Hour()), Minute: int64(t.Minute()), Second: int64(t.Second()), Nanosecond: int64(t.Nanosecond()),
			OffsetSeconds: int64(off),
		}
	case dbtype.LocalDateTime:
		t := time.Time(val)
		return relateclient.LocalDateTime{
			Year: int64(t.Year()), Month: int64(t.Month()), Day: int64(t.Day()),
			Hour: int64(t.Hour()), Minute: int64(t.Minute()), Second: int64(t.Second()), Nanosecond: int64(t.Nanosecond()),
		}
	case time.Time:
		_, off := val.Zone()
		dt := relateclient.DateTime{
			Year: int64(val.Year()), Month: int64(val.Month()), Day: int64(val.Day()),
			Hour: int64(val.Hour()), Minute: int64(val.Minute()), Second: int64(val.Second()), Nanosecond: int64(val.Nanosecond()),
			OffsetSeconds: int64(off),
		}
		// A named IANA zone binds by name (the engine resolves the offset at
		// that wall clock); a fixed offset, UTC or the process-local zone
		// bind by offset only.
		if name := val.Location().String(); name != "" && name != "UTC" && name != "Local" {
			dt.Zone = name
		}
		return dt
	case dbtype.Duration:
		return relateclient.Duration{Months: val.Months, Days: val.Days, Seconds: val.Seconds, Nanos: int64(val.Nanos)}
	case dbtype.Point2D:
		return relateclient.Point{SRID: int32(val.SpatialRefId), Dimension: 2, X: val.X, Y: val.Y}
	case dbtype.Point3D:
		return relateclient.Point{SRID: int32(val.SpatialRefId), Dimension: 3, X: val.X, Y: val.Y, Z: val.Z}
	case dbtype.Vector[float32]:
		return floatVector("FLOAT32", val.Elems)
	case dbtype.Vector[float64]:
		return floatVector("FLOAT", val.Elems)
	case dbtype.Vector[int8]:
		return intVector("INTEGER8", val.Elems)
	case dbtype.Vector[int16]:
		return intVector("INTEGER16", val.Elems)
	case dbtype.Vector[int32]:
		return intVector("INTEGER32", val.Elems)
	case dbtype.Vector[int64]:
		return intVector("INTEGER", val.Elems)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = toClientValue(item)
		}
		return out
	case map[string]any:
		return toClientParams(val)
	}
	// An embedding ([]float32, possibly a named type such as the query
	// package's embedVector) is a FLOAT32 vector, not a list of floats.
	if rv := reflect.ValueOf(v); rv.IsValid() && rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Float32 {
		elems := make([]float32, rv.Len())
		for i := range elems {
			elems[i] = float32(rv.Index(i).Float())
		}
		return floatVector("FLOAT32", elems)
	}
	return v
}

func floatVector[T float32 | float64](dtype string, elems []T) relateclient.Vector {
	floats := make([]float64, len(elems))
	for i, e := range elems {
		floats[i] = float64(e)
	}
	return relateclient.Vector{DType: dtype, Floats: floats}
}

func intVector[T int8 | int16 | int32 | int64](dtype string, elems []T) relateclient.Vector {
	ints := make([]int64, len(elems))
	for i, e := range elems {
		ints[i] = int64(e)
	}
	return relateclient.Vector{DType: dtype, Ints: ints}
}
