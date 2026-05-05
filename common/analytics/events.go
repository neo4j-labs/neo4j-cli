// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package analytics

import (
	"regexp"
	"runtime"
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
}

// CommandEventProperties carries the command-specific fields for COMMAND_USED
// and HELP_USED events. Base properties are merged in at send time by
// sendTrackEvent, so they are not embedded here.
type CommandEventProperties struct {
	Command string `json:"command"`
	Success bool   `json:"success"`
}

// TrackEvent is the envelope sent to Mixpanel for every analytics event.
// Event is set by EmitEvent from the caller-supplied suffix — do not set it directly.
type TrackEvent struct {
	Event      string      `json:"event"`
	Properties interface{} `json:"properties"`
}

// NewStartupEvent, NewCommandEvent, and NewHelpEvent have been removed.
// Use EmitStartupEvent, EmitCommandEvent, and EmitHelpEvent directly,
// or call EmitEvent with the appropriate suffix and a TrackEvent.

// getBaseProperties assembles properties common to all events.
// Called by sendTrackEvent at send time so timestamps and uptime reflect
// when the event was actually dispatched, not when it was constructed.
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
