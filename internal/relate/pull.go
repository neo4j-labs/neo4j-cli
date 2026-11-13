// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/afero"

	"github.com/neo4j/neo4j-go-driver/v6/neo4j/dbtype"
)

// identityMapName is the sidecar file pull keeps beside the store's engine
// files, mapping source element ids to local element ids per pull source so
// repeated pulls upsert instead of duplicating (the ADR-0080/0158 semantics
// in the shape the CLI's filesystem-backed stores can carry).
const identityMapName = "pull-identity.json"

// SubgraphNode is one pulled node, identified by its source element id.
type SubgraphNode struct {
	ElementID string
	Labels    []string
	Props     map[string]any
}

// SubgraphRel is one pulled relationship with its source element id and its
// endpoints' source element ids. localStart/localEnd are the endpoints'
// LOCAL element ids, filled during materialization.
type SubgraphRel struct {
	ElementID      string
	StartElementID string
	EndElementID   string
	Type           string
	Props          map[string]any
	localStart     string
	localEnd       string
}

// Subgraph is the deduplicated set of entities extracted from a pull
// query's rows: nodes keyed by source element id, relationships likewise.
// First occurrence wins for repeated entities (a well-formed (a, t, b)
// triple repeats nodes across rows).
type Subgraph struct {
	Nodes map[string]*SubgraphNode
	Rels  map[string]*SubgraphRel
}

// NewSubgraph returns an empty subgraph.
func NewSubgraph() *Subgraph {
	return &Subgraph{
		Nodes: map[string]*SubgraphNode{},
		Rels:  map[string]*SubgraphRel{},
	}
}

// EntityCounts reports the subgraph's node and relationship counts.
func (g *Subgraph) EntityCounts() (nodes, rels int) {
	return len(g.Nodes), len(g.Rels)
}

// ExtractEntities walks one pulled row's cells and adds every graph entity
// it finds (nodes, relationships, and the elements of paths) to the
// subgraph, deduplicating by source element id. It returns how many
// entities the row carried — a row of only scalars yields 0, which the
// caller counts to shape-error the query when every row was scalar.
func ExtractEntities(g *Subgraph, values []any) int {
	found := 0
	for _, v := range values {
		found += extractValue(g, v)
	}
	return found
}

func extractValue(g *Subgraph, v any) int {
	switch val := v.(type) {
	case dbtype.Node:
		if _, seen := g.Nodes[val.ElementId]; !seen {
			g.Nodes[val.ElementId] = &SubgraphNode{
				ElementID: val.ElementId,
				Labels:    append([]string(nil), val.Labels...),
				Props:     val.Props,
			}
			return 1
		}
		return 0
	case dbtype.Relationship:
		if _, seen := g.Rels[val.ElementId]; !seen {
			g.Rels[val.ElementId] = &SubgraphRel{
				ElementID:      val.ElementId,
				StartElementID: val.StartElementId,
				EndElementID:   val.EndElementId,
				Type:           val.Type,
				Props:          val.Props,
			}
			return 1
		}
		return 0
	case dbtype.Path:
		n := 0
		for i := range val.Nodes {
			n += extractValue(g, val.Nodes[i])
		}
		for i := range val.Relationships {
			n += extractValue(g, val.Relationships[i])
		}
		return n
	default:
		return 0
	}
}

// PullStats is the outcome of one materialization: created vs reused counts
// per element kind, plus the relationships skipped because the pull did not
// include one of their endpoints.
type PullStats struct {
	NodesCreated int
	NodesReused  int
	RelsCreated  int
	RelsReused   int
	RelsSkipped  int
}

// IdentityMap records, per pull source, the local element id each source
// element id materialized as — the persisted half of the upsert contract.
type IdentityMap struct {
	Sources map[string]SourceIdentities
}

// SourceIdentities holds one source's node and relationship id mappings.
type SourceIdentities struct {
	Nodes map[string]string `json:"nodes"`
	Rels  map[string]string `json:"rels"`
}

// LoadIdentityMap reads the sidecar beside the store at path; a missing
// file is an empty map, not an error.
func LoadIdentityMap(fs afero.Fs, storePath string) (*IdentityMap, error) {
	raw, err := afero.ReadFile(fs, filepath.Join(storePath, identityMapName))
	if err != nil {
		return &IdentityMap{Sources: map[string]SourceIdentities{}}, nil
	}
	var m IdentityMap
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", identityMapName, err)
	}
	if m.Sources == nil {
		m.Sources = map[string]SourceIdentities{}
	}
	return &m, nil
}

// Save writes the sidecar back beside the store.
func (m *IdentityMap) Save(fs afero.Fs, storePath string) error {
	raw, err := json.MarshalIndent(m, "", "\t")
	if err != nil {
		return err
	}
	return afero.WriteFile(fs, filepath.Join(storePath, identityMapName), append(raw, '\n'), 0o600)
}

// source returns (creating if needed) one source's identity tables.
func (m *IdentityMap) source(source string) SourceIdentities {
	s, ok := m.Sources[source]
	if !ok {
		s = SourceIdentities{Nodes: map[string]string{}, Rels: map[string]string{}}
		m.Sources[source] = s
		return s
	}
	if s.Nodes == nil {
		s.Nodes = map[string]string{}
	}
	if s.Rels == nil {
		s.Rels = map[string]string{}
	}
	m.Sources[source] = s
	return s
}

// Materialize writes (upserts) the subgraph into the store in batches,
// resolving identities through the map: entities the source already
// materialized are updated in place by local element id, new entities are
// created and recorded. Relationships whose endpoints were not part of the
// pull are skipped and counted. Properties replace the local entity's
// properties wholesale (SET n = props) — the source is the truth for the
// pulled scope.
//
// Batching: statements carry up to batchSize rows each, with each row's
// properties inlined as Cypher literals (the typed parameter binder does
// not cover temporal/spatial values yet). Nodes are grouped by label
// signature and relationships by type, so one statement covers one
// signature's batch. Every statement is its own auto-commit transaction.
func Materialize(store Store, g *Subgraph, m *IdentityMap, source string, batchSize int) (*PullStats, error) {
	if batchSize <= 0 {
		batchSize = DefaultPullBatchSize
	}
	stats := &PullStats{}
	ids := m.source(source)

	// Nodes first (relationships need their local ids), in deterministic
	// source-id order.
	nodeIDs := sortedKeys(g.Nodes)
	var fresh []*SubgraphNode
	for _, srcID := range nodeIDs {
		n := g.Nodes[srcID]
		if local, ok := ids.Nodes[srcID]; ok {
			if err := updateEntity(store, batchRows{kind: "node-update", local: local, props: n.Props}); err != nil {
				return nil, err
			}
			stats.NodesReused++
			continue
		}
		fresh = append(fresh, n)
	}

	// Create new nodes grouped by label signature.
	bySignature := map[string][]*SubgraphNode{}
	for _, n := range fresh {
		sig := strings.Join(n.Labels, "\x00")
		bySignature[sig] = append(bySignature[sig], n)
	}
	for _, group := range bySignature {
		labels := group[0].Labels
		for i := 0; i < len(group); i += batchSize {
			batch := group[i:min(i+batchSize, len(group))]
			rows := make([]string, len(batch))
			for j, n := range batch {
				props, err := literalProps(n.Props)
				if err != nil {
					return nil, fmt.Errorf("node %s: %w", n.ElementID, err)
				}
				rows[j] = "{props: " + props + ", src: " + quoteString(n.ElementID) + "}"
			}
			stmt := "UNWIND [" + strings.Join(rows, ", ") + "] AS row " +
				createNodeClause(labels) + " SET n = row.props " +
				"RETURN elementId(n) AS eid, row.src AS src"
			res, err := store.Run([]string{stmt}, nil, false)
			if err != nil {
				return nil, err
			}
			for _, row := range res[0].Rows {
				eid, _ := row[0].(string)
				src, _ := row[1].(string)
				ids.Nodes[src] = eid
			}
			stats.NodesCreated += len(batch)
		}
	}

	// Relationships, in deterministic source-id order.
	relIDs := sortedKeys(g.Rels)
	var freshRels []*SubgraphRel
	for _, srcID := range relIDs {
		r := g.Rels[srcID]
		if local, ok := ids.Rels[srcID]; ok {
			if err := updateEntity(store, batchRows{kind: "rel-update", local: local, props: r.Props}); err != nil {
				return nil, err
			}
			stats.RelsReused++
			continue
		}
		freshRels = append(freshRels, r)
	}

	byType := map[string][]*SubgraphRel{}
	for _, r := range freshRels {
		start, okStart := ids.Nodes[r.StartElementID]
		end, okEnd := ids.Nodes[r.EndElementID]
		if !okStart || !okEnd {
			stats.RelsSkipped++
			continue
		}
		r.localStart, r.localEnd = start, end
		byType[r.Type] = append(byType[r.Type], r)
	}
	for typ, group := range byType {
		for i := 0; i < len(group); i += batchSize {
			batch := group[i:min(i+batchSize, len(group))]
			rows := make([]string, len(batch))
			for j, r := range batch {
				props, err := literalProps(r.Props)
				if err != nil {
					return nil, fmt.Errorf("relationship %s: %w", r.ElementID, err)
				}
				rows[j] = "{props: " + props + ", src: " + quoteString(r.ElementID) +
					", start: " + quoteString(r.localStart) + ", end: " + quoteString(r.localEnd) + "}"
			}
			stmt := "UNWIND [" + strings.Join(rows, ", ") + "] AS row " +
				"MATCH (a), (b) WHERE elementId(a) = row.start AND elementId(b) = row.end " +
				"CREATE (a)-[r:" + QuoteIdent(typ) + "]->(b) SET r = row.props " +
				"RETURN elementId(r) AS eid, row.src AS src"
			res, err := store.Run([]string{stmt}, nil, false)
			if err != nil {
				return nil, err
			}
			for _, row := range res[0].Rows {
				eid, _ := row[0].(string)
				src, _ := row[1].(string)
				ids.Rels[src] = eid
			}
			stats.RelsCreated += len(batch)
		}
	}

	return stats, nil
}

// DefaultPullBatchSize bounds one statement's literal row list. Relate's own
// default (5000) assumes parameter binding; inlined literal rows grow with
// property payloads, so the CLI's default stays an order of magnitude lower.
const DefaultPullBatchSize = 500

// batchRows is a single-row update (entities the source already
// materialized).
type batchRows struct {
	kind  string
	local string
	props map[string]any
}

// updateEntity applies an in-place property replacement for one already
// materialized entity.
func updateEntity(store Store, r batchRows) error {
	props, err := literalProps(r.props)
	if err != nil {
		return err
	}
	row := "{props: " + props + ", local: " + quoteString(r.local) + "}"
	stmt := "UNWIND [" + row + "] AS row "
	if r.kind == "node-update" {
		stmt += "MATCH (n) WHERE elementId(n) = row.local SET n = row.props"
	} else {
		stmt += "MATCH ()-[r]->() WHERE elementId(r) = row.local SET r = row.props"
	}
	_, err = store.Run([]string{stmt}, nil, false)
	return err
}

// createNodeClause renders the CREATE clause for one label signature.
func createNodeClause(labels []string) string {
	var b strings.Builder
	b.WriteString("CREATE (n")
	for _, l := range labels {
		b.WriteByte(':')
		b.WriteString(QuoteIdent(l))
	}
	b.WriteByte(')')
	return b.String()
}

// literalProps renders a property map as a Cypher map literal; nil becomes
// the empty map so SET n = props never binds null.
func literalProps(props map[string]any) (string, error) {
	if props == nil {
		return "{}", nil
	}
	return literalMap(props)
}

// sortedKeys returns the map's keys in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
