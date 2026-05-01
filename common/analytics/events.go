package analytics

import (
	"regexp"
	"runtime"
	"strings"
	"time"

	"log/slog"

	"github.com/google/uuid"
)

// baseProperties are the fields attached to every Mixpanel track event.
type baseProperties struct {
	Token      string `json:"token"`
	Time       int64  `json:"time"`
	DistinctID string `json:"distinct_id"`
	InsertID   string `json:"$insert_id"`
	Uptime     int64  `json:"uptime"`
	OS         string `json:"$os"`
	OSArch     string `json:"os_arch"`
	CLIVersion string `json:"cli_version,omitempty"`
	MachineID  string `json:"machine_id,omitempty"`
}

// toolEventProperties carries the fields for a TOOL_USED event.
type toolEventProperties struct {
	baseProperties
	ToolName string `json:"tool_name"`
	Success  bool   `json:"success"`
}

// ActiveFlags captures which agent-relevant persistent flags were set for
// a given invocation. Sensitive flags (credentials, URIs) are intentionally
// excluded.
type ActiveFlags struct {
	AgentMode    bool   `json:"agent_mode"`
	AllowWrites  bool   `json:"allow_writes"`
	OutputFormat string `json:"output_format,omitempty"`
	TimeoutSet   bool   `json:"timeout_set"`
}

// commandEventProperties carries the fields for a COMMAND_USED event.
type commandEventProperties struct {
	baseProperties
	Command string `json:"command"`
	Success bool   `json:"success"`
	ActiveFlags
}

// TrackEvent is the envelope sent to Mixpanel for every analytics event.
type TrackEvent struct {
	Event      string      `json:"event"`
	Properties interface{} `json:"properties"`
}

// NewStartupEvent records that the CLI has started.
func (a *Analytics) NewStartupEvent() TrackEvent {
	return TrackEvent{
		Event:      strings.Join([]string{a.cfg.appName, "STARTUP"}, "_"),
		Properties: a.getBaseProperties(),
	}
}

// NewToolEvent records a tool invocation outcome.
func (a *Analytics) NewToolEvent(toolName string, success bool) TrackEvent {
	return TrackEvent{
		Event: strings.Join([]string{a.cfg.appName, "TOOL_USED"}, "_"),
		Properties: toolEventProperties{
			baseProperties: a.getBaseProperties(),
			ToolName:       toolName,
			Success:        success,
		},
	}
}

// NewCommandEvent records a CLI command invocation with the full command
// path and the agent-relevant flags that were active.
func (a *Analytics) NewCommandEvent(command string, success bool, flags ActiveFlags) TrackEvent {
	return TrackEvent{
		Event: strings.Join([]string{a.cfg.appName, "COMMAND_USED"}, "_"),
		Properties: commandEventProperties{
			baseProperties: a.getBaseProperties(),
			Command:        command,
			Success:        success,
			ActiveFlags:    flags,
		},
	}
}

// getBaseProperties assembles properties common to all events.
func (a *Analytics) getBaseProperties() baseProperties {
	uptime := time.Now().Unix() - a.cfg.startupTime
	return baseProperties{
		Token:      a.cfg.token,
		DistinctID: a.cfg.distinctID,
		Time:       time.Now().UnixMilli(),
		InsertID:   a.newInsertID(),
		Uptime:     uptime,
		OS:         runtime.GOOS,
		OSArch:     runtime.GOARCH,
		CLIVersion: a.cfg.cliVersion,
		MachineID:  a.cfg.machineID,
	}
}

func (a *Analytics) newInsertID() string {
	id, err := uuid.NewV6()
	if err != nil {
		slog.Error("error generating insert ID for analytics", "error", err.Error())
		return ""
	}
	return id.String()
}

// auraURIPattern matches the host patterns used by Neo4j Aura:
// databases.neo4j.io (classic) and instances.neo4j.io (multi-DB).
var auraURIPattern = regexp.MustCompile(`(databases|instances)\.neo4j\.io\b`)

// IsAuraURI reports whether uri points at a Neo4j Aura-managed instance.
// Exported so that tests and other packages can use it without duplicating
// the pattern.
func IsAuraURI(uri string) bool {
	return auraURIPattern.MatchString(uri)
}
