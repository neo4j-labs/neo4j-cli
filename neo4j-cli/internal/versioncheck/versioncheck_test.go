// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package versioncheck

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/update"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withRandFloat swaps the package-level dice-roll seam.
func withRandFloat(t *testing.T, v float64) {
	t.Helper()
	prev := randFloat
	randFloat = func() float64 { return v }
	t.Cleanup(func() { randFloat = prev })
}

// withNow swaps the time.Now seam.
func withNow(t *testing.T, ts time.Time) {
	t.Helper()
	prev := nowFn
	nowFn = func() time.Time { return ts }
	t.Cleanup(func() { nowFn = prev })
}

// withLatest swaps the upstream release lookup so tests can assert the
// goroutine path without standing up an httptest server.
func withLatest(t *testing.T, fn func(ctx context.Context, preReleases bool) (*update.Release, error)) {
	t.Helper()
	prev := latestFn
	latestFn = fn
	t.Cleanup(func() { latestFn = prev })
}

// resetOnce wipes the package-level once so a single test can call Schedule
// multiple times without the second call being suppressed by the
// already-fired sync.Once.
func resetOnce(t *testing.T) {
	t.Helper()
	once = sync.Once{}
	t.Cleanup(func() { once = sync.Once{} })
}

// newTestCfg returns a fresh in-memory clicfg.Config seeded with the given
// format value. The afero.MemMapFs is reachable via cfg.Aura.Fs().
func newTestCfg(t *testing.T, format string) *clicfg.Config {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"`+format+`"}`, "{}")
	require.NoError(t, err)
	return clicfg.NewConfig(fs, "v0.1.0", clicfg.GlobalScope)
}

// TestSchedule_DiceRollSampling table-drives the 5% sampler boundary. The
// implementation uses `randFloat() < SampleProbability` so values strictly
// below the threshold hit, values equal-or-above miss.
func TestSchedule_DiceRollSampling(t *testing.T) {
	cases := []struct {
		name      string
		randVal   float64
		shouldHit bool
	}{
		{"0.0 → hit (lowest possible)", 0.0, true},
		{"just below threshold → hit", SampleProbability - 0.0001, true},
		{"exactly at threshold → miss", SampleProbability, false},
		{"just above threshold → miss", SampleProbability + 0.0001, false},
		{"0.5 → miss", 0.5, false},
		{"0.99 → miss (highest realistic)", 0.99, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetOnce(t)
			t.Setenv(EnvDisable, "")
			withRandFloat(t, tc.randVal)
			withNow(t, time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC))

			fired := make(chan struct{}, 1)
			withLatest(t, func(ctx context.Context, preReleases bool) (*update.Release, error) {
				select {
				case fired <- struct{}{}:
				default:
				}
				return &update.Release{TagName: "v0.2.0"}, nil
			})

			cfg := newTestCfg(t, "default")
			Schedule(context.Background(), cfg, "v0.1.0")

			if tc.shouldHit {
				select {
				case <-fired:
				case <-time.After(2 * time.Second):
					t.Fatal("dice-roll hit but goroutine did not fire latestFn")
				}
			} else {
				select {
				case <-fired:
					t.Fatal("dice-roll miss but goroutine fired latestFn")
				case <-time.After(100 * time.Millisecond):
					// expected — no fire
				}
			}
		})
	}
}

func TestSchedule_DiceRollHitsAndWritesCache(t *testing.T) {
	resetOnce(t)
	t.Setenv(EnvDisable, "")
	withRandFloat(t, 0.0) // 0.0 < 0.05 → hit
	withNow(t, time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC))

	done := make(chan struct{})
	withLatest(t, func(ctx context.Context, preReleases bool) (*update.Release, error) {
		assert.False(t, preReleases, "stable-only path expected")
		defer close(done)
		return &update.Release{TagName: "v0.2.0"}, nil
	})

	cfg := newTestCfg(t, "default")
	Schedule(context.Background(), cfg, "v0.1.0")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Schedule goroutine did not call latestFn within 2s")
	}
	// Goroutine writes asynchronously; allow the writeCache to land.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if e := readCache(cfg.Aura.Fs()); e != nil {
			assert.Equal(t, "v0.2.0", e.LatestStable)
			assert.Equal(t, time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC), e.CheckedAt)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("cache file was never written")
}

func TestSchedule_FreshCacheSkipsNetwork(t *testing.T) {
	resetOnce(t)
	t.Setenv(EnvDisable, "")
	withRandFloat(t, 0.0) // dice roll hits
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	withNow(t, now)

	cfg := newTestCfg(t, "default")
	// Pre-seed a fresh cache (1h old, well under the 24h TTL).
	writeCache(cfg.Aura.Fs(), cacheEntry{
		CheckedAt:    now.Add(-1 * time.Hour),
		LatestStable: "v0.2.0",
	})

	called := false
	withLatest(t, func(ctx context.Context, preReleases bool) (*update.Release, error) {
		called = true
		return &update.Release{TagName: "v0.2.0"}, nil
	})

	Schedule(context.Background(), cfg, "v0.1.0")
	// Give any (unwanted) goroutine a moment to fire.
	time.Sleep(100 * time.Millisecond)
	assert.False(t, called, "fresh cache must skip the network call")
}

func TestSchedule_StaleCacheTriggersNetwork(t *testing.T) {
	resetOnce(t)
	t.Setenv(EnvDisable, "")
	withRandFloat(t, 0.0)
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	withNow(t, now)

	cfg := newTestCfg(t, "default")
	// Pre-seed a stale cache (25h old, just past the 24h TTL).
	writeCache(cfg.Aura.Fs(), cacheEntry{
		CheckedAt:    now.Add(-25 * time.Hour),
		LatestStable: "v0.0.5",
	})

	done := make(chan struct{})
	withLatest(t, func(ctx context.Context, preReleases bool) (*update.Release, error) {
		defer close(done)
		return &update.Release{TagName: "v0.2.0"}, nil
	})

	Schedule(context.Background(), cfg, "v0.1.0")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stale cache did not trigger network call within 2s")
	}
}

func TestSchedule_DevVersionSkips(t *testing.T) {
	resetOnce(t)
	t.Setenv(EnvDisable, "")
	withRandFloat(t, 0.0)

	called := false
	withLatest(t, func(ctx context.Context, preReleases bool) (*update.Release, error) {
		called = true
		return &update.Release{TagName: "v0.2.0"}, nil
	})

	cfg := newTestCfg(t, "default")
	Schedule(context.Background(), cfg, "dev")
	time.Sleep(50 * time.Millisecond)
	assert.False(t, called, "dev build must not fire the check")
}

func TestSchedule_EnvDisableSkipsAll(t *testing.T) {
	resetOnce(t)
	t.Setenv(EnvDisable, "1")
	withRandFloat(t, 0.0)

	called := false
	withLatest(t, func(ctx context.Context, preReleases bool) (*update.Release, error) {
		called = true
		return &update.Release{TagName: "v0.2.0"}, nil
	})

	cfg := newTestCfg(t, "default")
	Schedule(context.Background(), cfg, "v0.1.0")
	time.Sleep(50 * time.Millisecond)
	assert.False(t, called, "EnvDisable must short-circuit the dice roll")
}

func TestSchedule_LatestErrorIsSilent(t *testing.T) {
	resetOnce(t)
	t.Setenv(EnvDisable, "")
	withRandFloat(t, 0.0)
	withNow(t, time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC))

	var fired atomic.Bool
	withLatest(t, func(ctx context.Context, preReleases bool) (*update.Release, error) {
		fired.Store(true)
		return nil, assert.AnError
	})

	cfg := newTestCfg(t, "default")
	Schedule(context.Background(), cfg, "v0.1.0")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fired.Load() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.True(t, fired.Load(), "latestFn was never called")
	// Allow the goroutine to fully unwind (writeCache MUST NOT be called).
	time.Sleep(100 * time.Millisecond)
	assert.Nil(t, readCache(cfg.Aura.Fs()), "error path must NOT write the cache")
}

// makeRoot constructs a minimal cobra root for MaybeHint tests. It mounts
// the format flag (so cfg.Global.Format() works), an `update` subcommand to
// exercise the suppress-on-update branch, and a noop `query` subcommand for
// the happy-path control case.
func makeRoot(_ *testing.T, cfg *clicfg.Config) *cobra.Command {
	root := &cobra.Command{Use: "neo4j-cli"}
	root.Version = "v0.1.0"
	// Replicate app.go wiring just enough for cmd.Flags()/Root() lookups.
	root.PersistentFlags().String("format", "", "format")
	cfg.Global.BindFormat(root.PersistentFlags().Lookup("format"))

	root.AddCommand(&cobra.Command{Use: "update", RunE: func(cmd *cobra.Command, args []string) error { return nil }})
	root.AddCommand(&cobra.Command{Use: "query", RunE: func(cmd *cobra.Command, args []string) error { return nil }})
	return root
}

// runMaybeHint resolves the named subcommand via cmd.Find and dispatches
// MaybeHint against the matched leaf.
func runMaybeHint(t *testing.T, cfg *clicfg.Config, current, subName string) string {
	t.Helper()
	root := makeRoot(t, cfg)
	stderr := &bytes.Buffer{}
	root.SetErr(stderr)
	root.SetOut(&bytes.Buffer{})

	var leaf *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == subName {
			leaf = c
		}
	}
	require.NotNil(t, leaf, "test setup: subcommand %q not registered", subName)
	leaf.SetErr(stderr)
	leaf.SetOut(&bytes.Buffer{})

	MaybeHint(leaf, cfg, current)
	return stderr.String()
}

func seedCache(t *testing.T, cfg *clicfg.Config, latest string) {
	t.Helper()
	writeCache(cfg.Aura.Fs(), cacheEntry{
		CheckedAt:    time.Now(),
		LatestStable: latest,
	})
}

func TestMaybeHint_NewerCachedPrintsToStderr(t *testing.T) {
	t.Setenv(EnvDisable, "")
	cfg := newTestCfg(t, "default")
	seedCache(t, cfg, "v0.2.0")

	got := runMaybeHint(t, cfg, "v0.1.0", "query")
	assert.Contains(t, got, "A newer neo4j-cli is available")
	assert.Contains(t, got, "v0.2.0")
	assert.Contains(t, got, "v0.1.0")
	assert.Contains(t, got, "neo4j-cli update")
}

// TestMaybeHint_StdoutNeverWritten asserts the regression guard from
// task-014: MaybeHint MUST write only to stderr. A future refactor swapping
// to cmd.OutOrStdout would silently break scripts that pipe stdout, so we
// pin the behaviour explicitly.
func TestMaybeHint_StdoutNeverWritten(t *testing.T) {
	t.Setenv(EnvDisable, "")
	cfg := newTestCfg(t, "default")
	seedCache(t, cfg, "v0.2.0")

	root := makeRoot(t, cfg)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)

	var leaf *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "query" {
			leaf = c
		}
	}
	require.NotNil(t, leaf)
	leaf.SetOut(stdout)
	leaf.SetErr(stderr)

	MaybeHint(leaf, cfg, "v0.1.0")
	assert.Empty(t, stdout.String(), "MaybeHint must NEVER write to stdout — scripts piping stdout must stay clean")
	assert.NotEmpty(t, stderr.String(), "test setup sanity: nag should fire on stderr")
}

func TestMaybeHint_UpToDateNoOp(t *testing.T) {
	t.Setenv(EnvDisable, "")
	cfg := newTestCfg(t, "default")
	seedCache(t, cfg, "v0.1.0") // same as current

	got := runMaybeHint(t, cfg, "v0.1.0", "query")
	assert.Empty(t, got)
}

func TestMaybeHint_OlderCachedNoOp(t *testing.T) {
	t.Setenv(EnvDisable, "")
	cfg := newTestCfg(t, "default")
	seedCache(t, cfg, "v0.0.1") // older than current

	got := runMaybeHint(t, cfg, "v0.1.0", "query")
	assert.Empty(t, got)
}

func TestMaybeHint_SuppressedOnUpdateCmd(t *testing.T) {
	t.Setenv(EnvDisable, "")
	cfg := newTestCfg(t, "default")
	seedCache(t, cfg, "v0.2.0")

	got := runMaybeHint(t, cfg, "v0.1.0", "update")
	assert.Empty(t, got, "MaybeHint must suppress on `update`")
}

func TestMaybeHint_SuppressedOnFormatJSON(t *testing.T) {
	t.Setenv(EnvDisable, "")
	cfg := newTestCfg(t, "json")
	seedCache(t, cfg, "v0.2.0")

	got := runMaybeHint(t, cfg, "v0.1.0", "query")
	assert.Empty(t, got, "MaybeHint must suppress when format is json")
}

func TestMaybeHint_SuppressedByEnvDisable(t *testing.T) {
	t.Setenv(EnvDisable, "1")
	cfg := newTestCfg(t, "default")
	seedCache(t, cfg, "v0.2.0")

	got := runMaybeHint(t, cfg, "v0.1.0", "query")
	assert.Empty(t, got)
}

func TestMaybeHint_DevVersionNoOp(t *testing.T) {
	t.Setenv(EnvDisable, "")
	cfg := newTestCfg(t, "default")
	seedCache(t, cfg, "v0.2.0")

	got := runMaybeHint(t, cfg, "dev", "query")
	assert.Empty(t, got)
}

func TestMaybeHint_NoCacheNoOp(t *testing.T) {
	t.Setenv(EnvDisable, "")
	cfg := newTestCfg(t, "default")
	// no seedCache → no cache file

	got := runMaybeHint(t, cfg, "v0.1.0", "query")
	assert.Empty(t, got)
}

func TestMaybeHint_CorruptCacheNoOp(t *testing.T) {
	t.Setenv(EnvDisable, "")
	cfg := newTestCfg(t, "default")
	// Write garbage to the cache path.
	cachePathStr := filepath.Join(clicfg.ConfigPrefix, "neo4j", "cli", cacheFileName)
	require.NoError(t, cfg.Aura.Fs().MkdirAll(filepath.Dir(cachePathStr), 0o700))
	f, err := cfg.Aura.Fs().Create(cachePathStr)
	require.NoError(t, err)
	_, err = f.WriteString("{not-json")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got := runMaybeHint(t, cfg, "v0.1.0", "query")
	assert.Empty(t, got, "corrupt cache must be silent")
}

func TestMaybeHint_VersionFlagSuppresses(t *testing.T) {
	t.Setenv(EnvDisable, "")
	cfg := newTestCfg(t, "default")
	seedCache(t, cfg, "v0.2.0")

	root := makeRoot(t, cfg)
	stderr := &bytes.Buffer{}
	root.SetErr(stderr)
	root.SetOut(&bytes.Buffer{})

	// Cobra registers --version on the root via its `Version` field. Mark it
	// changed and dispatch MaybeHint at the root (the level that resolves
	// when `neo4j-cli --version` is run).
	root.InitDefaultVersionFlag()
	require.NoError(t, root.Flags().Set("version", "true"))

	MaybeHint(root, cfg, "v0.1.0")
	assert.Empty(t, stderr.String(), "MaybeHint must suppress when --version is set")
}

func TestMaybeHint_HelpFlagSuppresses(t *testing.T) {
	t.Setenv(EnvDisable, "")
	cfg := newTestCfg(t, "default")
	seedCache(t, cfg, "v0.2.0")

	root := makeRoot(t, cfg)
	stderr := &bytes.Buffer{}
	root.SetErr(stderr)
	root.SetOut(&bytes.Buffer{})

	var leaf *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "query" {
			leaf = c
		}
	}
	require.NotNil(t, leaf)

	// Find the InheritedFlags help flag — cobra adds it lazily on first
	// access. Initialize via ParseFlags to register, then mark it changed.
	leaf.InitDefaultHelpFlag()
	require.NoError(t, leaf.Flags().Set("help", "true"))

	leaf.SetErr(stderr)
	leaf.SetOut(&bytes.Buffer{})
	MaybeHint(leaf, cfg, "v0.1.0")
	assert.Empty(t, stderr.String(), "MaybeHint must suppress when --help is set")
}

func TestCacheEntry_Fresh(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		checkedAt time.Time
		want      bool
	}{
		{"23h old → fresh", now.Add(-23 * time.Hour), true},
		{"24h+1s old → stale", now.Add(-(24*time.Hour + time.Second)), false},
		{"future → fresh (clock skew)", now.Add(1 * time.Hour), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &cacheEntry{CheckedAt: tc.checkedAt, LatestStable: "v0.2.0"}
			assert.Equal(t, tc.want, e.fresh(now))
		})
	}
}

func TestCacheEntry_NilFresh(t *testing.T) {
	var e *cacheEntry
	assert.False(t, e.fresh(time.Now()), "nil entry is never fresh")
}

func TestCacheRoundTrip(t *testing.T) {
	cfg := newTestCfg(t, "default")
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	writeCache(cfg.Aura.Fs(), cacheEntry{CheckedAt: now, LatestStable: "v0.2.0"})
	got := readCache(cfg.Aura.Fs())
	require.NotNil(t, got)
	assert.Equal(t, "v0.2.0", got.LatestStable)
	assert.True(t, got.CheckedAt.Equal(now))

	// File on disk should be valid JSON with the documented field names.
	raw, err := afero.ReadFile(cfg.Aura.Fs(), cachePath())
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	assert.Contains(t, doc, "checked_at")
	assert.Contains(t, doc, "latest_stable")
}

func TestDefaultRandFloat_InRange(t *testing.T) {
	for i := 0; i < 1000; i++ {
		v := defaultRandFloat()
		require.GreaterOrEqual(t, v, 0.0)
		require.Less(t, v, 1.0)
	}
}

// TestSchedule_NonBlocking asserts Schedule returns immediately even when
// latestFn would block. Without a hard ctx timeout in the goroutine OR a
// detached background context we'd hang here.
func TestSchedule_NonBlocking(t *testing.T) {
	resetOnce(t)
	t.Setenv(EnvDisable, "")
	withRandFloat(t, 0.0)

	block := make(chan struct{})
	defer close(block)
	withLatest(t, func(ctx context.Context, preReleases bool) (*update.Release, error) {
		<-block // never fires until test cleanup
		return nil, nil
	})

	cfg := newTestCfg(t, "default")
	start := time.Now()
	Schedule(context.Background(), cfg, "v0.1.0")
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 100*time.Millisecond, "Schedule must return without awaiting the goroutine")
}
