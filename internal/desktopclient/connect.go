// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"context"
	"errors"

	"github.com/spf13/afero"

	"github.com/neo4j/cli/internal/clierr"
)

// connectFn is the single test seam for Connect, shared by every `desktop`
// subcommand package (they previously each kept a private copy).
var connectFn = connect

// SetConnectFnForTest overrides Connect for tests and returns a restore func.
func SetConnectFnForTest(fn func(context.Context, afero.Fs, int) (*Client, error)) func() {
	prev := connectFn
	connectFn = fn
	return func() { connectFn = prev }
}

// Connect probes for a running Desktop, resolves its data dir, loads the
// relate salt, and returns an authenticated Client. A probe miss or a
// missing/unreadable salt (Desktop has not finished first-run auth setup)
// both surface as the canonical "Desktop unreachable" error.
func Connect(ctx context.Context, fs afero.Fs, port int) (*Client, error) {
	return connectFn(ctx, fs, port)
}

func connect(ctx context.Context, fs afero.Fs, port int) (*Client, error) {
	// Discover runs first so its origin can be threaded into ResolveDataDir
	// for the /info/app discovery step.
	probe, err := Discover(ctx, port)
	if err != nil {
		if errors.Is(err, ErrNoDesktop) {
			return nil, UnreachableError()
		}
		return nil, clierr.NewFatalError("desktop: probe failed: %s", err.Error())
	}
	dataDir, err := ResolveDataDir(ctx, fs, probe)
	if err != nil {
		return nil, clierr.NewFatalError("desktop: could not resolve relate data dir: %s", err.Error())
	}
	salt, err := LoadSalt(fs, dataDir)
	if err != nil {
		// Missing/unreadable salt = Desktop has not finished first-run auth
		// setup. Route to the same unreachable error as a probe miss.
		return nil, UnreachableError()
	}
	return NewClient(probe, salt)
}
