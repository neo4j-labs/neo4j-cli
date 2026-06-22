// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"strings"
	"sync"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/query/linter"
)

// The schema-fetch queries mirror cypher-language-support's metadata poller
// (packages/query-tools/src/queries/) so `:lint --fetch-schema` sees the same
// schema the editor tooling does: one UNION ALL summary query with each
// collection capped at 1000, and db.schema.visualization for the
// (from)-[relType]->(to) triples that drive path-directionality warnings.
const (
	lintSummaryQuery = `CALL db.labels() YIELD label
RETURN COLLECT(label)[..1000] AS result
UNION ALL
CALL db.relationshipTypes() YIELD relationshipType
RETURN COLLECT(relationshipType)[..1000] AS result
UNION ALL
CALL db.propertyKeys() YIELD propertyKey
RETURN COLLECT(propertyKey)[..1000] AS result`

	lintGraphSchemaQuery = "CALL db.schema.visualization() YIELD *"

	// YIELD * (not an explicit column list) for the registries: the rows pass
	// through to the analyzer verbatim and must carry everything its
	// signature resolver reads (see linter.DbSchema); * also tolerates
	// columns coming and going across server versions.
	lintProceduresQuery = "SHOW PROCEDURES EXECUTABLE BY CURRENT USER YIELD *"
	lintFunctionsQuery  = "SHOW FUNCTIONS EXECUTABLE BY CURRENT USER YIELD *"
)

// fetchLintSchema pulls the linting schema from the connected database. The
// summary query is required — its failure fails the fetch. The remaining
// probes are best-effort: their failure (old server, restricted role) just
// leaves the corresponding checks inactive, mirroring how :schema treats its
// optional probes. fetchDefaultLanguage skips the SHOW DATABASES probe when
// the caller already knows the dialect (explicit --cypher-version overwrites
// whatever the probe would return).
func fetchLintSchema(ctx context.Context, c *conn, fetchDefaultLanguage bool) (*linter.DbSchema, error) {
	res, err := runStatement(ctx, c, lintSummaryQuery, nil)
	if err != nil {
		return nil, err
	}
	// One row per UNION ALL branch, in source order, single `result` column.
	// Anything else means the server answered in a shape this code does not
	// understand — fail loudly rather than silently lint schema-less.
	if len(res.Rows) != 3 || len(res.Rows[0]) < 1 || len(res.Rows[1]) < 1 || len(res.Rows[2]) < 1 {
		return nil, clierr.NewUpstreamError(
			"query: lint: unexpected schema summary shape: got %d row(s), want 3 (labels, relationship types, property keys)",
			len(res.Rows))
	}
	schema := &linter.DbSchema{
		Labels:            asStringSlice(res.Rows[0][0]),
		RelationshipTypes: asStringSlice(res.Rows[1][0]),
		PropertyKeys:      asStringSlice(res.Rows[2][0]),
	}

	// The optional probes are independent of the summary and of each other,
	// so they run concurrently — each runStatement opens its own session and
	// the driver is goroutine-safe (sessions are never shared). Each
	// goroutine writes its own variable and wg.Wait() publishes those writes
	// to this goroutine, so no locking is needed.
	var (
		graph       []linter.GraphSchemaRel
		defaultLang string
		procs       linter.Registry
		funcs       linter.Registry
	)
	var wg sync.WaitGroup
	probe := func(f func()) {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	probe(func() {
		// Deliberately less defensive than CLS here: Browser/the editor skip
		// the visualization call at >=200 labels+relTypes (sampled shape too
		// big to be useful, too expensive to compute on every poll), but a
		// one-shot lint fetches once per invocation, not on a poll loop —
		// and this fetch layer is due a rewrite anyway (see the
		// common-schema-model open question in linter/README.md).
		if res, err := runStatement(ctx, c, lintGraphSchemaQuery, nil); err == nil {
			graph = graphSchemaFromVisualization(res)
		}
	})
	if fetchDefaultLanguage {
		probe(func() { defaultLang = fetchLintDefaultLanguage(ctx, c) })
	}
	probe(func() { procs = fetchLintRegistry(ctx, c, lintProceduresQuery) })
	probe(func() { funcs = fetchLintRegistry(ctx, c, lintFunctionsQuery) })
	wg.Wait()

	schema.GraphSchema = graph
	schema.DefaultLanguage = defaultLang

	// The registries from one unprefixed fetch are keyed under BOTH
	// dialects: CLS fetches per dialect (CYPHER 5/25 prologues), but those
	// prologues error on older servers and the per-dialect catalog
	// difference is marginal — while a populated registry that is MISSING a
	// dialect key would flag every call in queries resolving to that dialect
	// as unknown (see linter.DbSchema). An empty or failed fetch leaves the
	// key absent so the checks stay off.
	if len(procs) > 0 {
		schema.Procedures = map[string]linter.Registry{
			string(linter.Cypher5):  procs,
			string(linter.Cypher25): procs,
		}
	}
	if len(funcs) > 0 {
		schema.Functions = map[string]linter.Registry{
			string(linter.Cypher5):  funcs,
			string(linter.Cypher25): funcs,
		}
	}
	return schema, nil
}

// fetchLintRegistry runs one SHOW PROCEDURES/FUNCTIONS query and maps each
// row to its raw column map, keyed by name. Failure or an empty result
// returns nil (the corresponding checks stay inactive), mirroring the other
// optional probes.
func fetchLintRegistry(ctx context.Context, c *conn, stmt string) linter.Registry {
	res, err := runStatement(ctx, c, stmt, nil)
	if err != nil {
		return nil
	}
	idx, ok := indexBy(res.Columns)["name"]
	if !ok {
		return nil
	}
	out := make(linter.Registry, len(res.Rows))
	for _, row := range res.Rows {
		if idx >= len(row) {
			continue
		}
		name := asString(row[idx])
		if name == "" {
			continue
		}
		entry := make(map[string]any, len(res.Columns))
		for i, col := range res.Columns {
			if i < len(row) {
				entry[col] = row[i]
			}
		}
		out[name] = entry
	}
	return out
}

// graphSchemaFromVisualization maps the single db.schema.visualization row
// (virtual nodes + virtual relationships) into (from)-[relType]->(to)
// triples, matching CLS's extractRelationshipsWithNamedNodes: each virtual
// node carries exactly one label, relationships are joined on element id.
func graphSchemaFromVisualization(res *queryResult) []linter.GraphSchemaRel {
	if len(res.Rows) != 1 {
		return nil
	}
	idx := indexBy(res.Columns)
	nodes, _ := rowGet(res.Rows[0], idx, "nodes").([]any)
	rels, _ := rowGet(res.Rows[0], idx, "relationships").([]any)

	labelByID := make(map[string]string, len(nodes))
	for _, n := range nodes {
		node, ok := n.(dbtype.Node)
		if !ok || len(node.Labels) == 0 {
			continue
		}
		labelByID[node.ElementId] = node.Labels[0]
	}

	out := make([]linter.GraphSchemaRel, 0, len(rels))
	for _, r := range rels {
		rel, ok := r.(dbtype.Relationship)
		if !ok {
			continue
		}
		from, fromOK := labelByID[rel.StartElementId]
		to, toOK := labelByID[rel.EndElementId]
		if !fromOK || !toOK {
			continue
		}
		out = append(out, linter.GraphSchemaRel{From: from, To: to, RelType: rel.Type})
	}
	return out
}

// fetchLintDefaultLanguage probes the target database's default Cypher
// version (SHOW DATABASES defaultLanguage column, 2025.x servers; absent
// columns or restricted roles just error and yield ""). With no explicit
// database the session targets the user's home database, so the row is
// matched on the `home` flag instead of by name. The value is uppercased
// (CLS does the same) and only the two dialects the linter knows are
// accepted — anything else is dropped rather than passed through.
func fetchLintDefaultLanguage(ctx context.Context, c *conn) string {
	stmt := "SHOW DATABASES YIELD name, home, defaultLanguage WHERE home RETURN defaultLanguage"
	var params map[string]any
	if c.database != "" {
		stmt = "SHOW DATABASES YIELD name, defaultLanguage WHERE name = $db RETURN defaultLanguage"
		params = map[string]any{"db": c.database}
	}
	res, err := runStatement(ctx, c, stmt, params)
	if err != nil || len(res.Rows) == 0 || len(res.Rows[0]) == 0 {
		return ""
	}
	switch v := strings.ToUpper(asString(res.Rows[0][0])); v {
	case string(linter.Cypher5), string(linter.Cypher25):
		return v
	default:
		return ""
	}
}
