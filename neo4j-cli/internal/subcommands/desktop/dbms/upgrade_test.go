// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// upgradeHelper mirrors createHelper/startHelper: dbms.NewCmd wired against an
// in-memory FS, with `newDesktopClientFn` pinned to a desktopclient.Client
// backed by an httptest server. Poll sleeps are no-ops so the force/stop poll
// runs instantly.
type upgradeHelper struct {
	t   *testing.T
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newUpgradeHelper(t *testing.T) *upgradeHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	t.Cleanup(dbms.SetCreatePollSleepFnForTest(func(_ time.Duration) {}))
	return &upgradeHelper{
		t:   t,
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

func (h *upgradeHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-upgrade"
		clientID = "cid-upgrade"
	)
	h.t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return clientID }))
	h.t.Cleanup(desktopclient.SetNowFnForTest(func() time.Time { return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC) }))
	srv := httptest.NewServer(handler)
	h.t.Cleanup(srv.Close)

	h.t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return desktopclient.NewClient(desktopclient.ProbeResult{Origin: srv.URL}, salt)
	}))
	return srv
}

func (h *upgradeHelper) run(command string) error {
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

// readUpgradeBody decodes the POST /dbmss/:id/upgrade JSON body.
func readUpgradeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("body json: %v (raw: %s)", err, string(b))
	}
	return out
}

// upgradeOptions returns the `options` sub-object from a decoded upgrade body,
// failing the test if it is missing or the wrong shape.
func upgradeOptions(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	raw, ok := body["options"]
	if !ok {
		t.Fatalf("expected `options` in upgrade body, got %+v", body)
	}
	opts, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected `options` to be an object, got %T", raw)
	}
	return opts
}

func TestUpgrade_RequiresID(t *testing.T) {
	h := newUpgradeHelper(t)
	if err := h.run("upgrade"); err == nil {
		t.Fatalf("expected error when <id> is missing")
	}
}

// TestUpgrade_StoppedExplicitVersion is the happy path: a stopped DBMS plus an
// explicit --version. Asserts the upgrade POST body (version + default options)
// and the rendered row.
func TestUpgrade_StoppedExplicitVersion(t *testing.T) {
	h := newUpgradeHelper(t)
	var (
		captured     map[string]any
		upgradeCalls atomic.Int32
	)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/upgrade":
			upgradeCalls.Add(1)
			captured = readUpgradeBody(t, r)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.26.1","status":"stopped","connectionUri":"neo4j://localhost:7687"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("upgrade abc --version 5.26.1 --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if upgradeCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 POST /upgrade, got %d", upgradeCalls.Load())
	}
	if captured["version"] != "5.26.1" {
		t.Fatalf("expected body version=5.26.1, got %v", captured["version"])
	}
	opts := upgradeOptions(t, captured)
	if opts["pluginUpgradeMode"] != "UPGRADABLE" {
		t.Fatalf("expected default pluginUpgradeMode=UPGRADABLE, got %v", opts["pluginUpgradeMode"])
	}
	if opts["migrate"] != true {
		t.Fatalf("expected default migrate=true, got %v", opts["migrate"])
	}
	if opts["backup"] != true {
		t.Fatalf("expected default backup=true, got %v", opts["backup"])
	}

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "abc" || got["version"] != "5.26.1" || got["status"] != "stopped" {
		t.Fatalf("unexpected rendered row: %+v", got)
	}
	if got["connectionUri"] != "neo4j://localhost:7687" {
		t.Fatalf("expected connectionUri in rendered row, got %v", got["connectionUri"])
	}
	// The stopped-DBMS start hint is on stderr.
	if !strings.Contains(h.err.String(), "desktop dbms start abc --rw") {
		t.Fatalf("expected stderr start hint, got: %q", h.err.String())
	}
}

// TestUpgrade_VersionOmitted_AutoPicksLatest verifies the auto-pick: with
// --version omitted, the leaf calls GET /dbmss/versions, picks the highest
// stable enterprise entry, emits a stderr breadcrumb, and POSTs that version.
func TestUpgrade_VersionOmitted_AutoPicksLatest(t *testing.T) {
	h := newUpgradeHelper(t)
	var (
		versionsCalls atomic.Int32
		captured      map[string]any
	)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/versions":
			versionsCalls.Add(1)
			_, _ = w.Write([]byte(`[
				{"dist":"/cache/neo4j-enterprise-2025.05.0","edition":"enterprise","origin":"cached","version":"2025.05.0"},
				{"dist":"/cache/neo4j-enterprise-2026.04.0","edition":"enterprise","origin":"cached","version":"2026.04.0"},
				{"dist":"https://dist.neo4j.org/neo4j-enterprise-5.26.1-unix.tar.gz","edition":"enterprise","origin":"online","version":"5.26.1"}
			]`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/upgrade":
			captured = readUpgradeBody(t, r)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"2026.04.0","status":"stopped"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("upgrade abc --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if versionsCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 GET /dbmss/versions, got %d", versionsCalls.Load())
	}
	if captured == nil || captured["version"] != "2026.04.0" {
		t.Fatalf("expected body version=2026.04.0 (highest stable enterprise), got %+v", captured)
	}
	stderr := h.err.String()
	if !strings.Contains(stderr, "Using Neo4j enterprise 2026.04.0") {
		t.Fatalf("expected stderr breadcrumb naming picked version, got: %q", stderr)
	}
	if !strings.Contains(stderr, "(cached)") {
		t.Fatalf("expected stderr breadcrumb naming origin, got: %q", stderr)
	}
}

// TestUpgrade_RunningWithoutForce_Refuses asserts a running target without
// --force fails with a refusal error AND issues NO upgrade POST.
func TestUpgrade_RunningWithoutForce_Refuses(t *testing.T) {
	h := newUpgradeHelper(t)
	var upgradeCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/upgrade":
			upgradeCalls.Add(1)
			t.Errorf("upgrade POST must NOT be issued when the DBMS is running and --force is absent")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("upgrade abc --version 5.26.1 --format json")
	if err == nil {
		t.Fatalf("expected refusal error for a running DBMS without --force")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected error to mention --force escape hatch, got: %v", err)
	}
	if !strings.Contains(err.Error(), "abc") {
		t.Fatalf("expected error to name the DBMS, got: %v", err)
	}
	if upgradeCalls.Load() != 0 {
		t.Fatalf("expected 0 upgrade POSTs, got %d", upgradeCalls.Load())
	}
}

// TestUpgrade_RunningWithForce_StopsThenUpgrades asserts the --force sequence:
// Stop -> GET(stopped) -> upgrade, with stop strictly before upgrade.
func TestUpgrade_RunningWithForce_StopsThenUpgrades(t *testing.T) {
	h := newUpgradeHelper(t)
	var (
		mu       sync.Mutex
		order    []string
		stopped  atomic.Bool
		captured map[string]any
	)
	record := func(label string) {
		mu.Lock()
		order = append(order, label)
		mu.Unlock()
	}
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			record("GET")
			if stopped.Load() {
				_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"stopped"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/abc/stop":
			record("STOP")
			stopped.Store(true)
			_, _ = w.Write([]byte(`"stopped"`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/upgrade":
			record("UPGRADE")
			captured = readUpgradeBody(t, r)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.26.1","status":"stopped"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("upgrade abc --version 5.26.1 --force --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if captured == nil || captured["version"] != "5.26.1" {
		t.Fatalf("expected upgrade to fire with version=5.26.1, got %+v", captured)
	}

	mu.Lock()
	defer mu.Unlock()
	stopIdx, upgradeIdx, getAfterStop := -1, -1, -1
	for i, label := range order {
		if label == "STOP" && stopIdx == -1 {
			stopIdx = i
		}
		if label == "UPGRADE" && upgradeIdx == -1 {
			upgradeIdx = i
		}
		if label == "GET" && stopIdx != -1 && i > stopIdx && getAfterStop == -1 {
			getAfterStop = i
		}
	}
	if stopIdx == -1 {
		t.Fatalf("expected STOP call, order=%v", order)
	}
	if upgradeIdx == -1 {
		t.Fatalf("expected UPGRADE call, order=%v", order)
	}
	if stopIdx >= upgradeIdx {
		t.Fatalf("STOP must happen BEFORE UPGRADE; order=%v", order)
	}
	// A GET(stopped) poll must sit between STOP and UPGRADE.
	if getAfterStop == -1 || getAfterStop > upgradeIdx {
		t.Fatalf("expected a GET(stopped) poll between STOP and UPGRADE; order=%v", order)
	}
	if !strings.Contains(h.err.String(), "Stopping") {
		t.Fatalf("expected 'Stopping' breadcrumb on stderr, got: %q", h.err.String())
	}
}

// TestUpgrade_PluginUpgradeMode_Mapping covers the case-insensitive lowercase
// values mapping to the uppercase wire enum.
func TestUpgrade_PluginUpgradeMode_Mapping(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag string
		wire string
	}{
		{name: "none", flag: "none", wire: "NONE"},
		{name: "all", flag: "all", wire: "ALL"},
		{name: "upper-case-input", flag: "NONE", wire: "NONE"},
		{name: "mixed-case-input", flag: "All", wire: "ALL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newUpgradeHelper(t)
			var captured map[string]any
			h.withHandler(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
					_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"stopped"}`))
				case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/upgrade":
					captured = readUpgradeBody(t, r)
					_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.26.1","status":"stopped"}`))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
			})

			if err := h.run("upgrade abc --version 5.26.1 --plugin-upgrade-mode " + tc.flag + " --format json"); err != nil {
				t.Fatalf("run: %v", err)
			}
			opts := upgradeOptions(t, captured)
			if opts["pluginUpgradeMode"] != tc.wire {
				t.Fatalf("expected pluginUpgradeMode=%q, got %v", tc.wire, opts["pluginUpgradeMode"])
			}
		})
	}
}

// TestUpgrade_PluginUpgradeMode_Invalid asserts an invalid value is a usage
// error rejected up front (no Desktop round-trip).
func TestUpgrade_PluginUpgradeMode_Invalid(t *testing.T) {
	h := newUpgradeHelper(t)
	var anyCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		anyCalls.Add(1)
		t.Errorf("no Desktop call expected on invalid --plugin-upgrade-mode; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run("upgrade abc --version 5.26.1 --plugin-upgrade-mode bogus --format json")
	if err == nil {
		t.Fatalf("expected usage error for invalid --plugin-upgrade-mode")
	}
	if !strings.Contains(err.Error(), "all, none, upgradable") {
		t.Fatalf("expected error to list valid values, got: %v", err)
	}
	if anyCalls.Load() != 0 {
		t.Fatalf("expected 0 Desktop calls on invalid mode, got %d", anyCalls.Load())
	}
}

// TestUpgrade_NoMigrate_And_BackupFalse asserts the boolean flags land in the
// body: --no-migrate -> migrate:false, --backup=false -> backup:false.
func TestUpgrade_NoMigrate_And_BackupFalse(t *testing.T) {
	h := newUpgradeHelper(t)
	var captured map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/upgrade":
			captured = readUpgradeBody(t, r)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.26.1","status":"stopped"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("upgrade abc --version 5.26.1 --no-migrate --backup=false --plugin-upgrade-mode none --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	opts := upgradeOptions(t, captured)
	if opts["migrate"] != false {
		t.Fatalf("expected migrate=false with --no-migrate, got %v", opts["migrate"])
	}
	if opts["backup"] != false {
		t.Fatalf("expected backup=false with --backup=false, got %v", opts["backup"])
	}
	if opts["pluginUpgradeMode"] != "NONE" {
		t.Fatalf("expected pluginUpgradeMode=NONE, got %v", opts["pluginUpgradeMode"])
	}
}

// TestUpgrade_FormatJSON_EmitsFullDbmsInfo asserts --format json emits the full
// upgraded DbmsInfo, not a slim envelope.
func TestUpgrade_FormatJSON_EmitsFullDbmsInfo(t *testing.T) {
	h := newUpgradeHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/upgrade":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.26.1","status":"stopped","connectionUri":"neo4j://localhost:7687"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("upgrade abc --version 5.26.1 --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	for k, want := range map[string]any{
		"id":            "abc",
		"name":          "my-dbms",
		"version":       "5.26.1",
		"status":        "stopped",
		"connectionUri": "neo4j://localhost:7687",
	} {
		if got[k] != want {
			t.Fatalf("expected %s=%v in full DbmsInfo, got %v", k, want, got[k])
		}
	}
}

func TestUpgrade_Annotated_Write(t *testing.T) {
	cfg := clicfg.NewConfig(mustTestFs(t), "test", clicfg.GlobalScope)
	parent := dbms.NewCmd(cfg)
	var upgradeCmd *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "upgrade" {
			upgradeCmd = c
			break
		}
	}
	if upgradeCmd == nil {
		t.Fatalf("upgrade command not registered under dbms")
	}
	if upgradeCmd.Annotations["write"] != "true" {
		t.Fatalf("upgrade must be annotated write=true; got %v", upgradeCmd.Annotations)
	}
}
