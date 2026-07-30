// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintPassthrough_JSONVerbatim(t *testing.T) {
	// The whole point of the json branch is that nothing is reordered,
	// reindented or re-enveloped, so every case asserts the exact bytes.
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "key order and indentation preserved",
			body: `{"zzz":1,"aaa":2,   "nested":{"b":true,"a":null}}`,
			want: "{\"zzz\":1,\"aaa\":2,   \"nested\":{\"b\":true,\"a\":null}}\n",
		},
		{
			name: "data envelope not unwrapped",
			body: `{"data":[{"id":"a"}],"errors":[{"message":"partial"}]}`,
			want: "{\"data\":[{\"id\":\"a\"}],\"errors\":[{\"message\":\"partial\"}]}\n",
		},
		{
			// Only the table branch unwraps a `data` object into a row.
			name: "data object envelope not unwrapped",
			body: `{"data":{"id":"abc","name":"x"}}`,
			want: "{\"data\":{\"id\":\"abc\",\"name\":\"x\"}}\n",
		},
		{
			name: "bare array",
			body: `[1,2,3]`,
			want: "[1,2,3]\n",
		},
		{
			name: "bare scalar",
			body: `"hello"`,
			want: "\"hello\"\n",
		},
		{
			name: "null",
			body: `null`,
			want: "null\n",
		},
		{
			name: "invalid json",
			body: `<html>not json</html>`,
			want: "<html>not json</html>\n",
		},
		{
			name: "already newline terminated stays single newline",
			body: "{\"a\":1}\n",
			want: "{\"a\":1}\n",
		},
		{
			name: "empty body writes nothing",
			body: "",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, cfg, stdout := newOutputCmd(t, "json")
			assert.NotPanics(t, func() {
				PrintPassthrough(cmd, cfg, []byte(tc.body))
			})
			assert.Equal(t, tc.want, stdout.String())
		})
	}
}

func TestPrintPassthrough_JSONBigIntegerNotRounded(t *testing.T) {
	// A verbatim write cannot go through encoding/json, so an integer beyond
	// float64's exact range must survive digit-for-digit.
	cmd, cfg, stdout := newOutputCmd(t, "json")
	body := `{"id":12345678901234567890}`
	PrintPassthrough(cmd, cfg, []byte(body))
	assert.Equal(t, body+"\n", stdout.String())
}

func TestPrintPassthrough_Toon(t *testing.T) {
	cmd, cfg, stdout := newOutputCmd(t, "toon")
	PrintPassthrough(cmd, cfg, []byte(`{"data":[{"id":"a"},{"id":"b"}]}`))

	out := stdout.String()
	assert.Contains(t, out, "data")
	assert.Contains(t, out, "a")
	assert.Contains(t, out, "b")
	// TOON is not JSON, so this is not just the verbatim fallback.
	var v any
	assert.Error(t, json.Unmarshal([]byte(out), &v), "toon output must not be valid JSON, got: %s", out)
	assert.True(t, strings.HasSuffix(out, "\n"))
}

func TestPrintPassthrough_ToonKeepsDataEnvelope(t *testing.T) {
	// The table branch unwraps a `data` object into a row; toon must not.
	cmd, cfg, stdout := newOutputCmd(t, "toon")
	PrintPassthrough(cmd, cfg, []byte(`{"data":{"id":"abc"}}`))

	out := stdout.String()
	assert.Contains(t, out, "data")
	assert.Contains(t, out, "abc")
	// TOON is not JSON, so this is neither the verbatim fallback nor a table.
	var v any
	assert.Error(t, json.Unmarshal([]byte(out), &v), "toon output must not be valid JSON, got: %s", out)
	assert.NotContains(t, out, "─")
}

func TestPrintPassthrough_ToonStripsControlBytes(t *testing.T) {
	// toon.Marshal rejects C0 control bytes; stripControlDeep must run first so a
	// response carrying an ANSI escape neither panics nor reaches the terminal.
	// The bytes arrive JSON-escaped (a raw control byte is invalid JSON) and the
	// unmarshal decodes them back to raw.
	cmd, cfg, stdout := newOutputCmd(t, "toon")
	body := []byte(`{"name":"foo\u001b[31mbar","note":"ring\u0007bell"}`)

	assert.NotPanics(t, func() { PrintPassthrough(cmd, cfg, body) })

	out := stdout.String()
	assert.NotEmpty(t, out)
	assert.NotContains(t, out, "\x1b", "output must not contain a raw ESC byte")
	assert.NotContains(t, out, "\x07", "output must not contain a raw BEL byte")
	// Not the verbatim fallback: the escaped form is gone too.
	assert.NotContains(t, out, `\u001b`)
	assert.Contains(t, out, "foo?[31mbar")
}

func TestPrintPassthrough_ToonFallsBackToBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid json", body: `{"a":`},
		{name: "plain text", body: `service unavailable`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, cfg, stdout := newOutputCmd(t, "toon")
			assert.NotPanics(t, func() { PrintPassthrough(cmd, cfg, []byte(tc.body)) })
			assert.Equal(t, tc.body+"\n", stdout.String())
		})
	}
}

func TestPrintPassthrough_ToonScalarsRenderWithoutPanic(t *testing.T) {
	// A bare scalar is valid JSON, so it reaches toon.Marshal rather than the
	// fallback; either way it must not panic and must print something.
	for _, body := range []string{`"hello"`, `42`, `null`, `[1,2,3]`, `true`} {
		t.Run(body, func(t *testing.T) {
			cmd, cfg, stdout := newOutputCmd(t, "toon")
			assert.NotPanics(t, func() { PrintPassthrough(cmd, cfg, []byte(body)) })
			assert.NotEmpty(t, stdout.String())
		})
	}
}

func TestPrintPassthrough_Table(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantCells   []string
		wantColumns []string // go-pretty upper-cases header cells
	}{
		{
			name:        "data array of objects",
			body:        `{"data":[{"id":"one","name":"first"},{"id":"two","name":"second"}]}`,
			wantCells:   []string{"one", "first", "two", "second"},
			wantColumns: []string{"ID", "NAME"},
		},
		{
			name:        "data array with links envelope",
			body:        `{"data":[{"id":"one"}],"links":{"next":"tok"}}`,
			wantCells:   []string{"one"},
			wantColumns: []string{"ID"},
		},
		{
			name:        "bare array of objects",
			body:        `[{"id":"one"},{"id":"two"}]`,
			wantCells:   []string{"one", "two"},
			wantColumns: []string{"ID"},
		},
		{
			name:        "bare object is one row",
			body:        `{"id":"one","tier":"professional-db"}`,
			wantCells:   []string{"one", "professional-db"},
			wantColumns: []string{"ID", "TIER"},
		},
		{
			name:        "data object is one row of the inner object",
			body:        `{"data":{"id":"abc","name":"x"}}`,
			wantCells:   []string{"abc", "x"},
			wantColumns: []string{"ID", "NAME"},
		},
		{
			name:        "data object with a links sibling still unwraps",
			body:        `{"data":{"id":"abc"},"links":{"next":"tok"}}`,
			wantCells:   []string{"abc"},
			wantColumns: []string{"ID"},
		},
		{
			name:        "data object with an errors sibling still unwraps",
			body:        `{"data":{"id":"abc"},"errors":[{"message":"partial"}]}`,
			wantCells:   []string{"abc"},
			wantColumns: []string{"ID"},
		},
		{
			// Unwrapping would drop `total`, so the envelope itself is the row.
			name:        "data object beside a foreign key is one row of the envelope",
			body:        `{"data":{"id":"abc"},"total":1}`,
			wantCells:   []string{"id:abc", "1"},
			wantColumns: []string{"DATA", "TOTAL"},
		},
		{
			name:        "ragged rows union all keys",
			body:        `[{"id":"one"},{"id":"two","extra":"x"}]`,
			wantCells:   []string{"one", "two", "x"},
			wantColumns: []string{"ID", "EXTRA"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, cfg, stdout := newOutputCmd(t, "table")
			assert.NotPanics(t, func() { PrintPassthrough(cmd, cfg, []byte(tc.body)) })

			out := stdout.String()
			for _, cell := range tc.wantCells {
				assert.Contains(t, out, cell)
			}
			for _, col := range tc.wantColumns {
				assert.Contains(t, out, col)
			}
			// A table, not the verbatim JSON fallback.
			assert.Contains(t, out, "─")
			var v any
			assert.Error(t, json.Unmarshal([]byte(out), &v))
		})
	}
}

func TestPrintPassthrough_TableColumnOrderIsDeterministic(t *testing.T) {
	// Go map iteration is randomised, so the column order has to come from a
	// sort, not from range order. Repeat enough to catch a regression.
	body := []byte(`{"data":[{"zzz":1,"mmm":2,"aaa":3}]}`)
	cmd, cfg, stdout := newOutputCmd(t, "table")
	PrintPassthrough(cmd, cfg, body)
	first := stdout.String()

	require.Less(t, strings.Index(first, "AAA"), strings.Index(first, "MMM"))
	require.Less(t, strings.Index(first, "MMM"), strings.Index(first, "ZZZ"))

	for i := 0; i < 25; i++ {
		cmd, cfg, stdout := newOutputCmd(t, "table")
		PrintPassthrough(cmd, cfg, body)
		assert.Equal(t, first, stdout.String())
	}
}

func TestPrintPassthrough_TableRaggedColumnOrderIsDeterministic(t *testing.T) {
	// The union is built across rows, so the per-row sort must hold there too.
	body := []byte(`[{"b":1,"a":2},{"d":3,"c":4}]`)
	cmd, cfg, stdout := newOutputCmd(t, "table")
	PrintPassthrough(cmd, cfg, body)
	first := stdout.String()

	for _, pair := range [][2]string{{"A", "B"}, {"B", "C"}, {"C", "D"}} {
		require.Less(t, strings.Index(first, pair[0]), strings.Index(first, pair[1]),
			"column %q must precede %q", pair[0], pair[1])
	}
	for i := 0; i < 25; i++ {
		cmd, cfg, stdout := newOutputCmd(t, "table")
		PrintPassthrough(cmd, cfg, body)
		assert.Equal(t, first, stdout.String())
	}
}

func TestPrintPassthrough_TableStripsControlBytes(t *testing.T) {
	// Both the header (a response key) and the cell are attacker-controlled here,
	// so the strip has to happen before the columns are derived from the keys.
	cmd, cfg, stdout := newOutputCmd(t, "table")
	body := []byte(`{"data":[{"na\u001bme":"foo\u001b[31mbar"}]}`)

	assert.NotPanics(t, func() { PrintPassthrough(cmd, cfg, body) })

	out := stdout.String()
	assert.NotContains(t, out, "\x1b", "header/cell must not carry a raw ESC byte")
	assert.Contains(t, out, "foo?[31mbar", "cell value is control-stripped, not dropped")
	assert.Contains(t, out, "NA?ME", "header is control-stripped, not dropped")
}

func TestPrintPassthrough_TableFallsBackToBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "bare scalar string", body: `"hello"`},
		{name: "bare number", body: `42`},
		{name: "null", body: `null`},
		{name: "bool", body: `true`},
		{name: "array of numbers", body: `[1,2,3]`},
		{name: "array of mixed shapes", body: `[{"id":"a"},1]`},
		{name: "invalid json", body: `{"a":`},
		{name: "plain text", body: `service unavailable`},
		{name: "empty data array yields no columns", body: `{"data":[]}`},
		// An envelope whose `data` cannot become a row would otherwise render a
		// nonsense one-cell DATA table.
		{name: "data holding a string", body: `{"data":"ok"}`},
		{name: "data holding a number", body: `{"data":7}`},
		{name: "data holding null", body: `{"data":null}`},
		{name: "data holding an empty object", body: `{"data":{}}`},
		{name: "data holding an array of scalars", body: `{"data":[1,2,3]}`},
		{name: "data holding a scalar beside links", body: `{"data":null,"links":{"next":"t"}}`},
		{name: "empty array", body: `[]`},
		{name: "object with no keys", body: `{}`},
		// printTable would read the colon as a nested-key path and render an
		// empty cell, so the body is shown instead.
		{name: "response key containing a colon", body: `{"a:b":"x"}`},
		{name: "colon key inside an unwrapped data object", body: `{"data":{"a:b":"x"}}`},
		{name: "colon key in a later row of the union", body: `[{"id":"a"},{"x:y":"b"}]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, cfg, stdout := newOutputCmd(t, "table")
			assert.NotPanics(t, func() { PrintPassthrough(cmd, cfg, []byte(tc.body)) })
			assert.Equal(t, tc.body+"\n", stdout.String())
		})
	}
}

func TestPrintPassthrough_UnrenderableBodyStripsControlBytes(t *testing.T) {
	// table resolves only for a terminal and toon for an agent harness, so the
	// unrenderable-body path must not emit a raw ESC. Only the json branch's
	// byte-for-byte contract overrides that.
	body := []byte("upstream said \x1b[31mboom\x1b[0m")
	for _, format := range []string{"table", "toon"} {
		t.Run(format, func(t *testing.T) {
			cmd, cfg, stdout := newOutputCmd(t, format)
			PrintPassthrough(cmd, cfg, body)
			assert.Equal(t, "upstream said ?[31mboom?[0m\n", stdout.String())
		})
	}
	t.Run("json stays verbatim", func(t *testing.T) {
		cmd, cfg, stdout := newOutputCmd(t, "json")
		PrintPassthrough(cmd, cfg, body)
		assert.Equal(t, string(body)+"\n", stdout.String())
	})
}

// TestPrintPassthrough_SpecEnvelopeShapes covers every response envelope the
// v2beta1 spec audit found, in all three formats, asserting only that nothing
// panics and that stdout is never silently empty for a non-empty body.
func TestPrintPassthrough_SpecEnvelopeShapes(t *testing.T) {
	bodies := map[string]string{
		"data list":       `{"data":[{"id":"a"}]}`,
		"data object":     `{"data":{"id":"a"}}`,
		"data and errors": `{"data":[{"id":"a"}],"errors":[{"message":"m"}]}`,
		"data and links":  `{"data":[{"id":"a"}],"links":{"next":"t"}}`,
		"bare object":     `{"token":"abc"}`,
		"bare array":      `[{"id":"a"}]`,
		"scalar":          `"a"`,
		"null":            `null`,
		"non json":        `oops`,
	}
	for _, format := range []string{"json", "table", "toon"} {
		for name, body := range bodies {
			t.Run(format+"/"+name, func(t *testing.T) {
				cmd, cfg, stdout := newOutputCmd(t, format)
				assert.NotPanics(t, func() { PrintPassthrough(cmd, cfg, []byte(body)) })
				assert.NotEmpty(t, stdout.String())
			})
		}
	}
}

func TestPrintPassthrough_EmptyBodyWritesNothingInEveryFormat(t *testing.T) {
	for _, format := range []string{"json", "table", "toon"} {
		t.Run(format, func(t *testing.T) {
			cmd, cfg, stdout := newOutputCmd(t, format)
			PrintPassthrough(cmd, cfg, nil)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestPrintPassthrough_HonoursResolvedFormatDefaults(t *testing.T) {
	// With no explicit --format the resolution precedence still applies, so a
	// non-TTY non-agent run must take the verbatim json branch.
	prevTTY := StdoutIsTerminal
	StdoutIsTerminal = func() bool { return false }
	t.Cleanup(func() { StdoutIsTerminal = prevTTY })

	cmd, cfg, stdout := newOutputCmd(t, "default")
	body := `{"data":[{"id":"a"}]}`
	PrintPassthrough(cmd, cfg, []byte(body))
	assert.Equal(t, body+"\n", stdout.String())
}
