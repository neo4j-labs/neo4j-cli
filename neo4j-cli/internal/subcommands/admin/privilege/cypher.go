// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clierr"
)

// buildPrivilegeCypher constructs the privilege Cypher fragment (without the
// CYPHER 25 prefix, which RunAdminStatement prepends, and without the trailing
// TO/FROM $role target, which the leaf command appends) for the given verb
// ("GRANT", "DENY", "REVOKE", "REVOKE GRANT", or "REVOKE DENY"), resolved
// category, and canonical action. Action and resource are inlined as keywords to
// match how Neo4j parses privilege Cypher; the returned params map is non-nil and
// empty (only $role, added by the caller, is parameterised).
//
// Precondition: action is already canonical and belongs to cat (the only caller,
// runPrivilegeMutation, passes the resolveCategoryAction result). Each category
// leaf registers only categoryMeta[cat].flags, so cross-scope flag combinations
// are structurally unrepresentable here; the only remaining validation is the two
// in-category qualifier rules for label/entity scopes. The cat assertion guards
// against future misuse of the helper, not against any reachable invocation.
func buildPrivilegeCypher(verb string, cat actionCategory, action string, opts privilegeOpts) (string, map[string]any, error) {
	if got, ok := validActions[action]; !ok || got != cat {
		return "", nil, fmt.Errorf("buildPrivilegeCypher: action %q does not belong to category %d", action, cat)
	}

	if len(opts.nodeLabels) > 0 && len(opts.relTypes) > 0 {
		return "", nil, clierr.NewUsageError("--node-label and --relationship-type are mutually exclusive")
	}

	var clause string
	switch cat {
	case propertyBearer, graphOnly:
		graph := opts.onGraph
		if graph == "" {
			graph = "*"
		}
		prop := ""
		if cat == propertyBearer {
			prop = " " + propertyClause(opts.properties)
		}
		clause = action + prop + " ON GRAPH " + cypherIdentifier(graph) + " " + entityClause(opts.nodeLabels, opts.relTypes)
	case graphWhole:
		graph := opts.onGraph
		if graph == "" {
			graph = "*"
		}
		clause = action + " ON GRAPH " + cypherIdentifier(graph)
	case labelScoped:
		if len(opts.nodeLabels) == 0 {
			return "", nil, clierr.NewUsageError("action %s requires --node-label", action)
		}
		graph := opts.onGraph
		if graph == "" {
			graph = "*"
		}
		clause = action + " " + strings.Join(escapeIdentifiers(opts.nodeLabels), ", ") + " ON GRAPH " + cypherIdentifier(graph)
	case database:
		db := opts.onDatabase
		if db == "" {
			db = "*"
		}
		clause = action + " ON DATABASE " + cypherIdentifier(db)
	case dbms:
		clause = action + " ON DBMS"
	case load:
		if opts.cidr == "" {
			clause = action + " ON ALL DATA"
		} else {
			clause = action + " ON CIDR " + cypherStringLiteral(opts.cidr)
		}
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

// cypherStringLiteral renders value as a double-quoted Cypher string literal,
// escaping embedded backslashes and double quotes. Used for the LOAD ON CIDR
// target, which is a string literal in Cypher — NOT an identifier, so it is
// double-quoted rather than backtick-quoted.
func cypherStringLiteral(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func escapeIdentifiers(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = cypherIdentifier(id)
	}
	return out
}
