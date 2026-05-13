// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package agentcontext

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSyntheticTree builds a small, inline cobra tree used by the walker
// tests. It is intentionally hermetic — no app.NewCmd, no clicfg.
//
// Shape:
//
//	root
//	  --rw           (persistent, inherited by all children)
//	  --secret       (persistent + hidden — should be skipped everywhere)
//	  visible        (subcommand, aliases ["v","vis"])
//	    --zeta       (local)
//	    --alpha      (local; sort-order check)
//	    --hidden     (local + hidden — should be skipped)
//	    nested       (sub-subcommand for recursion-depth coverage)
//	      --inner    (local)
//	  legacy         (deprecated subcommand — should NOT appear in commands map;
//	                  used for direct walkCommand Deprecated-field assertion)
//	  buried         (hidden subcommand — should NOT appear)
func newSyntheticTree() *cobra.Command {
	root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) {}}
	root.PersistentFlags().Bool("rw", false, "open in read-write mode")
	root.PersistentFlags().String("secret", "", "hidden persistent flag")
	_ = root.PersistentFlags().MarkHidden("secret")

	visible := &cobra.Command{
		Use:     "visible",
		Short:   "a visible command",
		Long:    "long description for visible",
		Example: "root visible --alpha=1",
		Aliases: []string{"v", "vis"},
		Run:     func(*cobra.Command, []string) {},
	}
	visible.Flags().String("zeta", "z-default", "zeta flag")
	visible.Flags().String("alpha", "a-default", "alpha flag")
	visible.Flags().String("hidden", "", "hidden local flag")
	_ = visible.Flags().MarkHidden("hidden")

	nested := &cobra.Command{Use: "nested [flags]", Short: "nested leaf", Run: func(*cobra.Command, []string) {}}
	nested.Flags().Int("inner", 0, "inner flag")
	visible.AddCommand(nested)

	legacy := &cobra.Command{
		Use:        "legacy",
		Short:      "deprecated command",
		Deprecated: "use new-legacy instead",
		Run:        func(*cobra.Command, []string) {},
	}
	buried := &cobra.Command{Use: "buried", Short: "hidden subcommand", Hidden: true, Run: func(*cobra.Command, []string) {}}

	root.AddCommand(visible, legacy, buried)
	return root
}

// findChild locates a direct child of parent by Use's first token. Fails
// the test if missing.
func findChild(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("subcommand %q not found under %q", name, parent.Name())
	return nil
}

func TestBuildContext_Envelope(t *testing.T) {
	root := newSyntheticTree()
	ctx := BuildContext(root, "v9.9.9-test")

	assert.Equal(t, 1, ctx.SchemaVersion)
	assert.Equal(t, "v9.9.9-test", ctx.CliVersion)
	assert.Equal(t, "neo4j-cli", ctx.Binary)
	assert.Equal(t, "--await", ctx.AsyncFlag)
	assert.Equal(t, clicfg.ValidFormatValues[:], ctx.OutputFormats)
	assert.Len(t, ctx.ExitCodes, 9, "exit_codes must list entries 0-8 (closed set per agent-cli-auditor §4.1)")
	assert.Equal(t, "success", ctx.ExitCodes["0"])
	for _, code := range []string{"1", "2", "3", "4", "5", "6", "7", "8"} {
		assert.NotEmpty(t, ctx.ExitCodes[code], "exit_codes[%q] must have a description", code)
	}
	assert.Len(t, ctx.ErrorCodes, 8, "error_codes must list all eight clierr categories")
	for _, code := range []string{
		"fatal_error",
		"usage_error",
		"not_found",
		"auth_error",
		"conflict",
		"validation_error",
		"rate_limited",
		"upstream_error",
	} {
		assert.Contains(t, ctx.ErrorCodes, code, "error_codes must include %q", code)
	}
}

func TestBuildContext_HidesHiddenAndDeprecatedSubcommands(t *testing.T) {
	ctx := BuildContext(newSyntheticTree(), "dev")

	require.Contains(t, ctx.Commands, "visible")
	assert.NotContains(t, ctx.Commands, "buried", "hidden subcommands must be skipped")
	assert.NotContains(t, ctx.Commands, "legacy",
		"deprecated subcommands are filtered by IsAvailableCommand (cobra contract)")
}

func TestBuildContext_CommandFieldsCaptured(t *testing.T) {
	ctx := BuildContext(newSyntheticTree(), "dev")

	visible, ok := ctx.Commands["visible"]
	require.True(t, ok)
	assert.Equal(t, "visible", visible.Use)
	assert.Equal(t, "a visible command", visible.Short)
	assert.Equal(t, "long description for visible", visible.Long)
	assert.Equal(t, "root visible --alpha=1", visible.Example)
	assert.Equal(t, []string{"v", "vis"}, visible.Aliases)
	assert.False(t, visible.Hidden)
	assert.Empty(t, visible.Deprecated)
}

func TestWalkCommand_DeprecatedFieldSurfaces(t *testing.T) {
	// IsAvailableCommand filters Deprecated commands out of the recursion,
	// so they never appear in BuildContext output. But walkCommand itself
	// preserves the Deprecated string — call it directly for the
	// REQ-V-005 "Deprecated field surfaces" assertion.
	root := newSyntheticTree()
	legacy := findChild(t, root, "legacy")
	got := walkCommand(legacy, 0)
	assert.Equal(t, "use new-legacy instead", got.Deprecated)
}

func TestBuildContext_RecursionDescendsAvailableChildren(t *testing.T) {
	ctx := BuildContext(newSyntheticTree(), "dev")

	visible := ctx.Commands["visible"]
	require.Contains(t, visible.Subcommands, "nested",
		"`nested` should appear under visible — Use=%q tokenises to first whitespace token", "nested [flags]")

	nested := visible.Subcommands["nested"]
	require.Len(t, nested.Flags, 2)
	// rw inherited from root + local inner; sorted alphabetically.
	assert.Equal(t, "inner", nested.Flags[0].Name)
	assert.False(t, nested.Flags[0].Inherited)
	assert.Equal(t, "rw", nested.Flags[1].Name)
	assert.True(t, nested.Flags[1].Inherited)
}

func TestCollectFlags_SortAndHiddenAndInherited(t *testing.T) {
	root := newSyntheticTree()
	visible := findChild(t, root, "visible")

	cases := []struct {
		name         string
		flagName     string
		wantPresent  bool
		wantInherit  bool
		wantDefault  string
		wantTypeName string
	}{
		{"local alpha", "alpha", true, false, "a-default", "string"},
		{"local zeta", "zeta", true, false, "z-default", "string"},
		{"local hidden flag is dropped", "hidden", false, false, "", ""},
		{"persistent parent flag is inherited", "rw", true, true, "false", "bool"},
		{"hidden persistent parent flag is dropped", "secret", false, false, "", ""},
	}

	flags := collectFlags(visible)
	byName := map[string]Flag{}
	for _, f := range flags {
		byName[f.Name] = f
	}

	// Sort assertion: alpha < rw < zeta (rw comes from inherited; sort is over the merged list).
	names := make([]string, 0, len(flags))
	for _, f := range flags {
		names = append(names, f.Name)
	}
	assert.Equal(t, []string{"alpha", "rw", "zeta"}, names,
		"flags must be sorted alphabetically over the merged local+inherited set")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, present := byName[tc.flagName]
			if !tc.wantPresent {
				assert.False(t, present, "flag %q must be skipped", tc.flagName)
				return
			}
			require.True(t, present, "flag %q expected on visible", tc.flagName)
			assert.Equal(t, tc.wantInherit, got.Inherited)
			assert.Equal(t, tc.wantDefault, got.Default)
			assert.Equal(t, tc.wantTypeName, got.Type)
			assert.NotEmpty(t, got.Description)
		})
	}
}

func TestBuildContext_AliasesNonNilForCommandsWithoutAliases(t *testing.T) {
	ctx := BuildContext(newSyntheticTree(), "dev")

	nested := ctx.Commands["visible"].Subcommands["nested"]
	require.NotNil(t, nested.Aliases, "Aliases must be non-nil so JSON emits [] not null")
	assert.Empty(t, nested.Aliases)
}

func TestFirstToken(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"list", "list"},
		{"List", "list"},
		{"list [flags]", "list"},
		{"  spaced [arg]", "spaced"},
		{"tab\tafter", "tab"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, firstToken(tc.in))
		})
	}
}
