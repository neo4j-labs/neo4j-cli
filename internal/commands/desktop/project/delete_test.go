// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/neo4j/cli/internal/confirm"
)

// TestDelete_ByUUID_SendsQuerystringDelete: a UUID positional skips the
// catalog lookup; the id rides in the DELETE querystring and the output is a
// slim `{id, deleted}` envelope.
func TestDelete_ByUUID_SendsQuerystringDelete(t *testing.T) {
	h := newProjectHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

	var listCalls atomic.Int32
	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/projects":
			listCalls.Add(1)
			t.Errorf("UUID positional must not trigger a catalog lookup; got GET %s", r.URL.Path)
		case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/projects":
			deleteCalls.Add(1)
			if got := r.URL.Query().Get("id"); got != validProjectID {
				t.Errorf("expected DELETE querystring id=%s, got %q", validProjectID, got)
			}
			_, _ = w.Write([]byte(`{"projects":[]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("project delete " + validProjectID + " --yes --force --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if listCalls.Load() != 0 {
		t.Fatalf("expected no GET /projects for a UUID positional; got %d", listCalls.Load())
	}
	if deleteCalls.Load() != 1 {
		t.Fatalf("expected exactly 1 DELETE call; got %d", deleteCalls.Load())
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != validProjectID {
		t.Fatalf("expected id in output, got %v", got["id"])
	}
	if got["deleted"] != true {
		t.Fatalf("expected deleted=true in output, got %v", got["deleted"])
	}
	if len(got) != 2 {
		t.Fatalf("expected exactly 2 keys (id, deleted); got %d: %s", len(got), h.out.String())
	}
}

// TestDelete_ByName_ResolvesViaCatalog: an exact project name resolves to its
// id through a GET /projects lookup before the DELETE.
func TestDelete_ByName_ResolvesViaCatalog(t *testing.T) {
	h := newProjectHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

	var deletedID string
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/projects":
			_, _ = w.Write([]byte(`{"projects":[{"id":"p1","name":"alpha"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/projects":
			deletedID = r.URL.Query().Get("id")
			_, _ = w.Write([]byte(`{"projects":[]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run("project delete alpha --yes --force --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if deletedID != "p1" {
		t.Fatalf("expected resolved id p1 in DELETE querystring, got %q", deletedID)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "p1" {
		t.Fatalf("expected id=p1 in output, got %v", got["id"])
	}
}

// TestDelete_TableConfirmation: default (table) format emits a one-line
// confirmation carrying the removed id.
func TestDelete_TableConfirmation(t *testing.T) {
	h := newProjectHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/fastify/api/projects" {
			_, _ = w.Write([]byte(`{"projects":[]}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run("project delete " + validProjectID + " --yes --force --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(h.out.String())
	want := "Deleted project " + validProjectID + "."
	if got != want {
		t.Fatalf("table confirmation mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

// TestDelete_NonTTY_OnlyYes_Exit2: both --yes and --force are required for
// non-TTY callers; the confirm gate fires BEFORE any HTTP call.
func TestDelete_NonTTY_OnlyYes_Exit2(t *testing.T) {
	h := newProjectHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("non-TTY without --force must not hit the API; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run("project delete " + validProjectID + " --yes")
	if err == nil {
		t.Fatalf("expected usage error when --force is missing")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("expected error to mention 'pass both --yes and --force', got: %v", err)
	}
}

func TestDelete_NonTTY_OnlyForce_Exit2(t *testing.T) {
	h := newProjectHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("non-TTY without --yes must not hit the API; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run("project delete " + validProjectID + " --force")
	if err == nil {
		t.Fatalf("expected usage error when --yes is missing")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("expected error to mention 'pass both --yes and --force', got: %v", err)
	}
}

// TestDelete_TTY_EmptyStdin_Cancels: TTY + empty stdin → cancelled, no DELETE.
func TestDelete_TTY_EmptyStdin_Cancels(t *testing.T) {
	h := newProjectHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return true }))
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

	err := h.run("project delete " + validProjectID + " --format json")
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected confirm.ErrCancelled on EOF cancel; got: %v", err)
	}
	if deleteCalls.Load() != 0 {
		t.Fatalf("expected zero DELETE on EOF; got %d", deleteCalls.Load())
	}
}

// TestDelete_UnknownName_UsageError: a name absent from the catalog is a
// usage error before any DELETE.
func TestDelete_UnknownName_UsageError(t *testing.T) {
	h := newProjectHelper(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))

	var deleteCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/projects":
			_, _ = w.Write([]byte(`{"projects":[{"id":"p1","name":"alpha"}]}`))
		case r.Method == http.MethodDelete:
			deleteCalls.Add(1)
			t.Errorf("DELETE must not fire for an unknown name")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run("project delete nope --yes --force --format json")
	if err == nil {
		t.Fatalf("expected usage error for unknown project name")
	}
	if !strings.Contains(err.Error(), "unknown project") {
		t.Fatalf("expected 'unknown project' error, got: %v", err)
	}
	if deleteCalls.Load() != 0 {
		t.Fatalf("expected zero DELETE calls; got %d", deleteCalls.Load())
	}
}

// TestDelete_RequiresPositional guards against the positional being silently
// optional.
func TestDelete_RequiresPositional(t *testing.T) {
	h := newProjectHelper(t)
	if err := h.run("project delete --yes --force"); err == nil {
		t.Fatalf("expected error when <project> is missing")
	}
}

// TestDelete_Annotated_Write: the delete leaf must be tagged write=true so
// the root --rw enforcement fires.
func TestDelete_Annotated_Write(t *testing.T) {
	leaf := findProjectLeaf(t, "delete")
	if leaf.Annotations["write"] != "true" {
		t.Fatalf("delete must be annotated write=true; got %v", leaf.Annotations)
	}
}

// TestDelete_Example_FlushLeft mirrors the whole-tree example gate.
func TestDelete_Example_FlushLeft(t *testing.T) {
	leaf := findProjectLeaf(t, "delete")
	if leaf.Example == "" {
		t.Fatalf("delete Example must be non-empty")
	}
	firstLine := strings.SplitN(leaf.Example, "\n", 2)[0]
	if strings.HasPrefix(firstLine, "  ") {
		t.Fatalf("delete Example first line must be flush-left; got %q", firstLine)
	}
	if c := strings.Count(leaf.Example, "neo4j-cli desktop project delete"); c < 3 {
		t.Fatalf("delete Example must contain >=3 invocations; got %d", c)
	}
	for _, want := range []string{"--rw", "--format json"} {
		if !strings.Contains(leaf.Example, want) {
			t.Fatalf("delete Example must contain %q; got:\n%s", want, leaf.Example)
		}
	}
}
