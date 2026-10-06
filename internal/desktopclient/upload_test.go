// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestClient_UploadDatabase_SendsBodyAndAuthHeaders(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var seenBody map[string]any
	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/fastify/api/dbmss/dbms-1/databases/upload" {
			t.Errorf("got %s %s, want POST /fastify/api/dbmss/dbms-1/databases/upload", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	_ = srv

	cl, err := NewClient(probe, salt)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	err = cl.UploadDatabase(context.Background(), "dbms-1",
		UploadSource{DatabaseName: "neo4j"},
		UploadTarget{
			URI:       "neo4j+s://abc.databases.neo4j.io",
			Username:  "neo4j",
			Password:  "aura-secret",
			Overwrite: true,
		})
	if err != nil {
		t.Fatalf("UploadDatabase: %v", err)
	}

	source, ok := seenBody["source"].(map[string]any)
	if !ok {
		t.Fatalf("body.source missing or wrong type: %+v", seenBody)
	}
	if source["databaseName"] != "neo4j" {
		t.Errorf("source.databaseName = %v, want neo4j", source["databaseName"])
	}
	target, ok := seenBody["target"].(map[string]any)
	if !ok {
		t.Fatalf("body.target missing or wrong type: %+v", seenBody)
	}
	if target["uri"] != "neo4j+s://abc.databases.neo4j.io" {
		t.Errorf("target.uri = %v", target["uri"])
	}
	if target["username"] != "neo4j" {
		t.Errorf("target.username = %v", target["username"])
	}
	if target["password"] != "aura-secret" {
		t.Errorf("target.password = %v", target["password"])
	}
	if target["overwrite"] != true {
		t.Errorf("target.overwrite = %v, want true", target["overwrite"])
	}
}

func TestClient_LoadDump_SendsBodyAndAuthHeaders(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	var seenBody map[string]any
	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/fastify/api/dbmss/dbms-1/databases/neo4j/load-dump" {
			t.Errorf("got %s %s, want POST /fastify/api/dbmss/dbms-1/databases/neo4j/load-dump", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	_ = srv

	cl, err := NewClient(probe, salt)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := cl.LoadDump(context.Background(), "dbms-1", "neo4j", "/tmp/movies.dump", true); err != nil {
		t.Fatalf("LoadDump: %v", err)
	}

	if seenBody["sourceFilePath"] != "/tmp/movies.dump" {
		t.Errorf("sourceFilePath = %v, want /tmp/movies.dump", seenBody["sourceFilePath"])
	}
	if seenBody["overwrite"] != true {
		t.Errorf("overwrite = %v, want true", seenBody["overwrite"])
	}
}

func TestClient_LoadDump_PropagatesServerError(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"dbms must be stopped"}`))
	})
	_ = srv

	cl, err := NewClient(probe, salt)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := cl.LoadDump(context.Background(), "dbms-1", "neo4j", "/tmp/movies.dump", false); err == nil {
		t.Fatalf("expected error on 500 response, got nil")
	}
}

func TestClient_ListTasks_DecodesTasks(t *testing.T) {
	const salt, clientID = "salt", "cid"
	pinClientSeams(t, clientID, time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC))

	srv, probe := newAuthedServer(t, salt, clientID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/fastify/api/tasks" {
			t.Errorf("got %s %s, want GET /fastify/api/tasks", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"t1","tags":["db:upload","dbms-1"],"status":{"isLoading":false,"isSuccess":true,"isError":false}}]`))
	})
	_ = srv

	cl, err := NewClient(probe, salt)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	got, err := cl.ListTasks(context.Background())
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(got) != 1 || got[0].ID != "t1" || !got[0].Status.IsSuccess {
		t.Fatalf("unexpected tasks: %+v", got)
	}
	if len(got[0].Tags) != 2 || got[0].Tags[0] != "db:upload" {
		t.Fatalf("unexpected tags: %+v", got[0].Tags)
	}
}

// fakeTaskLister serves a scripted sequence of ListTasks responses so the wait
// loop can be exercised deterministically without a live server.
type fakeTaskLister struct {
	responses [][]Task
	calls     int
}

func (f *fakeTaskLister) ListTasks(_ context.Context) ([]Task, error) {
	idx := f.calls
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	f.calls++
	return f.responses[idx], nil
}

func TestWaitForUploadTask(t *testing.T) {
	// Shrink the poll interval so the multi-tick cases run instantly.
	prev := uploadTaskPollInterval
	uploadTaskPollInterval = time.Millisecond
	t.Cleanup(func() { uploadTaskPollInterval = prev })

	loading := []Task{{ID: "t1", Tags: []string{"db:upload", "dbms-1"}, Status: TaskStatus{IsLoading: true}}}
	success := []Task{{ID: "t1", Tags: []string{"db:upload", "dbms-1"}, Status: TaskStatus{IsSuccess: true}}}
	failure := []Task{{ID: "t1", Tags: []string{"db:upload", "dbms-1"}, Status: TaskStatus{IsError: true}}}
	otherDbms := []Task{{ID: "t9", Tags: []string{"db:upload", "dbms-other"}, Status: TaskStatus{IsSuccess: true}}}

	tests := []struct {
		name      string
		responses [][]Task
		wantErr   bool
	}{
		{name: "immediate success", responses: [][]Task{success}, wantErr: false},
		{name: "loading then success", responses: [][]Task{loading, loading, success}, wantErr: false},
		{name: "immediate error", responses: [][]Task{failure}, wantErr: true},
		{name: "not registered then success", responses: [][]Task{{}, success}, wantErr: false},
		{name: "ignores other dbms then success", responses: [][]Task{otherDbms, success}, wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeTaskLister{responses: tc.responses}
			err := WaitForUploadTask(context.Background(), fake, "dbms-1")
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWaitForUploadTask_ContextCancelled(t *testing.T) {
	prev := uploadTaskPollInterval
	uploadTaskPollInterval = time.Millisecond
	t.Cleanup(func() { uploadTaskPollInterval = prev })

	// Always loading — never settles, so cancellation drives termination.
	fake := &fakeTaskLister{responses: [][]Task{{
		{ID: "t1", Tags: []string{"db:upload", "dbms-1"}, Status: TaskStatus{IsLoading: true}},
	}}}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := WaitForUploadTask(ctx, fake, "dbms-1")
	if err == nil {
		t.Fatalf("expected error when context is cancelled before task settles")
	}
}
