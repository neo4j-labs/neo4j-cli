// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCmd_Scaffold confirms the parent docker command is wired with the
// expected Use / Short / Long, exposes registered leaves, and is itself
// non-runnable (so TestAllLeafCommands_HaveExamples does not require an
// Example block on the parent).
func TestNewCmd_Scaffold(t *testing.T) {
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	cmd := NewCmd(cfg)
	require.NotNil(t, cmd)

	assert.Equal(t, "docker", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.False(t, cmd.Runnable(), "parent docker cmd must not be runnable; leaves carry RunE")

	names := make(map[string]bool, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}
	assert.True(t, names["create"], "create leaf should be registered on the docker parent")
}

// TestFakeDockerClient_SatisfiesInterface exercises the fake against every
// dockerClient verb so tests in later tasks can rely on the shared shape.
func TestFakeDockerClient_SatisfiesInterface(t *testing.T) {
	var c dockerClient = newFakeDockerClient()
	ctx := context.Background()

	out, err := c.Run(ctx, []string{"--name", "x", "neo4j:latest"})
	require.NoError(t, err)
	assert.Equal(t, "fake-container-id", out)

	require.NoError(t, c.Start(ctx, "x"))
	require.NoError(t, c.Stop(ctx, "x"))
	require.NoError(t, c.RemoveForce(ctx, "x"))

	entries, err := c.PsAll(ctx, []string{"label=" + LabelManaged + "=true"})
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// TestExecClient_LookupMissingDocker confirms REQ-F-060: when docker is
// absent from PATH, the resolver returns the documented clierr.UsageError
// with the install hint. We force the miss by clearing PATH on the spawn.
func TestExecClient_LookupMissingDocker(t *testing.T) {
	t.Setenv("PATH", "")
	ec := &execClient{}
	_, err := ec.resolve()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docker not found in PATH")
	assert.Contains(t, err.Error(), "install Docker Desktop")
}

// TestLabelsConstants is a regression guard for REQ-F-011: every label key
// must remain under the org.neo4j.cli namespace so the discovery filter
// (label=org.neo4j.cli.managed=true) continues to scope correctly.
func TestLabelsConstants(t *testing.T) {
	for _, lbl := range []string{
		LabelManaged,
		LabelEdition,
		LabelVersion,
		LabelBoltPort,
		LabelHTTPPort,
		LabelEphemeral,
	} {
		assert.True(t, strings.HasPrefix(lbl, "org.neo4j.cli."),
			"label %q must remain under the org.neo4j.cli namespace", lbl)
	}
}
