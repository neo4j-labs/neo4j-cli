// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package history

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func manyEntries(n int) []Entry {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	out := make([]Entry, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, mkEntry(fmt.Sprintf("neo4j-cli cmd-%02d", i), "agent", base.Add(time.Duration(i)*time.Minute)))
	}
	return out
}

func TestList_DefaultLast20NewestFirst(t *testing.T) {
	cfg := newTestConfigFmt(t, "table")
	seedEntries(t, cfg, manyEntries(25))

	out, err := runCmd(t, newListCmd(cfg))
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 20)
	// Newest (cmd-24) first, then descending; last shown is cmd-05.
	assert.Contains(t, lines[0], "neo4j-cli cmd-24")
	assert.Contains(t, lines[19], "neo4j-cli cmd-05")
	assert.NotContains(t, out, "cmd-04")
}

func TestList_LimitOverrides(t *testing.T) {
	cfg := newTestConfigFmt(t, "table")
	seedEntries(t, cfg, manyEntries(25))

	out, err := runCmd(t, newListCmd(cfg), "--limit", "3")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3)
	assert.Contains(t, lines[0], "cmd-24")
	assert.Contains(t, lines[2], "cmd-22")
}

func TestList_LimitZeroShowsAll(t *testing.T) {
	cfg := newTestConfigFmt(t, "table")
	seedEntries(t, cfg, manyEntries(25))

	out, err := runCmd(t, newListCmd(cfg), "--limit", "0")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 25)
}

func TestList_HumanLineIncludesMetadata(t *testing.T) {
	cfg := newTestConfigFmt(t, "table")
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	e := Entry{Time: at, Command: "neo4j-cli instance list", Invoker: "human", Version: "v1", Workspace: "{org}", Credential: "prod"}
	seedEntries(t, cfg, []Entry{e})

	out, err := runCmd(t, newListCmd(cfg))
	require.NoError(t, err)

	assert.Contains(t, out, "[2026-06-01T12:00:00Z] neo4j-cli instance list {invoker:human, workspace:{org}, credential:prod}")
}

func TestList_FormatVariants(t *testing.T) {
	for _, tc := range []struct {
		name     string
		format   string
		mustHave []string
	}{
		{"json", "json", []string{`"command": "neo4j-cli cmd-01"`, "[", "]"}},
		{"toon", "toon", []string{"command", "cmd-01"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestConfigFmt(t, tc.format)
			seedEntries(t, cfg, manyEntries(2))

			out, err := runCmd(t, newListCmd(cfg))
			require.NoError(t, err)

			for _, want := range tc.mustHave {
				assert.Contains(t, out, want)
			}
			// Newest-first ordering is preserved in structured output too.
			assert.Less(t, strings.Index(out, "cmd-01"), strings.Index(out, "cmd-00"))
		})
	}
}

func TestList_EmptyHistory(t *testing.T) {
	cfg := newTestConfigFmt(t, "json")

	out, err := runCmd(t, newListCmd(cfg))
	require.NoError(t, err)
	assert.Contains(t, out, "[]")
}
