// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDeployDesktop_UploadAndTasks_Succeeds drives the exact wire sequence the
// `aura instance deploy --from-desktop` path runs against Desktop's relate API
// (deployViaDesktop in the aura instance subtree): stop the source DBMS for a
// consistent dump, poll it to "stopped", POST the database upload, then poll
// GET /tasks until the db:upload task settles, and finally restart the DBMS.
// It asserts the fixture records the upload source/target body and transitions
// the task to isSuccess so the production WaitForUploadTask loop converges.
func TestDeployDesktop_UploadAndTasks_Succeeds(t *testing.T) {
	srv, st, signFn := newFixtureServer(t)
	const id = "deadbeef-0000-0000-0000-000000000001"

	adminDo(t, srv, http.MethodPost, "/_scenario/dbms", map[string]any{
		"id": id, "name": "source", "status": "started",
		"connectionUri": "neo4j://127.0.0.1:7687",
	}).Body.Close() //nolint:errcheck
	// autoProgress flips the source DBMS stopping->stopped on the deploy
	// path's poll-to-stopped step.
	adminDo(t, srv, http.MethodPost, "/_scenario/auto_progress", map[string]any{"enabled": true}).Body.Close() //nolint:errcheck

	// Stop the DBMS (deploy stops a running source for a clean dump).
	resp := authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/desktop/dbmss/"+id+"/stop", nil)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stop status = %d", resp.StatusCode)
	}

	// Poll until "stopped".
	if got := pollStatus(t, srv, signFn, id); got != "stopped" {
		t.Fatalf("dbms never reached stopped (last %q)", got)
	}

	// POST the database upload (source neo4j -> Aura target with overwrite).
	const (
		wantURI  = "neo4j+s://abc123.databases.neo4j.io"
		wantUser = "neo4j"
		wantPass = "super-secret-pass"
	)
	resp = authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/dbmss/"+id+"/databases/upload", map[string]any{
		"source": map[string]any{"databaseName": "neo4j"},
		"target": map[string]any{
			"uri": wantURI, "username": wantUser, "password": wantPass, "overwrite": true,
		},
	})
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d", resp.StatusCode)
	}

	// The fixture must have recorded the upload request shape verbatim.
	if len(st.uploads) != 1 {
		t.Fatalf("want 1 recorded upload, got %d", len(st.uploads))
	}
	rec := st.uploads[0]
	if rec.DbmsID != id || rec.SourceDatabase != "neo4j" || rec.TargetURI != wantURI ||
		rec.TargetUsername != wantUser || rec.TargetPassword != wantPass || !rec.Overwrite {
		t.Fatalf("recorded upload mismatch: %+v", rec)
	}

	// Poll GET /tasks until the db:upload task for this id settles to success.
	settled := false
	for i := 0; i < 5; i++ {
		tasks := getTasks(t, srv, signFn)
		for _, task := range tasks {
			if hasTags(task.Tags, "db:upload", id) && task.Status.IsSuccess {
				settled = true
			}
			if hasTags(task.Tags, "db:upload", id) && task.Status.IsError {
				t.Fatalf("upload task reported error on a success scenario")
			}
		}
		if settled {
			break
		}
	}
	if !settled {
		t.Fatalf("db:upload task never reached isSuccess")
	}

	// Restore the source DBMS to its prior running state.
	resp = authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/dbmss/"+id+"/start", nil)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("start status = %d", resp.StatusCode)
	}
}

// TestDeployDesktop_UploadTask_ReportsError covers the deploy sad-path: when
// the scenario arms upload_fail, the db:upload task settles to isError, which
// the production WaitForUploadTask surfaces as a failed deploy.
func TestDeployDesktop_UploadTask_ReportsError(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	const id = "deadbeef-0000-0000-0000-000000000002"
	adminDo(t, srv, http.MethodPost, "/_scenario/dbms", map[string]any{
		"id": id, "name": "source", "status": "stopped",
	}).Body.Close() //nolint:errcheck
	adminDo(t, srv, http.MethodPost, "/_scenario/upload_fail", map[string]any{"enabled": true}).Body.Close() //nolint:errcheck

	resp := authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/dbmss/"+id+"/databases/upload", map[string]any{
		"source": map[string]any{"databaseName": "neo4j"},
		"target": map[string]any{"uri": "neo4j+s://x.databases.neo4j.io", "username": "neo4j", "password": "p", "overwrite": true},
	})
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d", resp.StatusCode)
	}

	tasks := getTasks(t, srv, signFn)
	if len(tasks) != 1 {
		t.Fatalf("want 1 task, got %d", len(tasks))
	}
	if !tasks[0].Status.IsError {
		t.Fatalf("want db:upload task isError under upload_fail; got %+v", tasks[0].Status)
	}
}

// TestDeployDesktop_Upload_UnknownDbms_404 asserts the upload route 404s for an
// unknown DBMS so the production client surfaces the not-found error.
func TestDeployDesktop_Upload_UnknownDbms_404(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	resp := authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/dbmss/nope/databases/upload", map[string]any{
		"source": map[string]any{"databaseName": "neo4j"},
		"target": map[string]any{"uri": "neo4j+s://x.databases.neo4j.io", "username": "neo4j", "password": "p", "overwrite": true},
	})
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown dbms", resp.StatusCode)
	}
}

// pollStatus reads GET /dbmss/:id up to a few times and returns the last status
// seen (autoProgress advances stopping->stopped on each read).
func pollStatus(t *testing.T, srv *httptest.Server, signFn func(string) string, id string) string {
	t.Helper()
	var last string
	for i := 0; i < 5; i++ {
		resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/"+id, nil)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck
		var d struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(body, &d)
		last = d.Status
		if last == "stopped" {
			return last
		}
	}
	return last
}

// getTasks reads GET /tasks and decodes the task list.
func getTasks(t *testing.T, srv *httptest.Server, signFn func(string) string) []uploadTask {
	t.Helper()
	resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/tasks", nil)
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	var out []uploadTask
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode tasks: %v\n%s", err, body)
	}
	return out
}

func hasTags(tags []string, want ...string) bool {
	for _, w := range want {
		found := false
		for _, tag := range tags {
			if tag == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
