// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package tag_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/spf13/afero"
)

// assertFatalCatalogErr pins the create/update finder policy: a mutated entry
// that cannot be identified in Desktop's returned catalog state is a clierr
// FATAL error (exit 1) carrying each wanted substring — never a silently
// printed stale entry. Lives here (not helpers_test.go) so the helper file
// stays untouched by this change.
func assertFatalCatalogErr(t *testing.T, err error, substrings ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected fatal error, got nil")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != 1 {
		t.Fatalf("error = %v (%T), want clierr fatal error (exit 1)", err, err)
	}
	for _, s := range substrings {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("error = %q, want substring %q", err.Error(), s)
		}
	}
}

// readBody captures the JSON body of a request so tests can assert the wire
// shape — minimum fields populated, no unexpected extras.
func readBody(t *testing.T, r *http.Request) map[string]any {
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

// TestCreate_RequiresName guards against the positional being silently
// optional.
func TestCreate_RequiresName(t *testing.T) {
	h := newTagHelper(t)
	if err := h.run(`tag create`); err == nil {
		t.Fatalf("expected error when <name> is missing")
	}
}

// TestCreate_SuccessfulCreate covers the happy path: POST /tags carries only
// the name when --color is unset, and the created tag is resolved out of the
// returned catalog state by name.
func TestCreate_SuccessfulCreate(t *testing.T) {
	h := newTagHelper(t)
	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/fastify/api/tags" {
			capturedBody = readBody(t, r)
			_, _ = w.Write([]byte(`{"tags":[{"id":"t0","name":"other"},{"id":"t1","name":"Prod","color":"5"}]}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`tag create Prod --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if capturedBody == nil {
		t.Fatalf("expected POST /tags to be called")
	}
	if capturedBody["name"] != "Prod" {
		t.Fatalf("name mismatch in body: %v", capturedBody["name"])
	}
	if _, hasColor := capturedBody["color"]; hasColor {
		t.Fatalf("color must NOT be in body when --color not supplied; got: %v", capturedBody)
	}
	if len(capturedBody) != 1 {
		t.Fatalf("expected exactly 1 key when --color omitted; got %d: %v", len(capturedBody), capturedBody)
	}

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "t1" {
		t.Fatalf("expected created tag id resolved from state, got %v", got["id"])
	}
	if got["name"] != "Prod" {
		t.Fatalf("expected name in output, got %v", got["name"])
	}
	if got["color"] != "5" {
		t.Fatalf("expected color in output, got %v", got["color"])
	}
}

// TestCreate_WithColor verifies that a valid --color is forwarded verbatim.
func TestCreate_WithColor(t *testing.T) {
	h := newTagHelper(t)
	var capturedBody map[string]any
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/fastify/api/tags" {
			capturedBody = readBody(t, r)
			_, _ = w.Write([]byte(`{"tags":[{"id":"t1","name":"Prod","color":"12"}]}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	if err := h.run(`tag create Prod --color 12 --format json`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if capturedBody["color"] != "12" {
		t.Fatalf("expected color=12 in body, got %v", capturedBody["color"])
	}
}

// TestCreate_InvalidColor_UsageError covers the palette gate: anything
// outside "1".."12" must fail with a usage error BEFORE any HTTP call.
func TestCreate_InvalidColor_UsageError(t *testing.T) {
	cases := []string{"0", "13", "blue", "#ff0000", "5.5"}
	for _, color := range cases {
		t.Run(color, func(t *testing.T) {
			h := newTagHelper(t)
			h.withHandler(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("must not hit HTTP when --color is invalid; got %s %s", r.Method, r.URL.Path)
			})

			err := h.run(`tag create Prod --color "` + color + `"`)
			if err == nil {
				t.Fatalf("expected usage error for --color %q", color)
			}
			if !strings.Contains(err.Error(), "--color") {
				t.Fatalf("expected error to mention --color, got: %v", err)
			}
			if !strings.Contains(err.Error(), "1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12") {
				t.Fatalf("expected error to list valid palette values, got: %v", err)
			}
		})
	}
}

// TestCreate_CreatedTagMissingFromState covers the drift path: Desktop's
// post-create state must contain the new tag; if not, surface a fatal error
// rather than printing `null`.
func TestCreate_CreatedTagMissingFromState(t *testing.T) {
	h := newTagHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/fastify/api/tags" {
			_, _ = w.Write([]byte(`{"tags":[]}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`tag create Prod --format json`)
	if err == nil {
		t.Fatalf("expected error when created tag is missing from returned state")
	}
	if !strings.Contains(err.Error(), "missing from the catalog state") {
		t.Fatalf("unexpected error text: %v", err)
	}
}

// TestCreate_PreExistingSameName_FailsLoud: when the post-create state holds
// two tags with the requested name (e.g. a pre-existing same-named tag),
// create MUST fail with a fatal error listing the candidate ids — it must
// never print the stale first match as the created tag.
func TestCreate_PreExistingSameName_FailsLoud(t *testing.T) {
	h := newTagHelper(t)
	h.withHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/fastify/api/tags" {
			_, _ = w.Write([]byte(`{"tags":[{"id":"t-old","name":"Prod","color":"1"},{"id":"t-new","name":"Prod","color":"5"}]}`))
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	})

	err := h.run(`tag create Prod --format json`)
	assertFatalCatalogErr(t, err, `tag "Prod" matches 2 entries`, "t-old", "t-new")
	if strings.Contains(h.out.String(), "t-old") {
		t.Fatalf("must not print the stale pre-existing tag, got:\n%s", h.out.String())
	}
}

// TestCreate_PortFlagPropagates verifies the parent's persistent --port flag
// reaches the client constructor seam.
func TestCreate_PortFlagPropagates(t *testing.T) {
	h := newTagHelper(t)
	var gotPort int
	t.Cleanup(desktopclient.SetConnectFnForTest(func(_ context.Context, _ afero.Fs, port int) (*desktopclient.Client, error) {
		gotPort = port
		return nil, desktopclient.UnreachableError()
	}))

	_ = h.run(`tag create Prod --port 44231 --format json`)
	if gotPort != 44231 {
		t.Fatalf("expected --port=44231 to propagate; got %d", gotPort)
	}
}

// TestCreate_DesktopUnreachable_ReturnsCanonicalError verifies the canonical
// error mapping when Desktop is not running.
func TestCreate_DesktopUnreachable_ReturnsCanonicalError(t *testing.T) {
	h := newTagHelper(t)
	t.Cleanup(desktopclient.SetConnectFnForTest(func(_ context.Context, _ afero.Fs, _ int) (*desktopclient.Client, error) {
		return nil, desktopclient.UnreachableError()
	}))

	err := h.run(`tag create Prod --format json`)
	if err == nil {
		t.Fatalf("expected error when desktop is unreachable")
	}
	if !strings.Contains(err.Error(), "doesn't appear to be running") {
		t.Fatalf("expected canonical unreachable message, got: %v", err)
	}
}
