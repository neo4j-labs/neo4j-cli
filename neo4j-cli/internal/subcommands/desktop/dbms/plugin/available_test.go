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

// availableHelper mirrors listHelper but targets the `plugin available` leaf.
// Each plugin leaf gets its own helper instance because the seam is re-pinned
// per test via `t.Cleanup` — sharing helpers across tests would risk
// inter-test pollution if cobra ever stopped tearing down between subtests.
type availableHelper struct {
	t   *testing.T
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newAvailableHelper(t *testing.T) *availableHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	return &availableHelper{
		t:   t,
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

func (h *availableHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-plugin-available"
		clientID = "cid-plugin-available"
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

func (h *availableHelper) run(command string) error {
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

func TestPluginAvailable_RequiresDbmsID(t *testing.T) {
	h := newAvailableHelper(t)
	err := h.run("plugin available")
	if err == nil {
		t.Fatalf("expected error when <dbms-id> is missing")
	}
}

func TestPluginAvailable_Happy_JSON(t *testing.T) {
	// Happy path: relate returns a populated installable catalog; --format
	// json renders the full DbmsPlugin array.
	h := newAvailableHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc/plugins/available" {
			_, _ = w.Write([]byte(`[{"name":"apoc","version":"5.20.0","filePath":"/x/apoc.jar","pendingRestart":false},{"name":"gds","version":"2.10.0","filePath":"/x/gds.jar","pendingRestart":false},{"name":"neo-semantics","version":"5.20.0","filePath":"/x/n10s.jar","pendingRestart":false}]`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("plugin available abc --format json"); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, h.err.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 plugins, got %d (%v)", len(got), got)
	}
	wantNames := []string{"apoc", "gds", "neo-semantics"}
	for i, want := range wantNames {
		if got[i]["name"] != want {
			t.Fatalf("plugin[%d]: expected name %q, got %v", i, want, got[i]["name"])
		}
	}
}

func TestPluginAvailable_Happy_Table(t *testing.T) {
	// Default table format renders the column headers + one row per plugin.
	// Columns match `plugin list` exactly (relate returns the same DbmsPlugin
	// shape from both endpoints).
	h := newAvailableHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc/plugins/available" {
			_, _ = w.Write([]byte(`[{"name":"apoc","version":"5.20.0","filePath":"/x/apoc.jar","pendingRestart":false}]`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("plugin available abc --format table"); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, h.err.String())
	}
	out := h.out.String()
	for _, want := range []string{"NAME", "VERSION", "PENDING_RESTART", "FILE_PATH", "apoc", "5.20.0", "/x/apoc.jar"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected table output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestPluginAvailable_Empty_RendersEmptyTable(t *testing.T) {
	// Empty catalog: relate returns []. Table still shows headers + the
	// `(none)` placeholder row so users see the columns.
	h := newAvailableHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc/plugins/available" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("plugin available abc --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	for _, want := range []string{"NAME", "VERSION", "PENDING_RESTART", "FILE_PATH", "(none)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected empty-table output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestPluginAvailable_Empty_JSONReturnsEmptyArray(t *testing.T) {
	// Empty catalog: JSON consumers always see `[]`, never `null`.
	h := newAvailableHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc/plugins/available" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("plugin available abc --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(h.out.String())
	if got != "[]" {
		t.Fatalf("expected json [] for empty catalog, got %q", got)
	}
}

func TestPluginAvailable_DbmsNotFound_SurfacesCanonicalHint(t *testing.T) {
	// Relate's 404 body whose `message` contains "Could not find DBMS" maps
	// to desktopclient.ErrDbmsNotFound; the leaf translates that into the
	// REQ-F-042 canonical error pointing the user at `desktop dbms list`.
	h := newAvailableHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/ghost/plugins/available" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Could not find DBMS \"ghost\""}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	err := h.run("plugin available ghost --format json")
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

func TestPluginAvailable_PortFlagPropagates(t *testing.T) {
	// `--port <n>` reaches the plugin leaf via cobra's persistent-flag walk
	// from the `desktop dbms` parent. The seam swallows the value (the test
	// httptest server has its own port), but we assert the leaf accepted
	// the flag without parsing errors and the call still completed.
	h := newAvailableHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc/plugins/available" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("plugin available abc --port 44225 --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
}
