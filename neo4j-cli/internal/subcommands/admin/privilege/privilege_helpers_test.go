// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
)

// testConn returns a *dbconn.Conn for use in tests. The connection params are
// never used because tests always override privilegeExecFn with a fake.
func testConn() *dbconn.Conn {
	return &dbconn.Conn{
		URI:      "neo4j://localhost:7687",
		Username: "neo4j",
		Password: "test",
	}
}

type sequencedCall struct {
	cypher string
	params map[string]any
}

// withRecordingSequencedExecFn replaces privilegeExecFn with a sequenced fake
// that records each call's cypher/params into calls and returns responses in
// order. It fails the test if called more times than there are responses.
func withRecordingSequencedExecFn(t *testing.T, calls *[]sequencedCall, responses []struct {
	rows []map[string]any
	err  error
}) {
	t.Helper()
	orig := privilegeExecFn
	idx := 0
	privilegeExecFn = func(_ context.Context, _ *clicfg.Config, _ *dbconn.Conn, cypher string, params map[string]any) ([]map[string]any, error) {
		*calls = append(*calls, sequencedCall{cypher: cypher, params: params})
		if idx >= len(responses) {
			t.Fatalf("privilegeExecFn called %d times but only %d response(s) were provided", idx+1, len(responses))
		}
		r := responses[idx]
		idx++
		return r.rows, r.err
	}
	t.Cleanup(func() { privilegeExecFn = orig })
}

func twoOK() []struct {
	rows []map[string]any
	err  error
} {
	return []struct {
		rows []map[string]any
		err  error
	}{
		{rows: nil, err: nil},
		{rows: []map[string]any{}, err: nil},
	}
}

func TestCanonicalAction(t *testing.T) {
	cases := map[string]string{
		"all_graph_privileges":    "ALL GRAPH PRIVILEGES",
		"ALL GRAPH PRIVILEGES":    "ALL GRAPH PRIVILEGES",
		"read":                    "READ",
		"Set_Label":               "SET LABEL",
		"  create   role ":        "CREATE ROLE",
		"set-property":            "SET PROPERTY",
		"all-graph-privileges":    "ALL GRAPH PRIVILEGES",
		"set-label":               "SET LABEL",
		"create-role":             "CREATE ROLE",
		"all-database-privileges": "ALL DATABASE PRIVILEGES",
	}
	for input, want := range cases {
		if got := canonicalAction(input); got != want {
			t.Fatalf("canonicalAction(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestValidActions_FindAbsent(t *testing.T) {
	if _, ok := validActions["FIND"]; ok {
		t.Fatal("FIND must not be a valid action (REQ-F-013)")
	}
}

func TestKebabAction(t *testing.T) {
	cases := map[string]string{
		"SET LABEL":            "set-label",
		"ALL GRAPH PRIVILEGES": "all-graph-privileges",
		"READ":                 "read",
		"CREATE ROLE":          "create-role",
	}
	for canonical, want := range cases {
		if got := kebabAction(canonical); got != want {
			t.Fatalf("kebabAction(%q) = %q, want %q", canonical, got, want)
		}
	}
}

// TestCategoryMeta_CoversEveryActionExactlyOnce asserts categoryMeta and
// categoryOrder describe exactly the seven categories and that the union of all
// categories' actions equals validActions (every action reachable through one
// category).
func TestCategoryMeta_CoversEveryActionExactlyOnce(t *testing.T) {
	if len(categoryOrder) != 7 {
		t.Fatalf("categoryOrder has %d entries, want 7", len(categoryOrder))
	}
	if len(categoryMeta) != 7 {
		t.Fatalf("categoryMeta has %d entries, want 7", len(categoryMeta))
	}

	seen := map[string]bool{}
	for _, cat := range categoryOrder {
		meta, ok := categoryMeta[cat]
		if !ok {
			t.Fatalf("category %d in categoryOrder has no categoryMeta entry", cat)
		}
		if meta.name == "" {
			t.Fatalf("category %d has empty name", cat)
		}
		for _, action := range actionsForCategory(cat) {
			if seen[action] {
				t.Fatalf("action %q appears in more than one category", action)
			}
			seen[action] = true
		}
	}
	if len(seen) != len(validActions) {
		t.Fatalf("union of category actions has %d entries, want %d (validActions)", len(seen), len(validActions))
	}
	for action := range validActions {
		if !seen[action] {
			t.Fatalf("action %q in validActions is not covered by any category", action)
		}
	}
}

func TestCategoryMeta_NamesAndKebabActions(t *testing.T) {
	wantNames := map[actionCategory]string{
		propertyBearer: "property",
		graphOnly:      "entity",
		graphWhole:     "graph",
		labelScoped:    "label",
		load:           "load",
		database:       "database",
		dbms:           "dbms",
	}
	for cat, want := range wantNames {
		if got := categoryMeta[cat].name; got != want {
			t.Fatalf("categoryMeta[%d].name = %q, want %q", cat, got, want)
		}
	}
	got := kebabActionsForCategory(propertyBearer)
	want := []string{"match", "merge", "read", "set-property"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("kebabActionsForCategory(propertyBearer) = %v, want %v", got, want)
	}
}

func TestRenderCategoryExample(t *testing.T) {
	got := renderCategoryExample("grant", propertyBearer)
	for _, want := range []string{
		"# Grant a property privilege",
		"neo4j-cli admin privilege grant property read --on-graph * --property name --role analyst --credential local --rw",
		"--format json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderCategoryExample missing %q in:\n%s", want, got)
		}
	}
	if strings.Count(got, "neo4j-cli") < 2 {
		t.Fatalf("renderCategoryExample should have >= 2 invocations:\n%s", got)
	}
	if strings.HasPrefix(got, " ") || strings.HasPrefix(got, "\t") {
		t.Fatalf("renderCategoryExample must be flush-left:\n%s", got)
	}

	// A single-action category (load) omits the action token: the example is
	// "grant load --cidr ...", never "grant load load ...".
	loadExample := renderCategoryExample("grant", load)
	if !strings.Contains(loadExample, "neo4j-cli admin privilege grant load --cidr 127.0.0.1/32 --role analyst --credential local --rw") {
		t.Fatalf("load example must omit the action token:\n%s", loadExample)
	}
	if strings.Contains(loadExample, "grant load load") {
		t.Fatalf("load example must not repeat the action token:\n%s", loadExample)
	}
}

// The expected Cypher fragments below are server-validated: each was confirmed
// valid on a real Neo4j 2025.x server (see REQ-NF-005). Do not change them to
// invalid Cypher just to make a test pass — a green test asserting Cypher the
// server rejects is the exact regression these cases guard against.
func TestBuildPrivilegeCypher_HappyPaths(t *testing.T) {
	cases := []struct {
		name   string
		verb   string
		action string
		opts   privilegeOpts
		want   string
	}{
		{
			name:   "propertyBearer default",
			verb:   "GRANT",
			action: "READ",
			opts:   privilegeOpts{onGraph: "*"},
			want:   "GRANT READ {*} ON GRAPH * ELEMENTS *",
		},
		{
			name:   "propertyBearer with property and node label",
			verb:   "GRANT",
			action: "READ",
			opts:   privilegeOpts{onGraph: "neo4j", nodeLabels: []string{"Person"}, properties: []string{"name"}},
			want:   "GRANT READ {`name`} ON GRAPH `neo4j` NODES `Person`",
		},
		{
			name:   "propertyBearer multi property and rel type",
			verb:   "GRANT",
			action: "MATCH",
			opts:   privilegeOpts{onGraph: "neo4j", relTypes: []string{"KNOWS"}, properties: []string{"weight", "since"}},
			want:   "GRANT MATCH {`weight`, `since`} ON GRAPH `neo4j` RELATIONSHIPS `KNOWS`",
		},
		{
			name:   "propertyBearer default graph when on-graph absent",
			verb:   "GRANT",
			action: "READ",
			opts:   privilegeOpts{nodeLabels: []string{"Person"}},
			want:   "GRANT READ {*} ON GRAPH * NODES `Person`",
		},
		{
			name:   "graphOnly default",
			verb:   "GRANT",
			action: "TRAVERSE",
			opts:   privilegeOpts{onGraph: "*"},
			want:   "GRANT TRAVERSE ON GRAPH * ELEMENTS *",
		},
		{
			name:   "graphWhole write default graph",
			verb:   "GRANT",
			action: "write",
			opts:   privilegeOpts{onGraph: "*"},
			want:   "GRANT WRITE ON GRAPH *",
		},
		{
			name:   "graphWhole all graph privileges no entity clause",
			verb:   "GRANT",
			action: "all_graph_privileges",
			opts:   privilegeOpts{onGraph: "neo4j"},
			want:   "GRANT ALL GRAPH PRIVILEGES ON GRAPH `neo4j`",
		},
		{
			name:   "setLabel",
			verb:   "GRANT",
			action: "set_label",
			opts:   privilegeOpts{onGraph: "neo4j", nodeLabels: []string{"Person"}},
			want:   "GRANT SET LABEL `Person` ON GRAPH `neo4j`",
		},
		{
			name:   "setLabel multiple labels",
			verb:   "GRANT",
			action: "set_label",
			opts:   privilegeOpts{onGraph: "neo4j", nodeLabels: []string{"Person", "Movie"}},
			want:   "GRANT SET LABEL `Person`, `Movie` ON GRAPH `neo4j`",
		},
		{
			name:   "removeLabel",
			verb:   "GRANT",
			action: "remove_label",
			opts:   privilegeOpts{onGraph: "neo4j", nodeLabels: []string{"Person"}},
			want:   "GRANT REMOVE LABEL `Person` ON GRAPH `neo4j`",
		},
		{
			name:   "database with explicit database",
			verb:   "GRANT",
			action: "access",
			opts:   privilegeOpts{onDatabase: "neo4j"},
			want:   "GRANT ACCESS ON DATABASE `neo4j`",
		},
		{
			name:   "database default star",
			verb:   "GRANT",
			action: "access",
			opts:   privilegeOpts{},
			want:   "GRANT ACCESS ON DATABASE *",
		},
		{
			name:   "dbms",
			verb:   "GRANT",
			action: "create_role",
			opts:   privilegeOpts{},
			want:   "GRANT CREATE ROLE ON DBMS",
		},
		{
			name:   "load on all data by default",
			verb:   "GRANT",
			action: "load",
			opts:   privilegeOpts{},
			want:   "GRANT LOAD ON ALL DATA",
		},
		{
			name:   "load on cidr",
			verb:   "GRANT",
			action: "load",
			opts:   privilegeOpts{cidr: "127.0.0.1/32"},
			want:   `GRANT LOAD ON CIDR "127.0.0.1/32"`,
		},
		{
			name:   "revoke uses verb prefix",
			verb:   "REVOKE GRANT",
			action: "read",
			opts:   privilegeOpts{onGraph: "*"},
			want:   "REVOKE GRANT READ {*} ON GRAPH * ELEMENTS *",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canonical := canonicalAction(tc.action)
			got, params, err := buildPrivilegeCypher(tc.verb, validActions[canonical], canonical, tc.opts)
			if err != nil {
				t.Fatalf("buildPrivilegeCypher returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("cypher = %q, want %q", got, tc.want)
			}
			if params == nil {
				t.Fatal("params map must be non-nil")
			}
		})
	}
}

func TestBuildPrivilegeCypher_EscapesIdentifiers(t *testing.T) {
	cases := []struct {
		name   string
		verb   string
		action string
		opts   privilegeOpts
		want   string
	}{
		{
			name:   "database injection payload is a single quoted identifier",
			verb:   "GRANT",
			action: "access",
			opts:   privilegeOpts{onDatabase: "neo4j TO adminRole-- "},
			want:   "GRANT ACCESS ON DATABASE `neo4j TO adminRole-- `",
		},
		{
			name:   "graph injection payload is a single quoted identifier",
			verb:   "GRANT",
			action: "traverse",
			opts:   privilegeOpts{onGraph: "neo4j TO adminRole-- "},
			want:   "GRANT TRAVERSE ON GRAPH `neo4j TO adminRole-- ` ELEMENTS *",
		},
		{
			name:   "node label with internal backtick is doubled",
			verb:   "GRANT",
			action: "read",
			opts:   privilegeOpts{onGraph: "*", nodeLabels: []string{"Pe`rson"}},
			want:   "GRANT READ {*} ON GRAPH * NODES `Pe``rson`",
		},
		{
			name:   "property with internal backtick is doubled",
			verb:   "GRANT",
			action: "read",
			opts:   privilegeOpts{onGraph: "*", properties: []string{"na`me"}},
			want:   "GRANT READ {`na``me`} ON GRAPH * ELEMENTS *",
		},
		{
			name:   "relationship type injection payload is a single quoted identifier",
			verb:   "GRANT",
			action: "traverse",
			opts:   privilegeOpts{onGraph: "*", relTypes: []string{"KNOWS TO adminRole-- "}},
			want:   "GRANT TRAVERSE ON GRAPH * RELATIONSHIPS `KNOWS TO adminRole-- `",
		},
		{
			name:   "set label injection payload is a single quoted identifier",
			verb:   "GRANT",
			action: "set_label",
			opts:   privilegeOpts{onGraph: "*", nodeLabels: []string{"Person TO adminRole-- "}},
			want:   "GRANT SET LABEL `Person TO adminRole-- ` ON GRAPH *",
		},
		{
			name:   "cidr with embedded quote and backslash is escaped",
			verb:   "GRANT",
			action: "load",
			opts:   privilegeOpts{cidr: `1.2.3.4/32" TO adminRole\`},
			want:   `GRANT LOAD ON CIDR "1.2.3.4/32\" TO adminRole\\"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canonical := canonicalAction(tc.action)
			got, _, err := buildPrivilegeCypher(tc.verb, validActions[canonical], canonical, tc.opts)
			if err != nil {
				t.Fatalf("buildPrivilegeCypher returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("cypher = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBuildPrivilegeCypher_Errors covers the only two usage errors reachable
// through the command tree: the per-category factory registers only its
// category's flags, so cross-scope combinations are structurally impossible and
// no longer guarded here (see buildPrivilegeCypher's precondition comment). What
// remains are the two genuine in-category rules — label/relationship-type mutual
// exclusion and labelScoped requiring --node-label.
func TestBuildPrivilegeCypher_Errors(t *testing.T) {
	cases := []struct {
		name    string
		verb    string
		action  string
		opts    privilegeOpts
		wantMsg string
	}{
		{name: "setLabel missing node-label", verb: "GRANT", action: "set_label", opts: privilegeOpts{onGraph: "neo4j"}, wantMsg: "action SET LABEL requires --node-label"},
		{name: "removeLabel missing node-label", verb: "GRANT", action: "remove_label", opts: privilegeOpts{onGraph: "neo4j"}, wantMsg: "action REMOVE LABEL requires --node-label"},
		{name: "node-label and rel-type both set", verb: "GRANT", action: "read", opts: privilegeOpts{onGraph: "*", nodeLabels: []string{"Person"}, relTypes: []string{"KNOWS"}}, wantMsg: "--node-label and --relationship-type are mutually exclusive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canonical := canonicalAction(tc.action)
			_, _, err := buildPrivilegeCypher(tc.verb, validActions[canonical], canonical, tc.opts)
			if err == nil {
				t.Fatal("expected usage error, got nil")
			}
			var ce *clierr.CLIError
			if !errors.As(err, &ce) || ce.Code != 2 {
				t.Fatalf("error = %v, want usage error (code 2)", err)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantMsg)
			}
		})
	}
}
