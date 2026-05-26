// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package connection_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/connection"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

type listHelper struct {
	t   *testing.T
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newListHelper(t *testing.T) *listHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	return &listHelper{
		t:   t,
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

func (h *listHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-conn-list"
		clientID = "cid-conn-list"
	)
	h.t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return clientID }))
	h.t.Cleanup(desktopclient.SetNowFnForTest(func() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) }))
	srv := httptest.NewServer(handler)
	h.t.Cleanup(srv.Close)
	h.t.Cleanup(connection.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return desktopclient.NewClient(desktopclient.ProbeResult{Origin: srv.URL}, salt)
	}))
	return srv
}

func (h *listHelper) run(command string) error {
	h.t.Helper()
	args, err := shlex.Split(command)
	if err != nil {
		h.t.Fatalf("shlex: %v", err)
	}
	cfg := clicfg.NewConfig(h.fs, "test", clicfg.GlobalScope)
	cmd := desktop.NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetArgs(args)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)
	return cmd.Execute()
}

func TestConnectionList_JSON_RendersConnectionArray(t *testing.T) {
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/fastify/api/connections" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			return
		}
		_, _ = w.Write([]byte(`[
			{"id":"c1","name":"aura-dev","connectionUri":"neo4j+s://abc.databases.neo4j.io"},
			{"id":"c2","name":"self-hosted","connectionUri":"neo4j://10.0.0.1:7687"}
		]`))
	})
	if err := h.run("connection list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 connections, got %d (raw: %s)", len(got), h.out.String())
	}
	if got[0]["id"] != "c1" || got[1]["id"] != "c2" {
		t.Fatalf("expected ids [c1 c2], got %v", got)
	}
}

func TestConnectionList_EmptyJSON_RendersEmptyArrayNotNull(t *testing.T) {
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	if err := h.run("connection list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	trimmed := strings.TrimSpace(h.out.String())
	if trimmed != "[]" {
		t.Fatalf("expected `[]` for empty connection list, got %q", trimmed)
	}
}

func TestConnectionList_EmptyTable_RendersNonePlaceholder(t *testing.T) {
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	if err := h.run("connection list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	if !strings.Contains(out, "(none)") {
		t.Fatalf("expected `(none)` placeholder in empty table, got:\n%s", out)
	}
	// Column headers must still appear so the user knows what they're looking
	// at even with no rows.
	for _, col := range []string{"ID", "NAME", "CONNECTION"} {
		if !strings.Contains(strings.ToUpper(out), col) {
			t.Fatalf("expected column %q in header, got:\n%s", col, out)
		}
	}
}

func TestConnectionList_5xx_SurfacesError(t *testing.T) {
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("oops"))
	})
	err := h.run("connection list --format json")
	if err == nil {
		t.Fatalf("expected error on 5xx")
	}
	if !strings.Contains(err.Error(), "Neo4j Desktop 2") {
		t.Fatalf("expected Desktop-flavoured error text, got: %v", err)
	}
}

func TestConnectionList_PortFlagPropagatesToClientConstructor(t *testing.T) {
	h := newListHelper(t)
	var seenPort int
	t.Cleanup(connection.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		seenPort = port
		return nil, errors.New("stop here; we already captured the port")
	}))
	_ = h.run("connection list --port 44225 --format json")
	if seenPort != 44225 {
		t.Fatalf("expected --port=44225 to reach the client constructor, got %d", seenPort)
	}
}
