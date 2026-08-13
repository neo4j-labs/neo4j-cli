// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/clievents"
	"github.com/neo4j/cli/common/tee"
	"github.com/spf13/afero"
)

// CommandResult is the raw outcome of one in-process CLI invocation: whatever
// the command wrote to cobra's out and err streams, plus the error Execute
// returned. Err keeps its concrete type, so a caller can recover the command's
// *clierr.CLIError (and its exit code) with errors.As — the main reason commands
// run in-process rather than as a child binary.
type CommandResult struct {
	Stdout string
	Stderr string
	Err    error
}

// Executor runs neo4j-cli commands in-process, one at a time, against a cobra
// tree it builds fresh for every call.
//
// In-process rather than re-executing the binary because the arguments come
// from a model: a `--password` in a child's argv is world-readable through
// /proc/<pid>/cmdline, and this repo's rule is that secrets travel in the
// environment, never in argv.
//
// Fresh per call because cobra flag values are sticky: a reused tree would run
// the second call with the first call's --format, --rw and --yes still set. The
// config is rebuilt with the tree because the tree binds its flags into that
// config's viper instance and caches credentials against it. That covers the
// per-config globals (viper, the analytics service) but not the process-global
// ones the trees flip: desktopclient's debug switch survives a call and is only
// safe because the desktop root re-resolves it on every desktop invocation and
// nothing else reads it.
//
// Execute does NOT consult the policy table: deciding whether a path may be
// dispatched at all, and by which tool, belongs to the caller (see Check). One
// classification is load-bearing here rather than there: the lock is not
// reentrant, so a dispatched command that re-entered the executor would wedge
// the server for good. `mcp` being deny-classified in allow.go is what stops
// that.
type Executor struct {
	mu      sync.Mutex
	fs      afero.Fs
	version string
	newRoot RootFactory
}

// NewExecutor returns an Executor that dispatches against trees built by
// newRoot over cfg's filesystem and version. It fails rather than deferring a
// nil dereference to the first tool call when either piece is missing.
func NewExecutor(cfg *clicfg.Config, newRoot RootFactory) (*Executor, error) {
	if err := ValidateWiring(cfg, newRoot); err != nil {
		return nil, err
	}
	return &Executor{
		// AuraConfig.Fs is the only accessor for the filesystem a Config was
		// built over; the per-call configs must share it so the server sees the
		// same config file and credential store as the rest of the CLI.
		fs:      cfg.Aura.Fs(),
		version: cfg.Version,
		newRoot: newRoot,
	}, nil
}

// Execute runs args against a freshly built command tree and returns what it
// produced. Calls are serialised: the tree, its config and the process-wide
// state it touches are not safe to share between concurrent invocations.
//
// Execute never panics and never exits: a panic below it becomes Result.Err.
func (e *Executor) Execute(ctx context.Context, args []string) CommandResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	stdout := &tee.LimitedBuffer{}
	stderr := &tee.LimitedBuffer{}
	err := e.dispatch(ctx, args, stdout, stderr)

	return CommandResult{
		Stdout: string(stdout.Bytes()),
		Stderr: string(stderr.Bytes()),
		Err:    err,
	}
}

// dispatch builds the per-call config and tree, runs args against it, and
// converts a panic into an error the way neo4j-cli's main does.
func (e *Executor) dispatch(ctx context.Context, args []string, stdout, stderr io.Writer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicError(args, r)
		}
	}()

	cfg := clicfg.NewConfig(e.fs, e.version, clicfg.GlobalScope)
	// Every clicfg.NewConfig starts an analytics worker goroutine that only
	// exits once the channel is closed, so a per-call config that is never
	// flushed leaks one goroutine per tool call for the life of the server.
	defer cfg.Events.Flush()

	root := e.newRoot(cfg)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	// Under an MCP stdio transport the real stdin carries the protocol frames,
	// so no command may ever read it: an empty reader makes every prompt see
	// EOF and cancel (common/confirm reads cmd.InOrStdin()) instead of blocking
	// the server forever. Commands that reach for os.Stdin directly are covered
	// by ClaimStdio.
	root.SetIn(bytes.NewReader(nil))

	return root.ExecuteContext(ctx)
}

// panicError converts a recovered panic into an error, mirroring main's
// recoverPanic: an error value contributes its own text, and the arguments are
// redacted because they may carry a model-supplied secret.
//
// The panic value is redacted too. Panics here carry arbitrary runtime state —
// a failed keyring write, an unhandled API status, a Bolt error quoting a
// neo4j://user:pass@host URI — and this is the one string that puts uncontrolled
// state in front of a model.
func panicError(args []string, r any) error {
	detail := fmt.Sprintf("%v", r)
	if err, ok := r.(error); ok {
		detail = err.Error()
	}
	return clierr.NewFatalError(
		"unexpected error running args %s: %s, please report an issue in %s",
		clievents.RedactArgs(args), clievents.RedactText(detail), clierr.IssuesURL)
}

// ValidateWiring reports whether the pieces app.go injects into this group are
// present. Every leaf that dispatches a CLI command needs both, and neither is
// under the user's control, so a missing one is a wiring bug worth naming.
func ValidateWiring(cfg *clicfg.Config, newRoot RootFactory) error {
	if cfg == nil {
		return clierr.NewFatalError("the mcp command group was built without a config, please report an issue in %s", clierr.IssuesURL)
	}
	if newRoot == nil {
		return clierr.NewFatalError("the mcp command group was built without a root command factory, please report an issue in %s", clierr.IssuesURL)
	}
	return nil
}
