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
func formatAgentErr(err error) error {
	switch {
	case errors.Is(err, ErrUnknownAgent):
		return clierr.NewUsageError("%v\nvalid agents: %s", err, strings.Join(agentNames(), ", "))
	case errors.Is(err, ErrAgentNotDetected):
		return clierr.NewUsageError("%v", err)
	case errors.Is(err, ErrNoAgentsDetected):
		return clierr.NewUsageError("%v\nvalid agents: %s", err, strings.Join(agentNames(), ", "))
	default:
		return err
	}
}

func agentNames() []string {
	names := make([]string, 0, len(AGENTS))
	for i := range AGENTS {
		names = append(names, AGENTS[i].Name)
	}
	return names
}

// isAgentName reports whether name matches a known agent in AGENTS
// (case-insensitive). Used by the hard-break guard so a user typing
// `skill install claude-code` (the old positional shape) gets a clear
// pointer to `--agent claude-code` instead of a generic "unknown skill"
// error.
func isAgentName(name string) bool {
	return FindAgent(name) != nil
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
	return true, parseVersion(data)
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
