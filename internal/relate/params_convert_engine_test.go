// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build cgo && relate

// Real-engine round trip of every special type: a driver-side value goes in as
// a typed parameter, is stored by Materialize's statements, and comes back as
// the same value. Needs a Go client that binds the typed kinds through the
// engine's typed putters (relatedb/relate#901, REL8-929); with an older
// client the values bind as maps and these fail.

package relate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
)

func TestEngineTypedParams_RoundTrip(t *testing.T) {
	s := openTestStore(t)
	stockholm, err := time.LoadLocation("Europe/Stockholm")
	require.NoError(t, err)

	props := map[string]any{
		"d":   dbtype.Date(time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)),
		"lt":  dbtype.LocalTime(time.Date(2000, 1, 1, 10, 30, 15, 0, time.UTC)),
		"ldt": dbtype.LocalDateTime(time.Date(2020, 6, 1, 10, 30, 15, 0, time.UTC)),
		"dt":  time.Date(2020, 6, 1, 10, 0, 0, 0, stockholm),
		"du":  dbtype.Duration{Months: 1, Days: 2, Seconds: 3, Nanos: 4},
		"p2":  dbtype.Point2D{SpatialRefId: 7203, X: 1, Y: 2},
		"p3":  dbtype.Point3D{SpatialRefId: 4979, X: 1, Y: 2, Z: 3},
		"v":   dbtype.Vector[float32]{Elems: []float32{0.25, 0.5}},
		"l":   []any{dbtype.Date(time.Date(2021, 1, 2, 0, 0, 0, 0, time.UTC)), dbtype.Date(time.Date(2022, 3, 4, 0, 0, 0, 0, time.UTC))}, // property lists must be homogeneous
	}
	_, err = s.Run([]string{"UNWIND $rows AS row CREATE (n:T) SET n = row.props"},
		map[string]any{"rows": []any{map[string]any{"props": props}}}, false)
	require.NoError(t, err)

	res, err := s.Run([]string{"MATCH (n:T) RETURN n.d, n.lt, n.ldt, n.dt, n.du, n.p2, n.p3, n.v, n.l"}, nil, false)
	require.NoError(t, err)
	require.Len(t, res[0].Rows, 1)
	row := res[0].Rows[0]

	assert.Equal(t, props["d"], row[0], "date")
	assert.Equal(t, props["lt"], row[1], "local time")
	assert.Equal(t, props["ldt"], row[2], "local datetime")
	got, ok := row[3].(time.Time)
	require.True(t, ok, "datetime comes back as time.Time")
	assert.True(t, got.Equal(props["dt"].(time.Time)), "datetime instant survives the zone binding")
	assert.Equal(t, props["du"], row[4], "duration")
	assert.Equal(t, props["p2"], row[5], "2d point")
	assert.Equal(t, props["p3"], row[6], "3d point")
	assert.Equal(t, dbtype.Vector[float64]{Elems: []float64{0.25, 0.5}}, row[7], "vector (read side widens to float64)")
	assert.Equal(t, props["l"], row[8], "temporal nested in a list")
}

// Vector (and point/temporal) properties must survive a real pull: this
// aborted with "cannot render ... as a Cypher literal" before typed binding.
func TestEngineMaterialize_PullsVectorPointAndTemporalProps(t *testing.T) {
	s := openTestStore(t)
	g := NewSubgraph()
	g.Nodes["4:a:0"] = &SubgraphNode{ElementID: "4:a:0", Labels: []string{"Doc"}, Props: map[string]any{
		"emb":  dbtype.Vector[float32]{Elems: []float32{0.25, 0.5, 0.75}},
		"loc":  dbtype.Point2D{SpatialRefId: 7203, X: 1, Y: 2},
		"when": dbtype.Date(time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)),
	}}
	g.Nodes["4:a:1"] = &SubgraphNode{ElementID: "4:a:1", Labels: []string{"Doc"}, Props: map[string]any{"emb": dbtype.Vector[float32]{Elems: []float32{1, 2, 3}}}}
	g.Rels["5:a:0"] = &SubgraphRel{ElementID: "5:a:0", StartElementID: "4:a:0", EndElementID: "4:a:1", Type: "LINKS",
		Props: map[string]any{"at": dbtype.Duration{Days: 3}}}

	m := &IdentityMap{Sources: map[string]SourceIdentities{}}
	stats, err := Materialize(s, g, m, "src", 100)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.NodesCreated)
	assert.Equal(t, 1, stats.RelsCreated)

	res, err := s.Run([]string{"MATCH (d:Doc)-[r:LINKS]->() RETURN d.emb, d.loc, d.when, r.at"}, nil, false)
	require.NoError(t, err)
	require.Len(t, res[0].Rows, 1)
	row := res[0].Rows[0]
	assert.Equal(t, dbtype.Vector[float64]{Elems: []float64{0.25, 0.5, 0.75}}, row[0])
	assert.Equal(t, dbtype.Point2D{SpatialRefId: 7203, X: 1, Y: 2}, row[1])
	assert.Equal(t, dbtype.Date(time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)), row[2])
	assert.Equal(t, dbtype.Duration{Days: 3}, row[3])
}
