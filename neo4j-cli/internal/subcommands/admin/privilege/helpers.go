// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"fmt"
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
// onto cmd for category cat, binding it to the matching field of opts. It is the
// per-flag primitive the category-subcommand factory composes from
// categoryMeta.flags so each category gets only the flags valid for it. The
// --node-label help is category-specific: on the label category the flag is the
// required SET/REMOVE LABEL target, not the optional entity restriction it is on
// property/entity.
func registerPrivilegeFlag(cmd *cobra.Command, opts *privilegeOpts, cat actionCategory, flag string) {
	switch flag {
	case flagOnGraph:
		cmd.Flags().StringVar(&opts.onGraph, flagOnGraph, "", "Scope the privilege to a graph (use * for all)")
	case flagOnDatabase:
		cmd.Flags().StringVar(&opts.onDatabase, flagOnDatabase, "", "Scope the privilege to a database (use * for all)")
	case flagNodeLabel:
		help := "Restrict a graph privilege to node labels"
		if cat == labelScoped {
			help = "Node label(s) the SET LABEL / REMOVE LABEL privilege applies to (required; repeatable)"
		}
		cmd.Flags().StringArrayVar(&opts.nodeLabels, flagNodeLabel, nil, help)
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

	// A single-action category (today only load) takes NO positional: its sole
	// action is implied, so repeating it ("grant load load") carries no
	// information. The property is modelled once (categoryActions.singleAction),
	// not hardcoded to load — a future single-action category inherits it.
	actions := actionsForCategory(cat)
	singleAction := categoryActionsByCat[cat].singleAction

	use := meta.name + " <action>"
	args := cobra.ExactArgs(1)
	validArgs := kebabActionsForCategory(cat)
	if singleAction {
		use = meta.name
		args = cobra.NoArgs
		validArgs = nil
	}

	cmd := &cobra.Command{
		Use:       use,
		Short:     categoryShort(word, cat),
		Long:      categoryLong(word, cat),
		Example:   renderCategoryExample(word, cat),
		ValidArgs: validArgs,
		// For multi-action categories ValidArgs drives <TAB> completion only; Args
		// is ExactArgs(1) (not OnlyValidArgs) so a cross-category action reaches
		// RunE and yields the "use 'admin privilege grant database access'" hint
		// (REQ-F-030) rather than cobra's generic invalid-argument message.
		Args: args,
		RunE: func(cmd *cobra.Command, cmdArgs []string) error {
			var canonical string
			if singleAction {
				canonical = actions[0]
			} else {
				var err error
				canonical, err = resolveCategoryAction(cat, cmdArgs[0], word)
				if err != nil {
					return err
				}
			}
			resolvedVerb := verb
			if word == "revoke" {
				var err error
				resolvedVerb, err = revokeVerb(revokeType)
				if err != nil {
					return err
				}
			}
			return runPrivilegeMutation(cmd, cfg, *conn, resolvedVerb, cat, canonical, roleName, opts, target)
		},
	}
	cmd.Annotations = map[string]string{"write": "true"}

	cmd.Flags().StringVar(&roleName, "role", "", "Name of the role to "+word+" the privilege "+rolePreposition(word))
	if word == "revoke" {
		cmd.Flags().StringVar(&revokeType, "revoke-type", "", "Restrict the revoke to grant or deny privileges (grant|deny); omit to revoke both")
	}
	for _, flag := range meta.flags {
		registerPrivilegeFlag(cmd, &opts, cat, flag)
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
	actionText := "The action is the positional argument; valid actions are: " + strings.Join(kebabActionsForCategory(cat), ", ") + ". "
	// A single-action category takes no positional: the sole action is implied
	// by the command name, so the Long says so rather than describing an argument.
	if categoryActionsByCat[cat].singleAction {
		actionText = "Takes no action argument. "
	}
	return verbTitle(verbWord) + " a " + meta.shortNoun + " " + rolePreposition(verbWord) + " a role. " +
		actionText + meta.longRule + " --role is required."
}

// runPrivilegeMutation runs the shared write sequence: the --role check,
// buildPrivilegeCypher, SilenceUsage, exec via the seam (appending the role
// target with the given keyword, "TO" or "FROM"), then outputPrivileges. verb
// is the resolved privilege verb ("GRANT", "DENY", "REVOKE", ...); cat and
// action are the resolved category and canonical keyword from
// resolveCategoryAction.
func runPrivilegeMutation(cmd *cobra.Command, cfg *clicfg.Config, conn *dbconn.Conn, verb string, cat actionCategory, action, roleName string, opts privilegeOpts, target string) error {
	if roleName == "" {
		return clierr.NewUsageError("--role is required")
	}

	cypher, params, err := buildPrivilegeCypher(verb, cat, action, opts)
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
// revoke. onGraph and onDatabase are considered "set" when non-empty.
type privilegeOpts struct {
	onGraph    string
	onDatabase string
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
	flagNodeLabel  = "node-label"
	flagRelType    = "relationship-type"
	flagProperty   = "property"
	flagCidr       = "cidr"
)

// categoryInfo declares the user-facing surface for one action category: the
// kebab command name, the flags valid for every action in the category, a Long
// help fragment describing the category's rules, and a representative action +
// flag values used to render the Example. It is the single source (alongside
// validActions) for the per-category subcommand surface introduced by the
// discoverability redesign (REQ-F-034); newCategoryCmd is its only consumer.
//
// Per-flag requirements (e.g. label needs --node-label) are not declared here:
// buildPrivilegeCypher already enforces them as usage errors, so a separate
// requiredFlags field would be a second, unenforced source of the same rule.
// The longRule text states the requirement for help.
type categoryInfo struct {
	name          string
	flags         []string
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
		flags:         nil,
		shortNoun:     "DBMS privilege",
		longRule:      "Applies to the whole DBMS; takes no scope flag.",
		exampleAction: "create-role",
		exampleFlags:  "",
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

// categoryActions holds the per-category action data derived once from
// validActions: the canonical keywords (sorted), their kebab forms (sorted),
// and whether the category has exactly one action (so its action is implied by
// the command name and takes no positional). It is computed at package init —
// rather than re-scanning validActions on every help/exec call — and is the
// single home of the "single-action" property the help and command-build paths
// read.
type categoryActions struct {
	canonical    []string
	kebab        []string
	singleAction bool
}

// categoryActionsByCat is populated once in init() from validActions, keeping
// validActions the single source of action->category.
var categoryActionsByCat map[actionCategory]categoryActions

func init() {
	byCat := make(map[actionCategory][]string, len(categoryMeta))
	for action, cat := range validActions {
		byCat[cat] = append(byCat[cat], action)
	}
	categoryActionsByCat = make(map[actionCategory]categoryActions, len(byCat))
	for cat, canonical := range byCat {
		sort.Strings(canonical)
		kebab := make([]string, len(canonical))
		for i, a := range canonical {
			kebab[i] = kebabAction(a)
		}
		categoryActionsByCat[cat] = categoryActions{
			canonical:    canonical,
			kebab:        kebab,
			singleAction: len(canonical) == 1,
		}
	}
}

// actionsForCategory returns the canonical action keywords belonging to cat,
// sorted, derived once from validActions so categoryMeta need not duplicate the
// action lists already in validActions.
func actionsForCategory(cat actionCategory) []string {
	return categoryActionsByCat[cat].canonical
}

// kebabAction converts a canonical action keyword to its kebab form used for
// ValidArgs and the positional argument, e.g. "SET LABEL" -> "set-label" and
// "ALL GRAPH PRIVILEGES" -> "all-graph-privileges".
func kebabAction(canonical string) string {
	return strings.ToLower(strings.ReplaceAll(canonical, " ", "-"))
}

// categoryInvocation returns the command tail used to invoke a category, e.g.
// "database access" for a multi-action category or "load" for a single-action
// category (whose action is implied and takes no positional). Used by the
// cross-category hint so pointing at a single-action category omits the
// redundant action token.
func categoryInvocation(cat actionCategory, canonical string) string {
	name := categoryMeta[cat].name
	if categoryActionsByCat[cat].singleAction {
		return name
	}
	return name + " " + kebabAction(canonical)
}

// kebabActionsForCategory returns the kebab action keywords for cat, used for a
// category subcommand's ValidArgs.
func kebabActionsForCategory(cat actionCategory) []string {
	return categoryActionsByCat[cat].kebab
}

// categoryShort builds a category leaf's Short. The parenthesised action
// summary is omitted for a single-action category (it would be a redundant
// "(load)" suffix), so the load Short reads "Grant a LOAD privilege to a role".
func categoryShort(word string, cat actionCategory) string {
	meta := categoryMeta[cat]
	summary := actionSummary(cat)
	if summary != "" {
		summary += " "
	}
	return verbTitle(word) + " a " + meta.shortNoun + " " + summary + rolePreposition(word) + " a role"
}

// actionSummary renders the parenthesised action list spliced into a category
// leaf's Short so the valid actions are visible in the parent verb's subcommand
// listing. A single-action category returns "" (the sole action is implied by
// the command name — no redundant "(load)" suffix). Small categories (<= 6
// actions) list all their kebab actions; larger ones (database, dbms) show the
// first three plus the total count and point at --help, where categoryLong lists
// the full set. The threshold and preview count are defined only here.
func actionSummary(cat actionCategory) string {
	actions := kebabActionsForCategory(cat)
	if categoryActionsByCat[cat].singleAction {
		return ""
	}
	if len(actions) <= 6 {
		return "(" + strings.Join(actions, ", ") + ")"
	}
	preview := strings.Join(actions[:3], ", ")
	return fmt.Sprintf("(%s, … — %d actions; see --help)", preview, len(actions))
}

// renderCategoryExample builds a flush-left Example block for the given verb word
// ("grant"/"deny"/"revoke") and category, with two invocations: one minimal and
// one writing JSON. Each invocation has a # comment, the neo4j-cli prefix, and
// --rw (the category commands are all write commands). verbWord must be
// non-empty ("grant"/"deny"/"revoke").
func renderCategoryExample(verbWord string, cat actionCategory) string {
	meta := categoryMeta[cat]
	base := "neo4j-cli admin privilege " + verbWord + " " + meta.name
	// A single-action category takes no positional, so the example omits the
	// action token ("grant load --cidr ...", not "grant load load ...").
	if !categoryActionsByCat[cat].singleAction {
		base += " " + meta.exampleAction
	}
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

// resolveCategoryAction resolves a category subcommand's positional action to its
// canonical keyword, scoped to cat. The error messages are CLI-form (kebab) and
// category-scoped (REQ-F-039): when the positional resolves to an action in a
// DIFFERENT category it returns the cross-category hint naming the right command
// (REQ-F-030); when it is unknown (or otherwise not in cat) it returns an
// "unknown <category> action" error listing only this category's kebab actions —
// never the global Cypher-form enumeration. verbWord is the lower-case command
// word ("grant"/"deny"/"revoke") used in the cross-category hint.
func resolveCategoryAction(cat actionCategory, positional, verbWord string) (string, error) {
	canonical := canonicalAction(positional)
	got, ok := validActions[canonical]
	switch {
	case ok && got == cat:
		return canonical, nil
	case ok:
		return "", clierr.NewUsageError(
			"%s is a %s; use 'admin privilege %s %s'",
			positional, categoryMeta[got].shortNoun, verbWord, categoryInvocation(got, canonical),
		)
	default:
		return "", clierr.NewUsageError(
			"unknown %s action %q; valid actions are: %s",
			categoryMeta[cat].name, positional, strings.Join(kebabActionsForCategory(cat), ", "),
		)
	}
}

// canonicalAction normalises action input (case-insensitive; underscores,
// hyphens, and whitespace runs collapse to single spaces) into the canonical
// keyword form used as validActions keys. The result is not guaranteed to be a
// known action; callers look it up in validActions.
func canonicalAction(action string) string {
	cleaned := strings.NewReplacer("_", " ", "-", " ").Replace(action)
	return strings.ToUpper(strings.Join(strings.Fields(cleaned), " "))
}
