// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/neo4j/cli/common/clierr"
)

// Tag relate stamps on every database-upload task, alongside the source DBMS
// id, so WaitForUploadTask can locate the right task among concurrent work.
const uploadTaskTag = "db:upload"

// uploadKickTimeout caps the `POST .../databases/upload` request itself — this
// only enqueues the async task; the long-running dump/upload work is tracked
// via the tasks list, so the POST should return quickly.
const uploadKickTimeout = 2 * time.Minute

// uploadTaskPollInterval is the gap between consecutive `GET /tasks` polls
// while waiting for a db:upload task to settle.
var uploadTaskPollInterval = 2 * time.Second

// uploadTaskTimeout bounds the total wait for a db:upload task to settle. A
// full dump + restore of a non-trivial database can take many minutes, so the
// budget is deliberately generous.
const uploadTaskTimeout = 30 * time.Minute

// UploadDatabase enqueues a database upload from a local Desktop DBMS to a
// remote (Aura) target via `POST /dbmss/:id/databases/upload`. The call is
// asynchronous on the relate side — it returns once the task is accepted;
// callers poll WaitForUploadTask for completion.
func (c *Client) UploadDatabase(ctx context.Context, dbmsID string, source UploadSource, target UploadTarget) error {
	payload := map[string]any{
		"source": source,
		"target": target,
	}
	_, err := c.doWithTimeout(ctx, http.MethodPost,
		"/dbmss/"+url.PathEscape(dbmsID)+"/databases/upload", payload, uploadKickTimeout)
	return err
}

// ListTasks returns the relate task list (`GET /tasks`). Each task carries an
// id, tags, and a settle-state status; long-running operations such as
// database uploads are surfaced here.
func (c *Client) ListTasks(ctx context.Context) ([]Task, error) {
	body, err := c.do(ctx, http.MethodGet, "/tasks", nil)
	if err != nil {
		return nil, err
	}
	out := []Task{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, clierr.NewFatalError("desktop: failed to decode task list: %s", err.Error())
	}
	return out, nil
}

// taskLister is the subset of *Client WaitForUploadTask depends on, kept as an
// interface so tests can drive the polling loop without a live server.
type taskLister interface {
	ListTasks(ctx context.Context) ([]Task, error)
}

// WaitForUploadTask polls the task list until the db:upload task for dbmsID
// settles, returning nil on success and a fatal error on failure or timeout. A
// task matches when its tags contain BOTH uploadTaskTag and dbmsID. Until such
// a task appears the loop keeps polling — relate may not register it on the
// very first tick after the upload POST returns.
func WaitForUploadTask(ctx context.Context, client taskLister, dbmsID string) error {
	deadline := time.Now().Add(uploadTaskTimeout)
	pollCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	if done, err := uploadTaskSettled(pollCtx, client, dbmsID); done || err != nil {
		return err
	}

	ticker := time.NewTicker(uploadTaskPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return clierr.NewFatalError(
				"Neo4j Desktop 2 database upload did not complete within %s. The upload may still be running in Desktop — check Desktop's UI before retrying.",
				uploadTaskTimeout)
		case <-ticker.C:
			if done, err := uploadTaskSettled(pollCtx, client, dbmsID); done || err != nil {
				return err
			}
		}
	}
}

// uploadTaskSettled performs one poll: returns (true, nil) on success,
// (true, err) on a failed task, and (false, nil) while still in flight or not
// yet registered. A ListTasks transport error is treated as transient
// (false, nil) so a momentary blip doesn't abort an otherwise-healthy upload.
func uploadTaskSettled(ctx context.Context, client taskLister, dbmsID string) (bool, error) {
	tasks, err := client.ListTasks(ctx)
	if err != nil {
		return false, nil
	}
	for _, task := range tasks {
		if !taskHasTags(task, uploadTaskTag, dbmsID) {
			continue
		}
		switch {
		case task.Status.IsSuccess:
			return true, nil
		case task.Status.IsError:
			return true, clierr.NewFatalError(
				"Neo4j Desktop 2 reported the database upload failed (task %s). Check Desktop's UI for details.",
				task.ID)
		}
	}
	return false, nil
}

func taskHasTags(task Task, want ...string) bool {
	for _, w := range want {
		found := false
		for _, tag := range task.Tags {
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
