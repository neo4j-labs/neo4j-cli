// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// privilegeExecFn is the package-level test seam. It is set by NewCmd in
// production and replaced by tests to inject fake results without a real Bolt
// connection.
var privilegeExecFn adminutil.ExecFn

// validActions is the canonical list of Neo4j privilege action keywords
// accepted by GRANT / DENY / REVOKE. The canonical form uses spaces and
// uppercase; the flag value accepts underscores as word separators and is
// case-insensitive.
var validActions = []string{
	// Graph / element privileges
	"ACCESS",
	"FIND",
	"READ",
	"WRITE",
	"TRAVERSE",
	"MATCH",
	"MERGE",
	"CREATE",
	"DELETE",
	"SET LABEL",
	"REMOVE LABEL",
	"SET PROPERTY",
	"ALL GRAPH PRIVILEGES",
	"LOAD",
	// Database privileges
	"START",
	"STOP",
	"CREATE INDEX",
	"DROP INDEX",
	"SHOW INDEX",
	"INDEX MANAGEMENT",
	"CREATE CONSTRAINT",
	"DROP CONSTRAINT",
	"SHOW CONSTRAINT",
	"CONSTRAINT MANAGEMENT",
	"CREATE NEW NODE LABEL",
	"CREATE NEW RELATIONSHIP TYPE",
	"CREATE NEW PROPERTY NAME",
	"NAME MANAGEMENT",
	"ALL DATABASE PRIVILEGES",
	"SHOW TRANSACTION",
	"TERMINATE TRANSACTION",
	"TRANSACTION MANAGEMENT",
	// DBMS privileges
	"CREATE ROLE",
	"DROP ROLE",
	"ASSIGN ROLE",
	"REMOVE ROLE",
	"SHOW ROLE",
	"RENAME ROLE",
	"ROLE MANAGEMENT",
	"CREATE USER",
	"DROP USER",
	"SHOW USER",
	"SET USER STATUS",
	"SET PASSWORDS",
	"SET USER HOME DATABASE",
	"ALTER USER",
	"RENAME USER",
	"USER MANAGEMENT",
	"CREATE DATABASE",
	"DROP DATABASE",
	"DATABASE MANAGEMENT",
	"SHOW PRIVILEGE",
	"ASSIGN PRIVILEGE",
	"REMOVE PRIVILEGE",
	"PRIVILEGE MANAGEMENT",
	"EXECUTE PROCEDURE",
	"EXECUTE BOOSTED PROCEDURE",
	"EXECUTE ADMIN PROCEDURES",
	"EXECUTE FUNCTION",
	"EXECUTE BOOSTED FUNCTION",
	"ALL ON PROCEDURES",
	"ALL ON FUNCTIONS",
	"IMPERSONATE",
	"ALL DBMS PRIVILEGES",
	"SERVER MANAGEMENT",
	"COMPOSITE DATABASE MANAGEMENT",
	"ALIAS MANAGEMENT",
}

// dbmsActions is the set of actions that are scoped to ON DBMS only.
// These cannot be combined with graph-level qualifiers (--node-label,
// --relationship-type, --property).
var dbmsActions = map[string]bool{
	"CREATE ROLE":                   true,
	"DROP ROLE":                     true,
	"ASSIGN ROLE":                   true,
	"REMOVE ROLE":                   true,
	"SHOW ROLE":                     true,
	"RENAME ROLE":                   true,
	"ROLE MANAGEMENT":               true,
	"CREATE USER":                   true,
	"DROP USER":                     true,
	"SHOW USER":                     true,
	"SET USER STATUS":               true,
	"SET PASSWORDS":                 true,
	"SET USER HOME DATABASE":        true,
	"ALTER USER":                    true,
	"RENAME USER":                   true,
	"USER MANAGEMENT":               true,
	"CREATE DATABASE":               true,
	"DROP DATABASE":                 true,
	"DATABASE MANAGEMENT":           true,
	"SHOW PRIVILEGE":                true,
	"ASSIGN PRIVILEGE":              true,
	"REMOVE PRIVILEGE":              true,
	"PRIVILEGE MANAGEMENT":          true,
	"EXECUTE PROCEDURE":             true,
	"EXECUTE BOOSTED PROCEDURE":     true,
	"EXECUTE ADMIN PROCEDURES":      true,
	"EXECUTE FUNCTION":              true,
	"EXECUTE BOOSTED FUNCTION":      true,
	"ALL ON PROCEDURES":             true,
	"ALL ON FUNCTIONS":              true,
	"IMPERSONATE":                   true,
	"ALL DBMS PRIVILEGES":           true,
	"SERVER MANAGEMENT":             true,
	"COMPOSITE DATABASE MANAGEMENT": true,
	"ALIAS MANAGEMENT":              true,
}

// privilegeFlags holds the resource and qualifier flags shared by grant, deny,
// and revoke.
type privilegeFlags struct {
	onGraph    string
	onDatabase string
	onDbms     bool

	nodeLabels        []string
	relationshipTypes []string
	properties        []string
}

// normalizeAction converts a user-supplied action string (case-insensitive,
// underscore-or-space separator) to the canonical uppercase space-separated
// form used in Neo4j Cypher. Returns the normalized string and true if it
// matched a known action, or the normalized string and false if unrecognized.
func normalizeAction(raw string) (string, bool) {
	// Replace underscores with spaces and uppercase the whole thing.
	candidate := strings.ToUpper(strings.ReplaceAll(raw, "_", " "))
	// Collapse multiple spaces (in case the user passed "ALL  GRAPH" etc.)
	candidate = strings.Join(strings.Fields(candidate), " ")
	for _, a := range validActions {
		if a == candidate {
			return candidate, true
		}
	}
	return candidate, false
}

// buildPrivilegeCypher constructs the core privilege clause used in GRANT,
// DENY, and REVOKE statements. It returns the resource + qualifier clause
// (everything after the action keyword and before TO/FROM) or a validation
// error for mutually-exclusive flag combinations.
//
// The caller is responsible for prepending the verb (GRANT/DENY/REVOKE) and
// action and appending TO/FROM <role>.
//
// Examples:
//
//	buildPrivilegeCypher("READ", privilegeFlags{onGraph:"*"})
//	  → "READ {*} ON GRAPH * ELEMENTS *", nil
//
//	buildPrivilegeCypher("READ", privilegeFlags{onGraph:"neo4j", nodeLabels:["Person"], properties:["name"]})
//	  → "READ {name} ON GRAPH neo4j NODES Person", nil
func buildPrivilegeCypher(action string, f privilegeFlags) (string, error) {
	// --- resource exclusivity ---
	resourceCount := 0
	if f.onGraph != "" {
		resourceCount++
	}
	if f.onDatabase != "" {
		resourceCount++
	}
	if f.onDbms {
		resourceCount++
	}
	if resourceCount > 1 {
		return "", clierr.NewUsageError("--on-graph, --on-database, and --on-dbms are mutually exclusive")
	}

	// --- entity qualifier exclusivity ---
	if len(f.nodeLabels) > 0 && len(f.relationshipTypes) > 0 {
		return "", clierr.NewUsageError("--node-label and --relationship-type are mutually exclusive")
	}

	// --- graph-scoped qualifiers must not appear with --on-database / --on-dbms ---
	hasGraphQualifiers := len(f.nodeLabels) > 0 || len(f.relationshipTypes) > 0 || len(f.properties) > 0
	if hasGraphQualifiers && (f.onDatabase != "" || f.onDbms) {
		return "", clierr.NewUsageError("--node-label, --relationship-type, and --property are only valid with --on-graph")
	}

	// --- DBMS-level action + graph qualifiers ---
	if dbmsActions[action] && hasGraphQualifiers {
		return "", clierr.NewUsageError("--property, --node-label, and --relationship-type cannot be combined with DBMS-level action %q", action)
	}

	// --- build resource clause ---
	var resource string
	switch {
	case f.onDbms:
		resource = "ON DBMS"
	case f.onDatabase != "":
		resource = fmt.Sprintf("ON DATABASE %s", f.onDatabase)
	default:
		// ON GRAPH (default)
		graphName := f.onGraph
		if graphName == "" {
			graphName = "*"
		}

		// property clause
		var propClause string
		if len(f.properties) > 0 {
			propClause = fmt.Sprintf("{%s}", strings.Join(f.properties, ", "))
		} else {
			propClause = "{*}"
		}

		// entity clause
		var entityClause string
		switch {
		case len(f.nodeLabels) > 0:
			entityClause = fmt.Sprintf("NODES %s", strings.Join(f.nodeLabels, ", "))
		case len(f.relationshipTypes) > 0:
			entityClause = fmt.Sprintf("RELATIONSHIPS %s", strings.Join(f.relationshipTypes, ", "))
		default:
			entityClause = "ELEMENTS *"
		}

		resource = fmt.Sprintf("%s ON GRAPH %s %s", propClause, graphName, entityClause)
	}

	return fmt.Sprintf("%s %s", action, resource), nil
}

// validActionsHelp returns a comma-separated list of valid action names for
// inclusion in usage error messages.
func validActionsHelp() string {
	return strings.Join(validActions, ", ")
}

// newPrivilegeMutationCmd is the shared constructor for grant and deny. verb is
// "GRANT" or "DENY" and drives the Cypher prefix, help text, and examples.
func newPrivilegeMutationCmd(cfg *clicfg.Config, conn **dbconn.Conn, verb, short, long, example string) *cobra.Command {
	var actionFlag string
	var roleFlag string
	var flags privilegeFlags

	lverb := strings.ToLower(verb)

	cmd := &cobra.Command{
		Use:         lverb,
		Short:       short,
		Annotations: map[string]string{"write": "true"},
		Long:        long,
		Example:     example,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			normalized, ok := normalizeAction(actionFlag)
			if !ok {
				return clierr.NewUsageError("unknown --action %q; valid actions: %s", actionFlag, validActionsHelp())
			}

			clause, err := buildPrivilegeCypher(normalized, flags)
			if err != nil {
				return err
			}

			cypher := fmt.Sprintf("%s %s TO $role", verb, clause)
			_, err = privilegeExecFn(cmd.Context(), cfg, *conn, cypher, map[string]any{"role": roleFlag})
			return err
		},
	}

	cmd.Flags().StringVar(&actionFlag, "action", "", "Privilege action keyword (required; e.g. read, write, access, create_role)")
	cmd.Flags().StringVar(&roleFlag, "role", "", fmt.Sprintf("Role name to %s the privilege to (required)", lverb))
	cmd.Flags().StringVar(&flags.onGraph, "on-graph", "", "Target a specific graph by name (default: * when no resource flag is set)")
	cmd.Flags().StringVar(&flags.onDatabase, "on-database", "", "Target a specific database by name")
	cmd.Flags().BoolVar(&flags.onDbms, "on-dbms", false, "Target the DBMS (for DBMS-level privileges)")
	cmd.Flags().StringArrayVar(&flags.nodeLabels, "node-label", nil, "Node label qualifier (repeatable; only valid with --on-graph)")
	cmd.Flags().StringArrayVar(&flags.relationshipTypes, "relationship-type", nil, "Relationship type qualifier (repeatable; only valid with --on-graph)")
	cmd.Flags().StringArrayVar(&flags.properties, "property", nil, "Property name qualifier (repeatable; only valid with --on-graph; default: all properties)")

	_ = cmd.MarkFlagRequired("action")
	_ = cmd.MarkFlagRequired("role")

	return cmd
}
