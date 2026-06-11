// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package configmigrate carries users forward across `config.json` schema
// changes. The registry below is intentionally empty on first ship — the
// engine in configmigrate.go is the deliverable. A real migration lands the
// first time a config-shape change actually needs one.
package configmigrate

import (
	"fmt"

	"github.com/tidwall/sjson"
)

// Migration is one forward-only transform of the raw config.json bytes.
// Version is a monotonic integer (1-indexed); Description is a short human
// label used in error/warning messages; Apply receives the current file
// bytes and returns the transformed bytes (or an error to abort the run).
type Migration struct {
	Version     int
	Description string
	Apply       func([]byte) ([]byte, error)
}

// migrations is the package-level registry. Each new entry must be appended
// (never inserted) so the contiguous-ascending invariant enforced by init()
// holds.
var migrations = []Migration{
	{
		Version:     1,
		Description: "remove retired flag.aura-beta and aura.beta-enabled keys",
		Apply: func(data []byte) ([]byte, error) {
			// flag\.aura-beta: backslash-escapes the dot so sjson treats it as a
			// literal key named "flag.aura-beta", not a nested path.
			data, err := sjson.DeleteBytes(data, `flag\.aura-beta`)
			if err != nil {
				return nil, err
			}
			// aura.beta-enabled: dot IS the path separator here — matches
			// {"aura": {"beta-enabled": ...}} in the JSON.
			return sjson.DeleteBytes(data, "aura.beta-enabled")
		},
	},
}

func init() {
	if err := validateMigrations(migrations); err != nil {
		panic(err)
	}
}

// validateMigrations enforces that the slice is contiguous and ascending
// starting at Version=1 (so index i holds Version=i+1). Returns a descriptive
// error on any gap, duplicate, or non-1 start; nil for an empty slice or a
// valid one. Extracted from init() so tests can drive it directly without
// process-level panics.
func validateMigrations(ms []Migration) error {
	for i, m := range ms {
		want := i + 1
		if m.Version != want {
			return fmt.Errorf(
				"configmigrate: migrations slice must be contiguous and ascending starting at 1; index %d has Version=%d, want %d",
				i, m.Version, want,
			)
		}
	}
	return nil
}
