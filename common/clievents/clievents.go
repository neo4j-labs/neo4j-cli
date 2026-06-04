// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package clievents

import (
	"strings"

	"github.com/neo4j/cli/common/agent"
	"github.com/neo4j/cli/common/analytics"
	"github.com/spf13/pflag"
)

// invokerFn is the overridable seam for caller classification (agent/script/interactive) so tests can
// drive it deterministically without mutating real process state.
var invokerFn = agent.Invoker

// commandEventProperties carries the command-specific fields
type commandEventProperties struct {
	Command string `json:"command"`
	Success bool   `json:"success"`
	Invoker string `json:"invoker"`
}

// helpEventProperties carries properties for HELP events.
// Command is omitted when help is invoked with no command (e.g. bare --help).
type helpEventProperties struct {
	Command string `json:"command,omitempty"`
	Invoker string `json:"invoker"`
}

// queryEventProperties carries properties for QUERY events.
// Only the command name is recorded — the full command string is excluded to
// avoid capturing query content or --password values that may contain PII.
type queryEventProperties struct {
	Command string `json:"command"`
	Success bool   `json:"success"`
	IsAura  bool   `json:"is_aura"`
	Invoker string `json:"invoker"`
}

type startupEventProperties struct {
	Command string `json:"command,omitempty"`
	Invoker string `json:"invoker"`
}

// Emit inspects args to determine which analytics event to fire.
// args is expected to be os.Args[1:] so args[0] is the top-level command name.
func Emit(events analytics.Service, args []string, state bool) {
	flags := pflag.NewFlagSet("cliEvents", pflag.ContinueOnError)
	flags.ParseErrorsAllowlist = pflag.ParseErrorsAllowlist{UnknownFlags: true}
	var help bool
	var uri string

	flags.BoolVarP(&help, "help", "h", false, "")
	flags.StringVar(&uri, "uri", "", "")

	_ = flags.Parse(args)

	// inv classifies the caller as agent, script, or interactive — recorded on every event.
	inv := invokerFn()

	// No command name present — bare invocation or top-level --help.
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		events.EmitEvent("HELP", analytics.TrackEvent{
			Properties: helpEventProperties{Invoker: inv},
		})
		return
	}

	commandName := args[0]

	// --help with a known command — record the command name but not the flags.
	if help {
		events.EmitEvent("HELP", analytics.TrackEvent{
			Properties: helpEventProperties{Command: commandName, Invoker: inv},
		})
		return
	}

	switch commandName {
	case "startup":
		events.EmitEvent("STARTUP", analytics.TrackEvent{
			Properties: startupEventProperties{Command: "startup", Invoker: inv},
		})
	case "aura":
		// Aura commands generally contain no PII, but defensively redact
		// secret-bearing flag values (e.g. --client-secret on credential add).
		cliCommand := RedactArgs(args)
		events.EmitEvent("AURA", analytics.TrackEvent{
			Properties: commandEventProperties{Command: cliCommand, Success: state, Invoker: inv},
		})

	case "query":
		// Exclude the full command string — query content and flags like
		// --password may contain PII. Record only the command name.
		events.EmitEvent("QUERY", analytics.TrackEvent{
			Properties: queryEventProperties{
				Command: commandName,
				Success: state,
				IsAura:  analytics.IsAuraURI(uri),
				Invoker: inv,
			},
		})

	case "skill":
		// Skill commands contain no PII, but funnel through RedactArgs so the
		// secret-flag list stays a single source of truth.
		cliCommand := RedactArgs(args)
		events.EmitEvent("SKILL", analytics.TrackEvent{
			Properties: commandEventProperties{Command: cliCommand, Success: state, Invoker: inv},
		})

	default:
		cliCommand := RedactArgs(args)
		events.EmitEvent("COMMAND", analytics.TrackEvent{
			Properties: commandEventProperties{Command: cliCommand, Success: state, Invoker: inv},
		})
	}
}
