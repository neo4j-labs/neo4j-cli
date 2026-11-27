// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"encoding/base64"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSecret_DecodesDataAndCarriesGroupFlags(t *testing.T) {
	stubLookPath(t)
	calls := captureRun(t, []runResult{{stdout: `{
		"kind":"Secret", "metadata":{"name":"dev-auth","namespace":"team-a"},
		"data":{"NEO4J_AUTH":"` + base64.StdEncoding.EncodeToString([]byte("neo4j/password/with/slashes")) + `"}
	}`}})
	var debug strings.Builder
	SetDebugWriterForTest(t, &debug)
	data, err := NewClient(Settings{
		Kubeconfig: "/tmp/kube", Context: "kind-kind", Namespace: "team-a", Debug: true,
	}).GetSecret(context.Background(), "dev-auth")
	require.NoError(t, err)
	assert.Equal(t, []byte("neo4j/password/with/slashes"), data["NEO4J_AUTH"])
	require.Len(t, *calls, 1)
	assert.Equal(t, []string{
		"get", "secret", "dev-auth", "-o", "json", "--kubeconfig", "/tmp/kube", "--context", "kind-kind", "-n", "team-a",
	}, (*calls)[0].args)
	assert.NotContains(t, debug.String(), "password/with/slashes")
	assert.NotContains(t, debug.String(), base64.StdEncoding.EncodeToString(data["NEO4J_AUTH"]))
}

func TestGetSecret_RejectsMalformedData(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stdout string
	}{
		{name: "JSON", stdout: "not-json"},
		{name: "kind", stdout: `{"kind":"ConfigMap"}`},
		{name: "base64", stdout: `{"kind":"Secret","data":{"NEO4J_AUTH":"%%%"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubLookPath(t)
			captureRun(t, []runResult{{stdout: tc.stdout}})
			_, err := NewClient(Settings{}).GetSecret(context.Background(), "dev-auth")
			require.Error(t, err)
		})
	}
}

func TestGetSecret_PreservesNotFound(t *testing.T) {
	stubLookPath(t)
	captureRun(t, []runResult{{stderr: "Error from server (NotFound): secret not found", err: &exec.ExitError{}}})
	_, err := NewClient(Settings{}).GetSecret(context.Background(), "dev-auth")
	assert.True(t, IsNotFound(err))
}
