// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package agent detects whether the CLI is being driven by a known agent
// harness (Claude Code, Codex, Cursor, Junie, Copilot, …) via well-known
// environment variables. The signals mirror two upstream lists:
//
//   - `unjs/std-env` (https://github.com/unjs/std-env/blob/main/src/agents.ts)
//   - `vercel/detect-agent` (https://github.com/vercel/detect-agent/blob/main/agents.json)
//
// `AI_AGENT` is checked first: any non-empty value marks an agent, as upstream
// does. Its value is user-controlled, so Harness only forwards it when it names
// a harness this package already reports. Signals that an IDE also sets for a
// human (CURSOR_TRACE_ID, COPILOT_GITHUB_TOKEN, …) are deliberately absent:
// every signal here means "an agent drives this process".
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

// stdoutIsTerminal is the overridable seam for stdout TTY detection, mirroring
// stdinIsTerminal. An IDE-integrated terminal sets TERM_PROGRAM for a human
// too, so rules marked noTTY match only when stdout is NOT a terminal (an
// agent-spawned, non-interactive process) — same gate as upstream #185.
var stdoutIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// presenceVars enumerates env vars whose mere presence (non-empty) marks a
// known agent harness. label is the public harness name Harness reports, or
// "" for harnesses it does not name.
var presenceVars = []struct {
	name  string
	label string
}{
	{"CLAUDECODE", "claude-code"},        // Claude Code
	{"CLAUDE_CODE", "claude-code"},       // Claude Code (alt)
	{"REPL_ID", ""},                      // Replit
	{"GEMINI_CLI", "gemini-cli"},         // Gemini CLI
	{"CODEX_SANDBOX", "codex"},           // Codex
	{"CODEX_THREAD_ID", "codex"},         // Codex (alt)
	{"OPENCODE", "opencode"},             // OpenCode
	{"AUGMENT_AGENT", ""},                // Auggie
	{"GOOSE_PROVIDER", ""},               // Goose
	{"CURSOR_AGENT", "cursor"},           // Cursor
	{"JUNIE_DATA", "junie"},              // Junie
	{"JUNIE_SHIM_PATH", "junie"},         // Junie (alt)
	{"COPILOT_AGENT", "copilot"},         // GitHub Copilot
	{"COPILOT_CLI", "copilot"},           // GitHub Copilot (CLI)
	{"CLINE_ACTIVE", "cline"},            // Cline
	{"ANTIGRAVITY_AGENT", "antigravity"}, // Antigravity
}

// substringRule matches an agent by case-insensitive substring of an env var
// value (mirrors upstream `unjs/std-env` substring checks).
type substringRule struct {
	envVar    string
	substring string // already lowercase
	noTTY     bool   // match only when stdout is not a terminal
}

var substringRules = []substringRule{
	{envVar: "EDITOR", substring: "devin"},   // Devin
	{envVar: "PATH", substring: ".pi/agent"}, // pi
	// IDE-integrated terminals last: the Kiro IDE sets TERM_PROGRAM for a
	// human typing in its terminal too, so only a non-TTY stdout is an agent.
	{envVar: "TERM_PROGRAM", substring: "kiro", noTTY: true}, // Kiro
}

// harnessLabels is the closed set of labels Harness reports. AI_AGENT is
// user-controlled and feeds telemetry, history and feedback labels, so its
// value is forwarded only when it names one of these.
var harnessLabels = map[string]bool{
	"claude-code": true,
	"gemini-cli":  true,
	"codex":       true,
	"opencode":    true,
	"cursor":      true,
	"junie":       true,
	"copilot":     true,
	"cline":       true,
	"antigravity": true,
}

// aiAgentValue returns the lowercased, trimmed AI_AGENT value, or "" when unset
// or whitespace-only.
func aiAgentValue() string {
	return strings.ToLower(strings.TrimSpace(getenv("AI_AGENT")))
}

// harnessLabelForAI maps an AI_AGENT value to a public harness label, or ""
// when the value is not a known harness. The two Copilot vendor names upstream
// uses normalise to the same label.
func harnessLabelForAI(v string) string {
	switch v {
	case "github_copilot_vscode_agent", "github_copilot_cloud_agent":
		v = "copilot"
	}
	if harnessLabels[v] {
		return v
	}
	return ""
}

// Detect returns true if any known agent-harness env var is set.
func Detect() bool {
	if aiAgentValue() != "" {
		return true
	}
	for _, v := range presenceVars {
		if getenv(v.name) != "" {
			return true
		}
	}
	for _, rule := range substringRules {
		if rule.noTTY && stdoutIsTerminal() {
			continue
		}
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

// Harness returns the public label of the detected agent harness
// (claude-code, gemini-cli, codex, opencode, cursor, junie, copilot, cline,
// antigravity), or "" when the harness is unknown or unlabelled. AI_AGENT wins
// only when its value names a known harness, so an arbitrary string never
// becomes a label.
func Harness() string {
	if v := aiAgentValue(); v != "" {
		if label := harnessLabelForAI(v); label != "" {
			return label
		}
	}
	for _, v := range presenceVars {
		if v.label != "" && getenv(v.name) != "" {
			return v.label
		}
	}
	return ""
}

// Invoker is the single source of truth for caller classification across the
// CLI (command history + telemetry). It returns one of three values:
//
//   - "agent"       — a known agent harness env var is set (Detect)
//   - "script"      — no harness and stdin is not a TTY (piped, CI, cron)
//   - "human"       — no harness and stdin is a TTY (a person at a terminal)
func Invoker() string {
	switch {
	case Detect():
		return "agent"
	case !stdinIsTerminal():
		return "script"
	default:
		return "human"
	}
}
