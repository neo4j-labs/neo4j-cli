// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package desktopclient is the shared HTTP/auth/discovery client for the local
// Neo4j Desktop 2 "relate" API at http://localhost:<port>/fastify/api.
package desktopclient

// DbmsInfo mirrors the relate `GET /dbmss` response. Fields are
// omitempty-tagged where Desktop may legitimately omit them. The CLI only
// renders a subset by default (id, name, version, status, connectionUri); the
// rest are passed through for `--format json`.
type DbmsInfo struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	Tags          []string       `json:"tags,omitempty"`
	Project       string         `json:"project,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	ConnectionURI string         `json:"connectionUri,omitempty"`
	RootPath      string         `json:"rootPath,omitempty"`
	Status        string         `json:"status,omitempty"`
	ServerStatus  string         `json:"serverStatus,omitempty"`
	Version       string         `json:"version,omitempty"`
	Edition       string         `json:"edition,omitempty"`
	Prerelease    string         `json:"prerelease,omitempty"`
}

// DbmsVersion mirrors one entry in the `GET /dbmss/versions` response. The
// endpoint returns the full catalog Desktop knows about — both `origin:
// "cached"` (already on disk under `<userCache>/Cache/dbmss/`) and `origin:
// "online"` (still on dist.neo4j.org and would be downloaded during create).
// `dist` is either a local file URL or the dist.neo4j.org tarball URL
// depending on origin. `edition` is always `"enterprise"` against Desktop 2.
type DbmsVersion struct {
	Edition string `json:"edition"`
	Version string `json:"version"`
	Origin  string `json:"origin"`
	Dist    string `json:"dist"`
}

// UpgradeDbmsOptions are the optional `options` keys on `POST
// /dbmss/:id/upgrade`. Backup and Migrate are pointers so the client can
// distinguish "caller didn't set this" (nil → omit, server default applies)
// from an explicit `false`. PluginUpgradeMode is the uppercase wire enum
// (`ALL` / `NONE` / `UPGRADABLE`); empty means omit (server defaults to
// `UPGRADABLE`).
type UpgradeDbmsOptions struct {
	Backup            *bool
	Migrate           *bool
	PluginUpgradeMode string
}

// Credentials is the body returned by `GET /credentials/<key>` on the relate
// API. A `null` body (legacy DBMS / safeStorage unavailable) is represented
// by `(nil, nil)` from `GetCredentialsByKey` rather than this type's zero
// value.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Connection is a saved remote DB connection profile (Aura URI / remote Neo4j
// endpoint) the user has registered with Desktop. Optional fields are
// `omitempty` so they disappear from `--format json` output when Desktop
// doesn't populate them.
type Connection struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	Tags          []string       `json:"tags,omitempty"`
	Project       string         `json:"project,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	ConnectionURI string         `json:"connectionUri,omitempty"`
	CreatedAt     string         `json:"createdAt,omitempty"`
	ManifestPath  string         `json:"manifestPath,omitempty"`
}

// ConnectionCreateArgs are the inputs to CreateConnection. The CLI only
// surfaces the minimal set required to identify a remote DB and authenticate
// against it; relate's `tags`, `project`, and `metadata` fields are
// intentionally omitted.
type ConnectionCreateArgs struct {
	Name          string
	ConnectionURI string
	Username      string
	Password      string
	Description   string
}

// ConnectionUpdateArgs are the optional inputs to UpdateConnection. Every
// field is a pointer so the client can distinguish "caller didn't set this"
// (nil) from "caller wants to clear this" (non-nil empty string). The PATCH
// body contains ONLY the keys the caller populated.
type ConnectionUpdateArgs struct {
	Name          *string
	ConnectionURI *string
	Username      *string
	Password      *string
	Description   *string
}

// DbmsPlugin is one entry from the relate `dbms-plugins` routes. `Version` is
// optional in the relate typebox (the catalog scan can't always derive a
// version from the JAR filename).
//
// `PendingRestart` is relate's mtime-comparison between the JAR on disk and
// the running PID — `true` when the JAR was installed/updated since the DBMS
// was last started.
type DbmsPlugin struct {
	Name           string `json:"name"`
	Version        string `json:"version,omitempty"`
	FilePath       string `json:"filePath"`
	PendingRestart bool   `json:"pendingRestart"`
}

// EnvJSON is the metadata file Desktop drops in EnvConfigDir(). We only read
// it; nothing in the CLI mutates these files. Optional fields are
// `omitempty` so a missing `relateDataPath` or `httpOrigin` does not crash
// the parser.
type EnvJSON struct {
	Name           string `json:"name"`
	ID             string `json:"id"`
	Active         bool   `json:"active"`
	Type           string `json:"type"`
	RelateDataPath string `json:"relateDataPath,omitempty"`
	HTTPOrigin     string `json:"httpOrigin,omitempty"`
	ServerConfig   any    `json:"serverConfig,omitempty"`

	// Path records the absolute on-disk path of the JSON we read this from.
	// NOT a wire field; populated by the loader for diagnostics.
	Path string `json:"-"`
}
