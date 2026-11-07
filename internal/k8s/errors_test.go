// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exitErr builds a stand-in for a non-zero child exit so classifyRunError
// takes the stderr-mapping branch.
func exitErr() error {
	return &exec.ExitError{}
}

func TestClassifyRunError(t *testing.T) {
	ctx := context.Background()
	argv := []string{"get", "neo4j", "dev"}

	for _, tc := range []struct {
		name   string
		stderr string
		err    error
		code   int
	}{
		{name: "not found", stderr: `Error from server (NotFound): neo4j.neo4j.com "dev" not found`, err: exitErr(), code: 3},
		{name: "already exists", stderr: `Error from server (AlreadyExists): neo4j.neo4j.com "dev" already exists`, err: exitErr(), code: 5},
		{name: "conflict", stderr: `Error from server (Conflict): the object has been modified`, err: exitErr(), code: 5},
		{name: "server-side apply conflict", stderr: `error: Apply failed with 1 conflict: conflict with "neo4j-cli": .metadata.labels.app`, err: exitErr(), code: 5},
		{name: "forbidden", stderr: `Error from server (Forbidden): namespaces is forbidden`, err: exitErr(), code: 4},
		{name: "unauthorized", stderr: `Error from server (Unauthorized): the server has asked for the client to provide credentials`, err: exitErr(), code: 4},
		{name: "connection refused", stderr: "The connection to the server localhost:8080 was refused", err: exitErr(), code: 8},
		{name: "no such host", stderr: "dial tcp: lookup example.invalid: no such host", err: exitErr(), code: 8},
		{name: "unable to connect", stderr: "Unable to connect to the server: getting credentials: exec plugin", err: exitErr(), code: 8},
		{name: "helm uninstall of a missing release", stderr: `Error: uninstall: Release not loaded: neo4j-operator: release: not found`, err: exitErr(), code: 3},
		{name: "helm upgrade of a missing release", stderr: `Error: UPGRADE FAILED: "neo4j-operator" has no deployed releases`, err: exitErr(), code: 3},
		{name: "unknown shape stays fatal", stderr: "some future error nobody predicted", err: exitErr(), code: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyRunError(ctx, "kubectl", argv, runResult{stderr: tc.stderr, err: tc.err})
			require.Error(t, err)
			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, tc.code, ce.Code, "message: %s", err.Error())
			// The child's own stderr travels with the error so the operator
			// sees the real cause.
			assert.Contains(t, err.Error(), tc.stderr)
		})
	}
}

func TestClassifyRunError_ContextCancellationReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := classifyRunError(ctx, "kubectl", nil, runResult{stderr: "killed", err: exitErr()})
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled), "got %v", err)
}

func TestClassifyRunError_NilErrIsNil(t *testing.T) {
	assert.NoError(t, classifyRunError(context.Background(), "kubectl", nil, runResult{}))
}

func TestIsNotFound(t *testing.T) {
	assert.True(t, IsNotFound(clierr.NewNotFoundError("gone")))
	assert.False(t, IsNotFound(clierr.NewUsageError("bad input")))
	assert.False(t, IsNotFound(fmt.Errorf("plain error")))
}

func TestClassifyRunError_ProcessNeverRanIsUpstream(t *testing.T) {
	err := classifyRunError(context.Background(), "helm", []string{"version"}, runResult{err: fmt.Errorf("fork failed")})
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 8, ce.Code)
}

func TestClassifyRunError_RedactsStderrOnEveryBranch(t *testing.T) {
	// An exec credential plugin can echo a token into kubectl's error. This
	// lands on the upstream branch, not the fatal fallback.
	stderr := "Unable to connect to the server: getting credentials: exec plugin failed, token: abc123"
	err := classifyRunError(context.Background(), "kubectl", nil, runResult{stderr: stderr, err: exitErr()})
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 8, ce.Code)
	assert.NotContains(t, err.Error(), "abc123")
}
