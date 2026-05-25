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
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
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
	confirm.SetStdinIsTerminalForTest(t, func() bool { return true })
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

// Non-TTY without --yes/--force must hard-error so accidental piped
// invocations cannot silently delete a DBMS.
func TestDelete_NonTTY_WithoutFlags_Exit2(t *testing.T) {
	h := newDeleteHelper(t)
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalls.Add(1)
		}
		t.Errorf("non-TTY without flags must not hit the API; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run("delete abc")
	if err == nil {
		t.Fatalf("expected usage error when non-TTY and flags are missing")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *clierr.CLIError, got %v", err)
	}
	if ce.Code != 2 {
		t.Fatalf("expected exit 2, got %d", ce.Code)
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("expected error to mention 'pass both --yes and --force', got: %v", err)
	}
	if deleteCalls.Load() != 0 {
		t.Fatalf("expected zero DELETE calls in non-TTY+no-flags path; got %d", deleteCalls.Load())
	}
}

// Deliberate back-compat break: `desktop dbms delete --yes` (non-TTY) used to
// proceed; the shared gate now requires BOTH --yes and --force.
func TestDelete_NonTTY_OnlyYes_Exit2(t *testing.T) {
	h := newDeleteHelper(t)
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalls.Add(1)
		}
		t.Errorf("non-TTY without --force must not hit the API; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run("delete abc --yes")
	if err == nil {
		t.Fatalf("expected usage error when --force is missing")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("expected error to mention 'pass both --yes and --force', got: %v", err)
	}
	if deleteCalls.Load() != 0 {
		t.Fatalf("expected zero DELETE calls; got %d", deleteCalls.Load())
	}
}

func TestDelete_NonTTY_OnlyForce_Exit2(t *testing.T) {
	h := newDeleteHelper(t)
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("non-TTY without --yes must not hit the API; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run("delete abc --force")
	if err == nil {
		t.Fatalf("expected usage error when --yes is missing")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("expected error to mention 'pass both --yes and --force', got: %v", err)
	}
}

// Non-TTY + --yes --force proceeds straight to DELETE with no prompt.
func TestDelete_NonTTY_WithBothFlags_DeletesWithoutPrompt(t *testing.T) {
	h := newDeleteHelper(t)
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc":
			deleteCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("delete abc --yes --force --format json"); err != nil {
		t.Fatalf("run: %v", err)
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
	// confirmation payload.
	if _, ok := got["status"]; ok {
		t.Fatalf("delete JSON output must not include `status`; got: %s", h.out.String())
	}
	if len(got) != 3 {
		t.Fatalf("expected exactly 3 keys (id, name, deleted); got %d: %s", len(got), h.out.String())
	}
}

// Default (table) format emits the one-line confirmation with quoted name + id.
func TestDelete_NonTTY_WithBothFlags_TableConfirmation(t *testing.T) {
	h := newDeleteHelper(t)
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc" {
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("delete abc --yes --force --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(h.out.String())
	want := `Deleted DBMS "my-dbms" (abc).`
	if got != want {
		t.Fatalf("table confirmation mismatch:\n  got:  %q\n  want: %q", got, want)
	}
	if strings.Contains(h.out.String(), "STATUS") || strings.Contains(h.out.String(), "stopped") {
		t.Fatalf("delete output must not render a STATUS column; got: %s", h.out.String())
	}
}

// Toon format gets the same one-line confirmation.
func TestDelete_NonTTY_WithBothFlags_ToonConfirmation(t *testing.T) {
	h := newDeleteHelper(t)
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc" {
			_, _ = w.Write([]byte(`{"id":"abc","name":"my-dbms","status":"stopped"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("delete abc --yes --force --format toon"); err != nil {
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
func TestDelete_NonTTY_WithBothFlags_FallsBackToIDWhenNameMissing(t *testing.T) {
	t.Run("table", func(t *testing.T) {
		h := newDeleteHelper(t)
		confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

		h.withHandler(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc" {
				_, _ = w.Write([]byte(`{"id":"abc","status":"stopped"}`))
				return
			}
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		})

		if err := h.run("delete abc --yes --force --format table"); err != nil {
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
		confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

		h.withHandler(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc" {
				_, _ = w.Write([]byte(`{"id":"abc","status":"stopped"}`))
				return
			}
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		})

		if err := h.run("delete abc --yes --force --format json"); err != nil {
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

// TTY + --yes --force also skips the prompt.
func TestDelete_TTY_WithBothFlags_SkipsPrompt(t *testing.T) {
	h := newDeleteHelper(t)
	// Stdin must remain empty — if the leaf tries to read, the empty buffer
	// returns EOF and the test fails on the cancelled path.
	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/dbmss/abc" {
			deleteCalls.Add(1)
			_, _ = w.Write([]byte(`{"id":"abc","status":"deleted"}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("delete abc --yes --force --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if deleteCalls.Load() != 1 {
		t.Fatalf("expected 1 DELETE; got %d", deleteCalls.Load())
	}
	// Prompt text must not leak to either stream when --yes --force is used.
	if strings.Contains(h.err.String(), "Delete dbms") {
		t.Fatalf("--yes --force path leaked the prompt text to stderr; got: %s", h.err.String())
	}
}

// TTY + no flags + "y" → confirms and proceeds; prompt goes to stderr.
func TestDelete_TTY_PromptYes_Proceeds(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "yes\n", "YES\n", "Yes\n", "  y  \n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			h := newDeleteHelper(t)
			h.in.WriteString(answer)

			var deleteCalls atomic.Int32
			h.withHandler(func(w http.ResponseWriter, r *http.Request) {
				switch {
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
			if !strings.Contains(h.err.String(), `Delete dbms "abc"?`) {
				t.Fatalf("expected prompt text on stderr; got: %s", h.err.String())
			}
		})
	}
}

// TTY + no flags + non-affirmative answer → cancelled (exit 0), no DELETE.
func TestDelete_TTY_PromptNo_Cancels(t *testing.T) {
	for _, answer := range []string{"n\n", "N\n", "no\n", "\n", "anything else\n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			h := newDeleteHelper(t)
			h.in.WriteString(answer)

			var deleteCalls atomic.Int32
			h.withHandler(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deleteCalls.Add(1)
					t.Errorf("DELETE should not fire on non-affirmative answer; got %s", r.URL.Path)
				} else {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
			})

			err := h.run("delete abc --format json")
			if err != nil {
				t.Fatalf("expected nil err on cancel, got: %v", err)
			}
			if deleteCalls.Load() != 0 {
				t.Fatalf("expected zero DELETE calls on cancel; got %d", deleteCalls.Load())
			}
			if !strings.Contains(h.err.String(), "cancelled.") {
				t.Fatalf("expected 'cancelled.' on stderr; got: %s", h.err.String())
			}
		})
	}
}

// TTY + empty stdin (closed pipe) → cancelled (exit 0).
func TestDelete_TTY_EmptyStdin_Cancels(t *testing.T) {
	h := newDeleteHelper(t)
	// Empty buffer simulates immediate EOF.

	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalls.Add(1)
			t.Errorf("DELETE should not fire when stdin EOFs before any input")
		} else {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("delete abc --format json")
	if err != nil {
		t.Fatalf("expected nil err on EOF cancel, got: %v", err)
	}
	if deleteCalls.Load() != 0 {
		t.Fatalf("expected zero DELETE on EOF; got %d", deleteCalls.Load())
	}
}

func TestDelete_NoCredentialsJSONWrite(t *testing.T) {
	h := newDeleteHelper(t)
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

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

	if err := h.run("delete abc --yes --force --format json"); err != nil {
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

// Non-TTY + flags after a DELETE that 404s must surface the error (no
// silent absorption of a missed deletion).
func TestDelete_NoCredentialsJSONWrite_OnFailure(t *testing.T) {
	h := newDeleteHelper(t)
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

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

	if err := h.run("delete abc --yes --force --format json"); err == nil {
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
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })

	var gotPort int
	t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		gotPort = port
		return nil, desktopclient.UnreachableError()
	}))

	_ = h.run("delete abc --yes --force --port 44230 --format json")
	if gotPort != 44230 {
		t.Fatalf("expected --port=44230 to propagate to client constructor, got %d", gotPort)
	}
}

func TestDelete_DesktopUnreachable_ReturnsCanonicalError(t *testing.T) {
	h := newDeleteHelper(t)
	confirm.SetStdinIsTerminalForTest(t, func() bool { return false })
	t.Cleanup(dbms.SetNewDesktopClientFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return nil, desktopclient.UnreachableError()
	}))

	err := h.run("delete abc --yes --force --format json")
	if err == nil {
		t.Fatalf("expected error when desktop is unreachable")
	}
	if !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Fatalf("expected canonical REQ-F-008 message, got: %v", err)
	}
}
