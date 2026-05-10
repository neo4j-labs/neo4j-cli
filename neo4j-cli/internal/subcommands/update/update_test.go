// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package update

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withLatest swaps the latestFn seam for the test's duration.
func withLatest(t *testing.T, fn func(ctx context.Context, preReleases bool) (*Release, error)) {
	t.Helper()
	prev := latestFn
	latestFn = fn
	t.Cleanup(func() { latestFn = prev })
}

// withGetByTag swaps the getByTagFn seam.
func withGetByTag(t *testing.T, fn func(ctx context.Context, tag string) (*Release, error)) {
	t.Helper()
	prev := getByTagFn
	getByTagFn = fn
	t.Cleanup(func() { getByTagFn = prev })
}

// withDetect swaps the detectFn seam.
func withDetect(t *testing.T, fn func() (InstallMethod, string, error)) {
	t.Helper()
	prev := detectFn
	detectFn = fn
	t.Cleanup(func() { detectFn = prev })
}

// withSwap swaps the swapFn seam. Used by tests that want to assert the
// swap path is or is not invoked without setting up a real archive.
func withSwap(t *testing.T, fn func(ctx context.Context, urls AssetURLs, currentBinaryPath string) error) {
	t.Helper()
	prev := swapFn
	swapFn = fn
	t.Cleanup(func() { swapFn = prev })
}

// runWithOpts builds a fresh cobra command with NewCmd, sets the version on
// the config, and dispatches RunE with the supplied opts. Returns the
// stdout buffer and the error (if any).
func runWithOpts(t *testing.T, current string, opts runOpts) (string, error) {
	t.Helper()
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, current, clicfg.GlobalScope)

	cmd := NewCmd(cfg)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)

	err = runUpdate(context.Background(), cmd, cfg, opts)
	return out.String(), err
}

func TestRunUpdate_DevBuild_ShortCircuits(t *testing.T) {
	// REQ-F-002 / acceptance criterion 1: dev build exits 0 with no HTTP call.
	httpCalled := false
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		httpCalled = true
		return &Release{TagName: "v9.9.9"}, nil
	})
	withGetByTag(t, func(ctx context.Context, tag string) (*Release, error) {
		httpCalled = true
		return nil, fmt.Errorf("should not be called")
	})

	out, err := runWithOpts(t, "dev", runOpts{})
	require.NoError(t, err)
	assert.Contains(t, out, "dev build")
	assert.False(t, httpCalled, "no HTTP path should be touched on dev build")
}

func TestRunUpdate_EmptyVersion_TreatedAsDev(t *testing.T) {
	// Defensive: an empty cfg.Version should never happen in production but
	// the code must not crash.
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		t.Fatal("should not be called for empty version")
		return nil, nil
	})

	out, err := runWithOpts(t, "", runOpts{})
	require.NoError(t, err)
	assert.Contains(t, out, "dev build")
}

func TestRunUpdate_NoStableYet_PreReleasesHint(t *testing.T) {
	// REQ-F-006 / acceptance criterion 2: stable-only filter, zero matches
	// → friendly hint, exit 0.
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		assert.False(t, preReleases, "stable-only path expected")
		return nil, ErrNoStableRelease
	})

	out, err := runWithOpts(t, "v0.1.0", runOpts{})
	require.NoError(t, err)
	assert.Contains(t, out, "no stable release published yet")
	assert.Contains(t, out, "--pre-releases")
}

func TestRunUpdate_CheckMode_NewerAvailable_ReturnsError(t *testing.T) {
	// REQ-F-011 / acceptance criterion 3: --check + newer available exits 1
	// (non-nil error). The swap path must not be invoked.
	swapCalled := false
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		// Plain binary so the install-method passthrough doesn't intercept.
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string) error {
		swapCalled = true
		return nil
	})

	out, err := runWithOpts(t, "v0.1.0", runOpts{check: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "newer version is available")
	assert.Contains(t, out, "v0.1.0")
	assert.Contains(t, out, "v0.2.0")
	assert.False(t, swapCalled, "--check must never invoke swap")
}

func TestRunUpdate_CheckMode_UpToDate_ExitsZero(t *testing.T) {
	// REQ-F-011: --check + up-to-date returns nil (exit 0).
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.1.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})

	out, err := runWithOpts(t, "v0.1.0", runOpts{check: true})
	require.NoError(t, err)
	assert.Contains(t, out, "Already on v0.1.0")
}

func TestRunUpdate_VersionFlag_TagNotFound(t *testing.T) {
	// Acceptance criterion 4: --version with a semver-valid but non-existent
	// tag surfaces a clear "tag not found" error.
	withGetByTag(t, func(ctx context.Context, tag string) (*Release, error) {
		assert.Equal(t, "v9.9.9", tag)
		return nil, ErrTagNotFound
	})

	_, err := runWithOpts(t, "v0.1.0", runOpts{version: "v9.9.9"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "v9.9.9")
	assert.Contains(t, err.Error(), "not found")
}

func TestRunUpdate_VersionFlag_InvalidTag(t *testing.T) {
	// ValidateVersionTag rejects shell metacharacters before any HTTP call.
	getByTagCalled := false
	withGetByTag(t, func(ctx context.Context, tag string) (*Release, error) {
		getByTagCalled = true
		return nil, nil
	})

	_, err := runWithOpts(t, "v0.1.0", runOpts{version: "v0.1.0; rm -rf /"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --version")
	assert.False(t, getByTagCalled, "invalid tag must be rejected before any HTTP call")
}

func TestRunUpdate_DowngradeRejectedWithoutVersion(t *testing.T) {
	// Acceptance criterion 5: current > latest, no --version → reject.
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.1.0"}, nil
	})

	_, err := runWithOpts(t, "v0.2.0", runOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "newer than the latest")
	assert.Contains(t, err.Error(), "downgrade")
}

func TestRunUpdate_DowngradeAllowedWithExplicitVersion(t *testing.T) {
	// Same as above but with --version <older>: allowed (mechanically), no
	// extra UX wraps it.
	withGetByTag(t, func(ctx context.Context, tag string) (*Release, error) {
		return &Release{TagName: tag}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	swapCalled := false
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string) error {
		swapCalled = true
		return nil
	})

	out, err := runWithOpts(t, "v0.2.0", runOpts{version: "v0.1.0"})
	require.NoError(t, err)
	assert.True(t, swapCalled, "explicit --version downgrade should reach swap")
	assert.Contains(t, out, "Successfully updated from v0.2.0 to v0.1.0")
}

func TestRunUpdate_PkgMgrPassthrough_NoForce_ShowsHintAndExits(t *testing.T) {
	// REQ-F-010: detected pkg-mgr binary + no --force → print hint, exit 0,
	// never reach swap.
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodHomebrew, "/opt/homebrew/bin/neo4j-cli", nil
	})
	swapCalled := false
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string) error {
		swapCalled = true
		return nil
	})

	out, err := runWithOpts(t, "v0.1.0", runOpts{})
	require.NoError(t, err)
	assert.Contains(t, out, "brew upgrade neo4j-cli")
	assert.Contains(t, out, "v0.1.0")
	assert.Contains(t, out, "v0.2.0")
	assert.False(t, swapCalled, "pkg-mgr passthrough must skip swap")
}

func TestRunUpdate_ForceBypassesPkgMgrCheck(t *testing.T) {
	// Acceptance criterion 6: --force bypasses install-method detection.
	detectCalled := false
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		detectCalled = true
		return InstallMethodHomebrew, "/opt/homebrew/bin/neo4j-cli", nil
	})
	swapCalled := false
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string) error {
		swapCalled = true
		assert.Equal(t, "/opt/homebrew/bin/neo4j-cli", currentBinaryPath, "swap should target the resolved exe path")
		return nil
	})

	out, err := runWithOpts(t, "v0.1.0", runOpts{force: true})
	require.NoError(t, err)
	assert.True(t, swapCalled, "--force should reach swap even on pkg-mgr-managed binary")
	// detectFn is still called once (to obtain the resolved currentBinaryPath
	// for swap) but the Hint passthrough is bypassed.
	assert.True(t, detectCalled)
	assert.Contains(t, out, "Successfully updated from v0.1.0 to v0.2.0")
}

func TestRunUpdate_HappyPath_BinaryChannel(t *testing.T) {
	// Plain self-managed binary: detect returns "binary", swap runs.
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		assert.False(t, preReleases)
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string) error {
		assert.NotEmpty(t, urls.Archive)
		assert.NotEmpty(t, urls.Checksum)
		assert.Equal(t, "/tmp/neo4j-cli", currentBinaryPath)
		return nil
	})

	out, err := runWithOpts(t, "v0.1.0", runOpts{})
	require.NoError(t, err)
	assert.Contains(t, out, "Current version: v0.1.0")
	assert.Contains(t, out, "Checking for updates")
	assert.Contains(t, out, "Successfully updated from v0.1.0 to v0.2.0")
}

func TestRunUpdate_PreReleasesFlag_PassedThrough(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		assert.True(t, preReleases, "--pre-releases must propagate to Latest")
		return &Release{TagName: "v0.2.0-alpha.1"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string) error {
		return nil
	})

	out, err := runWithOpts(t, "v0.1.0", runOpts{preReleases: true})
	require.NoError(t, err)
	assert.Contains(t, out, "v0.2.0-alpha.1")
}

func TestRunUpdate_LookupError_SurfacesUpstream(t *testing.T) {
	// Generic network failure during release lookup must not be mistaken for
	// "no stable release" or "tag not found".
	upstreamErr := errors.New("connection refused")
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return nil, upstreamErr
	})

	_, err := runWithOpts(t, "v0.1.0", runOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "look up release")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestRunUpdate_InvalidCurrentVersion_FailsClean(t *testing.T) {
	// Defensive: app.Version is the release-pipeline-supplied tag, but if
	// somehow it's not valid semver we should fail loud rather than guess.
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})

	_, err := runWithOpts(t, "not-a-version", runOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid semver")
}

func TestRunUpdate_SwapFailure_PropagatesError(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string) error {
		return errors.New("simulated swap failure")
	})

	_, err := runWithOpts(t, "v0.1.0", runOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
	assert.Contains(t, err.Error(), "simulated swap failure")
}

func TestNewCmd_FlagsExposed(t *testing.T) {
	// Smoke-test that the four user-facing flags are still wired after the
	// RunE refactor.
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "v0.1.0", clicfg.GlobalScope)

	cmd := NewCmd(cfg)
	for _, name := range []string{"pre-releases", "check", "version", "force"} {
		f := cmd.Flags().Lookup(name)
		require.NotNil(t, f, "flag --%s should be registered", name)
	}
}

func TestRunUpdate_ChannelLabel_StableVsPreRelease(t *testing.T) {
	// The channel label appears in user-facing messages (and feeds JSON
	// output in task-007). Stable target → "stable"; prerelease tag with
	// --pre-releases → "pre-release".
	cases := []struct {
		name        string
		current     string
		preReleases bool
		latestTag   string
		wantChannel string
	}{
		{"stable target", "v0.1.0", false, "v0.2.0", "stable"},
		{"prerelease target with flag", "v0.1.0-alpha.1", true, "v0.1.0-alpha.2", "pre-release"},
		{"stable target via prerelease flag", "v0.1.0", true, "v0.2.0", "stable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
				return &Release{TagName: tc.latestTag}, nil
			})
			withDetect(t, func() (InstallMethod, string, error) {
				return InstallMethodBinary, "/tmp/neo4j-cli", nil
			})

			out, err := runWithOpts(t, tc.current, runOpts{check: true, preReleases: tc.preReleases})
			// --check + newer always errors — we just want to inspect the
			// channel string in the printed lines.
			require.Error(t, err)
			assert.Contains(t, out, "Latest "+tc.wantChannel+" version:", "out=%q", out)
		})
	}
}

// TestSeams_Update_Smoke makes sure the new RunE seams compile and behave
// when not swapped — production fills with the real impls.
func TestSeams_Update_Smoke(t *testing.T) {
	require.NotNil(t, latestFn)
	require.NotNil(t, getByTagFn)
	require.NotNil(t, detectFn)
	require.NotNil(t, swapFn)
}

// TestNewCmd_StubReplacedByRunUpdate ensures cmd.RunE actually dispatches to
// runUpdate — guards against a future refactor accidentally re-introducing
// the "not implemented" stub from task-002.
func TestNewCmd_StubReplacedByRunUpdate(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.1.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})

	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "v0.1.0", clicfg.GlobalScope)

	cmd := NewCmd(cfg)
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--check"})

	runErr := cmd.Execute()
	require.NoError(t, runErr)
	assert.Contains(t, out.String(), "Already on v0.1.0")
	// Sanity: we did NOT hit the stub.
	assert.NotContains(t, strings.ToLower(out.String()), "not implemented")
}

// quietCmd is a small helper kept here in case we want to assert that the
// Long string survives the RunE refactor (referenced by the skill bundle).
// Not strictly required for task-006 but cheap.
func TestNewCmd_LongDescriptionPreserved(t *testing.T) {
	fs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "v0.1.0", clicfg.GlobalScope)

	cmd := NewCmd(cfg)
	assert.Contains(t, cmd.Long, "Self-update")
	assert.Contains(t, cmd.Long, "--pre-releases")
	assert.Contains(t, cmd.Long, "--force")
}
