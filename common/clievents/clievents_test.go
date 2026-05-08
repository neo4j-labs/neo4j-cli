// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clievents

import (
	"strings"
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
		Properties: commandEventProperties{
			Command: "aura instances list --output json",
			Success: true,
		},
	})
	Emit(svc, []string{"aura", "instances", "list", "--output", "json"}, true)
}

func TestEmit_AuraCommand_PropagatesFailure(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("AURA", analytics.TrackEvent{
		Properties: commandEventProperties{
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
		Properties: commandEventProperties{
			Command: "skill list",
			Success: true,
		},
	})
	Emit(svc, []string{"skill", "list"}, true)
}

func TestEmit_SkillCommand_PropagatesFailure(t *testing.T) {
	svc := newMockService(t)
	svc.EXPECT().EmitEvent("SKILL", analytics.TrackEvent{
		Properties: commandEventProperties{
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
		Properties: commandEventProperties{
			Command: "unknown sub",
			Success: true,
		},
	})
	Emit(svc, []string{"unknown", "sub"}, true)
}

// ---- secret-flag redaction ------------------------------------------------

// captureCommand wires the mock to capture the command property emitted by
// Emit, returning a pointer to the captured string.
func captureCommand(t *testing.T, expectedSuffix string) (*amocks.MockService, *string) {
	t.Helper()
	svc := newMockService(t)
	captured := new(string)
	svc.EXPECT().
		EmitEvent(expectedSuffix, gomock.Any()).
		Do(func(_ string, ev analytics.TrackEvent) {
			switch p := ev.Properties.(type) {
			case commandEventProperties:
				*captured = p.Command
			case queryEventProperties:
				*captured = p.Command
			default:
				t.Fatalf("unexpected properties type %T", ev.Properties)
			}
		})
	return svc, captured
}

func TestEmit_RedactsSecretFlags(t *testing.T) {
	const secret = "supersecretvalue"
	tests := []struct {
		name     string
		args     []string
		suffix   string
		flagName string // flag name we expect to see in the output (without dashes)
	}{
		{
			name:     "aura credential add --client-secret",
			args:     []string{"aura", "credential", "aura-client", "add", "--client-secret", secret},
			suffix:   "AURA",
			flagName: "client-secret",
		},
		{
			name:     "credential dbms add --password (default branch)",
			args:     []string{"credential", "dbms", "add", "--password", secret},
			suffix:   "COMMAND",
			flagName: "password",
		},
		{
			name:     "credential embed add --api-key (default branch)",
			args:     []string{"credential", "embed", "add", "--api-key", secret},
			suffix:   "COMMAND",
			flagName: "api-key",
		},
		{
			name:     "aura dataapi graphql create --instance-password",
			args:     []string{"aura", "dataapi", "graphql", "create", "--instance-password", secret},
			suffix:   "AURA",
			flagName: "instance-password",
		},
		{
			name:     "skill install with stray --password (defensive)",
			args:     []string{"skill", "install", "--password", secret},
			suffix:   "SKILL",
			flagName: "password",
		},
		{
			name:     "equals form --client-secret=value",
			args:     []string{"aura", "credential", "aura-client", "add", "--client-secret=" + secret},
			suffix:   "AURA",
			flagName: "client-secret",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, captured := captureCommand(t, tc.suffix)
			Emit(svc, tc.args, true)
			if *captured == "" {
				t.Fatalf("no command property captured")
			}
			if !strings.Contains(*captured, "***") {
				t.Errorf("expected redaction placeholder *** in command, got %q", *captured)
			}
			if !strings.Contains(*captured, tc.flagName) {
				t.Errorf("expected flag name %q to remain in command, got %q", tc.flagName, *captured)
			}
			if strings.Contains(*captured, secret) {
				t.Errorf("secret value leaked into command property: %q", *captured)
			}
		})
	}
}
