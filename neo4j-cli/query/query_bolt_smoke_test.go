// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

//go:build !windows

package query

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

const (
	boltContainerName = "neo4j-bolt-test"
	boltImage         = "neo4j:latest"
	boltPassword      = "testtest"
	boltReadyTimeout  = 60 * time.Second
)

// TestBolt_Smoke is an env-gated integration test that boots a real
// neo4j:latest container with the Bolt port mapped to a random free port and
// asserts that summary.QueryType() correctly classifies read, write, and
// schema-write Cypher via EXPLAIN preflight. Skipped by default so
// `go test ./...` is unaffected; opt in with NEO4J_BOLT_TEST=1.
//
// Requires: docker on PATH.
//
// Build constraint: Unix-only (Linux container model).
func TestBolt_Smoke(t *testing.T) {
	if os.Getenv("NEO4J_BOLT_TEST") != "1" {
		t.Skip("set NEO4J_BOLT_TEST=1 to run; needs docker")
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatalf("docker is required on PATH but was not found: %v", err)
	}

	boltPort := freeBoltPort(t)

	// Pre-clean any leftover container from a crashed previous run.
	_ = exec.Command("docker", "rm", "-f", boltContainerName).Run()

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", boltContainerName).Run()
	})

	bootNeo4jBolt(t, boltPort)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	uri := fmt.Sprintf("neo4j://127.0.0.1:%d", boltPort)
	c := &conn{
		Conn: dbconn.Conn{
			URI:       uri,
			Username:  "neo4j",
			Password:  boltPassword,
			Database:  "neo4j",
			UserAgent: "neo4j-cli/bolt-smoke",
		},
	}

	require.NoError(t, waitForBoltReady(ctx, c), "neo4j Bolt endpoint did not become ready")
	defer c.driver.Close(ctx) //nolint:errcheck // close error not actionable in defer

	cases := []struct {
		name      string
		cypher    string
		queryType neo4j.QueryType
	}{
		{
			name:      "read EXPLAIN classifies as ReadOnly",
			cypher:    "EXPLAIN MATCH (n) RETURN n LIMIT 1",
			queryType: neo4j.QueryTypeReadOnly,
		},
		{
			name:      "write EXPLAIN classifies as ReadWrite",
			cypher:    "EXPLAIN CREATE (n:Tmp) RETURN n",
			queryType: neo4j.QueryTypeReadWrite,
		},
		{
			name:      "schema EXPLAIN classifies as SchemaWrite",
			cypher:    "EXPLAIN CREATE INDEX bolt_smoke_idx FOR (n:Tmp) ON (n.id)",
			queryType: neo4j.QueryTypeSchemaWrite,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := runStatementResponse(ctx, c, tc.cypher, nil, true)
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tc.queryType, resp.QueryType,
				"unexpected QueryType for cypher %q", tc.cypher)
		})
	}

	// The batch path (runStatementsResponseImpl) is the real single-session,
	// single-managed-transaction implementation that --atomic uses. Unit tests
	// stub it via the seam, so these subtests are the only coverage that the
	// transaction actually spans all statements and rolls back as a unit.

	t.Run("batch returns one result set per statement in order", func(t *testing.T) {
		results, err := runStatementsWithMode(ctx, c, []string{"RETURN 1 AS a", "RETURN 2 AS b"}, nil, true)
		require.NoError(t, err)
		require.Len(t, results, 2)
		assert.Equal(t, []string{"a"}, results[0].Columns)
		require.Len(t, results[0].Rows, 1)
		assert.Equal(t, int64(1), results[0].Rows[0][0])
		assert.Equal(t, []string{"b"}, results[1].Columns)
		require.Len(t, results[1].Rows, 1)
		assert.Equal(t, int64(2), results[1].Rows[0][0])
	})

	t.Run("atomic batch rolls back earlier writes when a later statement fails", func(t *testing.T) {
		_, err := runStatementsWithMode(ctx, c,
			[]string{"CREATE (:RollbackProbe)", "RETURN this_is_not_a_valid_variable"}, nil, false)
		require.Error(t, err, "an invalid second statement must fail the batch")

		check, err := runStatementsWithMode(ctx, c,
			[]string{"MATCH (n:RollbackProbe) RETURN count(n) AS c"}, nil, true)
		require.NoError(t, err)
		require.Len(t, check, 1)
		require.Len(t, check[0].Rows, 1)
		assert.Equal(t, int64(0), check[0].Rows[0][0],
			"the CREATE in a failed atomic batch must be rolled back")
	})

	t.Run("atomic batch commits all writes on success", func(t *testing.T) {
		t.Cleanup(func() {
			_, _ = runStatementsWithMode(ctx, c, []string{"MATCH (n:CommitProbe) DETACH DELETE n"}, nil, false)
		})
		_, err := runStatementsWithMode(ctx, c,
			[]string{"CREATE (:CommitProbe {n: 1})", "CREATE (:CommitProbe {n: 2})"}, nil, false)
		require.NoError(t, err)

		check, err := runStatementsWithMode(ctx, c,
			[]string{"MATCH (n:CommitProbe) RETURN count(n) AS c"}, nil, true)
		require.NoError(t, err)
		require.Len(t, check, 1)
		require.Len(t, check[0].Rows, 1)
		assert.Equal(t, int64(2), check[0].Rows[0][0],
			"both CREATEs in a successful atomic batch must commit")
	})
}

// freeBoltPort allocates and immediately releases a TCP port on 127.0.0.1.
// The tiny TOCTOU window between Close and `docker run -p` is acceptable for
// a dev-only smoke test.
func freeBoltPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

// bootNeo4jBolt starts neo4j:latest detached with only the Bolt port (7687)
// published to the chosen host port. HTTPS/HTTP remain inside the container.
func bootNeo4jBolt(t *testing.T, boltPort int) {
	t.Helper()
	out, err := exec.Command("docker", "run", "-d", "--rm",
		"--name", boltContainerName,
		"-p", fmt.Sprintf("%d:7687", boltPort),
		"-e", "NEO4J_AUTH=neo4j/"+boltPassword,
		boltImage,
	).CombinedOutput()
	require.NoError(t, err, "docker run failed: %s", string(out))
}

// waitForBoltReady opens a Bolt driver and probes connectivity until the
// container accepts queries or the deadline elapses. On success the conn's
// driver field is left open for the test to use; the caller is responsible
// for closing it via defer.
func waitForBoltReady(ctx context.Context, c *conn) error {
	deadline := time.Now().Add(boltReadyTimeout)
	for time.Now().Before(deadline) {
		if c.driver == nil {
			if err := c.openDriver(); err != nil {
				time.Sleep(1 * time.Second)
				continue
			}
		}

		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := c.driver.VerifyConnectivity(probeCtx)
		cancel()
		if err == nil {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	logs, _ := exec.Command("docker", "logs", "--tail", "50", boltContainerName).CombinedOutput()
	return fmt.Errorf("neo4j bolt not ready within %s; last 50 log lines:\n%s", boltReadyTimeout, string(logs))
}
