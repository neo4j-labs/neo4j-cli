// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp_test

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/app"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jsonRPCFrame stands in for the traffic an MCP client exchanges with the
// server over stdio.
const jsonRPCFrame = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"

func TestClaimStdio_RedirectsTheProcessStreams(t *testing.T) {
	realIn, realOut := os.Stdin, os.Stdout

	in, out, restore, err := mcp.ClaimStdio()
	require.NoError(t, err)
	t.Cleanup(restore)

	assert.Same(t, realIn, in, "the transport must be handed the process's real stdin")
	assert.Same(t, realOut, out, "the transport must be handed the process's real stdout")
	assert.Same(t, os.Stderr, os.Stdout, "a stray write to os.Stdout must land on stderr")

	n, err := os.Stdin.Read(make([]byte, 1))
	assert.Zero(t, n)
	assert.ErrorIs(t, err, io.EOF, "os.Stdin must read EOF, not the protocol stream")

	restore()
	assert.Same(t, realIn, os.Stdin)
	assert.Same(t, realOut, os.Stdout)

	restore()
	assert.Same(t, realIn, os.Stdin, "restore must be safe to call twice")
}

// TestClaimStdio_RefusesASecondClaim covers the guard: serving a second claim
// would hand out /dev/null and stderr as if they were the client's streams.
func TestClaimStdio_RefusesASecondClaim(t *testing.T) {
	_, _, restore, err := mcp.ClaimStdio()
	require.NoError(t, err)
	t.Cleanup(restore)

	in, out, again, err := mcp.ClaimStdio()
	require.Error(t, err)
	assert.Nil(t, in)
	assert.Nil(t, out)
	assert.Nil(t, again)
	assert.Contains(t, err.Error(), "already claimed")

	restore()

	_, _, restoreAgain, err := mcp.ClaimStdio()
	require.NoError(t, err, "a restored process must be claimable again")
	restoreAgain()
}

// TestClaimStdio_StrayStdoutWriteCannotCorruptTheFrame is the whole point of the
// swap: a command that writes to os.Stdout directly — a stray Println, a
// shelled-out docker inheriting the stream — would otherwise interleave with
// the JSON-RPC frames and desynchronise the client.
func TestClaimStdio_StrayStdoutWriteCannotCorruptTheFrame(t *testing.T) {
	transport := redirectProcessStream(t, &os.Stdout, "transport")
	strays := redirectProcessStream(t, &os.Stderr, "strays")

	_, out, restore, err := mcp.ClaimStdio()
	require.NoError(t, err)
	t.Cleanup(restore)
	require.Same(t, transport, out)

	exec := newExecutor(t, stubFactory(func() *cobra.Command {
		return &cobra.Command{
			Use: "noisy",
			RunE: func(cmd *cobra.Command, _ []string) error {
				fmt.Fprintln(os.Stdout, "stray write")            //nolint:errcheck // test fixture
				fmt.Fprintln(cmd.OutOrStdout(), "command output") //nolint:errcheck // test fixture
				return nil
			},
		}
	}))

	res := executeWithin(t, exec, "noisy")
	require.NoError(t, res.Err)
	assert.Contains(t, res.Stdout, "command output", "cobra's out stream must be captured for the caller")

	_, err = out.WriteString(jsonRPCFrame)
	require.NoError(t, err)

	assert.Equal(t, jsonRPCFrame, readProcessStream(t, transport), "nothing but the frame may reach the transport")
	assert.Contains(t, readProcessStream(t, strays), "stray write", "the stray write must be diverted to stderr")
}

// TestClaimStdio_CommandCannotConsumeTheProtocolStream covers the other half:
// `query` with no positional argument falls through to io.ReadAll(os.Stdin), so
// without the swap it would drain the client's frames and hang the server
// waiting for more.
func TestClaimStdio_CommandCannotConsumeTheProtocolStream(t *testing.T) {
	client, server, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	prev := os.Stdin
	os.Stdin = client
	t.Cleanup(func() { os.Stdin = prev })

	in, _, restore, err := mcp.ClaimStdio()
	require.NoError(t, err)
	t.Cleanup(restore)
	require.Same(t, client, in)

	_, err = server.WriteString(jsonRPCFrame)
	require.NoError(t, err)

	exec := newExecutor(t, app.NewCmd)
	res := executeWithin(t, exec, "query")
	require.Error(t, res.Err, "query with no Cypher and an empty stdin must fail, not block")
	assert.Contains(t, res.Err.Error(), "no Cypher provided")

	require.NoError(t, server.Close())
	unread, err := io.ReadAll(in)
	require.NoError(t, err)
	assert.Equal(t, jsonRPCFrame, string(unread), "the command must not have consumed the frame")
}

// redirectProcessStream points one of the os stream variables at a temp file for
// the duration of the test, standing in for the pipes an MCP client hands the
// server, and restores it afterwards.
func redirectProcessStream(t *testing.T, stream **os.File, name string) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), name)
	require.NoError(t, err)

	prev := *stream
	*stream = f
	t.Cleanup(func() {
		*stream = prev
		_ = f.Close()
	})
	return f
}

// readProcessStream returns everything written to a file from
// redirectProcessStream.
func readProcessStream(t *testing.T, f *os.File) string {
	t.Helper()
	content, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	return string(content)
}
