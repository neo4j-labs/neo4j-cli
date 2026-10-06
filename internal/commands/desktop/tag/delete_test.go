// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package tag_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/neo4j/cli/internal/confirm"
	"github.com/neo4j/cli/internal/confirm/confirmtest"
)

// TestDelete_RequiresPositional guards against the positional being silently
// optional.
func TestDelete_RequiresPositional(t *testing.T) {
	h := newTagHelper(t)
	if err := h.run(`tag delete`); err == nil {
		t.Fatalf("expected error when <tag> is missing")
	}
}

// TestDelete_ConfirmGate is the canonical (TTY × flag-state) replay shared
// across every destructive leaf. The UUID positional keeps ResolveTagIDs off
// the network so the only wire call is the DELETE itself.
func TestDelete_ConfirmGate(t *testing.T) {
	confirmtest.AssertLeafGate(t, confirmtest.LeafGateCase{
		Name:          "desktop tag delete",
		NoFlagsArgs:   "tag delete " + validTagID + " --format json",
		BothFlagsArgs: "tag delete " + validTagID + " --yes --force --format json",
		ResourceLabel: "tag",
		Run: func(t *testing.T, args, stdin string) confirmtest.GateRunResult {
			h := newTagHelper(t)
			h.in.WriteString(stdin)
			var deleteCalls atomic.Int32
			h.withHandler(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deleteCalls.Add(1)
					_, _ = w.Write([]byte(`{"tags":[]}`))
					return
				}
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			})
			err := h.run(args)
			return confirmtest.GateRunResult{Err: err, Stderr: h.err.String(), Invoked: deleteCalls.Load() > 0}
		},
	})
}

// TestDelete_NonTTY_OnlyYes_Exit2: both --yes and --force are required for
// non-TTY callers; the confirm gate fires BEFORE any client construction or
// catalog lookup, so the API sees zero requests. A NAME positional makes the
// ordering observable: pre-convergence the resolve GET ran before the gate.
func TestDelete_NonTTY_OnlyYes_Exit2(t *testing.T) {
	h := newTagHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("non-TTY without --force must not hit the API; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`tag delete staging --yes`)
	if err == nil {
		t.Fatalf("expected usage error when --force is missing")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("expected error to mention 'pass both --yes and --force', got: %v", err)
	}
}

func TestDelete_NonTTY_OnlyForce_Exit2(t *testing.T) {
	h := newTagHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("non-TTY without --yes must not hit the API; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`tag delete staging --force`)
	if err == nil {
		t.Fatalf("expected usage error when --yes is missing")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("expected error to mention 'pass both --yes and --force', got: %v", err)
	}
}

// TestDelete_TTY_EmptyStdin_Cancels: TTY + empty stdin → cancelled, no DELETE.
func TestDelete_TTY_EmptyStdin_Cancels(t *testing.T) {
	h := newTagHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return true }))
	// Empty buffer simulates immediate EOF.

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request should fire when stdin EOFs before any input; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`tag delete ` + validTagID + ` --format json`)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected confirm.ErrCancelled on EOF cancel; got: %v", err)
	}
}

// TestDelete_TTY_Decline_NoHTTP: a TTY caller answering "n" cancels before
// any client construction or catalog lookup — the name positional would have
// triggered a resolve GET under the old ordering, so zero requests proves
// confirm-first. The prompt must echo what the user typed, not a resolved id.
func TestDelete_TTY_Decline_NoHTTP(t *testing.T) {
	h := newTagHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return true }))
	h.in.WriteString("n\n")

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no request should fire when the TTY prompt is declined; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`tag delete staging --format json`)
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected confirm.ErrCancelled on decline; got: %v", err)
	}
	if !strings.Contains(h.err.String(), `tag "staging"`) {
		t.Fatalf("expected prompt to echo the typed name; stderr=%q", h.err.String())
	}
}

// TestDelete_ByUUID_SendsQuerystring covers the wire shape: the id rides in
// the QUERYSTRING (`DELETE /tags?id=<uuid>`) with no body, and the JSON
// output is the slim `{id, deleted}` envelope (aligned with project delete).
func TestDelete_ByUUID_SendsQuerystring(t *testing.T) {
	h := newTagHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			t.Errorf("UUID positional must not trigger a catalog GET; got GET %s", r.URL.Path)
		case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/tags":
			deleteCalls.Add(1)
			if got := r.URL.Query().Get("id"); got != validTagID {
				t.Errorf("expected querystring id=%s, got %q", validTagID, got)
			}
			_, _ = w.Write([]byte(`{"tags":[]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run(`tag delete ` + validTagID + ` --yes --force --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if deleteCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 DELETE call; got %d", deleteCalls.Load())
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != validTagID {
		t.Fatalf("expected removed id in output, got %v", got)
	}
	if got["deleted"] != true {
		t.Fatalf("expected deleted=true in output, got %v", got["deleted"])
	}
	if len(got) != 2 {
		t.Fatalf("expected exactly 2 keys (id, deleted) in the slim envelope; got %d: %s", len(got), h.out.String())
	}
}

// TestDelete_ByName_ResolvesThenDeletes covers the name path: the positional
// is resolved to an id via ResolveTagIDs (one GET /tags) before the DELETE.
func TestDelete_ByName_ResolvesThenDeletes(t *testing.T) {
	h := newTagHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/tags":
			_, _ = w.Write([]byte(`{"tags":[{"id":"t1","name":"staging"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/tags":
			deleteCalls.Add(1)
			if got := r.URL.Query().Get("id"); got != "t1" {
				t.Errorf("expected querystring id=t1, got %q", got)
			}
			_, _ = w.Write([]byte(`{"tags":[]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run(`tag delete staging --yes --force --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if deleteCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 DELETE call; got %d", deleteCalls.Load())
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "t1" {
		t.Fatalf("expected removed id t1 in output, got %v", got)
	}
	if got["deleted"] != true {
		t.Fatalf("expected deleted=true in output, got %v", got["deleted"])
	}
}

// TestDelete_UnknownName_UsageError covers the ResolveTagIDs miss: an
// unknown name must surface a usage error before any DELETE.
func TestDelete_UnknownName_UsageError(t *testing.T) {
	h := newTagHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/tags":
			_, _ = w.Write([]byte(`{"tags":[{"id":"t1","name":"staging"}]}`))
		case r.Method == http.MethodDelete:
			t.Errorf("must not DELETE when the name does not resolve")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run(`tag delete nope --yes --force --format json`)
	if err == nil {
		t.Fatalf("expected usage error for unknown tag name")
	}
	if !strings.Contains(err.Error(), "unknown tag") {
		t.Fatalf("expected 'unknown tag' error, got: %v", err)
	}
}

// TestDelete_TableConfirmation: default (table) format emits a one-line
// confirmation carrying the removed id.
func TestDelete_TableConfirmation(t *testing.T) {
	h := newTagHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/tags" {
			_, _ = w.Write([]byte(`{"tags":[]}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`tag delete ` + validTagID + ` --yes --force --format table`); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(h.out.String())
	want := `Deleted tag ` + validTagID + `.`
	if got != want {
		t.Fatalf("table confirmation mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}
