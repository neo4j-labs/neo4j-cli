// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/afero"
)

// newDesktopClientFn is the per-subtree test seam. Each leaf-subtree under `desktop`
// keeps its own so a test pinning one can't accidentally override another's.
var newDesktopClientFn = newDesktopClient

func newDesktopClient(ctx context.Context, fs afero.Fs, port int) (*desktopclient.Client, error) {
	// ProbePort runs first so its origin can be threaded into ResolveDataDir.
	probe, err := desktopclient.ProbePort(ctx, port)
	if err != nil {
		if errors.Is(err, desktopclient.ErrNoDesktop) {
			return nil, desktopclient.UnreachableError()
		}
		return nil, clierr.NewFatalError("desktop: probe failed: %s", err.Error())
	}
	dataDir, err := desktopclient.ResolveDataDir(ctx, fs, probe)
	if err != nil {
		return nil, clierr.NewFatalError("desktop: could not resolve relate data dir: %s", err.Error())
	}
	salt, err := desktopclient.LoadSalt(fs, dataDir)
	if err != nil {
		// Missing/unreadable salt = Desktop has not finished first-run auth setup.
		// Route to the same canonical hint as a probe miss.
		return nil, desktopclient.UnreachableError()
	}
	return desktopclient.NewClient(probe, salt)
}

// SetNewDesktopClientFnForTest overrides newDesktopClientFn for a test; returns a restore func.
func SetNewDesktopClientFnForTest(fn func(context.Context, afero.Fs, int) (*desktopclient.Client, error)) func() {
	prev := newDesktopClientFn
	newDesktopClientFn = fn
	return func() { newDesktopClientFn = prev }
}

// Mirrors the live Status strings Desktop reports — do NOT reintroduce
// `online`/`offline` aliases. Duplicated here because importing the sibling
// `dbms` package would create a cycle (it mounts `plugin.NewCmd`).
const (
	dbmsStatusStarted = "started"
	dbmsStatusStopped = "stopped"
)

const (
	pollInterval = 1 * time.Second
	pollTimeout  = 30 * time.Second
)

var pollSleepFn = func(d time.Duration) { time.Sleep(d) }

// SetPollSleepFnForTest overrides pollSleepFn for a test; returns a restore func.
func SetPollSleepFnForTest(fn func(time.Duration)) func() {
	prev := pollSleepFn
	pollSleepFn = fn
	return func() { pollSleepFn = prev }
}

var pollNowFn = func() time.Time { return time.Now() }

// SetPollNowFnForTest overrides pollNowFn for a test; returns a restore func.
func SetPollNowFnForTest(fn func() time.Time) func() {
	prev := pollNowFn
	pollNowFn = fn
	return func() { pollNowFn = prev }
}

// restartClient is the subset of `*desktopclient.Client` that `autoRestartIfRunning` needs.
type restartClient interface {
	StopDbms(ctx context.Context, id string) error
	StartDbms(ctx context.Context, id string) error
	GetDbms(ctx context.Context, id string) (*desktopclient.DbmsInfo, error)
}

// autoRestartIfRunning issues Stop → poll(stopped) → Start → poll(started) so a JAR
// change takes effect (the running JVM does not pick up plugin JARs without a restart).
// `effectVerb` is the past-tense verb describing the post-restart plugin state — install
// passes "active", uninstall passes "removed".
//
// On Stop/Start failure or poll timeout returns a `*restartErr` so the caller can downgrade
// to a stderr warning + exit 0; the plugin op itself MUST NOT be rolled back.
func autoRestartIfRunning(ctx context.Context, client restartClient, dbmsName, dbmsID, pluginName, effectVerb string, warnOut io.Writer) error {
	ref := dbmsRef(dbmsName, dbmsID)
	_, _ = fmt.Fprintf(warnOut, "Plugin change pending — restarting DBMS %s to apply...\n", ref)

	if err := client.StopDbms(ctx, dbmsID); err != nil {
		return &restartErr{op: "stop", ref: ref, err: err}
	}
	if _, err := pollUntilStatus(ctx, client, dbmsID, dbmsStatusStopped); err != nil {
		return &restartErr{op: "stop", ref: ref, err: err}
	}
	if err := client.StartDbms(ctx, dbmsID); err != nil {
		return &restartErr{op: "start", ref: ref, err: err}
	}
	if _, err := pollUntilStatus(ctx, client, dbmsID, dbmsStatusStarted); err != nil {
		return &restartErr{op: "start", ref: ref, err: err}
	}

	_, _ = fmt.Fprintf(warnOut, "DBMS restarted; plugin %q is now %s.\n", pluginName, effectVerb)
	return nil
}

// restartErr distinguishes auto-restart-only failures from primary plugin-op failures.
type restartErr struct {
	op  string // "stop" or "start"
	ref string // formatted DBMS reference for the user-facing message
	err error
}

func (e *restartErr) Error() string {
	return fmt.Sprintf("auto-restart %s of DBMS %s failed: %s", e.op, e.ref, e.err.Error())
}

// pollUntilStatus polls `GetDbms` every `pollInterval` up to `pollTimeout` for `target`.
// On timeout returns a fatal error carrying the last-seen status. Duplicated from the
// sibling `dbms` package to avoid an import cycle.
func pollUntilStatus(ctx context.Context, client restartClient, id, target string) (*desktopclient.DbmsInfo, error) {
	deadline := pollNowFn().Add(pollTimeout)
	var lastStatus string
	for {
		info, err := client.GetDbms(ctx, id)
		if err != nil {
			return nil, err
		}
		if info.Status == target {
			return info, nil
		}
		lastStatus = info.Status
		if pollNowFn().After(deadline) {
			return nil, clierr.NewFatalError(
				"timed out after %s waiting for DBMS %q to reach status %q (last status: %q). "+
					"Re-run with --no-restart to skip the auto-restart, or check `neo4j-cli desktop dbms list` for the current status.",
				pollTimeout, id, target, lastStatus)
		}
		if err := ctxSleep(ctx, pollInterval); err != nil {
			return nil, err
		}
	}
}

// ctxSleep is a cancel-aware sleep routed through `pollSleepFn`.
func ctxSleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		pollSleepFn(d)
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

// dbmsRef formats a DBMS reference as `"<name>" (<id>)` or `"<id>"` when name is empty.
func dbmsRef(name, id string) string {
	if name == "" {
		return fmt.Sprintf("%q", id)
	}
	return fmt.Sprintf("%q (%s)", name, id)
}
