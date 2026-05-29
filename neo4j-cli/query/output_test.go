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
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/test/utils/testfs"
)

// decodedResult is the JSON decode target for queryResult output; it mirrors
// the MarshalJSON envelope with exported fields so json.Unmarshal works.
type decodedResult struct {
	Columns         []string         `json:"columns"`
	Rows            []map[string]any `json:"rows"`
	Truncated       bool             `json:"truncated"`
	ArraysTruncated int              `json:"arrays_truncated"`
}

// newRenderCmd returns a fresh cobra command with stdout captured into the
// returned buffer. The format mode ("default", "json", or "table") is wired
// through the persisted config so renderRows reads it via cfg.Global.Format().
func newRenderCmd(t *testing.T, output string) (*cobra.Command, *clicfg.Config, *bytes.Buffer) {
	t.Helper()
	cfgJSON := `{"format":"` + output + `"}`
	fs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.QueryScope)

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

func TestFormatCell_StripsControlOnStrings(t *testing.T) {
	// case string: ANSI escape and DEL are redacted with "?".
	assert.Equal(t, "foo?[31mbar", formatCell("foo\x1b[31mbar"))
	assert.Equal(t, "x?y", formatCell("x\x7fy"))
	// Whitespace runes survive.
	assert.Equal(t, "a\tb\nc\rd", formatCell("a\tb\nc\rd"))
}

func TestFormatCell_JSONBranchUnaffected(t *testing.T) {
	// non-string values flow through json.Marshal which already escapes
	// control bytes; formatCell must NOT apply StripControl on this branch
	// (otherwise legitimate JSON-escaped sequences like "\\u001b" would
	// be double-mutated).
	got := formatCell([]any{"a\x1bb"})
	// json.Marshal escapes \x1b as the six-byte literal "\\u001b" inside the array literal.
	assert.Contains(t, got, `\u001b`, "non-string branch must keep JSON escape; got: %s", got)
	assert.NotContains(t, got, "?", "non-string branch must not be StripControl'd")
}

func TestRenderRows_TableStripsControlInStringCell(t *testing.T) {
	cmd, cfg, stdout := newRenderCmd(t, "table")
	renderRows(cmd, cfg, []string{"col"}, []map[string]any{{"col": "foo\x1b[31mbar"}}, false, 0)
	out := stdout.String()
	assert.Contains(t, out, "foo?[31mbar")
	assert.NotContains(t, out, "\x1b[31m", "raw ANSI escape must not reach the rendered table")
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
		wantRowValues       []map[string]any // optional: decoded row equality
		wantRawContains     []string         // optional: raw output substrings
		wantRawNotContains  []string         // optional: raw output forbidden substrings
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
		{
			// temporal string passes through MarshalJSON verbatim as a JSON string.
			name:                "temporal-shaped string serialises as JSON string",
			columns:             []string{"d"},
			rows:                []map[string]any{{"d": "2026-05-25"}},
			truncated:           false,
			arraysTruncated:     0,
			wantTruncated:       false,
			wantArraysTruncated: 0,
			wantRowCount:        1,
			wantRowValues:       []map[string]any{{"d": "2026-05-25"}},
			wantRawContains:     []string{`"d": "2026-05-25"`},
			wantRawNotContains:  []string{`"d": {}`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, cfg, stdout := newRenderCmd(t, "json")
			renderRows(cmd, cfg, tc.columns, tc.rows, tc.truncated, tc.arraysTruncated)

			out := stdout.String()
			var got decodedResult
			require.NoError(t, json.Unmarshal([]byte(out), &got))
			assert.Equal(t, tc.columns, got.Columns)
			assert.Equal(t, tc.wantTruncated, got.Truncated)
			assert.Equal(t, tc.wantArraysTruncated, got.ArraysTruncated)
			assert.Len(t, got.Rows, tc.wantRowCount)
			if tc.wantRowValues != nil {
				assert.Equal(t, tc.wantRowValues, got.Rows)
			}
			for _, s := range tc.wantRawContains {
				assert.Contains(t, out, s)
			}
			for _, s := range tc.wantRawNotContains {
				assert.NotContains(t, out, s)
			}
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
		name          string
		columns       []string
		rows          []map[string]any
		wantHeaders   []string // expected header column substrings (lower-cased compare)
		wantInBody    []string // substrings expected in the rendered body
		wantNotInBody []string // substrings forbidden in the rendered body
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
		{
			// temporal string passes through formatCell verbatim (flush, no quotes, no braces).
			name:          "temporal-shaped string renders flush",
			columns:       []string{"d"},
			rows:          []map[string]any{{"d": "2026-05-25"}},
			wantHeaders:   []string{"d"},
			wantInBody:    []string{"2026-05-25"},
			wantNotInBody: []string{`"2026-05-25"`, "{}"},
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
			for _, body := range tc.wantNotInBody {
				assert.NotContains(t, out, body, "forbidden body cell text %q in table output", body)
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
	// a TTY — TestMain seeds commonoutput.StdoutIsTerminal=true so this is the
	// default for the package-level test run.
	cmd, cfg, stdout := newRenderCmd(t, "default")
	renderRows(cmd, cfg, []string{"n"}, []map[string]any{{"n": float64(42)}}, false, 0)

	out := stdout.String()
	assert.Contains(t, out, "42")
	// Table rendering should not produce a JSON envelope.
	assert.NotContains(t, out, `"columns"`)
	assert.NotContains(t, out, `"truncated"`)
}

func TestRenderResults_SingleParityWithRenderRows(t *testing.T) {
	// len==1 must produce output byte-identical to the existing single path.
	for _, format := range []string{"json", "table"} {
		t.Run(format, func(t *testing.T) {
			columns := []string{"n", "m"}
			rows := []map[string]any{{"n": float64(1), "m": "alice"}}

			cmdA, cfgA, bufA := newRenderCmd(t, format)
			renderRows(cmdA, cfgA, columns, rows, false, 0)

			cmdB, cfgB, bufB := newRenderCmd(t, format)
			renderResults(cmdB, cfgB, []renderResult{{columns: columns, rows: rows}})

			assert.Equal(t, bufA.String(), bufB.String())
		})
	}
}

func TestRenderResults_MultiJSONArray(t *testing.T) {
	cmd, cfg, stdout := newRenderCmd(t, "json")
	renderResults(cmd, cfg, []renderResult{
		{columns: []string{"a"}, rows: []map[string]any{{"a": float64(1)}}},
		{columns: []string{"b"}, rows: []map[string]any{{"b": "two"}}, truncated: true},
	})

	out := stdout.String()
	var got []decodedResult
	require.NoError(t, json.Unmarshal([]byte(out), &got), "output must be a JSON array, got: %s", out)
	require.Len(t, got, 2)
	assert.Equal(t, []string{"a"}, got[0].Columns)
	assert.Equal(t, []string{"b"}, got[1].Columns)
	assert.False(t, got[0].Truncated)
	assert.True(t, got[1].Truncated)
}

func TestRenderResults_MultiTableStacked(t *testing.T) {
	cmd, cfg, stdout := newRenderCmd(t, "table")
	renderResults(cmd, cfg, []renderResult{
		{columns: []string{"a"}, rows: []map[string]any{{"a": "first"}}},
		{columns: []string{"b"}, rows: []map[string]any{{"b": "second"}}},
	})

	out := stdout.String()
	assert.Contains(t, out, "first")
	assert.Contains(t, out, "second")
	assert.Contains(t, out, "\n\n", "stacked tables must be separated by a blank line")
	assert.Less(t, strings.Index(out, "first"), strings.Index(out, "second"))
	assert.NotContains(t, out, `"columns"`, "table output must not be a JSON envelope")
}

func TestRenderResults_MultiToon(t *testing.T) {
	cmd, cfg, stdout := newRenderCmd(t, "toon")
	renderResults(cmd, cfg, []renderResult{
		{columns: []string{"a"}, rows: []map[string]any{{"a": float64(1)}}},
		{columns: []string{"b"}, rows: []map[string]any{{"b": float64(2)}}},
	})

	out := stdout.String()
	assert.NotEmpty(t, out)
	var v any
	assert.Error(t, json.Unmarshal([]byte(out), &v), "toon output must not be valid JSON")
}

// withStdoutIsTerminal locally overrides the common/output.StdoutIsTerminal
// seam for one test, auto-restoring the prior value via t.Cleanup. TestMain
// seeds the seam to true; tests that want to exercise the non-TTY branch (or
// re-assert TTY explicitly) call this helper.
func withStdoutIsTerminal(t *testing.T, isTTY bool) {
	t.Helper()
	prev := commonoutput.StdoutIsTerminal
	commonoutput.StdoutIsTerminal = func(io.Writer) bool { return isTTY }
	t.Cleanup(func() { commonoutput.StdoutIsTerminal = prev })
}

// TestRenderRows_TTYAwareDefault covers the four explicit/auto branches of
// ResolveOutput as exercised through renderRows: TTY+default→table,
// non-TTY+default→json, non-TTY+--format table→table, TTY+--format json→json.
func TestRenderRows_TTYAwareDefault(t *testing.T) {
	tests := []struct {
		name        string
		output      string // value persisted in cfg.Global.Format()
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
			name:        "non-TTY + explicit --format table -> table",
			output:      "table",
			isTTY:       false,
			wantJSON:    false,
			wantInTable: "42",
		},
		{
			name:     "TTY + explicit --format json -> json",
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
				var got decodedResult
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
