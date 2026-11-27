// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"encoding/json"
	"testing"

	"github.com/neo4j/cli/internal/clierr"
	engine "github.com/neo4j/cli/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreate_BuildsManifestChecksWatchScopeAndPrintsFullObject(t *testing.T) {
	env := newInstanceEnv()
	stdout, stderr, err := runInstance(t, env,
		"create dev --accept-license eval --memory 8Gi --cpu 2 --heap 2g --pagecache 1g --config db.transaction.timeout=60s --format json --rw")
	require.NoError(t, err)
	assert.Equal(t, []string{"default", "neo4j-operator-system"}, env.namespaces)
	assert.Equal(t, []string{engine.OperatorDeploymentName}, env.fake.DeploymentNames)
	assert.Equal(t, 1, env.fake.CreateNeo4jCalls)
	assert.Zero(t, env.fake.GetNeo4jCalls, "no readiness poll runs without --wait")
	assert.Contains(t, stderr, `Targeting Kubernetes context "kind-kind" in namespace "default"`)
	require.Len(t, env.fake.CreatedManifests, 1)
	manifest := string(env.fake.CreatedManifests[0])
	assert.Contains(t, manifest, "app.kubernetes.io/managed-by: neo4j-cli")
	assert.Contains(t, manifest, "db.transaction.timeout: \"60s\"")
	assert.Contains(t, manifest, "memory: \"8Gi\"")

	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &record))
	assert.Equal(t, "Neo4j", record["kind"])
	assert.Equal(t, "neo4j.com/v1", record["apiVersion"])
}

func TestCreate_WaitPollsUntilReady(t *testing.T) {
	env := newInstanceEnv()
	_, _, err := runInstance(t, env, "create dev --accept-license eval --wait --format json --rw")
	require.NoError(t, err)
	assert.Equal(t, 1, env.fake.GetNeo4jCalls)
}

func TestCreate_OperatorNamespaceFlagSelectsController(t *testing.T) {
	env := newInstanceEnv()
	_, _, err := runInstance(t, env, "create dev --accept-license eval --operator-namespace operators --format json --rw")
	require.NoError(t, err)
	assert.Equal(t, []string{"default", "operators"}, env.namespaces)
}

func TestCreate_FromFilePreservesTLS(t *testing.T) {
	env := newInstanceEnv()
	_, _, err := runInstance(t, env, "create dev --from-file /tmp/neo4j.yaml --format json --rw")
	require.NoError(t, err)
	require.Len(t, env.fake.CreatedManifests, 1)
	manifest := string(env.fake.CreatedManifests[0])
	assert.Contains(t, manifest, "secretName: bolt-tls")
	assert.Contains(t, manifest, "namespace: default")
	assert.Contains(t, manifest, "app.kubernetes.io/managed-by: neo4j-cli")
}

func TestCreate_FailsBeforeCreateWhenNamespaceIsNotWatched(t *testing.T) {
	env := newInstanceEnv()
	env.fake.DeploymentResult.WatchNamespaces = []string{"team-b"}
	_, stderr, err := runInstance(t, env, "create dev --accept-license eval --format json --rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `namespace "default" is not in the operator's watch scope`)
	assert.Zero(t, env.fake.CreateNeo4jCalls)
	assert.Contains(t, stderr, "Targeting Kubernetes context")
}

func TestCreate_FailsBeforeCreateWhenOperatorIsMissing(t *testing.T) {
	env := newInstanceEnv()
	env.fake.DeploymentErr = clierr.NewNotFoundError("missing")
	_, _, err := runInstance(t, env, "create dev --accept-license eval --format json --rw")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operator is not installed")
	assert.Zero(t, env.fake.CreateNeo4jCalls)
}

func TestCreate_LocalValidationRunsBeforeEngineCalls(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    string
		wantErr string
	}{
		{name: "licence", args: "create dev --rw", wantErr: "licence acceptance"},
		{name: "memory", args: "create dev --accept-license eval --heap 2g --pagecache 4g --rw", wantErr: "Raise --memory"},
		{name: "cluster admin path", args: "create dev --mode cluster --accept-license eval --rw", wantErr: "requires --insecure-admin-connection"},
		{name: "file flags are exclusive", args: "create dev --from-file /tmp/neo4j.yaml --heap 2g --rw", wantErr: "cannot be combined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newInstanceEnv()
			_, _, err := runInstance(t, env, tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Zero(t, env.fake.CurrentContextCalls)
			assert.Zero(t, env.fake.DeploymentCalls)
			assert.Zero(t, env.fake.CreateNeo4jCalls)
		})
	}
}

func TestCreate_ClusterWarnings(t *testing.T) {
	env := newInstanceEnv()
	_, stderr, err := runInstance(t, env,
		"create dev --mode cluster --primaries 1 --accept-license eval --insecure-admin-connection --format json --rw")
	require.NoError(t, err)
	assert.Contains(t, stderr, "cannot grow its primary pool")
	assert.Contains(t, stderr, "unencrypted pod-network connection")
}
