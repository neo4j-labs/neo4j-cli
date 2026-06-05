// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dataset

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dataset"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runList drives the `dataset list` leaf with the given args and returns stdout
// plus the execution error.
func runList(t *testing.T, args string) (string, error) {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"list"}, argv...))

	execErr := cmd.Execute()
	return out.String(), execErr
}

func TestList_RendersAllFormats(t *testing.T) {
	suggestions := dataset.List()
	require.NotEmpty(t, suggestions)
	first := suggestions[0]

	for _, tc := range []struct {
		name string
		args string
	}{
		{name: "json", args: "--format json"},
		{name: "table", args: "--format table"},
		{name: "toon", args: "--format toon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runList(t, tc.args)
			require.NoError(t, err)
			assert.Contains(t, out, first.Slug)
			assert.Contains(t, out, first.Title)
			assert.Contains(t, out, first.Description)
			assert.Contains(t, out, first.OwnerRepo)
		})
	}
}

func TestList_JSONShapeMatchesCatalog(t *testing.T) {
	out, err := runList(t, "--format json")
	require.NoError(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &rows))

	suggestions := dataset.List()
	require.Len(t, rows, len(suggestions))
	for i, s := range suggestions {
		assert.Equal(t, s.Slug, rows[i]["slug"])
		assert.Equal(t, s.Title, rows[i]["title"])
		assert.Equal(t, s.Description, rows[i]["description"])
		assert.Equal(t, s.OwnerRepo, rows[i]["repo"])
	}
}

func TestList_NoLoadLeafUnderDataset(t *testing.T) {
	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	cmd := NewCmd(cfg)
	for _, c := range cmd.Commands() {
		assert.NotEqual(t, "load", c.Name(), "dataset must be discovery-only — no load leaf")
	}
}
