// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package admin

import (
	"context"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

// fakeQueryRunner is the test double for queryRunner. It returns the
// configured rows or error without touching a real Bolt connection.
type fakeQueryRunner struct {
	rows []map[string]any
	err  error
}

func (f *fakeQueryRunner) run(_ context.Context, _ *dbconn.Conn, _ string, _ map[string]any) ([]map[string]any, error) {
	return f.rows, f.err
}

// withFakeRunner replaces adminRunnerFn for the duration of t and restores it
// after. The supplied fakeQueryRunner is returned on every call.
func withFakeRunner(t *testing.T, fake *fakeQueryRunner) {
	t.Helper()
	orig := adminRunnerFn
	adminRunnerFn = func(_ *clicfg.Config) queryRunner { return fake }
	t.Cleanup(func() { adminRunnerFn = orig })
}

func newTestConn() *dbconn.Conn {
	return &dbconn.Conn{
		URI:      "neo4j://localhost:7687",
		Username: "neo4j",
		Password: "password",
	}
}

func newTestCfg() *clicfg.Config {
	return &clicfg.Config{Version: "test"}
}
