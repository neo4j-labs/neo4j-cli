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

// addPrivilegeFlags registers the flag surface shared by grant, deny, and
// revoke onto cmd. verbWord ("grant", "deny", or "revoke") parameterises the
// usage strings; the per-command --revoke-type flag is registered by revoke
// itself.
func addPrivilegeFlags(cmd *cobra.Command, action, roleName *string, opts *privilegeOpts, verbWord string) {
	cmd.Flags().StringVar(action, "action", "", "Privilege action to "+verbWord+" (e.g. read, traverse, create_role)")
	cmd.Flags().StringVar(roleName, "role", "", "Name of the role to "+verbWord+" the privilege "+rolePreposition(verbWord))
	cmd.Flags().StringVar(&opts.onGraph, "on-graph", "", "Scope the privilege to a graph (use * for all)")
	cmd.Flags().StringVar(&opts.onDatabase, "on-database", "", "Scope the privilege to a database (use * for all)")
	cmd.Flags().BoolVar(&opts.onDbms, "on-dbms", false, "Scope the privilege to the DBMS")
	cmd.Flags().StringArrayVar(&opts.nodeLabels, "node-label", nil, "Restrict a graph privilege to node labels")
	cmd.Flags().StringArrayVar(&opts.relTypes, "relationship-type", nil, "Restrict a graph privilege to relationship types")
	cmd.Flags().StringArrayVar(&opts.properties, "property", nil, "Restrict a property privilege to properties")
}

func rolePreposition(verbWord string) string {
	if verbWord == "revoke" {
		return "from"
	}
	return "to"
}

// runPrivilegeMutation runs the shared write sequence: required-flag checks,
// buildPrivilegeCypher, SilenceUsage, exec via the seam (appending the role
// target with the given keyword, "TO" or "FROM"), then outputPrivileges. verb
// is the resolved privilege verb ("GRANT", "DENY", "REVOKE", ...).
func runPrivilegeMutation(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, verb, action, roleName string, opts privilegeOpts, target string) error {
	if action == "" {
		return clierr.NewUsageError("--action is required")
	}
	if roleName == "" {
		return clierr.NewUsageError("--role is required")
	}

	cypher, params, err := buildPrivilegeCypher(verb, action, opts)
	if err != nil {
		return err
	}
	cmd.SilenceUsage = true

	params["role"] = roleName
	if _, err := privilegeExecFn(cmd.Context(), cfg, conn, cypher+" "+target+" $role", params); err != nil {
		return err
	}
	return outputPrivileges(cmd, cfg, conn, roleName)
}

// actionCategory classifies a privilege action keyword. The category controls
// which resource scope and qualifier flags are valid and which Cypher template
// is emitted.
type actionCategory int

const (
	propertyBearer actionCategory = iota
	graphOnly
	graphWhole
	labelScoped
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

	"TRAVERSE": graphOnly,
	"CREATE":   graphOnly,
	"DELETE":   graphOnly,
	"LOAD":     graphOnly,

	"WRITE":                graphWhole,
	"ALL GRAPH PRIVILEGES": graphWhole,

	"SET LABEL":    labelScoped,
	"REMOVE LABEL": labelScoped,

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

	if category == graphWhole && (hasNodeLabel || hasRelType || hasProperty) {
		return "", nil, clierr.NewUsageError("%s does not accept node-label, relationship-type, or property qualifiers", normalized)
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
		clause = normalized + prop + " ON GRAPH " + cypherIdentifier(graph) + " " + entityClause(opts.nodeLabels, opts.relTypes)
	case graphWhole:
		if hasDatabase || opts.onDbms {
			return "", nil, clierr.NewUsageError("action %s is a graph privilege and accepts only --on-graph", normalized)
		}
		graph := opts.onGraph
		if graph == "" {
			graph = "*"
		}
		clause = normalized + " ON GRAPH " + cypherIdentifier(graph)
	case labelScoped:
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
		clause = normalized + " " + cypherIdentifier(opts.nodeLabels[0]) + " ON GRAPH " + cypherIdentifier(graph)
	case database:
		if hasGraph || opts.onDbms {
			return "", nil, clierr.NewUsageError("action %s is a database privilege and accepts only --on-database", normalized)
		}
		db := opts.onDatabase
		if db == "" {
			db = "*"
		}
		clause = normalized + " ON DATABASE " + cypherIdentifier(db)
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
	return "{" + strings.Join(escapeIdentifiers(properties), ", ") + "}"
}

// entityClause renders the entity qualifier shared by propertyBearer and
// graphOnly actions: ELEMENTS * when neither labels nor types, NODES l1, l2
// when only labels, RELATIONSHIPS t1, t2 when only types.
func entityClause(nodeLabels, relTypes []string) string {
	switch {
	case len(nodeLabels) > 0:
		return "NODES " + strings.Join(escapeIdentifiers(nodeLabels), ", ")
	case len(relTypes) > 0:
		return "RELATIONSHIPS " + strings.Join(escapeIdentifiers(relTypes), ", ")
	default:
		return "ELEMENTS *"
	}
}

// cypherIdentifier renders a user-supplied identifier for safe inlining into
// privilege Cypher. The * wildcard is returned unchanged (it is a keyword, not
// an identifier). Every other value is backtick-quoted with any internal
// backtick doubled, so Neo4j treats it as an opaque identifier — blocking
// comment (--) and keyword injection through flag values.
func cypherIdentifier(id string) string {
	if id == "*" {
		return id
	}
	return "`" + strings.ReplaceAll(id, "`", "``") + "`"
}

func escapeIdentifiers(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = cypherIdentifier(id)
	}
	return out
}
