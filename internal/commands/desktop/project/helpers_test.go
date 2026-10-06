// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/commands/desktop"
	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/neo4j/cli/internal/flags"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// projectHelper wires `desktop.NewCmd` against an in-memory FS, with the
// project subtree's `desktopclient.Connect` seam pinned to a desktopclient.Client
// backed by an httptest server. Mirrors the helper pattern in the sibling
// `connection` package's tests so the project leaves get the same hermetic
// end-to-end coverage.
type projectHelper struct {
	t   *testing.T
	in  *bytes.Buffer
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newProjectHelper(t *testing.T) *projectHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	return &projectHelper{
		t:   t,
		in:  &bytes.Buffer{},
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

// withHandler swaps the project subtree's client-constructor seam to a
// closure returning a desktopclient.Client wired to the supplied httptest
// handler.
func (h *projectHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-project"
		clientID = "cid-project"
	)
	h.t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return clientID }))
	h.t.Cleanup(desktopclient.SetNowFnForTest(func() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) }))
	srv := httptest.NewServer(handler)
	h.t.Cleanup(srv.Close)
	h.t.Cleanup(desktopclient.SetConnectFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return desktopclient.NewClient(desktopclient.ProbeResult{Origin: srv.URL}, salt)
	}))
	return srv
}

func (h *projectHelper) run(command string) error {
	h.t.Helper()
	args, err := shlex.Split(command)
	if err != nil {
		h.t.Fatalf("shlex: %v", err)
	}
	cfg := clicfg.NewConfig(h.fs, "test")
	cmd := desktop.NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetArgs(args)
	cmd.SetIn(h.in)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)
	return cmd.Execute()
}

// mustTestFs builds the in-memory FS for tests that only inspect the cobra
// tree (no execution).
func mustTestFs(t *testing.T) afero.Fs {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	return fs
}

// findProjectLeaf locates one leaf under the `project` subtree of a freshly
// built desktop command.
func findProjectLeaf(t *testing.T, leafName string) *cobra.Command {
	t.Helper()
	cfg := clicfg.NewConfig(mustTestFs(t), "test")
	parent := desktop.NewCmd(cfg)
	for _, c := range parent.Commands() {
		if c.Name() != "project" {
			continue
		}
		for _, sub := range c.Commands() {
			if sub.Name() == leafName {
				return sub
			}
		}
	}
	t.Fatalf("desktop project %s command not registered", leafName)
	return nil
}

// wantCreatedAt is the unix-ms fixture timestamp used across the project
// tests; wantCreatedAtRFC3339 is the RFC3339 UTC rendering the CLI must print.
const wantCreatedAt = 1747843200000

func wantCreatedAtRFC3339() string {
	return time.UnixMilli(wantCreatedAt).UTC().Format(time.RFC3339)
}

// assertFatalCatalogError pins the create/update finder policy: a mutated
// entry that cannot be identified in Desktop's returned catalog state is a
// clierr FATAL error (exit 1) carrying each wanted substring — never a
// silently printed `null` with exit 0.
func assertFatalCatalogError(t *testing.T, err error, substrings ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected fatal error, got nil")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != 1 {
		t.Fatalf("error = %v (%T), want clierr fatal error (exit 1)", err, err)
	}
	for _, s := range substrings {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("error = %q, want substring %q", err.Error(), s)
		}
	}
}
