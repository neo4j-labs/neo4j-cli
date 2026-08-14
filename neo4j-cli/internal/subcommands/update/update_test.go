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

	"github.com/neo4j/cli/common/clicfg"
	commonskill "github.com/neo4j/cli/common/skill"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestRunUpdate_Force_NonBinaryMethod_WarnsOnStderr pins REQ-F-004: when
// `update --force` overrides a detected package-manager-managed binary, the
// channel-correct ForceOverrideWarning lands on stderr (header + path + revert
// sentence + uninstall command + curl install line) while the success
// narrative ("Successfully updated from ...") lands on stdout. Covers all four
// non-binary InstallMethods.
func TestRunUpdate_Force_NonBinaryMethod_WarnsOnStderr(t *testing.T) {
	cases := []struct {
		method        InstallMethod
		path          string
		wantLabel     string
		wantUninstall string
	}{
		{
			method:        InstallMethodHomebrew,
			path:          "/opt/homebrew/bin/neo4j-cli",
			wantLabel:     "Homebrew",
			wantUninstall: "brew uninstall neo4j-cli",
		},
		{
			method:        InstallMethodNpm,
			path:          "/usr/lib/node_modules/@neo4j-labs/cli/bin/neo4j-cli",
			wantLabel:     "npm/pnpm/yarn",
			wantUninstall: "npm uninstall -g @neo4j-labs/cli",
		},
		{
			method:        InstallMethodPipx,
			path:          "/home/user/.local/pipx/venvs/neo4j-cli/bin/neo4j-cli",
			wantLabel:     "pipx",
			wantUninstall: "pipx uninstall neo4j-cli",
		},
		{
			method:        InstallMethodUv,
			path:          "/home/user/.local/share/uv/tools/neo4j-cli/bin/neo4j-cli",
			wantLabel:     "uv tool",
			wantUninstall: "uv tool uninstall neo4j-cli",
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.method), func(t *testing.T) {
			withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
				return &Release{TagName: "v0.2.0"}, nil
			})
			withDetect(t, func() (InstallMethod, string, error) {
				return tc.method, tc.path, nil
			})
			withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
				return nil
			})

			stdout, stderr, err := runWithOptsSplit(t, "v0.1.0", runOpts{force: true}, "default")
			require.NoError(t, err)

			// Stderr carries the warning block.
			assert.Contains(t, stderr, "Warning: --force overriding detected "+tc.wantLabel+" install at "+tc.path+".")
			assert.Contains(t, stderr, "The package manager may revert this change on next upgrade.")
			assert.Contains(t, stderr, tc.wantUninstall)
			assert.Contains(t, stderr, "curl -sSfL https://neo4j.sh/install.sh | bash")

			// Stdout carries the success line.
			assert.Contains(t, stdout, "Successfully updated from v0.1.0 to v0.2.0")
			// Success line must NOT bleed to stderr (it's progress-on-stderr
			// for "Current version" / "Checking for updates" but the final
			// "Successfully updated" line goes via cmd.Println → stdout).
			assert.NotContains(t, stdout, "Warning: --force overriding detected",
				"warning must not bleed onto stdout")
		})
	}
}

// TestRunUpdate_Force_BinaryMethod_NoWarning pins REQ-F-004's binary-channel
// short-circuit: `update --force` on a self-managed binary must NOT emit the
// warning header (there's nothing to override).
func TestRunUpdate_Force_BinaryMethod_NoWarning(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/usr/local/bin/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	stdout, stderrOut, err := runWithOptsSplit(t, "v0.1.0", runOpts{force: true}, "default")
	require.NoError(t, err)
	assert.NotContains(t, stderrOut, "--force overriding detected",
		"binary-channel --force must not emit the pkg-mgr override warning")
	assert.Contains(t, stdout, "Successfully updated from v0.1.0 to v0.2.0")
}

// TestRunUpdate_Force_JSONOutput_WarningOnStderrOnly pins REQ-F-004 for the
// JSON-output branch: the warning is for humans and must land on stderr, while
// stdout stays a clean parseable JSON document (no warning bleed). Scripts that
// pipe `update --force --format json` to jq continue to work.
func TestRunUpdate_Force_JSONOutput_WarningOnStderrOnly(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodHomebrew, "/opt/homebrew/bin/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return nil
	})

	stdout, stderrOut, err := runWithOptsSplit(t, "v0.1.0", runOpts{force: true}, "json")
	require.NoError(t, err)

	// stdout: valid JSON, install_method=homebrew, updated=true.
	doc := parseJSONOutput(t, stdout)
	assert.Equal(t, "homebrew", doc.InstallMethod)
	assert.True(t, doc.Updated)
	assert.NotContains(t, stdout, "Warning:", "warning text must not bleed into JSON stdout")
	assert.NotContains(t, stdout, "--force overriding detected")

	// stderr: full warning block.
	assert.Contains(t, stderrOut, "Warning: --force overriding detected Homebrew install at /opt/homebrew/bin/neo4j-cli.")
	assert.Contains(t, stderrOut, "The package manager may revert this change on next upgrade.")
	assert.Contains(t, stderrOut, "brew uninstall neo4j-cli")
	assert.Contains(t, stderrOut, "curl -sSfL https://neo4j.sh/install.sh | bash")
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

// TestRunUpdate_SwapErrSudoUnavailable_RerunHintMatrix verifies REQ-F-014: a
// `*errSudoUnavailable` returned from Swap turns into a clear two-line fatal
// error with a "Re-run with sudo: <full command>" hint that reflects the
// flags actually passed to the current invocation. The cobra usage block must
// NOT print (SilenceUsage is set by task-001 BEFORE swapFn is invoked).
func TestRunUpdate_SwapErrSudoUnavailable_RerunHintMatrix(t *testing.T) {
	cases := []struct {
		name     string
		opts     runOpts
		wantHint string
	}{
		{
			name:     "bare",
			opts:     runOpts{},
			wantHint: "sudo neo4j-cli update",
		},
		{
			name:     "pre-releases",
			opts:     runOpts{preReleases: true},
			wantHint: "sudo neo4j-cli update --pre-releases",
		},
		{
			name:     "version tag",
			opts:     runOpts{version: "v0.1.0-alpha.10"},
			wantHint: "sudo neo4j-cli update --version v0.1.0-alpha.10",
		},
		{
			name:     "force",
			opts:     runOpts{force: true},
			wantHint: "sudo neo4j-cli update --force",
		},
		{
			name:     "pre-releases and force",
			opts:     runOpts{preReleases: true, force: true},
			wantHint: "sudo neo4j-cli update --pre-releases --force",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			latestTag := "v0.2.0"
			withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
				return &Release{TagName: latestTag}, nil
			})
			withGetByTag(t, func(ctx context.Context, tag string) (*Release, error) {
				return &Release{TagName: tag}, nil
			})
			withDetect(t, func() (InstallMethod, string, error) {
				return InstallMethodBinary, "/usr/local/bin/neo4j-cli", nil
			})
			withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
				return &errSudoUnavailable{dir: "/usr/local/bin"}
			})

			out, err := runWithOptsParented(t, "v0.1.0", tc.opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot write to /usr/local/bin (permission denied).")
			assert.Contains(t, err.Error(), "Re-run with sudo:")
			assert.Contains(t, err.Error(), tc.wantHint)
			// SilenceUsage gate from task-001 must prevent the cobra Usage
			// block from leaking into stdout/stderr.
			assert.NotContains(t, out, "Usage:")
		})
	}
}

// TestRunUpdate_SwapErrPermissionWindows_AdminShellHint verifies REQ-F-014: a
// `*errPermissionWindows` returned from Swap turns into a clear two-line fatal
// error with an "Administrator shell" hint. Same SilenceUsage assertion as the
// sudo branch.
func TestRunUpdate_SwapErrPermissionWindows_AdminShellHint(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, `C:\Program Files\neo4j-cli\neo4j-cli.exe`, nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		return &errPermissionWindows{dir: `C:\Program Files\neo4j-cli`}
	})

	out, err := runWithOptsParented(t, "v0.1.0", runOpts{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `cannot write to C:\Program Files\neo4j-cli (permission denied).`)
	assert.Contains(t, err.Error(), "Re-run from an Administrator shell.")
	assert.NotContains(t, err.Error(), "Re-run with sudo")
	assert.NotContains(t, out, "Usage:")
}

// TestBuildReRunCommand verifies the flag-reconstruction helper in isolation,
// independent of the runUpdate flow. Mirrors the matrix in
// TestRunUpdate_SwapErrSudoUnavailable_RerunHintMatrix. Mounts `update` under
// a synthetic `neo4j-cli` parent so CommandPath returns the production form.
func TestBuildReRunCommand(t *testing.T) {
	tfs, err := testfs.GetTestFs(`{"format":"default"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, "v0.1.0", clicfg.GlobalScope)

	root := &cobra.Command{Use: "neo4j-cli"}
	updateCmd := NewCmd(cfg, nil, "")
	root.AddCommand(updateCmd)

	cases := []struct {
		name string
		opts runOpts
		want string
	}{
		{"bare", runOpts{}, "neo4j-cli update"},
		{"pre-releases", runOpts{preReleases: true}, "neo4j-cli update --pre-releases"},
		{"version", runOpts{version: "v0.1.0-alpha.10"}, "neo4j-cli update --version v0.1.0-alpha.10"},
		{"force", runOpts{force: true}, "neo4j-cli update --force"},
		{
			"all flags",
			runOpts{preReleases: true, version: "v1.0.0", force: true},
			"neo4j-cli update --pre-releases --version v1.0.0 --force",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildReRunCommand(updateCmd, tc.opts)
			assert.Equal(t, tc.want, got)
		})
	}
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

// TestNewCmd_ForceFlagShorthand pins CLI-85 REQ-F-003: `update --force` claims
// the `-f` short-form letter freed by dropping `--format`'s shorthand. Locks
// the new binding so a future refactor cannot silently drop it.
func TestNewCmd_ForceFlagShorthand(t *testing.T) {
	tfs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, "v0.1.0", clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
	flag := cmd.Flags().Lookup("force")
	require.NotNil(t, flag, "--force must be registered on `update`")
	assert.Equal(t, "f", flag.Shorthand,
		"--force must claim the `-f` shorthand freed by CLI-85")
}

// TestNewCmd_ForceShorthand_ParsesToTrue exercises the cobra parse path for
// `update -f --version <valid>` and confirms `force=true` reaches runOpts.
// Locks REQ-F-003 end-to-end via cobra arg dispatch (not just flag-registration
// introspection).
func TestNewCmd_ForceShorthand_ParsesToTrue(t *testing.T) {
	withGetByTag(t, func(ctx context.Context, tag string) (*Release, error) {
		return &Release{TagName: tag}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodHomebrew, "/opt/homebrew/bin/neo4j-cli", nil
	})
	swapCalled := false
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string, stderr io.Writer) error {
		swapCalled = true
		return nil
	})

	tfs, err := testfs.GetTestFs(`{"format":"default"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, "v0.1.0", clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"-f", "--version", "v0.2.0"})

	require.NoError(t, cmd.Execute(), "cobra must parse `update -f --version <valid>` without error")
	assert.True(t, swapCalled,
		"`-f` must propagate as force=true and bypass the homebrew passthrough hint")
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

	// Plain-text branch: hint line. The success line stays on stdout; the
	// install-skill tip moved to stderr per CLI-96.
	out, errOut, err := runWithBundleFormat(t, "v0.1.0", runOpts{}, "default")
	require.NoError(t, err)
	assert.Contains(t, out, "Successfully updated from v0.1.0 to v0.2.0")
	assert.Contains(t, errOut, "Tip: install the agent skill")
	assert.Contains(t, errOut, "neo4j-cli skill install")

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

// TestUpdateCmd_UpgradeAlias pins CLI-70 REQ-F-001: the `update` cobra command
// exposes `upgrade` as an alias so users typing the more conventional verb land
// on the self-update flow. Asserts both that the Aliases field is set and that
// the alias resolves to the same command when mounted on a root tree (cobra's
// Find walks aliases when matching subcommand names).
func TestUpdateCmd_UpgradeAlias(t *testing.T) {
	tfs, err := testfs.GetTestFs(`{"format":"json"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, "v0.1.0", clicfg.GlobalScope)

	updateCmd := NewCmd(cfg, nil, "")
	assert.Contains(t, updateCmd.Aliases, "upgrade",
		"update command must expose `upgrade` as an alias (CLI-70 REQ-F-001)")

	// Resolve the alias from a root tree, mirroring how cobra dispatches
	// `neo4j-cli upgrade ...` at runtime. Find returns the command matched by
	// either its Use name or any alias.
	root := &cobra.Command{Use: "neo4j-cli"}
	root.AddCommand(updateCmd)
	resolved, _, ferr := root.Find([]string{"upgrade"})
	require.NoError(t, ferr, "root.Find([]string{\"upgrade\"}) must resolve via the alias")
	assert.Same(t, updateCmd, resolved,
		"`upgrade` must resolve to the same *cobra.Command as `update`")

	// The `check` subcommand should be reachable via the alias too — cobra
	// propagates aliases to subcommands, so `upgrade check` matches the same
	// leaf as `update check`. Find returns the leaf and an empty remaining-arg
	// slice when the match is exact.
	checkResolved, remaining, cferr := root.Find([]string{"upgrade", "check"})
	require.NoError(t, cferr, "root.Find([]string{\"upgrade\", \"check\"}) must resolve")
	assert.Empty(t, remaining, "check subcommand must consume the second arg")
	assert.Equal(t, "check", checkResolved.Name(),
		"`upgrade check` must resolve to the `check` subcommand under update")
}
