// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package skill provides shared logic for the per-binary `skill` cobra
// subcommand: agent catalog, path expansion, bundle filesystem ops, and
// the install/remove/list/check installer.
package skill

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

// Agent describes a supported AI agent: a binary-agnostic record of where
// the agent's install marker lives and where its skill bundles go.
//
// DetectDir / SkillsDir are stored in their unexpanded form (with `~` and
// `$XDG_CONFIG_HOME`); call DetectPath / SkillsPath to resolve.
type Agent struct {
	Name        string // canonical lowercase id, e.g. "claude-code"
	DisplayName string // human-readable, e.g. "Claude Code"
	DetectDir   string // unexpanded path used to detect agent presence
	SkillsDir   string // unexpanded path where skill bundles are placed
}

// AGENTS is the supported agent catalog. Order is preserved for stable
// list output. Mirrors the Rust reference (oskarhane/neo4j-query).
var AGENTS = []Agent{
	{Name: "claude-code", DisplayName: "Claude Code", DetectDir: "~/.claude", SkillsDir: "~/.claude/skills"},
	{Name: "cursor", DisplayName: "Cursor", DetectDir: "~/.cursor", SkillsDir: "~/.cursor/skills"},
	{Name: "windsurf", DisplayName: "Windsurf", DetectDir: "~/.codeium/windsurf", SkillsDir: "~/.codeium/windsurf/skills"},
	{Name: "copilot", DisplayName: "Copilot", DetectDir: "~/.copilot", SkillsDir: "~/.copilot/skills"},
	{Name: "gemini-cli", DisplayName: "Gemini CLI", DetectDir: "~/.gemini", SkillsDir: "~/.gemini/skills"},
	{Name: "cline", DisplayName: "Cline", DetectDir: "~/.cline", SkillsDir: "~/.agents/skills"},
	{Name: "codex", DisplayName: "Codex", DetectDir: "~/.codex", SkillsDir: "~/.codex/skills"},
	{Name: "pi", DisplayName: "Pi", DetectDir: "~/.pi/agent", SkillsDir: "~/.pi/agent/skills"},
	{Name: "opencode", DisplayName: "OpenCode", DetectDir: "$XDG_CONFIG_HOME/opencode", SkillsDir: "$XDG_CONFIG_HOME/opencode/skills"},
	{Name: "junie", DisplayName: "Junie", DetectDir: "~/.junie", SkillsDir: "~/.junie/skills"},
}

// DetectPath returns the expanded DetectDir and ok=true if expansion
// succeeded. ok=false signals that no $HOME is available, in which case
// the agent should be treated as not-detected.
func (a Agent) DetectPath() (string, bool) {
	return expandPath(a.DetectDir)
}

// SkillsPath returns the expanded SkillsDir. See DetectPath for the ok
// semantics.
func (a Agent) SkillsPath() (string, bool) {
	return expandPath(a.SkillsDir)
}

// FindAgent looks up an agent by name, case-insensitive. Returns nil if
// no match. The returned pointer is stable (points into AGENTS).
func FindAgent(name string) *Agent {
	lower := strings.ToLower(name)
	for i := range AGENTS {
		if AGENTS[i].Name == lower {
			return &AGENTS[i]
		}
	}
	return nil
}

// DetectAgents returns the subset of AGENTS whose DetectDir exists on the
// given filesystem. Hermetic-friendly: pass afero.NewMemMapFs in tests.
// Order matches AGENTS.
func DetectAgents(fs afero.Fs) []*Agent {
	out := make([]*Agent, 0, len(AGENTS))
	for i := range AGENTS {
		p, ok := AGENTS[i].DetectPath()
		if !ok {
			continue
		}
		exists, err := afero.DirExists(fs, p)
		if err != nil || !exists {
			continue
		}
		out = append(out, &AGENTS[i])
	}
	return out
}

// expandPath resolves a path containing `~` or `$XDG_CONFIG_HOME`.
//   - `~` alone -> $HOME
//   - `~/foo`   -> $HOME/foo
//   - paths containing `$XDG_CONFIG_HOME` are substituted with that env
//     var, falling back to $HOME/.config when unset or empty
//   - other paths are returned unchanged
//
// Returns ok=false only when $HOME is missing and is needed (the path
// references `~` or falls through to the XDG fallback).
func expandPath(path string) (string, bool) {
	home := os.Getenv("HOME")

	if path == "~" {
		if home == "" {
			return "", false
		}
		return home, true
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home == "" {
			return "", false
		}
		return filepath.Join(home, rest), true
	}
	if strings.Contains(path, "$XDG_CONFIG_HOME") {
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			if home == "" {
				return "", false
			}
			xdg = filepath.Join(home, ".config")
		}
		// Catalog entries keep forward slashes (portable convention) but
		// `xdg` may already contain OS-native separators (e.g. `C:\…\.config`
		// on Windows). Run the substitution through filepath.FromSlash so
		// the whole result uses the OS separator — otherwise mixing yields
		// `C:\…\.config/opencode` on Windows.
		return filepath.FromSlash(strings.ReplaceAll(path, "$XDG_CONFIG_HOME", xdg)), true
	}
	return path, true
}
