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
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/commands/desktop/dbms"
	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/neo4j/cli/internal/flags"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// updateHelper mirrors upgradeHelper: dbms.NewCmd wired against an in-memory
// FS, with `desktopclient.Connect` pinned to a desktopclient.Client backed by an
// httptest server.
type updateHelper struct {
	t   *testing.T
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newUpdateHelper(t *testing.T) *updateHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	return &updateHelper{
		t:   t,
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

func (h *updateHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-update"
		clientID = "cid-update"
	)
	h.t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return clientID }))
	h.t.Cleanup(desktopclient.SetNowFnForTest(func() time.Time { return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC) }))
	srv := httptest.NewServer(handler)
	h.t.Cleanup(srv.Close)

	h.t.Cleanup(desktopclient.SetConnectFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return desktopclient.NewClient(desktopclient.ProbeResult{Origin: srv.URL}, salt)
	}))
	return srv
}

func (h *updateHelper) run(command string) error {
	h.t.Helper()
	args, err := shlex.Split(command)
	if err != nil {
		h.t.Fatalf("shlex: %v", err)
	}
	cfg := clicfg.NewConfig(h.fs, "test")
	cmd := dbms.NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetArgs(args)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)
	return cmd.Execute()
}

// readUpdateBody decodes the PATCH /dbmss/:id JSON body.
func readUpdateBody(t *testing.T, r *http.Request) map[string]any {
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

func TestUpdate_RequiresID(t *testing.T) {
	h := newUpdateHelper(t)
	if err := h.run("update"); err == nil {
		t.Fatalf("expected error when <id> is missing")
	}
}

// TestUpdate_NoFlags_UsageError asserts the no-flag gate: a usage error is
// returned and NO Desktop round-trip happens (an empty PATCH would be a
// silent no-op server-side).
func TestUpdate_NoFlags_UsageError(t *testing.T) {
	h := newUpdateHelper(t)
	var anyCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		anyCalls.Add(1)
		t.Errorf("no Desktop call expected without update flags; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run("update abc --format json")
	if err == nil {
		t.Fatalf("expected usage error when no update flag is supplied")
	}
	if !strings.Contains(err.Error(), "--name") || !strings.Contains(err.Error(), "--tags") {
		t.Fatalf("expected error to list the update flags, got: %v", err)
	}
	if anyCalls.Load() != 0 {
		t.Fatalf("expected 0 Desktop calls, got %d", anyCalls.Load())
	}
}

// TestUpdate_NameAndDescription asserts the PATCH body contains ONLY the
// supplied keys and the rendered JSON carries the full snake_case projection
// (including projects/tags).
func TestUpdate_NameAndDescription(t *testing.T) {
	h := newUpdateHelper(t)
	var captured map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/fastify/api/dbmss/abc":
			captured = readUpdateBody(t, r)
			_, _ = w.Write([]byte(`{"id":"abc","name":"renamed","description":"d","version":"5.26.1","status":"stopped","connectionUri":"neo4j://localhost:7687","projects":["p1"],"tags":["t1","t2"]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("update abc --name renamed --description d --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if captured["name"] != "renamed" || captured["description"] != "d" {
		t.Fatalf("expected name+description in body, got %+v", captured)
	}
	if _, ok := captured["tags"]; ok {
		t.Fatalf("tags must NOT be sent when --tags is absent, got %+v", captured)
	}
	if _, ok := captured["projects"]; ok {
		t.Fatalf("projects must NOT be sent when --project is absent, got %+v", captured)
	}

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "abc" || got["name"] != "renamed" || got["connection_uri"] != "neo4j://localhost:7687" {
		t.Fatalf("unexpected rendered row: %+v", got)
	}
	projects, ok := got["projects"].([]any)
	if !ok || len(projects) != 1 || projects[0] != "p1" {
		t.Fatalf("expected projects=[p1] in rendered row, got %+v", got["projects"])
	}
	tags, ok := got["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "t1" || tags[1] != "t2" {
		t.Fatalf("expected tags=[t1 t2] in rendered row, got %+v", got["tags"])
	}
}

// TestUpdate_ReplaceSemantics_NameResolution asserts `--project`/`--tags`
// values become the ENTIRE array in the PATCH body, with names resolved
// against Desktop's catalogs (GET /projects, GET /tags) before the PATCH.
func TestUpdate_ReplaceSemantics_NameResolution(t *testing.T) {
	h := newUpdateHelper(t)
	var (
		captured      map[string]any
		projectsCalls atomic.Int32
		tagsCalls     atomic.Int32
	)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/projects":
			projectsCalls.Add(1)
			_, _ = w.Write([]byte(`{"projects":[{"id":"p1","name":"Customer 360"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/tags":
			tagsCalls.Add(1)
			_, _ = w.Write([]byte(`{"tags":[{"id":"t1","name":"prod"},{"id":"t2","name":"eu"}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/fastify/api/dbmss/abc":
			captured = readUpdateBody(t, r)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","projects":["p1"],"tags":["t1","t2"]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run(`update abc --project "Customer 360" --tags prod,eu --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if projectsCalls.Load() != 1 || tagsCalls.Load() != 1 {
		t.Fatalf("expected 1 GET /projects and 1 GET /tags, got %d/%d", projectsCalls.Load(), tagsCalls.Load())
	}
	projects, ok := captured["projects"].([]any)
	if !ok || len(projects) != 1 || projects[0] != "p1" {
		t.Fatalf("expected body projects=[p1] (name resolved), got %+v", captured["projects"])
	}
	tags, ok := captured["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "t1" || tags[1] != "t2" {
		t.Fatalf("expected body tags=[t1 t2] (names resolved), got %+v", captured["tags"])
	}
}

// TestUpdate_UUIDPassthrough asserts UUID values skip catalog resolution
// entirely and land verbatim in the PATCH body.
func TestUpdate_UUIDPassthrough(t *testing.T) {
	h := newUpdateHelper(t)
	const (
		projectUUID = "f4e2f3c0-1111-2222-3333-444455556666"
		tagUUID     = "a1b2c3d4-5555-6666-7777-888899990000"
	)
	var (
		captured     map[string]any
		catalogCalls atomic.Int32
	)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && (r.URL.Path == "/fastify/api/projects" || r.URL.Path == "/fastify/api/tags"):
			catalogCalls.Add(1)
			t.Errorf("catalog fetch must NOT happen for all-UUID input; got %s %s", r.Method, r.URL.Path)
		case r.Method == http.MethodPatch && r.URL.Path == "/fastify/api/dbmss/abc":
			captured = readUpdateBody(t, r)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("update abc --project " + projectUUID + " --tags " + tagUUID + " --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if catalogCalls.Load() != 0 {
		t.Fatalf("expected 0 catalog fetches, got %d", catalogCalls.Load())
	}
	projects, ok := captured["projects"].([]any)
	if !ok || len(projects) != 1 || projects[0] != projectUUID {
		t.Fatalf("expected body projects=[%s] verbatim, got %+v", projectUUID, captured["projects"])
	}
	tags, ok := captured["tags"].([]any)
	if !ok || len(tags) != 1 || tags[0] != tagUUID {
		t.Fatalf("expected body tags=[%s] verbatim, got %+v", tagUUID, captured["tags"])
	}
}

// TestUpdate_ClearTags asserts `--tags ""` collapses to an empty array in the
// PATCH body (replace semantics: clearing the set).
func TestUpdate_ClearTags(t *testing.T) {
	h := newUpdateHelper(t)
	var captured map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/fastify/api/dbmss/abc":
			captured = readUpdateBody(t, r)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run(`update abc --tags "" --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	tags, ok := captured["tags"].([]any)
	if !ok {
		t.Fatalf("expected body tags to be an array, got %+v", captured["tags"])
	}
	if len(tags) != 0 {
		t.Fatalf("expected body tags=[] (cleared), got %+v", tags)
	}
}

func TestUpdate_Annotated_Write(t *testing.T) {
	cfg := clicfg.NewConfig(mustTestFs(t), "test")
	parent := dbms.NewCmd(cfg)
	var updateCmd *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "update" {
			updateCmd = c
			break
		}
	}
	if updateCmd == nil {
		t.Fatalf("update command not registered under dbms")
	}
	if updateCmd.Annotations["write"] != "true" {
		t.Fatalf("update must be annotated write=true; got %v", updateCmd.Annotations)
	}
}
