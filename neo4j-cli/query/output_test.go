// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
)

// newRenderCmd returns a fresh cobra command with stdout captured into the
// returned buffer. The output mode ("default", "json", or "table") is wired
// through the persisted aura config so renderRows reads it via cfg.Aura.Output().
func newRenderCmd(t *testing.T, output string) (*cobra.Command, *clicfg.Config, *bytes.Buffer) {
	t.Helper()
	cfgJSON := `{"aura":{"output":"` + output + `"}}`
	fs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test")

	cmd := &cobra.Command{}
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	return cmd, cfg, stdout
}

func TestRowsFromValues(t *testing.T) {
	tests := []struct {
		name    string
		columns []string
		values  [][]any
		want    []map[string]any
	}{
		{
			name:    "empty values produces empty slice",
			columns: []string{"n", "m"},
			values:  [][]any{},
			want:    []map[string]any{},
		},
		{
			name:    "preserves column order in mapping",
			columns: []string{"a", "b", "c"},
			values: [][]any{
				{float64(1), "two", true},
				{float64(10), "twenty", false},
			},
			want: []map[string]any{
				{"a": float64(1), "b": "two", "c": true},
				{"a": float64(10), "b": "twenty", "c": false},
			},
		},
		{
			name:    "missing positional value becomes nil",
			columns: []string{"a", "b"},
			values:  [][]any{{float64(1)}},
			want:    []map[string]any{{"a": float64(1), "b": nil}},
		},
		{
			name:    "extra positional value is dropped",
			columns: []string{"a"},
			values:  [][]any{{float64(1), float64(99)}},
			want:    []map[string]any{{"a": float64(1)}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := rowsFromValues(tc.columns, tc.values)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRenderRows_JSON(t *testing.T) {
	tests := []struct {
		name            string
		columns         []string
		rows            []map[string]any
		truncated       bool
		arraysTruncated int
		// expected JSON values (decoded to compare structurally, not byte-equal).
		wantTruncated       bool
		wantArraysTruncated int
		wantRowCount        int
	}{
		{
			name:                "happy path with two rows",
			columns:             []string{"n", "m"},
			rows:                []map[string]any{{"n": float64(1), "m": "alice"}, {"n": float64(2), "m": "bob"}},
			truncated:           false,
			arraysTruncated:     0,
			wantTruncated:       false,
			wantArraysTruncated: 0,
			wantRowCount:        2,
		},
		{
			name:                "truncated propagated to JSON",
			columns:             []string{"n"},
			rows:                []map[string]any{{"n": float64(1)}},
			truncated:           true,
			arraysTruncated:     0,
			wantTruncated:       true,
			wantArraysTruncated: 0,
			wantRowCount:        1,
		},
		{
			name:                "arrays_truncated propagated to JSON",
			columns:             []string{"xs"},
			rows:                []map[string]any{{"xs": []any{}}},
			truncated:           false,
			arraysTruncated:     3,
			wantTruncated:       false,
			wantArraysTruncated: 3,
			wantRowCount:        1,
		},
		{
			name:                "empty rows still emits valid JSON envelope",
			columns:             []string{"x"},
			rows:                nil,
			truncated:           false,
			arraysTruncated:     0,
			wantTruncated:       false,
			wantArraysTruncated: 0,
			wantRowCount:        0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, cfg, stdout := newRenderCmd(t, "json")
			renderRows(cmd, cfg, tc.columns, tc.rows, tc.truncated, tc.arraysTruncated)

			var got jsonRowsResult
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
			assert.Equal(t, tc.columns, got.Columns)
			assert.Equal(t, tc.wantTruncated, got.Truncated)
			assert.Equal(t, tc.wantArraysTruncated, got.ArraysTruncated)
			assert.Len(t, got.Rows, tc.wantRowCount)
		})
	}
}

func TestRenderRows_JSON_PreservesColumnOrder(t *testing.T) {
	cmd, cfg, stdout := newRenderCmd(t, "json")
	renderRows(cmd, cfg, []string{"z", "a", "m"}, []map[string]any{
		{"z": float64(1), "a": "first", "m": true},
	}, false, 0)

	// Asserts the JSON envelope's column array order, not just membership.
	var raw struct {
		Columns []string `json:"columns"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &raw))
	assert.Equal(t, []string{"z", "a", "m"}, raw.Columns)
}

func TestRenderRows_Table(t *testing.T) {
	tests := []struct {
		name        string
		columns     []string
		rows        []map[string]any
		wantHeaders []string // expected header column substrings (lower-cased compare)
		wantInBody  []string // substrings expected in the rendered body
	}{
		{
			name:        "scalar string + number + bool",
			columns:     []string{"name", "age", "active"},
			rows:        []map[string]any{{"name": "alice", "age": float64(30), "active": true}},
			wantHeaders: []string{"name", "age", "active"},
			wantInBody:  []string{"alice", "30", "true"},
		},
		{
			name:        "nested object renders as JSON-stringified cell",
			columns:     []string{"props"},
			rows:        []map[string]any{{"props": map[string]any{"k": "v"}}},
			wantHeaders: []string{"props"},
			wantInBody:  []string{`"k":"v"`},
		},
		{
			name:        "array renders as JSON-stringified cell",
			columns:     []string{"items"},
			rows:        []map[string]any{{"items": []any{float64(1), float64(2), float64(3)}}},
			wantHeaders: []string{"items"},
			wantInBody:  []string{"[1,2,3]"},
		},
		{
			name:        "nil renders as null literal",
			columns:     []string{"n"},
			rows:        []map[string]any{{"n": nil}},
			wantHeaders: []string{"n"},
			wantInBody:  []string{"null"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, cfg, stdout := newRenderCmd(t, "table")
			renderRows(cmd, cfg, tc.columns, tc.rows, false, 0)

			out := stdout.String()
			lower := strings.ToLower(out)
			for _, h := range tc.wantHeaders {
				assert.Contains(t, lower, strings.ToLower(h), "missing header %q in table output", h)
			}
			for _, body := range tc.wantInBody {
				assert.Contains(t, out, body, "missing body cell text %q in table output", body)
			}
		})
	}
}

func TestRenderRows_Table_PreservesColumnOrder(t *testing.T) {
	cmd, cfg, stdout := newRenderCmd(t, "table")
	renderRows(cmd, cfg, []string{"z", "a", "m"}, []map[string]any{
		{"z": "first-col", "a": "second-col", "m": "third-col"},
	}, false, 0)

	out := stdout.String()
	lower := strings.ToLower(out)
	idxZ := strings.Index(lower, "z")
	idxA := strings.Index(lower, "a")
	idxM := strings.Index(lower, "m")
	require.True(t, idxZ >= 0 && idxA >= 0 && idxM >= 0, "all headers must appear in output: %s", out)
	assert.Less(t, idxZ, idxA, "z must precede a in table header (declared order)")
	assert.Less(t, idxA, idxM, "a must precede m in table header (declared order)")

	// Body also follows column order.
	idxFirst := strings.Index(out, "first-col")
	idxSecond := strings.Index(out, "second-col")
	idxThird := strings.Index(out, "third-col")
	require.True(t, idxFirst >= 0 && idxSecond >= 0 && idxThird >= 0, "all cells must appear")
	assert.Less(t, idxFirst, idxSecond)
	assert.Less(t, idxSecond, idxThird)
}

func TestRenderRows_DefaultOutputRendersTable(t *testing.T) {
	// "default" must dispatch to the table renderer (not JSON) when stdout is
	// a TTY — TestMain seeds stdoutIsTerminal=true so this is the default for
	// the package-level test run.
	cmd, cfg, stdout := newRenderCmd(t, "default")
	renderRows(cmd, cfg, []string{"n"}, []map[string]any{{"n": float64(42)}}, false, 0)

	out := stdout.String()
	assert.Contains(t, out, "42")
	// Table rendering should not produce a JSON envelope.
	assert.NotContains(t, out, `"columns"`)
	assert.NotContains(t, out, `"truncated"`)
}

// withStdoutIsTerminal locally overrides the package-level seam for one test,
// auto-restoring the prior value via t.Cleanup. The package's TestMain seeds
// the seam to true; tests that want to exercise the non-TTY branch (or
// re-assert TTY explicitly) call this helper.
func withStdoutIsTerminal(t *testing.T, isTTY bool) {
	t.Helper()
	prev := stdoutIsTerminal
	stdoutIsTerminal = func(io.Writer) bool { return isTTY }
	t.Cleanup(func() { stdoutIsTerminal = prev })
}

// TestRenderRows_TTYAwareDefault covers the four explicit/auto branches of
// resolveOutput as exercised through renderRows: TTY+default→table,
// non-TTY+default→json, non-TTY+--output table→table, TTY+--output json→json.
func TestRenderRows_TTYAwareDefault(t *testing.T) {
	tests := []struct {
		name        string
		output      string // value persisted in cfg.Aura.Output()
		isTTY       bool
		wantJSON    bool   // true if the JSON envelope should be present
		wantInTable string // substring expected in table output (only when wantJSON=false)
	}{
		{
			name:        "TTY + default -> table",
			output:      "default",
			isTTY:       true,
			wantJSON:    false,
			wantInTable: "42",
		},
		{
			name:     "non-TTY + default -> json",
			output:   "default",
			isTTY:    false,
			wantJSON: true,
		},
		{
			name:        "non-TTY + explicit --output table -> table",
			output:      "table",
			isTTY:       false,
			wantJSON:    false,
			wantInTable: "42",
		},
		{
			name:     "TTY + explicit --output json -> json",
			output:   "json",
			isTTY:    true,
			wantJSON: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withStdoutIsTerminal(t, tc.isTTY)
			cmd, cfg, stdout := newRenderCmd(t, tc.output)
			renderRows(cmd, cfg, []string{"n"}, []map[string]any{{"n": float64(42)}}, false, 0)

			out := stdout.String()
			if tc.wantJSON {
				var got jsonRowsResult
				require.NoError(t, json.Unmarshal([]byte(out), &got),
					"output should be JSON envelope, got: %s", out)
				assert.Equal(t, []string{"n"}, got.Columns)
				assert.Len(t, got.Rows, 1)
			} else {
				assert.Contains(t, out, tc.wantInTable)
				assert.NotContains(t, out, `"columns"`,
					"table output should not contain JSON envelope keys")
				assert.NotContains(t, out, `"truncated"`,
					"table output should not contain JSON envelope keys")
			}
		})
	}
}
