// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package output_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestCommand returns a cobra command whose output and error streams are
// captured, plus the buffers holding them.
func newTestCommand() (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	cmd := &cobra.Command{Use: "list"}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	return cmd, out, errOut
}

// newTestConfig returns a config pinned to --format json so record rendering is
// independent of the host's TTY/agent-harness detection.
func newTestConfig(t *testing.T) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format": "json"}`, "{}")
	require.NoError(t, err)
	return clicfg.NewConfig(fs, "test")
}

// TestPrintPagedRecordsNarratesStopReasons pins the exact stderr wording for
// each way a paginated walk can stop early: a complete walk says nothing, a
// truncated one names the resource and the limit, and a page-capped one names
// the cap. The records themselves are printed in every case.
func TestPrintPagedRecordsNarratesStopReasons(t *testing.T) {
	records := []map[string]any{{"id": "vg-1"}, {"id": "vg-2"}}

	tests := []struct {
		name           string
		limit          int
		limitReached   bool
		pageCapReached bool
		wantErr        string
	}{
		{
			name: "complete walk narrates nothing",
		},
		{
			name:         "limit reached names the resource and the limit",
			limit:        2,
			limitReached: true,
			wantErr:      "Showing the first 2 virtual graphs; more are available. Raise or omit --limit to see them all.\n",
		},
		{
			name:           "page cap reached names the cap",
			pageCapReached: true,
			wantErr:        fmt.Sprintf("Stopped after %d pages, so this list may be incomplete.\n", auraclient.MaxListPages),
		},
		{
			name:           "both stop reasons narrate in order",
			limit:          2,
			limitReached:   true,
			pageCapReached: true,
			wantErr: "Showing the first 2 virtual graphs; more are available. Raise or omit --limit to see them all.\n" +
				fmt.Sprintf("Stopped after %d pages, so this list may be incomplete.\n", auraclient.MaxListPages),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, out, errOut := newTestCommand()

			output.PrintPagedRecords(cmd, newTestConfig(t), records, []string{"id"}, "virtual graphs", tt.limit, tt.limitReached, tt.pageCapReached)

			assert.Equal(t, tt.wantErr, errOut.String())
			assert.Contains(t, out.String(), `"id": "vg-1"`)
			assert.Contains(t, out.String(), `"id": "vg-2"`)
		})
	}
}
