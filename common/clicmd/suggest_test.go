// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clicmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSuggestSubcommand_NoArgsReturnsNil(t *testing.T) {
	parent := &cobra.Command{Use: "parent"}
	parent.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})

	if err := SuggestSubcommand(parent, nil); err != nil {
		t.Fatalf("expected nil error for empty args, got %v", err)
	}
	if err := SuggestSubcommand(parent, []string{}); err != nil {
		t.Fatalf("expected nil error for empty args slice, got %v", err)
	}
}

func TestSuggestSubcommand_UnknownReturnsWrappedError(t *testing.T) {
	parent := &cobra.Command{Use: "parent"}
	parent.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})

	err := SuggestSubcommand(parent, []string{"lsit"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, `unknown command "lsit"`) {
		t.Errorf("expected error to mention unknown command, got: %q", msg)
	}
	if !strings.Contains(msg, `"parent"`) {
		t.Errorf("expected error to include parent CommandPath, got: %q", msg)
	}
	// The cobra SuggestionsFor suffix begins with a newline + "\nDid you mean this?".
	if !strings.Contains(msg, "Did you mean this?") {
		t.Errorf("expected cobra SuggestionsFor suffix, got: %q", msg)
	}
	if !strings.Contains(msg, "list") {
		t.Errorf("expected suggestion to include 'list', got: %q", msg)
	}
}

func TestApplySuggestionsToParents_InstallsOnEligibleParents(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	parent := &cobra.Command{Use: "parent"}
	leaf := &cobra.Command{Use: "leaf", Run: func(*cobra.Command, []string) {}}

	parent.AddCommand(leaf)
	root.AddCommand(parent)

	ApplySuggestionsToParents(root)

	if root.Args == nil {
		t.Error("expected root.Args to be installed")
	}
	if parent.Args == nil {
		t.Error("expected parent.Args to be installed")
	}
	if leaf.Args != nil {
		t.Error("expected leaf.Args to remain nil (has Run)")
	}
	// The walker installs a help-printing RunE on eligible parents so the
	// Args validator actually fires under cobra's execute() flow.
	if root.RunE == nil {
		t.Error("expected root.RunE to be installed")
	}
	if parent.RunE == nil {
		t.Error("expected parent.RunE to be installed")
	}
	if root.Annotations[SuggestionsRunEAnnotation] != "true" {
		t.Error("expected root annotation marker")
	}
	if parent.Annotations[SuggestionsRunEAnnotation] != "true" {
		t.Error("expected parent annotation marker")
	}
}

func TestApplySuggestionsToParents_RespectsRunGuard(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	parentWithRun := &cobra.Command{
		Use: "parent-run",
		Run: func(*cobra.Command, []string) {},
	}
	parentWithRun.AddCommand(&cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(parentWithRun)

	ApplySuggestionsToParents(root)

	if parentWithRun.Args != nil {
		t.Error("expected parent with Run to be skipped")
	}
}

func TestApplySuggestionsToParents_RespectsRunEGuard(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	parentWithRunE := &cobra.Command{
		Use:  "parent-rune",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	parentWithRunE.AddCommand(&cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(parentWithRunE)

	ApplySuggestionsToParents(root)

	if parentWithRunE.Args != nil {
		t.Error("expected parent with RunE to be skipped")
	}
}

func TestApplySuggestionsToParents_RespectsArgsGuard(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	custom := cobra.ExactArgs(1)
	parentWithArgs := &cobra.Command{
		Use:  "parent-args",
		Args: custom,
	}
	parentWithArgs.AddCommand(&cobra.Command{Use: "child", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(parentWithArgs)

	ApplySuggestionsToParents(root)

	// We can't compare function values for equality, but we can confirm the
	// validator still rejects the wrong number of args and behaves like
	// ExactArgs(1) rather than SuggestSubcommand (which would accept empty
	// args).
	if err := parentWithArgs.Args(parentWithArgs, []string{}); err == nil {
		t.Error("expected pre-existing ExactArgs(1) validator to reject empty args, but it accepted them — guard failed")
	}
}

func TestApplySuggestionsToParents_RecursesDepth2AndBeyond(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	l1 := &cobra.Command{Use: "l1"}
	l2 := &cobra.Command{Use: "l2"}
	l3 := &cobra.Command{Use: "l3"}
	leaf := &cobra.Command{Use: "leaf", Run: func(*cobra.Command, []string) {}}

	l3.AddCommand(leaf)
	l2.AddCommand(l3)
	l1.AddCommand(l2)
	root.AddCommand(l1)

	ApplySuggestionsToParents(root)

	for _, c := range []*cobra.Command{root, l1, l2, l3} {
		if c.Args == nil {
			t.Errorf("expected Args installed at depth for %q", c.Use)
		}
	}
	if leaf.Args != nil {
		t.Error("expected leaf.Args to remain nil")
	}

	// Sanity-check that the installed Args behaves like SuggestSubcommand at
	// depth >= 2 (returns the unknown-command error for typos).
	err := l2.Args(l2, []string{"typo"})
	if err == nil {
		t.Fatal("expected error from suggestion validator at depth 2")
	}
	if !strings.Contains(err.Error(), `unknown command "typo"`) {
		t.Errorf("expected unknown-command error at depth 2, got: %q", err.Error())
	}
}

func TestApplySuggestionsToParents_NilRootSafe(t *testing.T) {
	// Should not panic.
	ApplySuggestionsToParents(nil)
}

func TestApplySuggestionsToParents_LeafWithoutSubcommandsSkipped(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	// A leaf command with no Run/RunE/subcommands — would never happen in
	// real cobra trees, but the walker should still skip it because
	// HasSubCommands() is false.
	leaf := &cobra.Command{Use: "leaf"}
	root.AddCommand(leaf)

	ApplySuggestionsToParents(root)

	if leaf.Args != nil {
		t.Error("expected leaf without subcommands to be skipped (HasSubCommands guard)")
	}
}
