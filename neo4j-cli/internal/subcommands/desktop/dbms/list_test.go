// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms_test

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
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/dbms"
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
		salt     = "salt-dbms-list"
		clientID = "cid-dbms-list"
	)
	h.t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return clientID }))
	h.t.Cleanup(desktopclient.SetNowFnForTest(func() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) }))
	srv := httptest.NewServer(handler)
	h.t.Cleanup(srv.Close)
	h.t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
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

func TestDbmsList_JSON_RendersEnrichedDbmsArray(t *testing.T) {
	// `desktop dbms list --format json` lists DBMSes via GET /dbmss (slim
	// shape) then fans out one GET /dbmss/:id per entry to enrich the rows
	// (status, version, edition). The rendered JSON array carries the
	// enriched fields.
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[
				{"id":"a","name":"alpha","connectionUri":"neo4j://localhost:7687"},
				{"id":"b","name":"beta","connectionUri":"neo4j://localhost:7688"}
			]`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/a":
			_, _ = w.Write([]byte(`{"id":"a","name":"alpha","version":"5.20.0","status":"started","connectionUri":"neo4j://localhost:7687"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/b":
			_, _ = w.Write([]byte(`{"id":"b","name":"beta","version":"2026.04.0","status":"stopped","connectionUri":"neo4j://localhost:7688"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	if err := h.run("dbms list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 dbmss, got %d (raw: %s)", len(got), h.out.String())
	}
	if got[0]["id"] != "a" || got[0]["version"] != "5.20.0" || got[0]["status"] != "started" {
		t.Fatalf("got[0] missing enriched fields: %v", got[0])
	}
	if got[1]["id"] != "b" || got[1]["version"] != "2026.04.0" || got[1]["status"] != "stopped" {
		t.Fatalf("got[1] missing enriched fields: %v", got[1])
	}
}

func TestDbmsList_EmptyJSON_RendersEmptyArrayNotNull(t *testing.T) {
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	if err := h.run("dbms list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	trimmed := strings.TrimSpace(h.out.String())
	if trimmed != "[]" {
		t.Fatalf("expected `[]` for empty dbms list, got %q", trimmed)
	}
}

func TestDbmsList_EmptyTable_RendersNonePlaceholder(t *testing.T) {
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	if err := h.run("dbms list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	if !strings.Contains(out, "(none)") {
		t.Fatalf("expected `(none)` placeholder in empty table, got:\n%s", out)
	}
	for _, col := range []string{"ID", "NAME", "VERSION", "STATUS"} {
		if !strings.Contains(strings.ToUpper(out), col) {
			t.Fatalf("expected column %q in header, got:\n%s", col, out)
		}
	}
}

func TestDbmsList_5xx_SurfacesError(t *testing.T) {
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fastify/api/dbmss" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("oops"))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})
	err := h.run("dbms list --format json")
	if err == nil {
		t.Fatalf("expected error on 5xx from ListDbmss")
	}
	if !strings.Contains(err.Error(), "Neo4j Desktop 2") {
		t.Fatalf("expected Desktop-flavoured error text, got: %v", err)
	}
}

func TestDbmsList_PortFlagPropagatesToClientConstructor(t *testing.T) {
	h := newListHelper(t)
	var seenPort int
	t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		seenPort = port
		return nil, errors.New("stop here; we already captured the port")
	}))
	_ = h.run("dbms list --port 44230 --format json")
	if seenPort != 44230 {
		t.Fatalf("expected --port=44230 to reach the client constructor, got %d", seenPort)
	}
}
