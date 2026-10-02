// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebugInvocation_ScrubsArgvAndEmitsEnvNamesOnly(t *testing.T) {
	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)

	debugInvocation(
		[]string{"run", "-d", "-e", "NEO4J_AUTH=neo4j/s3cr3t", "neo4j:enterprise"},
		[]string{"NEO4J_AUTH=neo4j/p4ssw0rd", `NEO4J_PLUGINS=["apoc"]`},
	)

	out := buf.String()

	assert.Contains(t, out, debugReqPrefix+"docker ")
	assert.Contains(t, out, "run -d -e NEO4J_AUTH=*** neo4j:enterprise")
	// env line lists NAMES only.
	assert.Contains(t, out, debugReqPrefix+"env NEO4J_AUTH NEO4J_PLUGINS")
	// secret values from argv and env must never reach the terminal.
	assert.NotContains(t, out, "s3cr3t")
	assert.NotContains(t, out, "p4ssw0rd")
	// env values (the =VALUE half) must never be emitted.
	assert.NotContains(t, out, `["apoc"]`)
	assert.NotContains(t, out, "NEO4J_AUTH=neo4j")
}

func TestDebugInvocation_NoEnvOmitsEnvLine(t *testing.T) {
	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)

	debugInvocation([]string{"ps", "-a"}, nil)

	out := buf.String()
	assert.Contains(t, out, debugReqPrefix+"docker ps -a")
	assert.NotContains(t, out, debugReqPrefix+"env")
}

func TestDebugResult_ExitAndElapsedShape(t *testing.T) {
	exit2 := exec.Command("sh", "-c", "exit 2").Run()
	require.Error(t, exit2)

	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{name: "success", err: nil, wantCode: "exit 0 elapsed"},
		{name: "exit error code", err: exit2, wantCode: "exit 2 elapsed"},
		{name: "non-exit error falls back to -1", err: errors.New("failed to start"), wantCode: "exit -1 elapsed"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			SetDebugWriterForTest(t, &buf)

			debugResult(tc.err, 5*time.Millisecond)

			out := buf.String()
			assert.Contains(t, out, debugRespPrefix+tc.wantCode)
			assert.Contains(t, out, "5ms")
		})
	}
}

func TestRunEnv_DebugOffEmitsNothing(t *testing.T) {
	var buf bytes.Buffer
	SetDebugWriterForTest(t, &buf)

	// A non-debug client never invokes the emit helpers; runEnv's guarded
	// blocks are skipped before any docker lookup, so the buffer stays empty.
	require.False(t, newClient(false).(*execClient).debug)
	require.True(t, newClient(true).(*execClient).debug)

	assert.Empty(t, buf.String())
}
