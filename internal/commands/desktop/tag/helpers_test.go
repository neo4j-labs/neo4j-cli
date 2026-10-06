// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package tag_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/desktop"
	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/neo4j/cli/internal/flags"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// validTagID is a well-formed UUID usable as a positional wherever a test
// needs the UUID fast-path (ResolveTagIDs passes UUIDs through without a
// catalog fetch).
const validTagID = "f4e2f3c0-1111-2222-3333-444455556666"

// tagHelper wires `desktop.NewCmd` against an in-memory FS, with the tag
// subtree's `desktopclient.Connect` seam pinned to a desktopclient.Client backed
// by an httptest server. Mirrors the connection package's helper pattern so
// the tag leaves get the same hermetic end-to-end coverage.
type tagHelper struct {
	t   *testing.T
	in  *bytes.Buffer
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newTagHelper(t *testing.T) *tagHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	return &tagHelper{
		t:   t,
		in:  &bytes.Buffer{},
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

// withHandler swaps the tag subtree's client-constructor seam to a closure
// returning a desktopclient.Client wired to the supplied httptest handler.
func (h *tagHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-tag"
		clientID = "cid-tag"
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

func (h *tagHelper) run(command string) error {
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
