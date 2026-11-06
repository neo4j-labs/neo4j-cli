// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"math"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuoteIdent(t *testing.T) {
	assert.Equal(t, "`Person`", QuoteIdent("Person"))
	assert.Equal(t, "`fo``o`", QuoteIdent("fo`o"))
}

func TestCypherLiteral(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, "NULL"},
		{"true", true, "true"},
		{"false", false, "false"},
		{"int64", int64(42), "42"},
		{"float64", 1.5, "1.5"},
		{"whole float", float64(3), "3"},
		{"exponent", 1e21, "1e21"},
		{"negative exponent", 1.5e-7, "1.5e-7"},
		{"infinity", math.Inf(1), "Infinity"},
		{"-infinity", math.Inf(-1), "-Infinity"},
		{"nan", math.NaN(), "NaN"},
		{"string", "Ada", "'Ada'"},
		{"escaped string", "a'b\\c", `'a\'b\\c'`},
		{"newline string", "a\nb", `'a\nb'`},
		{"control string", "a\x01b", `'a\u0001b'`},
		{"list", []any{int64(1), "x"}, `[1, 'x']`},
		{"map", map[string]any{"b": int64(1), "a": "x"}, `{'a': 'x', 'b': 1}`},
		{"date", dbtype.Date(time.Date(1985, 3, 31, 0, 0, 0, 0, time.UTC)), "date('1985-03-31')"},
		{"localtime", dbtype.LocalTime(time.Date(0, 1, 1, 12, 50, 35, 556000000, time.UTC)), "localtime('12:50:35.556')"},
		{"duration", dbtype.Duration{Months: 1, Days: 2, Seconds: 3, Nanos: 4}, "duration({months: 1, days: 2, seconds: 3, nanoseconds: 4})"},
		{"point2d", dbtype.Point2D{SpatialRefId: 7203, X: 1.5, Y: 2.5}, "point({srid: 7203, x: 1.5, y: 2.5})"},
		{"point3d", dbtype.Point3D{SpatialRefId: 9159, X: 1, Y: 2, Z: 3}, "point({srid: 9159, x: 1, y: 2, z: 3})"},
		{"datetime", time.Date(2026, 11, 6, 12, 0, 0, 0, time.FixedZone("", 3600)), "datetime('2026-11-06T12:00:00+01:00')"},
		{"localdatetime", dbtype.LocalDateTime(time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)), "localdatetime('2026-11-06T12:00:00')"},
		{"time", dbtype.Time(time.Date(0, 1, 1, 12, 50, 35, 0, time.FixedZone("", 3600))), "time('12:50:35+01:00')"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CypherLiteral(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("unsupported type errors", func(t *testing.T) {
		_, err := CypherLiteral(make(chan int))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chan")
	})

	t.Run("nested unsupported type names the path", func(t *testing.T) {
		_, err := CypherLiteral(map[string]any{"x": struct{}{}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "struct")
	})
}
