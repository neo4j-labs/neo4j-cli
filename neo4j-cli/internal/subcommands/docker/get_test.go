// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGet drives the `docker get` leaf against the fake docker client with
// the provided container fixtures. Returns the fake (for call assertions),
// stdout (for output assertions), stderr (for parity with other leaves),
// and the execution error.
func runGet(t *testing.T, args string, containers map[string]Container) (*fakeDockerClient, string, string, error) {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	fake := newFakeDockerClient()
	for name, c := range containers {
		fake.Containers[name] = c
	}
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"get"}, argv...))

	execErr := cmd.Execute()
	return fake, out.String(), errBuf.String(), execErr
}

// managedContainer is a small fixture builder for the `get` tests so each
// case names only the values it cares about. The Managed flag mirrors the
// `org.neo4j.cli.managed=true` label set by `create`.
func managedContainer(name, edition, version, boltPort, httpPort, image string, ephemeral bool, status string) Container {
	return Container{
		Name:      name,
		Status:    status,
		Edition:   edition,
		Version:   version,
		BoltPort:  boltPort,
		HTTPPort:  httpPort,
		Ephemeral: ephemeral,
		Image:     image,
		Managed:   true,
	}
}

func TestGet_ManagedContainer_RendersAllNineFields(t *testing.T) {
	// REQ-F-031: `get` renders the seven `list` columns plus uri and image
	// for a single container. Confirm via JSON so the assertion is exact.
	containers := map[string]Container{
		"dev": managedContainer("dev", "enterprise", "5.20", "7687", "7474", "neo4j:5.20-enterprise", false, "Up 5 minutes"),
	}

	fake, stdout, _, err := runGet(t, "dev --format json", containers)
	require.NoError(t, err)
	require.Len(t, fake.InspectCalls, 1)
	assert.Equal(t, "dev", fake.InspectCalls[0])

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1, "get must render a single-element array")
	row := rows[0]

	assert.Equal(t, "dev", row["name"])
	assert.Equal(t, "Up 5 minutes", row["status"])
	assert.Equal(t, "enterprise", row["edition"])
	assert.Equal(t, "5.20", row["version"])
	assert.Equal(t, "7687", row["bolt-port"])
	assert.Equal(t, "7474", row["http-port"])
	assert.Equal(t, false, row["ephemeral"])
	assert.Equal(t, "neo4j://localhost:7687", row["uri"])
	assert.Equal(t, "neo4j:5.20-enterprise", row["image"])
}

func TestGet_URIDerivedFromBoltPortLabel(t *testing.T) {
	// REQ-F-031: uri = neo4j://localhost:<bolt-port>; assert with a
	// non-default bolt port so the formatter is exercised.
	containers := map[string]Container{
		"local": managedContainer("local", "community", "latest", "7689", "7476", "neo4j:latest", false, "Up 1 minute"),
	}
	_, stdout, _, err := runGet(t, "local --format json", containers)
	require.NoError(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "neo4j://localhost:7689", rows[0]["uri"])
}

func TestGet_EphemeralContainer_RendersAsTrue(t *testing.T) {
	containers := map[string]Container{
		"tmp": managedContainer("tmp", "enterprise", "latest", "7687", "7474", "neo4j:latest-enterprise", true, "Up 30 seconds"),
	}
	_, stdout, _, err := runGet(t, "tmp --format json", containers)
	require.NoError(t, err)

	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, true, rows[0]["ephemeral"], "ephemeral=true must render as JSON bool true")
}

func TestGet_MissingContainer_UnknownError(t *testing.T) {
	// REQ-F-032: missing container → usage error pointing at `docker list`,
	// no partial render on stdout.
	fake, stdout, _, err := runGet(t, "ghost --format json", nil)
	require.Error(t, err)
	require.Len(t, fake.InspectCalls, 1, "Inspect must still be attempted before erroring")
	assert.Equal(t, "ghost", fake.InspectCalls[0])

	assert.Contains(t, err.Error(), `no managed container named "ghost"`)
	assert.Contains(t, err.Error(), "neo4j-cli docker list")
	assert.Empty(t, strings.TrimSpace(stdout), "no partial output on unknown name")
}

func TestGet_UnmanagedContainer_UnknownError(t *testing.T) {
	// REQ-F-032: a container that EXISTS in Docker but lacks the managed
	// label is still treated as unknown — same error, no rendered fields.
	// Mark Managed=false explicitly via a direct fixture so the contract
	// hits the in-leaf gate (not just the fake's not-found branch).
	containers := map[string]Container{
		"someones-postgres": {
			Name:    "someones-postgres",
			Status:  "Up 1 day",
			Image:   "postgres:16",
			Managed: false,
		},
	}
	_, stdout, _, err := runGet(t, "someones-postgres --format json", containers)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no managed container named "someones-postgres"`)
	assert.Contains(t, err.Error(), "neo4j-cli docker list")
	assert.Empty(t, strings.TrimSpace(stdout), "no partial output on unmanaged container")
}

func TestGet_NoArgs_CobraUsageError(t *testing.T) {
	// cobra.ExactArgs(1) is the gate: `docker get` with no args must error
	// before RunE fires, so no Inspect call is observed.
	fake, _, _, err := runGet(t, "", nil)
	require.Error(t, err)
	assert.Empty(t, fake.InspectCalls, "RunE must not fire when args validation fails")
	// cobra's stock message names the command and the required arg count
	// — assert on a stable substring rather than the full string.
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestGet_TooManyArgs_CobraUsageError(t *testing.T) {
	fake, _, _, err := runGet(t, "a b", nil)
	require.Error(t, err)
	assert.Empty(t, fake.InspectCalls)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestGet_FormatTable_RendersAllNineColumnsAndRow(t *testing.T) {
	containers := map[string]Container{
		"dev": managedContainer("dev", "enterprise", "5.20", "7687", "7474", "neo4j:5.20-enterprise", false, "Up 5 minutes"),
	}
	_, stdout, _, err := runGet(t, "dev --format table", containers)
	require.NoError(t, err)
	// go-pretty/v6 uppercases header text — assert against an uppercased copy.
	upper := strings.ToUpper(stdout)
	for _, col := range []string{"NAME", "STATUS", "EDITION", "VERSION", "BOLT-PORT", "HTTP-PORT", "EPHEMERAL", "URI", "IMAGE"} {
		assert.Contains(t, upper, col, "table missing column %q in output:\n%s", col, stdout)
	}
	for _, val := range []string{"dev", "Up 5 minutes", "enterprise", "5.20", "7687", "7474", "neo4j://localhost:7687", "neo4j:5.20-enterprise"} {
		assert.Contains(t, stdout, val, "table missing value %q in output:\n%s", val, stdout)
	}
}

func TestGet_InspectErrorTreatedAsUnknown(t *testing.T) {
	// A non-not-found docker error (e.g. daemon unreachable) is still
	// funneled into the unknown-name shape per the leaf's defensive choice
	// — Docker's stderr can change across versions and the unknown-name
	// hint is always actionable. Confirm via a fake InspectFn returning a
	// bespoke error.
	fake := newFakeDockerClient()
	fake.InspectFn = func(ctx context.Context, name string) (Container, error) {
		return Container{}, fmt.Errorf("Cannot connect to the Docker daemon")
	}
	origFactory := clientFactory
	clientFactory = func() dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	fs, fsErr := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, fsErr)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)
	cmd.SetArgs([]string{"get", "dev"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no managed container named "dev"`)
}
