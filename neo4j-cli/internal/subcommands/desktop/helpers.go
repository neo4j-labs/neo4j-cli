// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktop

import (
	"context"
	"errors"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/afero"
)

// newDesktopClient probes the port range, resolves the data dir, loads the
// salt, and signs a JWT. A probe miss or missing/unreadable salt both surface
// as the canonical "Desktop unreachable" message — a missing salt means
// Desktop has not yet finished first-run auth setup.
var newDesktopClientFn = newDesktopClient

func newDesktopClient(ctx context.Context, fs afero.Fs, port int) (*desktopclient.Client, error) {
	// ProbePort runs first so its origin can be threaded into ResolveDataDir
	// for the /info/app discovery step.
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
		// Missing/unreadable salt = Desktop has not finished first-run auth
		// setup. Route to the same unreachable error as a probe miss.
		return nil, desktopclient.UnreachableError()
	}
	return desktopclient.NewClient(probe, salt)
}

// SetNewDesktopClientFnForTest overrides the desktop client constructor.
func SetNewDesktopClientFnForTest(fn func(context.Context, afero.Fs, int) (*desktopclient.Client, error)) func() {
	prev := newDesktopClientFn
	newDesktopClientFn = fn
	return func() { newDesktopClientFn = prev }
}
