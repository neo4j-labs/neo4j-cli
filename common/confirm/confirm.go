// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package confirm provides a shared `--yes` / `--force` gate for destructive
// cobra leaves. Register binds the flags on cmd as a side effect; Require
// enforces them: TTY callers get a y/N prompt, non-TTY callers must pass both
// flags or receive a usage error (exit 2). Cancelled TTY prompts return
// ErrCancelled so leaves can exit 0 cleanly.
package confirm

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// ErrCancelled is returned by Require when the TTY caller declines the y/N
// prompt. Leaves match it via errors.Is and return nil so the process exits 0
// with no destructive action taken.
var ErrCancelled = errors.New("confirmation cancelled")

// stdinIsTerminal is the package-local TTY detector; swapped by
// SetStdinIsTerminalForTest so leaf tests can drive both branches without a
// real PTY.
var stdinIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// SetStdinIsTerminalForTest overrides the TTY detector for the duration of
// the test and restores it via t.Cleanup.
func SetStdinIsTerminalForTest(t *testing.T, fn func() bool) {
	t.Helper()
	prev := stdinIsTerminal
	stdinIsTerminal = fn
	t.Cleanup(func() { stdinIsTerminal = prev })
}

// Register binds --yes and --force on cmd. Both default to false. Call as a
// side-effect statement after the cobra.Command struct literal.
func Register(cmd *cobra.Command) {
	cmd.Flags().Bool("yes", false, "Confirm the destructive action. Required together with --force for non-TTY callers.")
	cmd.Flags().Bool("force", false, "Confirm the destructive action. Required together with --yes for non-TTY callers.")
}

// Require enforces the gate at call time by reading --yes and --force from
// cmd.Flags():
//   - both flags set ⇒ proceed (no prompt).
//   - non-TTY with either flag missing ⇒ *clierr.CLIError (exit 2).
//   - TTY with either flag missing ⇒ prompt; y/Y/yes proceeds, anything else
//     writes "cancelled." to stderr, silences cobra's own error/usage output,
//     and returns ErrCancelled. The top-level main intercepts ErrCancelled and
//     exits 0 with no further output, so leaves can just `return err`.
//
// resourceID is interpolated into the prompt and error copy; pass "" when the
// leaf has no positional argument and copy degrades to "this <type>".
func Require(cmd *cobra.Command, resourceID string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	force, _ := cmd.Flags().GetBool("force")
	if yes && force {
		return nil
	}

	resourceType := "resource"
	if parent := cmd.Parent(); parent != nil {
		if name := parent.Name(); name != "" {
			resourceType = name
		}
	}

	target := fmt.Sprintf("this %s", resourceType)
	if resourceID != "" {
		target = fmt.Sprintf("%s %q", resourceType, resourceID)
	}

	if !stdinIsTerminal() {
		return clierr.NewUsageError(
			"refusing to delete %s without confirmation: pass both --yes and --force to proceed non-interactively",
			target,
		)
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Delete %s? This action is irreversible. [y/N] ", target)
	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err == nil || line != "" {
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer == "y" || answer == "yes" {
			return nil
		}
	}
	return cancel(cmd)
}

// cancel narrates the cancellation to stderr, silences cobra's default
// error/usage rendering for this command, and returns ErrCancelled. The
// chokepoint in `neo4j-cli/main.go` matches ErrCancelled and exits 0.
func cancel(cmd *cobra.Command) error {
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "cancelled.")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	return ErrCancelled
}
