// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/neo4j/cli/common/clicfg"
	commonskill "github.com/neo4j/cli/common/skill"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
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
func withSwap(t *testing.T, fn func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error) {
	t.Helper()
	prev := swapFn
	swapFn = fn
	t.Cleanup(func() { swapFn = prev })
}

// withListSkills swaps the listSkillsFn seam. Returns a mock list of
// AgentInstall rows so tests can pre-seed installed/uninstalled agents
// without populating an afero.Fs with the right marker files.
func withListSkills(t *testing.T, fn func(filesystem afero.Fs, skillName string) ([]commonskill.AgentInstall, error)) {
	t.Helper()
	prev := listSkillsFn
	listSkillsFn = fn
	t.Cleanup(func() { listSkillsFn = prev })
}

// withInstallSkill swaps the installSkillFn seam. The test version
// records the per-agent invocation order and can simulate a refresh
// failure for a specific agent name.
func withInstallSkill(t *testing.T, fn func(filesystem afero.Fs, bundle fs.FS, skillName, version, agentFilter string) ([]*commonskill.Agent, error)) {
	t.Helper()
	prev := installSkillFn
	installSkillFn = fn
	t.Cleanup(func() { installSkillFn = prev })
}

// stubBundle is a tiny embed-shaped FS used by the post-swap skill-refresh
// tests. The content doesn't matter — refreshSkillBundles only checks for
// non-nil before invoking the seam.
var stubBundle fs.FS = fstest.MapFS{
	"SKILL.md": &fstest.MapFile{Data: []byte("stub")},
}

// runWithOpts builds a fresh cobra command with NewCmd, sets the version on
// the config, and dispatches RunE with the supplied opts. Returns the
// stdout buffer and the error (if any).
//
// Defaults to plain-text output (format="default"); JSON-output tests use
// runWithOptsFormat to override.
func runWithOpts(t *testing.T, current string, opts runOpts) (string, error) {
	t.Helper()
	return runWithOptsFormat(t, current, opts, "default")
}

// runWithOptsFormat is the format-explicit variant of runWithOpts. Used by
// the JSON-output golden tests. Pass "json" to seed cfg.Global.Format() with
// json so printResult routes through PrintBodyMap.
//
// `opts.bundle` / `opts.skillName` are left nil/empty by default — the
// post-swap skill-refresh path is exercised explicitly by the
// TestRunUpdate_PostSwap_* tests below which seed both. A nil bundle / empty
// skillName short-circuits refreshSkillBundles, which keeps the existing
// JSON / plain-text golden tests unchanged.
func runWithOptsFormat(t *testing.T, current string, opts runOpts, format string) (string, error) {
	t.Helper()
	cfgJSON := `{"format":"` + format + `"}`
	tfs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, current, clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
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

func TestRunUpdate_CheckMode_NewerAvailable_FriendlyHint(t *testing.T) {
	// REQ-F-001/002: `update check` + newer available exits 0 with a
	// two-line friendly hint pointing at the install command. The swap
	// path must not be invoked.
	swapCalled := false
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		// Plain binary so the install-method passthrough doesn't intercept.
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		swapCalled = true
		return nil
	})

	out, err := runWithOpts(t, "v0.1.0", runOpts{check: true})
	require.NoError(t, err, "`update check` + newer must NOT error — finding a new version is the success case")
	assert.Contains(t, out, "Current version: v0.1.0")
	assert.Contains(t, out, "Latest stable version: v0.2.0")
	assert.Contains(t, out, "New version available: v0.1.0 -> v0.2.0")
	assert.Contains(t, out, "Run `neo4j-cli update` to install.")
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
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
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
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
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
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
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
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
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
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
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
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
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
	tfs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, "v0.1.0", clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
	for _, name := range []string{"pre-releases", "version", "force"} {
		f := cmd.Flags().Lookup(name)
		require.NotNil(t, f, "flag --%s should be registered", name)
	}
	// `--check` was removed from the parent in favour of the `check`
	// subcommand. Guard against a future regression that re-registers it.
	assert.Nil(t, cmd.Flags().Lookup("check"), "--check must NOT be registered on the parent — use the `check` subcommand")
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
			// `update check` exits 0 even when newer is available — we
			// just want to inspect the channel string in the printed lines.
			require.NoError(t, err)
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

// TestPlainTextOutput_GoldenSuccess verifies the byte-for-byte plain-text
// success output per REQ-F-017 acceptance criterion 1.
func TestPlainTextOutput_GoldenSuccess(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    string
	}{
		{
			name:    "alpha tag",
			current: "v0.1.0-alpha.9",
			latest:  "v0.1.0-alpha.10",
			want: "Current version: v0.1.0-alpha.9\n" +
				"Checking for updates to latest version...\n" +
				"Successfully updated from v0.1.0-alpha.9 to v0.1.0-alpha.10\n",
		},
		{
			name:    "stable bump",
			current: "v1.0.0",
			latest:  "v1.1.0",
			want: "Current version: v1.0.0\n" +
				"Checking for updates to latest version...\n" +
				"Successfully updated from v1.0.0 to v1.1.0\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
				return &Release{TagName: tc.latest}, nil
			})
			withDetect(t, func() (InstallMethod, string, error) {
				return InstallMethodBinary, "/tmp/neo4j-cli", nil
			})
			withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
				return nil
			})

			out, err := runWithOpts(t, tc.current, runOpts{})
			require.NoError(t, err)
			assert.Equal(t, tc.want, out, "plain-text output must be byte-for-byte equal to the reference")
		})
	}
}

// parseJSONOutput parses an update-command JSON document into a typed struct.
// Helper reused across the JSON-output test cases below.
type updateJSON struct {
	Current       string `json:"current"`
	Latest        string `json:"latest"`
	Updated       bool   `json:"updated"`
	Check         bool   `json:"check"`
	Channel       string `json:"channel"`
	InstallMethod string `json:"install_method"`
}

func parseJSONOutput(t *testing.T, raw string) updateJSON {
	t.Helper()
	var doc updateJSON
	require.NoError(t, json.Unmarshal([]byte(raw), &doc), "stdout must be valid JSON: %q", raw)
	return doc
}

func TestJSONOutput_HappyPath(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{}, "json")
	require.NoError(t, err)

	doc := parseJSONOutput(t, out)
	assert.Equal(t, "v0.1.0", doc.Current)
	assert.Equal(t, "v0.2.0", doc.Latest)
	assert.True(t, doc.Updated)
	assert.False(t, doc.Check)
	assert.Equal(t, "stable", doc.Channel)
	assert.Equal(t, "binary", doc.InstallMethod)
}

func TestJSONOutput_CheckMode_NewerAvailable(t *testing.T) {
	// REQ-F-003 / REQ-F-018: `update check` + newer available exits 0 with
	// JSON sets updated:false, check:true. Scripts compare current!=latest
	// to detect drift rather than relying on a non-zero exit.
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{check: true}, "json")
	require.NoError(t, err, "`update check` + newer must NOT error — finding a new version is the success case")

	doc := parseJSONOutput(t, out)
	assert.Equal(t, "v0.1.0", doc.Current)
	assert.Equal(t, "v0.2.0", doc.Latest)
	assert.False(t, doc.Updated, "--check must never set updated=true")
	assert.True(t, doc.Check, "--check JSON must set check=true")
	assert.Equal(t, "binary", doc.InstallMethod)
}

func TestJSONOutput_CheckMode_UpToDate(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.1.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{check: true}, "json")
	require.NoError(t, err)

	doc := parseJSONOutput(t, out)
	assert.Equal(t, "v0.1.0", doc.Current)
	assert.Equal(t, "v0.1.0", doc.Latest)
	assert.False(t, doc.Updated)
	assert.True(t, doc.Check)
}

func TestJSONOutput_PkgMgrPassthrough(t *testing.T) {
	// REQ-F-018 / acceptance criterion 4: passthrough JSON includes
	// install_method=<channel> and updated=false.
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodHomebrew, "/opt/homebrew/bin/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		t.Fatal("swap must not run on pkg-mgr passthrough")
		return nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{}, "json")
	require.NoError(t, err)

	doc := parseJSONOutput(t, out)
	assert.Equal(t, "v0.1.0", doc.Current)
	assert.Equal(t, "v0.2.0", doc.Latest)
	assert.False(t, doc.Updated)
	assert.False(t, doc.Check)
	assert.Equal(t, "homebrew", doc.InstallMethod, "install_method must reflect detected pkg-mgr channel")
}

// TestJSONOutput_FieldOrderDeterministic asserts the documented REQ-F-018
// order (current, latest, updated, check, channel, install_method) is
// preserved in the rendered JSON byte stream so downstream scripts can rely
// on it.
func TestJSONOutput_FieldOrderDeterministic(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{}, "json")
	require.NoError(t, err)

	// Substring positions in the raw stdout — earlier-listed fields must
	// appear earlier in the JSON output.
	keys := []string{
		"\"current\"",
		"\"latest\"",
		"\"updated\"",
		"\"check\"",
		"\"channel\"",
		"\"install_method\"",
	}
	prev := -1
	for _, k := range keys {
		idx := strings.Index(out, k)
		require.GreaterOrEqual(t, idx, 0, "key %s missing from JSON output", k)
		assert.Greater(t, idx, prev, "key %s appears before its predecessor — REQ-F-018 order broken", k)
		prev = idx
	}
}

// TestTableOutput_HappyPath asserts --format table produces a structured
// table (not the plain-text running narrative) with the documented six
// columns rendered as headers. go-pretty/v6/table upper-cases header text by
// default, so assertions compare against strings.ToLower for header columns.
func TestTableOutput_HappyPath(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{}, "table")
	require.NoError(t, err)

	// The plain-text running narrative must NOT appear when --format table is
	// in effect — otherwise scripts piping table output get a polluted stream.
	assert.NotContains(t, out, "Successfully updated", "plain-text narrative must not bleed through structured output")
	assert.NotContains(t, out, "Checking for updates")

	// Headers (case-insensitive — go-pretty upper-cases by default).
	lower := strings.ToLower(out)
	for _, header := range []string{"current", "latest", "updated", "check", "channel", "install_method"} {
		assert.Contains(t, lower, header, "table output must include header %q", header)
	}
	// Body cells (exact case).
	assert.Contains(t, out, "v0.1.0", "table body must include current version")
	assert.Contains(t, out, "v0.2.0", "table body must include latest version")
	assert.Contains(t, out, "binary", "table body must include install_method")
}

// TestToonOutput_HappyPath asserts --format toon produces a structured toon
// document (not plain text and not JSON) containing the documented six fields.
func TestToonOutput_HappyPath(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	out, err := runWithOptsFormat(t, "v0.1.0", runOpts{}, "toon")
	require.NoError(t, err)

	// Toon must NOT be the plain-text narrative.
	assert.NotContains(t, out, "Successfully updated", "plain-text narrative must not bleed through structured output")
	assert.NotContains(t, out, "Checking for updates")

	// Toon must NOT be valid JSON (sanity guard against accidentally emitting
	// JSON when the user asked for toon).
	var v any
	jsonErr := json.Unmarshal([]byte(out), &v)
	assert.Error(t, jsonErr, "toon output should not parse as JSON, got: %q", out)

	// All six documented field keys must appear in the toon document.
	for _, key := range []string{"current", "latest", "updated", "check", "channel", "install_method"} {
		assert.Contains(t, out, key, "toon output must include key %q", key)
	}
	// Body values.
	assert.Contains(t, out, "v0.1.0")
	assert.Contains(t, out, "v0.2.0")
	assert.Contains(t, out, "binary")
}

// TestPrintableUpdateResult_AsArrayShape covers the table-render shape of
// printableUpdateResult. We don't ship a "table" output for update (the
// document isn't list-shaped) but PrintBodyMap insists on AsArray, so the
// data must round-trip cleanly even via the table branch.
func TestPrintableUpdateResult_AsArrayShape(t *testing.T) {
	p := printableUpdateResult{r: updateResult{
		current:       "v0.1.0",
		latest:        "v0.2.0",
		updated:       true,
		check:         false,
		channel:       "stable",
		installMethod: "binary",
	}}
	rows := p.AsArray()
	require.Len(t, rows, 1, "AsArray must wrap the single document in a one-row slice")
	row := rows[0]
	assert.Equal(t, "v0.1.0", row["current"])
	assert.Equal(t, "v0.2.0", row["latest"])
	assert.Equal(t, true, row["updated"])
	assert.Equal(t, false, row["check"])
	assert.Equal(t, "stable", row["channel"])
	assert.Equal(t, "binary", row["install_method"])
}

// TestNewCmd_StubReplacedByRunUpdate ensures cmd.RunE actually dispatches to
// runUpdate — guards against a future refactor accidentally re-introducing
// the "not implemented" stub from task-002.
//
// Originally drove `cmd.SetArgs([]string{"--check"})` against the parent; the
// `--check` flag was replaced by the `check` subcommand, so this case now
// invokes the subcommand path. The equivalent end-to-end check (cobra-arg
// dispatch through the new subcommand) lives in check_test.go's
// TestCheckCmd_DispatchesToRunUpdate.
func TestNewCmd_StubReplacedByRunUpdate(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.1.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})

	// format: "default" so the plain-text "Already on" line is emitted (this
	// test asserts that the stub from task-002 is replaced by runUpdate, not
	// the output formatting itself — that lives in TestPlainTextOutput_*).
	tfs, err := testfs.GetTestFs(`{"format":"default"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, "v0.1.0", clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"check"})

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
	tfs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, "v0.1.0", clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
	assert.Contains(t, cmd.Long, "Self-update")
	assert.Contains(t, cmd.Long, "--pre-releases")
	assert.Contains(t, cmd.Long, "--force")
}

// runWithBundleFormat is the bundle-aware variant of runWithOptsFormat. The
// post-swap skill-refresh tests below need NewCmd to receive a non-nil
// bundle + skillName so refreshSkillBundles fires.
func runWithBundleFormat(t *testing.T, current string, opts runOpts, format string) (string, string, error) {
	t.Helper()
	cfgJSON := `{"format":"` + format + `"}`
	tfs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, current, clicfg.GlobalScope)

	cmd := NewCmd(cfg, stubBundle, "neo4j-cli")
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errOut)

	opts.bundle = stubBundle
	opts.skillName = "neo4j-cli"

	err = runUpdate(context.Background(), cmd, cfg, opts)
	return out.String(), errOut.String(), err
}

// TestRunUpdate_PostSwap_RefreshesInstalledAgents covers the happy path:
// two agents are pre-seeded as installed; both get refreshed via
// installSkillFn and appear in the user-facing output.
func TestRunUpdate_PostSwap_RefreshesInstalledAgents(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	claude := &commonskill.Agent{Name: "claude-code", DisplayName: "Claude Code"}
	cursor := &commonskill.Agent{Name: "cursor", DisplayName: "Cursor"}
	codex := &commonskill.Agent{Name: "codex", DisplayName: "Codex"}
	withListSkills(t, func(filesystem afero.Fs, skillName string) ([]commonskill.AgentInstall, error) {
		return []commonskill.AgentInstall{
			{Agent: claude, Detected: true, Installed: true, InstalledVersion: "v0.1.0"},
			{Agent: cursor, Detected: true, Installed: true, InstalledVersion: "v0.1.0"},
			// codex detected but NOT installed — must NOT be refreshed.
			{Agent: codex, Detected: true, Installed: false},
		}, nil
	})
	var refreshed []string
	withInstallSkill(t, func(filesystem afero.Fs, bundle fs.FS, skillName, version, agentFilter string) ([]*commonskill.Agent, error) {
		assert.Equal(t, "neo4j-cli", skillName)
		assert.Equal(t, "v0.2.0", version, "refresh must use the new binary's version, not the old one")
		assert.NotNil(t, bundle, "bundle must be threaded through to the refresh call")
		refreshed = append(refreshed, agentFilter)
		// Pretend the install returned the matched agent.
		return []*commonskill.Agent{{Name: agentFilter}}, nil
	})

	out, _, err := runWithBundleFormat(t, "v0.1.0", runOpts{}, "default")
	require.NoError(t, err)
	assert.Equal(t, []string{"claude-code", "cursor"}, refreshed,
		"only Installed=true agents must be refreshed, in catalog order")
	assert.Contains(t, out, "Successfully updated from v0.1.0 to v0.2.0")
	assert.Contains(t, out, "Refreshed skill bundle for: claude-code, cursor")
	// The hint for the no-installs branch must NOT fire here.
	assert.NotContains(t, out, "Tip: install the agent skill")
}

// TestRunUpdate_PostSwap_NoAgentsInstalled covers the hint branch: no agent
// has the skill installed, so the user is told to run skill install. The
// JSON shape gains skill_install_suggested: true.
func TestRunUpdate_PostSwap_NoAgentsInstalled(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	claude := &commonskill.Agent{Name: "claude-code", DisplayName: "Claude Code"}
	withListSkills(t, func(filesystem afero.Fs, skillName string) ([]commonskill.AgentInstall, error) {
		return []commonskill.AgentInstall{
			{Agent: claude, Detected: true, Installed: false},
		}, nil
	})
	withInstallSkill(t, func(filesystem afero.Fs, bundle fs.FS, skillName, version, agentFilter string) ([]*commonskill.Agent, error) {
		t.Fatal("installSkillFn must not be called when no agent has the skill installed")
		return nil, nil
	})

	// Plain-text branch: hint line.
	out, _, err := runWithBundleFormat(t, "v0.1.0", runOpts{}, "default")
	require.NoError(t, err)
	assert.Contains(t, out, "Successfully updated from v0.1.0 to v0.2.0")
	assert.Contains(t, out, "Tip: install the agent skill")
	assert.Contains(t, out, "neo4j-cli skill install")

	// JSON branch: skill_install_suggested:true. Reset seams via withX helpers
	// (they auto-restore on Cleanup; re-applying overrides for the second sub-run).
	withListSkills(t, func(filesystem afero.Fs, skillName string) ([]commonskill.AgentInstall, error) {
		return []commonskill.AgentInstall{{Agent: claude, Detected: true, Installed: false}}, nil
	})
	jsonOut, _, err := runWithBundleFormat(t, "v0.1.0", runOpts{}, "json")
	require.NoError(t, err)

	var doc struct {
		Updated               bool     `json:"updated"`
		UpdatedSkills         []string `json:"updated_skills"`
		SkillInstallSuggested bool     `json:"skill_install_suggested"`
	}
	require.NoError(t, json.Unmarshal([]byte(jsonOut), &doc))
	assert.True(t, doc.Updated)
	assert.Empty(t, doc.UpdatedSkills, "updated_skills must be omitted/empty when no agent was refreshed")
	assert.True(t, doc.SkillInstallSuggested, "skill_install_suggested must be true on the no-installs branch")
}

// TestRunUpdate_PostSwap_RefreshFailure_NonFatal covers the resilience
// requirement: a refresh failure for one agent must NOT cause update to
// exit non-zero, and the binary update must still report updated:true.
// A stderr warning is the only user-visible signal.
func TestRunUpdate_PostSwap_RefreshFailure_NonFatal(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	claude := &commonskill.Agent{Name: "claude-code", DisplayName: "Claude Code"}
	cursor := &commonskill.Agent{Name: "cursor", DisplayName: "Cursor"}
	withListSkills(t, func(filesystem afero.Fs, skillName string) ([]commonskill.AgentInstall, error) {
		return []commonskill.AgentInstall{
			{Agent: claude, Detected: true, Installed: true},
			{Agent: cursor, Detected: true, Installed: true},
		}, nil
	})
	withInstallSkill(t, func(filesystem afero.Fs, bundle fs.FS, skillName, version, agentFilter string) ([]*commonskill.Agent, error) {
		if agentFilter == "claude-code" {
			return nil, errors.New("simulated refresh failure")
		}
		return []*commonskill.Agent{{Name: agentFilter}}, nil
	})

	out, errOut, err := runWithBundleFormat(t, "v0.1.0", runOpts{}, "default")
	require.NoError(t, err, "refresh failure must NOT fail the binary-update command")
	assert.Contains(t, out, "Successfully updated from v0.1.0 to v0.2.0")
	// Only cursor was refreshed (claude failed).
	assert.Contains(t, out, "Refreshed skill bundle for: cursor")
	assert.NotContains(t, out, "claude-code")
	// Stderr carries the warning so the user knows something didn't work.
	assert.Contains(t, errOut, "Warning")
	assert.Contains(t, errOut, "claude-code")
	assert.Contains(t, errOut, "simulated refresh failure")
}

// TestRunUpdate_PkgMgrPassthrough_DoesNotRefreshSkills asserts the pkg-mgr
// passthrough branch never reaches refreshSkillBundles — no swap occurred,
// so the on-disk bundle is not stale and we mustn't pretend it was refreshed.
func TestRunUpdate_PkgMgrPassthrough_DoesNotRefreshSkills(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodHomebrew, "/opt/homebrew/bin/neo4j-cli", nil
	})
	withListSkills(t, func(filesystem afero.Fs, skillName string) ([]commonskill.AgentInstall, error) {
		t.Fatal("listSkillsFn must not be called on the pkg-mgr passthrough branch (no swap occurred)")
		return nil, nil
	})
	withInstallSkill(t, func(filesystem afero.Fs, bundle fs.FS, skillName, version, agentFilter string) ([]*commonskill.Agent, error) {
		t.Fatal("installSkillFn must not be called on the pkg-mgr passthrough branch")
		return nil, nil
	})

	out, _, err := runWithBundleFormat(t, "v0.1.0", runOpts{}, "default")
	require.NoError(t, err)
	assert.Contains(t, out, "brew upgrade neo4j-cli")
	// And no skill bookkeeping leaked into the message.
	assert.NotContains(t, out, "Refreshed skill bundle")
	assert.NotContains(t, out, "Tip: install the agent skill")
}

// TestRunUpdate_CheckMode_DoesNotRefreshSkills mirrors the pkg-mgr branch
// for the --check path: no swap occurred, no refresh.
func TestRunUpdate_CheckMode_DoesNotRefreshSkills(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withListSkills(t, func(filesystem afero.Fs, skillName string) ([]commonskill.AgentInstall, error) {
		t.Fatal("listSkillsFn must not be called on --check (no swap occurred)")
		return nil, nil
	})
	withInstallSkill(t, func(filesystem afero.Fs, bundle fs.FS, skillName, version, agentFilter string) ([]*commonskill.Agent, error) {
		t.Fatal("installSkillFn must not be called on --check")
		return nil, nil
	})

	_, _, err := runWithBundleFormat(t, "v0.1.0", runOpts{check: true}, "default")
	require.NoError(t, err, "`update check` + newer exits 0 — refresh path is gated on a successful swap, not on the error/non-error split")
}

// TestRunUpdate_PostSwap_NilBundle_SkipsRefresh asserts the bundle-nil
// short-circuit: tests that don't thread a bundle through must not fire
// the skill seams (which would otherwise be ambient-noise in unrelated
// tests).
func TestRunUpdate_PostSwap_NilBundle_SkipsRefresh(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})
	withListSkills(t, func(filesystem afero.Fs, skillName string) ([]commonskill.AgentInstall, error) {
		t.Fatal("listSkillsFn must not be called when bundle is nil")
		return nil, nil
	})

	out, err := runWithOpts(t, "v0.1.0", runOpts{})
	require.NoError(t, err)
	assert.Contains(t, out, "Successfully updated from v0.1.0 to v0.2.0")
	assert.NotContains(t, out, "Refreshed skill bundle")
	assert.NotContains(t, out, "Tip: install the agent skill")
}

// TestRunUpdate_PostSwap_JSONHappyPath asserts the structured-output shape
// when both the binary and (at least one) skill bundle were refreshed:
// updated_skills lists the agents in catalog order, skill_install_suggested
// is omitted.
func TestRunUpdate_PostSwap_JSONHappyPath(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})
	claude := &commonskill.Agent{Name: "claude-code", DisplayName: "Claude Code"}
	cursor := &commonskill.Agent{Name: "cursor", DisplayName: "Cursor"}
	withListSkills(t, func(filesystem afero.Fs, skillName string) ([]commonskill.AgentInstall, error) {
		return []commonskill.AgentInstall{
			{Agent: claude, Detected: true, Installed: true},
			{Agent: cursor, Detected: true, Installed: true},
		}, nil
	})
	withInstallSkill(t, func(filesystem afero.Fs, bundle fs.FS, skillName, version, agentFilter string) ([]*commonskill.Agent, error) {
		return []*commonskill.Agent{{Name: agentFilter}}, nil
	})

	out, _, err := runWithBundleFormat(t, "v0.1.0", runOpts{}, "json")
	require.NoError(t, err)

	var doc struct {
		Current               string   `json:"current"`
		Latest                string   `json:"latest"`
		Updated               bool     `json:"updated"`
		UpdatedSkills         []string `json:"updated_skills"`
		SkillInstallSuggested bool     `json:"skill_install_suggested"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &doc))
	assert.Equal(t, "v0.1.0", doc.Current)
	assert.Equal(t, "v0.2.0", doc.Latest)
	assert.True(t, doc.Updated)
	assert.Equal(t, []string{"claude-code", "cursor"}, doc.UpdatedSkills)
	assert.False(t, doc.SkillInstallSuggested,
		"skill_install_suggested must be omitted/false when at least one agent was refreshed")

	// Field-order: updated_skills must appear AFTER install_method in the
	// raw stream so downstream scripts can rely on REQ-F-018 + the new keys.
	imIdx := strings.Index(out, "\"install_method\"")
	usIdx := strings.Index(out, "\"updated_skills\"")
	require.GreaterOrEqual(t, imIdx, 0)
	require.GreaterOrEqual(t, usIdx, 0)
	assert.Greater(t, usIdx, imIdx, "updated_skills must follow install_method in JSON output")
}

// TestPrintableUpdateResult_AsArrayShape_PostSwapFields asserts the table /
// toon AsArray shape includes updated_skills + skill_install_suggested when
// (and only when) they're populated.
func TestPrintableUpdateResult_AsArrayShape_PostSwapFields(t *testing.T) {
	cases := []struct {
		name                     string
		updatedSkills            []string
		skillInstallSuggested    bool
		wantUpdatedSkillsKey     bool
		wantSkillInstallSuggKey  bool
		wantSkillInstallSuggCell any
	}{
		{
			name:                    "no post-swap fields",
			updatedSkills:           nil,
			skillInstallSuggested:   false,
			wantUpdatedSkillsKey:    false,
			wantSkillInstallSuggKey: false,
		},
		{
			name:                    "agents refreshed",
			updatedSkills:           []string{"claude-code", "cursor"},
			skillInstallSuggested:   false,
			wantUpdatedSkillsKey:    true,
			wantSkillInstallSuggKey: false,
		},
		{
			name:                     "no agents — suggest install",
			updatedSkills:            nil,
			skillInstallSuggested:    true,
			wantUpdatedSkillsKey:     false,
			wantSkillInstallSuggKey:  true,
			wantSkillInstallSuggCell: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := printableUpdateResult{r: updateResult{
				current:               "v0.1.0",
				latest:                "v0.2.0",
				updated:               true,
				channel:               "stable",
				installMethod:         "binary",
				updatedSkills:         tc.updatedSkills,
				skillInstallSuggested: tc.skillInstallSuggested,
			}}
			rows := p.AsArray()
			require.Len(t, rows, 1)
			_, hasUS := rows[0]["updated_skills"]
			_, hasSIS := rows[0]["skill_install_suggested"]
			assert.Equal(t, tc.wantUpdatedSkillsKey, hasUS, "updated_skills key presence")
			assert.Equal(t, tc.wantSkillInstallSuggKey, hasSIS, "skill_install_suggested key presence")
			if tc.wantSkillInstallSuggKey {
				assert.Equal(t, tc.wantSkillInstallSuggCell, rows[0]["skill_install_suggested"])
			}
		})
	}
}
