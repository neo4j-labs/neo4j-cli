// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skillrefresh

import (
	"context"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/spf13/cobra"
)

// MaybeRefresh compares the last-refreshed version stored in the state file to
// the current binary version and, when they differ and skill-auto-refresh is
// enabled, spawns a background goroutine that reinstalls the skill bundles for
// every agent that has the skill installed.
//
// The function returns immediately; all I/O happens in the goroutine.
// ctx is forwarded so the goroutine can honour early cancellation.
func MaybeRefresh(_ context.Context, _ *cobra.Command, cfg *clicfg.Config, _ []byte, _ string) {
	if cfg == nil || cfg.Aura == nil {
		return
	}
	// TODO(task-003): implement full version-comparison and goroutine launch.
	// The cache helpers (readCache / writeCache / cachePath) are defined in cache.go.
	fs := cfg.Aura.Fs()
	_ = readCache(fs)
	// writeCache is called by the goroutine after install attempts complete.
	_ = writeCache
}
