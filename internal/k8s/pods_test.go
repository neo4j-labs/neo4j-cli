// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListPods_ReadsReadinessAndNamedBoltPort(t *testing.T) {
	stubLookPath(t)
	calls := captureRun(t, []runResult{{stdout: `{"items":[
		{"metadata":{"name":"dev-server-0"},"spec":{"containers":[{"ports":[
			{"name":"http","containerPort":7474},{"name":"bolt","containerPort":17687}
		]}]},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}},
		{"metadata":{"name":"dev-server-1","deletionTimestamp":"2026-11-27T12:00:00Z"},
		 "status":{"phase":"Running","conditions":[{"type":"Ready","status":"False"}]}}
	]}`}})
	pods, err := NewClient(Settings{Kubeconfig: "/tmp/kube", Context: "kind-kind", Namespace: "team-a"}).
		ListPods(context.Background(), "app.kubernetes.io/instance=dev,neo4j.com/pool=server")
	require.NoError(t, err)
	require.Len(t, pods, 2)
	assert.Equal(t, Pod{Name: "dev-server-0", Phase: "Running", Ready: true, BoltPort: 17687}, pods[0])
	assert.False(t, pods[1].Ready)
	assert.True(t, pods[1].Deleting)
	assert.Zero(t, pods[1].BoltPort)
	require.Len(t, *calls, 1)
	assert.Equal(t, []string{
		"get", "pods", "-l", "app.kubernetes.io/instance=dev,neo4j.com/pool=server", "-o", "json",
		"--kubeconfig", "/tmp/kube", "--context", "kind-kind", "-n", "team-a",
	}, (*calls)[0].args)
}

func TestListPods_RejectsMalformedJSON(t *testing.T) {
	stubLookPath(t)
	captureRun(t, []runResult{{stdout: "not-json"}})
	_, err := NewClient(Settings{}).ListPods(context.Background(), "app=neo4j")
	require.Error(t, err)
}
