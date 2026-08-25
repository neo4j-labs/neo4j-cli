// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonoutput "github.com/neo4j/cli/common/output"
)

// planFixture returns a 3-node EXPLAIN-shaped tree: a root with identifiers, a
// middle node with identifiers, and a leaf with none. It is shared by the JSON,
// table, and formatter tests so a structural mistake surfaces everywhere.
func planFixture() *planNode {
	return &planNode{
		Operator:    "ProduceResults",
		Identifiers: []string{"x"},
		Children: []planNode{
			{
				Operator:    "Filter",
				Identifiers: []string{"n"},
				Children: []planNode{
					{Operator: "NodeByLabelScan"},
				},
			},
		},
	}
}

// profiledFixture is the PROFILE twin of planFixture, with metrics on every node,
// so tests can pin the exact " (rows:, dbHits:, time:µs)" suffix each line.
func profiledFixture() *planNode {
	return &planNode{
		Operator:    "ProduceResults",
		Identifiers: []string{"x"},
		Rows:        1,
		DbHits:      2,
		Time:        3,
		Children: []planNode{
			{
				Operator:    "Filter",
				Identifiers: []string{"n"},
				Rows:        10,
				DbHits:      20,
				Time:        30,
				Children: []planNode{
					{Operator: "NodeByLabelScan", Rows: 100, DbHits: 200, Time: 300},
				},
			},
		},
	}
}

func TestFormatPlanTree_Explain(t *testing.T) {
	got := formatPlanTree(planFixture(), false, 0)
	want := []string{
		"ProduceResults => x",
		"  Filter => n",
		"    NodeByLabelScan",
	}
	assert.Equal(t, want, got)
}

func TestFormatPlanTree_Profiled(t *testing.T) {
	got := formatPlanTree(profiledFixture(), true, 0)
	want := []string{
		"ProduceResults => x (rows: 1, dbHits: 2, time: 3µs)",
		"  Filter => n (rows: 10, dbHits: 20, time: 30µs)",
		"    NodeByLabelScan (rows: 100, dbHits: 200, time: 300µs)",
	}
	assert.Equal(t, want, got)
}

func TestFormatPlanTree_MetricsSuffixFollowsProfiledFlagNotValues(t *testing.T) {
	// A profiled node with every metric zero must still print its metrics; an
	// EXPLAIN node with non-zero Rows/DbHits (impossible in practice, but the
	// tree type carries the fields) must still print none. The profiled flag,
	// not the values, decides the suffix.
	t.Run("zero-cost profiled node keeps the suffix", func(t *testing.T) {
		got := formatPlanTree(&planNode{Operator: "ProduceResults"}, true, 0)
		assert.Equal(t, []string{"ProduceResults (rows: 0, dbHits: 0, time: 0µs)"}, got)
	})

	t.Run("unprofiled node never adds the suffix", func(t *testing.T) {
		got := formatPlanTree(&planNode{Operator: "ProduceResults", Rows: 7, DbHits: 8, Time: 9}, false, 0)
		assert.Equal(t, []string{"ProduceResults"}, got)
	})
}

func TestFormatPlanTree_HonorsStartingDepth(t *testing.T) {
	tree := &planNode{Operator: "Root", Children: []planNode{{Operator: "Leaf"}}}
	got := formatPlanTree(tree, false, 2)
	want := []string{
		"    Root",
		"      Leaf",
	}
	assert.Equal(t, want, got)
}

func TestFormatPlanTree_NoDepthCap(t *testing.T) {
	// A six-level chain must be fully rendered — depth is capped only by the
	// pipeline's own operator count, never by the formatter.
	root := &planNode{Operator: "L0"}
	cur := root
	for i := 1; i <= 5; i++ {
		cur.Children = []planNode{{Operator: "L" + string(rune('0'+i))}}
		cur = &cur.Children[0]
	}
	got := formatPlanTree(root, false, 0)
	want := []string{
		"L0",
		"  L1",
		"    L2",
		"      L3",
		"        L4",
		"          L5",
	}
	assert.Equal(t, want, got)
}

func TestFormatPlanTree_Nil(t *testing.T) {
	assert.Nil(t, formatPlanTree(nil, false, 0))
}

// TestFormatPlanTree_StripsControlBytes locks the terminal-safety invariant the
// rest of the query output already honors (see formatCell): operator and
// identifier text is server-supplied, and a backtick-quoted Cypher identifier
// can smuggle an ANSI escape, so neither may reach the terminal raw.
func TestFormatPlanTree_StripsControlBytes(t *testing.T) {
	got := formatPlanTree(&planNode{
		Operator:    "Produce\x1b[31mResults",
		Identifiers: []string{"n\x1b[0m", "m\x07"},
		Children: []planNode{{
			Operator: "NodeByLabelScan\x00",
		}},
	}, false, 0)

	// StripControl neutralizes each control byte to '?' rather than deleting it,
	// so the assertion is that no raw control byte survives to reach the
	// terminal — the inert "[31m" remainder is harmless without its ESC.
	require.Len(t, got, 2)
	for _, line := range got {
		assert.NotContains(t, line, "\x1b", "ANSI escape reached the rendered line")
		assert.NotContains(t, line, "\x00")
		assert.NotContains(t, line, "\x07")
	}
	assert.Equal(t, "Produce?[31mResults => n?[0m, m?", got[0])
	assert.Equal(t, "  NodeByLabelScan?", got[1])
}

func TestRenderResults_JSON_CarriesExplainPlan(t *testing.T) {
	cmd, cfg, stdout := newRenderCmd(t, "json")
	renderResults(cmd, cfg, []renderResult{{
		columns:         []string{"n"},
		rows:            []map[string]any{{"n": float64(1)}},
		truncated:       true,
		arraysTruncated: 0,
		plan:            planFixture(),
	}})

	out := stdout.String()
	var got decodedResult
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.NotNil(t, got.Plan)
	assert.Equal(t, "ProduceResults", got.Plan.Operator)
	require.Len(t, got.Plan.Children, 1)
	assert.Equal(t, "Filter", got.Plan.Children[0].Operator)
	require.Len(t, got.Plan.Children[0].Children, 1)
	assert.Equal(t, "NodeByLabelScan", got.Plan.Children[0].Children[0].Operator)
	assert.Nil(t, got.Profile)
	// The rest of the envelope is unchanged.
	assert.Equal(t, []string{"n"}, got.Columns)
	assert.Equal(t, []map[string]any{{"n": float64(1)}}, got.Rows)
	assert.True(t, got.Truncated)
	assert.Equal(t, 0, got.ArraysTruncated)
	assert.NotContains(t, out, `"profile"`)
}

func TestRenderResults_JSON_CarriesProfile(t *testing.T) {
	cmd, cfg, stdout := newRenderCmd(t, "json")
	profile := &planNode{
		Operator:    "ProduceResults",
		Identifiers: []string{"x"},
		Rows:        5,
		DbHits:      7,
		Time:        11,
		Children: []planNode{
			{Operator: "NodeByLabelScan", Rows: 6, DbHits: 13, Time: 17},
		},
	}
	renderResults(cmd, cfg, []renderResult{{
		columns: []string{"n"},
		rows:    []map[string]any{{"n": float64(1)}},
		profile: profile,
	}})

	out := stdout.String()
	var got decodedResult
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.NotNil(t, got.Profile)
	assert.Equal(t, "ProduceResults", got.Profile.Operator)
	assert.Equal(t, int64(7), got.Profile.DbHits)
	assert.Equal(t, int64(5), got.Profile.Rows)
	assert.Len(t, got.Rows, 1, "result rows must still be present alongside the profile")
	assert.Nil(t, got.Plan)
	assert.NotContains(t, out, `"plan"`)
}

func TestRenderResults_Table_ExplainPrintsIndentedTree(t *testing.T) {
	cmd, cfg, stdout := newRenderCmd(t, "table")
	renderResults(cmd, cfg, []renderResult{{plan: planFixture()}})

	out := stdout.String()
	assert.Contains(t, out, "ProduceResults => x")
	assert.Contains(t, out, "  Filter => n")
	assert.Contains(t, out, "    NodeByLabelScan")
	// An EXPLAIN tree must not print per-operator metrics.
	assert.NotContains(t, out, "rows: ")
	assert.NotContains(t, out, "dbHits: ")
	assert.NotContains(t, out, "time: ")
}

func TestRenderResults_Table_ProfileAppendsMetricsAfterRowsTable(t *testing.T) {
	cmd, cfg, stdout := newRenderCmd(t, "table")
	renderResults(cmd, cfg, []renderResult{{
		columns: []string{"col"},
		rows:    []map[string]any{{"col": "value"}},
		profile: profiledFixture(),
	}})

	out := stdout.String()
	idxRows := strings.Index(out, "value")
	idxTree := strings.Index(out, "ProduceResults => x (rows: 1, dbHits: 2, time: 3µs)")
	require.True(t, idxRows >= 0 && idxTree >= 0, "both the rows table and the tree must appear: %s", out)
	assert.Less(t, idxRows, idxTree, "the rows table must precede the plan tree")
	assert.Contains(t, out, "rows: 100, dbHits: 200, time: 300µs", "child operators print their metrics too")
}

func TestRenderResults_Table_MultiStatementPrefixesRootLine(t *testing.T) {
	cmd, cfg, stdout := newRenderCmd(t, "table")
	renderResults(cmd, cfg, []renderResult{
		{
			columns: []string{"a"},
			rows:    []map[string]any{{"a": "one"}},
			plan:    &planNode{Operator: "P1", Children: []planNode{{Operator: "L1"}}},
		},
		{
			columns: []string{"b"},
			rows:    []map[string]any{{"b": "two"}},
			profile: &planNode{Operator: "P2", Rows: 1, DbHits: 2, Time: 3, Children: []planNode{{Operator: "L2", Rows: 4, DbHits: 5, Time: 6}}},
		},
	})

	out := stdout.String()
	assert.Contains(t, out, "statement 1: P1")
	assert.Contains(t, out, "  L1")
	assert.Contains(t, out, "statement 2: P2 (rows: 1, dbHits: 2, time: 3µs)")
	assert.Contains(t, out, "  L2 (rows: 4, dbHits: 5, time: 6µs)")
	// Only the root line of each tree carries the statement prefix.
	assert.NotContains(t, out, "statement 1:   L1")
	assert.NotContains(t, out, "statement 2:   L2")
	assert.Less(t, strings.Index(out, "statement 1: P1"), strings.Index(out, "statement 2: P2"))
}

func TestRenderResults_OrdinaryReadHasNoPlanSideChannels(t *testing.T) {
	t.Run("json has neither a plan nor a profile key", func(t *testing.T) {
		cmd, cfg, stdout := newRenderCmd(t, "json")
		renderResults(cmd, cfg, []renderResult{{
			columns: []string{"n"},
			rows:    []map[string]any{{"n": float64(1)}},
		}})

		out := stdout.String()
		var got decodedResult
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		assert.Nil(t, got.Plan)
		assert.Nil(t, got.Profile)
		assert.NotContains(t, out, `"plan"`)
		assert.NotContains(t, out, `"profile"`)
	})

	t.Run("single table emits no line beyond the existing table", func(t *testing.T) {
		cmd, cfg, stdout := newRenderCmd(t, "table")
		result := renderResult{columns: []string{"n"}, rows: []map[string]any{{"n": float64(1)}}}
		renderResults(cmd, cfg, []renderResult{result})

		cmdB, cfgB, expectedBuf := newRenderCmd(t, "table")
		commonoutput.PrintBodyMap(cmdB, cfgB, result, result.columns)

		assert.Equal(t, expectedBuf.String(), stdout.String())
	})

	t.Run("multi table emits no extra lines", func(t *testing.T) {
		cmd, cfg, stdout := newRenderCmd(t, "table")
		results := []renderResult{
			{columns: []string{"a"}, rows: []map[string]any{{"a": "one"}}},
			{columns: []string{"b"}, rows: []map[string]any{{"b": "two"}}},
		}
		renderResults(cmd, cfg, results)

		cmdB, cfgB, expectedBuf := newRenderCmd(t, "table")
		items := make([]commonoutput.ResponseData, len(results))
		fields := make([][]string, len(results))
		for i, r := range results {
			items[i] = r
			fields[i] = r.columns
		}
		commonoutput.PrintBodyMaps(cmdB, cfgB, items, fields)

		assert.Equal(t, expectedBuf.String(), stdout.String())
	})
}
