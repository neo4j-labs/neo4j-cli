// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"errors"
	"io/fs"
	"strings"

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

// resolveSkillSource maps a positional skill-name to the Source the
// installer/printer should copy when no catalog is in play. Empty arg
// defaults to the embedded self-skill; the self-skill resolver handles
// the canonical / binary-alias names; anything else is either a
// did-you-mean-agent hard-break (REQ-F-012) or an unknown-skill error.
// Catalog-aware callers use resolveCatalogSkillSource instead.
func resolveSkillSource(bundle fs.FS, version, binaryName, skillArg string) (Source, error) {
	if skillArg == "" {
		return Source{FS: bundle, Version: version}, nil
	}
	src, err := ResolveSelf(bundle, version, binaryName, skillArg)
	if err == nil {
		return src, nil
	}
	if !errors.Is(err, ErrNotSelfSkill) {
		return Source{}, err
	}
	if isAgentName(skillArg) {
		return Source{}, didYouMeanAgentErr(skillArg)
	}
	return Source{}, unknownSkillErr(skillArg)
}
