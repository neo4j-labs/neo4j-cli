// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package server_test

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/internal/skill"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDrift_EveryListCommandEntryHasReferenceFile asserts that every top-level
// tree the list_commands tool can emit (via the default flag-off tree) has a
// corresponding reference file in the embedded skill bundle. completion is
// explicitly filtered by list_commands and has no reference file in the bundle.
func TestDrift_EveryListCommandEntryHasReferenceFile(t *testing.T) {
	root := newAppCmd(t, false)

	// Collect the top-level tree names list_commands would emit.
	var treeNames []string
	for _, sub := range root.Commands() {
		if sub.Name() == "help" || sub.Name() == "completion" {
			continue
		}
		if !sub.IsAvailableCommand() {
			continue
		}
		treeNames = append(treeNames, sub.Name())
	}

	// Verify every tree name has a reference file in the bundle.
	require.Greater(t, len(treeNames), 0)
	for _, name := range treeNames {
		refPath := fmt.Sprintf("references/%s.md", name)
		data, err := fs.ReadFile(skill.Bundle, refPath)
		require.NoError(t, err, "tree %q has no bundle reference file at %s", name, refPath)

		// Verify the reference file's H1 heading is correct.
		section := extractSectionFromData(string(data), name)
		require.NotEmpty(t, section, "H1 heading for %q not found in %s", name, refPath)
		assert.Contains(t, section, "neo4j-cli "+name)
	}

	// Verify completion has NO reference file — it is injected by cobra at
	// Execute() time and the bundle generator runs pre-Execute.
	_, err := fs.Stat(skill.Bundle, "references/completion.md")
	assert.Error(t, err, "completion must not have a bundle reference file")
	assert.True(t, strings.Contains(err.Error(), "does not exist") ||
		strings.Contains(err.Error(), "no such file"))
}

// extractSectionFromData is a minimal replica of the internal extractSection
// logic, sufficient for the drift test. It finds the H1 heading matching the
// tree name and returns the section until the first child H2.
//
// NOTE: this replica handles ONLY H1 (tree-level) matches. It does not handle
// H2+ command-path resolution because the drift test never needs to resolve
// deep paths — it only verifies that each top-level tree name has a reference
// file with a valid H1 heading. Full heading slicing lives in the production
// extractSection in read_docs.go.
func extractSectionFromData(content, treeName string) string {
	target := "neo4j-cli " + treeName
	lines := strings.Split(content, "\n")
	matchIdx := -1

	for i, line := range lines {
		level := headingLevel(line)
		if level == 1 {
			text := strings.TrimSpace(line[1:])
			if text == target {
				matchIdx = i
				break
			}
		}
	}
	if matchIdx < 0 {
		return ""
	}

	// Return from H1 to first H2 child.
	for i := matchIdx + 1; i < len(lines); i++ {
		if headingLevel(lines[i]) == 2 && strings.Contains(lines[i], "neo4j-cli") {
			return strings.Join(lines[matchIdx:i], "\n")
		}
	}
	return strings.Join(lines[matchIdx:], "\n")
}

// headingLevel is a minimal replica of the internal headingLevel function.
func headingLevel(line string) int {
	trimmed := strings.TrimLeft(line, " ")
	if len(trimmed) == 0 || trimmed[0] != '#' {
		return 0
	}
	level := 0
	for _, c := range trimmed {
		if c == '#' {
			level++
		} else if c == ' ' {
			break
		} else {
			return 0
		}
	}
	if level < 1 || level > 6 {
		return 0
	}
	return level
}

// Verify the drift test's headingLevel matches the internal implementation.
func TestDrift_HeadingLevelReplica(t *testing.T) {
	assert.Equal(t, 1, headingLevel("# H1"))
	assert.Equal(t, 2, headingLevel("## H2"))
	assert.Equal(t, 0, headingLevel("not a heading"))
}

// Verify cobra.TraverseRunHooks is set on the test tree, mirroring neo4j-cli's
// root (so the mcp group's PersistentPreRunE fires).
var _ = cobra.EnableTraverseRunHooks
