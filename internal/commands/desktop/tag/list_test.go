// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package tag_test

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

func TestTagList_JSON_RendersTagArray(t *testing.T) {
	h := newTagHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/fastify/api/tags" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			return
		}
		_, _ = w.Write([]byte(`{"tags":[{"id":"t1","name":"Prod","color":"5"},{"id":"t2","name":"Dev"}],"filter":{"selectedTagIds":["t1"]}}`))
	})
	if err := h.run("tag list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tags, got %d (raw: %s)", len(got), h.out.String())
	}
	if got[0]["id"] != "t1" || got[0]["name"] != "Prod" || got[0]["color"] != "5" {
		t.Fatalf("unexpected first tag: %v", got[0])
	}
	if got[1]["id"] != "t2" || got[1]["name"] != "Dev" {
		t.Fatalf("unexpected second tag: %v", got[1])
	}
}

func TestTagList_EmptyJSON_RendersEmptyArrayNotNull(t *testing.T) {
	h := newTagHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tags":[]}`))
	})
	if err := h.run("tag list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	trimmed := strings.TrimSpace(h.out.String())
	if trimmed != "[]" {
		t.Fatalf("expected `[]` for empty tag list, got %q", trimmed)
	}
}

func TestTagList_Table_RendersOneRowPerTag(t *testing.T) {
	h := newTagHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tags":[{"id":"t1","name":"Prod","color":"5"},{"id":"t2","name":"Dev"}]}`))
	})
	if err := h.run("tag list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	for _, col := range []string{"ID", "NAME", "COLOR"} {
		if !strings.Contains(strings.ToUpper(out), col) {
			t.Fatalf("expected column %q in header, got:\n%s", col, out)
		}
	}
	for _, cell := range []string{"t1", "Prod", "5", "t2", "Dev"} {
		if !strings.Contains(out, cell) {
			t.Fatalf("expected cell %q in table, got:\n%s", cell, out)
		}
	}
}

func TestTagList_EmptyTable_RendersNonePlaceholder(t *testing.T) {
	h := newTagHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tags":[]}`))
	})
	if err := h.run("tag list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	if !strings.Contains(out, "(none)") {
		t.Fatalf("expected `(none)` placeholder in empty table, got:\n%s", out)
	}
	// Column headers must still appear so the user knows what they're looking
	// at even with no rows.
	for _, col := range []string{"ID", "NAME", "COLOR"} {
		if !strings.Contains(strings.ToUpper(out), col) {
			t.Fatalf("expected column %q in header, got:\n%s", col, out)
		}
	}
}

func TestTagList_5xx_SurfacesError(t *testing.T) {
	h := newTagHelper(t)
	h.withHandler(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("oops"))
	})
	err := h.run("tag list --format json")
	if err == nil {
		t.Fatalf("expected error on 5xx")
	}
	if !strings.Contains(err.Error(), "Neo4j Desktop 2") {
		t.Fatalf("expected Desktop-flavoured error text, got: %v", err)
	}
}

func TestTagList_PortFlagPropagatesToClientConstructor(t *testing.T) {
	h := newTagHelper(t)
	var seenPort int
	t.Cleanup(desktopclient.SetConnectFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		seenPort = port
		return nil, errors.New("stop here; we already captured the port")
	}))
	_ = h.run("tag list --port 44225 --format json")
	if seenPort != 44225 {
		t.Fatalf("expected --port=44225 to reach the client constructor, got %d", seenPort)
	}
}
