// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"strings"

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

	// lintGraphSchemaThreshold is the CLS heuristic: above this many
	// labels+relTypes the visualization call is skipped (the sampled graph
	// shape gets too big to be useful and too expensive to compute).
	lintGraphSchemaThreshold = 200
)

// fetchLintSchema pulls the linting schema from the connected database. The
// summary query is required — its failure fails the fetch. The graph-shape
// and default-language probes are best-effort: their failure (old server,
// restricted role) just leaves the corresponding checks inactive, mirroring
// how :schema treats its optional probes.
func fetchLintSchema(ctx context.Context, c *conn) (*linter.DbSchema, error) {
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

	if len(schema.Labels)+len(schema.RelationshipTypes) < lintGraphSchemaThreshold {
		if res, err := runStatement(ctx, c, lintGraphSchemaQuery, nil); err == nil {
			schema.GraphSchema = graphSchemaFromVisualization(res)
		}
	}

	schema.DefaultLanguage = fetchLintDefaultLanguage(ctx, c)
	return schema, nil
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
