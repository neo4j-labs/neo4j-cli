// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package history

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func newTestConfigFmt(t *testing.T, format string) *clicfg.Config {
	t.Helper()
	config := `{}`
	if format != "" {
		config = `{"format":"` + format + `"}`
	}
	fs, err := testfs.GetTestFs(config, "{}")
	require.NoError(t, err)
	return clicfg.NewConfig(fs, "test-version", clicfg.GlobalScope)
}

// seedEntries writes the given entries to the history file (oldest-first, one
// JSON line each) using the shared store writer.
func seedEntries(t *testing.T, cfg *clicfg.Config, entries []Entry) {
	t.Helper()
	var buf bytes.Buffer
	for _, e := range entries {
		line, err := json.Marshal(e)
		require.NoError(t, err)
		buf.Write(line)
		buf.WriteByte('\n')
	}
	require.NoError(t, afero.WriteFile(cfg.Aura.Fs(), path(), buf.Bytes(), 0600))
}

// runCmd executes a leaf command with the given args, capturing stdout+stderr.
func runCmd(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func mkEntry(cmdStr, invoker string, at time.Time) Entry {
	return Entry{Time: at, Command: cmdStr, Invoker: invoker, Version: "test-version"}
}
