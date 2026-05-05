// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clievents

import (
	"testing"

	"github.com/neo4j/cli/common/analytics"
	amocks "github.com/neo4j/cli/common/analytics/mocks"
	"go.uber.org/mock/gomock"
)

func newMockService(t *testing.T) *amocks.MockService {
	t.Helper()
	ctrl := gomock.NewController(t)
	return amocks.NewMockService(ctrl)
}

// ---- HELP events ----------------------------------------------------------

func TestEmit_NoArgs_EmitsHelp(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("HELP", analytics.TrackEvent{
		Properties: helpEventProperties{},
	})
	Emit(svc, []string{}, false)
}

func TestEmit_TopLevelHelpFlag_EmitsHelp(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("HELP", analytics.TrackEvent{
		Properties: helpEventProperties{},
	})
	Emit(svc, []string{"--help"}, false)
}

func TestEmit_ShortHelpFlag_EmitsHelp(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("HELP", analytics.TrackEvent{
		Properties: helpEventProperties{},
	})
	Emit(svc, []string{"-h"}, false)
}

func TestEmit_CommandWithHelpFlag_EmitsHelpWithCommandName(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("HELP", analytics.TrackEvent{
		Properties: helpEventProperties{Command: "aura"},
	})
	Emit(svc, []string{"aura", "instances", "list", "--help"}, false)
}

func TestEmit_CommandWithShortHelpFlag_EmitsHelpWithCommandName(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("HELP", analytics.TrackEvent{
		Properties: helpEventProperties{Command: "query"},
	})
	Emit(svc, []string{"query", "-h"}, false)
}

// ---- AURA events ----------------------------------------------------------

func TestEmit_AuraCommand_EmitsFullCommand(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("AURA", analytics.TrackEvent{
		Properties: analytics.CommandEventProperties{
			Command: "aura instances list --output json",
			Success: true,
		},
	})
	Emit(svc, []string{"aura", "instances", "list", "--output", "json"}, true)
}

func TestEmit_AuraCommand_PropagatesFailure(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("AURA", analytics.TrackEvent{
		Properties: analytics.CommandEventProperties{
			Command: "aura instances list",
			Success: false,
		},
	})
	Emit(svc, []string{"aura", "instances", "list"}, false)
}

// ---- QUERY events ---------------------------------------------------------

func TestEmit_QueryCommand_EmitsCommandNameOnly(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("QUERY", analytics.TrackEvent{
		Properties: queryEventProperties{
			Command: "query",
			Success: true,
			IsAura:  false,
		},
	})
	// Full args include a query string that could contain PII —
	// verify only the command name appears in the emitted event.
	Emit(svc, []string{"query", "--uri", "bolt://localhost:7687", "MATCH (n) RETURN n"}, true)
}

func TestEmit_QueryCommand_DetectsAuraURI(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("QUERY", analytics.TrackEvent{
		Properties: queryEventProperties{
			Command: "query",
			Success: true,
			IsAura:  true,
		},
	})
	Emit(svc, []string{"query", "--uri", "bolt+s://abc123.databases.neo4j.io"}, true)
}

func TestEmit_QueryCommand_NoURI_IsAuraFalse(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("QUERY", analytics.TrackEvent{
		Properties: queryEventProperties{
			Command: "query",
			Success: true,
			IsAura:  false,
		},
	})
	Emit(svc, []string{"query"}, true)
}

// ---- SKILL events ---------------------------------------------------------

func TestEmit_SkillCommand_EmitsFullCommand(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("SKILL", analytics.TrackEvent{
		Properties: analytics.CommandEventProperties{
			Command: "skill list",
			Success: true,
		},
	})
	Emit(svc, []string{"skill", "list"}, true)
}

func TestEmit_SkillCommand_PropagatesFailure(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("SKILL", analytics.TrackEvent{
		Properties: analytics.CommandEventProperties{
			Command: "skill install my-skill",
			Success: false,
		},
	})
	Emit(svc, []string{"skill", "install", "my-skill"}, false)
}

// ---- default (unknown command) --------------------------------------------

func TestEmit_UnknownCommand_EmitsCommandUsed(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("COMMAND", analytics.TrackEvent{
		Properties: analytics.CommandEventProperties{
			Command: "unknown sub",
			Success: true,
		},
	})
	Emit(svc, []string{"unknown", "sub"}, true)
}
