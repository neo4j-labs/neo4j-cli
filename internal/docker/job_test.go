// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobSpecArgs_FixedOrder(t *testing.T) {
	job := JobSpec{
		Image:  "img:1",
		Mounts: []Mount{{Source: "/h/in", Target: "/in", ReadOnly: true}, {Source: "vol", Target: "/data"}},
		Env:    []string{"A=1", "B=2"},
		Cmd:    []string{"run", "--flag"},
	}

	assert.Equal(t, []string{
		"--rm",
		"-v", "/h/in:/in:ro", "-v", "vol:/data",
		"-e", "A=1", "-e", "B=2",
		"img:1", "run", "--flag",
	}, job.Args())
}

func TestJobSpecArgs_MinimalJob(t *testing.T) {
	assert.Equal(t, []string{"--rm", "img:1"}, JobSpec{Image: "img:1"}.Args())
}

func TestDatabaseLoadJob_Golden(t *testing.T) {
	job := DatabaseLoadJob("neo4j:5.20-enterprise", "/tmp/stage", "neo4j-cli-dev-data", "movies")

	assert.Equal(t, []string{
		"--rm",
		"-v", "/tmp/stage:/import:ro",
		"-v", "neo4j-cli-dev-data:/data",
		"-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval",
		"neo4j:5.20-enterprise",
		"neo4j-admin", "database", "load", "movies",
		"--from-path=/import",
		"--overwrite-destination=true",
	}, job.Args())
}

// These guard the loader's hard-won invariants (see DatabaseLoadJob).
func TestDatabaseLoadJob_Invariants(t *testing.T) {
	joined := strings.Join(DatabaseLoadJob("img", "/s", "v", "db").Args(), " ")

	assert.NotContains(t, joined, "--entrypoint",
		"the default entrypoint must run so neo4j-admin drops to the neo4j user (uid 7474)")
	assert.Contains(t, joined, "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval",
		"the default entrypoint enforces the licence gate; without it neo4j-admin never runs")
	assert.Contains(t, joined, "/s:/import:ro", "the dump directory is mounted read-only")
	assert.True(t, strings.HasPrefix(joined, "--rm"), "a job never outlives its run")
	assert.NotContains(t, joined, "NEO4J_AUTH", "a job carries no credentials")
}

func TestRunJob_RunsTheJobArgsAndReturnsTheError(t *testing.T) {
	fake := NewFakeClient()
	job := DatabaseLoadJob("img", "/s", "v", "db")

	require.NoError(t, RunJob(context.Background(), fake, job))
	require.Len(t, fake.RunCalls, 1)
	assert.Equal(t, job.Args(), fake.RunCalls[0])

	boom := errors.New("docker: loader failed")
	fake.RunFn = func(context.Context, []string) (string, error) { return "", boom }
	assert.ErrorIs(t, RunJob(context.Background(), fake, job), boom)
}
