// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
	"github.com/neo4j/cli/neo4j-cli/app"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/mcp/server"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExecutor_RequiresBothInjectedPieces(t *testing.T) {
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	for _, tc := range []struct {
		name    string
		cfg     *clicfg.Config
		newRoot server.RootFactory
		want    string
	}{
		{name: "no config", newRoot: app.NewCmd, want: "without a config"},
		{name: "no root factory", cfg: cfg, want: "without a root command factory"},
		{name: "both present", cfg: cfg, newRoot: app.NewCmd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec, err := server.NewExecutor(tc.cfg, tc.newRoot)
			if tc.want == "" {
				require.NoError(t, err)
				assert.NotNil(t, exec)
				return
			}
			require.Error(t, err)
			assert.Nil(t, exec)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestMCPGroup_NilRootFactoryIsRefused covers the wiring guard on the group:
// only app.go supplies the factory, so a missing one is a bug to report rather
// than a nil dereference inside a tool call.
func TestMCPGroup_NilRootFactoryIsRefused(t *testing.T) {
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	cmd := mcp.NewCmd(cfg, nil)
	cmd.SetArgs([]string{"tool"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without a root command factory")
}

func TestExecutor_RunsCommandAndCapturesStdout(t *testing.T) {
	exec := newExecutor(t, app.NewCmd)

	res := executeWithin(t, exec, "config", "list", "--format", "json")
	require.NoError(t, res.Err, "stderr=%s", res.Stderr)
	assert.True(t, json.Valid([]byte(res.Stdout)), "stdout must be the command's own output: %s", res.Stdout)
}

func TestExecutor_ReportsCommandErrorWithItsType(t *testing.T) {
	exec := newExecutor(t, app.NewCmd)

	res := executeWithin(t, exec, "config", "get", "not-a-config-key")
	require.Error(t, res.Err)

	var ce *clierr.CLIError
	require.True(t, errors.As(res.Err, &ce), "the typed CLIError must survive in-process dispatch: %T", res.Err)
	assert.Equal(t, 2, ce.Code, "an invalid config key is a usage error")
}

// TestExecutor_BuildsFreshStatePerCall pins the reason commands are dispatched
// against a new tree and config every time: cobra flag values are sticky, so a
// reused tree renders the second call with the first call's --format (verified
// by reusing one), and the config that tree's flags are bound into has to be
// rebuilt with it.
func TestExecutor_BuildsFreshStatePerCall(t *testing.T) {
	var (
		configs []*clicfg.Config
		roots   []*cobra.Command
	)
	exec := newExecutor(t, func(cfg *clicfg.Config) *cobra.Command {
		configs = append(configs, cfg)
		root := app.NewCmd(cfg)
		roots = append(roots, root)
		return root
	})

	tabular := executeWithin(t, exec, "config", "list", "--format", "table")
	require.NoError(t, tabular.Err, "stderr=%s", tabular.Stderr)
	require.False(t, json.Valid([]byte(tabular.Stdout)), "--format table must not produce JSON: %s", tabular.Stdout)

	// No --format: resolution falls back to json for a non-terminal stdout. A
	// leaked viper binding would render a table here instead.
	defaulted := executeWithin(t, exec, "config", "list")
	require.NoError(t, defaulted.Err, "stderr=%s", defaulted.Stderr)
	assert.True(t, json.Valid([]byte(defaulted.Stdout)), "the previous call's --format leaked: %s", defaulted.Stdout)

	require.Len(t, configs, 2)
	assert.NotSame(t, configs[0], configs[1], "each call must get its own config")
	require.Len(t, roots, 2)
	assert.NotSame(t, roots[0], roots[1], "each call must get its own tree")
}

// TestExecutor_ConfirmPromptCancelsOnEOF covers the per-call SetIn half of the
// stdin protection, independently of the process-wide swap: a destructive leaf
// that decides to prompt must see EOF and cancel rather than read whatever
// happens to be on the process's stdin. The pipe holds a "y" the prompt would
// happily accept, so this fails if the per-call reader is dropped.
func TestExecutor_ConfirmPromptCancelsOnEOF(t *testing.T) {
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return true }))

	answer, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = answer.Close()
		_ = writer.Close()
	})
	_, err = writer.WriteString("y\n")
	require.NoError(t, err)

	prev := os.Stdin
	os.Stdin = answer
	t.Cleanup(func() { os.Stdin = prev })

	exec := newExecutor(t, app.NewCmd)

	res := executeWithin(t, exec, "credential", "dbms", "remove", "absent", "--rw")
	require.Error(t, res.Err)
	assert.True(t, errors.Is(res.Err, confirm.ErrCancelled), "want ErrCancelled, got %v", res.Err)
	assert.Contains(t, res.Stderr, "cancelled.")

	require.NoError(t, writer.Close())
	unread, err := io.ReadAll(answer)
	require.NoError(t, err)
	assert.Equal(t, "y\n", string(unread), "the prompt must not have read the process's stdin")
}

func TestExecutor_PanicBecomesError(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   any
		wantIn  string
		wantOut string
	}{
		{name: "error value", value: errors.New("kaboom"), wantIn: "kaboom"},
		{name: "string value", value: "kaboom", wantIn: "kaboom"},
		{
			// A panic carries arbitrary runtime state, which is the one thing
			// here the model sees without the CLI having chosen the wording.
			name:    "secret inside the panic value",
			value:   errors.New("write failed for bolt://neo4j:hunter3@localhost:7687"),
			wantIn:  "bolt://neo4j:***@localhost:7687",
			wantOut: "hunter3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newExecutor(t, stubFactory(func() *cobra.Command {
				leaf := &cobra.Command{
					Use:  "boom",
					RunE: func(*cobra.Command, []string) error { panic(tc.value) },
				}
				leaf.Flags().String("password", "", "")
				return leaf
			}))

			res := executeWithin(t, exec, "boom", "--password", "hunter2")
			require.Error(t, res.Err)
			assert.Contains(t, res.Err.Error(), tc.wantIn)
			assert.Contains(t, res.Err.Error(), "--password ***")
			assert.NotContains(t, res.Err.Error(), "hunter2", "a model-supplied secret must not be echoed back")
			if tc.wantOut != "" {
				assert.NotContains(t, res.Err.Error(), tc.wantOut)
			}

			var ce *clierr.CLIError
			assert.True(t, errors.As(res.Err, &ce), "a panic must surface as a typed error")
		})
	}
}

// TestExecutor_CancellationReachesTheCommand pins that the caller's context is
// dispatched with. It matters more than usual here: Execute holds the lock for
// the whole call, so a client cancelling a long query is the only thing that
// frees the executor for the next tool call.
func TestExecutor_CancellationReachesTheCommand(t *testing.T) {
	started := make(chan struct{})
	exec := newExecutor(t, stubFactory(func() *cobra.Command {
		return &cobra.Command{
			Use: "block",
			RunE: func(cmd *cobra.Command, _ []string) error {
				close(started)
				<-cmd.Context().Done()
				return cmd.Context().Err()
			},
		}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-started
		cancel()
	}()

	res := executeCtxWithin(t, ctx, exec, "block")
	require.Error(t, res.Err)
	assert.ErrorIs(t, res.Err, context.Canceled)
}

// TestExecutor_SerialisesCalls asserts the mutex actually excludes: the tree,
// its config and the process state it touches are not safe to share.
func TestExecutor_SerialisesCalls(t *testing.T) {
	var (
		mu       sync.Mutex
		inFlight int
		overlaps int
	)
	exec := newExecutor(t, stubFactory(func() *cobra.Command {
		return &cobra.Command{
			Use: "slow",
			RunE: func(*cobra.Command, []string) error {
				mu.Lock()
				inFlight++
				if inFlight > 1 {
					overlaps++
				}
				mu.Unlock()
				defer func() {
					mu.Lock()
					inFlight--
					mu.Unlock()
				}()
				for i := 0; i < 1000; i++ {
					runtime.Gosched()
				}
				return nil
			},
		}
	}))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, exec.Execute(context.Background(), []string{"slow"}).Err)
		}()
	}
	waitWithin(t, "8 concurrent executor calls", wg.Wait)

	mu.Lock()
	defer mu.Unlock()
	assert.Zero(t, overlaps, "calls must not overlap")
}

// TestExecutor_LeavesProcessStdioAlone keeps the process-wide swap where it
// belongs: ClaimStdio does it once when a server starts, so a per-call
// executor must not touch the globals (and must not be usable to break an
// unrelated caller's streams).
func TestExecutor_LeavesProcessStdioAlone(t *testing.T) {
	stdin, stdout, stderr := os.Stdin, os.Stdout, os.Stderr

	exec := newExecutor(t, app.NewCmd)
	require.NoError(t, executeWithin(t, exec, "config", "list", "--format", "json").Err)

	assert.Same(t, stdin, os.Stdin)
	assert.Same(t, stdout, os.Stdout)
	assert.Same(t, stderr, os.Stderr)
}
