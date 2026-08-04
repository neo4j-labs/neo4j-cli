// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package skill provides shared logic for the per-binary `skill` cobra
// subcommand: agent catalog, path expansion, bundle filesystem ops, and
// the install/remove/list/check installer.
package skill

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/afero"
)

// MCPFormat identifies the on-disk shape of an agent's MCP server config,
// so a writer can be selected without special-casing agent names. Only
// meaningful when Agent.MCPConfig is set. Unrelated to the CLI's
// `--format json|table|toon` output format.
type MCPFormat string

// MCPFormatServersJSON is a JSON document carrying a top-level `mcpServers`
// object keyed by server name.
const MCPFormatServersJSON MCPFormat = "mcp-servers-json"

// Agent describes a supported AI agent: a binary-agnostic record of where
// the agent's install marker lives, where its skill bundles go, and where
// its MCP server config lives.
//
// Capability is expressed by presence: an empty SkillsDir means the agent
// is not a skill target, an empty MCPConfig means it is not an MCP target.
// Use SkillAgents / MCPAgents to project the catalog rather than assuming
// every entry supports both.
//
// DetectDir / SkillsDir / MCPConfig are stored in their unexpanded form
// (with `~`, `$XDG_CONFIG_HOME` and `$APP_SUPPORT`); call DetectPath /
// SkillsPath / MCPConfigPath to resolve.
type Agent struct {
	Name        string    // canonical lowercase id, e.g. "claude-code"
	DisplayName string    // human-readable, e.g. "Claude Code"
	DetectDir   string    // unexpanded path used to detect agent presence
	SkillsDir   string    // unexpanded path where skill bundles are placed
	MCPConfig   string    // unexpanded path to the agent's MCP server config file
	MCPFormat   MCPFormat // shape of MCPConfig
}

// AGENTS is the supported agent catalog. Order is preserved for stable
// list output. Mirrors the Rust reference (oskarhane/neo4j-query).
var AGENTS = []Agent{
	{Name: "claude-code", DisplayName: "Claude Code", DetectDir: "~/.claude", SkillsDir: "~/.claude/skills"},
	{Name: "cursor", DisplayName: "Cursor", DetectDir: "~/.cursor", SkillsDir: "~/.cursor/skills"},
	{Name: "windsurf", DisplayName: "Windsurf", DetectDir: "~/.codeium/windsurf", SkillsDir: "~/.codeium/windsurf/skills"},
	{Name: "copilot", DisplayName: "Copilot", DetectDir: "~/.copilot", SkillsDir: "~/.copilot/skills"},
	{Name: "antigravity", DisplayName: "Antigravity", DetectDir: "~/.gemini/antigravity", SkillsDir: "~/.gemini/antigravity/skills"},
	{Name: "gemini-cli", DisplayName: "Gemini CLI", DetectDir: "~/.gemini", SkillsDir: "~/.gemini/skills"},
	{Name: "cline", DisplayName: "Cline", DetectDir: "~/.cline", SkillsDir: "~/.agents/skills"},
	{Name: "codex", DisplayName: "Codex", DetectDir: "~/.codex", SkillsDir: "~/.codex/skills"},
	{Name: "pi", DisplayName: "Pi", DetectDir: "~/.pi/agent", SkillsDir: "~/.pi/agent/skills"},
	{Name: "opencode", DisplayName: "OpenCode", DetectDir: "$XDG_CONFIG_HOME/opencode", SkillsDir: "$XDG_CONFIG_HOME/opencode/skills"},
	{Name: "junie", DisplayName: "Junie", DetectDir: "~/.junie", SkillsDir: "~/.junie/skills"},
	{
		Name:        "claude-desktop",
		DisplayName: "Claude Desktop",
		DetectDir:   "$APP_SUPPORT/Claude",
		MCPConfig:   "$APP_SUPPORT/Claude/claude_desktop_config.json",
		MCPFormat:   MCPFormatServersJSON,
	},
}

// DetectPath returns the expanded DetectDir and ok=true if expansion
// succeeded. ok=false signals that no $HOME is available, in which case
// the agent should be treated as not-detected.
func (a Agent) DetectPath() (string, bool) {
	return expandPath(a.DetectDir)
}

// SkillsPath returns the expanded SkillsDir. See DetectPath for the ok
// semantics. An absent capability is ok=false, not an empty path:
// `filepath.Join("", name)` is a relative path a caller would happily write
// into the working directory, so the guard belongs here and not only at the
// call sites.
func (a Agent) SkillsPath() (string, bool) {
	if !a.SupportsSkills() {
		return "", false
	}
	return expandPath(a.SkillsDir)
}

// MCPConfigPath returns the expanded MCPConfig. See SkillsPath for the ok
// semantics.
func (a Agent) MCPConfigPath() (string, bool) {
	if !a.SupportsMCP() {
		return "", false
	}
	return expandPath(a.MCPConfig)
}

// SupportsSkills reports whether this entry is a skill-bundle target.
func (a Agent) SupportsSkills() bool { return a.SkillsDir != "" }

// SupportsMCP reports whether this entry is an MCP server-config target.
func (a Agent) SupportsMCP() bool { return a.MCPConfig != "" }

// SkillAgents returns the skill-capable catalog entries in AGENTS order.
// Every skill-side consumer must project through this rather than walking
// AGENTS: an MCP-only entry has no SkillsDir, so installing into it would
// resolve to a relative path.
func SkillAgents() []*Agent { return filterAgents(Agent.SupportsSkills) }

// MCPAgents returns the MCP-capable catalog entries in AGENTS order.
func MCPAgents() []*Agent { return filterAgents(Agent.SupportsMCP) }

func filterAgents(pred func(Agent) bool) []*Agent {
	out := make([]*Agent, 0, len(AGENTS))
	for i := range AGENTS {
		if pred(AGENTS[i]) {
			out = append(out, &AGENTS[i])
		}
	}
	return out
}

// skillAgentCount is the size of SkillAgents() without the allocation, for
// capacity hints and the `N/M` denominator.
func skillAgentCount() int {
	n := 0
	for i := range AGENTS {
		if AGENTS[i].SupportsSkills() {
			n++
		}
	}
	return n
}

// FindAgent looks up an agent by name, case-insensitive, across the whole
// catalog regardless of capability. Returns nil if no match. The returned
// pointer is stable (points into AGENTS).
func FindAgent(name string) *Agent {
	lower := strings.ToLower(name)
	for i := range AGENTS {
		if AGENTS[i].Name == lower {
			return &AGENTS[i]
		}
	}
	return nil
}

// DetectAgents returns the skill-capable agents whose DetectDir exists on
// the given filesystem. Hermetic-friendly: pass afero.NewMemMapFs in
// tests. Order matches AGENTS.
func DetectAgents(fs afero.Fs) []*Agent {
	return detectAgentsIn(fs, SkillAgents())
}

// DetectMCPAgents is DetectAgents for the MCP-capable subset.
func DetectMCPAgents(fs afero.Fs) []*Agent {
	return detectAgentsIn(fs, MCPAgents())
}

func detectAgentsIn(fs afero.Fs, candidates []*Agent) []*Agent {
	out := make([]*Agent, 0, len(candidates))
	for _, a := range candidates {
		p, ok := a.DetectPath()
		if !ok {
			continue
		}
		exists, err := afero.DirExists(fs, p)
		if err != nil || !exists {
			continue
		}
		out = append(out, a)
	}
	return out
}

const (
	tokenXDGConfigHome = "$XDG_CONFIG_HOME"
	tokenAppSupport    = "$APP_SUPPORT"
)

// currentGOOS is a seam so the per-platform branches of appSupportDir are
// reachable from a test on any host.
var currentGOOS = runtime.GOOS

// expandPath resolves a path containing `~`, `$XDG_CONFIG_HOME` or
// `$APP_SUPPORT`.
//   - `~` alone -> $HOME
//   - `~/foo`   -> $HOME/foo
//   - `$XDG_CONFIG_HOME` -> that env var, falling back to $HOME/.config
//     when unset or empty
//   - `$APP_SUPPORT` -> the per-platform application-support root (see
//     appSupportDir)
//   - other paths are returned unchanged
//
// Returns ok=false only when the environment cannot supply the base the
// path needs.
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
	if strings.Contains(path, tokenAppSupport) {
		base, ok := appSupportDir(home)
		if !ok {
			return "", false
		}
		return substituteToken(path, tokenAppSupport, base), true
	}
	if strings.Contains(path, tokenXDGConfigHome) {
		xdg, ok := xdgConfigDir(home)
		if !ok {
			return "", false
		}
		return substituteToken(path, tokenXDGConfigHome, xdg), true
	}
	return path, true
}

// substituteToken replaces `token` with `base` and normalises the result.
// Catalog entries keep forward slashes (portable convention) but `base` may
// already contain OS-native separators (e.g. `C:\…\.config` on Windows).
// Running the substitution through filepath.FromSlash keeps the whole
// result on the OS separator — otherwise mixing yields
// `C:\…\.config/opencode` on Windows.
func substituteToken(path, token, base string) string {
	return filepath.FromSlash(strings.ReplaceAll(path, token, base))
}

// xdgConfigDir resolves $XDG_CONFIG_HOME, falling back to $HOME/.config
// when unset or empty.
func xdgConfigDir(home string) (string, bool) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg, true
	}
	if home == "" {
		return "", false
	}
	return filepath.Join(home, ".config"), true
}

// appSupportDir resolves the per-user application-support root that GUI apps
// write into. The three branches are deliberately Electron's `appData`
// mapping — `~/Library/Application Support`, `%APPDATA%`, then the XDG
// config dir — because the agents reached through this token are Electron
// apps, so their config lands wherever Electron puts it. The default branch
// therefore stays a real answer rather than an error, even though only
// darwin and windows have official Claude Desktop builds; an absent
// directory already resolves to not-detected.
//
// Reads $HOME rather than user.Current so subprocess tests stay isolated,
// following common/clicfg/darwin.go.
func appSupportDir(home string) (string, bool) {
	switch currentGOOS {
	case "darwin":
		if home == "" {
			return "", false
		}
		return filepath.Join(home, "Library", "Application Support"), true
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return appData, true
		}
		// Defensive: a real Windows session always sets %APPDATA%.
		if home == "" {
			return "", false
		}
		return filepath.Join(home, "AppData", "Roaming"), true
	default:
		return xdgConfigDir(home)
	}
}
