// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive runStatementsResponseImpl directly (not the
// runStatementsResponseFn seam) because the routing contract under test is
// internal to the impl: a single read-only statement must run in auto-commit
// mode (session.Run) while multi-statement batches and writes must keep the
// managed transaction (ExecuteRead/ExecuteWrite). The spies below embed the
// nil driver interfaces so any method the impl calls beyond the ones
// implemented here panics loudly — the same convention as noopDriver.

// spyDriver hands every NewSession the same spySession.
type spyDriver struct {
	neo4j.Driver
	session *spySession
}

func (d *spyDriver) NewSession(context.Context, neo4j.SessionConfig) neo4j.Session {
	return d.session
}

func (d *spyDriver) Close(context.Context) error { return nil }

// spySession records which execution mode was used. Run serves auto-commit
// statements; ExecuteRead/ExecuteWrite record the call and run the work
// callback against a spyTransaction so the managed path stays observable.
type spySession struct {
	neo4j.Session
	runCalls          []string
	executeReadCalls  int
	executeWriteCalls int
	txRunCalls        []string
}

func (s *spySession) Run(_ context.Context, cypher string, _ map[string]any, _ ...func(*neo4j.TransactionConfig)) (neo4j.Result, error) {
	s.runCalls = append(s.runCalls, cypher)
	return &fakeResult{}, nil
}

func (s *spySession) ExecuteRead(ctx context.Context, work neo4j.ManagedTransactionWork, _ ...func(*neo4j.TransactionConfig)) (any, error) {
	s.executeReadCalls++
	return work(&spyTransaction{session: s})
}

func (s *spySession) ExecuteWrite(ctx context.Context, work neo4j.ManagedTransactionWork, _ ...func(*neo4j.TransactionConfig)) (any, error) {
	s.executeWriteCalls++
	return work(&spyTransaction{session: s})
}

func (s *spySession) Close(context.Context) error { return nil }

// spyTransaction is the ManagedTransaction handed to managed-path work
// callbacks; its Run delegates back to the session's fakeResult and records
// the statement.
type spyTransaction struct {
	neo4j.ManagedTransaction
	session *spySession
}

func (t *spyTransaction) Run(_ context.Context, cypher string, _ map[string]any) (neo4j.Result, error) {
	t.session.txRunCalls = append(t.session.txRunCalls, cypher)
	return &fakeResult{}, nil
}

// fakeResult serves one record with a single column. The embedded nil
// neo4j.Result satisfies the interface; only Collect/Consume/Keys are called
// by runOneQueryResponse.
type fakeResult struct {
	neo4j.Result
}

func (r *fakeResult) Collect(context.Context) ([]*neo4j.Record, error) {
	return []*neo4j.Record{{Keys: []string{"n"}, Values: []any{int64(1)}}}, nil
}

func (r *fakeResult) Consume(context.Context) (neo4j.ResultSummary, error) {
	return nil, nil
}

func (r *fakeResult) Keys() ([]string, error) {
	return []string{"n"}, nil
}

func newSpyConn() (*conn, *spySession) {
	session := &spySession{}
	return &conn{driver: &spyDriver{session: session}}, session
}

func TestRunStatementsResponseImpl_SingleReadOnlyUsesAutoCommit(t *testing.T) {
	c, session := newSpyConn()

	resps, err := runStatementsResponseImpl(context.Background(), c, []string{"RETURN 1"}, nil, true)
	require.NoError(t, err)
	require.Len(t, resps, 1)
	assert.Equal(t, []string{"n"}, resps[0].Data.Fields)
	assert.Equal(t, [][]any{{int64(1)}}, resps[0].Data.Values)

	assert.Equal(t, []string{"RETURN 1"}, session.runCalls, "single read must run via auto-commit session.Run")
	assert.Zero(t, session.executeReadCalls, "single read must not open a managed transaction")
	assert.Zero(t, session.executeWriteCalls)
	assert.Empty(t, session.txRunCalls)
}

func TestRunStatementsResponseImpl_SingleWriteKeepsManagedTransaction(t *testing.T) {
	c, session := newSpyConn()

	_, err := runStatementsResponseImpl(context.Background(), c, []string{"CREATE (n)"}, nil, false)
	require.NoError(t, err)

	assert.Empty(t, session.runCalls, "write path must not use auto-commit")
	assert.Equal(t, 1, session.executeWriteCalls, "single write must keep ExecuteWrite (transient-error retry)")
	assert.Equal(t, []string{"CREATE (n)"}, session.txRunCalls)
}

func TestRunStatementsResponseImpl_MultiStatementKeepsManagedTransaction(t *testing.T) {
	c, session := newSpyConn()

	_, err := runStatementsResponseImpl(context.Background(), c, []string{"RETURN 1", "RETURN 2"}, nil, true)
	require.NoError(t, err)

	assert.Empty(t, session.runCalls, "multi-statement reads must keep the managed transaction (--atomic semantics)")
	assert.Equal(t, 1, session.executeReadCalls)
	assert.Equal(t, []string{"RETURN 1", "RETURN 2"}, session.txRunCalls)
}
