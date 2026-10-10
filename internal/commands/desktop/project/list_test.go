// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package project_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/spf13/afero"
)

func TestProjectList_JSON_RendersProjectArray(t *testing.T) {
	h := newProjectHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/fastify/api/projects" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			return
		}
		_, _ = w.Write([]byte(`{"projects":[
			{"id":"p1","name":"alpha","createdAt":1747843200000},
			{"id":"p2","name":"beta"}
		],"currentProject":"p1"}`))
	})
	if err := h.run("project list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 projects, got %d (raw: %s)", len(got), h.out.String())
	}
	if got[0]["id"] != "p1" || got[1]["id"] != "p2" {
		t.Fatalf("expected ids [p1 p2], got %v", got)
	}
	if got[0]["name"] != "alpha" || got[1]["name"] != "beta" {
		t.Fatalf("expected names [alpha beta], got %v", got)
	}
	// created_at renders as RFC3339 UTC; omitted entirely when Desktop does
	// not report a timestamp (legacy entries).
	if got[0]["created_at"] != wantCreatedAtRFC3339() {
		t.Fatalf("expected created_at=%q, got %v", wantCreatedAtRFC3339(), got[0]["created_at"])
	}
	if _, ok := got[1]["created_at"]; ok {
		t.Fatalf("expected created_at omitted for legacy entry, got %v", got[1])
	}
}

func TestProjectList_EmptyJSON_RendersEmptyArrayNotNull(t *testing.T) {
	h := newProjectHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"projects":[]}`))
	})
	if err := h.run("project list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	trimmed := strings.TrimSpace(h.out.String())
	if trimmed != "[]" {
		t.Fatalf("expected `[]` for empty project list, got %q", trimmed)
	}
}

func TestProjectList_EmptyTable_RendersNonePlaceholder(t *testing.T) {
	h := newProjectHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"projects":[]}`))
	})
	if err := h.run("project list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	if !strings.Contains(out, "(none)") {
		t.Fatalf("expected `(none)` placeholder in empty table, got:\n%s", out)
	}
	// Column headers must still appear so the user knows what they're looking
	// at even with no rows.
	for _, col := range []string{"ID", "NAME", "CREATED_AT"} {
		if !strings.Contains(strings.ToUpper(out), col) {
			t.Fatalf("expected column %q in header, got:\n%s", col, out)
		}
	}
}

func TestProjectList_Table_RendersRows(t *testing.T) {
	h := newProjectHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"projects":[{"id":"p1","name":"alpha","createdAt":1747843200000}]}`))
	})
	if err := h.run("project list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	for _, want := range []string{"p1", "alpha", wantCreatedAtRFC3339()} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in table output, got:\n%s", want, out)
		}
	}
}

func TestProjectList_5xx_SurfacesError(t *testing.T) {
	h := newProjectHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("oops"))
	})
	err := h.run("project list --format json")
	if err == nil {
		t.Fatalf("expected error on 5xx")
	}
	if !strings.Contains(err.Error(), "Neo4j Desktop 2") {
		t.Fatalf("expected Desktop-flavoured error text, got: %v", err)
	}
}

func TestProjectList_PortFlagPropagatesToClientConstructor(t *testing.T) {
	h := newProjectHelper(t)
	var seenPort int
	t.Cleanup(desktopclient.SetConnectFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		seenPort = port
		return nil, errors.New("stop here; we already captured the port")
	}))
	_ = h.run("project list --port 44225 --format json")
	if seenPort != 44225 {
		t.Fatalf("expected --port=44225 to reach the client constructor, got %d", seenPort)
	}
}
