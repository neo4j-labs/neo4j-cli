// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// readRequestBody captures the JSON body of a mutation call so tests can
// assert the wire shape.
func readRequestBody(t *testing.T, r *http.Request) map[string]any {
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

func TestCreate_PostsNameAndPrintsCreatedEntry(t *testing.T) {
	h := newProjectHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/fastify/api/projects" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			return
		}
		body := readRequestBody(t, r)
		if body["name"] != "my-project" {
			t.Errorf("expected POST body name=my-project, got %v", body)
		}
		if len(body) != 1 {
			t.Errorf("expected POST body to carry only `name`, got %v", body)
		}
		_, _ = w.Write([]byte(`{"projects":[
			{"id":"p1","name":"other","createdAt":1700000000000},
			{"id":"p2","name":"my-project","createdAt":1747843200000}
		],"currentProject":"p2"}`))
	})
	if err := h.run("project create my-project --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "p2" {
		t.Fatalf("expected created id=p2, got %v", got["id"])
	}
	if got["name"] != "my-project" {
		t.Fatalf("expected name=my-project, got %v", got["name"])
	}
	if got["created_at"] != wantCreatedAtRFC3339() {
		t.Fatalf("expected created_at=%q, got %v", wantCreatedAtRFC3339(), got["created_at"])
	}
}

// TestCreate_AmbiguousName_FailsLoud: when the post-create state holds two
// projects with the requested name, create MUST fail with a fatal error
// listing the candidate ids — never print one of them as the created entry.
func TestCreate_AmbiguousName_FailsLoud(t *testing.T) {
	h := newProjectHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"projects":[
			{"id":"p-old","name":"my-project","createdAt":1700000000000},
			{"id":"p-new","name":"my-project","createdAt":1747843200000}
		]}`))
	})
	err := h.run("project create my-project --format json")
	assertFatalCatalogError(t, err, `project "my-project" matches 2 entries`, "p-old", "p-new")
}

// TestCreate_CreatedProjectMissingFromState: when the post-create state lacks
// the requested name, create MUST fail with a fatal error instead of printing
// literal `null` with exit 0.
func TestCreate_CreatedProjectMissingFromState(t *testing.T) {
	h := newProjectHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"projects":[{"id":"p1","name":"other","createdAt":1700000000000}]}`))
	})
	err := h.run("project create my-project --format json")
	assertFatalCatalogError(t, err, `project "my-project" is missing from the catalog state`)
	if strings.Contains(h.out.String(), "null") {
		t.Fatalf("must not print null on failure, got:\n%s", h.out.String())
	}
}

func TestCreate_Table_RendersRow(t *testing.T) {
	h := newProjectHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"projects":[{"id":"p2","name":"my-project","createdAt":1747843200000}]}`))
	})
	if err := h.run("project create my-project --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	for _, want := range []string{"p2", "my-project", wantCreatedAtRFC3339()} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in table output, got:\n%s", want, out)
		}
	}
}

// TestCreate_RequiresName guards against the positional being silently
// optional.
func TestCreate_RequiresName(t *testing.T) {
	h := newProjectHelper(t)
	if err := h.run("project create"); err == nil {
		t.Fatalf("expected error when <name> is missing")
	}
}

// TestCreate_Annotated_Write: the create leaf must be tagged write=true so
// the root --rw enforcement fires.
func TestCreate_Annotated_Write(t *testing.T) {
	leaf := findProjectLeaf(t, "create")
	if leaf.Annotations["write"] != "true" {
		t.Fatalf("create must be annotated write=true; got %v", leaf.Annotations)
	}
}

// TestCreate_Example_FlushLeft mirrors the whole-tree example gate.
func TestCreate_Example_FlushLeft(t *testing.T) {
	leaf := findProjectLeaf(t, "create")
	if leaf.Example == "" {
		t.Fatalf("create Example must be non-empty")
	}
	firstLine := strings.SplitN(leaf.Example, "\n", 2)[0]
	if strings.HasPrefix(firstLine, "  ") {
		t.Fatalf("create Example first line must be flush-left; got %q", firstLine)
	}
	if c := strings.Count(leaf.Example, "neo4j-cli desktop project create"); c < 2 {
		t.Fatalf("create Example must contain >=2 invocations; got %d", c)
	}
	for _, want := range []string{"--rw", "--format json"} {
		if !strings.Contains(leaf.Example, want) {
			t.Fatalf("create Example must contain %q; got:\n%s", want, leaf.Example)
		}
	}
}

func TestCreate_5xx_SurfacesError(t *testing.T) {
	h := newProjectHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("oops"))
	})
	err := h.run("project create my-project --format json")
	if err == nil {
		t.Fatalf("expected error on 5xx")
	}
	if !strings.Contains(err.Error(), "Neo4j Desktop 2") {
		t.Fatalf("expected Desktop-flavoured error text, got: %v", err)
	}
}
