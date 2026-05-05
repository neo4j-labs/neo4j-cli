// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/neo4j/cli/common/clicfg"
)

// schemaResult is the structured introspection payload built from the
// sequential cypher calls. Field order is the canonical render order:
// database, nodes, relationships, relationship_paths, indexes, constraints.
// JSON output marshals this struct directly so consumers see consistent keys.
type schemaResult struct {
	Database          *databaseInfo    `json:"database,omitempty"`
	Nodes             []nodeProperty   `json:"nodes"`
	Relationships     []relProperty    `json:"relationships"`
	RelationshipPaths []relPath        `json:"relationship_paths"`
	Indexes           []map[string]any `json:"indexes"`
	Constraints       []map[string]any `json:"constraints"`
}

// databaseInfo holds optional metadata pulled from CALL dbms.components() and
// SHOW SETTINGS. Either field can be empty/missing if the corresponding query
// fails — failures of these optional probes do NOT fail the command.
type databaseInfo struct {
	Name            string   `json:"name,omitempty"`
	Versions        []string `json:"versions,omitempty"`
	Edition         string   `json:"edition,omitempty"`
	DefaultLanguage string   `json:"default_language,omitempty"`
}

// nodeProperty is one row from CALL db.schema.nodeTypeProperties().
type nodeProperty struct {
	NodeType      string   `json:"nodeType"`
	NodeLabels    []string `json:"nodeLabels"`
	PropertyName  string   `json:"propertyName"`
	PropertyTypes []string `json:"propertyTypes"`
	Mandatory     bool     `json:"mandatory"`
}

// relProperty is one row from CALL db.schema.relTypeProperties().
type relProperty struct {
	RelType       string   `json:"relType"`
	PropertyName  string   `json:"propertyName"`
	PropertyTypes []string `json:"propertyTypes"`
	Mandatory     bool     `json:"mandatory"`
}

// relPath records the (from-labels)-[:type]->(to-labels) shape returned by
// the per-relType MATCH probe.
type relPath struct {
	RelType string   `json:"relType"`
	From    []string `json:"from"`
	To      []string `json:"to"`
}

// newSchemaCmd builds the `:schema` cobra leaf. The `Use` is the literal
// `:schema` so users invoke it via `neo4j-cli query :schema` (matches the
// cypher-shell convention).
func newSchemaCmd(cfg *clicfg.Config) *cobra.Command {
	return &cobra.Command{
		Use:   ":schema",
		Short: "Introspect the connected database (labels, rel types, indexes, constraints)",
		Long: "Introspect the connected database. Runs a sequence of read-only " +
			"cypher calls and aggregates the result into one structured payload " +
			"with database info, node/relationship properties, relationship " +
			"paths, indexes, and constraints. --max-rows and --truncate-arrays-over do not apply.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSchema(cmd, cfg)
		},
	}
}

// runSchema is the `:schema` RunE body. It resolves the connection, executes
// the introspection queries sequentially, and renders the result. Required
// queries (1–5) failing aborts the command; the optional database probes
// (6) swallow errors so the command still succeeds with a partial section.
func runSchema(cmd *cobra.Command, cfg *clicfg.Config) error {
	cmd.SilenceUsage = true

	c, err := resolveConn(cmd, cfg)
	if err != nil {
		return err
	}

	if c.password == "" {
		pw, err := promptPassword(cmd)
		if err != nil {
			return err
		}
		c.password = pw
	}

	ctx := cmd.Context()

	nodes, err := fetchNodeProperties(ctx, c)
	if err != nil {
		return err
	}

	rels, err := fetchRelProperties(ctx, c)
	if err != nil {
		return err
	}

	relTypes := uniqueRelTypes(rels)
	paths, err := fetchRelPaths(ctx, c, relTypes)
	if err != nil {
		return err
	}

	indexes, err := fetchTabular(ctx, c,
		"SHOW INDEXES YIELD name, type, entityType, labelsOrTypes, properties, state, owningConstraint, options")
	if err != nil {
		return err
	}

	constraints, err := fetchTabular(ctx, c,
		"SHOW CONSTRAINTS YIELD name, type, entityType, labelsOrTypes, properties, ownedIndex, propertyType")
	if err != nil {
		return err
	}

	// Optional probes — failures are swallowed so a stripped-down server (or
	// one missing dbms.components in this user's role) does not fail :schema.
	dbInfo := fetchDatabaseInfo(ctx, c)
	if dbInfo != nil {
		dbInfo.Name = c.database
	} else if c.database != "" {
		dbInfo = &databaseInfo{Name: c.database}
	}

	result := schemaResult{
		Database:          dbInfo,
		Nodes:             nodes,
		Relationships:     rels,
		RelationshipPaths: paths,
		Indexes:           indexes,
		Constraints:       constraints,
	}

	renderSchema(cmd, cfg, result)
	return nil
}

// fetchNodeProperties runs query (1) and shapes the rows into nodeProperty.
func fetchNodeProperties(ctx context.Context, c *conn) ([]nodeProperty, error) {
	res, err := runStatement(ctx, c,
		"CALL db.schema.nodeTypeProperties() YIELD nodeType, nodeLabels, propertyName, propertyTypes, mandatory", nil)
	if err != nil {
		return nil, err
	}
	idx := indexBy(res.Columns)
	out := make([]nodeProperty, 0, len(res.Rows))
	for _, row := range res.Rows {
		out = append(out, nodeProperty{
			NodeType:      asString(rowGet(row, idx, "nodeType")),
			NodeLabels:    asStringSlice(rowGet(row, idx, "nodeLabels")),
			PropertyName:  asString(rowGet(row, idx, "propertyName")),
			PropertyTypes: asStringSlice(rowGet(row, idx, "propertyTypes")),
			Mandatory:     asBool(rowGet(row, idx, "mandatory")),
		})
	}
	return out, nil
}

// fetchRelProperties runs query (2) and shapes rows into relProperty.
func fetchRelProperties(ctx context.Context, c *conn) ([]relProperty, error) {
	res, err := runStatement(ctx, c,
		"CALL db.schema.relTypeProperties() YIELD relType, propertyName, propertyTypes, mandatory", nil)
	if err != nil {
		return nil, err
	}
	idx := indexBy(res.Columns)
	out := make([]relProperty, 0, len(res.Rows))
	for _, row := range res.Rows {
		out = append(out, relProperty{
			RelType:       asString(rowGet(row, idx, "relType")),
			PropertyName:  asString(rowGet(row, idx, "propertyName")),
			PropertyTypes: asStringSlice(rowGet(row, idx, "propertyTypes")),
			Mandatory:     asBool(rowGet(row, idx, "mandatory")),
		})
	}
	return out, nil
}

// fetchRelPaths runs query (3) once per relType. relType comes back from the
// driver wrapped in colons (`:OWNS`); strip them before interpolating into
// the MATCH pattern. Failure of any per-rel-type query fails the command.
func fetchRelPaths(ctx context.Context, c *conn, relTypes []string) ([]relPath, error) {
	out := make([]relPath, 0, len(relTypes))
	for _, t := range relTypes {
		stripped := stripRelTypeWrap(t)
		if stripped == "" {
			continue
		}
		stmt := fmt.Sprintf(
			"MATCH (n)-[r:`%s`]->(m) WITH DISTINCT labels(n) AS from, labels(m) AS to RETURN from, to",
			stripped)
		res, err := runStatement(ctx, c, stmt, nil)
		if err != nil {
			return nil, err
		}
		idx := indexBy(res.Columns)
		for _, row := range res.Rows {
			out = append(out, relPath{
				RelType: stripped,
				From:    asStringSlice(rowGet(row, idx, "from")),
				To:      asStringSlice(rowGet(row, idx, "to")),
			})
		}
	}
	return out, nil
}

// fetchTabular runs a YIELD-shaped query (indexes, constraints) and converts
// each row into a column→value map. Maps preserve column order for table
// rendering via the result's Columns slice (kept on the side via a closure).
func fetchTabular(ctx context.Context, c *conn, stmt string) ([]map[string]any, error) {
	res, err := runStatement(ctx, c, stmt, nil)
	if err != nil {
		return nil, err
	}
	return rowsFromValues(res.Columns, res.Rows), nil
}

// fetchDatabaseInfo runs the optional CALL dbms.components() + SHOW SETTINGS
// probes. Errors are swallowed: a missing-or-disallowed probe just leaves the
// corresponding field empty. Returns nil if both probes failed entirely.
func fetchDatabaseInfo(ctx context.Context, c *conn) *databaseInfo {
	info := &databaseInfo{}
	gotAny := false

	if res, err := runStatement(ctx, c, "CALL dbms.components()", nil); err == nil && len(res.Rows) > 0 {
		idx := indexBy(res.Columns)
		row := res.Rows[0]
		info.Versions = asStringSlice(rowGet(row, idx, "versions"))
		info.Edition = asString(rowGet(row, idx, "edition"))
		gotAny = true
	}

	if res, err := runStatement(ctx, c,
		"SHOW SETTINGS YIELD name, value WHERE name = 'db.query.default_language' RETURN value", nil); err == nil && len(res.Rows) > 0 {
		idx := indexBy(res.Columns)
		info.DefaultLanguage = asString(rowGet(res.Rows[0], idx, "value"))
		gotAny = true
	}

	if !gotAny {
		return nil
	}
	return info
}

// renderSchema prints the schemaResult in JSON or table form based on
// resolveOutput(cmd, cfg). When --output is "default" (the implicit value),
// the renderer auto-detects: TTY stdout → 5 stacked tables, piped or
// redirected stdout → JSON. Explicit --output table|json always wins. JSON
// mode emits the full struct; table mode emits the five stacked sub-tables
// separated by H2 markers in the canonical order.
func renderSchema(cmd *cobra.Command, cfg *clicfg.Config, r schemaResult) {
	if resolveOutput(cmd, cfg) == "table" {
		printSchemaTables(cmd, r)
		return
	}
	printSchemaJSON(cmd, r)
}

func printSchemaJSON(cmd *cobra.Command, r schemaResult) {
	bytes, err := json.MarshalIndent(r, "", "\t")
	if err != nil {
		// Encoding our own struct cannot fail in practice; mirror output.go.
		panic(err)
	}
	cmd.Println(string(bytes))
}

// printSchemaTables emits the five canonical sub-tables (Nodes, Relationships,
// Relationship Paths, Indexes, Constraints) separated by H2 markers in the
// documented order. The optional `database` info is JSON-only — table mode
// has no natural tabular shape for the single-record metadata payload.
func printSchemaTables(cmd *cobra.Command, r schemaResult) {
	cmd.Println("## Nodes")
	cmd.Println(renderNodesTable(r.Nodes))
	cmd.Println()

	cmd.Println("## Relationships")
	cmd.Println(renderRelsTable(r.Relationships))
	cmd.Println()

	cmd.Println("## Relationship Paths")
	cmd.Println(renderPathsTable(r.RelationshipPaths))
	cmd.Println()

	cmd.Println("## Indexes")
	cmd.Println(renderMapsTable(
		[]string{"name", "type", "entityType", "labelsOrTypes", "properties", "state", "owningConstraint", "options"},
		r.Indexes))
	cmd.Println()

	cmd.Println("## Constraints")
	cmd.Println(renderMapsTable(
		[]string{"name", "type", "entityType", "labelsOrTypes", "properties", "ownedIndex", "propertyType"},
		r.Constraints))
}

func renderNodesTable(rows []nodeProperty) string {
	t := table.NewWriter()
	t.AppendHeader(table.Row{"nodeType", "nodeLabels", "propertyName", "propertyTypes", "mandatory"})
	for _, r := range rows {
		t.AppendRow(table.Row{
			r.NodeType,
			formatCell(toAnySlice(r.NodeLabels)),
			r.PropertyName,
			formatCell(toAnySlice(r.PropertyTypes)),
			fmt.Sprintf("%v", r.Mandatory),
		})
	}
	t.SetStyle(table.StyleLight)
	return t.Render()
}

func renderRelsTable(rows []relProperty) string {
	t := table.NewWriter()
	t.AppendHeader(table.Row{"relType", "propertyName", "propertyTypes", "mandatory"})
	for _, r := range rows {
		t.AppendRow(table.Row{
			r.RelType,
			r.PropertyName,
			formatCell(toAnySlice(r.PropertyTypes)),
			fmt.Sprintf("%v", r.Mandatory),
		})
	}
	t.SetStyle(table.StyleLight)
	return t.Render()
}

func renderPathsTable(rows []relPath) string {
	t := table.NewWriter()
	t.AppendHeader(table.Row{"relType", "from", "to"})
	for _, r := range rows {
		t.AppendRow(table.Row{
			r.RelType,
			formatCell(toAnySlice(r.From)),
			formatCell(toAnySlice(r.To)),
		})
	}
	t.SetStyle(table.StyleLight)
	return t.Render()
}

// renderMapsTable renders a slice of column→value maps using the supplied
// column order as the header. Cells are formatted via formatCell so nested
// values stay as JSON literals.
func renderMapsTable(columns []string, rows []map[string]any) string {
	t := table.NewWriter()
	header := make(table.Row, 0, len(columns))
	for _, c := range columns {
		header = append(header, c)
	}
	t.AppendHeader(header)
	for _, m := range rows {
		row := make(table.Row, 0, len(columns))
		for _, c := range columns {
			row = append(row, formatCell(m[c]))
		}
		t.AppendRow(row)
	}
	t.SetStyle(table.StyleLight)
	return t.Render()
}

// uniqueRelTypes extracts the distinct relType values from relProperty rows,
// stripped of the colon wrapping. Sorted for deterministic ordering of the
// per-relType MATCH calls (and hence test output).
func uniqueRelTypes(rels []relProperty) []string {
	seen := make(map[string]struct{}, len(rels))
	for _, r := range rels {
		t := stripRelTypeWrap(r.RelType)
		if t == "" {
			continue
		}
		seen[t] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// stripRelTypeWrap removes the leading `:` and surrounding backticks/quotes
// the driver may wrap relType in (e.g. ":`OWNS`" → "OWNS").
func stripRelTypeWrap(s string) string {
	for len(s) > 0 && (s[0] == ':' || s[0] == '`' || s[0] == '"') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == '`' || s[len(s)-1] == '"') {
		s = s[:len(s)-1]
	}
	return s
}

// indexBy builds a name→position map for fast column lookup.
func indexBy(columns []string) map[string]int {
	out := make(map[string]int, len(columns))
	for i, c := range columns {
		out[c] = i
	}
	return out
}

// rowGet returns the value at the named column or nil if missing.
func rowGet(row []any, idx map[string]int, name string) any {
	i, ok := idx[name]
	if !ok || i >= len(row) {
		return nil
	}
	return row[i]
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func asStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	xs, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, asString(x))
	}
	return out
}

func toAnySlice(ss []string) []any {
	if ss == nil {
		return nil
	}
	out := make([]any, 0, len(ss))
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}
