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

// startHelper wires dbms.NewCmd against an in-memory FS, with the shared
// `newDesktopClientFn` seam pinned to a desktopclient.Client backed by an
// httptest server. Same shape as createHelper; kept colocated with the
// start leaf so each leaf's test surface stays self-contained.
type startHelper struct {
	t   *testing.T
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newStartHelper(t *testing.T) *startHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	// Pin the poll-loop sleep to a no-op so timeout tests are instantaneous.
	t.Cleanup(dbms.SetCreatePollSleepFnForTest(func(_ time.Duration) {}))
	return &startHelper{
		t:   t,
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

func (h *startHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-start"
		clientID = "cid-start"
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

func (h *startHelper) run(command string) error {
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

func TestStart_RequiresID(t *testing.T) {
	h := newStartHelper(t)
	err := h.run("start")
	if err == nil {
		t.Fatalf("expected error when <id> is missing")
	}
}

func TestStart_NoWait_EnrichedSingleGet(t *testing.T) {
	// `desktop start` (no `--wait`) issues exactly one POST /start followed by
	// one GET /dbmss/:id to enrich the rendered row; the GET is NOT a poll
	// loop (no retry, no 1s cadence). Asserts the enriched fields make it to
	// stdout in addition to `id`. Does NOT use `--force` so the pre-flight
	// list+enrich fan-out also fires; the empty list response means no
	// conflict and the start proceeds.
	h := newStartHelper(t)
	var getCalls atomic.Int32
	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			startCalls.Add(1)
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			getCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.20.0","status":"starting","connectionUri":"neo4j://localhost:7687"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("start abc --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if startCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 POST /start; got %d", startCalls.Load())
	}
	// Exactly one GET — enrichment is a single call, NOT the --wait poll loop.
	if getCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 GET /dbmss/abc (enrichment, no poll); got %d", getCalls.Load())
	}
	// Output carries the enriched DbmsInfo, not just `{id}`.
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
	if got["status"] != "starting" {
		t.Fatalf("expected enriched status=starting, got %v", got["status"])
	}
	if got["connectionUri"] != "neo4j://localhost:7687" {
		t.Fatalf("expected enriched connectionUri, got %v", got["connectionUri"])
	}
}

func TestStart_NoWait_EnrichmentFail_FallsBackToSlimEnvelope(t *testing.T) {
	// If the post-start GetDbms fails (Desktop quit between calls; rare), the
	// command exits 0 with the slim `{id}` envelope on stdout and a stderr-only
	// warning naming the id — the lifecycle call itself already succeeded.
	h := newStartHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			// Pre-flight returned empty list (no DBMSes) so no enrichment for
			// abc; this GET fires post-start as the enrichment call. Always
			// fail it to exercise the slim-envelope fallback.
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("start abc --format json"); err != nil {
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

func TestStart_Wait_PollsUntilOnline(t *testing.T) {
	h := newStartHelper(t)
	// started only on the third GET — proves we actually loop. The pre-flight
	// list returns empty so no conflict; pre-flight does NOT issue per-DBMS
	// GETs for `abc` (it only enriches list entries, of which there are none).
	var getCount atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			n := getCount.Add(1)
			if n < 3 {
				_, _ = w.Write([]byte(`{"id":"abc","status":"starting"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started","connectionUri":"neo4j://localhost:7687"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("start abc --wait --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if getCount.Load() < 3 {
		t.Fatalf("expected at least 3 GET /dbmss/abc calls, got %d", getCount.Load())
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["status"] != "started" {
		t.Fatalf("expected status=started in output, got %v", got["status"])
	}
	if got["connectionUri"] != "neo4j://localhost:7687" {
		t.Fatalf("expected connectionUri in output, got %v", got["connectionUri"])
	}
}

func TestStart_Wait_TimeoutExitsNonZeroWithLastStatus(t *testing.T) {
	h := newStartHelper(t)
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
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			getCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"abc","status":"starting"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("start abc --wait --format json")
	if err == nil {
		t.Fatalf("expected timeout error when DBMS never reaches started")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"started"`) {
		t.Fatalf("expected target status %q in error, got: %v", "started", err)
	}
	if !strings.Contains(err.Error(), `"starting"`) {
		t.Fatalf("expected last-seen status %q in error, got: %v", "starting", err)
	}
	if getCalls.Load() < 1 {
		t.Fatalf("expected at least 1 GET poll before timeout, got %d", getCalls.Load())
	}
}

func TestStart_NoCredentialsJSONWrite(t *testing.T) {
	// REQ-F-025: Desktop owns the credential lifecycle. `desktop start` must
	// NOT touch ~/.neo4j/cli/credentials.json on any path.
	h := newStartHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","status":"started"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	credsPath := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json")
	before, err := afero.ReadFile(h.fs, credsPath)
	if err != nil {
		t.Fatalf("read credentials before: %v", err)
	}

	if err := h.run("start abc --wait --format json"); err != nil {
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

func TestStart_Annotated_Write(t *testing.T) {
	cfg := clicfg.NewConfig(mustTestFs(t), "test", clicfg.GlobalScope)
	parent := dbms.NewCmd(cfg)
	var startCmd *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "start" {
			startCmd = c
			break
		}
	}
	if startCmd == nil {
		t.Fatalf("start command not registered under desktop")
	}
	if startCmd.Annotations["write"] != "true" {
		t.Fatalf("start must be annotated write=true; got %v", startCmd.Annotations)
	}
}

func TestStart_PortFlagPropagatesToClientConstructor(t *testing.T) {
	h := newStartHelper(t)
	var gotPort int
	t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		gotPort = port
		return nil, errors.New("stop here; we already captured the port")
	}))

	_ = h.run("start abc --port 44230 --format json")
	if gotPort != 44230 {
		t.Fatalf("expected --port=44230 to propagate to client constructor, got %d", gotPort)
	}
}

// TestStart_PreFlight_BlocksWhenAnotherRunning is the canonical case from the
// task description: starting DBMS B while A is already `status=started` must
// exit non-zero BEFORE the lifecycle call is made, naming the running DBMS in
// the error and suggesting `desktop dbms stop <id>` or `--force`.
func TestStart_PreFlight_BlocksWhenAnotherRunning(t *testing.T) {
	h := newStartHelper(t)
	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[{"id":"running-id","name":"running-db"},{"id":"abc","name":"target"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/running-id":
			_, _ = w.Write([]byte(`{"id":"running-id","name":"running-db","status":"started"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"target","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			startCalls.Add(1)
			_, _ = w.Write([]byte(`"started"`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("start abc --format json")
	if err == nil {
		t.Fatalf("expected pre-flight conflict error")
	}
	if startCalls.Load() != 0 {
		t.Fatalf("pre-flight must run BEFORE POST /start; got %d start calls", startCalls.Load())
	}
	if !strings.Contains(err.Error(), "running-db") || !strings.Contains(err.Error(), "running-id") {
		t.Fatalf("expected error to name the running DBMS, got: %v", err)
	}
	if !strings.Contains(err.Error(), "desktop dbms stop running-id") {
		t.Fatalf("expected error to suggest `desktop dbms stop <id>`, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected error to mention --force escape hatch, got: %v", err)
	}
}

// TestStart_PreFlight_SameIDAlreadyRunningIsNoOp confirms `start abc` is
// idempotent when `abc` itself is the running DBMS — the pre-flight only
// blocks on OTHER running DBMSes.
func TestStart_PreFlight_SameIDAlreadyRunningIsNoOp(t *testing.T) {
	h := newStartHelper(t)
	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[{"id":"abc","name":"target"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"target","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			startCalls.Add(1)
			_, _ = w.Write([]byte(`"started"`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("start abc --format json"); err != nil {
		t.Fatalf("expected no error when target DBMS is itself the running one, got: %v", err)
	}
	if startCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 POST /start; got %d", startCalls.Load())
	}
}

// TestStart_PreFlight_NoOthersRunningProceeds confirms the pre-flight is a
// happy-path no-op when nothing is running.
func TestStart_PreFlight_NoOthersRunningProceeds(t *testing.T) {
	h := newStartHelper(t)
	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[{"id":"abc","name":"target"},{"id":"other","name":"other-db"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"target","status":"stopped"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/other":
			_, _ = w.Write([]byte(`{"id":"other","name":"other-db","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			startCalls.Add(1)
			_, _ = w.Write([]byte(`"started"`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("start abc --format json"); err != nil {
		t.Fatalf("expected start to proceed when no other DBMS is running, got: %v", err)
	}
	if startCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 POST /start; got %d", startCalls.Load())
	}
}

// TestStart_Force_NoConflictDoesNotStopAnything verifies `--force` is a no-op
// when nothing else is running: the pre-flight list still fires (now used to
// LOCATE a conflicting DBMS, not to assert absence) but no StopDbms call is
// issued and start proceeds normally.
func TestStart_Force_NoConflictDoesNotStopAnything(t *testing.T) {
	h := newStartHelper(t)
	var stopCalls atomic.Int32
	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			startCalls.Add(1)
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","status":"starting"}`))
		case r.Method == http.MethodPost:
			// Catch any unexpected POST /stop and fail.
			if strings.HasSuffix(r.URL.Path, "/stop") {
				stopCalls.Add(1)
				t.Errorf("--force with no conflict must NOT issue StopDbms; got %s", r.URL.Path)
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("start abc --force --format json"); err != nil {
		t.Fatalf("expected --force path to succeed when nothing else runs, got: %v", err)
	}
	if stopCalls.Load() != 0 {
		t.Fatalf("expected 0 POST /stop calls under --force with no conflict, got %d", stopCalls.Load())
	}
	if startCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 POST /start under --force, got %d", startCalls.Load())
	}
}

// TestStart_Force_StopsConflictingThenStarts is the canonical happy-path case
// for the redefined --force semantics: when A is running and we `start B
// --force`, we first StopDbms(A), poll until A.status=stopped, then StartDbms(B)
// and enrich. Call ordering is asserted via a recording handler.
func TestStart_Force_StopsConflictingThenStarts(t *testing.T) {
	h := newStartHelper(t)
	var (
		mu       sync.Mutex
		order    []string
		aStopped atomic.Bool
	)
	record := func(label string) {
		mu.Lock()
		order = append(order, label)
		mu.Unlock()
	}
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			record("LIST")
			_, _ = w.Write([]byte(`[{"id":"running-id","name":"running-db"},{"id":"abc","name":"target"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/running-id":
			record("GET-running")
			if aStopped.Load() {
				_, _ = w.Write([]byte(`{"id":"running-id","name":"running-db","status":"stopped"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"running-id","name":"running-db","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/running-id/stop":
			record("STOP-running")
			aStopped.Store(true)
			_, _ = w.Write([]byte(`"stopped"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			record("GET-abc")
			_, _ = w.Write([]byte(`{"id":"abc","name":"target","status":"starting"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			record("START-abc")
			_, _ = w.Write([]byte(`"started"`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("start abc --force --format json"); err != nil {
		t.Fatalf("expected --force path to succeed, got: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// Verify ordering: STOP-running happens BEFORE START-abc.
	stopIdx, startIdx := -1, -1
	for i, label := range order {
		if label == "STOP-running" && stopIdx == -1 {
			stopIdx = i
		}
		if label == "START-abc" && startIdx == -1 {
			startIdx = i
		}
	}
	if stopIdx == -1 {
		t.Fatalf("expected STOP-running call, order=%v", order)
	}
	if startIdx == -1 {
		t.Fatalf("expected START-abc call, order=%v", order)
	}
	if stopIdx > startIdx {
		t.Fatalf("STOP-running must happen BEFORE START-abc; order=%v", order)
	}
	// Stderr breadcrumb names the DBMS we stopped.
	if !strings.Contains(h.err.String(), "running-db") {
		t.Fatalf("expected stderr breadcrumb naming running-db, got: %q", h.err.String())
	}
	if !strings.Contains(h.err.String(), "Stopping") {
		t.Fatalf("expected 'Stopping' breadcrumb prefix, got: %q", h.err.String())
	}
}

// TestStart_Force_StopFailureAbortsStart asserts StopDbms failure surfaces a
// canonical error naming the DBMS we tried to stop and the subsequent start
// is NOT issued — no orphan side effects.
func TestStart_Force_StopFailureAbortsStart(t *testing.T) {
	h := newStartHelper(t)
	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[{"id":"running-id","name":"running-db"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/running-id":
			_, _ = w.Write([]byte(`{"id":"running-id","name":"running-db","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/running-id/stop":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"stop boom"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			startCalls.Add(1)
			t.Errorf("StartDbms must NOT be called when StopDbms fails")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("start abc --force --format json")
	if err == nil {
		t.Fatalf("expected error when StopDbms fails")
	}
	if !strings.Contains(err.Error(), "running-db") || !strings.Contains(err.Error(), "running-id") {
		t.Fatalf("expected error to name the DBMS we tried to stop, got: %v", err)
	}
	if startCalls.Load() != 0 {
		t.Fatalf("expected 0 POST /start calls when stop fails, got %d", startCalls.Load())
	}
}

// TestStart_Force_StopPollTimeoutAbortsStart asserts that when the stop poll
// times out (target never reaches status=stopped), the canonical timeout
// error names the DBMS and the subsequent start is NOT issued.
func TestStart_Force_StopPollTimeoutAbortsStart(t *testing.T) {
	h := newStartHelper(t)
	// Advance the clock past the deadline after one poll so we hit timeout in <1ms.
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	var ticks atomic.Int32
	t.Cleanup(dbms.SetCreateNowFnForTest(func() time.Time {
		if ticks.Add(1) == 1 {
			return start
		}
		return start.Add(1 * time.Minute)
	}))

	var startCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[{"id":"running-id","name":"running-db"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/running-id":
			// Always report still-started so the poll times out.
			_, _ = w.Write([]byte(`{"id":"running-id","name":"running-db","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/running-id/stop":
			_, _ = w.Write([]byte(`"stopped"`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			startCalls.Add(1)
			t.Errorf("StartDbms must NOT be called when stop poll times out")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("start abc --force --format json")
	if err == nil {
		t.Fatalf("expected timeout error when DBMS never reaches stopped")
	}
	if !strings.Contains(err.Error(), "running-db") {
		t.Fatalf("expected timeout error to name the DBMS, got: %v", err)
	}
	if startCalls.Load() != 0 {
		t.Fatalf("expected 0 POST /start calls when stop poll times out, got %d", startCalls.Load())
	}
}

func TestStart_DesktopUnreachable_ReturnsCanonicalError(t *testing.T) {
	h := newStartHelper(t)
	t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return nil, desktopclient.UnreachableError()
	}))

	err := h.run("start abc --format json")
	if err == nil {
		t.Fatalf("expected error when desktop is unreachable")
	}
	if !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Fatalf("expected canonical REQ-F-008 message, got: %v", err)
	}
}
