// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"sort"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// privilegeExecFn is the package-level test seam. It is set by NewCmd from the
// injected admin.RunAdminStatement and replaced by tests to avoid real Bolt
// connections.
var privilegeExecFn adminutil.ExecFn

// privilegeFields are the canonical output columns for SHOW PRIVILEGES output.
// The immutable column returned by some Neo4j versions is excluded.
var privilegeFields = []string{"access", "action", "resource", "segment", "role"}

// outputPrivileges executes SHOW ROLE $name PRIVILEGES and prints the result
// with privilegeFields columns. Called after a successful mutation to confirm
// the role's updated privilege list.
func outputPrivileges(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, roleName string) error {
	rows, err := privilegeExecFn(cmd.Context(), cfg, conn, "SHOW ROLE $name PRIVILEGES", map[string]any{"name": roleName})
	if err != nil {
		return err
	}
	commonoutput.PrintBodyMap(cmd, cfg, adminutil.NewRows(rows, privilegeFields), privilegeFields)
	return nil
}

// actionCategory classifies a privilege action keyword. The category controls
// which resource scope and qualifier flags are valid and which Cypher template
// is emitted.
type actionCategory int

const (
	propertyBearer actionCategory = iota
	graphOnly
	setLabel
	removeLabel
	database
	dbms
)

// validActions maps each supported (normalised) privilege action keyword to its
// category. FIND is intentionally absent (REQ-F-013).
var validActions = map[string]actionCategory{
	"READ":         propertyBearer,
	"MATCH":        propertyBearer,
	"SET PROPERTY": propertyBearer,
	"MERGE":        propertyBearer,

	"TRAVERSE":             graphOnly,
	"WRITE":                graphOnly,
	"CREATE":               graphOnly,
	"DELETE":               graphOnly,
	"LOAD":                 graphOnly,
	"ALL GRAPH PRIVILEGES": graphOnly,

	"SET LABEL":    setLabel,
	"REMOVE LABEL": removeLabel,

	"ACCESS":                       database,
	"START":                        database,
	"STOP":                         database,
	"CREATE INDEX":                 database,
	"DROP INDEX":                   database,
	"SHOW INDEX":                   database,
	"INDEX MANAGEMENT":             database,
	"CREATE CONSTRAINT":            database,
	"DROP CONSTRAINT":              database,
	"SHOW CONSTRAINT":              database,
	"CONSTRAINT MANAGEMENT":        database,
	"CREATE NEW NODE LABEL":        database,
	"CREATE NEW RELATIONSHIP TYPE": database,
	"CREATE NEW PROPERTY NAME":     database,
	"NAME MANAGEMENT":              database,
	"ALL DATABASE PRIVILEGES":      database,
	"SHOW TRANSACTION":             database,
	"TERMINATE TRANSACTION":        database,
	"TRANSACTION MANAGEMENT":       database,

	"CREATE ROLE":            dbms,
	"DROP ROLE":              dbms,
	"ASSIGN ROLE":            dbms,
	"REMOVE ROLE":            dbms,
	"SHOW ROLE":              dbms,
	"ROLE MANAGEMENT":        dbms,
	"CREATE USER":            dbms,
	"DROP USER":              dbms,
	"SHOW USER":              dbms,
	"SET USER STATUS":        dbms,
	"SET USER HOME DATABASE": dbms,
	"ALTER USER":             dbms,
	"USER MANAGEMENT":        dbms,
	"CREATE DATABASE":        dbms,
	"DROP DATABASE":          dbms,
	"DATABASE MANAGEMENT":    dbms,
	"SHOW PRIVILEGE":         dbms,
	"PRIVILEGE MANAGEMENT":   dbms,
	"ALL DBMS PRIVILEGES":    dbms,
}

// privilegeOpts captures the resolved flag values shared by grant, deny, and
// revoke. onGraph and onDatabase are considered "set" when non-empty; onDbms
// when true.
type privilegeOpts struct {
	onGraph    string
	onDatabase string
	onDbms     bool
	nodeLabels []string
	relTypes   []string
	properties []string
}

// normalizeAction upper-cases the action and collapses underscores and runs of
// whitespace into single spaces, so "all_graph_privileges" and
// "ALL GRAPH PRIVILEGES" both normalise to "ALL GRAPH PRIVILEGES". It returns a
// usage error listing the valid keywords if the result is not a known action.
func normalizeAction(action string) (string, error) {
	normalized := strings.ToUpper(strings.Join(strings.Fields(strings.ReplaceAll(action, "_", " ")), " "))
	if _, ok := validActions[normalized]; !ok {
		return "", clierr.NewUsageError("unknown action %q; valid actions are: %s", action, strings.Join(sortedActions(), ", "))
	}
	return normalized, nil
}

func sortedActions() []string {
	out := make([]string, 0, len(validActions))
	for a := range validActions {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// buildPrivilegeCypher constructs the privilege Cypher fragment (without the
// CYPHER 25 prefix, which RunAdminStatement prepends, and without the trailing
// TO/FROM $role target, which the leaf command appends) for the given verb
// ("GRANT", "DENY", "REVOKE", "REVOKE GRANT", or "REVOKE DENY"), action, and
// flag options. Action and resource are inlined as keywords to match how Neo4j
// parses privilege Cypher; the returned params map is non-nil and empty (only
// $role, added by the caller, is parameterised). It returns a usage error for
// any invalid flag/category combination (REQ-F-011) before producing Cypher.
func buildPrivilegeCypher(verb, action string, opts privilegeOpts) (string, map[string]any, error) {
	normalized, err := normalizeAction(action)
	if err != nil {
		return "", nil, err
	}
	category := validActions[normalized]

	hasGraph := opts.onGraph != ""
	hasDatabase := opts.onDatabase != ""
	hasNodeLabel := len(opts.nodeLabels) > 0
	hasRelType := len(opts.relTypes) > 0
	hasProperty := len(opts.properties) > 0

	scopeCount := 0
	if hasGraph {
		scopeCount++
	}
	if hasDatabase {
		scopeCount++
	}
	if opts.onDbms {
		scopeCount++
	}
	if scopeCount > 1 {
		return "", nil, clierr.NewUsageError("--on-graph, --on-database, and --on-dbms are mutually exclusive")
	}

	if hasNodeLabel && hasRelType {
		return "", nil, clierr.NewUsageError("--node-label and --relationship-type are mutually exclusive")
	}

	if hasProperty && category != propertyBearer {
		return "", nil, clierr.NewUsageError("%s does not accept a property qualifier", normalized)
	}

	var clause string
	switch category {
	case propertyBearer, graphOnly:
		if hasDatabase || opts.onDbms {
			return "", nil, clierr.NewUsageError("action %s is a graph privilege and accepts only --on-graph", normalized)
		}
		graph := opts.onGraph
		if graph == "" {
			graph = "*"
		}
		prop := ""
		if category == propertyBearer {
			prop = " " + propertyClause(opts.properties)
		}
		clause = normalized + prop + " ON GRAPH " + graph + " " + entityClause(opts.nodeLabels, opts.relTypes)
	case setLabel, removeLabel:
		if hasDatabase || opts.onDbms {
			return "", nil, clierr.NewUsageError("action %s is a graph privilege and accepts only --on-graph", normalized)
		}
		if !hasNodeLabel {
			return "", nil, clierr.NewUsageError("action %s requires --node-label", normalized)
		}
		graph := opts.onGraph
		if graph == "" {
			graph = "*"
		}
		clause = normalized + " " + opts.nodeLabels[0] + " ON GRAPH " + graph
	case database:
		if hasGraph || opts.onDbms {
			return "", nil, clierr.NewUsageError("action %s is a database privilege and accepts only --on-database", normalized)
		}
		db := opts.onDatabase
		if db == "" {
			db = "*"
		}
		clause = normalized + " ON DATABASE " + db
	case dbms:
		if hasGraph || hasDatabase {
			return "", nil, clierr.NewUsageError("action %s is a DBMS privilege and accepts only --on-dbms", normalized)
		}
		if !opts.onDbms {
			return "", nil, clierr.NewUsageError("action %s requires --on-dbms", normalized)
		}
		clause = normalized + " ON DBMS"
	}

	return verb + " " + clause, map[string]any{}, nil
}

// propertyClause renders the property qualifier for propertyBearer actions:
// {*} when no properties, {p1, p2} otherwise.
func propertyClause(properties []string) string {
	if len(properties) == 0 {
		return "{*}"
	}
	return "{" + strings.Join(properties, ", ") + "}"
}

// entityClause renders the entity qualifier shared by propertyBearer and
// graphOnly actions: ELEMENTS * when neither labels nor types, NODES l1, l2
// when only labels, RELATIONSHIPS t1, t2 when only types.
func entityClause(nodeLabels, relTypes []string) string {
	switch {
	case len(nodeLabels) > 0:
		return "NODES " + strings.Join(nodeLabels, ", ")
	case len(relTypes) > 0:
		return "RELATIONSHIPS " + strings.Join(relTypes, ", ")
	default:
		return "ELEMENTS *"
	}
}
