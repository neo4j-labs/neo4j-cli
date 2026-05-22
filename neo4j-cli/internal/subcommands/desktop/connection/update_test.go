// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package connection_test

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
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/desktop/connection"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

const validUpdateID = "f4e2f3c0-1111-2222-3333-444455556666"

// updateHelper mirrors createHelper from create_test.go but pins seams
// appropriate to the update leaf: stdin defaults to TTY=true and the
// password reader fails the test unless an explicit override is installed.
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
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return true }))
	t.Cleanup(connection.SetPasswordReaderFnForTest(func() (string, error) {
		t.Fatalf("passwordReaderFn must not be called unless --password is omitted on a TTY")
		return "", nil
	}))
	return &updateHelper{t: t, out: &bytes.Buffer{}, err: &bytes.Buffer{}, fs: fs}
}

func (h *updateHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-conn-update"
		clientID = "cid-conn-update"
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

func (h *updateHelper) run(command string) error {
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

// TestUpdate_RejectsNonUUID covers the positional UUID gate. A non-UUID
// must fail with a usage error pointing at `desktop list` BEFORE any HTTP
// call.
func TestUpdate_RejectsNonUUID(t *testing.T) {
	h := newUpdateHelper(t)
	// Pin the client constructor to a noisy stub so a stray HTTP call would
	// fail the test instead of silently succeeding against a real probe.
	t.Cleanup(connection.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		t.Fatalf("must not construct client when id is not a UUID")
		return nil, nil
	}))

	err := h.run(`connection update not-a-uuid --name new`)
	if err == nil {
		t.Fatalf("expected usage error for non-UUID id")
	}
	if !strings.Contains(err.Error(), "UUID") {
		t.Fatalf("expected error to mention UUID, got: %v", err)
	}
	if !strings.Contains(err.Error(), "desktop list") {
		t.Fatalf("expected error to point at 'desktop list', got: %v", err)
	}
}

// TestUpdate_NoMutatingFlags_Errors covers the REQ-F-106 "at least one
// flag" gate. Zero mutating flags must surface a usage error BEFORE any
// HTTP call.
func TestUpdate_NoMutatingFlags_Errors(t *testing.T) {
	h := newUpdateHelper(t)
	t.Cleanup(connection.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		t.Fatalf("must not construct client when no mutating flags supplied")
		return nil, nil
	}))

	err := h.run(`connection update ` + validUpdateID)
	if err == nil {
		t.Fatalf("expected usage error when no mutating flags supplied")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("expected error to mention 'at least one', got: %v", err)
	}
}

// TestUpdate_PartialPatch_SingleField covers the wire-shape contract: the
// PATCH body must contain ONLY the keys the user actually set. Snapshot
// the received body and confirm exactly one key.
func TestUpdate_PartialPatch_SingleField(t *testing.T) {
	h := newUpdateHelper(t)
	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/fastify/api/connections/"+validUpdateID {
			capturedBody = readPostBody(t, r)
			_, _ = w.Write([]byte(`{"id":"` + validUpdateID + `","name":"renamed"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`connection update ` + validUpdateID + ` --name renamed --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(capturedBody) != 1 {
		t.Fatalf("expected PATCH body to contain ONLY 'name'; got %d keys: %v", len(capturedBody), capturedBody)
	}
	if capturedBody["name"] != "renamed" {
		t.Fatalf("expected name='renamed' in body, got %v", capturedBody["name"])
	}
}

// TestUpdate_PartialPatch_MultipleFields covers a multi-field PATCH: only
// the supplied keys appear in the body, with the supplied values.
func TestUpdate_PartialPatch_MultipleFields(t *testing.T) {
	h := newUpdateHelper(t)
	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/fastify/api/connections/"+validUpdateID {
			capturedBody = readPostBody(t, r)
			_, _ = w.Write([]byte(`{"id":"` + validUpdateID + `","name":"renamed"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`connection update ` + validUpdateID + ` --uri neo4j+s://new --password new-secret --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(capturedBody) != 2 {
		t.Fatalf("expected PATCH body to contain exactly 2 keys; got %d: %v", len(capturedBody), capturedBody)
	}
	if capturedBody["connectionUri"] != "neo4j+s://new" {
		t.Fatalf("expected connectionUri in body, got %v", capturedBody["connectionUri"])
	}
	if capturedBody["password"] != "new-secret" {
		t.Fatalf("expected password in body, got %v", capturedBody["password"])
	}
}

// TestUpdate_EmptyDescription_PreservesKey covers the empty-string-is-an-
// update contract: `--description ""` must reach the wire with an empty
// string value, NOT be dropped as "not set".
func TestUpdate_EmptyDescription_PreservesKey(t *testing.T) {
	h := newUpdateHelper(t)
	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			capturedBody = readPostBody(t, r)
			_, _ = w.Write([]byte(`{"id":"` + validUpdateID + `","name":"n"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`connection update ` + validUpdateID + ` --description "" --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, has := capturedBody["description"]; !has {
		t.Fatalf("expected description key to be present when --description \"\" supplied; got %v", capturedBody)
	}
	if capturedBody["description"] != "" {
		t.Fatalf("expected description=\"\" in body, got %v", capturedBody["description"])
	}
}

// TestUpdate_UnknownUUID_Surfaces404 covers the relate 404 path: a
// well-formed UUID that doesn't match any connection must surface a clear
// error via the shared transport.
func TestUpdate_UnknownUUID_Surfaces404(t *testing.T) {
	h := newUpdateHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"connection not found"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`connection update ` + validUpdateID + ` --name n --format json`)
	if err == nil {
		t.Fatalf("expected error on unknown UUID")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 in error, got: %v", err)
	}
}

// TestUpdate_EmptyPassword_PromptsOnTTY covers the password-rotation
// interactive flow: `--password ""` on a TTY triggers the no-echo prompt
// and the prompted value reaches the wire.
func TestUpdate_EmptyPassword_PromptsOnTTY(t *testing.T) {
	h := newUpdateHelper(t)
	t.Cleanup(connection.SetPasswordReaderFnForTest(func() (string, error) { return "prompted-new-pw", nil }))

	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			capturedBody = readPostBody(t, r)
			_, _ = w.Write([]byte(`{"id":"` + validUpdateID + `","name":"n"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`connection update ` + validUpdateID + ` --password "" --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if capturedBody["password"] != "prompted-new-pw" {
		t.Fatalf("expected prompted password in body, got %v", capturedBody["password"])
	}
	if !strings.Contains(h.err.String(), "Password:") {
		t.Fatalf("expected 'Password:' prompt on stderr, got: %q", h.err.String())
	}
}

// TestUpdate_EmptyPassword_NonTTYErrors covers the non-TTY guard: an empty
// --password on a scripted shell must error BEFORE any HTTP call.
func TestUpdate_EmptyPassword_NonTTYErrors(t *testing.T) {
	h := newUpdateHelper(t)
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return false }))
	t.Cleanup(connection.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		t.Fatalf("must not construct client when --password is empty on non-TTY")
		return nil, nil
	}))

	err := h.run(`connection update ` + validUpdateID + ` --password ""`)
	if err == nil {
		t.Fatalf("expected usage error when --password \"\" on non-TTY")
	}
	if !strings.Contains(err.Error(), "--password") {
		t.Fatalf("expected error to mention --password, got: %v", err)
	}
}

// TestUpdate_SuccessRendersConnection covers the happy-path render: the
// returned Connection is emitted in the requested format (JSON here).
func TestUpdate_SuccessRendersConnection(t *testing.T) {
	h := newUpdateHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			_, _ = w.Write([]byte(`{"id":"` + validUpdateID + `","name":"renamed","connectionUri":"neo4j+s://abc","project":"proj-1"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`connection update ` + validUpdateID + ` --name renamed --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != validUpdateID {
		t.Fatalf("expected id in output, got %v", got["id"])
	}
	if got["name"] != "renamed" {
		t.Fatalf("expected name=renamed in output, got %v", got["name"])
	}
}

// TestUpdate_Annotated_Write covers the cobra annotation: the update leaf
// must be tagged write=true so the root --rw enforcement fires.
func TestUpdate_Annotated_Write(t *testing.T) {
	cfg := clicfg.NewConfig(mustTestFs(t), "test", clicfg.GlobalScope)
	parent := desktop.NewCmd(cfg)
	var leaf *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "connection" {
			for _, sub := range c.Commands() {
				if sub.Name() == "update" {
					leaf = sub
				}
			}
		}
	}
	if leaf == nil {
		t.Fatalf("connection update command not registered")
	}
	if leaf.Annotations["write"] != "true" {
		t.Fatalf("update must be annotated write=true; got %v", leaf.Annotations)
	}
}

// TestUpdate_Example_FlushLeft mirrors the create equivalent: every new
// leaf carries a flush-left Example with ≥3 invocations.
func TestUpdate_Example_FlushLeft(t *testing.T) {
	cfg := clicfg.NewConfig(mustTestFs(t), "test", clicfg.GlobalScope)
	parent := desktop.NewCmd(cfg)
	var leaf *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "connection" {
			for _, sub := range c.Commands() {
				if sub.Name() == "update" {
					leaf = sub
				}
			}
		}
	}
	if leaf == nil {
		t.Fatalf("connection update command not registered")
	}
	if leaf.Example == "" {
		t.Fatalf("connection update Example must be non-empty")
	}
	firstLine := strings.SplitN(leaf.Example, "\n", 2)[0]
	if strings.HasPrefix(firstLine, "  ") {
		t.Fatalf("connection update Example first line must be flush-left; got %q", firstLine)
	}
	if c := strings.Count(leaf.Example, "neo4j-cli desktop connection update"); c < 3 {
		t.Fatalf("connection update Example must contain >=3 invocations; got %d", c)
	}
}
