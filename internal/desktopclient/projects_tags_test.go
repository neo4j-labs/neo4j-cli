// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/neo4j/cli/internal/clierr"
)

// recordedRequest captures what a handler saw so table-driven cases can
// assert method + path + querystring + decoded body after the call.
type recordedRequest struct {
	method string
	path   string
	query  string
	body   map[string]any
}

// projectTagHandler returns a handler that records the request and responds
// with `status`/`responseBody`.
func projectTagHandler(t *testing.T, status int, responseBody string, seen *recordedRequest) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		seen.method = r.Method
		seen.path = r.URL.Path
		seen.query = r.URL.RawQuery
		if r.Body != nil && r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&seen.body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(responseBody))
	}
}

func TestClient_ProjectTagEndpoints_MethodPathBody(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	const projectsState = `{"projects":[{"id":"p1","name":"Proj","createdAt":1750000000000}],"currentProject":"p1"}`
	const tagsState = `{"tags":[{"id":"t1","name":"Prod","color":"5"}],"filter":{"selectedTagIds":["t1"]}}`

	cases := []struct {
		name       string
		call       func(cl *Client) error
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   map[string]any
		wantNoBody bool
	}{
		{
			name:       "ListProjects",
			call:       func(cl *Client) error { _, err := cl.ListProjects(context.Background()); return err },
			wantMethod: http.MethodGet,
			wantPath:   "/fastify/api/projects",
			wantNoBody: true,
		},
		{
			name:       "CreateProject",
			call:       func(cl *Client) error { _, err := cl.CreateProject(context.Background(), "Proj"); return err },
			wantMethod: http.MethodPost,
			wantPath:   "/fastify/api/projects",
			wantBody:   map[string]any{"name": "Proj"},
		},
		{
			name: "UpdateProject_IdInBody",
			call: func(cl *Client) error {
				_, err := cl.UpdateProject(context.Background(), "p1", "Renamed")
				return err
			},
			wantMethod: http.MethodPatch,
			wantPath:   "/fastify/api/projects",
			wantBody:   map[string]any{"id": "p1", "name": "Renamed"},
		},
		{
			name:       "DeleteProject_IdInQuerystring",
			call:       func(cl *Client) error { _, err := cl.DeleteProject(context.Background(), "p1"); return err },
			wantMethod: http.MethodDelete,
			wantPath:   "/fastify/api/projects",
			wantQuery:  "id=p1",
			wantNoBody: true,
		},
		{
			name:       "ListTags",
			call:       func(cl *Client) error { _, err := cl.ListTags(context.Background()); return err },
			wantMethod: http.MethodGet,
			wantPath:   "/fastify/api/tags",
			wantNoBody: true,
		},
		{
			name:       "CreateTag_WithColor",
			call:       func(cl *Client) error { _, err := cl.CreateTag(context.Background(), "Prod", "5"); return err },
			wantMethod: http.MethodPost,
			wantPath:   "/fastify/api/tags",
			wantBody:   map[string]any{"name": "Prod", "color": "5"},
		},
		{
			name:       "CreateTag_NoColorOmitsKey",
			call:       func(cl *Client) error { _, err := cl.CreateTag(context.Background(), "Prod", ""); return err },
			wantMethod: http.MethodPost,
			wantPath:   "/fastify/api/tags",
			wantBody:   map[string]any{"name": "Prod"},
		},
		{
			name: "UpdateTag_IdInBody_ColorOmittedWhenEmpty",
			call: func(cl *Client) error {
				_, err := cl.UpdateTag(context.Background(), "t1", "Renamed", "")
				return err
			},
			wantMethod: http.MethodPatch,
			wantPath:   "/fastify/api/tags",
			wantBody:   map[string]any{"id": "t1", "name": "Renamed"},
		},
		{
			name: "UpdateTag_WithColor",
			call: func(cl *Client) error {
				_, err := cl.UpdateTag(context.Background(), "t1", "Renamed", "7")
				return err
			},
			wantMethod: http.MethodPatch,
			wantPath:   "/fastify/api/tags",
			wantBody:   map[string]any{"id": "t1", "name": "Renamed", "color": "7"},
		},
		{
			name:       "DeleteTag_IdInQuerystring",
			call:       func(cl *Client) error { _, err := cl.DeleteTag(context.Background(), "t1"); return err },
			wantMethod: http.MethodDelete,
			wantPath:   "/fastify/api/tags",
			wantQuery:  "id=t1",
			wantNoBody: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seen recordedRequest
			response := projectsState
			if strings.Contains(tc.name, "Tag") {
				response = tagsState
			}
			srv, probe := newAuthedServer(t, salt, clientID, projectTagHandler(t, http.StatusOK, response, &seen))
			_ = srv

			cl, err := NewClient(probe, salt)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if err := tc.call(cl); err != nil {
				t.Fatalf("call: %v", err)
			}
			if seen.method != tc.wantMethod || seen.path != tc.wantPath {
				t.Fatalf("got %s %s, want %s %s", seen.method, seen.path, tc.wantMethod, tc.wantPath)
			}
			if seen.query != tc.wantQuery {
				t.Fatalf("query = %q, want %q", seen.query, tc.wantQuery)
			}
			if tc.wantNoBody {
				if seen.body != nil {
					t.Fatalf("expected no request body, got %+v", seen.body)
				}
				return
			}
			if len(seen.body) != len(tc.wantBody) {
				t.Fatalf("body = %+v, want exactly %+v", seen.body, tc.wantBody)
			}
			for k, v := range tc.wantBody {
				if seen.body[k] != v {
					t.Errorf("body[%q] = %v, want %v", k, seen.body[k], v)
				}
			}
		})
	}
}

func TestClient_ProjectTagEndpoints_DecodeFullState(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	t.Run("ListProjects decodes entries + drops currentProject", func(t *testing.T) {
		srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"projects":[{"id":"p1","name":"A","createdAt":1750000000000},{"id":"p2","name":"B"}],"currentProject":"p1"}`))
		})
		_ = srv
		cl, _ := NewClient(probe, salt)
		got, err := cl.ListProjects(context.Background())
		if err != nil {
			t.Fatalf("ListProjects: %v", err)
		}
		if len(got) != 2 || got[0].ID != "p1" || got[0].Name != "A" || got[0].CreatedAt != 1750000000000 {
			t.Fatalf("unexpected: %+v", got)
		}
		if got[1].CreatedAt != 0 {
			t.Fatalf("optional createdAt must decode to 0 when absent, got %+v", got[1])
		}
	})

	t.Run("CreateProject returns full state incl currentProject", func(t *testing.T) {
		srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"projects":[{"id":"p9","name":"New"}],"currentProject":"p9"}`))
		})
		_ = srv
		cl, _ := NewClient(probe, salt)
		got, err := cl.CreateProject(context.Background(), "New")
		if err != nil {
			t.Fatalf("CreateProject: %v", err)
		}
		if got.CurrentProject != "p9" || len(got.Projects) != 1 || got.Projects[0].ID != "p9" {
			t.Fatalf("unexpected: %+v", got)
		}
	})

	t.Run("ListTags decodes entries + drops filter", func(t *testing.T) {
		srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tags":[{"id":"t1","name":"Prod","color":"5"},{"id":"t2","name":"Dev"}],"filter":{"selectedTagIds":["t1"]}}`))
		})
		_ = srv
		cl, _ := NewClient(probe, salt)
		got, err := cl.ListTags(context.Background())
		if err != nil {
			t.Fatalf("ListTags: %v", err)
		}
		if len(got) != 2 || got[0].ID != "t1" || got[0].Color != "5" {
			t.Fatalf("unexpected: %+v", got)
		}
		if got[1].Color != "" {
			t.Fatalf("optional color must decode empty when absent, got %+v", got[1])
		}
	})

	t.Run("DeleteTag returns full state incl filter", func(t *testing.T) {
		srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"tags":[],"filter":{"selectedTagIds":["t2"]}}`))
		})
		_ = srv
		cl, _ := NewClient(probe, salt)
		got, err := cl.DeleteTag(context.Background(), "t1")
		if err != nil {
			t.Fatalf("DeleteTag: %v", err)
		}
		if got.Filter == nil || len(got.Filter.SelectedTagIds) != 1 || got.Filter.SelectedTagIds[0] != "t2" {
			t.Fatalf("unexpected: %+v", got)
		}
	})
}

func TestClient_UpdateDbms_OnlySendsSuppliedFields(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	cases := []struct {
		name     string
		args     DbmsUpdateArgs
		wantBody map[string]any
	}{
		{
			name:     "NameOnly",
			args:     DbmsUpdateArgs{Name: strPtr("renamed")},
			wantBody: map[string]any{"name": "renamed"},
		},
		{
			name: "AllFields",
			args: DbmsUpdateArgs{
				Name:        strPtr("n"),
				Description: strPtr("d"),
				Tags:        &[]string{"t1", "t2"},
				Projects:    &[]string{"p1"},
			},
			wantBody: map[string]any{
				"name":        "n",
				"description": "d",
				"tags":        []any{"t1", "t2"},
				"projects":    []any{"p1"},
			},
		},
		{
			name:     "EmptySliceClearsTags",
			args:     DbmsUpdateArgs{Tags: &[]string{}},
			wantBody: map[string]any{"tags": []any{}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seen recordedRequest
			srv, probe := newAuthedServer(t, salt, clientID,
				projectTagHandler(t, http.StatusOK, `{"id":"abc","name":"n"}`, &seen))
			_ = srv

			cl, _ := NewClient(probe, salt)
			out, err := cl.UpdateDbms(context.Background(), "abc", tc.args)
			if err != nil {
				t.Fatalf("UpdateDbms: %v", err)
			}
			if out.ID != "abc" {
				t.Fatalf("unexpected: %+v", out)
			}
			if seen.method != http.MethodPatch || seen.path != "/fastify/api/dbmss/abc" {
				t.Fatalf("got %s %s, want PATCH /fastify/api/dbmss/abc", seen.method, seen.path)
			}
			if len(seen.body) != len(tc.wantBody) {
				t.Fatalf("body = %+v, want exactly %+v", seen.body, tc.wantBody)
			}
			for k, v := range tc.wantBody {
				if fmt.Sprintf("%v", seen.body[k]) != fmt.Sprintf("%v", v) {
					t.Errorf("body[%q] = %v, want %v", k, seen.body[k], v)
				}
			}
		})
	}
}

func TestClient_UpdateConnection_SendsTagsAndProjects(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var seen recordedRequest
	srv, probe := newAuthedServer(t, salt, clientID,
		projectTagHandler(t, http.StatusOK, `{"id":"abc","name":"X","connectionUri":"neo4j://localhost:7687"}`, &seen))
	_ = srv

	cl, _ := NewClient(probe, salt)
	_, err := cl.UpdateConnection(context.Background(), "abc", ConnectionUpdateArgs{
		Tags:     &[]string{"t1"},
		Projects: &[]string{},
	})
	if err != nil {
		t.Fatalf("UpdateConnection: %v", err)
	}
	if seen.method != http.MethodPatch || seen.path != "/fastify/api/connections/abc" {
		t.Fatalf("got %s %s, want PATCH /fastify/api/connections/abc", seen.method, seen.path)
	}
	if len(seen.body) != 2 {
		t.Fatalf("body = %+v, want exactly tags+projects", seen.body)
	}
	tags, ok := seen.body["tags"].([]any)
	if !ok || len(tags) != 1 || tags[0] != "t1" {
		t.Errorf("body[tags] = %v, want [t1]", seen.body["tags"])
	}
	projects, ok := seen.body["projects"].([]any)
	if !ok || len(projects) != 0 {
		t.Errorf("body[projects] = %v, want [] (non-nil empty slice must serialize, not be dropped)", seen.body["projects"])
	}
}

// ---- Resolvers ----

const (
	resolverUUID1 = "11111111-2222-3333-4444-555555555555"
	resolverUUID2 = "66666666-7777-8888-9999-aaaaaaaaaaaa"
)

func TestClient_ResolveProjectIDs(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	catalog := `{"projects":[{"id":"` + resolverUUID1 + `","name":"Prod"},{"id":"` + resolverUUID2 + `","name":"Prod"},{"id":"p3","name":"Staging"}]}`

	cases := []struct {
		name string
		// catalogBody, when non-empty, is served for GET /projects; when
		// empty the server fails the test on ANY request (proves the
		// all-UUID path never hits the network).
		catalogBody string
		input       []string
		want        []string
		wantErr     string // substring; empty means success
	}{
		{
			name:  "UUID passes through verbatim without catalog fetch",
			input: []string{resolverUUID1},
			want:  []string{resolverUUID1},
		},
		{
			name:        "Name resolves to ID",
			catalogBody: catalog,
			input:       []string{"Staging"},
			want:        []string{"p3"},
		},
		{
			name:        "Mixed UUID and name",
			catalogBody: catalog,
			input:       []string{resolverUUID1, "Staging"},
			want:        []string{resolverUUID1, "p3"},
		},
		{
			name:        "Unknown name is a usage error",
			catalogBody: catalog,
			input:       []string{"Nope"},
			wantErr:     `unknown project "Nope"`,
		},
		{
			name:        "Ambiguous name is a usage error listing candidate IDs",
			catalogBody: catalog,
			input:       []string{"Prod"},
			wantErr:     `ambiguous project name "Prod"`,
		},
		{
			name:        "Name match is case-sensitive",
			catalogBody: catalog,
			input:       []string{"staging"},
			wantErr:     `unknown project "staging"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
				if tc.catalogBody == "" {
					t.Errorf("unexpected request %s %s — all-UUID input must not fetch the catalog", r.Method, r.URL.Path)
					return
				}
				if r.Method != http.MethodGet || r.URL.Path != "/fastify/api/projects" {
					t.Errorf("got %s %s, want GET /fastify/api/projects", r.Method, r.URL.Path)
				}
				_, _ = w.Write([]byte(tc.catalogBody))
			})
			_ = srv

			cl, _ := NewClient(probe, salt)
			got, err := cl.ResolveProjectIDs(context.Background(), tc.input)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ResolveProjectIDs: %v", err)
				}
				if strings.Join(got, ",") != strings.Join(tc.want, ",") {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil (result %v)", tc.wantErr, got)
			}
			var ce *clierr.CLIError
			if !errors.As(err, &ce) || ce.Code != 2 {
				t.Fatalf("error = %v (%T), want clierr usage error (exit 2)", err, err)
			}
			if !strings.Contains(ce.Message, tc.wantErr) {
				t.Fatalf("message = %q, want substring %q", ce.Message, tc.wantErr)
			}
		})
	}
}

func TestClient_ResolveProjectIDs_AmbiguousErrorListsIDs(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"projects":[{"id":"` + resolverUUID1 + `","name":"Prod"},{"id":"` + resolverUUID2 + `","name":"Prod"}]}`))
	})
	_ = srv

	cl, _ := NewClient(probe, salt)
	_, err := cl.ResolveProjectIDs(context.Background(), []string{"Prod"})
	if err == nil {
		t.Fatalf("expected ambiguous error")
	}
	if !strings.Contains(err.Error(), resolverUUID1) || !strings.Contains(err.Error(), resolverUUID2) {
		t.Fatalf("ambiguous error must list candidate IDs, got: %v", err)
	}
}

func TestClient_ResolveTagIDs(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	catalog := `{"tags":[{"id":"` + resolverUUID1 + `","name":"Prod","color":"5"},{"id":"` + resolverUUID2 + `","name":"Prod"},{"id":"t3","name":"Dev"}]}`

	cases := []struct {
		name        string
		catalogBody string
		input       []string
		want        []string
		wantErr     string
	}{
		{
			name:  "UUID passes through verbatim without catalog fetch",
			input: []string{resolverUUID2},
			want:  []string{resolverUUID2},
		},
		{
			name:        "Name resolves to ID",
			catalogBody: catalog,
			input:       []string{"Dev"},
			want:        []string{"t3"},
		},
		{
			name:        "Unknown name is a usage error",
			catalogBody: catalog,
			input:       []string{"Nope"},
			wantErr:     `unknown tag "Nope"`,
		},
		{
			name:        "Ambiguous name is a usage error",
			catalogBody: catalog,
			input:       []string{"Prod"},
			wantErr:     `ambiguous tag name "Prod"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
				if tc.catalogBody == "" {
					t.Errorf("unexpected request %s %s — all-UUID input must not fetch the catalog", r.Method, r.URL.Path)
					return
				}
				if r.Method != http.MethodGet || r.URL.Path != "/fastify/api/tags" {
					t.Errorf("got %s %s, want GET /fastify/api/tags", r.Method, r.URL.Path)
				}
				_, _ = w.Write([]byte(tc.catalogBody))
			})
			_ = srv

			cl, _ := NewClient(probe, salt)
			got, err := cl.ResolveTagIDs(context.Background(), tc.input)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ResolveTagIDs: %v", err)
				}
				if strings.Join(got, ",") != strings.Join(tc.want, ",") {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil (result %v)", tc.wantErr, got)
			}
			var ce *clierr.CLIError
			if !errors.As(err, &ce) || ce.Code != 2 {
				t.Fatalf("error = %v (%T), want clierr usage error (exit 2)", err, err)
			}
			if !strings.Contains(ce.Message, tc.wantErr) {
				t.Fatalf("message = %q, want substring %q", ce.Message, tc.wantErr)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

func TestNonEmptyStrings(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "nil", in: nil, want: []string{}},
		{name: "empty slice", in: []string{}, want: []string{}},
		{name: "all empty", in: []string{"", ""}, want: []string{}},
		{name: "drops empties", in: []string{"a", "", "b"}, want: []string{"a", "b"}},
		{name: "keeps all", in: []string{"a", "b"}, want: []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NonEmptyStrings(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("NonEmptyStrings(%q) = %q, want %q", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("NonEmptyStrings(%q) = %q, want %q", tc.in, got, tc.want)
				}
			}
		})
	}
}

// TestClient_ProjectTagEndpoints_UnsupportedDesktopVersion covers Desktop
// versions predating the projects/tags routes: they answer unknown
// /fastify/api/* routes with the SPA fallback (HTTP 200, text/html,
// index.html body). Every catalog method — and the name→ID resolvers, which
// fetch the catalog before any PATCH — must surface the friendly upgrade
// message, never a raw decode error or leaked HTML.
func TestClient_ProjectTagEndpoints_UnsupportedDesktopVersion(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	const spaFallback = "<!DOCTYPE html>\n<html><head><title>Neo4j Desktop</title></head><body><div id=\"root\"></div></body></html>"

	cases := []struct {
		name string
		call func(cl *Client) error
	}{
		{name: "ListProjects", call: func(cl *Client) error { _, err := cl.ListProjects(context.Background()); return err }},
		{name: "CreateProject", call: func(cl *Client) error { _, err := cl.CreateProject(context.Background(), "P"); return err }},
		{name: "UpdateProject", call: func(cl *Client) error { _, err := cl.UpdateProject(context.Background(), "p1", "P"); return err }},
		{name: "DeleteProject", call: func(cl *Client) error { _, err := cl.DeleteProject(context.Background(), "p1"); return err }},
		{name: "ListTags", call: func(cl *Client) error { _, err := cl.ListTags(context.Background()); return err }},
		{name: "CreateTag", call: func(cl *Client) error { _, err := cl.CreateTag(context.Background(), "T", ""); return err }},
		{name: "UpdateTag", call: func(cl *Client) error { _, err := cl.UpdateTag(context.Background(), "t1", "T", ""); return err }},
		{name: "DeleteTag", call: func(cl *Client) error { _, err := cl.DeleteTag(context.Background(), "t1"); return err }},
		{
			name: "ResolveProjectIDs inherits the error",
			call: func(cl *Client) error {
				_, err := cl.ResolveProjectIDs(context.Background(), []string{"Prod"})
				return err
			},
		},
		{
			name: "ResolveTagIDs inherits the error",
			call: func(cl *Client) error { _, err := cl.ResolveTagIDs(context.Background(), []string{"Prod"}); return err },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(spaFallback))
			})
			_ = srv

			cl, err := NewClient(probe, salt)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			err = tc.call(cl)
			if err == nil {
				t.Fatalf("expected unsupported-version error, got nil")
			}
			var ce *clierr.CLIError
			if !errors.As(err, &ce) || ce.Code != 1 {
				t.Fatalf("error = %v (%T), want clierr fatal error (exit 1)", err, err)
			}
			if !strings.Contains(ce.Message, "does not support projects and tags") ||
				!strings.Contains(ce.Message, "Upgrade Neo4j Desktop 2 to the latest version") {
				t.Fatalf("message = %q, want friendly upgrade message", ce.Message)
			}
			if strings.Contains(ce.Message, "<") {
				t.Fatalf("message must not leak response HTML, got %q", ce.Message)
			}
			if strings.Contains(ce.Message, "invalid character") {
				t.Fatalf("message must not leak the raw decode error, got %q", ce.Message)
			}
		})
	}
}

// TestClient_ProjectTagEndpoints_CorruptJSONKeepsDecodeError pins the decode
// policy: only bodies that look like markup (trimmed body starts with '<')
// get the friendly upgrade message; other undecodable bodies keep the
// detailed decode error, since they indicate a genuine protocol break worth
// reporting rather than a missing route.
func TestClient_ProjectTagEndpoints_CorruptJSONKeepsDecodeError(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	cases := []struct {
		name string
		body string
	}{
		{name: "truncated JSON", body: `{"projects": [`},
		{name: "plain text", body: `not json at all`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			})
			_ = srv

			cl, err := NewClient(probe, salt)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			_, err = cl.ListProjects(context.Background())
			if err == nil {
				t.Fatalf("expected decode error, got nil")
			}
			var ce *clierr.CLIError
			if !errors.As(err, &ce) || ce.Code != 1 {
				t.Fatalf("error = %v (%T), want clierr fatal error (exit 1)", err, err)
			}
			if !strings.Contains(ce.Message, "desktop: failed to decode project list") {
				t.Fatalf("message = %q, want the detailed decode error", ce.Message)
			}
			if strings.Contains(ce.Message, "does not support projects and tags") {
				t.Fatalf("non-markup body must not get the upgrade message, got %q", ce.Message)
			}
		})
	}
}
