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
)

const validProjectID = "f4e2f3c0-1111-2222-3333-444455556666"

// TestUpdate_ByUUID_PatchesIDAndName: a UUID positional skips the catalog
// lookup; the PATCH body carries `{id, name}` and the printed entry is
// resolved from the post-update state by id.
func TestUpdate_ByUUID_PatchesIDAndName(t *testing.T) {
	h := newProjectHelper(t)
	var listCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/projects":
			listCalls.Add(1)
			t.Errorf("UUID positional must not trigger a catalog lookup; got GET %s", r.URL.Path)
		case r.Method == http.MethodPatch && r.URL.Path == "/fastify/api/projects":
			body := readRequestBody(t, r)
			if body["id"] != validProjectID || body["name"] != "renamed" {
				t.Errorf("expected PATCH body {id, name}, got %v", body)
			}
			if len(body) != 2 {
				t.Errorf("expected PATCH body to carry only id+name, got %v", body)
			}
			_, _ = w.Write([]byte(`{"projects":[{"id":"` + validProjectID + `","name":"renamed","createdAt":1747843200000}]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	if err := h.run("project update " + validProjectID + " --name renamed --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if listCalls.Load() != 0 {
		t.Fatalf("expected no GET /projects for a UUID positional; got %d", listCalls.Load())
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != validProjectID || got["name"] != "renamed" {
		t.Fatalf("expected updated entry, got %v", got)
	}
	if got["created_at"] != wantCreatedAtRFC3339() {
		t.Fatalf("expected created_at=%q, got %v", wantCreatedAtRFC3339(), got["created_at"])
	}
}

// TestUpdate_ByName_ResolvesViaCatalog: an exact project name resolves to its
// id through a GET /projects lookup before the PATCH.
func TestUpdate_ByName_ResolvesViaCatalog(t *testing.T) {
	h := newProjectHelper(t)
	var patchBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/projects":
			_, _ = w.Write([]byte(`{"projects":[{"id":"p1","name":"alpha","createdAt":1700000000000}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/fastify/api/projects":
			patchBody = readRequestBody(t, r)
			_, _ = w.Write([]byte(`{"projects":[{"id":"p1","name":"renamed","createdAt":1700000000000}]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	if err := h.run("project update alpha --name renamed --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if patchBody["id"] != "p1" {
		t.Fatalf("expected resolved id p1 in PATCH body, got %v", patchBody)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "p1" || got["name"] != "renamed" {
		t.Fatalf("expected updated entry, got %v", got)
	}
}

// TestUpdate_UpdatedProjectMissingFromState: when the post-update state lacks
// the patched id, update MUST fail with a fatal error instead of printing
// literal `null` with exit 0.
func TestUpdate_UpdatedProjectMissingFromState(t *testing.T) {
	h := newProjectHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/fastify/api/projects":
			_, _ = w.Write([]byte(`{"projects":[{"id":"p-other","name":"renamed","createdAt":1747843200000}]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	err := h.run("project update " + validProjectID + " --name renamed --format json")
	assertFatalCatalogError(t, err, `project "`+validProjectID+`" is missing from the catalog state`)
	if strings.Contains(h.out.String(), "null") {
		t.Fatalf("must not print null on failure, got:\n%s", h.out.String())
	}
}

// TestUpdate_UnknownName_UsageError: a name absent from the catalog is a
// usage error before any PATCH.
func TestUpdate_UnknownName_UsageError(t *testing.T) {
	h := newProjectHelper(t)
	var patchCalls atomic.Int32
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/projects":
			_, _ = w.Write([]byte(`{"projects":[{"id":"p1","name":"alpha"}]}`))
		case r.Method == http.MethodPatch:
			patchCalls.Add(1)
			t.Errorf("PATCH must not fire for an unknown name")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})
	err := h.run("project update nope --name renamed --format json")
	if err == nil {
		t.Fatalf("expected usage error for unknown project name")
	}
	if !strings.Contains(err.Error(), "unknown project") {
		t.Fatalf("expected 'unknown project' error, got: %v", err)
	}
	if patchCalls.Load() != 0 {
		t.Fatalf("expected zero PATCH calls; got %d", patchCalls.Load())
	}
}

// TestUpdate_RequiresName: cobra's MarkFlagRequired enforcement — missing
// --name is a usage error before any HTTP call.
func TestUpdate_RequiresName(t *testing.T) {
	h := newProjectHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not hit the API when --name is missing; got %s %s", r.Method, r.URL.Path)
	})
	err := h.run("project update " + validProjectID)
	if err == nil {
		t.Fatalf("expected error when --name is missing")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("expected error to mention --name, got: %v", err)
	}
}

// TestUpdate_RequiresPositional guards against the positional being silently
// optional.
func TestUpdate_RequiresPositional(t *testing.T) {
	h := newProjectHelper(t)
	if err := h.run("project update --name renamed"); err == nil {
		t.Fatalf("expected error when <project> is missing")
	}
}

// TestUpdate_Annotated_Write: the update leaf must be tagged write=true so
// the root --rw enforcement fires.
func TestUpdate_Annotated_Write(t *testing.T) {
	leaf := findProjectLeaf(t, "update")
	if leaf.Annotations["write"] != "true" {
		t.Fatalf("update must be annotated write=true; got %v", leaf.Annotations)
	}
}

// TestUpdate_Example_FlushLeft mirrors the whole-tree example gate.
func TestUpdate_Example_FlushLeft(t *testing.T) {
	leaf := findProjectLeaf(t, "update")
	if leaf.Example == "" {
		t.Fatalf("update Example must be non-empty")
	}
	firstLine := strings.SplitN(leaf.Example, "\n", 2)[0]
	if strings.HasPrefix(firstLine, "  ") {
		t.Fatalf("update Example first line must be flush-left; got %q", firstLine)
	}
	if c := strings.Count(leaf.Example, "neo4j-cli desktop project update"); c < 2 {
		t.Fatalf("update Example must contain >=2 invocations; got %d", c)
	}
	for _, want := range []string{"--rw", "--format json"} {
		if !strings.Contains(leaf.Example, want) {
			t.Fatalf("update Example must contain %q; got:\n%s", want, leaf.Example)
		}
	}
}
