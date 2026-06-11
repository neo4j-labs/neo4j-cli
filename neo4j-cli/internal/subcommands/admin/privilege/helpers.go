// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"fmt"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dbconn"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/admin/adminutil"
	"github.com/spf13/cobra"
)

// privilegeExecFn is the package-level test seam. It is set by NewCmd in
// production and replaced by tests to inject fake results without a real Bolt
// connection.
var privilegeExecFn adminutil.ExecFn

// actionCategory classifies each privilege action by which Cypher resource
// clause it requires. The category drives both Cypher generation and flag
// validation in buildPrivilegeCypher.
type actionCategory int

const (
	// catPropertyBearer actions (READ, MATCH, SET PROPERTY, MERGE) emit a
	// property qualifier clause and an entity clause inside ON GRAPH.
	catPropertyBearer actionCategory = iota
	// catGraphOnly actions (TRAVERSE, WRITE, CREATE, DELETE, LOAD,
	// ALL GRAPH PRIVILEGES) emit an entity clause on ON GRAPH but never a
	// property qualifier. --property is a usage error.
	catGraphOnly
	// catSetLabel / catRemoveLabel actions emit "SET LABEL <label> ON GRAPH <g>"
	// or "REMOVE LABEL <label> ON GRAPH <g>" using the first --node-label value.
	// --node-label is required; --property is forbidden.
	catSetLabel
	catRemoveLabel
	// catDatabase actions are scoped to ON DATABASE and may not be combined with
	// --on-graph, --on-dbms, or any entity/property qualifier.
	catDatabase
	// catDbms actions are scoped to ON DBMS only. --on-dbms is required.
	catDbms
)

// privilegeCategory maps every valid action keyword (canonical form) to its
// category. The map is the single authoritative source for both validation and
// Cypher generation.
var privilegeCategory = map[string]actionCategory{
	// catPropertyBearer
	"READ":         catPropertyBearer,
	"MATCH":        catPropertyBearer,
	"SET PROPERTY": catPropertyBearer,
	"MERGE":        catPropertyBearer,

	// catGraphOnly
	"TRAVERSE":             catGraphOnly,
	"WRITE":                catGraphOnly,
	"CREATE":               catGraphOnly,
	"DELETE":               catGraphOnly,
	"LOAD":                 catGraphOnly,
	"ALL GRAPH PRIVILEGES": catGraphOnly,

	// catSetLabel / catRemoveLabel
	"SET LABEL":    catSetLabel,
	"REMOVE LABEL": catRemoveLabel,

	// catDatabase
	"ACCESS":                       catDatabase,
	"START":                        catDatabase,
	"STOP":                         catDatabase,
	"CREATE INDEX":                 catDatabase,
	"DROP INDEX":                   catDatabase,
	"SHOW INDEX":                   catDatabase,
	"INDEX MANAGEMENT":             catDatabase,
	"CREATE CONSTRAINT":            catDatabase,
	"DROP CONSTRAINT":              catDatabase,
	"SHOW CONSTRAINT":              catDatabase,
	"CONSTRAINT MANAGEMENT":        catDatabase,
	"CREATE NEW NODE LABEL":        catDatabase,
	"CREATE NEW RELATIONSHIP TYPE": catDatabase,
	"CREATE NEW PROPERTY NAME":     catDatabase,
	"NAME MANAGEMENT":              catDatabase,
	"ALL DATABASE PRIVILEGES":      catDatabase,
	"SHOW TRANSACTION":             catDatabase,
	"TERMINATE TRANSACTION":        catDatabase,
	"TRANSACTION MANAGEMENT":       catDatabase,

	// catDbms
	"CREATE ROLE":                   catDbms,
	"DROP ROLE":                     catDbms,
	"ASSIGN ROLE":                   catDbms,
	"REMOVE ROLE":                   catDbms,
	"SHOW ROLE":                     catDbms,
	"RENAME ROLE":                   catDbms,
	"ROLE MANAGEMENT":               catDbms,
	"CREATE USER":                   catDbms,
	"DROP USER":                     catDbms,
	"SHOW USER":                     catDbms,
	"SET USER STATUS":               catDbms,
	"SET PASSWORDS":                 catDbms,
	"SET USER HOME DATABASE":        catDbms,
	"ALTER USER":                    catDbms,
	"RENAME USER":                   catDbms,
	"USER MANAGEMENT":               catDbms,
	"CREATE DATABASE":               catDbms,
	"DROP DATABASE":                 catDbms,
	"DATABASE MANAGEMENT":           catDbms,
	"SHOW PRIVILEGE":                catDbms,
	"ASSIGN PRIVILEGE":              catDbms,
	"REMOVE PRIVILEGE":              catDbms,
	"PRIVILEGE MANAGEMENT":          catDbms,
	"EXECUTE PROCEDURE":             catDbms,
	"EXECUTE BOOSTED PROCEDURE":     catDbms,
	"EXECUTE ADMIN PROCEDURES":      catDbms,
	"EXECUTE FUNCTION":              catDbms,
	"EXECUTE BOOSTED FUNCTION":      catDbms,
	"ALL ON PROCEDURES":             catDbms,
	"ALL ON FUNCTIONS":              catDbms,
	"IMPERSONATE":                   catDbms,
	"ALL DBMS PRIVILEGES":           catDbms,
	"SERVER MANAGEMENT":             catDbms,
	"COMPOSITE DATABASE MANAGEMENT": catDbms,
	"ALIAS MANAGEMENT":              catDbms,
}

// validActions is the canonical list of Neo4j privilege action keywords
// accepted by GRANT / DENY / REVOKE. The canonical form uses spaces and
// uppercase; the flag value accepts underscores as word separators and is
// case-insensitive. Derived from privilegeCategory keys to avoid duplication.
var validActions []string

func init() {
	validActions = make([]string, 0, len(privilegeCategory))
	for k := range privilegeCategory {
		validActions = append(validActions, k)
	}
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
	_, ok := privilegeCategory[candidate]
	return candidate, ok
}

// buildPrivilegeCypher constructs the core privilege clause used in GRANT,
// DENY, and REVOKE statements. It returns the action + resource + qualifier
// clause (everything after the verb keyword and before TO/FROM) or a
// validation error for mutually-exclusive flag combinations or category
// violations.
//
// The caller is responsible for prepending the verb (GRANT/DENY/REVOKE) and
// appending TO/FROM <role>.
func buildPrivilegeCypher(action string, f privilegeFlags) (string, error) {
	cat, known := privilegeCategory[action]
	if !known {
		return "", clierr.NewUsageError("unknown --action %q; valid actions: %s", action, validActionsHelp())
	}

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

	// --- per-category validation and Cypher construction ---
	switch cat {
	case catPropertyBearer:
		return buildGraphClause(action, f, true), nil

	case catGraphOnly:
		if len(f.properties) > 0 {
			return "", clierr.NewUsageError("--property is not valid for action %q", action)
		}
		return buildGraphClause(action, f, false), nil

	case catSetLabel, catRemoveLabel:
		if len(f.properties) > 0 {
			return "", clierr.NewUsageError("--property is not valid for action %q", action)
		}
		if len(f.nodeLabels) == 0 {
			return "", clierr.NewUsageError("--node-label is required for action %q", action)
		}
		graphName := f.onGraph
		if graphName == "" {
			graphName = "*"
		}
		label := f.nodeLabels[0]
		return fmt.Sprintf("%s %s ON GRAPH %s", action, label, graphName), nil

	case catDatabase:
		if f.onGraph != "" || f.onDbms {
			return "", clierr.NewUsageError("action %q requires --on-database (not --on-graph or --on-dbms)", action)
		}
		if hasGraphQualifiers {
			return "", clierr.NewUsageError("--node-label, --relationship-type, and --property cannot be combined with database-level action %q", action)
		}
		dbName := f.onDatabase
		if dbName == "" {
			dbName = "*"
		}
		return fmt.Sprintf("%s ON DATABASE %s", action, dbName), nil

	case catDbms:
		if !f.onDbms {
			return "", clierr.NewUsageError("action %q requires --on-dbms", action)
		}
		return fmt.Sprintf("%s ON DBMS", action), nil
	}

	// Unreachable: every category is handled above.
	return "", clierr.NewUsageError("unknown action category for %q", action)
}

// buildGraphClause constructs the ON GRAPH resource clause. When
// includeProperty is true (catPropertyBearer), a property qualifier is
// included. Otherwise (catGraphOnly) no property qualifier is emitted.
func buildGraphClause(action string, f privilegeFlags, includeProperty bool) string {
	graphName := f.onGraph
	if graphName == "" {
		graphName = "*"
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

	if includeProperty {
		var propClause string
		if len(f.properties) > 0 {
			propClause = fmt.Sprintf("{%s}", strings.Join(f.properties, ", "))
		} else {
			propClause = "{*}"
		}
		return fmt.Sprintf("%s %s ON GRAPH %s %s", action, propClause, graphName, entityClause)
	}

	return fmt.Sprintf("%s ON GRAPH %s %s", action, graphName, entityClause)
}

// validActionsHelp returns a comma-separated list of valid action names for
// inclusion in usage error messages.
func validActionsHelp() string {
	return strings.Join(validActions, ", ")
}

// emitRolePrivileges fetches SHOW ROLE $name PRIVILEGES via privilegeExecFn
// and prints the result through commonoutput.PrintBodyMap using the same field
// list as privilege list. Called by grant, deny, and revoke after a successful
// mutation to show the role's updated privilege set.
func emitRolePrivileges(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, role string) error {
	rows, err := privilegeExecFn(cmd.Context(), cfg, conn, "SHOW ROLE $name PRIVILEGES", map[string]any{"name": role})
	if err != nil {
		return err
	}
	fields := privilegeFields(rows)
	commonoutput.PrintBodyMap(cmd, cfg, adminutil.Rows(rows), fields)
	return nil
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
			if err != nil {
				return err
			}
			return emitRolePrivileges(cmd, cfg, *conn, roleFlag)
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
