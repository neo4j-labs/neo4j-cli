// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbconn

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/afero"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
)

const desktopCredentialPrefix = "desktop"

const desktopConnectionPrefix = "desktop-connection:"

var resolveDesktopActiveDbmsCredentialFn = resolveDesktopActiveDbmsCredential

var resolveDesktopConnectionCredentialFn = resolveDesktopConnectionCredential

// desktopMatch carries the result of a successful Desktop credential lookup.
// Exactly one of dbms / connection is non-nil in any successful result.
// creds is nil when Desktop returned a null credentials response — null-creds
// handling lives in finishDesktopMatch (prompt on TTY / fatal on non-TTY).
type desktopMatch struct {
	dbms       *desktopclient.DbmsInfo
	connection *desktopclient.Connection
	creds      *desktopclient.Credentials
}

// resolveDesktopActiveDbmsCredential resolves `--credential desktop` to the
// single running Desktop DBMS. Filters by status == "started".
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
// chain. Returns (nil, nil) when Desktop is not present so callers map the
// absence to a single unreachable error.
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

// buildConnFromDesktopMatch turns a successful desktopMatch into the *Conn
// shape ResolveConn returns.
func buildConnFromDesktopMatch(m *desktopMatch, version string, debug bool) *Conn {
	if version == "" {
		version = "dev"
	}
	uri := desktopMatchURI(m)
	if uri == "" {
		uri = DefaultURI
	}
	username := DefaultUsername
	password := ""
	if m.creds != nil {
		if m.creds.Username != "" {
			username = m.creds.Username
		}
		password = m.creds.Password
	}
	return &Conn{
		URI:       uri,
		Username:  username,
		Password:  password,
		Database:  DefaultDatabase,
		UserAgent: "neo4j-cli/v" + version,
		Debug:     debug,
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
