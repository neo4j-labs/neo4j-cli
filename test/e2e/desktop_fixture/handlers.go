// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// newMux wires the fixture's two route trees: the relate-API surface under
// `/fastify/...` (gated by requireAuth — except the probe target) and the
// `_scenario/*` admin control plane. Built once at startup so handlers
// share the same *state pointer.
func newMux(s *state) *http.ServeMux {
	mux := http.NewServeMux()

	// Probe target — production neo4j-cli's ProbePort hits this without
	// auth headers, expects 200, ignores the body. Exempt from authMode so
	// even a `reject` scenario still discovers Desktop's presence.
	mux.HandleFunc("/fastify/api-docs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		s.logRequest(fmt.Sprintf("%s %s -> 200 (probe)", r.Method, r.URL.Path))
	})

	// Single relate dispatcher — net/http's ServeMux doesn't pattern-match
	// path parameters, so we route everything under /fastify/api/ through
	// one handler and switch on method + suffix.
	mux.HandleFunc("/fastify/api/", requireAuth(s, func(w http.ResponseWriter, r *http.Request) {
		serveRelate(s, w, r)
	}))

	// Scenario admin (out-of-band, unauthenticated).
	mux.HandleFunc("/_scenario/reset", func(w http.ResponseWriter, r *http.Request) {
		s.reset()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/_scenario/dbms", func(w http.ResponseWriter, r *http.Request) {
		scenarioPutDbms(s, w, r)
	})
	mux.HandleFunc("/_scenario/connection", func(w http.ResponseWriter, r *http.Request) {
		scenarioPutConnection(s, w, r)
	})
	mux.HandleFunc("/_scenario/plugin", func(w http.ResponseWriter, r *http.Request) {
		scenarioPutPlugin(s, w, r)
	})
	mux.HandleFunc("/_scenario/credential", func(w http.ResponseWriter, r *http.Request) {
		scenarioPutCredential(s, w, r)
	})
	mux.HandleFunc("/_scenario/auth_mode", func(w http.ResponseWriter, r *http.Request) {
		scenarioSetAuthMode(s, w, r)
	})
	mux.HandleFunc("/_scenario/transition", func(w http.ResponseWriter, r *http.Request) {
		scenarioSetTransition(s, w, r)
	})
	mux.HandleFunc("/_scenario/auto_progress", func(w http.ResponseWriter, r *http.Request) {
		scenarioSetAutoProgress(s, w, r)
	})
	mux.HandleFunc("/_scenario/versions", func(w http.ResponseWriter, r *http.Request) {
		scenarioSetVersions(s, w, r)
	})
	mux.HandleFunc("/_scenario/upload_fail", func(w http.ResponseWriter, r *http.Request) {
		scenarioSetUploadFail(s, w, r)
	})
	mux.HandleFunc("/_scenario/log", func(w http.ResponseWriter, r *http.Request) {
		scenarioGetLog(s, w, r)
	})

	return mux
}

// serveRelate dispatches the authed /fastify/api/* surface to the matching
// per-resource handler. Switch order mirrors longest-prefix-wins so the
// `/dbmss/info` route matches before the catch-all `/dbmss/:id`.
func serveRelate(s *state, w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/fastify/api")

	switch {
	case path == "/dbmss" && r.Method == http.MethodGet:
		listDbmssLite(s, w, r)
	case path == "/desktop/dbmss" && r.Method == http.MethodPost:
		createDbms(s, w, r)
	case path == "/dbmss/info" && r.Method == http.MethodGet:
		listDbmssInfo(s, w, r)
	case path == "/dbmss/versions" && r.Method == http.MethodGet:
		listVersions(s, w, r)
	case strings.HasPrefix(path, "/dbmss/") && strings.HasSuffix(path, "/start") && r.Method == http.MethodPost:
		startDbms(s, w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/dbmss/"), "/start"))
	case strings.HasPrefix(path, "/desktop/dbmss/") && strings.HasSuffix(path, "/stop") && r.Method == http.MethodPost:
		stopDbms(s, w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/desktop/dbmss/"), "/stop"))
	case strings.HasPrefix(path, "/dbmss/") && strings.HasSuffix(path, "/plugins/installed") && r.Method == http.MethodGet:
		listInstalledPlugins(s, w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/dbmss/"), "/plugins/installed"))
	case strings.HasPrefix(path, "/dbmss/") && strings.HasSuffix(path, "/plugins/available") && r.Method == http.MethodGet:
		listAvailablePlugins(s, w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/dbmss/"), "/plugins/available"))
	case strings.HasPrefix(path, "/dbmss/") && strings.HasSuffix(path, "/plugins/install") && r.Method == http.MethodPost:
		installPlugin(s, w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/dbmss/"), "/plugins/install"))
	case strings.HasPrefix(path, "/dbmss/") && strings.HasSuffix(path, "/plugins/uninstall") && r.Method == http.MethodPost:
		uninstallPlugin(s, w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/dbmss/"), "/plugins/uninstall"))
	case strings.HasPrefix(path, "/dbmss/") && strings.HasSuffix(path, "/databases/upload") && r.Method == http.MethodPost:
		uploadDatabase(s, w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/dbmss/"), "/databases/upload"))
	case path == "/tasks" && r.Method == http.MethodGet:
		listTasks(s, w, r)
	case strings.HasPrefix(path, "/dbmss/") && r.Method == http.MethodGet:
		getDbms(s, w, r, strings.TrimPrefix(path, "/dbmss/"))
	case strings.HasPrefix(path, "/dbmss/") && r.Method == http.MethodDelete:
		deleteDbms(s, w, r, strings.TrimPrefix(path, "/dbmss/"))

	case path == "/connections" && r.Method == http.MethodGet:
		listConnections(s, w, r)
	case path == "/connections" && r.Method == http.MethodPost:
		createConnection(s, w, r)
	case strings.HasPrefix(path, "/connections/") && r.Method == http.MethodPatch:
		updateConnection(s, w, r, strings.TrimPrefix(path, "/connections/"))
	case strings.HasPrefix(path, "/connections/") && r.Method == http.MethodDelete:
		deleteConnection(s, w, r, strings.TrimPrefix(path, "/connections/"))

	case strings.HasPrefix(path, "/credentials/") && r.Method == http.MethodGet:
		getCredentials(s, w, r, strings.TrimPrefix(path, "/credentials/"))

	default:
		http.NotFound(w, r)
		s.logRequest(fmt.Sprintf("%s %s -> 404", r.Method, r.URL.Path))
	}
}

// listDbmssLite serves GET /dbmss — the lightweight shape WITHOUT status.
// Critical: relate's real route omits status here. Task-009 caught a bug
// that rode in via fixtures that put status on this endpoint.
func listDbmssLite(s *state, w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	out := make([]dbmsLite, 0, len(s.dbmsOrder))
	for _, id := range s.dbmsOrder {
		d := s.dbmss[id]
		out = append(out, dbmsLite{
			ID: d.ID, Name: d.Name, ConnectionURI: d.ConnectionURI,
			Version: d.Version, Edition: d.Edition,
		})
	}
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, out)
}

// listDbmssInfo serves GET /dbmss/info — full shape with status. This is
// the endpoint the query `-c desktop` resolver hits to filter by
// status=="started".
func listDbmssInfo(s *state, w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	out := make([]dbms, 0, len(s.dbmsOrder))
	for _, id := range s.dbmsOrder {
		out = append(out, *s.dbmss[id])
	}
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, out)
}

// getDbms serves GET /dbmss/:id. Decrements any pending transition counter
// for this id and flips the status when the counter hits zero — that's the
// hook the `start --wait` poll loop in production relies on.
func getDbms(s *state, w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	d, ok := s.dbmss[id]
	if !ok {
		s.mu.Unlock()
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("dbms %q not found", id))
		return
	}
	if t, ok := s.transitions[id]; ok {
		if t.afterCalls <= 0 {
			d.Status = t.toStatus
			delete(s.transitions, id)
		} else {
			t.afterCalls--
		}
	} else if s.autoProgress {
		// autoProgress is the broad-stroke equivalent of a per-id transition
		// — any GET advances a `starting` DBMS to `started` and a `stopping`
		// DBMS to `stopped`. Lets lifecycle e2e cases converge without
		// arming per-id transitions and without racing against the
		// production binary's preflight GET.
		switch d.Status {
		case "starting":
			d.Status = "started"
		case "stopping":
			d.Status = "stopped"
		}
	}
	out := *d
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, out)
}

// createDbms serves POST /dbmss. Mirrors relate's create body shape
// {name, version, credentials}. Returns 400 on duplicate name (the
// production client surfaces this as REQ-F-006 collision text). Stores the
// initial password under `dbms:<id>` so a follow-up
// `GET /credentials/dbms:<id>` returns the same value.
func createDbms(s *state, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Credentials string `json:"credentials"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(s, w, r, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == "" {
		writeError(s, w, r, http.StatusBadRequest, "name is required")
		return
	}
	if body.Version == "" {
		writeError(s, w, r, http.StatusBadRequest, "version is required")
		return
	}
	s.mu.Lock()
	for _, d := range s.dbmss {
		if d.Name == body.Name {
			s.mu.Unlock()
			writeError(s, w, r, http.StatusBadRequest, fmt.Sprintf("dbms with name %q already exists", body.Name))
			return
		}
	}
	id := uuid.NewString()
	d := &dbms{
		ID:            id,
		Name:          body.Name,
		Status:        "stopped",
		ConnectionURI: "neo4j://127.0.0.1:7687",
		Version:       body.Version,
		Edition:       "enterprise",
	}
	s.dbmss[id] = d
	s.dbmsOrder = append(s.dbmsOrder, id)
	s.credentials["dbms:"+id] = &creds{Username: "neo4j", Password: body.Credentials}
	out := *d
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, out)
}

// deleteDbms serves DELETE /dbmss/:id. Returns the deleted DbmsInfo — the
// production client decodes it for the success line.
func deleteDbms(s *state, w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	d, ok := s.dbmss[id]
	if !ok {
		s.mu.Unlock()
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("dbms %q not found", id))
		return
	}
	delete(s.dbmss, id)
	s.dbmsOrder = removeID(s.dbmsOrder, id)
	delete(s.credentials, "dbms:"+id)
	delete(s.transitions, id)
	out := *d
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, out)
}

// startDbms serves POST /dbmss/:id/start. Mutates status synchronously to
// `starting`; transitions can then flip it to `started` after N polls. Also
// flips `pendingRestart=false` on every installed plugin — REQ-F-043's
// fixture stand-in for relate's JAR-vs-PID mtime check. Returns relate's
// empty-object body — the production client discards it.
func startDbms(s *state, w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	d, ok := s.dbmss[id]
	if !ok {
		s.mu.Unlock()
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("dbms %q not found", id))
		return
	}
	d.Status = "starting"
	clearPendingRestart(d)
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, map[string]any{})
}

// stopDbms serves POST /dbmss/:id/stop. Symmetric to startDbms — flips
// status to `stopping`; transitions can flip to `stopped`. Also clears
// `pendingRestart` on installed plugins so a follow-up start observes a
// clean slate (the JAR-vs-PID timestamp comparison would tag this as
// stopped → next start is in-sync).
func stopDbms(s *state, w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	d, ok := s.dbmss[id]
	if !ok {
		s.mu.Unlock()
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("dbms %q not found", id))
		return
	}
	d.Status = "stopping"
	clearPendingRestart(d)
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, map[string]any{})
}

// clearPendingRestart flips pendingRestart=false on every installed plugin.
// Caller MUST hold s.mu. REQ-F-043 simulation: relate compares JAR mtime
// vs PID start time — every restart cycle synchronises the two, so we
// reset on start AND stop transitions to mirror that.
func clearPendingRestart(d *dbms) {
	for i := range d.installedPlugins {
		d.installedPlugins[i].PendingRestart = false
	}
}

// listInstalledPlugins serves GET /dbmss/:id/plugins/installed. Returns
// 404 with the relate-shaped `Could not find DBMS` body so client-side
// disambiguation (ErrDbmsNotFound) routes correctly.
func listInstalledPlugins(s *state, w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	d, ok := s.dbmss[id]
	if !ok {
		s.mu.Unlock()
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("Could not find DBMS %q", id))
		return
	}
	out := append([]dbmsPlugin(nil), d.installedPlugins...)
	s.mu.Unlock()
	if out == nil {
		out = []dbmsPlugin{}
	}
	writeJSON(s, w, r, http.StatusOK, out)
}

// listAvailablePlugins serves GET /dbmss/:id/plugins/available. Mirrors
// `listInstalledPlugins` shape with the DBMS-scoped catalog.
func listAvailablePlugins(s *state, w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	d, ok := s.dbmss[id]
	if !ok {
		s.mu.Unlock()
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("Could not find DBMS %q", id))
		return
	}
	out := append([]dbmsPlugin(nil), d.availablePlugins...)
	s.mu.Unlock()
	if out == nil {
		out = []dbmsPlugin{}
	}
	writeJSON(s, w, r, http.StatusOK, out)
}

// installPlugin serves POST /dbmss/:id/plugins/install. Body shape mirrors
// relate's `{pluginName}` (no path support — fixture is name-driven). On
// success returns the installed `DbmsPlugin`; idempotent — installing an
// already-installed plugin returns the existing entry (REQ-F-038-ish).
// 404 disambiguation: unknown DBMS → `Could not find DBMS ...`; known DBMS
// but plugin not in `availablePlugins` → `Could not find plugin ...`.
func installPlugin(s *state, w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		PluginName string `json:"pluginName"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(s, w, r, http.StatusBadRequest, err.Error())
		return
	}
	if body.PluginName == "" {
		writeError(s, w, r, http.StatusBadRequest, "pluginName is required")
		return
	}
	s.mu.Lock()
	d, ok := s.dbmss[id]
	if !ok {
		s.mu.Unlock()
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("Could not find DBMS %q", id))
		return
	}
	// Idempotent: already-installed → return existing entry verbatim.
	for _, p := range d.installedPlugins {
		if p.Name == body.PluginName {
			out := p
			s.mu.Unlock()
			writeJSON(s, w, r, http.StatusOK, out)
			return
		}
	}
	// Lookup in the available catalog. Path-style values (`/foo/bar.jar`)
	// also resolve when they match a catalog entry's FilePath — keeps the
	// fixture simple while still exercising the verbatim-forwarding path
	// covered by `desktopclient` unit tests.
	var match *dbmsPlugin
	for i := range d.availablePlugins {
		p := &d.availablePlugins[i]
		if p.Name == body.PluginName || (p.FilePath != "" && p.FilePath == body.PluginName) {
			match = p
			break
		}
	}
	if match == nil {
		s.mu.Unlock()
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("Could not find plugin %q", body.PluginName))
		return
	}
	// pendingRestart: true when the DBMS is currently `started` (the new
	// JAR is on disk but the running PID predates it); false when stopped
	// (next start will pick it up cleanly).
	newEntry := *match
	newEntry.PendingRestart = d.Status == "started"
	d.installedPlugins = append(d.installedPlugins, newEntry)
	out := newEntry
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, out)
}

// uninstallPlugin serves POST /dbmss/:id/plugins/uninstall. Body shape
// `{pluginName}` matches install. Always returns `{name}` — idempotent at
// the relate layer (REQ-F-038): uninstalling a not-installed plugin still
// succeeds with the supplied name.
func uninstallPlugin(s *state, w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		PluginName string `json:"pluginName"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(s, w, r, http.StatusBadRequest, err.Error())
		return
	}
	if body.PluginName == "" {
		writeError(s, w, r, http.StatusBadRequest, "pluginName is required")
		return
	}
	s.mu.Lock()
	d, ok := s.dbmss[id]
	if !ok {
		s.mu.Unlock()
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("Could not find DBMS %q", id))
		return
	}
	// Remove the plugin if present; absence is not an error (REQ-F-038).
	filtered := d.installedPlugins[:0]
	for _, p := range d.installedPlugins {
		if p.Name != body.PluginName {
			filtered = append(filtered, p)
		}
	}
	d.installedPlugins = filtered
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, map[string]string{"name": body.PluginName})
}

// listVersions serves GET /dbmss/versions. Returns whatever the scenario
// loaded — by default a single stable enterprise entry so `desktop dbms create`
// without --version picks it.
func listVersions(s *state, w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	out := append([]dbmsVersion(nil), s.versions...)
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, out)
}

// uploadDatabase serves POST /dbmss/:id/databases/upload. Records the request
// source/target body and registers a db:upload task tagged ["db:upload", <id>]
// with isLoading=true; subsequent GET /tasks polls settle it. Mirrors relate's
// fire-and-forget contract — the task list, not this response, carries the
// outcome, so the body is the empty object the production client discards.
// Unknown DBMS → 404 with the relate-shaped body.
func uploadDatabase(s *state, w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Source struct {
			DatabaseName string `json:"databaseName"`
		} `json:"source"`
		Target struct {
			URI       string `json:"uri"`
			Username  string `json:"username"`
			Password  string `json:"password"`
			Overwrite bool   `json:"overwrite"`
		} `json:"target"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(s, w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	if _, ok := s.dbmss[id]; !ok {
		s.mu.Unlock()
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("Could not find DBMS %q", id))
		return
	}
	s.uploads = append(s.uploads, uploadRecord{
		DbmsID:         id,
		SourceDatabase: body.Source.DatabaseName,
		TargetURI:      body.Target.URI,
		TargetUsername: body.Target.Username,
		TargetPassword: body.Target.Password,
		Overwrite:      body.Target.Overwrite,
	})
	s.uploadTasks = append(s.uploadTasks, &uploadTask{
		ID:     uuid.NewString(),
		Tags:   []string{"db:upload", id},
		Status: uploadTaskStat{IsLoading: true},
	})
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, map[string]any{})
}

// listTasks serves GET /tasks. On each poll it settles every still-loading
// db:upload task — to isError when the scenario armed uploadFail, otherwise to
// isSuccess. This drives the production WaitForUploadTask poll loop to
// convergence (success by default; one toggle exercises the failed-task path).
func listTasks(s *state, w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	for _, t := range s.uploadTasks {
		if t.Status.IsLoading {
			if s.uploadFail {
				t.Status = uploadTaskStat{IsError: true}
			} else {
				t.Status = uploadTaskStat{IsSuccess: true}
			}
		}
	}
	out := make([]uploadTask, 0, len(s.uploadTasks))
	for _, t := range s.uploadTasks {
		out = append(out, *t)
	}
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, out)
}

// listConnections serves GET /connections. Returns the saved connections
// in insertion order.
func listConnections(s *state, w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	out := make([]connection, 0, len(s.connOrder))
	for _, id := range s.connOrder {
		out = append(out, *s.connections[id])
	}
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, out)
}

// createConnection serves POST /connections. Stores creds under
// `connection:<id>` for follow-up GET /credentials lookups. Returns 400 on
// duplicate name (mirrors relate's behaviour and the production binary's
// error path).
func createConnection(s *state, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string `json:"name"`
		ConnectionURI string `json:"connectionUri"`
		Username      string `json:"username"`
		Password      string `json:"password"`
		Description   string `json:"description"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(s, w, r, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == "" || body.ConnectionURI == "" || body.Username == "" {
		writeError(s, w, r, http.StatusBadRequest, "name, connectionUri, username are required")
		return
	}
	s.mu.Lock()
	for _, c := range s.connections {
		if c.Name == body.Name {
			s.mu.Unlock()
			writeError(s, w, r, http.StatusBadRequest, fmt.Sprintf("connection with name %q already exists", body.Name))
			return
		}
	}
	id := uuid.NewString()
	c := &connection{
		ID:            id,
		Name:          body.Name,
		Description:   body.Description,
		ConnectionURI: body.ConnectionURI,
	}
	s.connections[id] = c
	s.connOrder = append(s.connOrder, id)
	s.credentials["connection:"+id] = &creds{Username: body.Username, Password: body.Password}
	out := *c
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, out)
}

// updateConnection serves PATCH /connections/:id. Applies only the keys
// present in the body — task-005 asserts the production client builds the
// PATCH body with EXACTLY the supplied keys, so the fixture inspects which
// keys arrived for the body-shape assertion side of those tests.
func updateConnection(s *state, w http.ResponseWriter, r *http.Request, id string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(s, w, r, http.StatusBadRequest, err.Error())
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		writeError(s, w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.mu.Lock()
	c, ok := s.connections[id]
	if !ok {
		s.mu.Unlock()
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("connection %q not found", id))
		return
	}
	if v, ok := raw["name"]; ok {
		c.Name, _ = v.(string)
	}
	if v, ok := raw["connectionUri"]; ok {
		c.ConnectionURI, _ = v.(string)
	}
	if v, ok := raw["description"]; ok {
		c.Description, _ = v.(string)
	}
	// username and password go to credentials, not the connection record —
	// relate stores them via safeStorage under connection:<id>.
	cur := s.credentials["connection:"+id]
	if cur == nil {
		cur = &creds{}
	}
	if v, ok := raw["username"]; ok {
		cur.Username, _ = v.(string)
	}
	if v, ok := raw["password"]; ok {
		cur.Password, _ = v.(string)
	}
	s.credentials["connection:"+id] = cur
	out := *c
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, out)
}

// deleteConnection serves DELETE /connections/:id. Returns the deleted
// Connection so the production client can render the confirmation line.
func deleteConnection(s *state, w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	c, ok := s.connections[id]
	if !ok {
		s.mu.Unlock()
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("connection %q not found", id))
		return
	}
	delete(s.connections, id)
	s.connOrder = removeID(s.connOrder, id)
	delete(s.credentials, "connection:"+id)
	out := *c
	s.mu.Unlock()
	writeJSON(s, w, r, http.StatusOK, out)
}

// getCredentials serves GET /credentials/:key. Three shapes:
//
//   - key present, value non-nil → 200 {username, password}
//   - key present, value nil     → 200 `null` (REQ-F-028 null-creds path)
//   - key absent                 → 404
//
// Test scenarios choose between "present-nil" and "absent" by setting the
// map entry to nil vs. leaving it unset.
func getCredentials(s *state, w http.ResponseWriter, r *http.Request, key string) {
	s.mu.Lock()
	v, present := s.credentials[key]
	s.mu.Unlock()
	if !present {
		writeError(s, w, r, http.StatusNotFound, fmt.Sprintf("credentials %q not found", key))
		return
	}
	if v == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("null"))
		s.logRequest(fmt.Sprintf("%s %s -> 200 null", r.Method, r.URL.Path))
		return
	}
	writeJSON(s, w, r, http.StatusOK, v)
}

// writeJSON marshals and writes a JSON response, logging the outcome. All
// successful relate paths go through here so the trace line shape stays
// consistent.
func writeJSON(s *state, w http.ResponseWriter, r *http.Request, status int, v any) {
	buf, err := json.Marshal(v)
	if err != nil {
		writeError(s, w, r, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
	s.logRequest(fmt.Sprintf("%s %s -> %d", r.Method, r.URL.Path, status))
}

// writeError surfaces an error as a JSON object — Desktop's relate routes
// return errors as `{message: "..."}` and the production client surfaces
// the message verbatim, so the shape matters.
func writeError(s *state, w http.ResponseWriter, r *http.Request, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"message":%q}`, msg)
	s.logRequest(fmt.Sprintf("%s %s -> %d (%s)", r.Method, r.URL.Path, status, msg))
}

// decodeBody decodes the request JSON into v, returning a usable error on
// failure. Empty body is OK — leaves v at zero.
func decodeBody(r *http.Request, v any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode body: %w", err)
	}
	return nil
}

// removeID returns a new slice with id stripped. Used to keep dbmsOrder /
// connOrder in sync with the maps. O(n) but n is small (≤ 10 in any
// realistic test scenario). Allocates a fresh slice so the caller's slice
// header is not aliased.
func removeID(xs []string, id string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}
