// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package clicmd contains shared helpers for cobra command trees.
package clicmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// SuggestSubcommand is a cobra Args validator that returns an "unknown
// command" error including cobra's SuggestionsFor output. It is intended to
// be installed on parent commands that have subcommands but no Run logic,
// so that typos at any nesting level surface a "Did you mean ...?" hint
// instead of silently falling through to help.
//
// When len(args) == 0 it returns nil so that running the parent command
// without arguments continues to print help.
//
// The error format mirrors cobra's own legacyArgs path
// (see github.com/spf13/cobra args.go) so the output is indistinguishable
// from the root-level typo behaviour cobra already produces.
func SuggestSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("unknown command %q for %q%s", args[0], cmd.CommandPath(), suggestionsSuffix(cmd, args[0]))
}

// suggestionsSuffix replicates the cobra (unexported) findSuggestions
// helper: when SuggestionsFor returns one or more candidates it formats
// them as a "Did you mean this?" block; otherwise it returns the empty
// string. It also mirrors cobra's lazy default of SuggestionsMinimumDistance
// = 2 when unset (cobra's exported SuggestionsFor does NOT apply that
// default — only the unexported findSuggestions does, so callers that go
// through SuggestionsFor directly otherwise get zero suggestions).
func suggestionsSuffix(cmd *cobra.Command, typedName string) string {
	if cmd.DisableSuggestions {
		return ""
	}
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2
	}
	suggestions := cmd.SuggestionsFor(typedName)
	if len(suggestions) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\nDid you mean this?\n")
	for _, s := range suggestions {
		fmt.Fprintf(&sb, "\t%s\n", s)
	}
	return sb.String()
}

// ApplySuggestionsToParents recursively walks the cobra tree rooted at root
// and installs SuggestSubcommand as the Args validator on every parent
// command that:
//
//  1. has subcommands (HasSubCommands()), and
//  2. has no Run or RunE (so we never override commands that produce
//     output of their own), and
//  3. has no pre-existing Args validator (so we never override an
//     explicit validator like cobra.ExactArgs(1)).
//
// The walk is idempotent and side-effect-free aside from setting Args on
// matched commands.
func ApplySuggestionsToParents(root *cobra.Command) {
	if root == nil {
		return
	}
	if root.HasSubCommands() && root.Run == nil && root.RunE == nil && root.Args == nil {
		root.Args = SuggestSubcommand
	}
	for _, child := range root.Commands() {
		ApplySuggestionsToParents(child)
	}
}
