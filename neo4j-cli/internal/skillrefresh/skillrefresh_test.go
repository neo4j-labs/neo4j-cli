// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skillrefresh

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	commonskill "github.com/neo4j/cli/common/skill"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withListFn swaps the package-level listFn seam and restores it after t.
func withListFn(t *testing.T, fn func(afero.Fs, string) ([]commonskill.AgentInstall, error)) {
	t.Helper()
	prev := listFn
	listFn = fn
	t.Cleanup(func() { listFn = prev })
}

// withInstallFn swaps the package-level installFn seam and restores it after t.
func withInstallFn(t *testing.T, fn func(afero.Fs, fs.FS, string, string, string) ([]*commonskill.Agent, error)) {
	t.Helper()
	prev := installFn
	installFn = fn
	t.Cleanup(func() { installFn = prev })
}

// newTestCfg returns a fresh in-memory clicfg.Config seeded with the given
// version. skill-auto-refresh defaults to true via viper defaults.
func newTestCfg(t *testing.T, version string) *clicfg.Config {
	t.Helper()
	memFs, err := testfs.GetTestFs(`{"format":"default"}`, "{}")
	require.NoError(t, err)
	return clicfg.NewConfig(memFs, version, clicfg.GlobalScope)
}

// newTestCfgWithAutoRefresh returns a Config with skill-auto-refresh set to
// the given string value (mirrors the on-disk "true"/"false" shape).
func newTestCfgWithAutoRefresh(t *testing.T, version, autoRefresh string) *clicfg.Config {
	t.Helper()
	config := `{"format":"default","skill-auto-refresh":"` + autoRefresh + `"}`
	memFs, err := testfs.GetTestFs(config, "{}")
	require.NoError(t, err)
	return clicfg.NewConfig(memFs, version, clicfg.GlobalScope)
}

// makeCmd returns a minimal cobra command wired with stderr/stdout buffers.
func makeCmd(stderr *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{Use: "neo4j-cli"}
	cmd.SetErr(stderr)
	cmd.SetOut(&bytes.Buffer{})
	return cmd
}

// fakeAgent returns a minimal AgentInstall with the given installed state.
func fakeAgent(name, displayName string, installed bool) commonskill.AgentInstall {
	return commonskill.AgentInstall{
		Agent:     &commonskill.Agent{Name: name, DisplayName: displayName},
		Detected:  true,
		Installed: installed,
	}
}

// waitForCacheVersion polls the state file until it records the expected version
// or the deadline is exceeded. Returns true on success.
func waitForCacheVersion(t *testing.T, fs afero.Fs, want string) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		e := readCache(fs)
		if e != nil && e.LastRefreshedVersion == want {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// TestMaybeRefresh_TableDriven covers the core branches of MaybeRefresh.
func TestMaybeRefresh_TableDriven(t *testing.T) {
	cases := []struct {
		name string
		// setupCfg builds the Config; if nil, defaults to version "v1.1.0" with auto-refresh enabled.
		setupCfg func(t *testing.T) *clicfg.Config
		// preSeedVersion writes a cache entry before calling MaybeRefresh, "" = no seed.
		preSeedVersion string
		// fakeAgents is the slice returned by the listFn seam.
		fakeAgents []commonskill.AgentInstall
		// installErr is returned by installFn for all agents; nil = success.
		installErr error
		// wantRefreshFired: true if a goroutine should trigger install calls.
		wantRefreshFired bool
		// wantCacheVersion is the version expected in the state file after the call.
		// "" means "don't check cache" (e.g. version match no-op).
		wantCacheVersion string
		// wantStderrContains substrings (all must appear) on success path.
		wantStderrContains []string
		// wantStderrAbsent substrings (must NOT appear) — for no-op cases.
		wantStderrAbsent []string
		// wantWarning: true if we expect a "Warning:" line in stderr.
		wantWarning bool
		// customInstallFn, if non-nil, replaces the default uniform-error installFn
		// for cases that need per-call behaviour (e.g. first succeeds, second fails).
		customInstallFn func(afero.Fs, fs.FS, string, string, string) ([]*commonskill.Agent, error)
	}{
		{
			name:             "no state file — refresh triggered",
			preSeedVersion:   "",
			fakeAgents:       []commonskill.AgentInstall{fakeAgent("claude-code", "Claude Code", true)},
			wantRefreshFired: true,
			wantCacheVersion: "v1.1.0",
			wantStderrContains: []string{
				"Refreshed neo4j-cli skill for 1 agent(s)",
				"v1.1.0",
			},
		},
		{
			name:             "version match — no-op",
			preSeedVersion:   "v1.1.0",
			fakeAgents:       []commonskill.AgentInstall{fakeAgent("claude-code", "Claude Code", true)},
			wantRefreshFired: false,
			wantStderrAbsent: []string{"Refreshed"},
		},
		{
			name:             "version mismatch — refresh triggered",
			preSeedVersion:   "v1.0.0",
			fakeAgents:       []commonskill.AgentInstall{fakeAgent("claude-code", "Claude Code", true)},
			wantRefreshFired: true,
			wantCacheVersion: "v1.1.0",
			wantStderrContains: []string{
				"Refreshed neo4j-cli skill for 1 agent(s)",
				"v1.0.0",
				"v1.1.0",
			},
		},
		{
			name: "opt-out via config — no-op",
			setupCfg: func(t *testing.T) *clicfg.Config {
				t.Helper()
				return newTestCfgWithAutoRefresh(t, "v1.1.0", "false")
			},
			fakeAgents:       []commonskill.AgentInstall{fakeAgent("claude-code", "Claude Code", true)},
			wantRefreshFired: false,
			wantStderrAbsent: []string{"Refreshed"},
		},
		{
			name: "dev version — no-op",
			setupCfg: func(t *testing.T) *clicfg.Config {
				t.Helper()
				return newTestCfg(t, "dev")
			},
			fakeAgents:       []commonskill.AgentInstall{fakeAgent("claude-code", "Claude Code", true)},
			wantRefreshFired: false,
			wantStderrAbsent: []string{"Refreshed"},
		},
		{
			name:             "no installed agents — cache updated, no install calls",
			preSeedVersion:   "v1.0.0",
			fakeAgents:       []commonskill.AgentInstall{fakeAgent("claude-code", "Claude Code", false)},
			wantRefreshFired: true,
			wantCacheVersion: "v1.1.0",
			wantStderrAbsent: []string{"Refreshed"},
		},
		{
			name:           "partial failure — warning printed, cache updated, success count reported",
			preSeedVersion: "v1.0.0",
			fakeAgents: []commonskill.AgentInstall{
				fakeAgent("claude-code", "Claude Code", true),
				fakeAgent("cursor", "Cursor", true),
			},
			// First agent succeeds, second fails — one success + one warning.
			customInstallFn: func() func(afero.Fs, fs.FS, string, string, string) ([]*commonskill.Agent, error) {
				call := 0
				return func(_ afero.Fs, _ fs.FS, _, _, _ string) ([]*commonskill.Agent, error) {
					call++
					if call == 1 {
						return []*commonskill.Agent{}, nil
					}
					return nil, errors.New("injected install error")
				}
			}(),
			wantRefreshFired:   true,
			wantCacheVersion:   "v1.1.0",
			wantWarning:        true,
			wantStderrContains: []string{"Refreshed neo4j-cli skill for 1 agent(s)"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build config.
			var cfg *clicfg.Config
			if tc.setupCfg != nil {
				cfg = tc.setupCfg(t)
			} else {
				cfg = newTestCfg(t, "v1.1.0")
			}

			// Pre-seed the cache if requested.
			if tc.preSeedVersion != "" {
				writeCache(cfg.Aura.Fs(), cacheEntry{LastRefreshedVersion: tc.preSeedVersion})
			}

			// Track whether install was called.
			installCalled := false

			// Inject fake listFn.
			withListFn(t, func(_ afero.Fs, _ string) ([]commonskill.AgentInstall, error) {
				return tc.fakeAgents, nil
			})

			// Inject fake installFn — use customInstallFn when provided, otherwise
			// fall back to the uniform-error behaviour driven by tc.installErr.
			if tc.customInstallFn != nil {
				customFn := tc.customInstallFn
				withInstallFn(t, func(aFs afero.Fs, b fs.FS, sn, v, af string) ([]*commonskill.Agent, error) {
					installCalled = true
					return customFn(aFs, b, sn, v, af)
				})
			} else {
				withInstallFn(t, func(_ afero.Fs, _ fs.FS, _, _, _ string) ([]*commonskill.Agent, error) {
					installCalled = true
					if tc.installErr != nil {
						return nil, tc.installErr
					}
					return []*commonskill.Agent{}, nil
				})
			}

			// Build cobra command with captured stderr.
			stderr := &bytes.Buffer{}
			cmd := makeCmd(stderr)

			// Call MaybeRefresh — it returns immediately and runs work in a goroutine.
			MaybeRefresh(context.Background(), cmd, cfg, nil, "neo4j-cli")

			// For no-op cases, give the goroutine a short window to (not) fire.
			if !tc.wantRefreshFired {
				time.Sleep(50 * time.Millisecond)
				assert.False(t, installCalled, "install must not be called in no-op case")
				for _, absent := range tc.wantStderrAbsent {
					assert.NotContains(t, stderr.String(), absent)
				}
				return
			}

			// For refresh cases, wait for the cache version to land.
			if tc.wantCacheVersion != "" {
				ok := waitForCacheVersion(t, cfg.Aura.Fs(), tc.wantCacheVersion)
				require.True(t, ok, "cache was never written with version %s", tc.wantCacheVersion)
			}

			// Allow the goroutine to fully unwind before checking stderr.
			time.Sleep(50 * time.Millisecond)

			for _, want := range tc.wantStderrContains {
				assert.Contains(t, stderr.String(), want)
			}
			for _, absent := range tc.wantStderrAbsent {
				assert.NotContains(t, stderr.String(), absent)
			}
			if tc.wantWarning {
				assert.Contains(t, stderr.String(), "Warning:")
			}
		})
	}
}

// TestMaybeRefresh_ContextCancellation verifies that a goroutine exits cleanly
// when the context is cancelled before any install work runs.
func TestMaybeRefresh_ContextCancellation(t *testing.T) {
	cfg := newTestCfg(t, "v1.1.0")

	blocked := make(chan struct{})
	withListFn(t, func(_ afero.Fs, _ string) ([]commonskill.AgentInstall, error) {
		// Block until the test unblocks (simulates a slow list operation that
		// the test never needs to complete).
		<-blocked
		return []commonskill.AgentInstall{fakeAgent("claude-code", "Claude Code", true)}, nil
	})

	installCalled := false
	withInstallFn(t, func(_ afero.Fs, _ fs.FS, _, _, _ string) ([]*commonskill.Agent, error) {
		installCalled = true
		return []*commonskill.Agent{}, nil
	})

	// Cancel context immediately before calling MaybeRefresh so the goroutine
	// finds ctx.Done() closed on first check.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stderr := &bytes.Buffer{}
	cmd := makeCmd(stderr)
	MaybeRefresh(ctx, cmd, cfg, nil, "neo4j-cli")

	// Unblock the listFn so the goroutine can proceed (it should bail at the
	// ctx.Done() guard before reaching listFn, but unblocking is safe).
	close(blocked)

	time.Sleep(50 * time.Millisecond)
	assert.False(t, installCalled, "install must not be called when ctx is already cancelled")
	assert.Empty(t, stderr.String())
}

// TestMaybeRefresh_NilCfg verifies that a nil config is handled gracefully.
func TestMaybeRefresh_NilCfg(t *testing.T) {
	stderr := &bytes.Buffer{}
	cmd := makeCmd(stderr)
	// Must not panic.
	MaybeRefresh(context.Background(), cmd, nil, nil, "neo4j-cli")
	assert.Empty(t, stderr.String())
}

// TestMaybeRefresh_MultipleAgents verifies that when multiple agents are
// installed, the success count reflects all of them.
func TestMaybeRefresh_MultipleAgents(t *testing.T) {
	cfg := newTestCfg(t, "v1.1.0")

	withListFn(t, func(_ afero.Fs, _ string) ([]commonskill.AgentInstall, error) {
		return []commonskill.AgentInstall{
			fakeAgent("claude-code", "Claude Code", true),
			fakeAgent("cursor", "Cursor", true),
			fakeAgent("windsurf", "Windsurf", false), // not installed — must be skipped
		}, nil
	})

	installCount := 0
	withInstallFn(t, func(_ afero.Fs, _ fs.FS, _, _, _ string) ([]*commonskill.Agent, error) {
		installCount++
		return []*commonskill.Agent{}, nil
	})

	stderr := &bytes.Buffer{}
	cmd := makeCmd(stderr)
	MaybeRefresh(context.Background(), cmd, cfg, nil, "neo4j-cli")

	ok := waitForCacheVersion(t, cfg.Aura.Fs(), "v1.1.0")
	require.True(t, ok, "cache was never written")
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 2, installCount, "only installed agents must be passed to installFn")
	assert.Contains(t, stderr.String(), "Refreshed neo4j-cli skill for 2 agent(s)")
}
