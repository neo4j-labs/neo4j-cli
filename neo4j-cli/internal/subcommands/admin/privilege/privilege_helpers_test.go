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

func TestNormalizeAction(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{name: "underscore form", input: "all_graph_privileges", want: "ALL GRAPH PRIVILEGES", wantOK: true},
		{name: "spaced upper form", input: "ALL GRAPH PRIVILEGES", want: "ALL GRAPH PRIVILEGES", wantOK: true},
		{name: "lower single word", input: "read", want: "READ", wantOK: true},
		{name: "mixed case underscore", input: "Set_Label", want: "SET LABEL", wantOK: true},
		{name: "extra whitespace", input: "  create   role ", want: "CREATE ROLE", wantOK: true},
		{name: "unknown action", input: "find", wantOK: false},
		{name: "empty", input: "", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeAction(tc.input)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("normalizeAction(%q) returned error: %v", tc.input, err)
				}
				if got != tc.want {
					t.Fatalf("normalizeAction(%q) = %q, want %q", tc.input, got, tc.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("normalizeAction(%q) = %q, want error", tc.input, got)
			}
			var ce *clierr.CLIError
			if !errors.As(err, &ce) || ce.Code != 2 {
				t.Fatalf("normalizeAction(%q) error = %v, want usage error (code 2)", tc.input, err)
			}
		})
	}
}

func TestValidActions_FindAbsent(t *testing.T) {
	if _, ok := validActions["FIND"]; ok {
		t.Fatal("FIND must not be a valid action (REQ-F-013)")
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
			opts:   privilegeOpts{onDbms: true},
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
			got, params, err := buildPrivilegeCypher(tc.verb, tc.action, tc.opts)
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
			got, _, err := buildPrivilegeCypher(tc.verb, tc.action, tc.opts)
			if err != nil {
				t.Fatalf("buildPrivilegeCypher returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("cypher = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildPrivilegeCypher_Errors(t *testing.T) {
	cases := []struct {
		name    string
		verb    string
		action  string
		opts    privilegeOpts
		wantMsg string
	}{
		{name: "unknown action", verb: "GRANT", action: "find", opts: privilegeOpts{onGraph: "*"}, wantMsg: "unknown action"},
		{name: "graphOnly with on-database", verb: "GRANT", action: "traverse", opts: privilegeOpts{onDatabase: "neo4j"}},
		{name: "propertyBearer with on-dbms", verb: "GRANT", action: "read", opts: privilegeOpts{onDbms: true}},
		{name: "database with on-graph", verb: "GRANT", action: "access", opts: privilegeOpts{onGraph: "neo4j"}},
		{name: "database with on-dbms", verb: "GRANT", action: "access", opts: privilegeOpts{onDbms: true}},
		{name: "dbms missing on-dbms", verb: "GRANT", action: "create_role", opts: privilegeOpts{}, wantMsg: "action CREATE ROLE requires --on-dbms"},
		{name: "dbms with on-graph", verb: "GRANT", action: "create_role", opts: privilegeOpts{onGraph: "neo4j"}},
		{name: "dbms with on-database", verb: "GRANT", action: "create_role", opts: privilegeOpts{onDatabase: "neo4j"}},
		{name: "setLabel missing node-label", verb: "GRANT", action: "set_label", opts: privilegeOpts{onGraph: "neo4j"}, wantMsg: "action SET LABEL requires --node-label"},
		{name: "removeLabel missing node-label", verb: "GRANT", action: "remove_label", opts: privilegeOpts{onGraph: "neo4j"}, wantMsg: "action REMOVE LABEL requires --node-label"},
		{name: "graphOnly with property", verb: "GRANT", action: "traverse", opts: privilegeOpts{onGraph: "*", properties: []string{"name"}}, wantMsg: "TRAVERSE does not accept a property qualifier"},
		{name: "graphWhole with node-label", verb: "GRANT", action: "write", opts: privilegeOpts{onGraph: "*", nodeLabels: []string{"Person"}}, wantMsg: "WRITE does not accept node-label, relationship-type, or property qualifiers"},
		{name: "graphWhole with relationship-type", verb: "GRANT", action: "all_graph_privileges", opts: privilegeOpts{onGraph: "*", relTypes: []string{"KNOWS"}}, wantMsg: "ALL GRAPH PRIVILEGES does not accept node-label, relationship-type, or property qualifiers"},
		{name: "graphWhole with property", verb: "GRANT", action: "write", opts: privilegeOpts{onGraph: "*", properties: []string{"name"}}, wantMsg: "WRITE does not accept node-label, relationship-type, or property qualifiers"},
		{name: "graphWhole with on-database", verb: "GRANT", action: "write", opts: privilegeOpts{onDatabase: "neo4j"}, wantMsg: "action WRITE is a graph privilege and accepts only --on-graph"},
		{name: "graphWhole with on-dbms", verb: "GRANT", action: "write", opts: privilegeOpts{onDbms: true}, wantMsg: "action WRITE is a graph privilege and accepts only --on-graph"},
		{name: "database with property", verb: "GRANT", action: "access", opts: privilegeOpts{properties: []string{"name"}}, wantMsg: "ACCESS does not accept a property qualifier"},
		{name: "node-label and rel-type both set", verb: "GRANT", action: "read", opts: privilegeOpts{onGraph: "*", nodeLabels: []string{"Person"}, relTypes: []string{"KNOWS"}}, wantMsg: "--node-label and --relationship-type are mutually exclusive"},
		{name: "two scope flags set", verb: "GRANT", action: "read", opts: privilegeOpts{onGraph: "*", onDatabase: "neo4j"}, wantMsg: "--on-graph, --on-database, and --on-dbms are mutually exclusive"},
		{name: "cidr with non-load action", verb: "GRANT", action: "read", opts: privilegeOpts{onGraph: "*", cidr: "127.0.0.1/32"}, wantMsg: "--cidr is only valid for the LOAD action"},
		{name: "load with on-graph", verb: "GRANT", action: "load", opts: privilegeOpts{onGraph: "*"}, wantMsg: "action LOAD does not accept --on-graph, --on-database, or --on-dbms"},
		{name: "load with on-database", verb: "GRANT", action: "load", opts: privilegeOpts{onDatabase: "neo4j"}, wantMsg: "action LOAD does not accept --on-graph, --on-database, or --on-dbms"},
		{name: "load with on-dbms", verb: "GRANT", action: "load", opts: privilegeOpts{onDbms: true}, wantMsg: "action LOAD does not accept --on-graph, --on-database, or --on-dbms"},
		{name: "load with node-label", verb: "GRANT", action: "load", opts: privilegeOpts{nodeLabels: []string{"Person"}}, wantMsg: "action LOAD does not accept node-label or relationship-type qualifiers"},
		{name: "load with relationship-type", verb: "GRANT", action: "load", opts: privilegeOpts{relTypes: []string{"KNOWS"}}, wantMsg: "action LOAD does not accept node-label or relationship-type qualifiers"},
		{name: "load with property", verb: "GRANT", action: "load", opts: privilegeOpts{properties: []string{"name"}}, wantMsg: "LOAD does not accept a property qualifier"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := buildPrivilegeCypher(tc.verb, tc.action, tc.opts)
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
