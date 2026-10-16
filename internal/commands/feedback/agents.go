// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	serverName       = "neo4j-feedback"
	claudeCodeAgent  = "claude-code"
	claudeBinary     = "claude"
	codexAgent       = "codex"
	codexBinary      = "codex"
	registerDeadline = 15 * time.Second
)

// Seams for tests. Registration shells out to the `claude` CLI so this package
// never edits ~/.claude.json, which also holds unrelated Claude Code state.
var (
	lookPath       = exec.LookPath
	executablePath = os.Executable
	execClaude     = func(ctx context.Context, args ...string) error { return runAgentCLI(ctx, claudeBinary, args...) }
	execCodex      = func(ctx context.Context, args ...string) error { return runAgentCLI(ctx, codexBinary, args...) }
)

// runAgentCLI runs an agent's CLI and, on failure, keeps the first line of its
// output in the error, so notes say why (for example "Read-only file system")
// and not just "exit status 1".
func runAgentCLI(ctx context.Context, bin string, args ...string) error {
	out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err == nil {
		return nil
	}
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0])
	if line == "" {
		return err
	}
	if len(line) > 160 {
		line = line[:160]
	}
	return fmt.Errorf("%w: %s", err, line)
}

// manualAddCommand is what an operator runs when automatic registration is not
// possible.
func manualAddCommand(bin, variant string) string {
	return fmt.Sprintf("claude mcp add -s user %s -e %s=%s -e %s=true -- %s feedback serve", serverName, instructionsEnv, variant, flagEnv, bin)
}

// selfPath is the path the agents are told to run. It resolves symlinks to find
// the real binary, but prefers the stable location on the PATH when that points
// at the same file: a Homebrew `bin/neo4j-cli` symlink survives `brew upgrade`,
// while the versioned Cellar path it points to is removed.
func selfPath() string {
	p, err := executablePath()
	if err != nil {
		return "neo4j-cli"
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p
	}
	if lp, err := lookPath("neo4j-cli"); err == nil {
		if abs, err := filepath.Abs(lp); err == nil {
			if lreal, err := filepath.EvalSymlinks(abs); err == nil && lreal == real {
				return abs
			}
		}
	}
	return real
}

func claudeAvailable() bool {
	_, err := lookPath(claudeBinary)
	return err == nil
}

// flagEnv turns on flag.feedback-command for the registered server, so the
// server starts even when the flag was set only in the installer's
// environment. It goes away with the flag.
const flagEnv = "NEO4J_CLI_FLAG_FEEDBACK_COMMAND"

// registerClaudeCode registers the feedback server with Claude Code at user
// scope. It returns the agents now registered and notes for the operator.
// The server instructions variant is set in the registration environment.
// Idempotent: any existing entry is replaced so the binary path and variant stay current.
func registerClaudeCode(ctx context.Context, variant string) (agents, notes []string) {
	return registerClaudeCodeAt(ctx, selfPath(), variant)
}

// lostNote is added when re-registering removed a working entry and the new
// one could not be added, so the person knows the agent has none now.
const lostNote = " The previous registration was removed, so the agent has none until this succeeds."

func registerClaudeCodeAt(ctx context.Context, bin, variant string) (agents, notes []string) {
	if !claudeAvailable() {
		return []string{}, []string{"Claude Code not registered: `claude` was not found on PATH. Run: " + manualAddCommand(bin, variant)}
	}
	ctx, cancel := context.WithTimeout(ctx, registerDeadline)
	defer cancel()
	removed := execClaude(ctx, "mcp", "remove", "-s", "user", serverName) == nil // absent is fine
	if err := execClaude(ctx, "mcp", "add", "-s", "user", serverName,
		"-e", instructionsEnv+"="+variant, "-e", flagEnv+"=true",
		"--", bin, "feedback", "serve"); err != nil {
		note := "Claude Code registration failed (" + err.Error() + "). Run: " + manualAddCommand(bin, variant)
		if removed {
			note += lostNote
		}
		return []string{}, []string{note}
	}
	return []string{claudeCodeAgent}, nil
}

// unregisterClaudeCode removes the feedback server from Claude Code. A missing
// `claude` binary or entry is not an error: there is nothing to remove.
func unregisterClaudeCode(ctx context.Context) (notes []string) {
	if !claudeAvailable() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, registerDeadline)
	defer cancel()
	if !claudeRegistered(ctx) {
		return nil
	}
	if err := execClaude(ctx, "mcp", "remove", "-s", "user", serverName); err != nil {
		return []string{"Claude Code unregistration failed (" + err.Error() + "). Run: claude mcp remove -s user " + serverName}
	}
	return nil
}

// claudeRegistered reports whether Claude Code currently has the server.
func claudeRegistered(ctx context.Context) bool {
	if !claudeAvailable() {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, registerDeadline)
	defer cancel()
	return execClaude(ctx, "mcp", "get", serverName) == nil
}

// registeredAgents lists the agents that currently have the feedback server.
func registeredAgents(ctx context.Context) []string {
	agents := []string{}
	if claudeRegistered(ctx) {
		agents = append(agents, claudeCodeAgent)
	}
	if codexRegistered(ctx) {
		agents = append(agents, codexAgent)
	}
	return agents
}

func codexAvailable() bool {
	_, err := lookPath(codexBinary)
	return err == nil
}

func manualCodexAddCommand(bin, variant string) string {
	return fmt.Sprintf("codex mcp add %s --env %s=%s --env %s=true -- %s feedback serve", serverName, instructionsEnv, variant, flagEnv, bin)
}

// registerCodex registers the feedback server with Codex. Codex starts MCP
// servers with a restricted environment, so the variant and the (experimental)
// feature flag are passed explicitly. Idempotent: an existing entry is replaced.
func registerCodex(ctx context.Context, variant string) (agents, notes []string) {
	return registerCodexAt(ctx, selfPath(), variant)
}

func registerCodexAt(ctx context.Context, bin, variant string) (agents, notes []string) {
	ctx, cancel := context.WithTimeout(ctx, registerDeadline)
	defer cancel()
	removed := execCodex(ctx, "mcp", "remove", serverName) == nil // absent is fine
	if err := execCodex(ctx, "mcp", "add", serverName,
		"--env", instructionsEnv+"="+variant, "--env", flagEnv+"=true",
		"--", bin, "feedback", "serve"); err != nil {
		note := "Codex registration failed (" + err.Error() + "). Run: " + manualCodexAddCommand(bin, variant)
		if removed {
			note += lostNote
		}
		return []string{}, []string{note}
	}
	return []string{codexAgent}, nil
}

// unregisterCodex removes the feedback server from Codex. A missing `codex`
// binary or entry is not an error.
func unregisterCodex(ctx context.Context) (notes []string) {
	if !codexAvailable() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, registerDeadline)
	defer cancel()
	if !codexRegistered(ctx) {
		return nil
	}
	if err := execCodex(ctx, "mcp", "remove", serverName); err != nil {
		return []string{"Codex unregistration failed (" + err.Error() + "). Run: codex mcp remove " + serverName}
	}
	return nil
}

func codexRegistered(ctx context.Context) bool {
	if !codexAvailable() {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, registerDeadline)
	defer cancel()
	return execCodex(ctx, "mcp", "get", serverName) == nil
}

// registerAgents registers the tool with every supported agent that is
// installed (Claude Code, Codex). An agent that is not installed is skipped
// silently; only when none is installed does it say how to register by hand.
func registerAgents(ctx context.Context, variant string) (agents, notes []string) {
	agents = []string{}
	bin := selfPath()
	hasClaude, hasCodex := claudeAvailable(), codexAvailable()
	if hasClaude {
		a, n := registerClaudeCodeAt(ctx, bin, variant)
		agents, notes = append(agents, a...), append(notes, n...)
	}
	if hasCodex {
		a, n := registerCodexAt(ctx, bin, variant)
		agents, notes = append(agents, a...), append(notes, n...)
	}
	if !hasClaude && !hasCodex {
		notes = append(notes,
			"No supported coding agent found on PATH (Claude Code, Codex), so nothing was registered. "+
				"Claude Code: `"+manualAddCommand(bin, variant)+"`. "+
				"Codex: `"+manualCodexAddCommand(bin, variant)+"`.")
	}
	// A path that will not outlive a cache clean is worth a warning, once, whichever agents were registered.
	if kind := ephemeralKind(bin); kind != "" && len(agents) > 0 {
		notes = append(notes, ephemeralWarning(bin, kind))
	}
	return agents, notes
}

// unregisterAgents removes the feedback server from every agent that has it.
func unregisterAgents(ctx context.Context) (notes []string) {
	notes = append(notes, unregisterClaudeCode(ctx)...)
	notes = append(notes, unregisterCodex(ctx)...)
	return notes
}
