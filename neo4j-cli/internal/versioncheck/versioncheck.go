// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package versioncheck implements the silent background "is there a newer
// neo4j-cli release?" probe and the resulting one-line stderr nag.
//
// Two integration points:
//
//   - Schedule(ctx, cfg) — fires (with 5% probability) a background goroutine
//     that hits GitHub for the latest stable release and writes the result
//     to the cache. Errors are silent. Hard 5s timeout. Skipped if the cache
//     is fresh (<24h) so that the dice roll only causes a network call once
//     per day at most.
//
//   - MaybeHint(cmd, cfg, current) — reads the cache (no network) and prints
//     a single line to stderr when the cached `latest_stable` is newer than
//     the running binary's version. Suppressed on `neo4j-cli update`,
//     `--version`, `--help`, and `--format json`.
//
// Both are wired via the root cobra PersistentPreRunE in app/app.go. The
// dice roll, network call, and stderr nag are all separately disabled via
// `NEO4J_CLI_NO_UPDATE_NAG=1`.
package versioncheck

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"os"
	"sync"
	"time"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/update"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

// EnvDisable is the env-var users set to opt out of both the silent dice
// roll and the stderr nag. Any non-empty value disables.
const EnvDisable = "NEO4J_CLI_NO_UPDATE_NAG"

// SampleProbability is the chance per invocation that a fresh check is
// fired. 1-in-20 ≈ 5%. Power users running heavy automation thus pay the
// network cost on average once every 20 invocations.
const SampleProbability = 0.05

// checkTimeout caps the background check. The release-list call is cheap;
// 5 s comfortably covers a slow connection without delaying the foreground
// command's exit (we don't await the goroutine).
const checkTimeout = 5 * time.Second

// devVersion mirrors update.devVersion. We can't import the unexported sentinel,
// but the value is part of the build contract (set in app.Version when no
// ldflags tag is baked in).
const devVersion = "dev"

// Test seams. Production fills with real impls; tests swap via the
// withRandFloat / withNow / withLatest helpers in versioncheck_test.go.
//
// Per AGENTS.md "Package-level test seams": exposed as package-level vars so
// PersistentPreRunE wiring can be unit-tested against deterministic randomness
// and clock without re-testing the upstream release lookup.
var (
	// randFloat returns a float in [0, 1). The default uses crypto/rand for a
	// non-predictable dice roll — the actual probability calc is not security-
	// sensitive but a deterministic PRNG seed across processes would all hit
	// (or skip) on the same wall-clock second.
	randFloat = defaultRandFloat
	// nowFn shadows time.Now so cache TTL and "checked_at" can be pinned in
	// tests.
	nowFn = time.Now
	// latestFn is the underlying release-lookup; we wrap update.Latest so
	// tests can assert no network call without swapping the GitHub seam.
	latestFn = update.Latest
)

// once guards Schedule so that even if PersistentPreRunE somehow fires twice
// (composed hooks, traverse) the goroutine only spins up once per process.
var once sync.Once

// Schedule fires (with SampleProbability) a background goroutine that hits
// GitHub for the latest stable release and updates the cache. Returns
// immediately; the goroutine is detached and not awaited.
//
// Deliberately silent: every error path discards the error. The goroutine
// also defers a panic recovery so a bug in upstream cannot take the host
// process down.
//
// `current` is the running binary's version (app.Version). If "dev" or
// otherwise non-semver, we skip — there's nothing meaningful to compare
// against.
func Schedule(ctx context.Context, cfg *clicfg.Config, current string) {
	if disabled() {
		return
	}
	if current == "" || current == devVersion {
		return
	}
	if !semver.IsValid(current) {
		return
	}
	// Single-shot per process — Cobra's traverse-hook config plus future
	// composed PersistentPreRunE chains shouldn't double-fire the dice roll.
	once.Do(func() { scheduleOnce(ctx, cfg) })
}

// scheduleOnce performs the dice roll and starts the goroutine. Split from
// Schedule for testability without juggling sync.Once across cases.
func scheduleOnce(_ context.Context, cfg *clicfg.Config) {
	// Cache fresh → skip the network call entirely. The dice roll still
	// gates the cache READ so we don't refetch every invocation.
	if randFloat() >= SampleProbability {
		return
	}
	if cfg == nil || cfg.Aura == nil {
		return
	}
	fs := cfg.Aura.Fs()
	if cached := readCache(fs); cached.fresh(nowFn()) {
		return
	}

	go func() {
		defer func() { _ = recover() }()

		// Use a private timeout — we deliberately do NOT inherit the cobra
		// command's ctx here because:
		//
		//   - PersistentPreRunE returns immediately, so the parent ctx is
		//     bound to the entire foreground run. If the user's command takes
		//     20 minutes (e.g. a long Cypher query), we don't want the GH
		//     check hanging open that long.
		//   - If the user's command exits FAST (<5s), the foreground process
		//     just exits and the goroutine is killed mid-flight — exactly the
		//     "don't delay the user's command" requirement.
		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		defer cancel()

		release, err := latestFn(ctx, false)
		if err != nil || release == nil {
			return
		}
		if !semver.IsValid(release.TagName) {
			return
		}
		writeCache(fs, cacheEntry{
			CheckedAt:    nowFn(),
			LatestStable: release.TagName,
		})
	}()
}

// MaybeHint reads the cache (no network) and prints a single nag line to
// stderr when the cached `latest_stable` is newer than `current`.
//
// Suppressed when:
//   - `NEO4J_CLI_NO_UPDATE_NAG` is set (any non-empty value).
//   - cmd is the `update` command (would be confusing).
//   - `--help` or `--version` was requested (we don't pollute help output).
//   - cfg.Global.Format() is "json" (downstream JSON consumers stay clean).
//
// The hint goes to cmd.ErrOrStderr (NOT stdout) so scripts piping stdout
// remain unaffected.
func MaybeHint(cmd *cobra.Command, cfg *clicfg.Config, current string) {
	if disabled() {
		return
	}
	if cmd == nil || cfg == nil {
		return
	}
	if shouldSuppress(cmd, cfg) {
		return
	}
	if current == "" || current == devVersion || !semver.IsValid(current) {
		return
	}
	cached := readCache(cfg.Aura.Fs())
	if cached == nil {
		return
	}
	if !semver.IsValid(cached.LatestStable) {
		return
	}
	if semver.Compare(cached.LatestStable, current) <= 0 {
		return
	}
	// Format mirrors REQ-F task-014. One single line, no trailing prose.
	_, _ = cmd.ErrOrStderr().Write([]byte(
		"A newer neo4j-cli is available: " + cached.LatestStable +
			" (you have " + current + "). Run `neo4j-cli update` to upgrade.\n",
	))
}

// disabled reports whether the env var opt-out is set.
func disabled() bool {
	return os.Getenv(EnvDisable) != ""
}

// shouldSuppress checks the per-invocation suppression rules (the env-var
// gate is in disabled()).
func shouldSuppress(cmd *cobra.Command, cfg *clicfg.Config) bool {
	if cfg.Global != nil && cfg.Global.Format() == "json" {
		return true
	}
	// `--help` and `--version` short-circuit the cobra dispatch, but
	// PersistentPreRunE still runs. Detect both.
	if helpFlag := cmd.Flags().Lookup("help"); helpFlag != nil && helpFlag.Changed {
		return true
	}
	// Walk up to the root and look for `--version`. Cobra registers the
	// flag on the root only.
	root := cmd.Root()
	if versionFlag := root.PersistentFlags().Lookup("version"); versionFlag != nil && versionFlag.Changed {
		return true
	}
	if versionFlag := root.Flags().Lookup("version"); versionFlag != nil && versionFlag.Changed {
		return true
	}
	// Suppress on `neo4j-cli update` itself.
	if isUpdateCmd(cmd) {
		return true
	}
	return false
}

// isUpdateCmd reports whether cmd is the `update` command (or one of its
// future descendants).
func isUpdateCmd(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "update" {
			return true
		}
	}
	return false
}

// defaultRandFloat returns a float in [0, 1) using crypto/rand. We don't need
// cryptographic randomness for a 5% sampler, but using crypto/rand keeps the
// dependency surface small (no math/rand seed plumbing) and avoids the
// historical `rand.Seed` / global mutable state footguns.
func defaultRandFloat() float64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a value that ALWAYS suppresses the network call. The
		// stderr nag still works off the cache; the worst outcome is no
		// fresh check until rand recovers.
		return 1.0
	}
	// Use the top 53 bits to fit in a float64 mantissa without precision loss.
	u := binary.BigEndian.Uint64(b[:]) >> 11
	return float64(u) / float64(uint64(1)<<53)
}
