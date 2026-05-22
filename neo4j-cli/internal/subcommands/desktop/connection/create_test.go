// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package connection_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/connection"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// createHelper wires `desktop.NewCmd` against an in-memory FS, with the
// connection subtree's `newDesktopClientFn` seam pinned to a desktopclient.
// Client backed by an httptest server. Mirrors the createHelper pattern in
// the parent `desktop` package's tests so the connection leaves get the same
// hermetic end-to-end coverage.
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
	// Default TTY=true so each test opts INTO the non-TTY branch explicitly.
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return true }))
	// Default password reader fails the test if invoked unexpectedly — the
	// passing tests that rely on it override it explicitly.
	t.Cleanup(connection.SetPasswordReaderFnForTest(func() (string, error) {
		t.Fatalf("passwordReaderFn must not be called unless --password is omitted on a TTY")
		return "", nil
	}))
	return &createHelper{
		t:   t,
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

// withHandler swaps the connection subtree's client-constructor seam to a
// closure returning a desktopclient.Client wired to the supplied httptest
// handler.
func (h *createHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-conn-create"
		clientID = "cid-conn-create"
	)
	h.t.Cleanup(desktopclient.SetUUIDFnForTest(func() string { return clientID }))
	h.t.Cleanup(desktopclient.SetNowFnForTest(func() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) }))
	srv := httptest.NewServer(handler)
	h.t.Cleanup(srv.Close)

	h.t.Cleanup(connection.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
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
	cmd := desktop.NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetArgs(args)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)
	return cmd.Execute()
}

// readPostBody captures the JSON body of POST /connections so we can assert
// the wire shape — minimum fields populated, no unexpected extras.
func readPostBody(t *testing.T, r *http.Request) map[string]any {
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

// TestCreate_RequiresName / Uri / Username covers cobra's MarkFlagRequired
// enforcement: missing any of the three mandatory flags is a usage error
// before any HTTP call.
func TestCreate_RequiresFlags(t *testing.T) {
	cases := []struct {
		cmdline string
		want    string
	}{
		{`connection create --uri neo4j://x --username neo4j --password p`, `"name"`},
		{`connection create --name n --username neo4j --password p`, `"uri"`},
		{`connection create --name n --uri neo4j://x --password p`, `"username"`},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			h := newCreateHelper(t)
			err := h.run(tc.cmdline)
			if err == nil {
				t.Fatalf("expected required-flag error")
			}
			if !strings.Contains(err.Error(), "required") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected required-flag error mentioning %s, got: %v", tc.want, err)
			}
		})
	}
}

// TestCreate_SuccessfulCreate covers the happy path: all flags supplied,
// POST /connections wire shape is exactly the four mandatory fields, the
// returned Connection is rendered.
func TestCreate_SuccessfulCreate(t *testing.T) {
	h := newCreateHelper(t)
	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/fastify/api/connections" {
			capturedBody = readPostBody(t, r)
			_, _ = w.Write([]byte(`{"id":"f4e2f3c0-1111-2222-3333-444455556666","name":"aura-prod","connectionUri":"neo4j+s://abc.databases.neo4j.io","project":"proj-1"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`connection create --name aura-prod --uri neo4j+s://abc.databases.neo4j.io --username neo4j --password supersecret --format json`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if capturedBody == nil {
		t.Fatalf("expected POST /connections to be called")
	}
	if capturedBody["name"] != "aura-prod" {
		t.Fatalf("name mismatch in body: %v", capturedBody["name"])
	}
	if capturedBody["connectionUri"] != "neo4j+s://abc.databases.neo4j.io" {
		t.Fatalf("connectionUri mismatch in body: %v", capturedBody["connectionUri"])
	}
	if capturedBody["username"] != "neo4j" {
		t.Fatalf("username mismatch in body: %v", capturedBody["username"])
	}
	if capturedBody["password"] != "supersecret" {
		t.Fatalf("password mismatch in body: %v", capturedBody["password"])
	}
	if _, hasDescription := capturedBody["description"]; hasDescription {
		t.Fatalf("description must NOT be in body when --description not supplied; got: %v", capturedBody)
	}
	if len(capturedBody) != 4 {
		t.Fatalf("expected exactly 4 keys when --description omitted; got %d: %v", len(capturedBody), capturedBody)
	}

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "f4e2f3c0-1111-2222-3333-444455556666" {
		t.Fatalf("expected id in output, got %v", got["id"])
	}
	if got["name"] != "aura-prod" {
		t.Fatalf("expected name in output, got %v", got["name"])
	}
}

// TestCreate_WithDescription verifies that --description is forwarded
// verbatim when set.
func TestCreate_WithDescription(t *testing.T) {
	h := newCreateHelper(t)
	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/fastify/api/connections" {
			capturedBody = readPostBody(t, r)
			_, _ = w.Write([]byte(`{"id":"id1","name":"n","description":"dev tier"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`connection create --name n --uri neo4j://x --username neo4j --password p --description "dev tier" --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if capturedBody["description"] != "dev tier" {
		t.Fatalf("expected description=dev tier in body, got %v", capturedBody["description"])
	}
}

// TestCreate_NonTTY_NoPassword_Errors covers the runtime password gate: when
// --password is omitted on a non-TTY, the leaf must surface a usage error
// BEFORE any HTTP call.
func TestCreate_NonTTY_NoPassword_Errors(t *testing.T) {
	h := newCreateHelper(t)
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not hit HTTP when password is missing on non-TTY; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`connection create --name n --uri neo4j://x --username neo4j`)
	if err == nil {
		t.Fatalf("expected usage error when --password missing on non-TTY")
	}
	if !strings.Contains(err.Error(), "--password") {
		t.Fatalf("expected error to mention --password, got: %v", err)
	}
}

// TestCreate_TTY_PromptsForPassword covers the interactive prompt path: on
// a TTY without --password the user is prompted; the prompted value is sent
// on the POST.
func TestCreate_TTY_PromptsForPassword(t *testing.T) {
	h := newCreateHelper(t)
	t.Cleanup(connection.SetPasswordReaderFnForTest(func() (string, error) { return "prompted-pw", nil }))

	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/fastify/api/connections" {
			capturedBody = readPostBody(t, r)
			_, _ = w.Write([]byte(`{"id":"id1","name":"n"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`connection create --name n --uri neo4j://x --username neo4j --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if capturedBody["password"] != "prompted-pw" {
		t.Fatalf("expected prompted password in body, got %v", capturedBody["password"])
	}
	if !strings.Contains(h.err.String(), "Password:") {
		t.Fatalf("expected 'Password:' prompt on stderr, got: %q", h.err.String())
	}
}

// TestCreate_DuplicateName_400Surfaces the relate-side 400 response shape (a
// duplicate name) must surface as a clear fatal error to the user.
func TestCreate_DuplicateName_400Surfaces(t *testing.T) {
	h := newCreateHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/fastify/api/connections" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"connection with name 'aura-prod' already exists"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`connection create --name aura-prod --uri neo4j+s://x --username neo4j --password p --format json`)
	if err == nil {
		t.Fatalf("expected error on duplicate name")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected status code in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected response body slice in error, got: %v", err)
	}
}

// TestCreate_Annotated_Write covers the cobra annotation: the create leaf
// must be tagged write=true so the root --rw enforcement fires.
func TestCreate_Annotated_Write(t *testing.T) {
	cfg := clicfg.NewConfig(mustTestFs(t), "test", clicfg.GlobalScope)
	parent := desktop.NewCmd(cfg)
	var leaf *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "connection" {
			for _, sub := range c.Commands() {
				if sub.Name() == "create" {
					leaf = sub
				}
			}
		}
	}
	if leaf == nil {
		t.Fatalf("connection create command not registered")
	}
	if leaf.Annotations["write"] != "true" {
		t.Fatalf("create must be annotated write=true; got %v", leaf.Annotations)
	}
}

// TestCreate_Example_FlushLeft is a focused per-leaf gate that mirrors the
// TestAllLeafCommands_HaveExamples whole-tree gate: each new leaf carries
// a flush-left Example with ≥3 invocations.
func TestCreate_Example_FlushLeft(t *testing.T) {
	cfg := clicfg.NewConfig(mustTestFs(t), "test", clicfg.GlobalScope)
	parent := desktop.NewCmd(cfg)
	var leaf *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "connection" {
			for _, sub := range c.Commands() {
				if sub.Name() == "create" {
					leaf = sub
				}
			}
		}
	}
	if leaf == nil {
		t.Fatalf("connection create command not registered")
	}
	if leaf.Example == "" {
		t.Fatalf("connection create Example must be non-empty")
	}
	firstLine := strings.SplitN(leaf.Example, "\n", 2)[0]
	if strings.HasPrefix(firstLine, "  ") {
		t.Fatalf("connection create Example first line must be flush-left; got %q", firstLine)
	}
	if c := strings.Count(leaf.Example, "neo4j-cli desktop connection create"); c < 3 {
		t.Fatalf("connection create Example must contain >=3 invocations; got %d", c)
	}
}

// TestCreate_PortFlagPropagates verifies the parent's persistent --port flag
// reaches the client constructor seam.
func TestCreate_PortFlagPropagates(t *testing.T) {
	h := newCreateHelper(t)
	var gotPort int
	t.Cleanup(connection.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		gotPort = port
		return nil, desktopclient.UnreachableError()
	}))

	_ = h.run(`connection create --name n --uri neo4j://x --username neo4j --password p --port 44231 --format json`)
	if gotPort != 44231 {
		t.Fatalf("expected --port=44231 to propagate; got %d", gotPort)
	}
}

// TestCreate_DesktopUnreachable_ReturnsCanonicalError verifies the REQ-F-008
// canonical error mapping when Desktop is not running.
func TestCreate_DesktopUnreachable_ReturnsCanonicalError(t *testing.T) {
	h := newCreateHelper(t)
	t.Cleanup(connection.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return nil, desktopclient.UnreachableError()
	}))

	err := h.run(`connection create --name n --uri neo4j://x --username neo4j --password p --format json`)
	if err == nil {
		t.Fatalf("expected error when desktop is unreachable")
	}
	if !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Fatalf("expected canonical REQ-F-008 message, got: %v", err)
	}
}

// mustTestFs returns a fresh in-memory FS pre-seeded by testfs.GetTestFs.
func mustTestFs(t *testing.T) afero.Fs {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	return fs
}
