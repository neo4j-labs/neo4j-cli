// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms_test

import (
	"bytes"
	"context"
	"encoding/json"
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

// deleteHelper wires dbms.NewCmd against an in-memory FS, mirroring the
// startHelper / stopHelper shape. Each leaf's test surface is colocated so
// behaviour stays auditable without grepping through a shared file.
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
	// Default to TTY=true so each test opts INTO the non-TTY branch
	// explicitly. The opposite default would cause every test that forgets
	// to set the seam to silently take the non-TTY refusal path.
	t.Cleanup(dbms.SetDeleteStdinIsTTYFnForTest(func() bool { return true }))
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
		salt     = "salt-delete"
		clientID = "cid-delete"
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

func (h *deleteHelper) run(command string) error {
	h.t.Helper()
	args, err := shlex.Split(command)
	if err != nil {
		h.t.Fatalf("shlex: %v", err)
	}
	cfg := clicfg.NewConfig(h.fs, "test", clicfg.GlobalScope)
	cmd := dbms.NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	cmd.SetArgs(args)
	cmd.SetIn(h.in)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)
	return cmd.Execute()
}

func TestDelete_RequiresID(t *testing.T) {
	h := newDeleteHelper(t)
	err := h.run("delete")
	if err == nil {
		t.Fatalf("expected error when <id> is missing")
	}
}

// Non-TTY without --yes must hard-error so accidental piped invocations cannot
// silently delete a DBMS (REQ-F-003).
func TestDelete_NonTTY_WithoutYes_ExitsNonZero(t *testing.T) {
	h := newDeleteHelper(t)
	// Override default TTY=true seed for this test.
	t.Cleanup(dbms.SetDeleteStdinIsTTYFnForTest(func() bool { return false }))

	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalls.Add(1)
		}
		t.Errorf("non-TTY without --yes must not hit the API; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run("delete abc")
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

// Non-TTY + --yes proceeds straight to DELETE with no prompt.
func TestDelete_NonTTY_WithYes_DeletesWithoutPrompt(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(dbms.SetDeleteStdinIsTTYFnForTest(func() bool { return false }))

	var getCalls atomic.Int32
	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			getCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc":
			deleteCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("delete abc --yes --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	// --yes must skip the confirmation-only GetDbms call too — only the
	// DELETE should fire.
	if getCalls.Load() != 0 {
		t.Fatalf("--yes path must not issue a confirmation GET; got %d GETs", getCalls.Load())
	}
	if deleteCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 DELETE call; got %d", deleteCalls.Load())
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "abc" {
		t.Fatalf("expected id=abc in output, got %v", got["id"])
	}
	if got["name"] != "my-dbms" {
		t.Fatalf("expected name=my-dbms in output, got %v", got["name"])
	}
	if got["deleted"] != true {
		t.Fatalf("expected deleted=true in output, got %v", got["deleted"])
	}
	// The misleading pre-delete status field must NOT appear in the JSON
	// confirmation payload — that was the whole bug this task fixes.
	if _, ok := got["status"]; ok {
		t.Fatalf("delete JSON output must not include `status`; got: %s", h.out.String())
	}
	// Exactly these three keys, no others.
	if len(got) != 3 {
		t.Fatalf("expected exactly 3 keys (id, name, deleted); got %d: %s", len(got), h.out.String())
	}
}

// Default (table) format emits the one-line confirmation with quoted name + id.
func TestDelete_NonTTY_WithYes_TableConfirmation(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(dbms.SetDeleteStdinIsTTYFnForTest(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc" {
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("delete abc --yes --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(h.out.String())
	want := `Deleted DBMS "my-dbms" (abc).`
	if got != want {
		t.Fatalf("table confirmation mismatch:\n  got:  %q\n  want: %q", got, want)
	}
	// The misleading `STATUS` column must not appear on the delete path.
	if strings.Contains(h.out.String(), "STATUS") || strings.Contains(h.out.String(), "stopped") {
		t.Fatalf("delete output must not render a STATUS column; got: %s", h.out.String())
	}
}

// Toon format gets the same one-line confirmation (delete is a single fact,
// not a resource worth rendering as a structured document).
func TestDelete_NonTTY_WithYes_ToonConfirmation(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(dbms.SetDeleteStdinIsTTYFnForTest(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc" {
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("delete abc --yes --format toon"); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(h.out.String())
	want := `Deleted DBMS "my-dbms" (abc).`
	if got != want {
		t.Fatalf("toon confirmation mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

// When Desktop's DELETE response omits `name`, the confirmation line falls
// back to the id in both positions and the JSON shape carries an empty name.
func TestDelete_NonTTY_WithYes_FallsBackToIDWhenNameMissing(t *testing.T) {
	t.Run("table", func(t *testing.T) {
		h := newDeleteHelper(t)
		t.Cleanup(dbms.SetDeleteStdinIsTTYFnForTest(func() bool { return false }))

		h.withHandler(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc" {
				_, _ = w.Write([]byte(`{"id":"abc","status":"stopped"}`))
				return
			}
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		})

		if err := h.run("delete abc --yes --format table"); err != nil {
			t.Fatalf("run: %v", err)
		}
		got := strings.TrimSpace(h.out.String())
		want := `Deleted DBMS "abc" (abc).`
		if got != want {
			t.Fatalf("table confirmation mismatch:\n  got:  %q\n  want: %q", got, want)
		}
	})

	t.Run("json", func(t *testing.T) {
		h := newDeleteHelper(t)
		t.Cleanup(dbms.SetDeleteStdinIsTTYFnForTest(func() bool { return false }))

		h.withHandler(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc" {
				_, _ = w.Write([]byte(`{"id":"abc","status":"stopped"}`))
				return
			}
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		})

		if err := h.run("delete abc --yes --format json"); err != nil {
			t.Fatalf("run: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
			t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
		}
		if got["id"] != "abc" {
			t.Fatalf("expected id=abc, got %v", got["id"])
		}
		if got["name"] != "" {
			t.Fatalf("expected empty name on fallback path, got %v", got["name"])
		}
		if got["deleted"] != true {
			t.Fatalf("expected deleted=true, got %v", got["deleted"])
		}
	})
}

// TTY + --yes also skips the prompt.
func TestDelete_TTY_WithYes_SkipsPrompt(t *testing.T) {
	h := newDeleteHelper(t)
	// Stdin must remain empty — if the leaf tries to read, the empty buffer
	// returns EOF and the test fails noisily on the "no confirmation"
	// non-zero exit.
	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc" {
			deleteCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"abc","status":"deleted"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("delete abc --yes --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if deleteCalls.Load() != 1 {
		t.Fatalf("expected 1 DELETE; got %d", deleteCalls.Load())
	}
	// Prompt text must not leak to stdout when --yes is used.
	if strings.Contains(h.out.String(), "Delete DBMS") {
		t.Fatalf("--yes path leaked the prompt text to stdout; got: %s", h.out.String())
	}
}

// TTY + no --yes + "y" → confirms and proceeds.
func TestDelete_TTY_PromptYes_Proceeds(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "yes\n", "YES\n", "Yes\n", "  y  \n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			h := newDeleteHelper(t)
			h.in.WriteString(answer)

			var deleteCalls atomic.Int32
			h.withHandler(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
					_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
				case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc":
					deleteCalls.Add(1)
					_, _ = w.Write([]byte(`{"id":"abc","status":"deleted"}`))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
			})

			if err := h.run("delete abc --format json"); err != nil {
				t.Fatalf("run: %v", err)
			}
			if deleteCalls.Load() != 1 {
				t.Fatalf("expected 1 DELETE on confirm; got %d", deleteCalls.Load())
			}
			if !strings.Contains(h.out.String(), "Delete DBMS") {
				t.Fatalf("expected prompt text on stdout; got: %s", h.out.String())
			}
			if !strings.Contains(h.out.String(), `"my-dbms"`) || !strings.Contains(h.out.String(), "(abc)") {
				t.Fatalf("prompt must name the dbms %q and id %q; got: %s", "my-dbms", "abc", h.out.String())
			}
		})
	}
}

// TTY + no --yes + non-affirmative answer → aborts with non-zero exit, no
// DELETE call.
func TestDelete_TTY_PromptNo_Aborts(t *testing.T) {
	for _, answer := range []string{"n\n", "N\n", "no\n", "\n", "anything else\n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			h := newDeleteHelper(t)
			h.in.WriteString(answer)

			var deleteCalls atomic.Int32
			h.withHandler(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
					_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
				case r.Method == http.MethodDelete:
					deleteCalls.Add(1)
					t.Errorf("DELETE should not fire on non-affirmative answer; got %s", r.URL.Path)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
			})

			err := h.run("delete abc --format json")
			if err == nil {
				t.Fatalf("expected non-zero exit when user declines")
			}
			if deleteCalls.Load() != 0 {
				t.Fatalf("expected zero DELETE calls on decline; got %d", deleteCalls.Load())
			}
		})
	}
}

// TTY + empty stdin (closed pipe) → aborts. Belt-and-braces against a TTY
// detector that wrongly returns true.
func TestDelete_TTY_EmptyStdin_Aborts(t *testing.T) {
	h := newDeleteHelper(t)
	// Empty buffer simulates immediate EOF.

	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"started"}`))
		case r.Method == http.MethodDelete:
			deleteCalls.Add(1)
			t.Errorf("DELETE should not fire when stdin EOFs before any input")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("delete abc --format json")
	if err == nil {
		t.Fatalf("expected non-zero exit when stdin EOFs without input")
	}
	if deleteCalls.Load() != 0 {
		t.Fatalf("expected zero DELETE on EOF; got %d", deleteCalls.Load())
	}
}

// Falls back to the id in the prompt when Desktop returns a DBMS with no
// name (degenerate but observable in the wild).
func TestDelete_TTY_FallsBackToIDWhenNameMissing(t *testing.T) {
	h := newDeleteHelper(t)
	h.in.WriteString("y\n")

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","status":"started"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc":
			_, _ = w.Write([]byte(`{"id":"abc","status":"deleted"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("delete abc --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(h.out.String(), `"abc"`) || !strings.Contains(h.out.String(), "(abc)") {
		t.Fatalf("expected prompt to fall back to id %q; got: %s", "abc", h.out.String())
	}
}

func TestDelete_NoCredentialsJSONWrite(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(dbms.SetDeleteStdinIsTTYFnForTest(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc" {
			_, _ = w.Write([]byte(`{"id":"abc","status":"deleted"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	credsPath := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json")
	before, err := afero.ReadFile(h.fs, credsPath)
	if err != nil {
		t.Fatalf("read credentials before: %v", err)
	}

	if err := h.run("delete abc --yes --format json"); err != nil {
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

// Non-TTY + --yes after a DELETE that 404s must surface the error (no
// silent absorption of a missed deletion).
func TestDelete_NoCredentialsJSONWrite_OnFailure(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(dbms.SetDeleteStdinIsTTYFnForTest(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	credsPath := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", "credentials.json")
	before, err := afero.ReadFile(h.fs, credsPath)
	if err != nil {
		t.Fatalf("read credentials before: %v", err)
	}

	if err := h.run("delete abc --yes --format json"); err == nil {
		t.Fatalf("expected non-zero exit on 404 from Desktop")
	}

	after, err := afero.ReadFile(h.fs, credsPath)
	if err != nil {
		t.Fatalf("read credentials after: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("credentials.json was mutated on failure path; before=%q after=%q", string(before), string(after))
	}
}

func TestDelete_Annotated_Write(t *testing.T) {
	cfg := clicfg.NewConfig(mustTestFs(t), "test", clicfg.GlobalScope)
	parent := dbms.NewCmd(cfg)
	var deleteCmd *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == "delete" {
			deleteCmd = c
			break
		}
	}
	if deleteCmd == nil {
		t.Fatalf("delete command not registered under desktop")
	}
	if deleteCmd.Annotations["write"] != "true" {
		t.Fatalf("delete must be annotated write=true; got %v", deleteCmd.Annotations)
	}
}

func TestDelete_PortFlagPropagatesToClientConstructor(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(dbms.SetDeleteStdinIsTTYFnForTest(func() bool { return false }))

	var gotPort int
	t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		gotPort = port
		return nil, desktopclient.UnreachableError()
	}))

	_ = h.run("delete abc --yes --port 44230 --format json")
	if gotPort != 44230 {
		t.Fatalf("expected --port=44230 to propagate to client constructor, got %d", gotPort)
	}
}

func TestDelete_DesktopUnreachable_ReturnsCanonicalError(t *testing.T) {
	h := newDeleteHelper(t)
	t.Cleanup(dbms.SetDeleteStdinIsTTYFnForTest(func() bool { return false }))
	t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return nil, desktopclient.UnreachableError()
	}))

	err := h.run("delete abc --yes --format json")
	if err == nil {
		t.Fatalf("expected error when desktop is unreachable")
	}
	if !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Fatalf("expected canonical REQ-F-008 message, got: %v", err)
	}
}
