// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"encoding/json"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePlan is a driver-free neo4j.Plan stub. Children holds concrete fakePlan
// values boxed into the driver interface so tests can build arbitrary-shaped
// plan trees without a real driver.
type fakePlan struct {
	operator    string
	arguments   map[string]any
	identifiers []string
	children    []neo4j.Plan
}

func (p fakePlan) Operator() string          { return p.operator }
func (p fakePlan) Arguments() map[string]any { return p.arguments }
func (p fakePlan) Identifiers() []string     { return p.identifiers }
func (p fakePlan) Children() []neo4j.Plan    { return p.children }

// compile-time check that the stub satisfies the driver interface.
var _ neo4j.Plan = fakePlan{}

// fakeProfiledPlan is a driver-free neo4j.ProfiledPlan stub: the profile-only
// metrics are plain fields the converter copies field-for-field.
type fakeProfiledPlan struct {
	operator        string
	arguments       map[string]any
	identifiers     []string
	children        []neo4j.ProfiledPlan
	dbHits          int64
	records         int64
	time            int64
	pageCacheHits   int64
	pageCacheMisses int64
}

func (p fakeProfiledPlan) Operator() string               { return p.operator }
func (p fakeProfiledPlan) Arguments() map[string]any      { return p.arguments }
func (p fakeProfiledPlan) Identifiers() []string          { return p.identifiers }
func (p fakeProfiledPlan) Children() []neo4j.ProfiledPlan { return p.children }
func (p fakeProfiledPlan) DbHits() int64                  { return p.dbHits }
func (p fakeProfiledPlan) Records() int64                 { return p.records }
func (p fakeProfiledPlan) Time() int64                    { return p.time }
func (p fakeProfiledPlan) PageCacheHits() int64           { return p.pageCacheHits }
func (p fakeProfiledPlan) PageCacheMisses() int64         { return p.pageCacheMisses }
func (p fakeProfiledPlan) PageCacheHitRatio() float64     { return 0 }

// compile-time check that the stub satisfies the driver interface.
var _ neo4j.ProfiledPlan = fakeProfiledPlan{}

func TestPlanNodeFromPlan(t *testing.T) {
	t.Run("nil plan yields nil", func(t *testing.T) {
		var p neo4j.Plan
		assert.Nil(t, planNodeFromPlan(p))
	})

	t.Run("leaf copies operator, arguments and identifiers", func(t *testing.T) {
		got := planNodeFromPlan(fakePlan{
			operator:    "NodeByLabelScan",
			arguments:   map[string]any{"LabelName": "Person"},
			identifiers: []string{"n"},
		})
		require.NotNil(t, got)
		assert.Equal(t, "NodeByLabelScan", got.Operator)
		assert.Equal(t, map[string]any{"LabelName": "Person"}, got.Arguments)
		assert.Equal(t, []string{"n"}, got.Identifiers)
	})

	t.Run("profile-only metrics stay zero for EXPLAIN plans", func(t *testing.T) {
		got := planNodeFromPlan(fakePlan{operator: "ProduceResults"})
		require.NotNil(t, got)
		assert.Zero(t, got.Rows)
		assert.Zero(t, got.DbHits)
		assert.Zero(t, got.Time)
		assert.Zero(t, got.PageCacheHits)
		assert.Zero(t, got.PageCacheMisses)
	})

	t.Run("recurses over children preserving depth and operators", func(t *testing.T) {
		got := planNodeFromPlan(fakePlan{
			operator: "Root",
			children: []neo4j.Plan{
				fakePlan{
					operator: "Child",
					children: []neo4j.Plan{
						fakePlan{operator: "Grandchild"},
					},
				},
			},
		})
		require.NotNil(t, got)
		assert.Equal(t, "Root", got.Operator)
		require.Len(t, got.Children, 1)
		assert.Equal(t, "Child", got.Children[0].Operator)
		require.Len(t, got.Children[0].Children, 1)
		assert.Equal(t, "Grandchild", got.Children[0].Children[0].Operator)
	})
}

func TestPlanNodeFromProfile(t *testing.T) {
	t.Run("nil profiled plan yields nil", func(t *testing.T) {
		var p neo4j.ProfiledPlan
		assert.Nil(t, planNodeFromProfile(p))
	})

	t.Run("copies operator, arguments, identifiers and profile metrics", func(t *testing.T) {
		got := planNodeFromProfile(fakeProfiledPlan{
			operator:        "NodeByLabelScan",
			arguments:       map[string]any{"LabelName": "Person"},
			identifiers:     []string{"n"},
			dbHits:          10,
			records:         3,
			time:            7,
			pageCacheHits:   5,
			pageCacheMisses: 2,
		})
		require.NotNil(t, got)
		assert.Equal(t, "NodeByLabelScan", got.Operator)
		assert.Equal(t, map[string]any{"LabelName": "Person"}, got.Arguments)
		assert.Equal(t, []string{"n"}, got.Identifiers)
		assert.Equal(t, int64(10), got.DbHits)
		assert.Equal(t, int64(3), got.Rows)
		assert.Equal(t, int64(7), got.Time)
		assert.Equal(t, int64(5), got.PageCacheHits)
		assert.Equal(t, int64(2), got.PageCacheMisses)
	})

	t.Run("recurses over children preserving depth and operators", func(t *testing.T) {
		got := planNodeFromProfile(fakeProfiledPlan{
			operator: "Root",
			dbHits:   1,
			children: []neo4j.ProfiledPlan{
				fakeProfiledPlan{
					operator: "Child",
					records:  2,
					children: []neo4j.ProfiledPlan{
						fakeProfiledPlan{operator: "Grandchild", dbHits: 3},
					},
				},
			},
		})
		require.NotNil(t, got)
		assert.Equal(t, "Root", got.Operator)
		assert.Equal(t, int64(1), got.DbHits)
		require.Len(t, got.Children, 1)
		assert.Equal(t, "Child", got.Children[0].Operator)
		assert.Equal(t, int64(2), got.Children[0].Rows)
		require.Len(t, got.Children[0].Children, 1)
		assert.Equal(t, "Grandchild", got.Children[0].Children[0].Operator)
		assert.Equal(t, int64(3), got.Children[0].Children[0].DbHits)
	})
}

func TestPlanNodeJSON(t *testing.T) {
	t.Run("only operator set marshals to operator-only object", func(t *testing.T) {
		b, err := json.Marshal(planNode{Operator: "X"})
		require.NoError(t, err)
		assert.Equal(t, `{"operator":"X"}`, string(b))
	})
}
