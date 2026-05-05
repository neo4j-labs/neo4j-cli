// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
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

func TestResolveOutput_Toon(t *testing.T) {
	// ResolveOutput must return "toon" when cfg.Global.Format() is "toon",
	// regardless of TTY state.
	prev := StdoutIsTerminal
	StdoutIsTerminal = func(_ io.Writer) bool { return false }
	t.Cleanup(func() { StdoutIsTerminal = prev })

	cmd, cfg, _ := newOutputCmd(t, "toon")
	got := ResolveOutput(cmd, cfg)
	assert.Equal(t, "toon", got)
}

func TestResolveOutput_ToonWithTTY(t *testing.T) {
	// Even when the writer looks like a TTY, an explicit "toon" config wins.
	prev := StdoutIsTerminal
	StdoutIsTerminal = func(_ io.Writer) bool { return true }
	t.Cleanup(func() { StdoutIsTerminal = prev })

	cmd, cfg, _ := newOutputCmd(t, "toon")
	got := ResolveOutput(cmd, cfg)
	assert.Equal(t, "toon", got)
}
