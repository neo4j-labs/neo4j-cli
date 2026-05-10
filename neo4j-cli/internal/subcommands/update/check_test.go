// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package update

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findCheckSubcommand returns the `check` cobra subcommand mounted under the
// `update` parent built by NewCmd. Fails the test if it isn't present.
func findCheckSubcommand(t *testing.T, parent *cobra.Command) *cobra.Command {
	t.Helper()
	for _, c := range parent.Commands() {
		if c.Name() == "check" {
			return c
		}
	}
	t.Fatalf("`check` subcommand not mounted on update parent — found %v", commandNames(parent))
	return nil
}

func commandNames(parent *cobra.Command) []string {
	out := make([]string, 0, len(parent.Commands()))
	for _, c := range parent.Commands() {
		out = append(out, c.Name())
	}
	return out
}

// newUpdateCmdForTest builds a parent `update` cobra tree backed by an empty
// test FS and a default-format config. Mirrors runWithOpts but exposes the
// parent so subcommand dispatch tests can drive `cmd.SetArgs([]string{"check", ...})`.
//
// Splits stdout from stderr so the JSON branch can assert a clean stdout —
// cobra writes the trailing "Error: ..." + Usage block to cmd.ErrOrStderr()
// when a RunE returns non-nil.
func newUpdateCmdForTest(t *testing.T, current, format string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cfgJSON := `{"format":"` + format + `"}`
	tfs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, current, clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	return cmd, out, errOut
}

// TestCheckCmd_Mounted asserts the `check` subcommand is mounted on the
// `update` parent (cmd.Find succeeds).
func TestCheckCmd_Mounted(t *testing.T) {
	parent, _, _ := newUpdateCmdForTest(t, "v0.1.0", "default")
	got, _, err := parent.Find([]string{"check"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "check", got.Name())
}

// TestCheckCmd_FlagsRegistered asserts `--pre-releases` and `--version` are
// registered on the subcommand and `--force` / `--check` are NOT.
func TestCheckCmd_FlagsRegistered(t *testing.T) {
	parent, _, _ := newUpdateCmdForTest(t, "v0.1.0", "default")
	check := findCheckSubcommand(t, parent)

	for _, name := range []string{"pre-releases", "version"} {
		f := check.Flags().Lookup(name)
		require.NotNil(t, f, "flag --%s should be registered on `update check`", name)
	}
	for _, name := range []string{"force", "check"} {
		f := check.Flags().Lookup(name)
		assert.Nil(t, f, "flag --%s must NOT be registered on `update check`", name)
	}
}

// TestCheckCmd_UnknownForceFlag mirrors the manual smoke check from task-006:
// passing `--force` to the subcommand surfaces cobra's "unknown flag" error.
func TestCheckCmd_UnknownForceFlag(t *testing.T) {
	parent, _, _ := newUpdateCmdForTest(t, "v0.1.0", "default")
	parent.SetArgs([]string{"check", "--force"})
	err := parent.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag")
	assert.Contains(t, err.Error(), "force")
}

// TestCheckCmd_DispatchesToRunUpdate drives the subcommand via cobra.SetArgs
// and asserts RunE plumbs through to runUpdate with check=true: swap is never
// invoked, JSON sets check:true / updated:false, and the error returned for
// "newer available" is a clierr.NewUsageError carrying the comparison.
func TestCheckCmd_DispatchesToRunUpdate(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		return &Release{TagName: "v0.2.0"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	swapCalled := false
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string) error {
		swapCalled = true
		return nil
	})

	parent, out, _ := newUpdateCmdForTest(t, "v0.1.0", "json")
	parent.SetArgs([]string{"check"})

	err := parent.Execute()
	require.Error(t, err, "newer available must surface a usage error so exit code is non-zero")
	assert.Contains(t, err.Error(), "newer version is available")
	assert.False(t, swapCalled, "`update check` must never invoke swap")

	var doc struct {
		Current string `json:"current"`
		Latest  string `json:"latest"`
		Updated bool   `json:"updated"`
		Check   bool   `json:"check"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc), "stdout must be valid JSON: %q", out.String())
	assert.Equal(t, "v0.1.0", doc.Current)
	assert.Equal(t, "v0.2.0", doc.Latest)
	assert.False(t, doc.Updated, "`update check` must never set updated=true")
	assert.True(t, doc.Check, "`update check` must set check=true in JSON output")
}

// TestCheckCmd_PreReleasesFlagPropagates asserts `--pre-releases` reaches
// resolveTarget so latestFn is called with preReleases=true.
func TestCheckCmd_PreReleasesFlagPropagates(t *testing.T) {
	preReleasesSeen := false
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		preReleasesSeen = preReleases
		return &Release{TagName: "v0.2.0-alpha.1"}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string) error {
		t.Fatal("swap must not run on `update check`")
		return nil
	})

	parent, _, _ := newUpdateCmdForTest(t, "v0.1.0", "default")
	parent.SetArgs([]string{"check", "--pre-releases"})
	err := parent.Execute()
	require.Error(t, err, "newer pre-release available — usage error expected")
	assert.True(t, preReleasesSeen, "--pre-releases must propagate from the subcommand into latestFn")
}

// TestCheckCmd_VersionFlagPropagates asserts `--version <tag>` reaches
// resolveTarget so getByTagFn is called with the supplied tag (and latestFn is
// NOT called).
func TestCheckCmd_VersionFlagPropagates(t *testing.T) {
	withLatest(t, func(ctx context.Context, preReleases bool) (*Release, error) {
		t.Fatal("latestFn must not be called when --version is supplied")
		return nil, nil
	})
	gotTag := ""
	withGetByTag(t, func(ctx context.Context, tag string) (*Release, error) {
		gotTag = tag
		return &Release{TagName: tag}, nil
	})
	withDetect(t, func() (InstallMethod, string, error) {
		return InstallMethodBinary, "/tmp/neo4j-cli", nil
	})
	withSwap(t, func(ctx context.Context, urls AssetURLs, currentBinaryPath string) error {
		t.Fatal("swap must not run on `update check`")
		return nil
	})

	parent, _, _ := newUpdateCmdForTest(t, "v0.1.0-alpha.9", "default")
	parent.SetArgs([]string{"check", "--version", "v0.1.0-alpha.10"})
	err := parent.Execute()
	require.Error(t, err, "newer tagged version — usage error expected")
	assert.Equal(t, "v0.1.0-alpha.10", gotTag, "--version must propagate from the subcommand into getByTagFn")
}
