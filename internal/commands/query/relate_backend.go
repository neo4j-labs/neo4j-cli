// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"errors"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/relate"
)

// relateTarget reports whether this invocation selects the embedded Relate
// backend: the --relate flag was passed a non-empty value.
func relateTarget(cmd *cobra.Command) bool {
	f := cmd.Flag("relate")
	return f != nil && f.Changed && f.Value.String() != ""
}

// resolveRelateConn validates flag exclusivity and opens the embedded store
// selected by --relate, returning a conn whose relateStore carries execution
// (no Bolt driver is opened; no password is resolved or prompted). The store
// is created when its path is missing.
func resolveRelateConn(cmd *cobra.Command, cfg *clicfg.Config) (*conn, error) {
	if err := rejectConflictingConnFlags(cmd); err != nil {
		return nil, err
	}
	if relate.OpenStore == nil {
		return nil, relate.NotCompiledError("--relate")
	}
	path, _ := cmd.Flags().GetString("relate")
	namespace, _ := cmd.Flags().GetString("relate-namespace")
	store, err := relate.OpenStore(relate.ResolveStorePath(path), namespace)
	if err != nil {
		return nil, categorizeRelateError(err)
	}
	return &conn{relateStore: store}, nil
}

// rejectConflictingConnFlags enforces the mutual exclusion between --relate
// and the server-connection flags. --relate targets a local embedded store:
// there is no server, no credential, and no remote database name (the store
// is a path; -d stays the remote database name for Bolt and is meaningless
// here).
func rejectConflictingConnFlags(cmd *cobra.Command) error {
	conflicting := []string{}
	for _, name := range []string{"uri", "username", "password", "database", "credential"} {
		if f := cmd.Flag(name); f != nil && f.Changed {
			conflicting = append(conflicting, "--"+name)
		}
	}
	if len(conflicting) > 0 {
		return clierr.NewUsageError(
			"--relate cannot be used together with %s: --relate targets a local embedded "+
				"Relate store (a path), not a server connection",
			strings.Join(conflicting, ", "))
	}
	return nil
}

// runRelateBatch executes statements through the embedded store and shapes
// each result into the same queryResponse envelope the Bolt path produces,
// so every downstream consumer (render layer, --rw classifier, plan
// renderer) is shared unchanged.
//
// Atomicity: a multi-statement batch only ever arrives here from the
// --atomic path (the default mode executes statements one at a time), so
// len(statements) > 1 selects the store's single-transaction mode. A
// single-statement run uses auto-commit — for one statement the two are
// semantically identical.
//
// Durability: when a statement other than an EXPLAIN preflight mutated the
// store, a checkpoint (snapshot + WAL recycle) is written before returning,
// mirroring the Relate CLI's own durability behavior.
func runRelateBatch(c *conn, statements []string, params map[string]any) ([]*queryResponse, error) {
	results, err := c.relateStore.Run(statements, params, len(statements) > 1)
	if err != nil {
		return nil, err
	}

	resps := make([]*queryResponse, 0, len(results))
	wrote := false
	for i, r := range results {
		resp := &queryResponse{
			QueryType: mapRelateQueryType(r.QueryType),
			Counters:  relateCounters{stats: r.Stats},
		}
		resp.Data.Fields = r.Columns
		resp.Data.Values = r.Rows
		if r.Bookmark != "" {
			resp.Bookmarks = []string{r.Bookmark}
		}
		if r.Plan != nil {
			if r.Plan.Actual != nil {
				resp.Profile = relateProfiledPlan{relateExplainPlan{p: r.Plan}}
			} else {
				resp.Plan = relateExplainPlan{p: r.Plan}
			}
		}
		resps = append(resps, resp)

		// EXPLAIN reports the wrapped statement's type without executing, so
		// an EXPLAIN-prefixed statement (the --rw preflight) must not count
		// as a write. PROFILE does execute and counts.
		if r.QueryType != relate.QueryTypeRead && !isExplainPrefixed(statements[i]) {
			wrote = true
		}
	}
	if wrote {
		if err := c.relateStore.Checkpoint(); err != nil {
			return nil, err
		}
	}
	return resps, nil
}

// isExplainPrefixed reports whether the statement text begins with the
// EXPLAIN keyword (case-insensitive, word-boundary tolerant via the
// surrounding whitespace trim — the same lenience executionMode applies).
func isExplainPrefixed(stmt string) bool {
	mode, _ := executionMode(stmt)
	return mode == "EXPLAIN"
}

// relateCounters adapts the engine's mutation counters onto the driver's
// Counters interface so statsFromCounters snapshots them exactly as it does a
// Bolt summary — the embedded path then shares the whole stats surface (the
// table-mode "N nodes created, …" lines and the JSON/TOON envelope's stats
// object) with no backend-specific rendering. The engine reports the canonical
// 11 counters with zeros included on every result; it has no system-database
// updates, so the system counters stay zero. An all-zero result (a read, an
// EXPLAIN, a no-op write) reports ContainsUpdates() == false, which
// statsFromCounters turns into a nil writeStats so read output stays
// byte-identical with the Bolt path.
type relateCounters struct {
	stats relate.StatementStats
}

func (c relateCounters) ContainsUpdates() bool {
	return c.NodesCreated() > 0 || c.NodesDeleted() > 0 ||
		c.RelationshipsCreated() > 0 || c.RelationshipsDeleted() > 0 ||
		c.PropertiesSet() > 0 || c.LabelsAdded() > 0 || c.LabelsRemoved() > 0 ||
		c.IndexesAdded() > 0 || c.IndexesRemoved() > 0 ||
		c.ConstraintsAdded() > 0 || c.ConstraintsRemoved() > 0
}
func (c relateCounters) ContainsSystemUpdates() bool { return false }
func (c relateCounters) SystemUpdates() int          { return 0 }

func (c relateCounters) NodesCreated() int         { return int(c.stats.NodesCreated) }
func (c relateCounters) NodesDeleted() int         { return int(c.stats.NodesDeleted) }
func (c relateCounters) RelationshipsCreated() int { return int(c.stats.RelationshipsCreated) }
func (c relateCounters) RelationshipsDeleted() int { return int(c.stats.RelationshipsDeleted) }
func (c relateCounters) PropertiesSet() int        { return int(c.stats.PropertiesSet) }
func (c relateCounters) LabelsAdded() int          { return int(c.stats.LabelsAdded) }
func (c relateCounters) LabelsRemoved() int        { return int(c.stats.LabelsRemoved) }
func (c relateCounters) IndexesAdded() int         { return int(c.stats.IndexesAdded) }
func (c relateCounters) IndexesRemoved() int       { return int(c.stats.IndexesRemoved) }
func (c relateCounters) ConstraintsAdded() int     { return int(c.stats.ConstraintsAdded) }
func (c relateCounters) ConstraintsRemoved() int   { return int(c.stats.ConstraintsRemoved) }

// mapRelateQueryType converts the engine's statement classification onto the
// driver's QueryType vocabulary the --rw classifier compares against:
// a read stays ReadOnly and everything else (w, rw, s) is not ReadOnly, which
// is the only distinction the classifier makes.
func mapRelateQueryType(qt relate.QueryType) neo4j.QueryType {
	switch qt {
	case relate.QueryTypeRead:
		return neo4j.QueryTypeReadOnly
	case relate.QueryTypeWrite:
		return neo4j.QueryTypeWriteOnly
	case relate.QueryTypeSchema:
		return neo4j.QueryTypeSchemaWrite
	default:
		return neo4j.QueryTypeReadWrite
	}
}

// categorizeQueryError dispatches error classification by backend: engine
// failures from the embedded store get the Relate-specific mapping, anything
// else keeps the Bolt mapping. It is applied at the same single boundary the
// Bolt path classifies at (runStatementResponse / runStatementsResponse).
func categorizeQueryError(c *conn, err error) error {
	if c != nil && c.relateStore != nil {
		return categorizeRelateError(err)
	}
	return categorizeBoltError(err)
}

// categorizeRelateError wraps an embedded-engine failure into a typed
// *CLIError, mirroring categorizeBoltError's shape:
//   - an *relate.EngineError with a Neo.ClientError.* code is a validation
//     error (exit 6) — the statement was rejected;
//   - a Retryable engine error (write-write conflict) is an upstream error
//     (exit 8) with re-run guidance;
//   - a StageInternal failure is an upstream error with close-and-reopen
//     guidance (a broken handle, a caught panic, an undecodable argument);
//   - any other engine error is a plain upstream error (exit 8);
//   - a non-engine error (the boundary's pre-statement refusals: interior
//     NULs, unsupported parameter types) is a validation error (exit 6).
func categorizeRelateError(err error) error {
	if err == nil {
		return nil
	}
	var ce *clierr.CLIError
	if errors.As(err, &ce) {
		return err
	}
	var ee *relate.EngineError
	if errors.As(err, &ee) {
		switch {
		case ee.Stage == relate.StageInternal:
			return clierr.NewUpstreamError("%w", err).WithSuggestion(
				"the embedded engine reported an internal failure; re-run the command (the store " +
					"is reopened fresh each run) and report the issue if it persists")
		case ee.Retryable:
			return clierr.NewUpstreamError("%w", err).WithSuggestion(
				"the statement hit a retryable conflict (e.g. a concurrent write to the same store); " +
					"re-run it once the other writer is done")
		case strings.HasPrefix(ee.Code, "Neo.ClientError."):
			return clierr.NewValidationError("%w", err)
		default:
			return clierr.NewUpstreamError("%w", err)
		}
	}
	return clierr.NewValidationError("%w", err)
}

// relateExplainPlan adapts the engine's plan tree onto the driver's Plan
// interface so planNodeFromPlan snapshots it exactly as it does a Bolt plan
// (including argument redaction). The engine's plan shape is
// operator/details/identifiers/estimated rows; the details line rides under
// the "Details" argument key, matching how Neo4j exposes it.
type relateExplainPlan struct {
	p *relate.Plan
}

func (a relateExplainPlan) Operator() string      { return a.p.Operator }
func (a relateExplainPlan) Identifiers() []string { return a.p.Identifiers }
func (a relateExplainPlan) Children() []neo4j.Plan {
	children := make([]neo4j.Plan, 0, len(a.p.Children))
	for i := range a.p.Children {
		children = append(children, relateExplainPlan{p: &a.p.Children[i]})
	}
	return children
}

func (a relateExplainPlan) Arguments() map[string]any {
	args := map[string]any{}
	if a.p.Details != "" {
		args["Details"] = a.p.Details
	}
	if a.p.EstimatedRows != 0 {
		args["EstimatedRows"] = a.p.EstimatedRows
	}
	return args
}

// relateProfiledPlan adapts a PROFILEd plan tree onto the driver's
// ProfiledPlan interface: the same fields as the EXPLAIN adapter plus the
// actuals (rows, db hits, time). Time is converted from the engine's
// milliseconds to the driver's nanoseconds.
type relateProfiledPlan struct {
	relateExplainPlan
}

func (a relateProfiledPlan) Children() []neo4j.ProfiledPlan {
	children := make([]neo4j.ProfiledPlan, 0, len(a.p.Children))
	for i := range a.p.Children {
		children = append(children, relateProfiledPlan{relateExplainPlan{p: &a.p.Children[i]}})
	}
	return children
}

func (a relateProfiledPlan) Records() int64 { return actualOf(a.p).Rows }
func (a relateProfiledPlan) DbHits() int64  { return actualOf(a.p).DbHits }
func (a relateProfiledPlan) Time() int64 {
	return int64(actualOf(a.p).TimeMs * float64(1e6))
}

func (a relateProfiledPlan) PageCacheHits() int64   { return 0 }
func (a relateProfiledPlan) PageCacheMisses() int64 { return 0 }
func (a relateProfiledPlan) PageCacheHitRatio() float64 {
	return 0
}

// actualOf returns the profile actuals of a plan node; the zero PlanActual
// for a node without them (a PROFILE tree whose leaf reports none).
func actualOf(p *relate.Plan) relate.PlanActual {
	if p.Actual == nil {
		return relate.PlanActual{}
	}
	return *p.Actual
}
