// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet_ReturnsFullServerObject(t *testing.T) {
	env := newInstanceEnv()
	stdout, _, err := runInstance(t, env, "get dev -n team-a --format json")
	require.NoError(t, err)
	assert.Equal(t, "team-a", env.settings.Namespace)
	assert.Equal(t, []string{"dev"}, env.fake.GetNeo4jNames)
	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &record))
	assert.Equal(t, "Neo4j", record["kind"])
	assert.Contains(t, record, "status")
}

func TestGet_TableUsesSummaryColumns(t *testing.T) {
	env := newInstanceEnv()
	stdout, _, err := runInstance(t, env, "get dev --format table")
	require.NoError(t, err)
	assert.Contains(t, stdout, "NAME")
	assert.Contains(t, stdout, "READY_SERVERS")
	assert.Contains(t, stdout, "dev")
	assert.Contains(t, stdout, "Standalone")
}
