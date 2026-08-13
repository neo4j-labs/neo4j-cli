// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/neo4j/cli/common/clierr"
)

// formatAgentErr converts skill-package sentinel errors into user-facing
// usage errors that include the valid agent names. Other errors pass
// through unchanged. Used by install + remove leaves.
// When ErrUnknownAgent names an MCP-only agent, the error points at
// `mcp install --agent <name>` instead of listing skill agents.
func formatAgentErr(err error) error {
	switch {
	case errors.Is(err, ErrUnknownAgent):
		if name := extractAgentName(err); name != "" && isMCPOnlyAgent(name) {
			return clierr.NewUsageError("unknown agent: %q is an MCP-only agent — use 'mcp install --agent %s' instead", name, name)
		}
		return clierr.NewUsageError("%v\nvalid agents: %s", err, strings.Join(agentNames(), ", "))
	case errors.Is(err, ErrAgentNotDetected):
		return clierr.NewUsageError("%v", err)
	case errors.Is(err, ErrNoAgentsDetected):
		return clierr.NewUsageError("%v\nvalid agents: %s", err, strings.Join(agentNames(), ", "))
	default:
		return err
	}
}

// isMCPOnlyAgent reports whether name matches a catalog entry that supports
// MCP but not skills. Used so `skill install --agent claude-desktop` produces
// a helpful error pointing at `mcp install --agent <name>`.
func isMCPOnlyAgent(name string) bool {
	a := FindAgent(name)
	return a != nil && a.SupportsMCP() && !a.SupportsSkills()
}

// extractAgentName attempts to extract the quoted agent name from an
// ErrUnknownAgent-wrapping error which is formatted as `skill: unknown agent: "name"`.
func extractAgentName(err error) string {
	msg := err.Error()
	// Format: 'skill: unknown agent: "name"'
	if idx := strings.LastIndex(msg, ": "); idx >= 0 {
		return strings.Trim(msg[idx+2:], `"`)
	}
	return ""
}

// agentNames lists the skill-capable agent names. Load-bearing: it is
// interpolated into the `skill install` / `skill remove` Long text, which is
// rendered into the committed skill bundle, so an MCP-only catalog entry
// leaking in here would both advertise skill support the app lacks and drift
// the bundle.
func agentNames() []string {
	return agentNamesOf(SkillAgents())
}

// MCPAgentNames lists the MCP-capable agent names, for the `mcp` command
// group's help text and --agent validation.
func MCPAgentNames() []string {
	return agentNamesOf(MCPAgents())
}

func agentNamesOf(agents []*Agent) []string {
	names := make([]string, 0, len(agents))
	for _, a := range agents {
		names = append(names, a.Name)
	}
	return names
}

// findSkillAgent resolves an --agent value on the skill surface. A catalog
// entry with no SkillsDir is not a skill target, so it resolves to nil just
// like an unknown name.
func findSkillAgent(name string) *Agent {
	a := FindAgent(name)
	if a == nil || !a.SupportsSkills() {
		return nil
	}
	return a
}

// isAgentName reports whether name matches a skill-capable agent
// (case-insensitive). Used by the hard-break guard so a user typing
// `skill install claude-code` (the old positional shape) gets a clear
// pointer to `--agent claude-code` instead of a generic "unknown skill"
// error.
func isAgentName(name string) bool {
	return findSkillAgent(name) != nil
}

// didYouMeanAgentErr returns the hard-break usage error mandated by
// REQ-F-012 when a `<skill-name>` positional matches a known agent name
// instead of a real skill. The lowercased canonical form is suggested so
// the user can copy-paste the fix.
func didYouMeanAgentErr(name string) error {
	a := FindAgent(name)
	canonical := strings.ToLower(name)
	if a != nil {
		canonical = a.Name
	}
	return clierr.NewUsageError("unknown skill: %s; did you mean '--agent %s'?", name, canonical)
}

// unknownSkillErr is the generic non-agent-collision branch of the
// positional-skill validator. Catalog wiring in later tasks may wrap
// this with a refresh hint; for now it's a plain usage error.
func unknownSkillErr(name string) error {
	return clierr.NewUsageError("unknown skill: %s", name)
}

// mustNotCombineAllAndPositional rejects the simultaneous use of `--all`
// and a `[skill-name]` positional, shared by install + remove so the
// wording stays in one place.
func mustNotCombineAllAndPositional(allFlag bool, skillArg string) error {
	if allFlag && skillArg != "" {
		return clierr.NewUsageError("--all cannot be combined with a [skill-name] positional")
	}
	return nil
}

// readInstalledSkill reads the per-agent install state for `skillName`
// from `filesystem`. Returns whether SKILL.md exists and the parsed
// frontmatter `version:` (empty string when missing or unparseable).
// Shared by installer.List + BuildInventory.
func readInstalledSkill(filesystem afero.Fs, agent *Agent, skillName string) (bool, string) {
	sp, ok := agent.SkillsPath()
	if !ok {
		return false, ""
	}
	skillFile := filepath.Join(sp, skillName, "SKILL.md")
	exists, _ := afero.Exists(filesystem, skillFile)
	if !exists {
		return false, ""
	}
	data, err := afero.ReadFile(filesystem, skillFile)
	if err != nil {
		return true, ""
	}
	return true, sanitizeVersion(parseVersion(data))
}

// agentDetected reports whether `agent`'s DetectDir exists on `filesystem`.
// Shared by installer.List + BuildInventory.
func agentDetected(filesystem afero.Fs, agent *Agent) bool {
	dp, ok := agent.DetectPath()
	if !ok {
		return false
	}
	exists, _ := afero.DirExists(filesystem, dp)
	return exists
}
