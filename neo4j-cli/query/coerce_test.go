// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoerceDriverValue_Scalars(t *testing.T) {
	date := dbtype.Date(time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC))
	localDT := dbtype.LocalDateTime(time.Date(2026, 5, 25, 14, 30, 45, 0, time.UTC))
	localT := dbtype.LocalTime(time.Date(0, 1, 1, 14, 30, 45, 0, time.UTC))
	timeVal := dbtype.Time(time.Date(0, 1, 1, 14, 30, 45, 0, time.UTC))
	dur := dbtype.Duration{Months: 0, Days: 1, Seconds: 0, Nanos: 0}
	nativeTime := time.Date(2026, 5, 25, 14, 30, 45, 0, time.UTC)

	type unknown struct{ X int }

	tests := []struct {
		name string
		in   any
		want any
	}{
		{
			name: "dbtype.Date renders as YYYY-MM-DD",
			in:   date,
			want: "2026-05-25",
		},
		{
			name: "dbtype.LocalDateTime renders as ISO-8601 local form",
			in:   localDT,
			want: localDT.String(),
		},
		{
			name: "dbtype.LocalTime renders as hh:mm:ss",
			in:   localT,
			want: localT.String(),
		},
		{
			name: "dbtype.Time renders as hh:mm:ss with offset",
			in:   timeVal,
			want: timeVal.String(),
		},
		{
			name: "dbtype.Duration renders as ISO-8601 duration",
			in:   dur,
			want: "P1D",
		},
		{
			name: "native time.Time renders as RFC3339Nano",
			in:   nativeTime,
			want: nativeTime.Format(time.RFC3339Nano),
		},
		{
			name: "int64 passes through",
			in:   int64(42),
			want: int64(42),
		},
		{
			name: "float64 passes through",
			in:   float64(3.14),
			want: float64(3.14),
		},
		{
			name: "string passes through",
			in:   "hello",
			want: "hello",
		},
		{
			name: "bool passes through",
			in:   true,
			want: true,
		},
		{
			name: "nil passes through",
			in:   nil,
			want: nil,
		},
		{
			name: "unknown struct passes through",
			in:   unknown{X: 7},
			want: unknown{X: 7},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := coerceDriverValue(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCoerceDriverValue_Containers(t *testing.T) {
	date := dbtype.Date(time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC))

	t.Run("[]any recurses into temporal elements", func(t *testing.T) {
		got := coerceDriverValue([]any{date, int64(1), "foo"})
		assert.Equal(t, []any{"2026-05-25", int64(1), "foo"}, got)
	})

	t.Run("map[string]any recurses into temporal values", func(t *testing.T) {
		got := coerceDriverValue(map[string]any{"d": date, "n": int64(1)})
		assert.Equal(t, map[string]any{"d": "2026-05-25", "n": int64(1)}, got)
	})

	t.Run("nested map inside slice is coerced", func(t *testing.T) {
		got := coerceDriverValue([]any{map[string]any{"d": date}})
		assert.Equal(t, []any{map[string]any{"d": "2026-05-25"}}, got)
	})
}

func TestCoerceDriverValue_GraphEntities(t *testing.T) {
	date := dbtype.Date(time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC))

	t.Run("dbtype.Node Props are coerced in place", func(t *testing.T) {
		node := dbtype.Node{
			Id:        1,
			ElementId: "n1",
			Labels:    []string{"Person"},
			Props:     map[string]any{"birthday": date, "name": "alice"},
		}
		got := coerceDriverValue(node)
		gotNode, ok := got.(dbtype.Node)
		require.True(t, ok, "expected dbtype.Node, got %T", got)
		assert.Equal(t, "2026-05-25", gotNode.Props["birthday"])
		assert.Equal(t, "alice", gotNode.Props["name"])
	})

	t.Run("dbtype.Relationship Props are coerced in place", func(t *testing.T) {
		rel := dbtype.Relationship{
			Id:    2,
			Type:  "KNOWS",
			Props: map[string]any{"since": date},
		}
		got := coerceDriverValue(rel)
		gotRel, ok := got.(dbtype.Relationship)
		require.True(t, ok, "expected dbtype.Relationship, got %T", got)
		assert.Equal(t, "2026-05-25", gotRel.Props["since"])
	})

	t.Run("dbtype.Path coerces Props of Nodes and Relationships", func(t *testing.T) {
		path := dbtype.Path{
			Nodes: []dbtype.Node{
				{Id: 1, Props: map[string]any{"birthday": date}},
				{Id: 2, Props: map[string]any{"birthday": date}},
			},
			Relationships: []dbtype.Relationship{
				{Id: 3, Type: "KNOWS", Props: map[string]any{"since": date}},
			},
		}
		got := coerceDriverValue(path)
		gotPath, ok := got.(dbtype.Path)
		require.True(t, ok, "expected dbtype.Path, got %T", got)
		assert.Equal(t, "2026-05-25", gotPath.Nodes[0].Props["birthday"])
		assert.Equal(t, "2026-05-25", gotPath.Nodes[1].Props["birthday"])
		assert.Equal(t, "2026-05-25", gotPath.Relationships[0].Props["since"])
	})
}
