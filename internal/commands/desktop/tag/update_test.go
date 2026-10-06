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
)

// TestUpdate_RequiresNameFlag covers cobra's MarkFlagRequired enforcement:
// --name is mandatory because Desktop's TagUpdateSchema demands it.
func TestUpdate_RequiresNameFlag(t *testing.T) {
	h := newTagHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not hit HTTP when --name is missing; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`tag update ` + validTagID)
	if err == nil {
		t.Fatalf("expected required-flag error when --name is missing")
	}
	if !strings.Contains(err.Error(), "required") || !strings.Contains(err.Error(), `"name"`) {
		t.Fatalf("expected required-flag error mentioning \"name\", got: %v", err)
	}
}

// TestUpdate_ByUUID covers the UUID fast-path: ResolveTagIDs passes the UUID
// through without a catalog GET, and the PATCH body carries {id, name} only
// when --color is unset.
func TestUpdate_ByUUID(t *testing.T) {
	h := newTagHelper(t)
	var listCalls atomic.Int32
	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/tags":
			listCalls.Add(1)
			t.Errorf("UUID positional must not trigger a catalog GET; got GET %s", r.URL.Path)
		case r.Method == http.MethodPatch && r.URL.Path == "/fastify/api/tags":
			capturedBody = readBody(t, r)
			_, _ = w.Write([]byte(`{"tags":[{"id":"` + validTagID + `","name":"pre-prod","color":"5"}]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run(`tag update ` + validTagID + ` --name pre-prod --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if listCalls.Load() != 0 {
		t.Fatalf("UUID positional must not trigger a catalog GET; got %d", listCalls.Load())
	}
	if capturedBody["id"] != validTagID {
		t.Fatalf("id mismatch in body: %v", capturedBody["id"])
	}
	if capturedBody["name"] != "pre-prod" {
		t.Fatalf("name mismatch in body: %v", capturedBody["name"])
	}
	if _, hasColor := capturedBody["color"]; hasColor {
		t.Fatalf("color must NOT be in body when --color not supplied; got: %v", capturedBody)
	}
	if len(capturedBody) != 2 {
		t.Fatalf("expected exactly 2 keys when --color omitted; got %d: %v", len(capturedBody), capturedBody)
	}

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != validTagID || got["name"] != "pre-prod" || got["color"] != "5" {
		t.Fatalf("unexpected output: %v", got)
	}
}

// TestUpdate_ByName_ResolvesViaCatalog covers the name path: the positional
// is resolved to an id via ResolveTagIDs (one GET /tags) before the PATCH.
func TestUpdate_ByName_ResolvesViaCatalog(t *testing.T) {
	h := newTagHelper(t)
	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/tags":
			_, _ = w.Write([]byte(`{"tags":[{"id":"t1","name":"staging","color":"2"}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/fastify/api/tags":
			capturedBody = readBody(t, r)
			_, _ = w.Write([]byte(`{"tags":[{"id":"t1","name":"pre-prod","color":"7"}]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := h.run(`tag update staging --name pre-prod --color 7 --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if capturedBody["id"] != "t1" {
		t.Fatalf("expected resolved id t1 in body, got %v", capturedBody["id"])
	}
	if capturedBody["name"] != "pre-prod" {
		t.Fatalf("name mismatch in body: %v", capturedBody["name"])
	}
	if capturedBody["color"] != "7" {
		t.Fatalf("expected color=7 in body, got %v", capturedBody["color"])
	}

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "t1" || got["name"] != "pre-prod" || got["color"] != "7" {
		t.Fatalf("unexpected output: %v", got)
	}
}

// TestUpdate_UpdatedTagMissingFromState: when the post-update state lacks the
// patched id, update MUST fail with a fatal error instead of printing `null`.
func TestUpdate_UpdatedTagMissingFromState(t *testing.T) {
	h := newTagHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/fastify/api/tags":
			_, _ = w.Write([]byte(`{"tags":[{"id":"t-other","name":"pre-prod"}]}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run(`tag update ` + validTagID + ` --name pre-prod --format json`)
	assertFatalCatalogErr(t, err, `tag "`+validTagID+`" is missing from the catalog state`)
	if strings.Contains(h.out.String(), "null") {
		t.Fatalf("must not print null on failure, got:\n%s", h.out.String())
	}
}

// TestUpdate_UnknownName_UsageError covers the ResolveTagIDs miss: an
// unknown name must surface a usage error before any PATCH.
func TestUpdate_UnknownName_UsageError(t *testing.T) {
	h := newTagHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/fastify/api/tags":
			_, _ = w.Write([]byte(`{"tags":[{"id":"t1","name":"staging"}]}`))
		case r.Method == http.MethodPatch:
			t.Errorf("must not PATCH when the name does not resolve")
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	err := h.run(`tag update nope --name x --format json`)
	if err == nil {
		t.Fatalf("expected usage error for unknown tag name")
	}
	if !strings.Contains(err.Error(), "unknown tag") {
		t.Fatalf("expected 'unknown tag' error, got: %v", err)
	}
}

// TestUpdate_InvalidColor_UsageError covers the palette gate: anything
// outside "1".."12" must fail with a usage error BEFORE any HTTP call.
func TestUpdate_InvalidColor_UsageError(t *testing.T) {
	h := newTagHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not hit HTTP when --color is invalid; got %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`tag update ` + validTagID + ` --name x --color 99`)
	if err == nil {
		t.Fatalf("expected usage error for --color 99")
	}
	if !strings.Contains(err.Error(), "--color") {
		t.Fatalf("expected error to mention --color, got: %v", err)
	}
	if !strings.Contains(err.Error(), "1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12") {
		t.Fatalf("expected error to list valid palette values, got: %v", err)
	}
}
