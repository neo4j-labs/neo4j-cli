// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"encoding/json"
	"testing"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelete_StartsDeletionWithoutWaiting(t *testing.T) {
	env := newInstanceEnv()
	stdout, stderr, err := runInstance(t, env, "delete dev --format json --rw")
	require.NoError(t, err)
	assert.Equal(t, []string{"dev"}, env.fake.DeleteNeo4jNames)
	assert.Zero(t, env.fake.GetNeo4jCalls)
	assert.Contains(t, stderr, "Targeting Kubernetes context")
	assert.Contains(t, stderr, "retention policy")
	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &report))
	assert.Equal(t, "deletion_started", report["status"])
	assert.Equal(t, "follows_spec", report["pvc_retention"])
}

func TestDelete_WaitPollsUntilAbsent(t *testing.T) {
	env := newInstanceEnv()
	env.fake.GetNeo4jErr = clierr.NewNotFoundError("gone")
	stdout, _, err := runInstance(t, env, "delete dev --wait --format json --rw")
	require.NoError(t, err)
	assert.Equal(t, 1, env.fake.GetNeo4jCalls)
	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &report))
	assert.Equal(t, "deleted", report["status"])
}

func TestWriteLeaves_RequireRw(t *testing.T) {
	for _, args := range []string{
		"create dev --accept-license eval",
		"delete dev",
	} {
		t.Run(args, func(t *testing.T) {
			env := newInstanceEnv()
			_, _, err := runInstance(t, env, args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--rw")
			assert.Zero(t, env.fake.CurrentContextCalls)
			assert.Zero(t, env.fake.CreateNeo4jCalls)
			assert.Zero(t, env.fake.DeleteNeo4jCalls)
		})
	}
}
