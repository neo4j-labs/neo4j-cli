// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runList drives the `docker list` leaf against the fake docker client with
// the given pre-populated PsEntries. Returns the fake (for filter assertions),
// stdout (for output assertions) and the execution error.
func runList(t *testing.T, args string, entries []PsEntry) (*fakeDockerClient, string, error) {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	fake := newFakeDockerClient()
	fake.PsEntries = entries
	origFactory := clientFactory
	clientFactory = func(bool) dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

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
	return fake, out.String(), execErr
}

// managedLabels returns a Docker-style comma-separated labels string with the
// `org.neo4j.cli.managed=true` marker plus the per-test metadata. Mirrors what
// `docker ps --format '{{json .}}'` emits for a container created by
// `neo4j-cli docker create`.
func managedLabels(edition, version, boltPort, httpPort, ephemeral string) string {
	return strings.Join([]string{
		LabelManaged + "=true",
		LabelEdition + "=" + edition,
		LabelVersion + "=" + version,
		LabelBoltPort + "=" + boltPort,
		LabelHTTPPort + "=" + httpPort,
		LabelEphemeral + "=" + ephemeral,
	}, ",")
}

func TestList_PassesManagedLabelFilterToDocker(t *testing.T) {
	// `docker list` must pass label=org.neo4j.cli.managed=true through to
	// `docker ps --filter` so production calls stay efficient even when the
	// host has hundreds of unrelated containers (REQ-F-020).
	fake, _, err := runList(t, "", nil)
	require.NoError(t, err)
	require.Len(t, fake.PsAllCalls, 1)
	assert.Equal(t, []string{"label=" + LabelManaged + "=true"}, fake.PsAllCalls[0])
}

func TestList_EmptyResult_FormatJson_RendersEmptyArray(t *testing.T) {
	// REQ-F-022: empty list renders as `[]`, exit 0 — never `null`, never an error.
	_, stdout, err := runList(t, "--format json", nil)
	require.NoError(t, err)

	// Strip cobra's trailing newline before comparing.
	trimmed := strings.TrimSpace(stdout)
	assert.Equal(t, "[]", trimmed, "empty result must render as []; got %q", stdout)

	// And the JSON must parse to a zero-length slice (not null).
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	assert.Empty(t, rows)
}

func TestList_EmptyResult_FormatTable_RendersHeaderOnly(t *testing.T) {
	// Table mode on empty input still renders the header row so an operator
	// piping `docker list` into less/grep sees the column layout. The
	// go-pretty/v6 table writer uppercases header text, so assert
	// case-insensitively for the 7 documented columns.
	_, stdout, err := runList(t, "--format table", nil)
	require.NoError(t, err)
	upper := strings.ToUpper(stdout)
	for _, col := range []string{"NAME", "STATUS", "EDITION", "VERSION", "BOLT_PORT", "HTTP_PORT", "EPHEMERAL"} {
		assert.Contains(t, upper, col, "table header missing column %q", col)
	}
}

func TestList_OneManagedRunning_RendersAllSevenFields(t *testing.T) {
	entries := []PsEntry{
		{
			ID:     "abc123",
			Names:  "dev",
			Status: "Up 5 minutes",
			State:  "running",
			Image:  "neo4j:latest-enterprise",
			Labels: managedLabels("enterprise", "latest", "7687", "7474", "false"),
		},
	}

	_, stdout, err := runList(t, "--format json", entries)
	require.NoError(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	row := rows[0]

	assert.Equal(t, "dev", row["name"])
	assert.Equal(t, "Up 5 minutes", row["status"])
	assert.Equal(t, "enterprise", row["edition"])
	assert.Equal(t, "latest", row["version"])
	assert.Equal(t, "7687", row["bolt_port"])
	assert.Equal(t, "7474", row["http_port"])
	assert.Equal(t, false, row["ephemeral"], "ephemeral=false label must render as JSON bool false")
}

func TestList_EphemeralLabel_RendersAsBoolTrue(t *testing.T) {
	entries := []PsEntry{
		{
			Names:  "tmp",
			Status: "Up 30 seconds",
			Labels: managedLabels("enterprise", "latest", "7687", "7474", "true"),
		},
	}
	_, stdout, err := runList(t, "--format json", entries)
	require.NoError(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, true, rows[0]["ephemeral"], "ephemeral=true label must render as JSON bool true")
}

func TestList_OneManagedOneUnmanaged_FiltersUnmanaged(t *testing.T) {
	// Even if docker (or a misbehaving test seam) hands us an unmanaged
	// container, the in-Go filter must drop it so REQ-F-020 holds.
	entries := []PsEntry{
		{
			Names:  "dev",
			Status: "Up 1 hour",
			Labels: managedLabels("community", "5.20", "7687", "7474", "false"),
		},
		{
			Names:  "other",
			Status: "Up 2 days",
			Labels: "com.example.unrelated=yes",
		},
		{
			Names:  "nolabels",
			Status: "Up 3 days",
			Labels: "",
		},
	}

	_, stdout, err := runList(t, "--format json", entries)
	require.NoError(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1, "only the managed container should render")
	assert.Equal(t, "dev", rows[0]["name"])
	assert.Equal(t, "community", rows[0]["edition"])
	assert.Equal(t, "5.20", rows[0]["version"])
}

func TestList_TwoManaged_RunningAndExited_BothRendered(t *testing.T) {
	entries := []PsEntry{
		{
			Names:  "running-one",
			Status: "Up 10 minutes",
			Labels: managedLabels("enterprise", "5.20", "7687", "7474", "false"),
		},
		{
			Names:  "stopped-one",
			Status: "Exited (0) 3 minutes ago",
			Labels: managedLabels("community", "latest", "7688", "7475", "false"),
		},
	}

	_, stdout, err := runList(t, "--format json", entries)
	require.NoError(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 2)
	assert.Equal(t, "running-one", rows[0]["name"])
	assert.Equal(t, "Up 10 minutes", rows[0]["status"])
	assert.Equal(t, "stopped-one", rows[1]["name"])
	assert.Equal(t, "Exited (0) 3 minutes ago", rows[1]["status"])
}

func TestList_StripsLeadingSlashFromName(t *testing.T) {
	// The docker daemon historically prepends "/" to container names; the
	// `{{json .}}` format usually elides it, but defending against it here
	// keeps the rendered output predictable across daemon versions.
	entries := []PsEntry{
		{
			Names:  "/dev",
			Status: "Up 1 minute",
			Labels: managedLabels("enterprise", "latest", "7687", "7474", "false"),
		},
	}
	_, stdout, err := runList(t, "--format json", entries)
	require.NoError(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "dev", rows[0]["name"])
}

func TestList_FormatTable_RendersAllSevenColumnsAndRow(t *testing.T) {
	entries := []PsEntry{
		{
			Names:  "dev",
			Status: "Up 5 minutes",
			Labels: managedLabels("enterprise", "5.20", "7687", "7474", "false"),
		},
	}
	_, stdout, err := runList(t, "--format table", entries)
	require.NoError(t, err)
	// go-pretty/v6 uppercases header text — assert headers against the
	// uppercased copy of stdout.
	upper := strings.ToUpper(stdout)
	for _, col := range []string{"NAME", "STATUS", "EDITION", "VERSION", "BOLT_PORT", "HTTP_PORT", "EPHEMERAL"} {
		assert.Contains(t, upper, col, "table missing column %q in output:\n%s", col, stdout)
	}
	// Row values render as-is (lower-case where appropriate).
	for _, val := range []string{"dev", "Up 5 minutes", "enterprise", "5.20", "7687", "7474"} {
		assert.Contains(t, stdout, val, "table missing value %q in output:\n%s", val, stdout)
	}
}

func TestParseLabels_HandlesEmptyAndMalformed(t *testing.T) {
	assert.Empty(t, parseLabels(""))

	out := parseLabels("k1=v1,k2=v2")
	assert.Equal(t, "v1", out["k1"])
	assert.Equal(t, "v2", out["k2"])

	// Malformed entries (no `=`) are silently skipped.
	out2 := parseLabels("k1=v1,malformed,k2=v2")
	assert.Equal(t, "v1", out2["k1"])
	assert.Equal(t, "v2", out2["k2"])
	_, hasMalformed := out2["malformed"]
	assert.False(t, hasMalformed)

	// Values may themselves contain `=` (e.g. base64 paddings) — first `=` wins.
	out3 := parseLabels("k=a=b")
	assert.Equal(t, "a=b", out3["k"])
}

func TestFirstName_PrefersFirstNonEmptyTrimmed(t *testing.T) {
	assert.Equal(t, "", firstName(""))
	assert.Equal(t, "dev", firstName("dev"))
	assert.Equal(t, "dev", firstName("/dev"))
	assert.Equal(t, "dev", firstName("dev,alias1"))
	assert.Equal(t, "alias1", firstName(",alias1"))
}
