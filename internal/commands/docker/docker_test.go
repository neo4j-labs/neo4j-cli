// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	engine "github.com/neo4j/cli/internal/docker"
	"testing"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/testutil/testfs"
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
	assert.True(t, names["get"], "get leaf should be registered on the docker parent")
	assert.True(t, names["list"], "list leaf should be registered on the docker parent")
}

// TestFakeDockerClient_SatisfiesInterface exercises the fake against every
// dockerClient verb so tests in later tasks can rely on the shared shape.
func TestFakeDockerClient_SatisfiesInterface(t *testing.T) {
	var c engine.Client = engine.NewFakeClient()
	ctx := context.Background()

	out, err := c.Run(ctx, []string{"--name", "x", "neo4j:latest"})
	require.NoError(t, err)
	assert.Equal(t, "fake-container-id", out)

	require.NoError(t, c.Start(ctx, "x"))
	require.NoError(t, c.Stop(ctx, "x"))
	require.NoError(t, c.RemoveForce(ctx, "x"))

	entries, err := c.PsAll(ctx, []string{"label=" + engine.LabelManaged + "=true"})
	require.NoError(t, err)
	assert.Empty(t, entries)
}
