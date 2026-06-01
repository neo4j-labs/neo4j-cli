// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// scenarioPutDbms accepts an add-or-replace payload and persists it in the
// in-memory catalog. Used by e2e cases to pre-seed any combination of
// dbmss without going through the create endpoint (which generates IDs and
// forces status=stopped). Body schema:
//
//	{
//	  "id":            "<uuid>",            // optional, generated if missing
//	  "name":          "...",
//	  "status":        "started" | "stopped" | "starting" | "stopping",
//	  "connectionUri": "...",
//	  "version":       "...",
//	  "edition":       "...",
//	  "creds":         {"username":"...","password":"..."} | null | omitted
//	}
//
// `creds=null` registers a null entry under `dbms:<id>` so the production
// client's null-creds branch (REQ-F-028) fires; omitting the key leaves
// credentials unset (404 from GET /credentials).
func scenarioPutDbms(s *state, w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID            string          `json:"id"`
		Name          string          `json:"name"`
		Status        string          `json:"status"`
		ConnectionURI string          `json:"connectionUri"`
		Version       string          `json:"version"`
		Edition       string          `json:"edition"`
		Creds         json.RawMessage `json:"creds"`
	}
	if err := decodeBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.ID == "" {
		body.ID = uuid.NewString()
	}
	if body.Status == "" {
		body.Status = "stopped"
	}
	if body.Version == "" {
		body.Version = "5.20.0"
	}
	if body.Edition == "" {
		body.Edition = "enterprise"
	}
	s.mu.Lock()
	if _, ok := s.dbmss[body.ID]; !ok {
		s.dbmsOrder = append(s.dbmsOrder, body.ID)
	}
	s.dbmss[body.ID] = &dbms{
		ID: body.ID, Name: body.Name, Status: body.Status,
		ConnectionURI: body.ConnectionURI, Version: body.Version, Edition: body.Edition,
	}
	if body.Creds != nil {
		trimmed := strings.TrimSpace(string(body.Creds))
		switch trimmed {
		case "null":
			s.credentials["dbms:"+body.ID] = nil
		default:
			var c creds
			if err := json.Unmarshal(body.Creds, &c); err == nil {
				s.credentials["dbms:"+body.ID] = &c
			}
		}
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":"` + body.ID + `"}`))
}

// scenarioPutConnection mirrors scenarioPutDbms for the `connections`
// resource. Same `creds` semantics — pass `null` to register a null entry
// and exercise the prompt-or-fail path.
func scenarioPutConnection(s *state, w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID            string          `json:"id"`
		Name          string          `json:"name"`
		Description   string          `json:"description"`
		Project       string          `json:"project"`
		ConnectionURI string          `json:"connectionUri"`
		Creds         json.RawMessage `json:"creds"`
	}
	if err := decodeBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.ID == "" {
		body.ID = uuid.NewString()
	}
	s.mu.Lock()
	if _, ok := s.connections[body.ID]; !ok {
		s.connOrder = append(s.connOrder, body.ID)
	}
	s.connections[body.ID] = &connection{
		ID: body.ID, Name: body.Name, Description: body.Description,
		Project: body.Project, ConnectionURI: body.ConnectionURI,
	}
	if body.Creds != nil {
		trimmed := strings.TrimSpace(string(body.Creds))
		switch trimmed {
		case "null":
			s.credentials["connection:"+body.ID] = nil
		default:
			var c creds
			if err := json.Unmarshal(body.Creds, &c); err == nil {
				s.credentials["connection:"+body.ID] = &c
			}
		}
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":"` + body.ID + `"}`))
}

// scenarioPutPlugin seeds the per-DBMS plugin lists (`available` +
// `installed`) used by the `/dbmss/:id/plugins/*` routes. Body schema:
//
//	{
//	  "dbms_id":   "<id>",                     // required, must already exist
//	  "available": [{name, version, filePath, pendingRestart}, ...],
//	  "installed": [{name, version, filePath, pendingRestart}, ...]
//	}
//
// Omitting `available` or `installed` leaves that side untouched. Pass
// an empty `[]` to clear. Unknown `dbms_id` returns 400 — call
// `_scenario/dbms` first to seed the DBMS.
func scenarioPutPlugin(s *state, w http.ResponseWriter, r *http.Request) {
	var body struct {
		DbmsID    string        `json:"dbms_id"`
		Available *[]dbmsPlugin `json:"available"`
		Installed *[]dbmsPlugin `json:"installed"`
	}
	if err := decodeBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.DbmsID == "" {
		http.Error(w, "dbms_id required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	d, ok := s.dbmss[body.DbmsID]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "unknown dbms id", http.StatusBadRequest)
		return
	}
	if body.Available != nil {
		d.availablePlugins = append([]dbmsPlugin(nil), (*body.Available)...)
	}
	if body.Installed != nil {
		d.installedPlugins = append([]dbmsPlugin(nil), (*body.Installed)...)
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// scenarioPutCredential stores an arbitrary credential entry by full key
// (`dbms:<id>` or `connection:<id>`). Body: `{key, value}` where `value`
// is `null` or `{username, password}`. Lets tests register credentials
// without a matching DBMS / connection — useful for edge cases where the
// production client looks up credentials for an unknown id.
func scenarioPutCredential(s *state, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	}
	if err := decodeBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Key == "" {
		http.Error(w, "key required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	trimmed := strings.TrimSpace(string(body.Value))
	if body.Value == nil || trimmed == "null" {
		s.credentials[body.Key] = nil
	} else {
		var c creds
		if err := json.Unmarshal(body.Value, &c); err != nil {
			s.mu.Unlock()
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.credentials[body.Key] = &c
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// scenarioSetAuthMode flips the auth-middleware behaviour for subsequent
// /fastify/api/* requests. See auth.go for the four modes and what they
// simulate.
func scenarioSetAuthMode(s *state, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if err := decodeBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var m authMode
	switch body.Mode {
	case "accept", "":
		m = authModeAccept
	case "reject":
		m = authModeReject
	case "500":
		m = authModeStatus500
	case "close":
		m = authModeClose
	default:
		http.Error(w, "mode must be one of accept|reject|500|close", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.auth = m
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// scenarioSetTransition arms a status flip for a DBMS so the
// `start --wait` / `stop --wait` polling loop in production has something
// to converge on. Body `{id, to_status, after_calls}` — the next
// `GET /dbmss/:id` call decrements after_calls; when it hits 0 the status
// flips to to_status and the transition is cleared. Set after_calls=0 for
// an immediate flip on the next read.
func scenarioSetTransition(s *state, w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID         string `json:"id"`
		ToStatus   string `json:"to_status"`
		AfterCalls int    `json:"after_calls"`
	}
	if err := decodeBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.ID == "" || body.ToStatus == "" {
		http.Error(w, "id and to_status required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if _, ok := s.dbmss[body.ID]; !ok {
		s.mu.Unlock()
		http.Error(w, "unknown dbms id", http.StatusBadRequest)
		return
	}
	s.transitions[body.ID] = &transition{toStatus: body.ToStatus, afterCalls: body.AfterCalls}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// scenarioSetAutoProgress toggles the auto-progress flag — when on, every GET
// /dbmss/:id automatically flips `starting` → `started` and `stopping` →
// `stopped`. Lets e2e cases drive the production `--wait` poll loop to
// convergence without arming a per-id transition (which would race against
// the preflight enrichment GET).
func scenarioSetAutoProgress(s *state, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.autoProgress = body.Enabled
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// scenarioSetVersions overwrites the `/dbmss/versions` catalog. Body is
// the full `[]dbmsVersion` payload. Tests use this to exercise the version
// auto-pick path in `desktop dbms create` when --version is omitted.
func scenarioSetVersions(s *state, w http.ResponseWriter, r *http.Request) {
	var body []dbmsVersion
	if err := decodeBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.versions = body
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// scenarioSetUploadFail toggles whether the next-settled db:upload task
// reports isError instead of isSuccess. Body `{enabled: bool}`. Lets the
// deploy sad-path (Desktop reported the upload failed) get e2e coverage.
func scenarioSetUploadFail(s *state, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeBody(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.uploadFail = body.Enabled
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// scenarioGetLog returns the accumulated request trace as a plain-text
// log. The e2e harness dumps this on test failure so CI logs are
// debuggable.
func scenarioGetLog(s *state, w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	out := strings.Join(s.requestLog, "\n")
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(out))
}
