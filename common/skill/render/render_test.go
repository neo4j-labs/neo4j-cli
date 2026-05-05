// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package render

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// updateGolden controls whether failing tests rewrite their expected
// output. Pass `-update` to `go test` to regenerate the testdata files.
var updateGolden = flag.Bool("update", false, "regenerate render testdata golden files")

// fixtureRoot builds a deterministic cobra tree exercising every render
// path: a simple leaf with flags + examples, a nested parent with two
// children (one of them hidden), and a help cmd that must be excluded.
func fixtureRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "fixture-cli",
		Short: "Fixture CLI used by render tests",
		Long: `Fixture CLI for golden-file render tests.

This tree is intentionally small but covers global flags, sorted
subcommands, hidden subcommands, nested subcommands, and flag tables.`,
	}
	root.PersistentFlags().String("output", "table", "Output format (table|json)")
	root.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose logging")

	// Subcommand ordered last alphabetically to verify sort.
	zebra := &cobra.Command{
		Use:     "zebra",
		Short:   "Last alphabetical sub — verifies sort order",
		Example: `  fixture-cli zebra`,
	}

	// Parent with two children, one hidden.
	instance := &cobra.Command{
		Use:   "instance",
		Short: "Manage instances",
	}
	instance.AddCommand(&cobra.Command{
		Use:     "list",
		Short:   "List instances",
		Example: `  fixture-cli instance list`,
	})
	instance.AddCommand(&cobra.Command{
		Use:    "secret",
		Short:  "Hidden cmd — must be excluded",
		Hidden: true,
	})
	create := &cobra.Command{
		Use:     "create",
		Short:   "Create an instance",
		Long:    "Create a new fixture instance with defaults.",
		Example: `  fixture-cli instance create --name foo`,
	}
	create.Flags().String("name", "", "Instance name")
	create.Flags().Int("size", 0, "Size in GB")
	instance.AddCommand(create)

	// Hidden top-level — must be excluded from SKILL.md index AND not
	// produce a references file.
	hidden := &cobra.Command{
		Use:    "hidden-top",
		Short:  "Hidden — must NOT appear in references/",
		Hidden: true,
	}

	root.AddCommand(zebra)
	root.AddCommand(instance)
	root.AddCommand(hidden)

	return root
}

func TestBundle_GoldenFiles(t *testing.T) {
	root := fixtureRoot()

	bundle, err := Bundle(root, Options{
		Name:        "fixture-cli",
		Description: "Fixture CLI for golden-file tests. Trigger when render code changes.",
		Additions:   "- One gotcha line.\n- Another gotcha line.\n",
	})
	require.NoError(t, err)

	wantKeys := []string{
		"SKILL.md",
		"references/instance.md",
		"references/zebra.md",
	}
	for _, k := range wantKeys {
		assert.Contains(t, bundle, k, "missing key %q", k)
	}
	assert.Len(t, bundle, len(wantKeys), "unexpected extra keys: %v", keysOf(bundle))

	// Hidden top-level must not produce a reference file.
	assert.NotContains(t, bundle, "references/hidden-top.md")

	for _, k := range wantKeys {
		assertGolden(t, k, bundle[k])
	}
}

func TestBundle_Deterministic(t *testing.T) {
	root := fixtureRoot()
	opts := Options{
		Name:        "fixture-cli",
		Description: "deterministic check",
		Additions:   "- gotcha\n",
	}
	first, err := Bundle(root, opts)
	require.NoError(t, err)
	second, err := Bundle(fixtureRoot(), opts)
	require.NoError(t, err)

	assert.Equal(t, len(first), len(second))
	for k, v := range first {
		assert.Equalf(t, v, second[k], "key %s differs across runs", k)
	}
}

func TestBundle_VersionPlaceholder(t *testing.T) {
	root := fixtureRoot()
	bundle, err := Bundle(root, Options{
		Name:        "x",
		Description: "y",
	})
	require.NoError(t, err)
	skill := string(bundle["SKILL.md"])
	assert.Contains(t, skill, "version: {{VERSION}}\n")
	assert.Contains(t, skill, "name: x\n")
	assert.Contains(t, skill, "description: y\n")
}

func TestBundle_ExcludesHiddenAndHelp(t *testing.T) {
	root := &cobra.Command{Use: "r", Short: "r"}
	root.AddCommand(&cobra.Command{Use: "visible", Short: "shown"})
	root.AddCommand(&cobra.Command{Use: "hidden", Short: "x", Hidden: true})
	// Cobra auto-generates a `help` cmd lazily; add explicitly to make
	// the rule unambiguous.
	root.AddCommand(&cobra.Command{Use: "help", Short: "implicit help"})

	bundle, err := Bundle(root, Options{Name: "r", Description: "r"})
	require.NoError(t, err)
	assert.Contains(t, bundle, "references/visible.md")
	assert.NotContains(t, bundle, "references/hidden.md")
	assert.NotContains(t, bundle, "references/help.md")

	skill := string(bundle["SKILL.md"])
	assert.Contains(t, skill, "[`visible`]")
	assert.NotContains(t, skill, "[`hidden`]")
	assert.NotContains(t, skill, "[`help`]")
}

func TestBundle_TOCWhenLong(t *testing.T) {
	// Build a sub whose rendered body exceeds the TOC threshold by
	// nesting many leaves under it.
	root := &cobra.Command{Use: "r", Short: "r"}
	parent := &cobra.Command{Use: "big", Short: "Many subs"}
	for i := 0; i < 30; i++ {
		c := &cobra.Command{
			Use:     fmt.Sprintf("leaf%02d", i),
			Short:   fmt.Sprintf("Leaf %d short", i),
			Long:    fmt.Sprintf("Leaf %d long description.", i),
			Example: fmt.Sprintf("  r big leaf%02d --flag", i),
		}
		c.Flags().String("flag", "", "test flag")
		parent.AddCommand(c)
	}
	root.AddCommand(parent)

	bundle, err := Bundle(root, Options{Name: "r", Description: "r"})
	require.NoError(t, err)

	ref := string(bundle["references/big.md"])
	// TOC sentinel.
	assert.Contains(t, ref, "## Contents\n", "expected TOC for >100-line reference")
	// First TOC entry should anchor-link the first leaf heading.
	assert.Contains(t, ref, "(#r-big-leaf00)")
}

func TestBundle_NoTOCWhenShort(t *testing.T) {
	root := &cobra.Command{Use: "r", Short: "r"}
	root.AddCommand(&cobra.Command{
		Use:   "small",
		Short: "tiny",
	})
	bundle, err := Bundle(root, Options{Name: "r", Description: "r"})
	require.NoError(t, err)
	ref := string(bundle["references/small.md"])
	assert.NotContains(t, ref, "## Contents\n")
}

func TestBundle_FlagTableSorted(t *testing.T) {
	root := &cobra.Command{Use: "r", Short: "r"}
	leaf := &cobra.Command{Use: "leaf", Short: "leaf"}
	leaf.Flags().String("zeta", "", "z flag")
	leaf.Flags().String("alpha", "", "a flag")
	leaf.Flags().String("mid", "", "m flag")
	root.AddCommand(leaf)

	bundle, err := Bundle(root, Options{Name: "r", Description: "r"})
	require.NoError(t, err)
	ref := string(bundle["references/leaf.md"])
	a := strings.Index(ref, "--alpha")
	m := strings.Index(ref, "--mid")
	z := strings.Index(ref, "--zeta")
	require.True(t, a > 0 && m > a && z > m, "flags out of order: alpha=%d mid=%d zeta=%d", a, m, z)
}

func TestBundle_HiddenFlagsExcluded(t *testing.T) {
	root := &cobra.Command{Use: "r", Short: "r"}
	leaf := &cobra.Command{Use: "leaf", Short: "leaf"}
	leaf.Flags().String("public", "", "shown")
	leaf.Flags().String("secret", "", "hidden")
	require.NoError(t, leaf.Flags().MarkHidden("secret"))
	root.AddCommand(leaf)

	bundle, err := Bundle(root, Options{Name: "r", Description: "r"})
	require.NoError(t, err)
	ref := string(bundle["references/leaf.md"])
	assert.Contains(t, ref, "--public")
	assert.NotContains(t, ref, "--secret")
}

func TestBundle_SubWithNoFlags(t *testing.T) {
	root := &cobra.Command{Use: "r", Short: "r"}
	root.AddCommand(&cobra.Command{
		Use:   "noflags",
		Short: "no flags here",
	})
	bundle, err := Bundle(root, Options{Name: "r", Description: "r"})
	require.NoError(t, err)
	ref := string(bundle["references/noflags.md"])
	assert.NotContains(t, ref, "Flags:")
	assert.NotContains(t, ref, "| Flag | Type")
}

func TestBundle_Errors(t *testing.T) {
	root := &cobra.Command{Use: "r", Short: "r"}
	_, err := Bundle(nil, Options{Name: "r", Description: "r"})
	assert.Error(t, err)
	_, err = Bundle(root, Options{Description: "r"})
	assert.Error(t, err)
	_, err = Bundle(root, Options{Name: "r"})
	assert.Error(t, err)
}

// keysOf returns sorted map keys for stable assertion output.
func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// assertGolden compares `got` against testdata/<name>. Pass `-update` to
// regenerate. Path separators inside `name` are translated to filesystem
// separators.
func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", filepath.FromSlash(name))
	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, got, 0644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoErrorf(t, err, "read golden %s (run `go test ./common/skill/render -update` to regenerate)", path)
	if !assert.Equalf(t, string(want), string(got), "golden mismatch for %s", name) {
		t.Logf("hint: regenerate with `go test ./common/skill/render -update`")
	}
}
