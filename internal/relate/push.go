// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"fmt"
	"sort"
	"strings"
)

// PushLabel is one label's slice of a push plan: the identity key its nodes
// MERGE on remotely, and how many local nodes carry the label.
type PushLabel struct {
	Label string
	Key   string
	Count int64
}

// PushRelGroup is one (start label, type, end label) slice of a push plan.
// StartLabel/EndLabel are the endpoint labels chosen for the remote MATCH
// (the first of each endpoint's sorted labels that has a resolved key) and
// StartKey/EndKey their identity keys.
type PushRelGroup struct {
	Type                 string
	StartLabel, StartKey string
	EndLabel, EndKey     string
	Count                int64
}

// PushPlan is the full description of what a push would write: the label
// slices (nodes, MERGE-keyed) and the relationship groups (MATCH-keyed
// endpoints, MERGE on the pattern).
type PushPlan struct {
	Labels PushLabels
	Rels   []PushRelGroup
	// UnkeyedLabels are labels in scope with no resolved identity key —
	// pushing them is refused, and the plan names them so the error and the
	// --dry-run report share one source.
	UnkeyedLabels []string
	// UnkeyedRels is the count of relationships whose endpoints have no
	// keyed label between them; they are skipped and reported.
	UnkeyedRels int64
}

// PushLabels keeps label slices in sorted order for deterministic output.
type PushLabels []PushLabel

// PushKeySpec maps label → identity key property, resolved from the remote's
// single-property uniqueness constraints and the --merge-key overrides.
type PushKeySpec map[string]string

// ParseMergeKey parses one `Label.property` override entry. A property name
// cannot itself contain a dot, so a second dot is a malformed spec.
func ParseMergeKey(spec string) (label, key string, err error) {
	label, key, found := strings.Cut(spec, ".")
	if !found || label == "" || key == "" || strings.Contains(key, ".") {
		return "", "", fmt.Errorf("invalid --merge-key %q: expected Label.property", spec)
	}
	return label, key, nil
}

// BuildPushPlan reads the local store's shape scoped by labels and types,
// joins it with the resolved key spec, and returns the plan. A label in
// scope without a key lands in UnkeyedLabels (the caller refuses unless
// dry-running); relationship triples whose endpoints have no keyed label
// count into UnkeyedRels.
func BuildPushPlan(store Store, labels, types []string, keys PushKeySpec) (*PushPlan, error) {
	plan := &PushPlan{}

	type relRow struct {
		startLabels []string
		endLabels   []string
		typ         string
		count       int64
	}

	// One shape query covers rel groups; label counts come per label.
	var relRows []relRow
	if res, err := store.Run([]string{
		"MATCH (a)-[r]->(b) RETURN labels(a) AS sl, type(r) AS t, labels(b) AS el, count(*) AS c",
	}, nil, false); err == nil && len(res) == 1 {
		for _, row := range res[0].Rows {
			sl, _ := row[0].([]any)
			typ, _ := row[1].(string)
			el, _ := row[2].([]any)
			c, _ := row[3].(int64)
			if len(sl) == 0 || len(el) == 0 {
				continue // an unlabeled endpoint has no identity to merge on
			}
			if inScope(types, typ) {
				relRows = append(relRows, relRow{startLabels: toStrings(sl), endLabels: toStrings(el), typ: typ, count: c})
			}
		}
	}

	// Scope labels: explicit --labels or every label in the store.
	scope := labels
	if scope == nil {
		if res, err := store.Run([]string{"CALL db.labels()"}, nil, false); err == nil && len(res) == 1 {
			for _, row := range res[0].Rows {
				if l, ok := row[0].(string); ok {
					scope = append(scope, l)
				}
			}
		}
		sort.Strings(scope)
	}

	for _, label := range scope {
		key, ok := keys[label]
		if !ok {
			plan.UnkeyedLabels = append(plan.UnkeyedLabels, label)
			continue
		}
		count := int64(0)
		if res, err := store.Run([]string{"MATCH (n:" + QuoteIdent(label) + ") RETURN count(n) AS c"}, nil, false); err == nil && len(res) == 1 && len(res[0].Rows) > 0 {
			count, _ = res[0].Rows[0][0].(int64)
		}
		plan.Labels = append(plan.Labels, PushLabel{Label: label, Key: key, Count: count})
	}

	for _, r := range relRows {
		startLabel, startKey := keyedLabel(r.startLabels, keys)
		endLabel, endKey := keyedLabel(r.endLabels, keys)
		if startLabel == "" || endLabel == "" {
			plan.UnkeyedRels += r.count
			continue
		}
		plan.Rels = append(plan.Rels, PushRelGroup{
			Type: r.typ, Count: r.count,
			StartLabel: startLabel, StartKey: startKey,
			EndLabel: endLabel, EndKey: endKey,
		})
	}
	return plan, nil
}

// keyedLabel picks the first of the endpoint's sorted labels that has a
// resolved key (sorting makes the choice deterministic regardless of the
// label order the engine reports); "" when none does.
func keyedLabel(labels []string, keys PushKeySpec) (label, key string) {
	sorted := append([]string(nil), labels...)
	sort.Strings(sorted)
	for _, l := range sorted {
		if k, ok := keys[l]; ok {
			return l, k
		}
	}
	return "", ""
}

// inScope reports whether typ is in the filter (empty filter = all).
func inScope(types []string, typ string) bool {
	if len(types) == 0 {
		return true
	}
	for _, t := range types {
		if t == typ {
			return true
		}
	}
	return false
}

// ReadPushNodes reads one label's nodes as parameter rows for the remote
// MERGE: each row carries the key value and the full property map. A node
// whose key property is missing or null is an error naming the label and
// key — merging on a null identity would create garbage.
func ReadPushNodes(store Store, label, key string) ([]map[string]any, error) {
	res, err := store.Run([]string{
		"MATCH (n:" + QuoteIdent(label) + ") RETURN n." + QuoteIdent(key) + " AS key, properties(n) AS props",
	}, nil, false)
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(res[0].Rows))
	nullKeys := 0
	for _, row := range res[0].Rows {
		k := row[0]
		if k == nil {
			nullKeys++
			continue
		}
		props, _ := row[1].(map[string]any)
		if props == nil {
			props = map[string]any{}
		}
		rows = append(rows, map[string]any{"key": k, "props": props})
	}
	if nullKeys > 0 {
		return nil, fmt.Errorf("%d nodes of label %s have no value for the key property %s; the remote MERGE would create garbage — fix the data or choose another key",
			nullKeys, label, key)
	}
	return rows, nil
}

// ReadPushRels reads one group's relationships as parameter rows: the
// endpoints' key values and the relationship's properties.
func ReadPushRels(store Store, g PushRelGroup) ([]map[string]any, error) {
	res, err := store.Run([]string{
		"MATCH (a:" + QuoteIdent(g.StartLabel) + ")-[r:" + QuoteIdent(g.Type) + "]->(b:" + QuoteIdent(g.EndLabel) + ") " +
			"RETURN a." + QuoteIdent(g.StartKey) + " AS start, b." + QuoteIdent(g.EndKey) + " AS end, properties(r) AS props",
	}, nil, false)
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(res[0].Rows))
	for _, row := range res[0].Rows {
		props, _ := row[2].(map[string]any)
		if props == nil {
			props = map[string]any{}
		}
		rows = append(rows, map[string]any{"start": row[0], "end": row[1], "props": props})
	}
	return rows, nil
}

// PushNodesStatement renders the remote node upsert for one label: MERGE on
// the identity key, properties replaced wholesale (the local store is the
// truth for the pushed scope). The matched row identity is returned so the
// caller can count what actually landed.
func PushNodesStatement(label, key string) string {
	return "UNWIND $rows AS row MERGE (n:" + QuoteIdent(label) + " {" + QuoteIdent(key) + ": row.key}) " +
		"SET n = row.props RETURN row.key AS key"
}

// PushRelsStatement renders the remote relationship upsert for one group:
// MATCH the endpoints by their identity keys, MERGE the pattern (optionally
// keyed by a relationship property), set the properties. Endpoints that do
// not exist remotely simply do not match — the returned row identity lets
// the caller count unmatched rows.
func PushRelsStatement(g PushRelGroup, relKey string) string {
	pattern := QuoteIdent(g.Type)
	if relKey != "" {
		pattern += " {" + QuoteIdent(relKey) + ": row.rkey}"
	}
	return "UNWIND $rows AS row " +
		"MATCH (a:" + QuoteIdent(g.StartLabel) + " {" + QuoteIdent(g.StartKey) + ": row.start}), " +
		"(b:" + QuoteIdent(g.EndLabel) + " {" + QuoteIdent(g.EndKey) + ": row.end}) " +
		"MERGE (a)-[r:" + pattern + "]->(b) " +
		"SET r = row.props RETURN row.start AS start, row.end AS end"
}

// toStrings converts a []any of strings.
func toStrings(values []any) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
