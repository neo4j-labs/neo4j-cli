// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package plugin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/dbms"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/dbms/plugin"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// listHelper wires dbms.NewCmd (which mounts the plugin subtree) against an
// in-memory FS, with the plugin package's `newDesktopClientFn` seam pinned to
// a desktopclient.Client backed by an httptest server. Mirrors the
// startHelper / connectionListHelper shape — each leaf-subtree owns its own
// test surface so the seams stay isolated.
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
		salt     = "salt-plugin-list"
		clientID = "cid-plugin-list"
	)
	h.t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return clientID }))
	h.t.Cleanup(desktopclient.SetNowFnForTest(func() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) }))
	srv := httptest.NewServer(handler)
	h.t.Cleanup(srv.Close)

	h.t.Cleanup(plugin.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
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
	cmd := dbms.NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetArgs(args)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)
	return cmd.Execute()
}

func TestPluginList_RequiresDbmsID(t *testing.T) {
	h := newListHelper(t)
	err := h.run("plugin list")
	if err == nil {
		t.Fatalf("expected error when <dbms-id> is missing")
	}
}

func TestPluginList_Happy_JSON(t *testing.T) {
	// Happy path: relate returns a populated installed-plugin list; --format
	// json renders the full DbmsPlugin array.
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc/plugins/installed" {
			_, _ = w.Write([]byte(`[{"name":"apoc","version":"5.20.0","filePath":"/x/apoc.jar","pendingRestart":false},{"name":"gds","version":"2.10.0","filePath":"/x/gds.jar","pendingRestart":true}]`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("plugin list abc --format json"); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, h.err.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 plugins, got %d (%v)", len(got), got)
	}
	if got[0]["name"] != "apoc" || got[1]["name"] != "gds" {
		t.Fatalf("unexpected plugin names: %v / %v", got[0]["name"], got[1]["name"])
	}
	if got[1]["pending_restart"] != true {
		t.Fatalf("expected pending_restart=true on gds, got %v", got[1]["pending_restart"])
	}
}

func TestPluginList_Happy_Table(t *testing.T) {
	// Default table format renders the column headers + one row per plugin.
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc/plugins/installed" {
			_, _ = w.Write([]byte(`[{"name":"apoc","version":"5.20.0","filePath":"/x/apoc.jar","pendingRestart":false}]`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("plugin list abc --format table"); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, h.err.String())
	}
	out := h.out.String()
	// jedib0t/go-pretty uppercases column headers by default; the leaf
	// registers them lower-case, the renderer casts them at print time.
	for _, want := range []string{"NAME", "VERSION", "PENDING_RESTART", "FILE_PATH", "apoc", "5.20.0", "/x/apoc.jar"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestPluginList_Empty_RendersEmptyTable(t *testing.T) {
	// Empty installed list: relate returns []. Table still shows headers +
	// the `(none)` placeholder row so users see the columns.
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc/plugins/installed" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("plugin list abc --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	for _, want := range []string{"NAME", "VERSION", "PENDING_RESTART", "FILE_PATH", "(none)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected empty-table output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestPluginList_Empty_JSONReturnsEmptyArray(t *testing.T) {
	// Empty installed list: JSON consumers always see `[]`, never `null`.
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc/plugins/installed" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("plugin list abc --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(h.out.String())
	if got != "[]" {
		t.Fatalf("expected json [] for empty plugin list, got %q", got)
	}
}

func TestPluginList_DbmsNotFound_SurfacesCanonicalHint(t *testing.T) {
	// Relate's 404 body whose `message` contains "Could not find DBMS" maps
	// to desktopclient.ErrDbmsNotFound; the leaf translates that into the
	// REQ-F-042 canonical error pointing the user at `desktop dbms list`.
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/ghost/plugins/installed" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Could not find DBMS \"ghost\""}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	err := h.run("plugin list ghost --format json")
	if err == nil {
		t.Fatalf("expected DBMS-not-found error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ghost") {
		t.Fatalf("expected error to name DBMS id %q, got: %q", "ghost", msg)
	}
	if !strings.Contains(msg, "desktop dbms list") {
		t.Fatalf("expected error to point at `desktop dbms list`, got: %q", msg)
	}
}

func TestPluginList_PortFlagPropagates(t *testing.T) {
	// `--port <n>` reaches the plugin leaf via cobra's persistent-flag walk
	// from the `desktop dbms` parent. The seam swallows the value (the test
	// httptest server has its own port), but we assert the leaf accepted
	// the flag without parsing errors and the call still completed.
	h := newListHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc/plugins/installed" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("plugin list abc --port 44225 --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
}
