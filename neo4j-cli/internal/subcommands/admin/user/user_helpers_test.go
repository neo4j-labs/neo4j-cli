// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package user_test

import (
	"context"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	. "github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/user"
)

// fakeExecFn is the test-double type for adminutil.ExecFn. Tests construct a
// value of this type to control the rows/error returned by userExecFn.
type fakeExecFn func(ctx context.Context, cfg *clicfg.Config, conn *dbconn.Conn, cypher string, params map[string]any) ([]map[string]any, error) //nolint:unused

// withFakeExecFn replaces userExecFn for the duration of t with fake and
// restores the original value in t.Cleanup.
func withFakeExecFn(t *testing.T, fake fakeExecFn) { //nolint:unused
	t.Helper()
	orig := *ExportedUserExecFn
	*ExportedUserExecFn = adminutil.ExecFn(fake)
	t.Cleanup(func() { *ExportedUserExecFn = orig })
}

// withFakeStdinIsTTY replaces dbconn.StdinIsTTY for the duration of t with
// the supplied value and restores the original in t.Cleanup.
func withFakeStdinIsTTY(t *testing.T, isTTY bool) { //nolint:unused
	t.Helper()
	orig := dbconn.StdinIsTTY
	dbconn.StdinIsTTY = func() bool { return isTTY }
	t.Cleanup(func() { dbconn.StdinIsTTY = orig })
}

// withFakePasswordReader replaces dbconn.PasswordReader for the duration of t
// with a function that returns pw (and no error) and restores the original in
// t.Cleanup.
func withFakePasswordReader(t *testing.T, pw string, err error) { //nolint:unused
	t.Helper()
	orig := dbconn.PasswordReader
	dbconn.PasswordReader = func() (string, error) { return pw, err }
	t.Cleanup(func() { dbconn.PasswordReader = orig })
}

// testConn returns a *dbconn.Conn for use in tests. The connection params are
// never used because tests always override userExecFn with a fake.
func testConn() *dbconn.Conn { //nolint:unused
	return &dbconn.Conn{
		URI:      "neo4j://localhost:7687",
		Username: "neo4j",
		Password: "test",
	}
}
