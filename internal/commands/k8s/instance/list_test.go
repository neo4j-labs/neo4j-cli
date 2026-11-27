// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"encoding/json"
	"testing"

	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestList_ReturnsFullServerObjects(t *testing.T) {
	env := newInstanceEnv()
	stdout, _, err := runInstance(t, env, "list --format json")
	require.NoError(t, err)
	assert.Equal(t, 1, env.fake.ListNeo4jCalls)
	var records []map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &records))
	require.Len(t, records, 1)
	assert.Equal(t, "Neo4j", records[0]["kind"])
}

func TestList_EmptyResultIsJSONArray(t *testing.T) {
	env := newInstanceEnv()
	env.fake.ListNeo4jResult = []engine.Neo4j{}
	stdout, _, err := runInstance(t, env, "list --format json")
	require.NoError(t, err)
	assert.JSONEq(t, "[]", stdout)
}
