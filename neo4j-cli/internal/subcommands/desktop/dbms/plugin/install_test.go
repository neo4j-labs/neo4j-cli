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
	"sync"
	"sync/atomic"
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

// installHelper wires `dbms.NewCmd` (which mounts the plugin subtree) against
// an in-memory FS, with the plugin package's `newDesktopClientFn` seam pinned
// to a desktopclient.Client backed by an httptest server. Mirrors the
// listHelper / availableHelper shape — each leaf-subtree owns its own test
// surface so the seams stay isolated and re-runs don't pollute each other.
type installHelper struct {
	t   *testing.T
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newInstallHelper(t *testing.T) *installHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	// Pin the auto-restart poll-loop sleep to a no-op so timeout tests stay
	// instantaneous (production sleeps 1s between polls; tests would otherwise
	// burn 30s of real wall-clock on the timeout path).
	t.Cleanup(plugin.SetPollSleepFnForTest(func(_ time.Duration) {}))
	return &installHelper{
		t:   t,
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

func (h *installHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-plugin-install"
		clientID = "cid-plugin-install"
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

func (h *installHelper) run(command string) error {
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

// recorder is a minimal request-ordering capture used by the auto-restart
// tests. Mirrors the recording-handler pattern in start_test.go / create_test.go.
type recorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *recorder) record(label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, label)
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	copy(out, r.calls)
	return out
}

func TestPluginInstall_RequiresBothPositionals(t *testing.T) {
	for _, args := range []string{
		"plugin install",
		"plugin install only-one",
	} {
		t.Run(args, func(t *testing.T) {
			h := newInstallHelper(t)
			if err := h.run(args); err == nil {
				t.Fatalf("expected error when positionals are missing for %q", args)
			}
		})
	}
}

func TestPluginInstall_Annotated_WriteTrue(t *testing.T) {
	// The leaf must be annotated `write=true` so the root --rw gate refuses
	// it in non-interactive contexts (REQ-F-014). We can't drive the gate
	// itself from the dbms.NewCmd entrypoint (the gate lives on the root
	// PersistentPreRunE), but the annotation is the load-bearing contract.
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	root := dbms.NewCmd(cfg)
	plug, _, err := root.Find([]string{"plugin", "install"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got := plug.Annotations["write"]; got != "true" {
		t.Fatalf("expected write=true annotation on plugin install, got %q", got)
	}
}

func TestPluginInstall_StartedDbms_Default_AutoRestartSequence(t *testing.T) {
	// Happy path, DBMS started, no --no-restart: leaf issues
	// GetDbms (pre-op) → InstallPlugin → StopDbms → poll(stopped) → StartDbms
	// → poll(started). Verified via a recording handler that captures call
	// order. Both stderr breadcrumbs ("restarting" + "now active") fire.
	h := newInstallHelper(t)
	rec := &recorder{}
	var stopCalls atomic.Int32
	var startCalls atomic.Int32
	var getCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			n := getCalls.Add(1)
			rec.record("GET /dbmss/abc")
			// Sequence: 1=preOp(started), 2=postStop(stopped), 3=postStart(started).
			switch n {
			case 1:
				_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"started","connectionUri":"neo4j://localhost:7687"}`))
			case 2:
				_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"stopped"}`))
			default:
				_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"started"}`))
			}
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/install":
			rec.record("POST /plugins/install")
			_, _ = w.Write([]byte(`{"name":"apoc","version":"5.20.0","filePath":"/x/apoc.jar","pendingRestart":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/abc/stop":
			stopCalls.Add(1)
			rec.record("POST /stop")
			_, _ = w.Write([]byte(`"stopped"`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			startCalls.Add(1)
			rec.record("POST /start")
			_, _ = w.Write([]byte(`"started"`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("plugin install abc --plugin apoc --format json"); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, h.err.String())
	}

	// Call order: pre-op GetDbms, install, stop+pollGet, start+pollGet.
	want := []string{
		"GET /dbmss/abc",        // pre-op status capture
		"POST /plugins/install", // primary write
		"POST /stop",            // auto-restart Stop
		"GET /dbmss/abc",        // poll until stopped
		"POST /start",           // auto-restart Start
		"GET /dbmss/abc",        // poll until started
	}
	got := rec.snapshot()
	if len(got) != len(want) {
		t.Fatalf("call sequence length mismatch: want %d (%v), got %d (%v)", len(want), want, len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("call sequence[%d] = %q; want %q (full=%v)", i, got[i], want[i], got)
		}
	}
	if stopCalls.Load() != 1 || startCalls.Load() != 1 {
		t.Fatalf("expected 1 stop + 1 start call; got stop=%d start=%d", stopCalls.Load(), startCalls.Load())
	}

	// Both stderr breadcrumbs fired.
	stderr := h.err.String()
	if !strings.Contains(stderr, "restarting DBMS") {
		t.Fatalf("expected stderr breadcrumb mentioning restarting DBMS; got: %q", stderr)
	}
	if !strings.Contains(stderr, "is now active") {
		t.Fatalf("expected stderr breadcrumb confirming plugin is active; got: %q", stderr)
	}

	// stdout shows the installed plugin as JSON (full DbmsPlugin wire shape).
	var pl map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &pl); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if pl["name"] != "apoc" {
		t.Fatalf("expected installed plugin name=apoc, got %v", pl["name"])
	}
}

func TestPluginInstall_StoppedDbms_Default_NoRestartCalls(t *testing.T) {
	// DBMS stopped before the install: NO restart is issued (no Stop, no
	// Start) and a stderr breadcrumb tells the user the plugin will activate
	// on next start. The primary install POST still fires.
	h := newInstallHelper(t)
	var stopCalls atomic.Int32
	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/install":
			_, _ = w.Write([]byte(`{"name":"apoc","version":"5.20.0","filePath":"/x/apoc.jar","pendingRestart":false}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/abc/stop":
			stopCalls.Add(1)
			t.Errorf("unexpected stop call on stopped DBMS")
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			startCalls.Add(1)
			t.Errorf("unexpected start call on stopped DBMS")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("plugin install abc --plugin apoc --format json"); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, h.err.String())
	}
	if stopCalls.Load() != 0 || startCalls.Load() != 0 {
		t.Fatalf("expected no stop/start on stopped DBMS; got stop=%d start=%d", stopCalls.Load(), startCalls.Load())
	}
	if !strings.Contains(h.err.String(), "will activate on next start") {
		t.Fatalf("expected `will activate on next start` breadcrumb; got: %q", h.err.String())
	}
}

func TestPluginInstall_StartedDbms_NoRestart_SkipsRestart(t *testing.T) {
	// `--no-restart` on a running DBMS: NO Stop/Start calls, and a manual-
	// restart hint breadcrumb explains the user must restart for the plugin
	// to activate.
	h := newInstallHelper(t)
	var stopCalls atomic.Int32
	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/install":
			_, _ = w.Write([]byte(`{"name":"apoc","version":"5.20.0","filePath":"/x/apoc.jar","pendingRestart":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/abc/stop":
			stopCalls.Add(1)
			t.Errorf("unexpected stop call when --no-restart was passed")
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			startCalls.Add(1)
			t.Errorf("unexpected start call when --no-restart was passed")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("plugin install abc --plugin apoc --no-restart --format json"); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, h.err.String())
	}
	if stopCalls.Load() != 0 || startCalls.Load() != 0 {
		t.Fatalf("expected no stop/start with --no-restart; got stop=%d start=%d", stopCalls.Load(), startCalls.Load())
	}
	if !strings.Contains(h.err.String(), "--no-restart was passed") {
		t.Fatalf("expected manual-restart hint mentioning --no-restart; got: %q", h.err.String())
	}
}

func TestPluginInstall_PluginNotFound_SurfacesCanonicalHint(t *testing.T) {
	// Relate's 404 body whose `message` contains "Could not find plugin" maps
	// to desktopclient.ErrPluginNotFound; the leaf translates that into the
	// REQ-F-041 canonical error pointing the user at `desktop dbms plugin
	// available <dbms-id>`.
	h := newInstallHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/install":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Could not find plugin \"not-a-plugin\""}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("plugin install abc --plugin not-a-plugin")
	if err == nil {
		t.Fatalf("expected plugin-not-found error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "not-a-plugin") {
		t.Fatalf("expected error to name plugin %q, got: %q", "not-a-plugin", msg)
	}
	if !strings.Contains(msg, "desktop dbms plugin available") {
		t.Fatalf("expected error to point at `desktop dbms plugin available`, got: %q", msg)
	}
}

func TestPluginInstall_DbmsNotFound_SurfacesCanonicalHint(t *testing.T) {
	// DBMS not found: relate 404s both the pre-op GetDbms and the install
	// POST. The install path's `doPlugin` disambiguates the 404 body into
	// `ErrDbmsNotFound`, which the leaf translates to REQ-F-042 wording. The
	// pre-op GetDbms uses the generic `do()` path (no sentinel), so the
	// install POST is what produces the canonical hint.
	h := newInstallHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/ghost":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Could not find DBMS \"ghost\""}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/ghost/plugins/install":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Could not find DBMS \"ghost\""}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("plugin install ghost --plugin apoc")
	if err == nil {
		t.Fatalf("expected DBMS-not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected error to name DBMS id, got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "desktop dbms list") {
		t.Fatalf("expected error to point at `desktop dbms list`, got: %q", err.Error())
	}
}

func TestPluginInstall_AutoRestart_StopFailure_WarnsAndExitsZero(t *testing.T) {
	// Auto-restart Stop fails after the install succeeded: leaf exits 0
	// (REQ-F-039 — plugin op is not rolled back) but a stderr warning names
	// the failure mode and tells the user how to recover.
	h := newInstallHelper(t)
	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/install":
			_, _ = w.Write([]byte(`{"name":"apoc","version":"5.20.0","filePath":"/x/apoc.jar","pendingRestart":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/abc/stop":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`stop failed`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			startCalls.Add(1)
			t.Errorf("unexpected start after a stop failure")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("plugin install abc --plugin apoc --format json"); err != nil {
		t.Fatalf("expected exit 0 on auto-restart stop failure (REQ-F-039), got error: %v", err)
	}
	if startCalls.Load() != 0 {
		t.Fatalf("expected no start after stop failure; got %d", startCalls.Load())
	}
	stderr := h.err.String()
	if !strings.Contains(stderr, "Warning:") {
		t.Fatalf("expected `Warning:` prefix on auto-restart failure; got: %q", stderr)
	}
	if !strings.Contains(stderr, "auto-restart stop") {
		t.Fatalf("expected stderr to name the failed operation; got: %q", stderr)
	}
	// stdout still shows the installed plugin row — install succeeded.
	if !strings.Contains(h.out.String(), "apoc") {
		t.Fatalf("expected installed plugin on stdout despite restart failure; got: %q", h.out.String())
	}
}

func TestPluginInstall_AutoRestart_StartFailure_WarnsAndExitsZero(t *testing.T) {
	// Stop succeeds but Start fails: same downgrade-to-warning semantics.
	h := newInstallHelper(t)
	var getCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			n := getCalls.Add(1)
			switch n {
			case 1:
				_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
			default:
				_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
			}
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/install":
			_, _ = w.Write([]byte(`{"name":"apoc","version":"5.20.0","filePath":"/x/apoc.jar","pendingRestart":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/abc/stop":
			_, _ = w.Write([]byte(`"stopped"`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`start failed`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("plugin install abc --plugin apoc --format json"); err != nil {
		t.Fatalf("expected exit 0 on auto-restart start failure (REQ-F-039), got error: %v", err)
	}
	stderr := h.err.String()
	if !strings.Contains(stderr, "Warning:") || !strings.Contains(stderr, "auto-restart start") {
		t.Fatalf("expected stderr to warn about auto-restart start failure; got: %q", stderr)
	}
}

func TestPluginInstall_PortFlagPropagates(t *testing.T) {
	// `--port <n>` reaches the leaf via cobra's persistent-flag walk from the
	// `desktop dbms` parent. The seam swallows the value (the test httptest
	// server has its own port), but we assert the flag is accepted without
	// parsing errors and the call still completes.
	h := newInstallHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/install":
			_, _ = w.Write([]byte(`{"name":"apoc","version":"5.20.0","filePath":"/x/apoc.jar","pendingRestart":false}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("plugin install abc --plugin apoc --port 44225 --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
}
