// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package configmigrate carries users forward across `config.json` schema
// changes. The registry below is intentionally empty on first ship — the
// engine in configmigrate.go is the deliverable. A real migration lands the
// first time a config-shape change actually needs one.
package configmigrate

import "fmt"

// Migration is one forward-only transform of the raw config.json bytes.
// Version is a monotonic integer (1-indexed); Description is a short human
// label used in error/warning messages; Apply receives the current file
// bytes and returns the transformed bytes (or an error to abort the run).
type Migration struct {
	Version     int
	Description string
	Apply       func([]byte) ([]byte, error)
}

// migrations is the package-level registry. Ships empty — first concrete
// entry lands when a config-shape change actually needs one (e.g. removing
// a graduated feature-flag key). Each new entry must be appended (never
// inserted) so the contiguous-ascending invariant enforced by init() holds.
var migrations = []Migration{}

func init() {
	for i, m := range migrations {
		want := i + 1
		if m.Version != want {
			panic(fmt.Sprintf(
				"configmigrate: migrations slice must be contiguous and ascending starting at 1; index %d has Version=%d, want %d",
				i, m.Version, want,
			))
		}
	}
}
