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
	"sync/atomic"
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

const validDeleteID = "f4e2f3c0-1111-2222-3333-444455556666"

// deleteHelper mirrors updateHelper / createHelper but pins seams appropriate
// to the delete leaf: stdin defaults to TTY=true (each test opts INTO the
// non-TTY branch explicitly).
type deleteHelper struct {
	t   *testing.T
	in  *bytes.Buffer
	out *bytes.Buffer
	err *bytes.Buffer
	fs  afero.Fs
}

func newDeleteHelper(t *testing.T) *deleteHelper {
	t.Helper()
	cobra.EnableTraverseRunHooks = true
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	if err != nil {
		t.Fatalf("GetTestFs: %v", err)
	}
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return true }))
	return &deleteHelper{
		t:   t,
		in:  &bytes.Buffer{},
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
		fs:  fs,
	}
}

func (h *deleteHelper) withHandler(handler http.HandlerFunc) *httptest.Server {
	h.t.Helper()
	const (
		salt     = "salt-conn-delete"
		clientID = "cid-conn-delete"
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

func (h *deleteHelper) run(command string) error {
	h.t.Helper()
	args, err := shlex.Split(command)
	if err != nil {
		h.t.Fatalf("shlex: %v", err)
	}
	cfg := clicfg.NewConfig(h.fs, "test", clicfg.GlobalScope)
	cmd := desktop.NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetArgs(args)
	cmd.SetIn(h.in)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)
	return cmd.Execute()
}

// TestDelete_RejectsNonUUID covers the positional UUID gate. A non-UUID must
// fail with a usage error pointing at `desktop list` BEFORE any HTTP call.
func TestDelete_RejectsNonUUID(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(connection.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		t.Fatalf("must not construct client when id is not a UUID")
		return nil, nil
	}))

	err := h.run(`connection delete not-a-uuid --yes`)
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

// TestDelete_RequiresID guards against the positional being silently optional.
func TestDelete_RequiresID(t *testing.T) {
	h := newDeleteHelper(t)
	err := h.run(`connection delete`)
	if err == nil {
		t.Fatalf("expected error when <id> is missing")
	}
}

// TestDelete_NonTTY_WithoutYes_ExitsNonZero: non-TTY without --yes must hard-
// error so accidental piped invocations cannot silently delete a connection.
func TestDelete_NonTTY_WithoutYes_ExitsNonZero(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return false }))

	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalls.Add(1)
		}
		t.Errorf("non-TTY without --yes must not hit the API; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`connection delete ` + validDeleteID)
	if err == nil {
		t.Fatalf("expected usage error when non-TTY and --yes is missing")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected error to mention --yes, got: %v", err)
	}
	if deleteCalls.Load() != 0 {
		t.Fatalf("expected zero DELETE calls in non-TTY+no-yes path; got %d", deleteCalls.Load())
	}
}

// TestDelete_NonTTY_WithYes_DeletesWithoutPrompt: --yes proceeds straight to
// DELETE with no prompt and no preceding GET / list call.
func TestDelete_NonTTY_WithYes_DeletesWithoutPrompt(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return false }))

	var listCalls atomic.Int32
	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/connections":
			listCalls.Add(1)
			t.Errorf("--yes path must not issue a confirmation list; got GET %s", r.URL.Path)
		case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/connections/"+validDeleteID:
			deleteCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"` + validDeleteID + `","name":"aura-prod","connectionUri":"neo4j+s://abc"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run(`connection delete ` + validDeleteID + ` --yes --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if listCalls.Load() != 0 {
		t.Fatalf("--yes path must not issue a confirmation list; got %d", listCalls.Load())
	}
	if deleteCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 DELETE call; got %d", deleteCalls.Load())
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != validDeleteID {
		t.Fatalf("expected id in output, got %v", got["id"])
	}
	if got["name"] != "aura-prod" {
		t.Fatalf("expected name=aura-prod in output, got %v", got["name"])
	}
	if got["deleted"] != true {
		t.Fatalf("expected deleted=true in output, got %v", got["deleted"])
	}
	if len(got) != 3 {
		t.Fatalf("expected exactly 3 keys (id, name, deleted); got %d: %s", len(got), h.out.String())
	}
}

// TestDelete_NonTTY_WithYes_TableConfirmation: default (table) format emits a
// quoted-name one-line confirmation mirroring `desktop delete`.
func TestDelete_NonTTY_WithYes_TableConfirmation(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/connections/"+validDeleteID {
			_, _ = w.Write([]byte(`{"id":"` + validDeleteID + `","name":"aura-prod"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`connection delete ` + validDeleteID + ` --yes --format table`); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(h.out.String())
	want := `Deleted connection "aura-prod" (` + validDeleteID + `).`
	if got != want {
		t.Fatalf("table confirmation mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

// TestDelete_NonTTY_WithYes_ToonConfirmation: toon format gets the same one-
// line confirmation as table.
func TestDelete_NonTTY_WithYes_ToonConfirmation(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/connections/"+validDeleteID {
			_, _ = w.Write([]byte(`{"id":"` + validDeleteID + `","name":"aura-prod"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`connection delete ` + validDeleteID + ` --yes --format toon`); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(h.out.String())
	want := `Deleted connection "aura-prod" (` + validDeleteID + `).`
	if got != want {
		t.Fatalf("toon confirmation mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

// TestDelete_FallsBackToIDWhenNameMissing: when Desktop's DELETE response
// omits `name`, the confirmation line falls back to the id in both positions.
func TestDelete_FallsBackToIDWhenNameMissing(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/connections/"+validDeleteID {
			_, _ = w.Write([]byte(`{"id":"` + validDeleteID + `"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`connection delete ` + validDeleteID + ` --yes --format table`); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(h.out.String())
	want := `Deleted connection "` + validDeleteID + `" (` + validDeleteID + `).`
	if got != want {
		t.Fatalf("table confirmation mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

// TestDelete_TTY_WithYes_SkipsPrompt: TTY + --yes also skips the prompt.
func TestDelete_TTY_WithYes_SkipsPrompt(t *testing.T) {
	h := newDeleteHelper(t)

	var listCalls atomic.Int32
	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/connections":
			listCalls.Add(1)
			t.Errorf("--yes path must not issue a confirmation list; got GET %s", r.URL.Path)
		case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/connections/"+validDeleteID:
			deleteCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"` + validDeleteID + `","name":"aura-prod"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run(`connection delete ` + validDeleteID + ` --yes --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if listCalls.Load() != 0 {
		t.Fatalf("--yes must not issue a list call; got %d", listCalls.Load())
	}
	if deleteCalls.Load() != 1 {
		t.Fatalf("expected 1 DELETE; got %d", deleteCalls.Load())
	}
	if strings.Contains(h.out.String(), "Delete connection") {
		t.Fatalf("--yes path leaked the prompt text to stdout; got: %s", h.out.String())
	}
}

// TestDelete_TTY_PromptYes_Proceeds: TTY + no --yes + affirmative answer →
// list is consulted for the prompt label, then DELETE fires.
func TestDelete_TTY_PromptYes_Proceeds(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "yes\n", "YES\n", "Yes\n", "  y  \n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			h := newDeleteHelper(t)
			h.in.WriteString(answer)

			var deleteCalls atomic.Int32
			h.withHandler(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/connections":
					_, _ = w.Write([]byte(`[{"id":"` + validDeleteID + `","name":"aura-prod"}]`))
				case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/connections/"+validDeleteID:
					deleteCalls.Add(1)
					_, _ = w.Write([]byte(`{"id":"` + validDeleteID + `","name":"aura-prod"}`))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
			})

			if err := h.run(`connection delete ` + validDeleteID + ` --format json`); err != nil {
				t.Fatalf("run: %v", err)
			}
			if deleteCalls.Load() != 1 {
				t.Fatalf("expected 1 DELETE on confirm; got %d", deleteCalls.Load())
			}
			if !strings.Contains(h.out.String(), "Delete connection") {
				t.Fatalf("expected prompt text on stdout; got: %s", h.out.String())
			}
			if !strings.Contains(h.out.String(), `"aura-prod"`) || !strings.Contains(h.out.String(), "("+validDeleteID+")") {
				t.Fatalf("prompt must name the connection %q and id %q; got: %s", "aura-prod", validDeleteID, h.out.String())
			}
		})
	}
}

// TestDelete_TTY_PromptNo_Aborts: TTY + no --yes + non-affirmative answer →
// aborts with non-zero exit, no DELETE call.
func TestDelete_TTY_PromptNo_Aborts(t *testing.T) {
	for _, answer := range []string{"n\n", "N\n", "no\n", "\n", "anything else\n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			h := newDeleteHelper(t)
			h.in.WriteString(answer)

			var deleteCalls atomic.Int32
			h.withHandler(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/connections":
					_, _ = w.Write([]byte(`[{"id":"` + validDeleteID + `","name":"aura-prod"}]`))
				case r.Method == http.MethodDelete:
					deleteCalls.Add(1)
					t.Errorf("DELETE should not fire on non-affirmative answer; got %s", r.URL.Path)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
			})

			err := h.run(`connection delete ` + validDeleteID + ` --format json`)
			if err == nil {
				t.Fatalf("expected non-zero exit when user declines")
			}
			if deleteCalls.Load() != 0 {
				t.Fatalf("expected zero DELETE calls on decline; got %d", deleteCalls.Load())
			}
		})
	}
}

// TestDelete_TTY_EmptyStdin_Aborts: TTY + empty stdin (closed pipe) → aborts
// without firing DELETE.
func TestDelete_TTY_EmptyStdin_Aborts(t *testing.T) {
	h := newDeleteHelper(t)
	// Empty buffer simulates immediate EOF.

	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/connections":
			_, _ = w.Write([]byte(`[{"id":"` + validDeleteID + `","name":"aura-prod"}]`))
		case r.Method == http.MethodDelete:
			deleteCalls.Add(1)
			t.Errorf("DELETE should not fire when stdin EOFs before any input")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run(`connection delete ` + validDeleteID + ` --format json`)
	if err == nil {
		t.Fatalf("expected non-zero exit when stdin EOFs without input")
	}
	if deleteCalls.Load() != 0 {
		t.Fatalf("expected zero DELETE on EOF; got %d", deleteCalls.Load())
	}
}

// TestDelete_TTY_PromptNameless_WhenConnectionMissingFromList: if the id is
// missing from the list (stale state, race) the prompt is rendered without
// the name. The user still has to confirm.
func TestDelete_TTY_PromptNameless_WhenConnectionMissingFromList(t *testing.T) {
	h := newDeleteHelper(t)
	h.in.WriteString("y\n")

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/connections":
			// Empty list — the connection is not visible.
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/connections/"+validDeleteID:
			_, _ = w.Write([]byte(`{"id":"` + validDeleteID + `"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run(`connection delete ` + validDeleteID + ` --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	// Prompt must include the id but not a quoted name (which would be
	// misleading when we couldn't resolve the name).
	if !strings.Contains(h.out.String(), "Delete connection") || !strings.Contains(h.out.String(), "("+validDeleteID+")") {
		t.Fatalf("expected nameless prompt with id; got: %s", h.out.String())
	}
	if strings.Contains(h.out.String(), `Delete connection "`) {
		t.Fatalf("expected nameless prompt (no quoted name) when list misses the id; got: %s", h.out.String())
	}
}

// TestDelete_TTY_PromptNameless_WhenListErrors: if ListConnections errors we
// still let the user delete by id alone — a flaky list endpoint shouldn't
// block a delete the user clearly intends. The prompt falls back to nameless.
func TestDelete_TTY_PromptNameless_WhenListErrors(t *testing.T) {
	h := newDeleteHelper(t)
	h.in.WriteString("y\n")

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/connections":
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/connections/"+validDeleteID:
			_, _ = w.Write([]byte(`{"id":"` + validDeleteID + `","name":"aura-prod"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run(`connection delete ` + validDeleteID + ` --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(h.out.String(), `Delete connection "`) {
		t.Fatalf("expected nameless prompt (no quoted name) when list errors; got: %s", h.out.String())
	}
	if !strings.Contains(h.out.String(), "("+validDeleteID+")") {
		t.Fatalf("expected id in prompt; got: %s", h.out.String())
	}
}

// TestDelete_UnknownUUID_Surfaces404: a well-formed UUID that doesn't match
// any connection must surface a clear error via the shared transport.
func TestDelete_UnknownUUID_Surfaces404(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"connection not found"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`connection delete ` + validDeleteID + ` --yes --format json`)
	if err == nil {
		t.Fatalf("expected error on unknown UUID")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404 in error, got: %v", err)
	}
}

// TestDelete_Annotated_Write: the delete leaf must be tagged write=true so
// the root --rw enforcement fires.
func TestDelete_Annotated_Write(t *testing.T) {
	cfg := clicfg.NewConfig(mustTestFs(t), "test", clicfg.GlobalScope)
	parent := desktop.NewCmd(cfg)
	var leaf *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "connection" {
			for _, sub := range c.Commands() {
				if sub.Name() == "delete" {
					leaf = sub
				}
			}
		}
	}
	if leaf == nil {
		t.Fatalf("connection delete command not registered")
	}
	if leaf.Annotations["write"] != "true" {
		t.Fatalf("delete must be annotated write=true; got %v", leaf.Annotations)
	}
}

// TestDelete_Example_FlushLeft mirrors the create / update equivalents: every
// new leaf carries a flush-left Example with ≥3 invocations.
func TestDelete_Example_FlushLeft(t *testing.T) {
	cfg := clicfg.NewConfig(mustTestFs(t), "test", clicfg.GlobalScope)
	parent := desktop.NewCmd(cfg)
	var leaf *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "connection" {
			for _, sub := range c.Commands() {
				if sub.Name() == "delete" {
					leaf = sub
				}
			}
		}
	}
	if leaf == nil {
		t.Fatalf("connection delete command not registered")
	}
	if leaf.Example == "" {
		t.Fatalf("connection delete Example must be non-empty")
	}
	firstLine := strings.SplitN(leaf.Example, "\n", 2)[0]
	if strings.HasPrefix(firstLine, "  ") {
		t.Fatalf("connection delete Example first line must be flush-left; got %q", firstLine)
	}
	if c := strings.Count(leaf.Example, "neo4j-cli desktop connection delete"); c < 3 {
		t.Fatalf("connection delete Example must contain >=3 invocations; got %d", c)
	}
}

// TestDelete_PortFlagPropagates verifies the parent's persistent --port flag
// reaches the client constructor seam.
func TestDelete_PortFlagPropagates(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return false }))

	var gotPort int
	t.Cleanup(connection.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		gotPort = port
		return nil, desktopclient.UnreachableError()
	}))

	_ = h.run(`connection delete ` + validDeleteID + ` --yes --port 44231 --format json`)
	if gotPort != 44231 {
		t.Fatalf("expected --port=44231 to propagate; got %d", gotPort)
	}
}

// TestDelete_DesktopUnreachable_ReturnsCanonicalError verifies the REQ-F-008
// canonical error mapping when Desktop is not running.
func TestDelete_DesktopUnreachable_ReturnsCanonicalError(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(connection.SetStdinIsTTYFnForTest(func() bool { return false }))
	t.Cleanup(connection.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return nil, desktopclient.UnreachableError()
	}))

	err := h.run(`connection delete ` + validDeleteID + ` --yes --format json`)
	if err == nil {
		t.Fatalf("expected error when desktop is unreachable")
	}
	if !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Fatalf("expected canonical REQ-F-008 message, got: %v", err)
	}
}
