// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// newFixtureServer spins up an httptest.Server wired to the fixture's mux
// + state. Returns the server, the state pointer (so tests can poke
// scenario state directly without going through the admin endpoints when
// the admin endpoints aren't the SUT), and a helper that returns a fresh
// JWT signed with the same composite key the production client would use.
func newFixtureServer(t *testing.T) (*httptest.Server, *state, func(clientID string) string) {
	t.Helper()
	st := newState("testsalt")
	srv := httptest.NewServer(newMux(st))
	st.setOrigin(srv.URL)
	t.Cleanup(srv.Close)
	signFn := func(clientID string) string {
		key := fmt.Sprintf("%s-%s-%s", "testsalt", srv.URL, clientID)
		claims := jwt.MapClaims{
			"iat": time.Now().Unix(),
			"exp": time.Now().Add(time.Hour).Unix(),
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := tok.SignedString([]byte(key))
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}
		return signed
	}
	return srv, st, signFn
}

// authedDo issues an authed request against the fixture, signing the token
// with the documented composite key.
func authedDo(t *testing.T, srv *httptest.Server, signFn func(string) string, method, path string, body any) *http.Response {
	t.Helper()
	const clientID = "fixture-test-client"
	var reqBody io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, srv.URL+path, reqBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Client-Id", clientID)
	req.Header.Set("X-API-Token", signFn(clientID))
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// adminDo issues an unauthenticated request against an admin endpoint.
func adminDo(t *testing.T, srv *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, srv.URL+path, reqBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func TestProbe_NoAuth(t *testing.T) {
	srv, _, _ := newFixtureServer(t)
	resp, err := srv.Client().Get(srv.URL + "/fastify/api-docs")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe status = %d, want 200", resp.StatusCode)
	}
}

func TestAuth_MissingHeaders_Rejected(t *testing.T) {
	srv, _, _ := newFixtureServer(t)
	resp, err := srv.Client().Get(srv.URL + "/fastify/api/dbmss")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAuth_WrongSalt_Rejected(t *testing.T) {
	srv, _, _ := newFixtureServer(t)
	const clientID = "x"
	wrongKey := fmt.Sprintf("%s-%s-%s", "wrongsalt", srv.URL, clientID)
	claims := jwt.MapClaims{"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString([]byte(wrongKey))

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/fastify/api/dbmss", nil)
	req.Header.Set("X-Client-Id", clientID)
	req.Header.Set("X-API-Token", signed)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAuth_ValidToken_Accepted(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss", nil)
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestDbmssList_LightweightShape_NoStatus(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	// Seed via admin.
	resp := adminDo(t, srv, http.MethodPost, "/_scenario/dbms", map[string]any{
		"id":   "11111111-1111-1111-1111-111111111111",
		"name": "alpha", "status": "started",
		"connectionUri": "neo4j://127.0.0.1:7687",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("seed status = %d", resp.StatusCode)
	}
	resp.Body.Close() //nolint:errcheck

	resp = authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss", nil)
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	// The lightweight shape MUST NOT carry a status field — task-009
	// regression gate.
	if strings.Contains(string(body), "\"status\"") {
		t.Fatalf("/dbmss carries status field — should be absent. body=%s", body)
	}
}

func TestDbmssInfo_FullShape_WithStatus(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	adminDo(t, srv, http.MethodPost, "/_scenario/dbms", map[string]any{
		"id":   "22222222-2222-2222-2222-222222222222",
		"name": "beta", "status": "started",
		"connectionUri": "neo4j://127.0.0.1:7687",
	}).Body.Close() //nolint:errcheck

	resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/info", nil)
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "\"status\":\"started\"") {
		t.Fatalf("/dbmss/info must carry status=started; body=%s", body)
	}
}

func TestDbmsTransition_FlipsStatusAfterPolls(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	const id = "33333333-3333-3333-3333-333333333333"
	adminDo(t, srv, http.MethodPost, "/_scenario/dbms", map[string]any{
		"id": id, "name": "gamma", "status": "starting",
	}).Body.Close() //nolint:errcheck
	adminDo(t, srv, http.MethodPost, "/_scenario/transition", map[string]any{
		"id": id, "to_status": "started", "after_calls": 2,
	}).Body.Close() //nolint:errcheck

	// First two reads still see "starting".
	for i := 0; i < 2; i++ {
		resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/"+id, nil)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck
		if !strings.Contains(string(body), "\"status\":\"starting\"") {
			t.Fatalf("read %d: expected starting, got %s", i, body)
		}
	}
	// Third read flips to "started".
	resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/"+id, nil)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if !strings.Contains(string(body), "\"status\":\"started\"") {
		t.Fatalf("expected started after 2 polls, got %s", body)
	}
}

func TestAuthMode_Reject(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	adminDo(t, srv, http.MethodPost, "/_scenario/auth_mode", map[string]any{"mode": "reject"}).Body.Close() //nolint:errcheck
	resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss", nil)
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 under reject mode", resp.StatusCode)
	}
}

func TestAuthMode_500(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	adminDo(t, srv, http.MethodPost, "/_scenario/auth_mode", map[string]any{"mode": "500"}).Body.Close() //nolint:errcheck
	resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss", nil)
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
}

func TestConnection_Create_Get_Update_Delete(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)

	// Create.
	resp := authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/connections", map[string]any{
		"name": "Aura prod", "connectionUri": "neo4j+s://x.databases.neo4j.io",
		"username": "neo4j", "password": "p",
	})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create status=%d body=%s", resp.StatusCode, body)
	}
	var created connection
	_ = json.Unmarshal(body, &created)
	if created.ID == "" {
		t.Fatalf("created connection without id: %s", body)
	}

	// List.
	resp = authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/connections", nil)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if !strings.Contains(string(body), created.ID) {
		t.Fatalf("list missing created id; body=%s", body)
	}

	// PATCH (partial — only description).
	resp = authedDo(t, srv, signFn, http.MethodPatch, "/fastify/api/connections/"+created.ID, map[string]any{
		"description": "updated",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if !strings.Contains(string(body), "\"description\":\"updated\"") {
		t.Fatalf("patch did not update description; body=%s", body)
	}

	// GET credentials by key.
	resp = authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/credentials/connection:"+created.ID, nil)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if !strings.Contains(string(body), "\"password\":\"p\"") {
		t.Fatalf("creds not stored under connection:<id>; body=%s", body)
	}

	// Delete.
	resp = authedDo(t, srv, signFn, http.MethodDelete, "/fastify/api/connections/"+created.ID, nil)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete status=%d", resp.StatusCode)
	}
	resp = authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/connections", nil)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if strings.Contains(string(body), created.ID) {
		t.Fatalf("delete did not remove from list; body=%s", body)
	}
}

func TestCredentials_NullVsAbsent(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)

	// Null-creds entry (REQ-F-028 prompt-or-fail path).
	adminDo(t, srv, http.MethodPost, "/_scenario/credential", map[string]any{
		"key": "dbms:nullcreds", "value": nil,
	}).Body.Close() //nolint:errcheck
	resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/credentials/dbms:nullcreds", nil)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "null" {
		t.Fatalf("null creds path: status=%d body=%q (want 200 'null')", resp.StatusCode, body)
	}

	// Absent → 404.
	resp = authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/credentials/dbms:nope", nil)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("absent creds: status=%d, want 404", resp.StatusCode)
	}
}

func TestReset_ClearsAllState(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	adminDo(t, srv, http.MethodPost, "/_scenario/dbms", map[string]any{"name": "x"}).Body.Close()           //nolint:errcheck
	adminDo(t, srv, http.MethodPost, "/_scenario/connection", map[string]any{"name": "y"}).Body.Close()     //nolint:errcheck
	adminDo(t, srv, http.MethodPost, "/_scenario/auth_mode", map[string]any{"mode": "reject"}).Body.Close() //nolint:errcheck

	adminDo(t, srv, http.MethodPost, "/_scenario/reset", nil).Body.Close() //nolint:errcheck

	// Auth must be back to accept (otherwise the next call returns 401).
	resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss", nil)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("reset did not clear: status=%d body=%s", resp.StatusCode, body)
	}
	resp = authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/connections", nil)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("connections after reset: %s", body)
	}
}

// --- Plugin endpoint coverage ---

// seedDbmsWithPlugins primes the fixture with one DBMS plus its plugin
// catalogs. Used by every plugin-route test below so they read top-to-
// bottom without scenario-admin noise.
func seedDbmsWithPlugins(t *testing.T, srv *httptest.Server, id, status string, available, installed []map[string]any) {
	t.Helper()
	adminDo(t, srv, http.MethodPost, "/_scenario/dbms", map[string]any{
		"id": id, "name": "p-" + id, "status": status,
		"connectionUri": "neo4j://127.0.0.1:7687",
	}).Body.Close() //nolint:errcheck
	plug := map[string]any{"dbms_id": id}
	if available != nil {
		plug["available"] = available
	}
	if installed != nil {
		plug["installed"] = installed
	}
	resp := adminDo(t, srv, http.MethodPost, "/_scenario/plugin", plug)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("seed plugin: status=%d", resp.StatusCode)
	}
}

func TestPlugins_ListInstalled_Seeded(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	seedDbmsWithPlugins(t, srv, "p-01", "started", nil, []map[string]any{
		{"name": "apoc", "version": "5.20.0", "filePath": "/p/apoc.jar", "pendingRestart": false},
		{"name": "gds", "version": "2.6.0", "filePath": "/p/gds.jar", "pendingRestart": true},
	})

	resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/p-01/plugins/installed", nil)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"name":"apoc"`) || !strings.Contains(string(body), `"name":"gds"`) {
		t.Fatalf("installed list missing entries; body=%s", body)
	}
}

func TestPlugins_ListAvailable_Seeded(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	seedDbmsWithPlugins(t, srv, "p-02", "stopped", []map[string]any{
		{"name": "apoc", "version": "5.20.0", "filePath": "/p/apoc.jar"},
		{"name": "neo-semantics", "filePath": "/p/n10s.jar"},
	}, nil)

	resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/p-02/plugins/available", nil)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"name":"apoc"`) || !strings.Contains(string(body), `"name":"neo-semantics"`) {
		t.Fatalf("available list missing entries; body=%s", body)
	}
}

func TestPlugins_ListEmpty_NonNilArray(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	seedDbmsWithPlugins(t, srv, "p-03", "stopped", nil, nil)

	for _, suffix := range []string{"/plugins/installed", "/plugins/available"} {
		resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/p-03"+suffix, nil)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck
		if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "[]" {
			t.Fatalf("%s: status=%d body=%q (want 200 '[]')", suffix, resp.StatusCode, body)
		}
	}
}

func TestPlugins_InstallAddsToInstalled_PendingRestartMatchesStatus(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	seedDbmsWithPlugins(t, srv, "p-04", "started", []map[string]any{
		{"name": "apoc", "version": "5.20.0", "filePath": "/p/apoc.jar"},
	}, nil)

	resp := authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/dbmss/p-04/plugins/install",
		map[string]any{"pluginName": "apoc"})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("install status=%d body=%s", resp.StatusCode, body)
	}
	// pendingRestart=true because the DBMS is currently `started` — relate's
	// JAR-vs-PID mtime check would tag it as needing a restart to pick up
	// the new JAR.
	if !strings.Contains(string(body), `"pendingRestart":true`) {
		t.Fatalf("install response missing pendingRestart=true; body=%s", body)
	}

	// Follow-up GET /plugins/installed shows the new entry.
	resp = authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/p-04/plugins/installed", nil)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if !strings.Contains(string(body), `"name":"apoc"`) || !strings.Contains(string(body), `"pendingRestart":true`) {
		t.Fatalf("installed list missing apoc/pendingRestart=true; body=%s", body)
	}
}

func TestPlugins_InstallOnStoppedDbms_PendingRestartFalse(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	seedDbmsWithPlugins(t, srv, "p-04b", "stopped", []map[string]any{
		{"name": "apoc", "version": "5.20.0", "filePath": "/p/apoc.jar"},
	}, nil)

	resp := authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/dbmss/p-04b/plugins/install",
		map[string]any{"pluginName": "apoc"})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("install status=%d body=%s", resp.StatusCode, body)
	}
	// Stopped DBMS → next start picks up the JAR cleanly → pendingRestart=false.
	if !strings.Contains(string(body), `"pendingRestart":false`) {
		t.Fatalf("install on stopped DBMS: expected pendingRestart=false; body=%s", body)
	}
}

func TestPlugins_InstallIdempotent_ReturnsExistingEntry(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	seedDbmsWithPlugins(t, srv, "p-05", "started",
		[]map[string]any{{"name": "apoc", "filePath": "/p/apoc.jar"}},
		[]map[string]any{{"name": "apoc", "version": "5.20.0", "filePath": "/p/apoc.jar", "pendingRestart": false}},
	)

	// Already-installed plugin → server returns existing entry verbatim,
	// no duplicate appended.
	resp := authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/dbmss/p-05/plugins/install",
		map[string]any{"pluginName": "apoc"})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("install status=%d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"pendingRestart":false`) {
		t.Fatalf("re-install returned new entry instead of existing; body=%s", body)
	}

	// Installed list must still have exactly one apoc entry.
	resp = authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/p-05/plugins/installed", nil)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if strings.Count(string(body), `"name":"apoc"`) != 1 {
		t.Fatalf("install not idempotent — duplicate apoc; body=%s", body)
	}
}

func TestPlugins_UninstallRemoves_Idempotent(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	seedDbmsWithPlugins(t, srv, "p-06", "started", nil, []map[string]any{
		{"name": "apoc", "filePath": "/p/apoc.jar"},
	})

	// First uninstall removes from list.
	resp := authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/dbmss/p-06/plugins/uninstall",
		map[string]any{"pluginName": "apoc"})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != `{"name":"apoc"}` {
		t.Fatalf("uninstall status=%d body=%s (want 200 {\"name\":\"apoc\"})", resp.StatusCode, body)
	}
	resp = authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/p-06/plugins/installed", nil)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("uninstall did not remove plugin; body=%s", body)
	}

	// Second uninstall (already removed) — idempotent, same shape.
	resp = authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/dbmss/p-06/plugins/uninstall",
		map[string]any{"pluginName": "apoc"})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != `{"name":"apoc"}` {
		t.Fatalf("idempotent uninstall: status=%d body=%s", resp.StatusCode, body)
	}
}

func TestPlugins_StartStopFlipsPendingRestartFalse(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	seedDbmsWithPlugins(t, srv, "p-07", "started", nil, []map[string]any{
		{"name": "apoc", "filePath": "/p/apoc.jar", "pendingRestart": true},
	})

	// Stop cycle clears pendingRestart on every installed plugin (REQ-F-043
	// fixture simulation).
	resp := authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/desktop/dbmss/p-07/stop", nil)
	resp.Body.Close() //nolint:errcheck
	resp = authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/p-07/plugins/installed", nil)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if !strings.Contains(string(body), `"pendingRestart":false`) {
		t.Fatalf("stop did not clear pendingRestart; body=%s", body)
	}

	// Seed pendingRestart=true again and check the start path clears too.
	adminDo(t, srv, http.MethodPost, "/_scenario/plugin", map[string]any{
		"dbms_id":   "p-07",
		"installed": []map[string]any{{"name": "apoc", "filePath": "/p/apoc.jar", "pendingRestart": true}},
	}).Body.Close() //nolint:errcheck
	resp = authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/dbmss/p-07/start", nil)
	resp.Body.Close() //nolint:errcheck
	resp = authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/p-07/plugins/installed", nil)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if !strings.Contains(string(body), `"pendingRestart":false`) {
		t.Fatalf("start did not clear pendingRestart; body=%s", body)
	}
}

func TestPlugins_404_DisambiguatesDbmsVsPlugin(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	seedDbmsWithPlugins(t, srv, "p-08", "stopped", []map[string]any{
		{"name": "apoc", "filePath": "/p/apoc.jar"},
	}, nil)

	// Unknown DBMS → message must contain DBMS but NOT plugin so the
	// production client's classifyPluginNotFound routes to ErrDbmsNotFound.
	resp := authedDo(t, srv, signFn, http.MethodGet, "/fastify/api/dbmss/ghost/plugins/installed", nil)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("ghost dbms: status=%d, want 404", resp.StatusCode)
	}
	lower := strings.ToLower(string(body))
	if !strings.Contains(lower, "dbms") || strings.Contains(lower, "plugin") {
		t.Fatalf("dbms-not-found body must contain 'dbms' not 'plugin'; body=%s", body)
	}

	// Known DBMS, unknown plugin → message must contain plugin (not dbms)
	// so disambiguation routes to ErrPluginNotFound.
	resp = authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/dbmss/p-08/plugins/install",
		map[string]any{"pluginName": "not-a-plugin"})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown plugin: status=%d, want 404", resp.StatusCode)
	}
	lower = strings.ToLower(string(body))
	if !strings.Contains(lower, "plugin") || strings.Contains(lower, "dbms") {
		t.Fatalf("plugin-not-found body must contain 'plugin' not 'dbms'; body=%s", body)
	}
}

func TestCreateDbms_DuplicateName_400(t *testing.T) {
	srv, _, signFn := newFixtureServer(t)
	create := map[string]any{"name": "dup", "version": "5.20.0", "credentials": "p"}
	resp := authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/desktop/dbmss", create)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first create status=%d", resp.StatusCode)
	}
	resp = authedDo(t, srv, signFn, http.MethodPost, "/fastify/api/desktop/dbmss", create)
	resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("duplicate create status=%d, want 400", resp.StatusCode)
	}
}
