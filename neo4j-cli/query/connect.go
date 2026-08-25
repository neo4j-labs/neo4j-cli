// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/config"
	"github.com/neo4j/neo4j-go-driver/v6/neo4j/log"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

// conn holds the resolved Neo4j connection details. The opened Bolt driver
// is attached lazily via openDriver after the password (if any) has been
// resolved or prompted. Callers MUST close the driver via defer once they are
// done using the connection. TLS is selected exclusively by the URI scheme
// (e.g. neo4j+s:// for verified TLS, neo4j+ssc:// for self-signed certs).
type conn struct {
	dbconn.Conn
	driver neo4j.Driver
}

// queryResult is the parsed tabular payload of a successful Cypher run. The
// shape matches what callers (run.go, schema.go) expect: positional rows where
// each row's order matches Columns. Plan and Profile are the driver-free
// EXPLAIN/PROFILE plan trees (mutually exclusive, nil when neither was
// reported) so renderResult can carry them onward.
type queryResult struct {
	Columns []string
	Rows    [][]any
	Stats   *writeStats
	Plan    *planNode
	Profile *planNode
}

// writeStats is a driver-free snapshot of the mutation counters reported by a
// statement's ResultSummary. It is nil for pure reads (statsFromCounters returns
// nil when the counters report no updates) so read output stays unchanged.
type writeStats struct {
	NodesCreated         int `json:"nodes_created,omitempty"`
	NodesDeleted         int `json:"nodes_deleted,omitempty"`
	RelationshipsCreated int `json:"relationships_created,omitempty"`
	RelationshipsDeleted int `json:"relationships_deleted,omitempty"`
	PropertiesSet        int `json:"properties_set,omitempty"`
	LabelsAdded          int `json:"labels_added,omitempty"`
	LabelsRemoved        int `json:"labels_removed,omitempty"`
	IndexesAdded         int `json:"indexes_added,omitempty"`
	IndexesRemoved       int `json:"indexes_removed,omitempty"`
	ConstraintsAdded     int `json:"constraints_added,omitempty"`
	ConstraintsRemoved   int `json:"constraints_removed,omitempty"`
	SystemUpdates        int `json:"system_updates,omitempty"`
}

// statsFromCounters copies a driver Counters into the driver-free writeStats,
// returning nil when the statement made no updates (a pure read) so callers can
// nil-check to decide whether to render a stats line at all. A nil counters
// argument also yields nil.
func statsFromCounters(c neo4j.Counters) *writeStats {
	if c == nil || !c.ContainsUpdates() {
		return nil
	}
	return &writeStats{
		NodesCreated:         c.NodesCreated(),
		NodesDeleted:         c.NodesDeleted(),
		RelationshipsCreated: c.RelationshipsCreated(),
		RelationshipsDeleted: c.RelationshipsDeleted(),
		PropertiesSet:        c.PropertiesSet(),
		LabelsAdded:          c.LabelsAdded(),
		LabelsRemoved:        c.LabelsRemoved(),
		IndexesAdded:         c.IndexesAdded(),
		IndexesRemoved:       c.IndexesRemoved(),
		ConstraintsAdded:     c.ConstraintsAdded(),
		ConstraintsRemoved:   c.ConstraintsRemoved(),
		SystemUpdates:        c.SystemUpdates(),
	}
}

// planNode is a driver-free snapshot of one node in a statement's EXPLAIN or
// PROFILE plan tree, as reported by ResultSummary.Plan()/Profile(). Both plan
// shapes share this tree type: the profile-only metrics (rows, db_hits, time,
// page cache) stay zero for a plain EXPLAIN plan. It is nil when there is no
// plan to report (planNodeFromPlan/planNodeFromProfile return nil for a nil
// driver plan) so callers can nil-check to decide whether to render a plan at
// all.
type planNode struct {
	Operator    string         `json:"operator"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	Identifiers []string       `json:"identifiers,omitempty"`
	Children    []planNode     `json:"children,omitempty"`
	Rows        int64          `json:"rows,omitempty"`
	DbHits      int64          `json:"db_hits,omitempty"`
	// Time is the server's raw nanosecond count for this operator (the driver
	// copies the wire value with no conversion). The table renderer converts to
	// µs; the JSON envelope emits the raw ns int for machine consumers. The root
	// operator's Time is always zero — the driver populates time only for child
	// operators — so omitempty drops it there.
	Time            int64 `json:"time,omitempty"`
	PageCacheHits   int64 `json:"page_cache_hits,omitempty"`
	PageCacheMisses int64 `json:"page_cache_misses,omitempty"`
}

// planNodeFromPlan copies a driver Plan into the driver-free planNode, leaving
// every profile-only metric zero (this is the EXPLAIN shape where the plan was
// never executed so no cost data exists). Recurses over Children() so the whole
// tree is snapshotted. A nil plan yields nil — an EXPLAIN run without a plan
// (or a non-EXPLAIN run) reports nothing.
func planNodeFromPlan(p neo4j.Plan) *planNode {
	if p == nil {
		return nil
	}
	node := &planNode{
		Operator:    p.Operator(),
		Arguments:   p.Arguments(),
		Identifiers: p.Identifiers(),
	}
	for _, child := range p.Children() {
		if c := planNodeFromPlan(child); c != nil {
			node.Children = append(node.Children, *c)
		}
	}
	return node
}

// planNodeFromProfile copies a driver ProfiledPlan into the driver-free
// planNode, carrying the profile-only metrics (rows, db_hits, time, page cache)
// alongside the operator/arguments/identifiers both shapes share. Recurses over
// Children() so the whole tree is snapshotted. A nil profiled plan yields nil —
// a run without profiling reports nothing.
func planNodeFromProfile(p neo4j.ProfiledPlan) *planNode {
	if p == nil {
		return nil
	}
	node := &planNode{
		Operator:        p.Operator(),
		Arguments:       p.Arguments(),
		Identifiers:     p.Identifiers(),
		Rows:            p.Records(),
		DbHits:          p.DbHits(),
		Time:            p.Time(),
		PageCacheHits:   p.PageCacheHits(),
		PageCacheMisses: p.PageCacheMisses(),
	}
	for _, child := range p.Children() {
		if c := planNodeFromProfile(child); c != nil {
			node.Children = append(node.Children, *c)
		}
	}
	return node
}

// planFromResponse snapshots the profile-or-plan tree carried by a response
// into the driver-free planNode, applying the EXPLAIN/PROFILE exclusivity rule
// both statement paths share: a statement profiles XOR explains, so when the
// profiled plan is present the profile field is set and the plan left nil, and
// vice versa. Both come back nil when the statement reported neither (a
// non-EXPLAIN/PROFILE run).
func planFromResponse(resp *queryResponse) (plan, profile *planNode) {
	if resp.Profile != nil {
		return nil, planNodeFromProfile(resp.Profile)
	}
	if resp.Plan != nil {
		return planNodeFromPlan(resp.Plan), nil
	}
	return nil, nil
}

// queryResponse is the structured envelope around a Cypher response. Backed
// by the Bolt driver Result + ResultSummary, it carries the statement's rows,
// bookmarks, and plan trees. QueryType is taken straight from
// ResultSummary.QueryType() and is what the --rw classifier inspects for
// EXPLAIN preflight runs (QueryTypeReadOnly → safe; everything else requires
// --rw). The driver also exposes the equivalent (deprecated)
// StatementType() / StatementTypeReadOnly aliases — we use the QueryType
// names so staticcheck does not flag them.
type queryResponse struct {
	Data struct {
		Fields []string
		Values [][]any
	}
	Bookmarks []string
	QueryType neo4j.QueryType
	Counters  neo4j.Counters
	// Plan and Profile are the raw driver plan trees reported by the
	// ResultSummary. Both stay nil for an ordinary statement; when a plan is
	// present the statement profiled XOR explained, so one is set and the other
	// nil. Snapshot rule in planFromResponse.
	Plan    neo4j.Plan
	Profile neo4j.ProfiledPlan
}

// buildDriverConfigurer returns the closure neo4j.NewDriver applies to its
// default *config.Config. Sets UserAgent when non-empty; attaches the shared
// dbconn.NewStderrLogger at DEBUG level when debug is true; otherwise leaves
// c.Log nil so the driver stays silent. Extracted from driverOpener so tests
// can exercise the wiring against a synthetic *config.Config without touching
// neo4j.NewDriver.
func buildDriverConfigurer(userAgent string, debug bool) func(*config.Config) {
	return func(c *config.Config) {
		if userAgent != "" {
			c.UserAgent = userAgent
		}
		if debug {
			c.Log = dbconn.NewStderrLogger(log.DEBUG)
		}
		// interactive CLI fails fast; the driver's 1m default reads as a hang
		c.ConnectionAcquisitionTimeout = 10 * time.Second
		// interactive CLI fail-fast; default 30s feels like a hang
		c.MaxTransactionRetryTime = 10 * time.Second
	}
}

// driverOpener is the test seam used to construct the Bolt driver. Production
// calls neo4j.NewDriver; tests can swap in a fake to bypass the real bolt://
// connection. When debug is true the configurer attaches dbconn.NewStderrLogger
// at DEBUG level so the driver's wire activity goes to stderr; when false
// c.Log is left at its nil default.
var driverOpener = func(target string, username, password, userAgent string, debug bool) (neo4j.Driver, error) {
	return neo4j.NewDriver(target, neo4j.BasicAuth(username, password, ""), buildDriverConfigurer(userAgent, debug))
}

// runStatementResponseFn is the test seam used by runStatementResponse. It
// lets tests inject canned responses without booting a real Neo4j or
// constructing a Bolt driver. Production sets it to runStatementResponseImpl.
// The readOnly flag selects ExecuteRead vs ExecuteWrite in production; tests
// can assert on it to verify correct routing.
var runStatementResponseFn = runStatementResponseImpl

// runStatementsResponseFn is the batch counterpart of runStatementResponseFn.
// It lets tests inject canned per-statement responses for the single-transaction
// (--atomic) path without booting a real Neo4j. Production sets it to
// runStatementsResponseImpl. The readOnly flag selects ExecuteRead vs
// ExecuteWrite in production; tests can assert on it.
var runStatementsResponseFn = runStatementsResponseImpl

// resolveConn resolves connection parameters from .env, OS environment, and
// command-line flags via dbconn.ResolveConn, then wraps the result in the
// query-local conn type which adds driver management. The returned conn does
// NOT hold an open driver — callers should fill in the password (prompt if
// needed) and then call c.openDriver(ctx) before issuing queries, and defer
// c.driver.Close(ctx) for cleanup.
func resolveConn(cmd *cobra.Command, cfg *clicfg.Config) (*conn, error) {
	base, err := dbconn.ResolveConn(cmd, cfg, false)
	if err != nil {
		return nil, err
	}
	return &conn{Conn: *base}, nil
}

// openDriver opens a Bolt driver using the resolved connection params and
// stores it on c.driver. Idempotent when already opened. Caller is
// responsible for closing the driver via c.driver.Close(ctx) (typically
// `defer`).
//
// Driver-construction errors (connectivity, malformed URI, TLS handshake)
// surface as upstream errors so the process exits with code 8 — they are
// transport-level failures, not user-input validation.
func (c *conn) openDriver() error {
	if c == nil {
		return errors.New("query: nil connection")
	}
	if c.driver != nil {
		return nil
	}
	d, err := driverOpener(c.URI, c.Username, c.Password, c.UserAgent, c.Debug)
	if err != nil {
		return categorizeBoltError(fmt.Errorf("query: open driver: %w", err))
	}
	c.driver = d
	return nil
}

// runStatementResponse executes a Cypher statement against the Bolt driver
// attached to c and returns the parsed envelope (rows + bookmarks + plan).
// Routes through runStatementResponseFn so tests can override. The readOnly
// flag drives ExecuteRead vs ExecuteWrite selection inside the production
// impl; preflight EXPLAIN and read-only execution pass true, post-classified
// write execution passes false.
//
// Driver errors are categorised here (the single dispatch boundary that both
// production and the test seam flow through) so callers further up never have
// to deal with raw Bolt-driver errors: Cypher ClientError-class failures map
// to validation errors (exit 6); transport / TransientError / DatabaseError
// failures map to upstream errors (exit 8).
func runStatementResponse(ctx context.Context, c *conn, statement string, params map[string]any, readOnly bool) (*queryResponse, error) {
	resp, err := runStatementResponseFn(ctx, c, statement, params, readOnly)
	if err != nil {
		return nil, categorizeBoltError(err)
	}
	return resp, nil
}

// runStatementResponseImpl is the single-statement Bolt-backed implementation,
// expressed as a batch-of-one over runStatementsResponseImpl. Delegating to the
// impl (not the categorizing runStatementsResponse wrapper) keeps error
// categorization at the single runStatementResponse boundary — no double-wrap.
// A successful batch-of-one always yields exactly one envelope, so resps[0] is
// safe.
func runStatementResponseImpl(ctx context.Context, c *conn, statement string, params map[string]any, readOnly bool) (*queryResponse, error) {
	resps, err := runStatementsResponseImpl(ctx, c, []string{statement}, params, readOnly)
	if err != nil {
		return nil, err
	}
	return resps[0], nil
}

// runStatement executes a Cypher statement and returns the tabular result.
// Defaults to readOnly=true; callers that classify a statement as a write
// (post-EXPLAIN or with --rw) MUST use runStatementWrite instead.
func runStatement(ctx context.Context, c *conn, statement string, params map[string]any) (*queryResult, error) {
	return runStatementWithMode(ctx, c, statement, params, true)
}

// runStatementWrite executes a Cypher statement on the write path, routing
// through ExecuteWrite. Used by the query run leaf when --rw is set (the
// user has opted in) or when the post-classification path determines the
// statement mutates state.
func runStatementWrite(ctx context.Context, c *conn, statement string, params map[string]any) (*queryResult, error) {
	return runStatementWithMode(ctx, c, statement, params, false)
}

func runStatementWithMode(ctx context.Context, c *conn, statement string, params map[string]any, readOnly bool) (*queryResult, error) {
	parsed, err := runStatementResponse(ctx, c, statement, params, readOnly)
	if err != nil {
		return nil, err
	}

	plan, profile := planFromResponse(parsed)
	return &queryResult{
		Columns: parsed.Data.Fields,
		Rows:    parsed.Data.Values,
		Stats:   statsFromCounters(parsed.Counters),
		Plan:    plan,
		Profile: profile,
	}, nil
}

// runStatementsResponse executes a batch of Cypher statements inside a single
// managed transaction and returns one parsed envelope per statement, in source
// order. Routes through runStatementsResponseFn so tests can override. The
// readOnly flag drives ExecuteRead vs ExecuteWrite selection inside the
// production impl.
//
// Driver errors are categorised here (the single dispatch boundary that both
// production and the test seam flow through), mirroring runStatementResponse:
// Cypher ClientError-class failures map to validation errors (exit 6);
// transport / TransientError / DatabaseError failures map to upstream errors
// (exit 8). An error from any statement aborts the transaction, so the managed
// transaction rolls back automatically and no partial result is returned.
func runStatementsResponse(ctx context.Context, c *conn, statements []string, params map[string]any, readOnly bool) ([]*queryResponse, error) {
	resps, err := runStatementsResponseFn(ctx, c, statements, params, readOnly)
	if err != nil {
		return nil, categorizeBoltError(err)
	}
	return resps, nil
}

// runStatementsResponseImpl is the real Bolt-backed batch implementation. Opens
// ONE session targeted at c.Database and runs ONE managed transaction
// (ExecuteRead when readOnly is true, ExecuteWrite otherwise) whose work
// callback loops tx.Run → Collect → Consume per statement, appending a
// *queryResponse each (reusing coerceDriverValue). Any error returned from the callback aborts the
// transaction, so the driver rolls it back automatically. The session is closed
// via defer; the driver retains pooling.
func runStatementsResponseImpl(ctx context.Context, c *conn, statements []string, params map[string]any, readOnly bool) ([]*queryResponse, error) {
	if c == nil {
		return nil, errors.New("query: nil connection")
	}
	if c.driver == nil {
		return nil, errors.New("query: connection driver not opened (call openDriver first)")
	}

	// An empty c.Database leaves DatabaseName unset so the server resolves the
	// connecting user's home database
	session := c.driver.NewSession(ctx, neo4j.SessionConfig{DatabaseName: c.Database})
	defer session.Close(ctx) //nolint:errcheck // session close error not actionable in defer

	work := func(tx neo4j.ManagedTransaction) (any, error) {
		responses := make([]*queryResponse, 0, len(statements))
		for _, statement := range statements {
			result, err := tx.Run(ctx, statement, params)
			if err != nil {
				return nil, err
			}

			records, err := result.Collect(ctx)
			if err != nil {
				return nil, err
			}

			summary, err := result.Consume(ctx)
			if err != nil {
				return nil, err
			}

			resp := &queryResponse{}
			if len(records) > 0 {
				resp.Data.Fields = append([]string(nil), records[0].Keys...)
				resp.Data.Values = make([][]any, 0, len(records))
				for _, rec := range records {
					row := make([]any, len(rec.Values))
					for i, v := range rec.Values {
						row[i] = coerceDriverValue(v)
					}
					resp.Data.Values = append(resp.Data.Values, row)
				}
			} else {
				keys, _ := result.Keys()
				resp.Data.Fields = append([]string(nil), keys...)
				resp.Data.Values = [][]any{}
			}

			if summary != nil {
				resp.QueryType = summary.QueryType()
				resp.Counters = summary.Counters()
				resp.Plan = summary.Plan()
				resp.Profile = summary.Profile()
			}

			responses = append(responses, resp)
		}
		return responses, nil
	}

	var (
		out any
		err error
	)
	if readOnly {
		out, err = session.ExecuteRead(ctx, work)
	} else {
		out, err = session.ExecuteWrite(ctx, work)
	}
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	resps, ok := out.([]*queryResponse)
	if !ok {
		return nil, errors.New("query: unexpected nil response from managed transaction")
	}
	return resps, nil
}

// runStatementsWithMode executes a batch of statements in a single managed
// transaction and unwraps the envelopes into []*queryResult (Columns/Rows),
// mirroring runStatementWithMode. Results are returned in source order.
func runStatementsWithMode(ctx context.Context, c *conn, statements []string, params map[string]any, readOnly bool) ([]*queryResult, error) {
	parsed, err := runStatementsResponse(ctx, c, statements, params, readOnly)
	if err != nil {
		return nil, err
	}

	results := make([]*queryResult, 0, len(parsed))
	for _, p := range parsed {
		plan, profile := planFromResponse(p)
		results = append(results, &queryResult{
			Columns: p.Data.Fields,
			Rows:    p.Data.Values,
			Stats:   statsFromCounters(p.Counters),
			Plan:    plan,
			Profile: profile,
		})
	}
	return results, nil
}
