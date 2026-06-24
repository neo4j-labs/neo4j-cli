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

// registerPrivilegeFlag registers a single privilege flag (by its long name)
// onto cmd, binding it to the matching field of opts. It is the per-flag
// primitive the category-subcommand factory composes from categoryMeta.flags so
// each category gets only the flags valid for it.
func registerPrivilegeFlag(cmd *cobra.Command, opts *privilegeOpts, flag string) {
	switch flag {
	case flagOnGraph:
		cmd.Flags().StringVar(&opts.onGraph, flagOnGraph, "", "Scope the privilege to a graph (use * for all)")
	case flagOnDatabase:
		cmd.Flags().StringVar(&opts.onDatabase, flagOnDatabase, "", "Scope the privilege to a database (use * for all)")
	case flagOnDbms:
		cmd.Flags().BoolVar(&opts.onDbms, flagOnDbms, false, "Scope the privilege to the DBMS")
	case flagNodeLabel:
		cmd.Flags().StringArrayVar(&opts.nodeLabels, flagNodeLabel, nil, "Restrict a graph privilege to node labels")
	case flagRelType:
		cmd.Flags().StringArrayVar(&opts.relTypes, flagRelType, nil, "Restrict a graph privilege to relationship types")
	case flagProperty:
		cmd.Flags().StringArrayVar(&opts.properties, flagProperty, nil, "Restrict a property privilege to properties")
	case flagCidr:
		cmd.Flags().StringVar(&opts.cidr, flagCidr, "", "Scope a LOAD privilege to a CIDR range (LOAD only; defaults to all data)")
	}
}

func rolePreposition(verbWord string) string {
	if verbWord == "revoke" {
		return "from"
	}
	return "to"
}

// newCategoryCmd builds one category leaf for the given verb ("GRANT"/"DENY" or
// a REVOKE verb base) and action category, driven entirely by categoryMeta[cat].
// The seven category commands are structurally identical apart from their
// metadata, so they are data-driven from a single factory rather than each
// hand-authored in its own file. This is the documented exception to the
// one-file-per-leaf rule in AGENTS.md ("Cobra Command Layout") recorded by
// REQ-F-035; grant/deny/revoke each loop over categoryOrder calling this factory.
func newCategoryCmd(cfg *clicfg.Config, conn **dbconn.Conn, verb string, cat actionCategory) *cobra.Command {
	meta := categoryMeta[cat]
	word := verbWord(verb)
	target := "TO"
	if word == "revoke" {
		target = "FROM"
	}

	var roleName, revokeType string
	var opts privilegeOpts

	cmd := &cobra.Command{
		Use:       meta.name + " <action>",
		Short:     verbTitle(word) + " a " + meta.shortNoun + " " + rolePreposition(word) + " a role",
		Long:      categoryLong(word, cat),
		Example:   renderCategoryExample(word, cat),
		ValidArgs: kebabActionsForCategory(cat),
		// ValidArgs drives <TAB> completion only; Args is ExactArgs(1) (not
		// OnlyValidArgs) so a cross-category action reaches RunE and yields the
		// "use 'admin privilege grant database access'" hint (REQ-F-030) rather
		// than cobra's generic invalid-argument message.
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			canonical, err := normalizeAction(args[0])
			if err != nil {
				return err
			}
			if got := validActions[canonical]; got != cat {
				return clierr.NewUsageError(
					"%s is a %s; use 'admin privilege %s %s %s'",
					args[0], categoryMeta[got].shortNoun, word, categoryMeta[got].name, kebabAction(canonical),
				)
			}
			resolvedVerb := verb
			if word == "revoke" {
				resolvedVerb, err = revokeVerb(revokeType)
				if err != nil {
					return err
				}
			}
			return runPrivilegeMutation(cmd, cfg, *conn, resolvedVerb, canonical, roleName, opts, target)
		},
	}
	cmd.Annotations = map[string]string{"write": "true"}

	cmd.Flags().StringVar(&roleName, "role", "", "Name of the role to "+word+" the privilege "+rolePreposition(word))
	if word == "revoke" {
		cmd.Flags().StringVar(&revokeType, "revoke-type", "", "Restrict the revoke to grant or deny privileges (grant|deny); omit to revoke both")
	}
	for _, flag := range meta.flags {
		registerPrivilegeFlag(cmd, &opts, flag)
	}

	return cmd
}

// verbWord maps a privilege verb ("GRANT", "DENY", "REVOKE", "REVOKE GRANT",
// "REVOKE DENY") to the lower-case command word used in help and examples.
func verbWord(verb string) string {
	switch {
	case strings.HasPrefix(verb, "REVOKE"):
		return "revoke"
	case verb == "DENY":
		return "deny"
	default:
		return "grant"
	}
}

func verbTitle(verbWord string) string {
	return strings.ToUpper(verbWord[:1]) + verbWord[1:]
}

// categoryLong builds the Long help for a category subcommand from its metadata:
// the verb, the category's actions, the scope/qualifier rule, and that --role is
// required.
func categoryLong(verbWord string, cat actionCategory) string {
	meta := categoryMeta[cat]
	return verbTitle(verbWord) + " a " + meta.shortNoun + " " + rolePreposition(verbWord) + " a role. " +
		"The action is the positional argument; valid actions are: " + strings.Join(kebabActionsForCategory(cat), ", ") + ". " +
		meta.longRule + " --role is required."
}

// runPrivilegeMutation runs the shared write sequence: the --role check,
// buildPrivilegeCypher, SilenceUsage, exec via the seam (appending the role
// target with the given keyword, "TO" or "FROM"), then outputPrivileges. verb
// is the resolved privilege verb ("GRANT", "DENY", "REVOKE", ...) and action is
// the already-resolved canonical keyword.
func runPrivilegeMutation(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, verb, action, roleName string, opts privilegeOpts, target string) error {
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

// The iota order here is incidental; the user-facing display/registration order
// of the category subcommands lives in categoryOrder.
const (
	propertyBearer actionCategory = iota
	graphOnly
	graphWhole
	labelScoped
	database
	dbms
	load
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

	"WRITE":                graphWhole,
	"ALL GRAPH PRIVILEGES": graphWhole,

	"LOAD": load,

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
	cidr       string
}

// privilege flag long names, shared by registerPrivilegeFlag and categoryMeta so
// the per-category registered-flag sets stay in sync with what
// buildPrivilegeCypher reads.
const (
	flagOnGraph    = "on-graph"
	flagOnDatabase = "on-database"
	flagOnDbms     = "on-dbms"
	flagNodeLabel  = "node-label"
	flagRelType    = "relationship-type"
	flagProperty   = "property"
	flagCidr       = "cidr"
)

// categoryInfo declares the user-facing surface for one action category: the
// kebab command name, the flags valid for every action in the category, which of
// those are required, a Long help fragment describing the category's rules, and
// a representative action + flag values used to render the Example. It is the
// single source (alongside validActions) for the per-category subcommand surface
// introduced by the discoverability redesign (REQ-F-034); the subcommand factory
// in a later task is its only consumer.
type categoryInfo struct {
	name          string
	flags         []string
	requiredFlags []string
	shortNoun     string
	longRule      string
	exampleAction string
	exampleFlags  string
}

// categoryMeta maps each actionCategory to its user-facing surface. The action
// keywords are derived from validActions (see actionsForCategory), so adding an
// action only requires editing validActions; only a brand-new category needs an
// entry here.
var categoryMeta = map[actionCategory]categoryInfo{
	propertyBearer: {
		name:          "property",
		flags:         []string{flagOnGraph, flagNodeLabel, flagRelType, flagProperty},
		shortNoun:     "property privilege",
		longRule:      "Scope with --on-graph (default *); restrict to properties with --property and to entities with --node-label or --relationship-type (mutually exclusive).",
		exampleAction: "read",
		exampleFlags:  "--on-graph * --property name",
	},
	graphOnly: {
		name:          "entity",
		flags:         []string{flagOnGraph, flagNodeLabel, flagRelType},
		shortNoun:     "graph entity privilege",
		longRule:      "Scope with --on-graph (default *); restrict to entities with --node-label or --relationship-type (mutually exclusive). No property qualifier.",
		exampleAction: "traverse",
		exampleFlags:  "--on-graph * --node-label Person",
	},
	graphWhole: {
		name:          "graph",
		flags:         []string{flagOnGraph},
		shortNoun:     "whole-graph privilege",
		longRule:      "Scope with --on-graph (default *). WRITE and ALL GRAPH PRIVILEGES accept no node-label, relationship-type, or property qualifiers.",
		exampleAction: "write",
		exampleFlags:  "--on-graph *",
	},
	labelScoped: {
		name:          "label",
		flags:         []string{flagOnGraph, flagNodeLabel},
		requiredFlags: []string{flagNodeLabel},
		shortNoun:     "label privilege",
		longRule:      "Scope with --on-graph (default *); --node-label is required and may be repeated to cover multiple labels.",
		exampleAction: "set-label",
		exampleFlags:  "--node-label Person --on-graph neo4j",
	},
	load: {
		name:          "load",
		flags:         []string{flagCidr},
		shortNoun:     "LOAD privilege",
		longRule:      "Defaults to ON ALL DATA; restrict to a CIDR range with --cidr. Accepts no scope or entity flags.",
		exampleAction: "load",
		exampleFlags:  "--cidr 127.0.0.1/32",
	},
	database: {
		name:          "database",
		flags:         []string{flagOnDatabase},
		shortNoun:     "database privilege",
		longRule:      "Scope with --on-database (default *).",
		exampleAction: "access",
		exampleFlags:  "--on-database neo4j",
	},
	dbms: {
		name:          "dbms",
		flags:         []string{flagOnDbms},
		requiredFlags: []string{flagOnDbms},
		shortNoun:     "DBMS privilege",
		longRule:      "Requires --on-dbms.",
		exampleAction: "create-role",
		exampleFlags:  "--on-dbms",
	},
}

// categoryOrder fixes the help and registration order of the category
// subcommands under each verb.
var categoryOrder = []actionCategory{
	propertyBearer,
	graphOnly,
	graphWhole,
	labelScoped,
	load,
	database,
	dbms,
}

// actionsForCategory returns the canonical action keywords belonging to cat,
// sorted, so categoryMeta need not duplicate the action lists already in
// validActions.
func actionsForCategory(cat actionCategory) []string {
	out := make([]string, 0, len(validActions))
	for action, c := range validActions {
		if c == cat {
			out = append(out, action)
		}
	}
	sort.Strings(out)
	return out
}

// kebabAction converts a canonical action keyword to its kebab form used for
// ValidArgs and the positional argument, e.g. "SET LABEL" -> "set-label" and
// "ALL GRAPH PRIVILEGES" -> "all-graph-privileges".
func kebabAction(canonical string) string {
	return strings.ToLower(strings.ReplaceAll(canonical, " ", "-"))
}

// kebabActionsForCategory returns the kebab action keywords for cat, used for a
// category subcommand's ValidArgs.
func kebabActionsForCategory(cat actionCategory) []string {
	canonical := actionsForCategory(cat)
	out := make([]string, len(canonical))
	for i, a := range canonical {
		out[i] = kebabAction(a)
	}
	return out
}

// renderCategoryExample builds a flush-left Example block for the given verb word
// ("grant"/"deny"/"revoke") and category, with two invocations: one minimal and
// one writing JSON. Each invocation has a # comment, the neo4j-cli prefix, and
// --rw (the category commands are all write commands). verbWord must be
// non-empty ("grant"/"deny"/"revoke").
func renderCategoryExample(verbWord string, cat actionCategory) string {
	meta := categoryMeta[cat]
	base := "neo4j-cli admin privilege " + verbWord + " " + meta.name + " " + meta.exampleAction
	flags := ""
	if meta.exampleFlags != "" {
		flags = " " + meta.exampleFlags
	}
	title := verbTitle(verbWord)
	return "# " + title + " a " + meta.shortNoun + " to the analyst role\n" +
		base + flags + " --role analyst --credential local --rw\n\n" +
		"# " + title + " the same privilege, output as JSON\n" +
		base + flags + " --role analyst --credential local --rw --format json"
}

// normalizeAction upper-cases the action and collapses underscores, hyphens, and
// runs of whitespace into single spaces, so "all_graph_privileges",
// "all-graph-privileges", and "ALL GRAPH PRIVILEGES" all normalise to
// "ALL GRAPH PRIVILEGES". The hyphen separator resolves the kebab positional
// (e.g. "set-property" -> "SET PROPERTY") used by the category subcommands. It
// returns a usage error listing the valid keywords if the result is not a known
// action.
func normalizeAction(action string) (string, error) {
	cleaned := strings.NewReplacer("_", " ", "-", " ").Replace(action)
	normalized := strings.ToUpper(strings.Join(strings.Fields(cleaned), " "))
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

	if opts.cidr != "" && category != load {
		return "", nil, clierr.NewUsageError("--cidr is only valid for the LOAD action")
	}

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
		clause = normalized + " " + strings.Join(escapeIdentifiers(opts.nodeLabels), ", ") + " ON GRAPH " + cypherIdentifier(graph)
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
	case load:
		if hasGraph || hasDatabase || opts.onDbms {
			return "", nil, clierr.NewUsageError("action %s does not accept --on-graph, --on-database, or --on-dbms (use --cidr)", normalized)
		}
		if hasNodeLabel || hasRelType {
			return "", nil, clierr.NewUsageError("action %s does not accept node-label or relationship-type qualifiers", normalized)
		}
		if opts.cidr == "" {
			clause = normalized + " ON ALL DATA"
		} else {
			clause = normalized + " ON CIDR " + cypherStringLiteral(opts.cidr)
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
