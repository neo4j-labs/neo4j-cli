// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package database

import (
	"context"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

// testConn returns a *dbconn.Conn for use in tests. The connection params are
// never used because tests always override dbExecFn with a fake.
func testConn() *dbconn.Conn {
	return &dbconn.Conn{
		URI:      "neo4j://localhost:7687",
		Username: "neo4j",
		Password: "test",
	}
}

// withFakeExecFn replaces dbExecFn for the duration of t with a fake that
// returns the supplied rows or error.
func withFakeExecFn(t *testing.T, rows []map[string]any, execErr error) {
	t.Helper()
	orig := dbExecFn
	dbExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		return rows, execErr
	}
	t.Cleanup(func() { dbExecFn = orig })
}
