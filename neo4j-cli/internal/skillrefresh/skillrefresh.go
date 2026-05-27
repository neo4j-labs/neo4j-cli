// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package skillrefresh implements the silent background "refresh installed
// agent skills when the binary version changes" probe. When the running
// binary version differs from the version recorded in the state file
// (skill-refresh.json), a goroutine reinstalls the skill bundle for every
// agent that currently has the skill installed, then updates the state file.
//
// MaybeRefresh is the only public entry point. It returns immediately and
// does all work in the background goroutine. All errors are either silently
// swallowed or emitted as single-line warnings to stderr — the foreground
// command is never affected.
package skillrefresh

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/neo4j/cli/common/clicfg"
	commonskill "github.com/neo4j/cli/common/skill"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

// Test seams — package-level vars so tests can inject fakes without
// rewriting the real commonskill calls. Production keeps the real impls.
var (
	listFn = func(filesystem afero.Fs, skillName string) ([]commonskill.AgentInstall, error) {
		return commonskill.List(filesystem, skillName)
	}
	installFn = func(filesystem afero.Fs, bundle fs.FS, skillName, version, agentFilter string) ([]*commonskill.Agent, error) {
		return commonskill.Install(filesystem, commonskill.Source{FS: bundle, Version: version}, skillName, agentFilter)
	}
)

// devVersion is the sentinel used when no ldflags tag has been baked in.
// Refresh is skipped for dev builds — there is nothing meaningful to compare.
const devVersion = "dev"

// autoRefreshEnabled reports whether skill-auto-refresh is enabled. The value
// from viper can be a bool (default) or a string ("true"/"false") when the
// user has persisted it via `config set`. Both shapes are handled.
func autoRefreshEnabled(cfg *clicfg.Config) bool {
	if cfg == nil || cfg.Global == nil {
		return false
	}
	v := cfg.Global.Get("skill-auto-refresh")
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val == "true"
	default:
		// Unknown type — treat as enabled (the default is true).
		return true
	}
}

// MaybeRefresh compares the last-refreshed version in the state file to the
// running binary version. When they differ and skill-auto-refresh is enabled,
// it spawns a background goroutine that reinstalls the skill bundle for every
// agent that currently has the skill installed and updates the state file.
//
// The function returns immediately — all I/O and install work happens in the
// goroutine. ctx is forwarded so the goroutine can honour early cancellation.
// bundle is the embedded FS containing the skill bundle to install. skillName
// is the binary-specific skill name (e.g. "neo4j-cli").
func MaybeRefresh(ctx context.Context, cmd *cobra.Command, cfg *clicfg.Config, bundle fs.FS, skillName string) {
	if cfg == nil || cfg.Aura == nil {
		return
	}
	if !autoRefreshEnabled(cfg) {
		return
	}

	current := cfg.Version
	if current == "" || current == devVersion {
		return
	}

	auraFs := cfg.Aura.Fs()
	cached := readCache(auraFs)
	if cached != nil && cached.LastRefreshedVersion == current {
		return
	}

	oldVersion := ""
	if cached != nil {
		oldVersion = cached.LastRefreshedVersion
	}

	go func() {
		defer func() { _ = recover() }()

		// Honour context cancellation — if the parent command exits before we
		// start, skip the refresh entirely.
		select {
		case <-ctx.Done():
			return
		default:
		}

		rows, err := listFn(auraFs, skillName)
		if err != nil {
			return
		}

		// Only re-install agents that currently have the skill installed.
		installed := make([]commonskill.AgentInstall, 0, len(rows))
		for _, r := range rows {
			if r.Installed {
				installed = append(installed, r)
			}
		}
		if len(installed) == 0 {
			// Nothing to refresh; still update the state file so we don't
			// re-check on every subsequent invocation.
			writeCache(auraFs, cacheEntry{LastRefreshedVersion: current})
			return
		}

		// Install per agent; collect per-agent errors as warnings.
		successCount := 0
		for _, ai := range installed {
			select {
			case <-ctx.Done():
				// Cancelled mid-loop — write what we have and exit.
				if successCount > 0 {
					writeCache(auraFs, cacheEntry{LastRefreshedVersion: current})
				}
				return
			default:
			}

			_, installErr := installFn(auraFs, bundle, skillName, current, ai.Agent.Name)
			if installErr != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
					"Warning: neo4j-cli skill auto-refresh failed for %s: %v\n",
					ai.Agent.DisplayName, installErr)
				continue
			}
			successCount++
		}

		// Update the state file regardless of partial failures — on the next
		// invocation the already-refreshed agents will have the correct version
		// and only genuinely broken ones would re-trigger.
		writeCache(auraFs, cacheEntry{LastRefreshedVersion: current})

		if successCount > 0 {
			old := oldVersion
			if old == "" {
				old = devVersion
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
				"Refreshed neo4j-cli skill for %d agent(s) (%s → %s)\n",
				successCount, old, current)
		}
	}()
}
