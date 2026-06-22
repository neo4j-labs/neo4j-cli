// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"errors"
	"strings"
	"testing"

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
			want:   "GRANT READ {name} ON GRAPH neo4j NODES Person",
		},
		{
			name:   "propertyBearer multi property and rel type",
			verb:   "GRANT",
			action: "MATCH",
			opts:   privilegeOpts{onGraph: "neo4j", relTypes: []string{"KNOWS"}, properties: []string{"weight", "since"}},
			want:   "GRANT MATCH {weight, since} ON GRAPH neo4j RELATIONSHIPS KNOWS",
		},
		{
			name:   "propertyBearer default graph when on-graph absent",
			verb:   "GRANT",
			action: "READ",
			opts:   privilegeOpts{nodeLabels: []string{"Person"}},
			want:   "GRANT READ {*} ON GRAPH * NODES Person",
		},
		{
			name:   "graphOnly default",
			verb:   "GRANT",
			action: "TRAVERSE",
			opts:   privilegeOpts{onGraph: "*"},
			want:   "GRANT TRAVERSE ON GRAPH * ELEMENTS *",
		},
		{
			name:   "graphOnly normalized all graph privileges",
			verb:   "GRANT",
			action: "all_graph_privileges",
			opts:   privilegeOpts{onGraph: "neo4j"},
			want:   "GRANT ALL GRAPH PRIVILEGES ON GRAPH neo4j ELEMENTS *",
		},
		{
			name:   "setLabel",
			verb:   "GRANT",
			action: "set_label",
			opts:   privilegeOpts{onGraph: "neo4j", nodeLabels: []string{"Person"}},
			want:   "GRANT SET LABEL Person ON GRAPH neo4j",
		},
		{
			name:   "removeLabel",
			verb:   "GRANT",
			action: "remove_label",
			opts:   privilegeOpts{onGraph: "neo4j", nodeLabels: []string{"Person"}},
			want:   "GRANT REMOVE LABEL Person ON GRAPH neo4j",
		},
		{
			name:   "database with explicit database",
			verb:   "GRANT",
			action: "access",
			opts:   privilegeOpts{onDatabase: "neo4j"},
			want:   "GRANT ACCESS ON DATABASE neo4j",
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
		{name: "database with property", verb: "GRANT", action: "access", opts: privilegeOpts{properties: []string{"name"}}, wantMsg: "ACCESS does not accept a property qualifier"},
		{name: "node-label and rel-type both set", verb: "GRANT", action: "read", opts: privilegeOpts{onGraph: "*", nodeLabels: []string{"Person"}, relTypes: []string{"KNOWS"}}, wantMsg: "--node-label and --relationship-type are mutually exclusive"},
		{name: "two scope flags set", verb: "GRANT", action: "read", opts: privilegeOpts{onGraph: "*", onDatabase: "neo4j"}, wantMsg: "--on-graph, --on-database, and --on-dbms are mutually exclusive"},
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
