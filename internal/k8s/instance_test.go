// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"testing"
	"time"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const readyNeo4jJSON = `{
  "apiVersion":"neo4j.com/v1","kind":"Neo4j",
  "metadata":{"name":"dev","namespace":"team-a","generation":2},
  "spec":{"edition":"enterprise","version":"2026.05.0","topology":{"mode":"Standalone"}},
  "status":{"phase":"Running","observedGeneration":2,
    "serverSummary":{"servers":1,"ready":1},
    "conditions":[{"type":"Ready","status":"True","message":"ready"}]}
}`

func TestNeo4jReads_ArgvAndParsing(t *testing.T) {
	stubLookPath(t)
	calls := captureRun(t, []runResult{
		{stdout: readyNeo4jJSON},
		{stdout: `{"items":[` + readyNeo4jJSON + `]}`},
		{},
	})
	client := NewClient(Settings{Kubeconfig: "/tmp/kube", Context: "kind-kind", Namespace: "team-a"})

	instance, err := client.GetNeo4j(context.Background(), "dev")
	require.NoError(t, err)
	assert.Equal(t, "dev", instance.Name)
	assert.Equal(t, "team-a", instance.Namespace)
	assert.Equal(t, "Standalone", instance.Mode)
	assert.True(t, instance.Ready)
	assert.Equal(t, int32(1), instance.ReadyServers)
	assert.Equal(t, "neo4j.com/v1", instance.Record["apiVersion"])

	instances, err := client.ListNeo4j(context.Background())
	require.NoError(t, err)
	require.Len(t, instances, 1)
	require.NoError(t, client.DeleteNeo4j(context.Background(), "dev"))

	require.Len(t, *calls, 3)
	assert.Equal(t, []string{
		"get", "neo4j", "dev", "-o", "json", "--kubeconfig", "/tmp/kube", "--context", "kind-kind", "-n", "team-a",
	}, (*calls)[0].args)
	assert.Equal(t, []string{
		"get", "neo4j", "-l", "app.kubernetes.io/managed-by=neo4j-cli", "-o", "json",
		"--kubeconfig", "/tmp/kube", "--context", "kind-kind", "-n", "team-a",
	}, (*calls)[1].args)
	assert.Equal(t, []string{
		"delete", "neo4j", "dev", "--wait=false", "--kubeconfig", "/tmp/kube", "--context", "kind-kind", "-n", "team-a",
	}, (*calls)[2].args)
}

func TestCreateNeo4j_PipesManifestOnStdin(t *testing.T) {
	stubLookPath(t)
	var gotPath string
	var gotArgs, gotInput []string
	SetRunWithInputFnForTest(t, func(_ context.Context, path string, args []string, input []byte) runResult {
		gotPath = path
		gotArgs = args
		gotInput = []string{string(input)}
		return runResult{stdout: readyNeo4jJSON}
	})
	client := NewClient(Settings{Context: "kind-kind", Namespace: "team-a"})
	instance, err := client.CreateNeo4j(context.Background(), []byte("kind: Neo4j\n"))
	require.NoError(t, err)
	assert.Equal(t, "dev", instance.Name)
	assert.Equal(t, "/usr/local/bin/fake-kubectl", gotPath)
	assert.Equal(t, []string{"create", "-f", "-", "-o", "json", "--context", "kind-kind", "-n", "team-a"}, gotArgs)
	assert.Equal(t, []string{"kind: Neo4j\n"}, gotInput)
}

func TestOperatorDeployment_ReadsOnlyOperatorNamespace(t *testing.T) {
	stubLookPath(t)
	deployment := `{
	  "metadata":{"name":"neo4j-operator-controller-manager","namespace":"operators","generation":1},
	  "spec":{"replicas":1,"template":{"spec":{"containers":[{"name":"manager","env":[{"name":"WATCH_NAMESPACE","value":"team-a,team-b"}]}]}}},
	  "status":{"replicas":1,"readyReplicas":1,"observedGeneration":1,"conditions":[{"type":"Available","status":"True"}]}
	}`
	calls := captureRun(t, []runResult{{stdout: deployment}})
	client := NewClient(Settings{Context: "kind-kind", Namespace: "team-a"})
	status, err := client.OperatorDeployment(context.Background(), "operators")
	require.NoError(t, err)
	assert.True(t, status.Available)
	assert.Equal(t, []string{"team-a", "team-b"}, status.WatchNamespaces)
	require.Len(t, *calls, 1)
	assert.Equal(t, []string{
		"get", "deployment", OperatorDeploymentName, "-o", "json", "--context", "kind-kind", "-n", "operators",
	}, (*calls)[0].args)
}

func TestParseNeo4j_StaleGenerationIsNotReady(t *testing.T) {
	instance, err := parseNeo4j(readyNeo4jJSON)
	require.NoError(t, err)
	instance.Record["metadata"].(map[string]any)["generation"] = float64(3)
	instance, err = neo4jFromRecord(instance.Record)
	require.NoError(t, err)
	assert.False(t, instance.Ready)
}

func TestWaitForReady(t *testing.T) {
	previous := waitPollInterval
	waitPollInterval = time.Millisecond
	t.Cleanup(func() { waitPollInterval = previous })

	t.Run("polls to the current ready generation", func(t *testing.T) {
		fake := NewFakeClient()
		pending := defaultFakeNeo4j()
		pending.Ready = false
		pending.Phase = "Provisioning"
		calls := 0
		fake.GetNeo4jFn = func(context.Context, string) (Neo4j, error) {
			calls++
			if calls == 1 {
				return pending, nil
			}
			return defaultFakeNeo4j(), nil
		}
		instance, err := WaitForReady(context.Background(), fake, "dev", time.Second)
		require.NoError(t, err)
		assert.True(t, instance.Ready)
		assert.Equal(t, 2, calls)
	})

	t.Run("failed phase returns the condition message", func(t *testing.T) {
		fake := NewFakeClient()
		failed := defaultFakeNeo4j()
		failed.Ready = false
		failed.Phase = "Failed"
		failed.Message = "admission failed"
		fake.GetNeo4jResult = failed
		_, err := WaitForReady(context.Background(), fake, "dev", time.Second)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "admission failed")
	})

	t.Run("timeout is an upstream error", func(t *testing.T) {
		fake := NewFakeClient()
		pending := defaultFakeNeo4j()
		pending.Ready = false
		pending.Phase = "Provisioning"
		fake.GetNeo4jResult = pending
		_, err := WaitForReady(context.Background(), fake, "dev", 5*time.Millisecond)
		require.Error(t, err)
		var cliErr *clierr.CLIError
		require.ErrorAs(t, err, &cliErr)
		assert.Equal(t, "upstream_error", cliErr.CodeName)
	})

	t.Run("deadline inside a read reports the last phase seen", func(t *testing.T) {
		fake := NewFakeClient()
		pending := defaultFakeNeo4j()
		pending.Ready = false
		pending.Phase = "Provisioning"
		calls := 0
		fake.GetNeo4jFn = func(ctx context.Context, _ string) (Neo4j, error) {
			calls++
			if calls == 1 {
				return pending, nil
			}
			<-ctx.Done()
			return Neo4j{}, ctx.Err()
		}
		_, err := WaitForReady(context.Background(), fake, "dev", 5*time.Millisecond)
		require.Error(t, err)
		var cliErr *clierr.CLIError
		require.ErrorAs(t, err, &cliErr)
		assert.Equal(t, "upstream_error", cliErr.CodeName)
		assert.Contains(t, err.Error(), "timed out")
		assert.Contains(t, err.Error(), "last phase: Provisioning")
	})
}

func TestWaitForDelete(t *testing.T) {
	previous := waitPollInterval
	waitPollInterval = time.Millisecond
	t.Cleanup(func() { waitPollInterval = previous })
	fake := NewFakeClient()
	calls := 0
	fake.GetNeo4jFn = func(context.Context, string) (Neo4j, error) {
		calls++
		if calls < 2 {
			return defaultFakeNeo4j(), nil
		}
		return Neo4j{}, clierr.NewNotFoundError("gone")
	}
	require.NoError(t, WaitForDelete(context.Background(), fake, "dev", time.Second))
	assert.Equal(t, 2, calls)
}

func TestWaitForDelete_DeadlineInsideAReadStillReportsTheTimeout(t *testing.T) {
	fake := NewFakeClient()
	fake.GetNeo4jFn = func(ctx context.Context, _ string) (Neo4j, error) {
		<-ctx.Done()
		return Neo4j{}, ctx.Err()
	}
	err := WaitForDelete(context.Background(), fake, "dev", 5*time.Millisecond)
	require.Error(t, err)
	var cliErr *clierr.CLIError
	require.ErrorAs(t, err, &cliErr)
	assert.Equal(t, "upstream_error", cliErr.CodeName)
	assert.Contains(t, err.Error(), "to be deleted")
}
