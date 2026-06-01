// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
)

const desktopCredentialPrefix = "desktop"

const desktopConnectionPrefix = "desktop-connection:"

// Return shape mirrors the legacy seam so the null-creds path in resolveConn
// can be reused unchanged.
var resolveDesktopActiveDbmsCredentialFn = resolveDesktopActiveDbmsCredential

var resolveDesktopConnectionCredentialFn = resolveDesktopConnectionCredential

// desktopMatch carries the result of a successful Desktop credential lookup.
// Exactly one of dbms / connection is non-nil in any successful result.
// creds is nil when Desktop's `GET /credentials/<key>` returned the JSON
// literal `null` — null-creds handling lives in resolveConn (prompt on TTY /
// fatal on non-TTY) and applies to both prefix forms.
type desktopMatch struct {
	dbms       *desktopclient.DbmsInfo
	connection *desktopclient.Connection
	creds      *desktopclient.Credentials
}

// resolveDesktopActiveDbmsCredential resolves `--credential desktop` to the
// single running Desktop DBMS.
//
// Filters `GET /dbmss/info` by `status == "started"` — we must hit the
// `/info` endpoint, NOT plain `/dbmss`, because the lightweight `/dbmss`
// response shape omits the `status` field; filtering against `/dbmss` would
// silently match zero DBMSes every time. The >1 branch is defensive —
// relate's design guarantees ≤1 running.
func resolveDesktopActiveDbmsCredential(ctx context.Context, fs afero.Fs) (*desktopMatch, error) {
	client, err := newDesktopFallthroughClient(ctx, fs)
	if err != nil || client == nil {
		return nil, desktopclient.UnreachableError()
	}

	list, err := client.ListDbmssInfo(ctx)
	if err != nil {
		return nil, err
	}

	running := make([]desktopclient.DbmsInfo, 0, len(list))
	for i := range list {
		if list[i].Status == "started" {
			running = append(running, list[i])
		}
	}

	switch len(running) {
	case 0:
		return nil, clierr.NewFatalError(
			"No running DBMS in Neo4j Desktop 2. Start one with 'neo4j-cli desktop dbms start <id>'.")
	case 1:
		dbms := running[0]
		creds, err := client.GetCredentialsByKey(ctx, "dbms:"+dbms.ID)
		if err != nil {
			return nil, err
		}
		return &desktopMatch{dbms: &dbms, creds: creds}, nil
	default:
		ids := make([]string, 0, len(running))
		for _, d := range running {
			ids = append(ids, d.ID)
		}
		return nil, clierr.NewFatalError(
			"Multiple running DBMSes reported by Neo4j Desktop 2 (%s). "+
				"Stop all but one, or pick a saved connection with --credential desktop-connection:<id>.",
			strings.Join(ids, ", "))
	}
}

// resolveDesktopConnectionCredential resolves
// `--credential desktop-connection:<uuid>` to a specific saved Desktop
// connection. Caller has already stripped the `desktop-connection:` prefix.
func resolveDesktopConnectionCredential(ctx context.Context, fs afero.Fs, raw string) (*desktopMatch, error) {
	if _, err := uuid.Parse(raw); err != nil {
		return nil, clierr.NewUsageError(
			"--credential desktop-connection:<id> requires a UUID; got %q. "+
				"Run 'neo4j-cli desktop list' to see connection ids.", raw)
	}

	client, err := newDesktopFallthroughClient(ctx, fs)
	if err != nil || client == nil {
		return nil, desktopclient.UnreachableError()
	}

	list, err := client.ListConnections(ctx)
	if err != nil {
		return nil, err
	}

	var match *desktopclient.Connection
	for i := range list {
		if list[i].ID == raw {
			match = &list[i]
			break
		}
	}
	if match == nil {
		return nil, clierr.NewFatalError(
			"Neo4j Desktop 2 has no connection with id %s. "+
				"Run 'neo4j-cli desktop list' to see saved connections.", raw)
	}

	creds, err := client.GetCredentialsByKey(ctx, "connection:"+raw)
	if err != nil {
		return nil, err
	}
	return &desktopMatch{connection: match, creds: creds}, nil
}

// newDesktopFallthroughClient bundles the discovery → data-dir → salt → client
// chain that both Desktop prefix resolvers begin with. Returns (nil, nil)
// for every "Desktop is just not here" failure so callers can map the
// absence to a single unreachable error rather than distinguishing each
// kind. Discover runs first so its origin can be threaded into
// ResolveDataDir for the /info/app discovery step.
func newDesktopFallthroughClient(ctx context.Context, fs afero.Fs) (*desktopclient.Client, error) {
	probe, err := desktopclient.Discover(ctx, 0)
	if err != nil {
		if errors.Is(err, desktopclient.ErrNoDesktop) {
			return nil, nil
		}
		return nil, nil
	}
	dataDir, err := desktopclient.ResolveDataDir(ctx, fs, probe)
	if err != nil {
		return nil, nil
	}
	salt, err := desktopclient.LoadSalt(fs, dataDir)
	if err != nil {
		return nil, nil
	}
	client, err := desktopclient.NewClient(probe, salt)
	if err != nil {
		return nil, nil
	}
	return client, nil
}

// SetResolveDesktopActiveDbmsCredentialFnForTest overrides the
// `--credential desktop` resolver seam and returns a restore func.
func SetResolveDesktopActiveDbmsCredentialFnForTest(fn func(context.Context, afero.Fs) (*desktopMatch, error)) func() {
	prev := resolveDesktopActiveDbmsCredentialFn
	resolveDesktopActiveDbmsCredentialFn = fn
	return func() { resolveDesktopActiveDbmsCredentialFn = prev }
}

// SetResolveDesktopConnectionCredentialFnForTest overrides the
// `--credential desktop-connection:<uuid>` resolver seam and returns a
// restore func.
func SetResolveDesktopConnectionCredentialFnForTest(fn func(context.Context, afero.Fs, string) (*desktopMatch, error)) func() {
	prev := resolveDesktopConnectionCredentialFn
	resolveDesktopConnectionCredentialFn = fn
	return func() { resolveDesktopConnectionCredentialFn = prev }
}

// buildConnFromDesktopMatch turns a successful desktopMatch into the *conn
// shape resolveConn returns. Callers can mutate the returned *conn's
// password field before openDriver runs (prompted-password path).
func buildConnFromDesktopMatch(m *desktopMatch, cfg *clicfg.Config, cmd *cobra.Command) *conn {
	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	uri := desktopMatchURI(m)
	if uri == "" {
		// Desktop normally returns connectionUri for an online DBMS; an
		// offline / partially-started DBMS may omit it. Fall back to the
		// default localhost Bolt URI so the user's next step (start the
		// DBMS) produces a recognisable connection error rather than a
		// "missing URI" message.
		uri = defaultURI
	}
	username := defaultUsername
	password := ""
	if m.creds != nil {
		if m.creds.Username != "" {
			username = m.creds.Username
		}
		password = m.creds.Password
	}
	return &conn{
		uri:       uri,
		username:  username,
		password:  password,
		database:  defaultDatabase,
		userAgent: "neo4j-cli/v" + version,
		debug:     resolveDebug(cmd),
	}
}

// desktopMatchURI returns the ConnectionURI of whichever side of the match
// union is populated.
func desktopMatchURI(m *desktopMatch) string {
	if m == nil {
		return ""
	}
	if m.dbms != nil {
		return m.dbms.ConnectionURI
	}
	if m.connection != nil {
		return m.connection.ConnectionURI
	}
	return ""
}

// desktopMatchIdentity returns the name and id of whichever side of the
// match union is populated.
func desktopMatchIdentity(m *desktopMatch) (name, id string) {
	if m == nil {
		return "", ""
	}
	if m.dbms != nil {
		return m.dbms.Name, m.dbms.ID
	}
	if m.connection != nil {
		return m.connection.Name, m.connection.ID
	}
	return "", ""
}
