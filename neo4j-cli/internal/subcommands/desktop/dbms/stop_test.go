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
	"path/filepath"
	"strings"
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

// stopHelper wires dbms.NewCmd against an in-memory FS, mirroring the
// startHelper / createHelper shape. Test surfaces are colocated per leaf
// so each leaf's behaviour stays auditable without grepping through a
// shared file.
type stopHelper struct {
	t   *testing.T
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newStopHelper(t *testing.T) *stopHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	t.Cleanup(dbms.SetCreatePollSleepFnForTest(func(_ time.Duration) {}))
	return &stopHelper{
		t:   t,
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

func (h *stopHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-stop"
		clientID = "cid-stop"
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

func (h *stopHelper) run(command string) error {
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

func TestStop_RequiresID(t *testing.T) {
	h := newStopHelper(t)
	err := h.run("stop")
	if err == nil {
		t.Fatalf("expected error when <id> is missing")
	}
}

func TestStop_NoWait_EnrichedSingleGet(t *testing.T) {
	// `desktop stop` (no `--wait`) issues exactly one POST /stop followed by
	// one GET /dbmss/:id to enrich the rendered row; the GET is NOT a poll
	// loop. Mirrors TestStart_NoWait_EnrichedSingleGet.
	h := newStopHelper(t)
	var getCalls atomic.Int32
	var stopCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/abc/stop":
			stopCalls.Add(1)
			_, _ = w.Write([]byte(`"stopped"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			getCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"stopping","connectionUri":"neo4j://localhost:7687"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("stop abc --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if stopCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 POST /stop; got %d", stopCalls.Load())
	}
	if getCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 GET /dbmss/abc (enrichment, no poll); got %d", getCalls.Load())
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "abc" {
		t.Fatalf("expected id=abc in output, got %v", got["id"])
	}
	if got["name"] != "my-dbms" {
		t.Fatalf("expected enriched name=my-dbms, got %v", got["name"])
	}
	if got["version"] != "5.20.0" {
		t.Fatalf("expected enriched version=5.20.0, got %v", got["version"])
	}
	if got["status"] != "stopping" {
		t.Fatalf("expected enriched status=stopping, got %v", got["status"])
	}
	if got["connectionUri"] != "neo4j://localhost:7687" {
		t.Fatalf("expected enriched connectionUri, got %v", got["connectionUri"])
	}
}

func TestStop_NoWait_EnrichmentFail_FallsBackToSlimEnvelope(t *testing.T) {
	// If the post-stop GetDbms fails, command exits 0 with the slim envelope
	// + a stderr warning naming the id — lifecycle call already succeeded.
	h := newStopHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/abc/stop":
			_, _ = w.Write([]byte(`"stopped"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("stop abc --format json"); err != nil {
		t.Fatalf("expected exit 0 on post-call GetDbms failure, got: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "abc" {
		t.Fatalf("expected slim {id} envelope with id=abc, got %v", got["id"])
	}
	if _, hasName := got["name"]; hasName {
		t.Fatalf("expected slim envelope (no name field) on enrichment failure, got %v", got)
	}
	if !strings.Contains(h.err.String(), `"abc"`) {
		t.Fatalf("expected stderr warning naming id=abc, got: %q", h.err.String())
	}
	if !strings.Contains(h.err.String(), "Warning") {
		t.Fatalf("expected stderr warning prefix, got: %q", h.err.String())
	}
}

func TestStop_Wait_PollsUntilOffline(t *testing.T) {
	h := newStopHelper(t)
	var getCount atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/abc/stop":
			_, _ = w.Write([]byte(`"stopped"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			n := getCount.Add(1)
			if n < 3 {
				_, _ = w.Write([]byte(`{"id":"abc","status":"stopping"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("stop abc --wait --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if getCount.Load() < 3 {
		t.Fatalf("expected at least 3 GET /dbmss/abc calls, got %d", getCount.Load())
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["status"] != "stopped" {
		t.Fatalf("expected status=stopped in output, got %v", got["status"])
	}
}

func TestStop_Wait_TimeoutExitsNonZeroWithLastStatus(t *testing.T) {
	h := newStopHelper(t)
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	var ticks atomic.Int32
	t.Cleanup(dbms.SetCreateNowFnForTest(func() time.Time {
		if ticks.Add(1) == 1 {
			return start
		}
		return start.Add(1 * time.Minute)
	}))

	var getCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/abc/stop":
			_, _ = w.Write([]byte(`"stopped"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			getCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"abc","status":"stopping"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("stop abc --wait --format json")
	if err == nil {
		t.Fatalf("expected timeout error when DBMS never reaches stopped")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"stopped"`) {
		t.Fatalf("expected target status %q in error, got: %v", "stopped", err)
	}
	if !strings.Contains(err.Error(), `"stopping"`) {
		t.Fatalf("expected last-seen status %q in error, got: %v", "stopping", err)
	}
	if getCalls.Load() < 1 {
		t.Fatalf("expected at least 1 GET poll before timeout, got %d", getCalls.Load())
	}
}

func TestStop_NoCredentialsJSONWrite(t *testing.T) {
	h := newStopHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/abc/stop":
			_, _ = w.Write([]byte(`"stopped"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","status":"stopped"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	credsPath := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json")
	before, err := afero.ReadFile(h.fs, credsPath)
	if err != nil {
		t.Fatalf("read credentials before: %v", err)
	}

	if err := h.run("stop abc --wait --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}

	after, err := afero.ReadFile(h.fs, credsPath)
	if err != nil {
		t.Fatalf("read credentials after: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("credentials.json was mutated; before=%q after=%q", string(before), string(after))
	}
}

func TestStop_Annotated_Write(t *testing.T) {
	cfg := clicfg.NewConfig(mustTestFs(t), "test", clicfg.GlobalScope)
	parent := dbms.NewCmd(cfg)
	var stopCmd *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "stop" {
			stopCmd = c
			break
		}
	}
	if stopCmd == nil {
		t.Fatalf("stop command not registered under desktop")
	}
	if stopCmd.Annotations["write"] != "true" {
		t.Fatalf("stop must be annotated write=true; got %v", stopCmd.Annotations)
	}
}

func TestStop_PortFlagPropagatesToClientConstructor(t *testing.T) {
	h := newStopHelper(t)
	var gotPort int
	t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		gotPort = port
		return nil, errors.New("stop here; we already captured the port")
	}))

	_ = h.run("stop abc --port 44230 --format json")
	if gotPort != 44230 {
		t.Fatalf("expected --port=44230 to propagate to client constructor, got %d", gotPort)
	}
}

func TestStop_DesktopUnreachable_ReturnsCanonicalError(t *testing.T) {
	h := newStopHelper(t)
	t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return nil, desktopclient.UnreachableError()
	}))

	err := h.run("stop abc --format json")
	if err == nil {
		t.Fatalf("expected error when desktop is unreachable")
	}
	if !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Fatalf("expected canonical REQ-F-008 message, got: %v", err)
	}
}
