// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package agent detects whether the CLI is being driven by a known agent
// harness (Claude Code, Codex, Cursor, …) via well-known environment
// variables. The agent list is sourced from upstream `unjs/std-env`
// (https://github.com/unjs/std-env/blob/main/src/agents.ts).
package agent

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// getenv is the overridable seam for environment lookups so tests can drive
// detection without mutating real process state.
var getenv = os.Getenv

// stdinIsTerminal is the overridable seam for stdin TTY detection, mirroring
// the getenv seam so tests can drive the Invoker matrix.
var stdinIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// presenceVars enumerates env vars whose mere presence (non-empty) marks a
// known agent harness.
var presenceVars = []string{
	"CLAUDECODE",      // Claude Code
	"CLAUDE_CODE",     // Claude Code (alt)
	"REPL_ID",         // Replit
	"GEMINI_CLI",      // Gemini CLI
	"CODEX_SANDBOX",   // Codex
	"CODEX_THREAD_ID", // Codex (alt)
	"OPENCODE",        // OpenCode
	"AUGMENT_AGENT",   // Auggie
	"GOOSE_PROVIDER",  // Goose
	"CURSOR_AGENT",    // Cursor
}

// substringRule matches an agent by case-insensitive substring of an env var
// value (mirrors upstream `unjs/std-env` substring checks).
type substringRule struct {
	envVar    string
	substring string // already lowercase
}

var substringRules = []substringRule{
	{envVar: "EDITOR", substring: "devin"},      // Devin
	{envVar: "TERM_PROGRAM", substring: "kiro"}, // Kiro
	{envVar: "PATH", substring: ".pi/agent"},    // pi
}

// Detect returns true if any known agent-harness env var is set.
func Detect() bool {
	for _, name := range presenceVars {
		if getenv(name) != "" {
			return true
		}
	}
	for _, rule := range substringRules {
		v := getenv(rule.envVar)
		if v == "" {
			continue
		}
		if strings.Contains(strings.ToLower(v), rule.substring) {
			return true
		}
	}
	return false
}

// Invoker is the single source of truth for agent/non-agent classification
// across the CLI (command history + telemetry). It returns "agent" when a known
// agent harness is detected (Detect) or when stdin is not a TTY, and "non-agent"
// only for an interactive terminal with no harness env var. "non-agent" covers
// interactive humans as well as scripted/CI use that runs in a TTY.
func Invoker() string {
	if Detect() || !stdinIsTerminal() {
		return "agent"
	}
	return "non-agent"
}
