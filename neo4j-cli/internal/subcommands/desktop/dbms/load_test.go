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
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dataset"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/dbms"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// loadHelper wires `dbms.NewCmd` against an in-memory FS with the desktop
// client seam pointed at an httptest server and the dataset Resolve/Download
// seams stubbed with deterministic fakes. End-to-end: cobra flag parse → leaf
// RunE → desktopclient → httptest handler, with the dataset layer mocked so no
// network is touched.
type loadHelper struct {
	t   *testing.T
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newLoadHelper(t *testing.T) *loadHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	t.Cleanup(dbms.SetCreatePollSleepFnForTest(func(_ time.Duration) {}))
	// Non-TTY by default so an omitted --password on the new path surfaces the
	// usage error rather than hanging on a real password read.
	t.Cleanup(dbms.SetCreateStdinIsTTYFnForTest(func() bool { return false }))
	t.Cleanup(dbms.SetCreatePasswordReaderFnForTest(func() (string, error) {
		t.Fatalf("createPasswordReaderFn must not be called unless a TTY-prompt test arranges it")
		return "", nil
	}))
	return &loadHelper{t: t, out: &bytes.Buffer{}, err: &bytes.Buffer{}, fs: fs}
}

func (h *loadHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-load"
		clientID = "cid-load"
	)
	h.t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return clientID }))
	h.t.Cleanup(desktopclient.SetNowFnForTest(func() time.Time { return time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC) }))
	srv := httptest.NewServer(handler)
	h.t.Cleanup(srv.Close)
	h.t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return desktopclient.NewClient(desktopclient.ProbeResult{Origin: srv.URL}, salt)
	}))
	return srv
}

// stubDataset pins the dataset seams to a fixed Spec/dump path. Returns a slice
// recording every (ownerRepo, version) Resolve was called with.
func (h *loadHelper) stubDataset(spec dataset.Spec) *[]string {
	h.t.Helper()
	var resolveCalls []string
	dbms.StubDatasetSeams(
		h.t,
		func(_ context.Context, ownerRepo, version string) (dataset.Spec, error) {
			resolveCalls = append(resolveCalls, ownerRepo+"@"+version)
			return spec, nil
		},
		func(_ context.Context, _ dataset.Spec, _ int64) (string, func(), error) {
			return "/tmp/fake-dump/neo4j.dump", func() {}, nil
		},
	)
	return &resolveCalls
}

func (h *loadHelper) run(command string) error {
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

// recordingHandler returns a handler that records requests (method+path, and
// body for the load-dump call) and replies with canned successes for the load
// flow's routes. seenStatus is the status GetDbms reports.
type loadRequests struct {
	mu           sync.Mutex
	paths        []string
	loadDumpBody map[string]any
	plugins      []string
	createBody   map[string]any
}

func (r *loadRequests) record(p string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, p)
}

func (r *loadRequests) saw(p string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, got := range r.paths {
		if got == p {
			return true
		}
	}
	return false
}

func decodeBody(t *testing.T, req *http.Request) map[string]any {
	t.Helper()
	b, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("body json: %v (raw: %s)", err, string(b))
	}
	return out
}

func TestLoad_RequiresExactlyOneTarget(t *testing.T) {
	h := newLoadHelper(t)
	err := h.run("load neo4j-graph-examples/movies")
	if err == nil || !strings.Contains(err.Error(), "exactly one of --dbms-id or --name is required") {
		t.Fatalf("expected exactly-one usage error, got: %v", err)
	}
}

func TestLoad_TargetsAreMutuallyExclusive(t *testing.T) {
	h := newLoadHelper(t)
	err := h.run("load neo4j-graph-examples/movies --dbms-id abc --name movies")
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually-exclusive usage error, got: %v", err)
	}
}

func TestLoad_ExistingRequiresForce(t *testing.T) {
	h := newLoadHelper(t)
	err := h.run("load neo4j-graph-examples/movies --dbms-id abc")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected --force usage error, got: %v", err)
	}
}

func TestLoad_NewRequiresPasswordOnNonTTY(t *testing.T) {
	h := newLoadHelper(t)
	err := h.run("load neo4j-graph-examples/movies --name movies")
	if err == nil || !strings.Contains(err.Error(), "--password is required when stdin is not a terminal") {
		t.Fatalf("expected non-TTY --password error, got: %v", err)
	}
}

func TestLoad_ExistingDbms_StopsLoadsInstallsStarts(t *testing.T) {
	h := newLoadHelper(t)
	resolveCalls := h.stubDataset(dataset.Spec{
		Owner: "neo4j-graph-examples", Repo: "movies", Branch: "main",
		DumpPath: "data/movies.dump", Plugins: []string{"apoc"},
	})

	var rec loadRequests
	var stopped bool
	var stateMu sync.Mutex
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		p := r.Method + " " + r.URL.Path
		rec.record(p)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			stateMu.Lock()
			status := "started"
			if stopped {
				status = "stopped"
			}
			stateMu.Unlock()
			_, _ = w.Write([]byte(`{"id":"abc","name":"db1","version":"5.26.1","status":"` + status + `"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss/abc/stop":
			stateMu.Lock()
			stopped = true
			stateMu.Unlock()
			_, _ = w.Write([]byte(`"stopped"`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/databases/neo4j/load-dump":
			rec.mu.Lock()
			rec.loadDumpBody = decodeBody(t, r)
			rec.mu.Unlock()
			_, _ = w.Write([]byte(`"loaded"`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/plugins/install":
			body := decodeBody(t, r)
			rec.mu.Lock()
			rec.plugins = append(rec.plugins, body["pluginName"].(string))
			rec.mu.Unlock()
			_, _ = w.Write([]byte(`{"name":"apoc"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		default:
			t.Errorf("unexpected request: %s", p)
		}
	})

	if err := h.run("load neo4j-graph-examples/movies --dbms-id abc --force"); err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, h.err.String())
	}

	if got := (*resolveCalls); len(got) != 1 || got[0] != "neo4j-graph-examples/movies@5.26.1" {
		t.Fatalf("resolve should use the existing DBMS version, got: %v", got)
	}
	for _, want := range []string{
		"GET /fastify/api/dbmss/abc",
		"POST /fastify/api/desktop/dbmss/abc/stop",
		"POST /fastify/api/dbmss/abc/databases/neo4j/load-dump",
		"POST /fastify/api/dbmss/abc/plugins/install",
		"POST /fastify/api/dbmss/abc/start",
	} {
		if !rec.saw(want) {
			t.Errorf("expected request %q to have been made; paths: %v", want, rec.paths)
		}
	}
	if rec.loadDumpBody["overwrite"] != true {
		t.Errorf("load-dump overwrite should be true, got: %v", rec.loadDumpBody["overwrite"])
	}
	if rec.loadDumpBody["sourceFilePath"] != "/tmp/fake-dump/neo4j.dump" {
		t.Errorf("load-dump sourceFilePath mismatch: %v", rec.loadDumpBody["sourceFilePath"])
	}
	if len(rec.plugins) != 1 || rec.plugins[0] != "apoc" {
		t.Errorf("expected apoc plugin installed, got: %v", rec.plugins)
	}
}

func TestLoad_ExistingDbms_StoppedSkipsStop(t *testing.T) {
	h := newLoadHelper(t)
	h.stubDataset(dataset.Spec{Owner: "o", Repo: "r", Branch: "main", DumpPath: "d.dump"})

	var rec loadRequests
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r.Method + " " + r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"db1","version":"5.26.1","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/databases/neo4j/load-dump":
			_, _ = w.Write([]byte(`"loaded"`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("load o/r --dbms-id abc --force"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if rec.saw("POST /fastify/api/desktop/dbmss/abc/stop") {
		t.Error("stop should be skipped when the DBMS is already stopped")
	}
}

func TestLoad_NewDbms_CreatesLoadsInstallsStarts(t *testing.T) {
	h := newLoadHelper(t)
	resolveCalls := h.stubDataset(dataset.Spec{
		Owner: "neo4j-graph-examples", Repo: "movies", Branch: "main",
		DumpPath: "data/movies.dump", Plugins: []string{"apoc"},
	})

	var rec loadRequests
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		p := r.Method + " " + r.URL.Path
		rec.record(p)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			rec.mu.Lock()
			rec.createBody = decodeBody(t, r)
			rec.mu.Unlock()
			_, _ = w.Write([]byte(`{"id":"new1","name":"movies","version":"5.26.1","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/new1/databases/neo4j/load-dump":
			_, _ = w.Write([]byte(`"loaded"`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/new1/plugins/install":
			body := decodeBody(t, r)
			rec.mu.Lock()
			rec.plugins = append(rec.plugins, body["pluginName"].(string))
			rec.mu.Unlock()
			_, _ = w.Write([]byte(`{"name":"apoc"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/new1/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/new1":
			_, _ = w.Write([]byte(`{"id":"new1","name":"movies","version":"5.26.1","status":"started"}`))
		default:
			t.Errorf("unexpected request: %s", p)
		}
	})

	if err := h.run("load neo4j-graph-examples/movies --name movies --version 5.26.1 --password supersecret"); err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, h.err.String())
	}

	if got := (*resolveCalls); len(got) != 1 || got[0] != "neo4j-graph-examples/movies@5.26.1" {
		t.Fatalf("resolve should use --version, got: %v", got)
	}
	if rec.createBody["version"] != "5.26.1" || rec.createBody["credentials"] != "supersecret" {
		t.Errorf("create body mismatch: %v", rec.createBody)
	}
	for _, want := range []string{
		"POST /fastify/api/desktop/dbmss",
		"POST /fastify/api/dbmss/new1/databases/neo4j/load-dump",
		"POST /fastify/api/dbmss/new1/plugins/install",
		"POST /fastify/api/dbmss/new1/start",
	} {
		if !rec.saw(want) {
			t.Errorf("expected request %q; paths: %v", want, rec.paths)
		}
	}
}

func TestLoad_NewDbms_AutoPicksVersionWhenOmitted(t *testing.T) {
	h := newLoadHelper(t)
	resolveCalls := h.stubDataset(dataset.Spec{Owner: "o", Repo: "r", Branch: "main", DumpPath: "d.dump"})

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/versions":
			_, _ = w.Write([]byte(`[{"edition":"enterprise","version":"5.26.1","origin":"cached"},{"edition":"enterprise","version":"5.20.0","origin":"online"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/desktop/dbmss":
			_, _ = w.Write([]byte(`{"id":"new1","name":"r","version":"5.26.1","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/new1/databases/neo4j/load-dump":
			_, _ = w.Write([]byte(`"loaded"`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/new1/start":
			_, _ = w.Write([]byte(`"started"`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/new1":
			_, _ = w.Write([]byte(`{"id":"new1","name":"r","version":"5.26.1","status":"started"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("load o/r --name r --password p"); err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, h.err.String())
	}
	if got := (*resolveCalls); len(got) != 1 || got[0] != "o/r@5.26.1" {
		t.Fatalf("resolve should use the auto-picked latest stable enterprise version, got: %v", got)
	}
}

func TestLoad_DatabaseOverride(t *testing.T) {
	h := newLoadHelper(t)
	h.stubDataset(dataset.Spec{Owner: "o", Repo: "r", Branch: "main", DumpPath: "d.dump"})

	var rec loadRequests
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r.Method + " " + r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"db1","version":"5.26.1","status":"stopped"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/databases/movies/load-dump":
			_, _ = w.Write([]byte(`"loaded"`))
		case r.Method == http.MethodPost && r.URL.Path == "/fastify/api/dbmss/abc/start":
			_, _ = w.Write([]byte(`"started"`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("load o/r --dbms-id abc --force --database movies"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !rec.saw("POST /fastify/api/dbmss/abc/databases/movies/load-dump") {
		t.Errorf("expected load-dump against the movies database; paths: %v", rec.paths)
	}
}
