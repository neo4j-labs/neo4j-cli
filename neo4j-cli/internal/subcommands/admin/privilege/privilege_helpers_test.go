// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"context"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

// testConn returns a *dbconn.Conn for use in tests. The connection params are
// never used because tests always override privilegeExecFn with a fake.
//
//nolint:unused // consumed by leaf command tests added in later tasks
func testConn() *dbconn.Conn {
	return &dbconn.Conn{
		URI:      "neo4j://localhost:7687",
		Username: "neo4j",
		Password: "test",
	}
}

// withFakePrivilegeExecFn replaces privilegeExecFn for the duration of t with a
// fake that always returns the supplied rows or error.
//
//nolint:unused // consumed by leaf command tests added in later tasks
func withFakePrivilegeExecFn(t *testing.T, rows []map[string]any, execErr error) {
	t.Helper()
	orig := privilegeExecFn
	privilegeExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		return rows, execErr
	}
	t.Cleanup(func() { privilegeExecFn = orig })
}

// withSequencedPrivilegeExecFn replaces privilegeExecFn with a sequenced fake
// that returns responses in the order provided. It calls t.Fatalf if the exec
// function is called more times than there are responses. The original is
// restored via t.Cleanup.
//
//nolint:unused // consumed by leaf command tests added in later tasks
func withSequencedPrivilegeExecFn(t *testing.T, responses []struct {
	rows []map[string]any
	err  error
}) {
	t.Helper()
	orig := privilegeExecFn
	call := 0
	privilegeExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
		if call >= len(responses) {
			t.Fatalf("privilegeExecFn called %d times but only %d response(s) were provided", call+1, len(responses))
		}
		r := responses[call]
		call++
		return r.rows, r.err
	}
	t.Cleanup(func() { privilegeExecFn = orig })
}
