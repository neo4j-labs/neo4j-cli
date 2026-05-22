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

// uninstallHelper mirrors `installHelper` — the test surface for the
// uninstall leaf is structurally identical because the lifecycle (pre-op
// GetDbms → POST /plugins/uninstall → optional Stop/Start restart) matches.
// Keeping a separate helper struct keeps the seam pins ratchet-clean
// per-test-file and avoids leaking helper state across leaves.
type uninstallHelper struct {
	t   *testing.T
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newUninstallHelper(t *testing.T) *uninstallHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	t.Cleanup(plugin.SetPollSleepFnForTest(func(_ time.Duration) {}))
	return &uninstallHelper{
		t:   t,
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

func (h *uninstallHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-plugin-uninstall"
		clientID = "cid-plugin-uninstall"
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

func (h *uninstallHelper) run(command string) error {
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

// uninstallRecorder mirrors the install_test.go recorder — call-ordering
// capture for the auto-restart sequence tests.
type uninstallRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *uninstallRecorder) record(label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, label)
}

func (r *uninstallRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	copy(out, r.calls)
	return out
}

func TestPluginUninstall_RequiresBothPositionals(t *testing.T) {
	for _, args := range []string{
		"plugin uninstall",
		"plugin uninstall only-one",
	} {
		t.Run(args, func(t *testing.T) {
			h := newUninstallHelper(t)
			if err := h.run(args); err == nil {
				t.Fatalf("expected error when positionals are missing for %q", args)
			}
		})
	}
}

func TestPluginUninstall_Annotated_WriteTrue(t *testing.T) {
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	root := dbms.NewCmd(cfg)
	plug, _, err := root.Find([]string{"plugin", "uninstall"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got := plug.Annotations["write"]; got != "true" {
		t.Fatalf("expected write=true annotation on plugin uninstall, got %q", got)
	}
}

func TestPluginUninstall_StartedDbms_Default_AutoRestartSequence(t *testing.T) {
	// Happy path: DBMS started, no --no-restart. Leaf issues
	// GetDbms (pre-op) → UninstallPlugin → StopDbms → poll(stopped) → StartDbms
	// → poll(started). Verified via recording handler.
	h := newUninstallHelper(t)
	rec := &uninstallRecorder{}
	var stopCalls atomic.Int32
	var startCalls atomic.Int32
	var getCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			n := getCalls.Add(1)
			rec.record("GET /dbmss/abc")
			switch n {
			case 1:
				_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"started","connectionUri":"neo4j://localhost:7687"}`))
			case 2:
				_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"stopped"}`))
			default:
				_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"started"}`))
			}
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/uninstall":
			rec.record("POST /plugins/uninstall")
			_, _ = w.Write([]byte(`{"name":"apoc"}`))
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

	if err := h.run("plugin uninstall abc --plugin apoc --format json"); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, h.err.String())
	}

	want := []string{
		"GET /dbmss/abc",          // pre-op status capture
		"POST /plugins/uninstall", // primary write
		"POST /stop",              // auto-restart Stop
		"GET /dbmss/abc",          // poll until stopped
		"POST /start",             // auto-restart Start
		"GET /dbmss/abc",          // poll until started
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

	stderr := h.err.String()
	if !strings.Contains(stderr, "restarting DBMS") {
		t.Fatalf("expected stderr breadcrumb mentioning restarting DBMS; got: %q", stderr)
	}
	if !strings.Contains(stderr, "is now removed") {
		t.Fatalf("expected stderr breadcrumb confirming plugin uninstall was applied; got: %q", stderr)
	}

	// stdout shows {name, uninstalled: true} as JSON.
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &payload); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if payload["name"] != "apoc" {
		t.Fatalf("expected uninstalled name=apoc, got %v", payload["name"])
	}
	if payload["uninstalled"] != true {
		t.Fatalf("expected uninstalled=true, got %v", payload["uninstalled"])
	}
	// Output shape is strictly {name, uninstalled} — no extra keys leak.
	if _, hasStatus := payload["status"]; hasStatus {
		t.Fatalf("did not expect `status` key in uninstall JSON; got: %s", h.out.String())
	}
	if _, hasID := payload["id"]; hasID {
		t.Fatalf("did not expect `id` key in uninstall JSON (relate response has only name); got: %s", h.out.String())
	}
}

func TestPluginUninstall_StoppedDbms_Default_NoRestartCalls(t *testing.T) {
	// DBMS stopped before the uninstall: NO restart issued; stderr breadcrumb
	// tells the user the removal will be picked up on next start.
	h := newUninstallHelper(t)
	var stopCalls atomic.Int32
	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/uninstall":
			_, _ = w.Write([]byte(`{"name":"apoc"}`))
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

	if err := h.run("plugin uninstall abc --plugin apoc --format json"); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, h.err.String())
	}
	if stopCalls.Load() != 0 || startCalls.Load() != 0 {
		t.Fatalf("expected no stop/start on stopped DBMS; got stop=%d start=%d", stopCalls.Load(), startCalls.Load())
	}
	if !strings.Contains(h.err.String(), "removal will be picked up on next start") {
		t.Fatalf("expected `removal will be picked up on next start` breadcrumb; got: %q", h.err.String())
	}
}

func TestPluginUninstall_StartedDbms_NoRestart_SkipsRestart(t *testing.T) {
	// --no-restart on a running DBMS: NO Stop/Start calls; manual-restart hint
	// breadcrumb explains the JVM will keep the plugin loaded until restart.
	h := newUninstallHelper(t)
	var stopCalls atomic.Int32
	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/uninstall":
			_, _ = w.Write([]byte(`{"name":"apoc"}`))
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

	if err := h.run("plugin uninstall abc --plugin apoc --no-restart --format json"); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, h.err.String())
	}
	if stopCalls.Load() != 0 || startCalls.Load() != 0 {
		t.Fatalf("expected no stop/start with --no-restart; got stop=%d start=%d", stopCalls.Load(), startCalls.Load())
	}
	if !strings.Contains(h.err.String(), "--no-restart was passed") {
		t.Fatalf("expected manual-restart hint mentioning --no-restart; got: %q", h.err.String())
	}
}

func TestPluginUninstall_AlreadyRemoved_Idempotent(t *testing.T) {
	// REQ-F-038: uninstalling an already-removed plugin returns 200 with the
	// {name} shape — the leaf surfaces that as a normal success, NOT an error.
	// Both relate and the leaf are idempotent on this path.
	h := newUninstallHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/uninstall":
			// Relate returns the same shape whether the plugin was actually
			// installed or already removed — the fixture mirrors that.
			_, _ = w.Write([]byte(`{"name":"already-gone"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("plugin uninstall abc --plugin already-gone --format json"); err != nil {
		t.Fatalf("expected exit 0 on idempotent uninstall, got: %v (stderr=%s)", err, h.err.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &payload); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if payload["name"] != "already-gone" || payload["uninstalled"] != true {
		t.Fatalf("expected {name:already-gone, uninstalled:true}; got %v", payload)
	}
}

func TestPluginUninstall_PluginNotFound_SurfacesCanonicalHint(t *testing.T) {
	// 404 body with "Could not find plugin" maps to ErrPluginNotFound → REQ-F-041.
	h := newUninstallHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/uninstall":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Could not find plugin \"not-a-plugin\""}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("plugin uninstall abc --plugin not-a-plugin")
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

func TestPluginUninstall_DbmsNotFound_SurfacesCanonicalHint(t *testing.T) {
	// 404 body with "Could not find DBMS" maps to ErrDbmsNotFound → REQ-F-042.
	h := newUninstallHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/ghost":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Could not find DBMS \"ghost\""}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/ghost/plugins/uninstall":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Could not find DBMS \"ghost\""}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("plugin uninstall ghost --plugin apoc")
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

func TestPluginUninstall_AutoRestart_StopFailure_WarnsAndExitsZero(t *testing.T) {
	// Auto-restart Stop fails after the uninstall succeeded: exit 0, stderr
	// warning. REQ-F-039 — uninstall op is not rolled back.
	h := newUninstallHelper(t)
	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/uninstall":
			_, _ = w.Write([]byte(`{"name":"apoc"}`))
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

	if err := h.run("plugin uninstall abc --plugin apoc --format json"); err != nil {
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
	// stdout still shows the success confirmation.
	if !strings.Contains(h.out.String(), "apoc") {
		t.Fatalf("expected uninstalled plugin confirmation on stdout despite restart failure; got: %q", h.out.String())
	}
}

func TestPluginUninstall_PortFlagPropagates(t *testing.T) {
	h := newUninstallHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/uninstall":
			_, _ = w.Write([]byte(`{"name":"apoc"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("plugin uninstall abc --plugin apoc --port 44225 --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestPluginUninstall_DefaultFormat_OneLineConfirmation(t *testing.T) {
	// Default (table) format renders a single confirmation line — no per-row
	// table is generated because the uninstall payload is a single fact.
	h := newUninstallHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/uninstall":
			_, _ = w.Write([]byte(`{"name":"apoc"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("plugin uninstall abc --plugin apoc --format table"); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, h.err.String())
	}
	stdout := h.out.String()
	if !strings.Contains(stdout, `Uninstalled plugin "apoc".`) {
		t.Fatalf("expected one-line confirmation on stdout; got: %q", stdout)
	}
}
