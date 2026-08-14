// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package update

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/neo4j/cli/common/clicfg"
	commonskill "github.com/neo4j/cli/common/skill"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
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

// listReleasesStubbed records whether the current test installed its own
// listReleasesFn via withListReleases. The runWithOpts* helpers default the
// seam to an empty list so the ~48 pre-changelog tests never reach the network
// (REQ-NF-002), but that default must NOT clobber an explicit per-test stub —
// the helpers run after it.
var listReleasesStubbed bool

// withListReleases swaps the listReleasesFn seam.
func withListReleases(t *testing.T, fn func(ctx context.Context, preReleases bool) ([]Release, error)) {
	t.Helper()
	prev := listReleasesFn
	prevStubbed := listReleasesStubbed
	listReleasesFn = fn
	listReleasesStubbed = true
	t.Cleanup(func() {
		listReleasesFn = prev
		listReleasesStubbed = prevStubbed
	})
}

// defaultListReleases points the seam at an empty list unless the test already
// stubbed it. Called by the runWithOpts* helpers.
func defaultListReleases(t *testing.T) {
	t.Helper()
	if listReleasesStubbed {
		return
	}
	prev := listReleasesFn
	listReleasesFn = func(ctx context.Context, preReleases bool) ([]Release, error) { return nil, nil }
	t.Cleanup(func() { listReleasesFn = prev })
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

	defaultListReleases(t)

	err = runUpdate(context.Background(), cmd, cfg, opts)
	return out.String(), err
}

// runWithOptsSplit mirrors runWithOptsFormat but routes stdout and stderr to
// SEPARATE buffers so callers can assert which stream carries which payload
// (e.g. the --force pkg-mgr override warning must land on stderr only while
// JSON / plain-text update output lands on stdout). Used by the
// TestRunUpdate_Force_* tests below.
func runWithOptsSplit(t *testing.T, current string, opts runOpts, format string) (stdout, stderr string, err error) {
	t.Helper()
	cfgJSON := `{"format":"` + format + `"}`
	tfs, terr := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, terr)
	cfg := clicfg.NewConfig(tfs, current, clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	cmd.SetOut(stdoutBuf)
	cmd.SetErr(stderrBuf)

	defaultListReleases(t)

	err = runUpdate(context.Background(), cmd, cfg, opts)
	return stdoutBuf.String(), stderrBuf.String(), err
}

// runWithOptsParented mirrors runWithOptsFormat but mounts the `update` cobra
// command under a synthetic `neo4j-cli` root so `cmd.CommandPath()` returns
// the production-realistic `neo4j-cli update` rather than the bare `update`
// you'd get from an unparented sub-cobra. Used by the sentinel-error hint
// tests because the hint relies on CommandPath().
func runWithOptsParented(t *testing.T, current string, opts runOpts) (string, error) {
	t.Helper()
	tfs, err := testfs.GetTestFs(`{"format":"default"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, current, clicfg.GlobalScope)

	root := &cobra.Command{Use: "neo4j-cli"}
	updateCmd := NewCmd(cfg, nil, "")
	root.AddCommand(updateCmd)

	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	updateCmd.SetOut(out)
	updateCmd.SetErr(out)

	defaultListReleases(t)

	err = runUpdate(context.Background(), updateCmd, cfg, opts)
	return out.String(), err
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

	defaultListReleases(t)

	err = runUpdate(context.Background(), cmd, cfg, opts)
	return out.String(), errOut.String(), err
}

// runWithSplitBuffers mirrors runWithOpts but binds stdout and stderr to
// separate buffers so tests can assert which stream a line lands on.
// Returns (stdout, stderr).
func runWithSplitBuffers(t *testing.T, current string, opts runOpts) (string, string) {
	t.Helper()
	tfs, err := testfs.GetTestFs(`{"format":"default"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(tfs, current, clicfg.GlobalScope)

	cmd := NewCmd(cfg, nil, "")
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)

	defaultListReleases(t)

	_ = runUpdate(context.Background(), cmd, cfg, opts)
	return outBuf.String(), errBuf.String()
}
