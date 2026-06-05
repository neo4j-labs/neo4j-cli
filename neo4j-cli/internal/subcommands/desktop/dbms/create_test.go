// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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

// createHelper wires dbms.NewCmd against an in-memory FS, with the shared
// `newDesktopClientFn` seam pinned to a desktopclient.Client backed by an
// httptest server. End-to-end: cobra flag parse → leaf RunE → desktopclient →
// httptest handler. Poll sleeps are pinned to no-op so the 30s timeout doesn't
// burn wall-clock time on the failure paths.
type createHelper struct {
	t   *testing.T
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newCreateHelper(t *testing.T) *createHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	// Pin the poll-loop sleep to a no-op so timeout tests are instantaneous.
	t.Cleanup(dbms.SetCreatePollSleepFnForTest(func(_ time.Duration) {}))
	// Default stdin=non-TTY so tests that omit --password get the explicit
	// "required when stdin is not a terminal" error path. TTY-specific
	// tests override this with SetCreateStdinIsTTYFnForTest.
	t.Cleanup(dbms.SetCreateStdinIsTTYFnForTest(func() bool { return false }))
	// Guard: any test that omits --password without explicitly arranging
	// the TTY-prompt path would silently hang on the real readPassword
	// call. Fail loudly instead.
	t.Cleanup(dbms.SetCreatePasswordReaderFnForTest(func() (string, error) {
		t.Fatalf("createPasswordReaderFn must not be called unless a TTY-prompt test arranges it")
		return "", nil
	}))
	return &createHelper{
		t:   t,
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

// withHandler swaps the `newDesktopClientFn` seam to a closure that returns
// a desktopclient.Client wired to the supplied httptest handler. The handler
// receives every request the leaf sends.
func (h *createHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-create"
		clientID = "cid-create"
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

func (h *createHelper) run(command string) error {
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

// readDbmssCreateBody captures the JSON body of the POST /dbmss request so we
// can assert REQ-F-002 — the body must contain `name, version, credentials`
// and MUST NOT contain `edition`.
func readDbmssCreateBody(t *testing.T, r *http.Request) map[string]any {
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

func TestCreate_RequiresName(t *testing.T) {
	h := newCreateHelper(t)
	err := h.run("create --version 5.21.0 --password p")
	if err == nil || !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("expected --name required error, got: %v", err)
	}
}

func TestCreate_RequiresPassword(t *testing.T) {
	// Non-TTY + no --password → usage error pointing the user at either
	// --password explicitly or running interactively. (Default helper
	// pins stdin to non-TTY.)
	h := newCreateHelper(t)
	err := h.run("create --name n --version 5.21.0")
	if err == nil || !strings.Contains(err.Error(), "--password is required when stdin is not a terminal") {
		t.Fatalf("expected non-TTY --password required error, got: %v", err)
	}
}

func TestCreate_PromptsForPasswordOnTTY(t *testing.T) {
	// TTY + no --password → prompt via createPasswordReaderFn; the prompted
	// value lands in the request body as `credentials` so the password
	// never appears in argv.
	h := newCreateHelper(t)
	t.Cleanup(dbms.SetCreateStdinIsTTYFnForTest(func() bool { return true }))
	t.Cleanup(dbms.SetCreatePasswordReaderFnForTest(func() (string, error) { return "tty-prompted", nil }))

	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			capturedBody = readDbmssCreateBody(t, r)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"creating"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	if err := h.run("create --name my-dbms --version 5.21.0 --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if capturedBody["credentials"] != "tty-prompted" {
		t.Fatalf("expected the TTY-prompted password in body, got %v", capturedBody["credentials"])
	}
}

func TestCreate_RejectsEmptyPromptedPassword(t *testing.T) {
	// TTY-prompt that returns empty must fail closed rather than create
	// the DBMS with an empty password.
	h := newCreateHelper(t)
	t.Cleanup(dbms.SetCreateStdinIsTTYFnForTest(func() bool { return true }))
	t.Cleanup(dbms.SetCreatePasswordReaderFnForTest(func() (string, error) { return "", nil }))

	err := h.run("create --name x --version 5.21.0")
	if err == nil || !strings.Contains(err.Error(), "empty password is not allowed") {
		t.Fatalf("expected empty-password rejection, got: %v", err)
	}
}

// TestCreate_PostBody_OmitsEdition is the REQ-F-002 / acceptance-criterion
// gate: the POST /dbmss body MUST contain {name, version, credentials} and
// MUST NEVER include `edition`. Even a future caller-side leak (e.g. someone
// adds an --edition flag) would fail this test.
func TestCreate_PostBody_OmitsEdition(t *testing.T) {
	h := newCreateHelper(t)
	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			capturedBody = readDbmssCreateBody(t, r)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.21.0","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.21.0","status":"started","connectionUri":"neo4j://localhost:7687"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("create --name my-dbms --version 5.21.0 --password supersecret --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}

	if capturedBody == nil {
		t.Fatalf("expected POST /dbmss to be called")
	}
	if capturedBody["name"] != "my-dbms" {
		t.Fatalf("expected name=my-dbms in body, got %v", capturedBody["name"])
	}
	if capturedBody["version"] != "5.21.0" {
		t.Fatalf("expected version=5.21.0 in body, got %v", capturedBody["version"])
	}
	if capturedBody["credentials"] != "supersecret" {
		t.Fatalf("expected credentials=supersecret in body, got %v", capturedBody["credentials"])
	}
	if _, hasEdition := capturedBody["edition"]; hasEdition {
		t.Fatalf("body must NOT contain `edition` (REQ-F-002): %v", capturedBody)
	}
}

func TestCreate_Wait_PollsUntilStarted(t *testing.T) {
	// `--wait` opts INTO polling: the rendered payload carries the polled
	// `status=started` + connectionUri, not just the CreateDbms response.
	h := newCreateHelper(t)
	// `started` only on the third GET — proves we actually loop.
	var getCount atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.21.0","status":"creating"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			n := getCount.Add(1)
			if n < 3 {
				_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"starting"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started","connectionUri":"neo4j://localhost:7687"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("create --name my-dbms --version 5.21.0 --password p --wait --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if getCount.Load() < 3 {
		t.Fatalf("expected at least 3 GET /dbmss/abc calls, got %d", getCount.Load())
	}
	// Verify the rendered payload carries status=started + connection URI.
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["status"] != "started" {
		t.Fatalf("expected status=started in output, got %v", got["status"])
	}
	if got["connection_uri"] != "neo4j://localhost:7687" {
		t.Fatalf("expected connection_uri in output, got %v", got["connection_uri"])
	}
}

func TestCreate_DefaultPath_DoesNotPoll(t *testing.T) {
	// Default behaviour: return after the POST /start call resolves; do NOT
	// poll for status=started. The rendered payload reflects the initial
	// CreateDbms response (status=creating). Matches the sibling `dbms start`
	// / `dbms stop` opt-in `--wait` convention.
	h := newCreateHelper(t)
	var getCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","version":"5.21.0","status":"creating"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			getCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"abc","status":"starting"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("create --name my-dbms --version 5.21.0 --password p --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if getCalls.Load() != 0 {
		t.Fatalf("default create must NOT poll GetDbms; got %d GETs", getCalls.Load())
	}
	// Output reflects the CreateDbms response (status=creating), not a poll
	// result.
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["status"] != "creating" {
		t.Fatalf("expected status=creating in output, got %v", got["status"])
	}
}

func TestCreate_PollTimeout_ExitsNonZeroWithLastStatus(t *testing.T) {
	// Drive the 30s deadline check via the createNowFn seam — advance the
	// clock past the deadline after exactly one poll so the leaf takes the
	// timeout branch deterministically in <1ms.
	h := newCreateHelper(t)
	start := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	var ticks atomic.Int32
	t.Cleanup(dbms.SetCreateNowFnForTest(func() time.Time {
		// 1st call: deadline computed from `start`.
		// Subsequent calls: report 1 minute later — past the 30s deadline.
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
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"creating"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			getCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"abc","status":"starting"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("create --name my-dbms --version 5.21.0 --password p --wait --format json")
	if err == nil {
		t.Fatalf("expected error when DBMS never reaches started under --wait")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), `"starting"`) {
		t.Fatalf("expected last-seen status %q in error, got: %v", "starting", err)
	}
	if getCalls.Load() < 1 {
		t.Fatalf("expected at least 1 GET poll before timeout, got %d", getCalls.Load())
	}
}

func TestCreate_NoCredentialsJSONWrite(t *testing.T) {
	// REQ-F-025: Desktop owns the credential lifecycle. `desktop create`
	// must NOT write to ~/.neo4j/cli/credentials.json on any path.
	h := newCreateHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	credsPath := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json")
	before, err := afero.ReadFile(h.fs, credsPath)
	if err != nil {
		t.Fatalf("read credentials before: %v", err)
	}

	if err := h.run("create --name my-dbms --version 5.21.0 --password supersecret --format json"); err != nil {
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

func TestCreate_Annotated_Write(t *testing.T) {
	// Sanity check: the create command must be annotated write=true so the
	// root enforcement fires on non-TTY callers.
	cfg := clicfg.NewConfig(mustTestFs(t), "test", clicfg.GlobalScope)
	parent := dbms.NewCmd(cfg)
	var createCmd *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "create" {
			createCmd = c
			break
		}
	}
	if createCmd == nil {
		t.Fatalf("create command not registered under desktop")
	}
	if createCmd.Annotations["write"] != "true" {
		t.Fatalf("create must be annotated write=true; got %v", createCmd.Annotations)
	}
}

func TestCreate_NoEditionFlag(t *testing.T) {
	// REQ-F-002: there is no --edition flag — Desktop 2 is enterprise-only
	// and exposing one would imply choice we don't have.
	cfg := clicfg.NewConfig(mustTestFs(t), "test", clicfg.GlobalScope)
	parent := dbms.NewCmd(cfg)
	var createCmd *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "create" {
			createCmd = c
			break
		}
	}
	if createCmd == nil {
		t.Fatalf("create command not registered under desktop")
	}
	if f := createCmd.Flag("edition"); f != nil {
		t.Fatalf("create must NOT register an --edition flag; got %+v", f)
	}
}

func TestCreate_PortFlagPropagatesToClientConstructor(t *testing.T) {
	h := newCreateHelper(t)
	var gotPort int
	t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		gotPort = port
		return nil, errors.New("stop here; we already captured the port")
	}))

	_ = h.run("create --name n --version 5.21.0 --password p --port 44230 --force --format json")
	if gotPort != 44230 {
		t.Fatalf("expected --port=44230 to propagate to client constructor, got %d", gotPort)
	}
}

func TestCreate_DesktopUnreachable_ReturnsCanonicalError(t *testing.T) {
	h := newCreateHelper(t)
	t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return nil, desktopclient.UnreachableError()
	}))

	err := h.run("create --name n --version 5.21.0 --password p --force --format json")
	if err == nil {
		t.Fatalf("expected error when desktop is unreachable")
	}
	if !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Fatalf("expected canonical REQ-F-008 message, got: %v", err)
	}
}

// TestCreate_PreFlight_BlocksWhenAnotherRunning is the canonical pre-flight
// case for create: a `status=started` DBMS at the time of `desktop create`
// must exit non-zero BEFORE the POST /dbmss call so no orphan DBMS is left on
// disk. Verified via a call counter on the create endpoint.
func TestCreate_PreFlight_BlocksWhenAnotherRunning(t *testing.T) {
	h := newCreateHelper(t)
	var createCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[{"id":"running-id","name":"running-db"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/running-id":
			_, _ = w.Write([]byte(`{"id":"running-id","name":"running-db","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			createCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"new","name":"new-db","status":"creating"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("create --name new-db --version 5.21.0 --password p --format json")
	if err == nil {
		t.Fatalf("expected pre-flight conflict error")
	}
	if createCalls.Load() != 0 {
		t.Fatalf("pre-flight must run BEFORE POST /dbmss; got %d create calls", createCalls.Load())
	}
	if !strings.Contains(err.Error(), "running-db") || !strings.Contains(err.Error(), "running-id") {
		t.Fatalf("expected error to name the running DBMS, got: %v", err)
	}
	if !strings.Contains(err.Error(), "new-db") {
		t.Fatalf("expected error to mention the requested name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "desktop dbms stop running-id") {
		t.Fatalf("expected error to suggest `desktop dbms stop <id>`, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected error to mention --force escape hatch, got: %v", err)
	}
}

// TestCreate_Force_NoConflictDoesNotStopAnything verifies `--force` is a no-op
// when nothing else is running: the pre-flight list still fires (it's how we
// LOCATE conflicts) but no StopDbms call is issued and create proceeds.
func TestCreate_Force_NoConflictDoesNotStopAnything(t *testing.T) {
	h := newCreateHelper(t)
	var stopCalls atomic.Int32
	var createCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			createCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"abc","name":"new-db","status":"creating"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","status":"starting"}`))
		case r.Method == http.MethodPost:
			if strings.HasSuffix(r.URL.Path, "/stop") {
				stopCalls.Add(1)
				t.Errorf("--force with no conflict must NOT issue StopDbms; got %s", r.URL.Path)
			}
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("create --name new-db --version 5.21.0 --password p --force --format json"); err != nil {
		t.Fatalf("expected --force path to succeed with no conflict, got: %v", err)
	}
	if stopCalls.Load() != 0 {
		t.Fatalf("expected 0 POST /stop calls under --force with no conflict, got %d", stopCalls.Load())
	}
	if createCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 POST /dbmss under --force, got %d", createCalls.Load())
	}
}

// TestCreate_Force_StopsConflictingBeforeCreate is the canonical happy-path
// case for the redefined --force semantics: when A is running and we
// `desktop create --force`, we first StopDbms(A), poll until stopped, then
// CreateDbms and StartDbms the new one. Critical: stop happens BEFORE create.
func TestCreate_Force_StopsConflictingBeforeCreate(t *testing.T) {
	h := newCreateHelper(t)
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
			_, _ = w.Write([]byte(`[{"id":"running-id","name":"running-db"}]`))
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
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			record("CREATE")
			_, _ = w.Write([]byte(`{"id":"new","name":"new-db","status":"creating"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/new/start":
			record("START-new")
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/new":
			record("GET-new")
			_, _ = w.Write([]byte(`{"id":"new","name":"new-db","status":"starting"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("create --name new-db --version 5.21.0 --password p --force --format json"); err != nil {
		t.Fatalf("expected --force path to succeed, got: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	// Critical ordering: STOP-running must happen BEFORE CREATE so no orphan
	// DBMS lands on disk if the stop fails.
	stopIdx, createIdx := -1, -1
	for i, label := range order {
		if label == "STOP-running" && stopIdx == -1 {
			stopIdx = i
		}
		if label == "CREATE" && createIdx == -1 {
			createIdx = i
		}
	}
	if stopIdx == -1 {
		t.Fatalf("expected STOP-running call, order=%v", order)
	}
	if createIdx == -1 {
		t.Fatalf("expected CREATE call, order=%v", order)
	}
	if stopIdx > createIdx {
		t.Fatalf("STOP-running must happen BEFORE CREATE; order=%v", order)
	}
	if !strings.Contains(h.err.String(), "running-db") {
		t.Fatalf("expected stderr breadcrumb naming running-db, got: %q", h.err.String())
	}
	if !strings.Contains(h.err.String(), "Stopping") {
		t.Fatalf("expected 'Stopping' breadcrumb prefix, got: %q", h.err.String())
	}
}

// TestCreate_Force_StopFailureAbortsCreate asserts StopDbms failure under
// --force surfaces a canonical error naming the DBMS and the subsequent
// CreateDbms is NOT issued — no orphan DBMS lands on disk.
func TestCreate_Force_StopFailureAbortsCreate(t *testing.T) {
	h := newCreateHelper(t)
	var createCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[{"id":"running-id","name":"running-db"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/running-id":
			_, _ = w.Write([]byte(`{"id":"running-id","name":"running-db","status":"started"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/running-id/stop":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"stop boom"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			createCalls.Add(1)
			t.Errorf("CreateDbms must NOT be called when StopDbms fails")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("create --name new-db --version 5.21.0 --password p --force --format json")
	if err == nil {
		t.Fatalf("expected error when StopDbms fails under --force")
	}
	if !strings.Contains(err.Error(), "running-db") || !strings.Contains(err.Error(), "running-id") {
		t.Fatalf("expected error to name the DBMS we tried to stop, got: %v", err)
	}
	if createCalls.Load() != 0 {
		t.Fatalf("expected 0 POST /dbmss calls when stop fails, got %d", createCalls.Load())
	}
}

// TestCreate_VersionOmitted_PicksLatestStable verifies REQ-F-030: when
// `--version` is omitted, the leaf calls `GET /dbmss/versions`, picks the
// highest stable enterprise entry, POSTs the picked version, and emits a
// stderr breadcrumb naming the version + origin.
func TestCreate_VersionOmitted_PicksLatestStable(t *testing.T) {
	h := newCreateHelper(t)
	var (
		versionsCalls atomic.Int32
		capturedBody  map[string]any
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
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			capturedBody = readDbmssCreateBody(t, r)
			_, _ = w.Write([]byte(`{"id":"abc","name":"new-db","version":"2026.04.0","status":"creating"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"new-db","version":"2026.04.0","status":"started"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("create --name new-db --password p --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if versionsCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 GET /dbmss/versions, got %d", versionsCalls.Load())
	}
	if capturedBody == nil || capturedBody["version"] != "2026.04.0" {
		t.Fatalf("expected POST body version=2026.04.0 (the highest stable enterprise entry), got body=%+v", capturedBody)
	}
	stderr := h.err.String()
	if !strings.Contains(stderr, "Using Neo4j enterprise 2026.04.0") {
		t.Fatalf("expected stderr breadcrumb naming picked version, got: %q", stderr)
	}
	if !strings.Contains(stderr, "(cached)") {
		t.Fatalf("expected stderr breadcrumb naming origin, got: %q", stderr)
	}
}

// TestCreate_VersionSupplied_SkipsVersionsLookup verifies that when the user
// pins `--version`, the CLI does NOT call `GET /dbmss/versions` — the
// explicit value is honoured verbatim.
func TestCreate_VersionSupplied_SkipsVersionsLookup(t *testing.T) {
	h := newCreateHelper(t)
	var (
		versionsCalls atomic.Int32
		capturedBody  map[string]any
	)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/versions":
			versionsCalls.Add(1)
			t.Errorf("--version supplied → must NOT call GET /dbmss/versions")
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			capturedBody = readDbmssCreateBody(t, r)
			_, _ = w.Write([]byte(`{"id":"abc","name":"new-db","version":"5.26.0","status":"creating"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"new-db","version":"5.26.0","status":"started"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("create --name new-db --version 5.26.0 --password p --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if versionsCalls.Load() != 0 {
		t.Fatalf("expected 0 GET /dbmss/versions calls when --version is supplied, got %d", versionsCalls.Load())
	}
	if capturedBody == nil || capturedBody["version"] != "5.26.0" {
		t.Fatalf("expected POST body version=5.26.0 (verbatim), got body=%+v", capturedBody)
	}
	if h.err.String() != "" {
		t.Fatalf("expected no stderr breadcrumb when --version supplied, got: %q", h.err.String())
	}
}

// TestCreate_VersionOmitted_EmptyResponse_Fatal verifies the no-stable-versions
// path: empty versions catalog → fatal error pointing the user at
// `--version <vX>`, no POST to /dbmss.
func TestCreate_VersionOmitted_EmptyResponse_Fatal(t *testing.T) {
	h := newCreateHelper(t)
	var createCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/versions":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			createCalls.Add(1)
			t.Errorf("must NOT POST /dbmss when no stable version is selectable")
		default:
			// /dbmss GET for pre-flight may fire; tolerate.
			if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("create --name n --password p --format json")
	if err == nil {
		t.Fatalf("expected fatal error when versions response is empty")
	}
	if !strings.Contains(err.Error(), "--version") {
		t.Fatalf("expected error to point at --version, got: %v", err)
	}
	if createCalls.Load() != 0 {
		t.Fatalf("expected 0 POST /dbmss when versions response is empty, got %d", createCalls.Load())
	}
}

// TestCreate_VersionOmitted_AllPrerelease_Fatal verifies pre-release filtering:
// if every catalog entry has a semver pre-release suffix (e.g. `-alpha`),
// the leaf surfaces the same fatal-error hint as for the empty response.
func TestCreate_VersionOmitted_AllPrerelease_Fatal(t *testing.T) {
	h := newCreateHelper(t)
	var createCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/versions":
			_, _ = w.Write([]byte(`[
				{"dist":"/cache/neo4j-enterprise-2026.05.0-alpha","edition":"enterprise","origin":"cached","version":"2026.05.0-alpha"},
				{"dist":"https://dist.neo4j.org/neo4j-enterprise-2026.06.0-rc1.tar.gz","edition":"enterprise","origin":"online","version":"2026.06.0-rc1"}
			]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			createCalls.Add(1)
			t.Errorf("must NOT POST /dbmss when only pre-releases are available")
		default:
			if r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("create --name n --password p --format json")
	if err == nil {
		t.Fatalf("expected fatal error when only pre-releases are available")
	}
	if !strings.Contains(err.Error(), "--version") {
		t.Fatalf("expected error to point at --version, got: %v", err)
	}
	if createCalls.Load() != 0 {
		t.Fatalf("expected 0 POST /dbmss when only pre-releases are available, got %d", createCalls.Load())
	}
}

// TestPickLatestStableEnterprise_TieBreakCachedWins is a focused unit test for
// the tie-break rule: same Version → cached origin wins over online so create
// avoids a dist.neo4j.org download.
func TestPickLatestStableEnterprise_TieBreakCachedWins(t *testing.T) {
	versions := []desktopclient.DbmsVersion{
		{Version: "2026.04.0", Edition: "enterprise", Origin: "online", Dist: "https://dist/online"},
		{Version: "2026.04.0", Edition: "enterprise", Origin: "cached", Dist: "/cache/local"},
	}
	got, err := dbms.PickLatestStableEnterpriseForTest(versions)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.Origin != "cached" {
		t.Fatalf("expected cached entry to win the tie, got origin=%q", got.Origin)
	}
}

// TestPickLatestStableEnterprise_SemverOverLexical guards against the obvious
// lexical bug: `2026.04.0 > 5.26.1` under semver despite `5` > `2` lexically.
func TestPickLatestStableEnterprise_SemverOverLexical(t *testing.T) {
	versions := []desktopclient.DbmsVersion{
		{Version: "5.26.1", Edition: "enterprise", Origin: "online"},
		{Version: "2026.04.0", Edition: "enterprise", Origin: "cached"},
		{Version: "5.26.0", Edition: "enterprise", Origin: "online"},
	}
	got, err := dbms.PickLatestStableEnterpriseForTest(versions)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got.Version != "2026.04.0" {
		t.Fatalf("expected 2026.04.0 (highest semver), got %q", got.Version)
	}
}

// mustTestFs returns a fresh in-memory FS pre-seeded by testfs.GetTestFs. Used
// by the structural tests above that don't need an httptest server.
func mustTestFs(t *testing.T) afero.Fs {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	return fs
}
