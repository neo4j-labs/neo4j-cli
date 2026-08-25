// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
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

// TestRunStatementWithMode_CarriesPlanAndProfile proves the single-statement
// snapshot step: the seam's Plan/Profile flow into queryResult under the
// EXPLAIN/PROFILE XOR rule (profile wins when both-should-never-be-set, plan
// fills the absence), and an ordinary run carries neither.
func TestRunStatementWithMode_CarriesPlanAndProfile(t *testing.T) {
	t.Run("EXPLAIN response carries plan, no profile", func(t *testing.T) {
		withRunStatementSeam(t, func(_ context.Context, _ *conn, _ string, _ map[string]any, _ bool) (*queryResponse, error) {
			resp := &queryResponse{}
			resp.Plan = fakePlan{operator: "ProduceResults"}
			return resp, nil
		})

		c := &conn{Conn: dbconn.Conn{Database: "neo4j"}}
		res, err := runStatement(context.Background(), c, "EXPLAIN RETURN 1", nil)
		require.NoError(t, err)
		require.NotNil(t, res.Plan)
		assert.Equal(t, "ProduceResults", res.Plan.Operator)
		assert.Nil(t, res.Profile)
	})

	t.Run("PROFILE response carries profile, no plan", func(t *testing.T) {
		withRunStatementSeam(t, func(_ context.Context, _ *conn, _ string, _ map[string]any, _ bool) (*queryResponse, error) {
			resp := &queryResponse{}
			resp.Profile = fakeProfiledPlan{operator: "ProduceResults", records: 7, dbHits: 3}
			return resp, nil
		})

		c := &conn{Conn: dbconn.Conn{Database: "neo4j"}}
		res, err := runStatement(context.Background(), c, "PROFILE RETURN 1", nil)
		require.NoError(t, err)
		assert.Nil(t, res.Plan)
		require.NotNil(t, res.Profile)
		assert.Equal(t, "ProduceResults", res.Profile.Operator)
		assert.Equal(t, int64(7), res.Profile.Rows)
		assert.Equal(t, int64(3), res.Profile.DbHits)
	})

	t.Run("ordinary run carries neither", func(t *testing.T) {
		withRunStatementSeam(t, func(_ context.Context, _ *conn, _ string, _ map[string]any, _ bool) (*queryResponse, error) {
			return &queryResponse{}, nil
		})

		c := &conn{Conn: dbconn.Conn{Database: "neo4j"}}
		res, err := runStatement(context.Background(), c, "RETURN 1", nil)
		require.NoError(t, err)
		assert.Nil(t, res.Plan)
		assert.Nil(t, res.Profile)
	})
}

// TestRunStatementsWithMode_CarriesPerStatementPlan proves the --atomic batch
// path applies the snapshot rule per statement: each queryResult carries its own
// plan or profile, never the first statement's leaked across the batch. The seam
// keys each response off the statement text so the ordinary run reports neither.
func TestRunStatementsWithMode_CarriesPerStatementPlan(t *testing.T) {
	withRunStatementsSeam(t, func(_ context.Context, _ *conn, statements []string, _ map[string]any, _ bool) ([]*queryResponse, error) {
		resps := make([]*queryResponse, len(statements))
		for i, statement := range statements {
			resp := &queryResponse{}
			switch {
			case strings.HasPrefix(statement, "EXPLAIN"):
				resp.Plan = fakePlan{operator: fmt.Sprintf("explain-%d", i)}
			case strings.HasPrefix(statement, "PROFILE"):
				resp.Profile = fakeProfiledPlan{operator: fmt.Sprintf("profile-%d", i), records: int64(i)}
			}
			resps[i] = resp
		}
		return resps, nil
	})

	c := &conn{Conn: dbconn.Conn{Database: "neo4j"}}
	results, err := runStatementsWithMode(context.Background(), c, []string{"EXPLAIN RETURN 1", "PROFILE RETURN 2", "RETURN 3"}, nil, false)
	require.NoError(t, err)
	require.Len(t, results, 3)

	require.NotNil(t, results[0].Plan)
	assert.Equal(t, "explain-0", results[0].Plan.Operator)
	assert.Nil(t, results[0].Profile)

	require.NotNil(t, results[1].Profile)
	assert.Equal(t, "profile-1", results[1].Profile.Operator)
	assert.Equal(t, int64(1), results[1].Profile.Rows)
	assert.Nil(t, results[1].Plan)

	assert.Nil(t, results[2].Plan)
	assert.Nil(t, results[2].Profile)
}

// TestTruncateResult_CarriesPlanAndProfile proves the copy step: truncateResult
// forwards the queryResult's plan/profile onto the renderResult unchanged under
// the same XOR rule, so the downstream renderer can emit them.
func TestTruncateResult_CarriesPlanAndProfile(t *testing.T) {
	cmd := &cobra.Command{SilenceUsage: true}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	res := &queryResult{
		Columns: []string{"n"},
		Rows:    [][]any{{int64(1)}},
		Plan:    planNodeFromPlan(fakePlan{operator: "ProduceResults"}),
	}

	out := truncateResult(cmd, res, 0, 0, false, 1)
	require.NotNil(t, out.plan)
	assert.Equal(t, "ProduceResults", out.plan.Operator)
	assert.Nil(t, out.profile)

	res.Plan = nil
	res.Profile = planNodeFromProfile(fakeProfiledPlan{operator: "ProduceResults", records: 5})
	out = truncateResult(cmd, res, 0, 0, false, 1)
	assert.Nil(t, out.plan)
	require.NotNil(t, out.profile)
	assert.Equal(t, "ProduceResults", out.profile.Operator)
	assert.Equal(t, int64(5), out.profile.Rows)

	res.Plan = nil
	res.Profile = nil
	out = truncateResult(cmd, res, 0, 0, false, 1)
	assert.Nil(t, out.plan)
	assert.Nil(t, out.profile)
}
