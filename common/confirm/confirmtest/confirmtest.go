// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package confirmtest provides a shared replay of the four canonical
// destructive-leaf gating scenarios so each leaf's _test.go can collapse
// ~80 lines of boilerplate into a single AssertLeafGate call.
//
// The helper is leaf-helper agnostic: callers wire their existing test
// helper (AuraTestHelper, a per-package deleteHelper, …) through the
// LeafGateCase.Run closure. The four scenarios drive (TTY × flag-state)
// and assert the contract enforced by confirm.Require:
//
//   - non-TTY without flags     → exit 2 with "pass both --yes and --force"; sink NOT invoked.
//   - non-TTY with --yes --force → sink invoked; nil (or non-gate) error.
//   - TTY + "y\n"               → sink invoked; "Delete <label>" on stderr.
//   - TTY + ""                  → sink NOT invoked; "cancelled." on stderr; ErrCancelled returned.
package confirmtest

import (
	"errors"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/confirm"
)

// LeafGateCase describes one destructive leaf's gating contract. The Run
// closure builds and executes the leaf for one scenario, returning what the
// scenario assertions need to inspect (returned error, stderr buffer, whether
// the destructive sink was invoked).
//
// Run is called four times — once per canonical scenario — and the caller is
// responsible for re-building any per-scenario state (a fresh httptest mock,
// a fresh cobra tree, a fresh credentials store, …) so the scenarios don't
// bleed into each other.
type LeafGateCase struct {
	// Name is included in the subtest name; usually the leaf's full command
	// path ("aura instance delete").
	Name string

	// NoFlagsArgs is the argv string passed when neither --yes nor --force
	// should be set (used by the TTY scenarios and the non-TTY-without-flags
	// scenario). Must NOT include "--yes" or "--force".
	NoFlagsArgs string

	// BothFlagsArgs is the argv string passed when both flags should be set
	// (used by the non-TTY-with-both-flags scenario). Must include
	// "--yes --force".
	BothFlagsArgs string

	// ResourceLabel is the parent-command name interpolated into the prompt
	// (e.g. "instance"); used to assert the TTY prompt mentions the resource.
	ResourceLabel string

	// Run executes the leaf for one scenario and returns the result. The
	// confirmtest helper toggles the TTY seam (via SetStdinIsTerminal +
	// t.Cleanup) before calling Run; Run does NOT need to set it.
	Run func(t *testing.T, args string, stdin string) GateRunResult
}

// GateRunResult captures the observable outputs of one scenario run.
type GateRunResult struct {
	// Err is the error returned by cmd.Execute (nil on success).
	Err error
	// Stderr is everything the leaf wrote to its err stream.
	Stderr string
	// Invoked reports whether the destructive sink (HTTP DELETE, fs mutation,
	// …) fired. False on the gated paths; true on the proceeds paths.
	Invoked bool
}

// AssertLeafGate replays the four canonical gating scenarios against c.Run
// and asserts the contract enforced by confirm.Require. Each scenario runs
// as a t.Run subtest so failures point at the offending scenario.
func AssertLeafGate(t *testing.T, c LeafGateCase) {
	t.Helper()

	t.Run("NonTTY_WithoutFlags_Exit2", func(t *testing.T) {
		t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
		got := c.Run(t, c.NoFlagsArgs, "")

		if got.Err == nil {
			t.Fatalf("expected error on non-TTY without flags; got nil")
		}
		var ce *clierr.CLIError
		if !errors.As(got.Err, &ce) {
			t.Fatalf("expected *clierr.CLIError on non-TTY without flags; got %T (%v)", got.Err, got.Err)
		}
		if ce.Code != 2 {
			t.Fatalf("expected exit code 2; got %d", ce.Code)
		}
		if !strings.Contains(got.Err.Error(), "pass both --yes and --force") {
			t.Fatalf("error %q missing 'pass both --yes and --force'", got.Err.Error())
		}
		if got.Invoked {
			t.Fatalf("destructive sink fired on gated path (non-TTY without flags)")
		}
	})

	t.Run("NonTTY_WithBothFlags_Proceeds", func(t *testing.T) {
		t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
		got := c.Run(t, c.BothFlagsArgs, "")

		if errors.Is(got.Err, confirm.ErrCancelled) {
			t.Fatalf("non-TTY with both flags must not cancel; got ErrCancelled")
		}
		if !got.Invoked {
			t.Fatalf("destructive sink did NOT fire on non-TTY with --yes --force (err=%v stderr=%q)", got.Err, got.Stderr)
		}
	})

	t.Run("TTY_AnswerY_Proceeds", func(t *testing.T) {
		t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return true }))
		got := c.Run(t, c.NoFlagsArgs, "y\n")

		if errors.Is(got.Err, confirm.ErrCancelled) {
			t.Fatalf("TTY 'y' answer must not cancel; got ErrCancelled")
		}
		if !got.Invoked {
			t.Fatalf("destructive sink did NOT fire on TTY 'y' (err=%v stderr=%q)", got.Err, got.Stderr)
		}
		if c.ResourceLabel != "" && !strings.Contains(got.Stderr, "Delete "+c.ResourceLabel) {
			t.Fatalf("expected prompt to mention 'Delete %s'; stderr=%q", c.ResourceLabel, got.Stderr)
		}
	})

	t.Run("TTY_AnswerN_Cancels", func(t *testing.T) {
		t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return true }))
		got := c.Run(t, c.NoFlagsArgs, "N\n")

		if !errors.Is(got.Err, confirm.ErrCancelled) {
			t.Fatalf("expected confirm.ErrCancelled on TTY 'N'; got %v", got.Err)
		}
		if got.Invoked {
			t.Fatalf("destructive sink fired on cancelled path")
		}
		if !strings.Contains(got.Stderr, "cancelled.") {
			t.Fatalf("expected 'cancelled.' on stderr; got %q", got.Stderr)
		}
	})
}
