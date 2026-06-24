// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package privilege

import (
	"sort"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// verbParent builds a grant/deny/revoke parent for structural assertions.
func verbParent(t *testing.T, word string) *cobra.Command {
	t.Helper()
	cfg := clicfg.NewConfig(afero.NewMemMapFs(), "test", clicfg.GlobalScope)
	conn := testConn()
	switch word {
	case "grant":
		return newGrantCmd(cfg, &conn)
	case "deny":
		return newDenyCmd(cfg, &conn)
	case "revoke":
		return newRevokeCmd(cfg, &conn)
	default:
		t.Fatalf("unknown verb %q", word)
		return nil
	}
}

// flagNames returns the sorted long names of every flag registered directly on
// cmd (local flags only; inherited persistent flags are not included).
func flagNames(cmd *cobra.Command) []string {
	var got []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) { got = append(got, f.Name) })
	sort.Strings(got)
	return got
}

// wantFlagSet returns the expected sorted flag set for a category command under
// the given verb: the category's permitted flags + --role, + --revoke-type for
// revoke. This mirrors REQ-F-031's table.
func wantFlagSet(word string, cat actionCategory) []string {
	got := append([]string{"role"}, categoryMeta[cat].flags...)
	if word == "revoke" {
		got = append(got, "revoke-type")
	}
	sort.Strings(got)
	return got
}

// TestVerbParents_RegisterExactlySevenCategories asserts each of grant/deny/
// revoke registers exactly the seven category subcommands named per categoryMeta
// (REQ-NF-006).
func TestVerbParents_RegisterExactlySevenCategories(t *testing.T) {
	for _, word := range []string{"grant", "deny", "revoke"} {
		t.Run(word, func(t *testing.T) {
			parent := verbParent(t, word)

			var names []string
			for _, sub := range parent.Commands() {
				names = append(names, strings.Fields(sub.Use)[0])
			}
			sort.Strings(names)

			var want []string
			for _, cat := range categoryOrder {
				want = append(want, categoryMeta[cat].name)
			}
			sort.Strings(want)

			require.Len(t, parent.Commands(), 7, "verb %q must register exactly 7 categories", word)
			assert.Equal(t, want, names)
		})
	}
}

// TestCategoryCommands_FlagSetMatchesCategory asserts every category command
// exposes exactly its category's permitted flags + --role (+ --revoke-type for
// revoke) and no others (REQ-F-031 / REQ-NF-006).
func TestCategoryCommands_FlagSetMatchesCategory(t *testing.T) {
	for _, word := range []string{"grant", "deny", "revoke"} {
		parent := verbParent(t, word)
		byName := map[string]*cobra.Command{}
		for _, sub := range parent.Commands() {
			byName[strings.Fields(sub.Use)[0]] = sub
		}
		for _, cat := range categoryOrder {
			cat := cat
			name := categoryMeta[cat].name
			t.Run(word+"/"+name, func(t *testing.T) {
				sub := byName[name]
				require.NotNil(t, sub)
				assert.Equal(t, wantFlagSet(word, cat), flagNames(sub))
			})
		}
	}
}

// TestCategoryCommands_ValidArgsAreKebabActions asserts each category command's
// ValidArgs equals its kebab action keywords and the union across categories
// equals validActions — every action reachable through exactly one category
// (REQ-NF-006).
func TestCategoryCommands_ValidArgsAreKebabActions(t *testing.T) {
	parent := verbParent(t, "grant")
	byName := map[string]*cobra.Command{}
	for _, sub := range parent.Commands() {
		byName[strings.Fields(sub.Use)[0]] = sub
	}

	seen := map[string]actionCategory{}
	for _, cat := range categoryOrder {
		sub := byName[categoryMeta[cat].name]
		require.NotNil(t, sub)

		// Single-action categories (load) take no positional: Use carries no
		// <action>, Args is NoArgs, and ValidArgs is nil. Multi-action
		// categories expose one positional with ValidArgs == kebab actions.
		if len(actionsForCategory(cat)) == 1 {
			assert.Nil(t, sub.ValidArgs, "single-action %s must have nil ValidArgs", categoryMeta[cat].name)
			assert.Equal(t, categoryMeta[cat].name, sub.Use, "single-action %s Use omits <action>", categoryMeta[cat].name)
		} else {
			assert.Equal(t, kebabActionsForCategory(cat), sub.ValidArgs, "ValidArgs for %s", categoryMeta[cat].name)
		}

		for _, canonical := range actionsForCategory(cat) {
			if prev, dup := seen[canonical]; dup {
				t.Fatalf("action %q reachable through both %s and %s", canonical, categoryMeta[prev].name, categoryMeta[cat].name)
			}
			seen[canonical] = cat
		}
	}

	require.Len(t, seen, len(validActions), "union of ValidArgs must equal validActions")
	for action := range validActions {
		if _, ok := seen[action]; !ok {
			t.Fatalf("action %q in validActions not reachable through any category", action)
		}
	}
}

// TestRevokeCategories_CarryRevokeType asserts revoke's category commands carry
// --revoke-type while grant/deny's do not (REQ-NF-006).
func TestRevokeCategories_CarryRevokeType(t *testing.T) {
	revoke := verbParent(t, "revoke")
	for _, sub := range revoke.Commands() {
		assert.NotNil(t, sub.Flags().Lookup("revoke-type"), "%s must carry --revoke-type", sub.Use)
	}
	for _, word := range []string{"grant", "deny"} {
		parent := verbParent(t, word)
		for _, sub := range parent.Commands() {
			assert.Nil(t, sub.Flags().Lookup("revoke-type"), "%s %s must not carry --revoke-type", word, sub.Use)
		}
	}
}

// TestCategoryCommands_ShortIncludesActionSummary asserts each category leaf's
// Short surfaces its valid actions: small categories list all their kebab
// actions in parentheses; database and dbms show the three-action preview plus
// the total count and "see --help" (REQ-NF-009).
func TestCategoryCommands_ShortIncludesActionSummary(t *testing.T) {
	grant := verbParent(t, "grant")
	byName := map[string]*cobra.Command{}
	for _, sub := range grant.Commands() {
		byName[strings.Fields(sub.Use)[0]] = sub
	}

	fullList := map[string]string{
		"property": "(match, merge, read, set-property)",
		"entity":   "(create, delete, traverse)",
		"graph":    "(all-graph-privileges, write)",
		"label":    "(remove-label, set-label)",
	}
	for name, want := range fullList {
		t.Run(name, func(t *testing.T) {
			sub := byName[name]
			require.NotNil(t, sub)
			assert.Contains(t, sub.Short, want)
		})
	}

	// load is single-action: its Short carries no "(load)" action-summary
	// suffix, just "Grant a LOAD privilege to a role".
	t.Run("load", func(t *testing.T) {
		sub := byName["load"]
		require.NotNil(t, sub)
		assert.NotContains(t, sub.Short, "(load)")
		assert.Equal(t, "Grant a LOAD privilege to a role", sub.Short)
	})

	for _, name := range []string{"database", "dbms"} {
		t.Run(name, func(t *testing.T) {
			sub := byName[name]
			require.NotNil(t, sub)
			assert.Contains(t, sub.Short, "… — 19 actions; see --help)")
		})
	}
}

// TestActionSummary asserts the threshold (<= 6 -> full list) and the truncated
// preview form directly against the helper.
func TestActionSummary(t *testing.T) {
	assert.Equal(t, "(match, merge, read, set-property)", actionSummary(propertyBearer))
	assert.Equal(t, "(create, delete, traverse)", actionSummary(graphOnly))
	assert.Equal(t, "(all-graph-privileges, write)", actionSummary(graphWhole))
	assert.Equal(t, "(remove-label, set-label)", actionSummary(labelScoped))
	// load is single-action: no parenthesised summary (the action is implied).
	assert.Equal(t, "", actionSummary(load))

	assert.Equal(t, "(access, all-database-privileges, constraint-management, … — 19 actions; see --help)", actionSummary(database))
	assert.Equal(t, "(all-dbms-privileges, alter-user, assign-role, … — 19 actions; see --help)", actionSummary(dbms))
}

// TestCategoryCommands_HaveFlushLeftExample asserts every category leaf carries a
// non-empty flush-left Example (REQ-NF-006; mirrors TestAllLeafCommands_HaveExamples
// scoped to this tree).
func TestCategoryCommands_HaveFlushLeftExample(t *testing.T) {
	for _, word := range []string{"grant", "deny", "revoke"} {
		parent := verbParent(t, word)
		for _, sub := range parent.Commands() {
			t.Run(word+"/"+strings.Fields(sub.Use)[0], func(t *testing.T) {
				require.NotEmpty(t, sub.Example, "leaf must have a non-empty Example")
				assert.False(t, strings.HasPrefix(sub.Example, " ") || strings.HasPrefix(sub.Example, "\t"),
					"Example must be flush-left")
				assert.GreaterOrEqual(t, strings.Count(sub.Example, "neo4j-cli"), 2,
					"Example must have >= 2 invocations")
			})
		}
	}
}
