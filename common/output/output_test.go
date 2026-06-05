// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/tee"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// simpleData is a minimal ResponseData implementation used across tests.
type simpleData struct {
	rows []map[string]any
}

func (s simpleData) AsArray() []map[string]any { return s.rows }

func (s simpleData) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"data": s.rows})
}

// newOutputCmd returns a command with stdout captured into the returned buffer,
// and a config wired with the given format value.
func newOutputCmd(t *testing.T, format string) (*cobra.Command, *clicfg.Config, *bytes.Buffer) {
	t.Helper()
	cfgJSON := `{"format":"` + format + `"}`
	fs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	cmd := &cobra.Command{}
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	return cmd, cfg, stdout
}

func TestPrintBodyMap_Toon(t *testing.T) {
	tests := []struct {
		name   string
		rows   []map[string]any
		fields []string
	}{
		{
			name:   "single row with string field",
			rows:   []map[string]any{{"name": "alice", "age": float64(30)}},
			fields: []string{"name", "age"},
		},
		{
			name:   "multiple rows",
			rows:   []map[string]any{{"id": "1", "status": "active"}, {"id": "2", "status": "paused"}},
			fields: []string{"id", "status"},
		},
		{
			name:   "empty rows produces non-empty toon document",
			rows:   []map[string]any{},
			fields: []string{"id"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, cfg, stdout := newOutputCmd(t, "toon")
			data := simpleData{rows: tc.rows}
			PrintBodyMap(cmd, cfg, data, tc.fields)

			toonOut := stdout.String()
			assert.NotEmpty(t, toonOut, "toon output must be non-empty")

			// Toon output must not be valid JSON.
			var v any
			err := json.Unmarshal([]byte(toonOut), &v)
			assert.Error(t, err, "toon output should not be valid JSON, got: %s", toonOut)

			// Compare against JSON path for the same data: they must differ.
			jsonCmd, jsonCfg, jsonBuf := newOutputCmd(t, "json")
			PrintBodyMap(jsonCmd, jsonCfg, data, tc.fields)
			jsonOut := jsonBuf.String()
			assert.NotEqual(t, toonOut, jsonOut, "toon output must differ from json output")
		})
	}
}

func TestPrintBodyMap_ToonContainsTopLevelKeys(t *testing.T) {
	// Verify that the toon output contains the same top-level keys as the
	// JSON equivalent. The simpleData MarshalJSON wraps rows in {"data": ...},
	// so the top-level key "data" must appear in both outputs.
	rows := []map[string]any{{"id": "abc"}}
	data := simpleData{rows: rows}

	cmd, cfg, stdout := newOutputCmd(t, "toon")
	PrintBodyMap(cmd, cfg, data, []string{"id"})
	toonOut := stdout.String()

	// "data" is the top-level key emitted by simpleData.MarshalJSON.
	assert.Contains(t, toonOut, "data", "toon output should contain the top-level key 'data'")
}

func TestPrintBodyMaps_JSONArray(t *testing.T) {
	cmd, cfg, stdout := newOutputCmd(t, "json")
	items := []ResponseData{
		simpleData{rows: []map[string]any{{"id": "1"}}},
		simpleData{rows: []map[string]any{{"id": "2"}}},
	}
	fields := [][]string{{"id"}, {"id"}}
	PrintBodyMaps(cmd, cfg, items, fields)

	out := stdout.String()
	var decoded []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &decoded),
		"output must be a JSON array, got: %s", out)
	require.Len(t, decoded, 2)
	// simpleData.MarshalJSON wraps rows under "data".
	assert.Contains(t, out, `"data"`)
}

func TestPrintBodyMaps_TableStacked(t *testing.T) {
	cmd, cfg, stdout := newOutputCmd(t, "table")
	items := []ResponseData{
		simpleData{rows: []map[string]any{{"id": "alpha"}}},
		simpleData{rows: []map[string]any{{"id": "beta"}}},
	}
	fields := [][]string{{"id"}, {"id"}}
	PrintBodyMaps(cmd, cfg, items, fields)

	out := stdout.String()
	assert.Contains(t, out, "alpha")
	assert.Contains(t, out, "beta")
	// Stacked blocks: a blank line separates the two rendered tables.
	assert.Contains(t, out, "\n\n", "table blocks must be separated by a blank line")
	// First block precedes second.
	assert.Less(t, strings.Index(out, "alpha"), strings.Index(out, "beta"))
	// Not a JSON array.
	var v any
	assert.Error(t, json.Unmarshal([]byte(out), &v))
}

func TestPrintBodyMaps_Toon(t *testing.T) {
	cmd, cfg, stdout := newOutputCmd(t, "toon")
	items := []ResponseData{
		simpleData{rows: []map[string]any{{"id": "1"}}},
		simpleData{rows: []map[string]any{{"id": "2"}}},
	}
	fields := [][]string{{"id"}, {"id"}}
	PrintBodyMaps(cmd, cfg, items, fields)

	out := stdout.String()
	assert.NotEmpty(t, out)
	// TOON output is not valid JSON.
	var v any
	assert.Error(t, json.Unmarshal([]byte(out), &v))
}

func TestStripControl(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty string", in: "", want: ""},
		{name: "no control chars", in: "hello world", want: "hello world"},
		{name: "preserves tab/newline/CR", in: "a\tb\nc\rd", want: "a\tb\nc\rd"},
		{name: "ANSI escape redacted", in: "foo\x1b[31mbar", want: "foo?[31mbar"},
		{name: "DEL redacted", in: "x\x7fy", want: "x?y"},
		{name: "NUL redacted", in: "x\x00y", want: "x?y"},
		{name: "BEL redacted", in: "x\x07y", want: "x?y"},
		{name: "back-cr-zero redacted", in: "\x08\x00", want: "??"},
		{name: "non-ascii utf8 preserved", in: "café ☃", want: "café ☃"},
		{name: "high control byte (0x9b CSI) preserved (not C0)", in: "\u009b[31m", want: "\u009b[31m"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, StripControl(tc.in))
		})
	}
}

func TestResolveOutput_Toon(t *testing.T) {
	// ResolveOutput must return "toon" when cfg.Global.Format() is "toon",
	// regardless of TTY state.
	prev := StdoutIsTerminal
	StdoutIsTerminal = func() bool { return false }
	t.Cleanup(func() { StdoutIsTerminal = prev })

	cmd, cfg, _ := newOutputCmd(t, "toon")
	got := ResolveOutput(cmd, cfg)
	assert.Equal(t, "toon", got)
}

func TestResolveOutput_ToonWithTTY(t *testing.T) {
	// Even when the writer looks like a TTY, an explicit "toon" config wins.
	prev := StdoutIsTerminal
	StdoutIsTerminal = func() bool { return true }
	t.Cleanup(func() { StdoutIsTerminal = prev })

	cmd, cfg, _ := newOutputCmd(t, "toon")
	got := ResolveOutput(cmd, cfg)
	assert.Equal(t, "toon", got)
}

func TestResolveOutput_WrappedStdoutStillTable(t *testing.T) {
	// CLI-109 regression: main.go wraps the command's writers with a tee
	// io.MultiWriter. The pre-fix StdoutIsTerminal type-asserted the writer to
	// *os.File, so wrapping made it report non-TTY and defaults fell back to
	// "json". The parameterless seam reads the real os.Stdout FD and is immune
	// to the wrapping, so a TTY default must still resolve to "table".
	cmd, cfg, _ := newOutputCmd(t, "default")
	buf := &tee.LimitedBuffer{}
	cmd.SetOut(io.MultiWriter(os.Stdout, buf))
	cmd.SetErr(io.MultiWriter(os.Stderr, buf))

	prevTTY := StdoutIsTerminal
	StdoutIsTerminal = func() bool { return true }
	t.Cleanup(func() { StdoutIsTerminal = prevTTY })

	prevAgent := IsAgent
	IsAgent = func() bool { return false }
	t.Cleanup(func() { IsAgent = prevAgent })

	assert.Equal(t, "table", ResolveOutput(cmd, cfg))
}

func TestResolveOutput_AgentDefaultsToon(t *testing.T) {
	prevAgent := IsAgent
	IsAgent = func() bool { return true }
	t.Cleanup(func() { IsAgent = prevAgent })

	// Agent wins even over a TTY stdout.
	prevTTY := StdoutIsTerminal
	StdoutIsTerminal = func() bool { return true }
	t.Cleanup(func() { StdoutIsTerminal = prevTTY })

	cmd, cfg, _ := newOutputCmd(t, "default")
	assert.Equal(t, "toon", ResolveOutput(cmd, cfg))
}

func TestResolveOutput_ExplicitOverridesAutoBranches(t *testing.T) {
	// Explicit --format must win over both the agent and TTY auto-branches.
	prevAgent := IsAgent
	IsAgent = func() bool { return true }
	t.Cleanup(func() { IsAgent = prevAgent })

	prevTTY := StdoutIsTerminal
	StdoutIsTerminal = func() bool { return true }
	t.Cleanup(func() { StdoutIsTerminal = prevTTY })

	for _, format := range []string{"json", "table", "toon"} {
		t.Run(format, func(t *testing.T) {
			cmd, cfg, _ := newOutputCmd(t, format)
			assert.Equal(t, format, ResolveOutput(cmd, cfg))
		})
	}
}
