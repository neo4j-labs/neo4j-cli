// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import "sync"

// authMode controls how the auth middleware in handlers.go responds to authed
// /fastify/api/* requests. `accept` is the default — requests with a valid
// JWT pass; everything else 401s. The three negative modes simulate the three
// transport sad-paths the production binary must handle:
//
//   - reject: any authed call returns 401 (REQ-F-010)
//   - status500: any authed call returns 500 with a short body (REQ-F-024)
//   - close: any authed call slams the connection mid-stream (REQ-F-008
//     transport EOF path)
//
// The probe endpoint (`/fastify/api-docs`) is exempt from authMode — it
// always returns 200 so the seam-bypassed probe + salt loader still succeed.
type authMode int

const (
	authModeAccept authMode = iota
	authModeReject
	authModeStatus500
	authModeClose
)

// transition pins a single status flip for a DBMS so the test can simulate
// the `start --wait` / `stop --wait` polling loop. The fixture decrements
// `afterCalls` on each `GET /fastify/api/dbmss/:id` for the matching id;
// when it reaches 0, the DBMS's `Status` flips to `toStatus` and the
// transition is cleared. Set `afterCalls` to 0 for an immediate flip on the
// next read.
type transition struct {
	toStatus   string
	afterCalls int
}

// dbms is the wire-shape sent on `/fastify/api/dbmss/info` and
// `/fastify/api/dbmss/:id`. The lightweight `/fastify/api/dbmss` route omits
// `Status` and `ServerStatus` by projecting via dbmsLite (see handlers.go) —
// matching relate's actual route in
// `packages/web/src/fastify/routes/dbms.routes.ts:7-17`. Task-009 caught a
// resolver bug that hinged on this distinction.
//
// `availablePlugins` + `installedPlugins` carry the per-DBMS plugin state
// the `/plugins/*` routes serve. They never leak into the `dbms` JSON
// payload — both fields are unexported, so `json.Marshal` skips them.
type dbms struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	ConnectionURI string `json:"connectionUri"`
	Version       string `json:"version"`
	Edition       string `json:"edition"`

	// availablePlugins is the installable catalog `GET /plugins/available`
	// returns for this DBMS. Seeded by scenarioPutPlugin; stays empty by
	// default so the install endpoint surfaces ErrPluginNotFound unless the
	// scenario primes it.
	availablePlugins []dbmsPlugin
	// installedPlugins is the list `GET /plugins/installed` returns. Install
	// appends here; uninstall removes; start/stop flip pendingRestart to
	// false to simulate relate's JAR-vs-PID mtime comparison (REQ-F-043).
	installedPlugins []dbmsPlugin
}

// dbmsPlugin mirrors `desktopclient.DbmsPlugin` so the JSON wire shape on
// `/plugins/installed` + `/plugins/available` + `/plugins/install` matches
// what the production client expects. Same `omitempty` rules on `Version` —
// relate omits the field when the JAR-scan can't derive one.
type dbmsPlugin struct {
	Name           string `json:"name"`
	Version        string `json:"version,omitempty"`
	FilePath       string `json:"filePath"`
	PendingRestart bool   `json:"pendingRestart"`
}

// dbmsLite is the lightweight shape `GET /fastify/api/dbmss` returns —
// critically WITHOUT the status field, since task-009 caught a regression
// where the resolver filtered on `list[i].Status` against this endpoint and
// always saw an empty string. Keep status off this shape on purpose.
type dbmsLite struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ConnectionURI string `json:"connectionUri"`
	Version       string `json:"version"`
	Edition       string `json:"edition"`
}

// uploadTask mirrors one entry in the relate `GET /tasks` response that the
// Aura deploy path waits on. `POST /dbmss/:id/databases/upload` registers one
// tagged ["db:upload", <dbmsId>] with isLoading=true; subsequent `GET /tasks`
// polls advance it to isSuccess (or isError when the scenario forces failure).
type uploadTask struct {
	ID     string         `json:"id"`
	Tags   []string       `json:"tags"`
	Status uploadTaskStat `json:"status"`
}

// uploadTaskStat mirrors the relate task `status` object — exactly one boolean
// is true once the task settles.
type uploadTaskStat struct {
	IsLoading bool `json:"isLoading"`
	IsSuccess bool `json:"isSuccess"`
	IsError   bool `json:"isError"`
}

// uploadRecord captures the request shape the production client sent to
// `POST /dbmss/:id/databases/upload` so tests can assert the source/target body.
type uploadRecord struct {
	DbmsID         string
	SourceDatabase string
	TargetURI      string
	TargetUsername string
	TargetPassword string
	Overwrite      bool
}

// dbmsVersion mirrors one entry in `GET /dbmss/versions`. Fixture serves a
// small canned catalog; `desktop dbms create` without --version picks the latest
// stable enterprise entry from this list.
type dbmsVersion struct {
	Edition string `json:"edition"`
	Version string `json:"version"`
	Origin  string `json:"origin"`
	Dist    string `json:"dist"`
}

// connection mirrors a saved remote DB connection profile — Desktop's
// `Connection` from `packages/web/src/fastify/routes/connection.routes.ts`.
// The fixture round-trips every field; the production client only renders a
// subset by default.
type connection struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Project       string `json:"project,omitempty"`
	ConnectionURI string `json:"connectionUri"`
}

// creds is the wire-shape returned by `GET /fastify/api/credentials/:key`.
// A nil entry for a given key means Desktop returns the JSON literal `null`
// (the REQ-F-028 prompt-or-fail path).
type creds struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// state is the in-memory scenario store the fixture mutates via the
// `_scenario/*` admin endpoints. Every field is read and written under `mu`
// because handlers.go runs concurrently per net/http's default ServeMux.
type state struct {
	mu sync.Mutex

	// salt is the JWT signing salt the fixture validates against. Set once
	// at process start from --salt; tests that need wrong-salt rejection
	// should sign with a different value and rely on accept being the
	// default authMode.
	salt string

	// origin is the value folded into the JWT signing key on the verify
	// side. Production neo4j-cli signs with whatever the seam returns from
	// NEO4J_CLI_DESKTOP_E2E_HTTP_ORIGIN — the harness sets it to the
	// fixture URL so signing and verification agree.
	origin string

	auth authMode

	dbmss       map[string]*dbms // keyed by ID
	dbmsOrder   []string         // insertion order for deterministic list output
	connections map[string]*connection
	connOrder   []string

	// credentials maps the relate credential key (`dbms:<id>` or
	// `connection:<id>`) to a *creds — nil means "Desktop knows about this
	// key but has no creds" (REQ-F-028 null-creds path), absent means 404.
	credentials map[string]*creds

	// transitions maps dbms ID → pending status flip used to simulate
	// `start --wait` polling. Decremented on each GET /dbmss/:id; flipped
	// and cleared when afterCalls reaches 0.
	transitions map[string]*transition

	// autoProgress, when true, makes every GET /dbmss/:id automatically
	// advance a `starting` DBMS to `started` and a `stopping` DBMS to
	// `stopped`. Lets e2e tests drive the production `--wait` poll loop to
	// convergence without arming a per-id transition by hand for every
	// lifecycle scenario. Off by default; toggled via
	// `POST /_scenario/auto_progress`.
	autoProgress bool

	// versions is the canned `/dbmss/versions` catalog. Defaults to a
	// single stable enterprise entry — tests that need version selection
	// behavior overwrite it via the scenario admin.
	versions []dbmsVersion

	// uploadTasks accumulates the db:upload tasks registered by
	// `POST /dbmss/:id/databases/upload`. `GET /tasks` returns them and, on
	// each poll, settles any still-loading task per uploadFail (success by
	// default, error when uploadFail is true). This is how the Aura deploy
	// path's WaitForUploadTask poll loop converges against the fixture.
	uploadTasks []*uploadTask

	// uploadFail, when true, makes the next-settled db:upload task report
	// isError instead of isSuccess. Toggled via `POST /_scenario/upload_fail`
	// so the deploy sad-path (upload task failed) has e2e coverage.
	uploadFail bool

	// uploads records every `POST .../databases/upload` request body so tests
	// can assert the exact source/target shape the production client sent.
	uploads []uploadRecord

	// requestLog accumulates one line per /fastify/api/* call (method +
	// path + status) so test failures can dump the trace. Bounded only by
	// process lifetime; e2e tests spawn a fresh process per Go test so
	// this is fine.
	requestLog []string
}

// newState returns a fixture state initialised to "accept" auth mode with an
// empty DBMS/connection catalog and one default enterprise version entry.
// `origin` is patched in main() once net.Listen has returned the chosen
// port.
func newState(salt string) *state {
	return &state{
		salt:        salt,
		auth:        authModeAccept,
		dbmss:       map[string]*dbms{},
		connections: map[string]*connection{},
		credentials: map[string]*creds{},
		transitions: map[string]*transition{},
		versions: []dbmsVersion{
			{Edition: "enterprise", Version: "5.20.0", Origin: "online",
				Dist: "https://dist.neo4j.org/neo4j-enterprise-5.20.0-unix.tar.gz"},
		},
	}
}

// setOrigin pins the origin folded into the JWT verification key. Called
// once from main() after the listen port is known; safe to call from tests
// before any request lands.
func (s *state) setOrigin(o string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.origin = o
}

// reset wipes scenario state but preserves salt + origin (those are
// process-level, not per-scenario). Called by POST /_scenario/reset so
// individual e2e cases can run order-independently.
func (s *state) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auth = authModeAccept
	s.dbmss = map[string]*dbms{}
	s.dbmsOrder = nil
	s.connections = map[string]*connection{}
	s.connOrder = nil
	s.credentials = map[string]*creds{}
	s.transitions = map[string]*transition{}
	s.autoProgress = false
	s.versions = []dbmsVersion{
		{Edition: "enterprise", Version: "5.20.0", Origin: "online",
			Dist: "https://dist.neo4j.org/neo4j-enterprise-5.20.0-unix.tar.gz"},
	}
	s.uploadTasks = nil
	s.uploadFail = false
	s.uploads = nil
	s.requestLog = nil
}

// logRequest appends one trace line. Holds the lock just for the append so
// it composes cleanly with the call-site lock acquisition.
func (s *state) logRequest(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requestLog = append(s.requestLog, line)
}
