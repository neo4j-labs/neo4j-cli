// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package update

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withExecutable swaps the package-level executableFn seam to return the
// supplied path (and a nil error). t.Cleanup restores the previous value.
func withExecutable(t *testing.T, path string) {
	t.Helper()
	prev := executableFn
	executableFn = func() (string, error) { return path, nil }
	t.Cleanup(func() { executableFn = prev })
}

// withHomeDir swaps homeDirFn so pipx/uv expansion is hermetic.
func withHomeDir(t *testing.T, home string) {
	t.Helper()
	prev := homeDirFn
	homeDirFn = func() (string, error) { return home, nil }
	t.Cleanup(func() { homeDirFn = prev })
}

// makeFakeBinary writes an empty file at the given path (creating parents)
// and returns the absolute path. Used by Detect tests where EvalSymlinks
// requires the target to actually exist.
func makeFakeBinary(t *testing.T, p string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	f, err := os.Create(p)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	abs, err := filepath.Abs(p)
	require.NoError(t, err)
	return abs
}

func TestClassify_TableDriven(t *testing.T) {
	// classify operates on a pre-resolved absolute path string. We feed
	// fully-qualified, OS-native paths so Windows runs match too.
	home := filepath.FromSlash("/home/user")
	withHomeDir(t, home)

	cases := []struct {
		name string
		path string
		want InstallMethod
	}{
		{
			name: "homebrew apple silicon",
			path: filepath.FromSlash("/opt/homebrew/bin/neo4j-cli"),
			want: InstallMethodHomebrew,
		},
		{
			name: "homebrew cellar",
			path: filepath.FromSlash("/usr/local/Cellar/neo4j-cli/0.1.0/bin/neo4j-cli"),
			want: InstallMethodHomebrew,
		},
		{
			name: "linuxbrew",
			path: filepath.FromSlash("/home/linuxbrew/.linuxbrew/bin/neo4j-cli"),
			want: InstallMethodHomebrew,
		},
		{
			name: "npm global",
			path: filepath.FromSlash("/usr/lib/node_modules/@neo4j-labs/cli/bin/neo4j-cli"),
			want: InstallMethodNpm,
		},
		{
			name: "npm user-prefix",
			path: filepath.FromSlash("/home/user/.npm-global/lib/node_modules/@neo4j-labs/cli/dist/neo4j-cli"),
			want: InstallMethodNpm,
		},
		{
			name: "pipx venv",
			path: filepath.Join(home, ".local", "pipx", "venvs", "neo4j-cli", "bin", "neo4j-cli"),
			want: InstallMethodPipx,
		},
		{
			name: "pipx share",
			path: filepath.Join(home, ".local", "share", "pipx", "venvs", "neo4j-cli", "bin", "neo4j-cli"),
			want: InstallMethodPipx,
		},
		{
			name: "uv tool",
			path: filepath.Join(home, ".local", "share", "uv", "tools", "neo4j-cli", "bin", "neo4j-cli"),
			want: InstallMethodUv,
		},
		{
			name: "plain binary",
			path: filepath.FromSlash("/usr/local/bin/neo4j-cli"),
			want: InstallMethodBinary,
		},
		{
			name: "tmp",
			path: filepath.FromSlash("/tmp/n4j-test"),
			want: InstallMethodBinary,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.path)
			assert.Equal(t, tc.want, got, "path=%s", tc.path)
		})
	}
}

func TestClassify_NoHomeMatchesOnlyAbsolutePrefixes(t *testing.T) {
	// homeDirFn returning an error must NOT crash — pipx/uv simply skip.
	prev := homeDirFn
	homeDirFn = func() (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { homeDirFn = prev })

	got := classify(filepath.FromSlash("/opt/homebrew/bin/neo4j-cli"))
	assert.Equal(t, InstallMethodHomebrew, got)

	// A path that would have matched pipx falls through to "binary" when
	// HOME is unavailable.
	got = classify(filepath.FromSlash("/home/user/.local/pipx/venvs/neo4j-cli/bin/neo4j-cli"))
	assert.Equal(t, InstallMethodBinary, got)
}

func TestDetect_SymlinkResolvesIntoPipxVenv(t *testing.T) {
	// Symlinks aren't a thing on plain Windows runs in CI; skip there.
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin / dev mode on Windows; logic covered by classify tests")
	}

	// Resolve t.TempDir() through EvalSymlinks so the home prefix matches
	// the resolved binary path on macOS (where /var → /private/var).
	tmpRaw := t.TempDir()
	tmp, err := filepath.EvalSymlinks(tmpRaw)
	require.NoError(t, err)
	withHomeDir(t, tmp)

	// Real binary inside the pipx venv.
	pipxBin := filepath.Join(tmp, ".local", "pipx", "venvs", "neo4j-cli", "bin", "neo4j-cli")
	makeFakeBinary(t, pipxBin)

	// Symlink at ~/.local/bin/neo4j-cli pointing into the pipx venv.
	linkDir := filepath.Join(tmp, ".local", "bin")
	require.NoError(t, os.MkdirAll(linkDir, 0o755))
	link := filepath.Join(linkDir, "neo4j-cli")
	require.NoError(t, os.Symlink(pipxBin, link))

	withExecutable(t, link)

	method, resolved, err := Detect()
	require.NoError(t, err)
	assert.Equal(t, InstallMethodPipx, method)
	// EvalSymlinks may canonicalise /private/var on macOS — assert via
	// suffix rather than exact equality.
	assert.True(t, strings.HasSuffix(resolved, filepath.Join("venvs", "neo4j-cli", "bin", "neo4j-cli")),
		"resolved path %q should end at the pipx venv binary", resolved)
}

func TestDetect_PlainBinary(t *testing.T) {
	tmp := t.TempDir()
	withHomeDir(t, tmp)

	bin := filepath.Join(tmp, "neo4j-cli")
	makeFakeBinary(t, bin)
	withExecutable(t, bin)

	method, resolved, err := Detect()
	require.NoError(t, err)
	assert.Equal(t, InstallMethodBinary, method)
	assert.NotEmpty(t, resolved)
}

func TestDetect_ExecutableFnError(t *testing.T) {
	prev := executableFn
	executableFn = func() (string, error) { return "", os.ErrPermission }
	t.Cleanup(func() { executableFn = prev })

	method, _, err := Detect()
	require.Error(t, err)
	// Falls back to "binary" so RunE doesn't block self-update on a
	// detection failure.
	assert.Equal(t, InstallMethodBinary, method)
}

func TestDetect_EvalSymlinksFailsFallsBackToRawPath(t *testing.T) {
	// Point at a path that does not exist — EvalSymlinks fails. classify
	// still runs against the raw exe path, so a synthetic
	// /opt/homebrew/... path matches Homebrew.
	if runtime.GOOS == "windows" {
		t.Skip("path matching uses Unix prefixes; covered hermetically on linux/darwin")
	}
	withExecutable(t, "/opt/homebrew/bin/does-not-exist-neo4j-cli")
	withHomeDir(t, t.TempDir())

	method, _, err := Detect()
	require.NoError(t, err)
	assert.Equal(t, InstallMethodHomebrew, method)
}

func TestHint_HomebrewIncludesAllThreeBlocks(t *testing.T) {
	h := Hint(InstallMethodHomebrew)
	require.NotEmpty(t, h)

	// Block 1: pkg-mgr upgrade command.
	assert.Contains(t, h, "brew upgrade neo4j-cli")
	// Block 2: install-script command.
	assert.Contains(t, h, installScriptCmd)
	assert.Contains(t, h, "https://neo4j.sh/install.sh")
	// Block 3: optional uninstall + PATH-order rationale.
	assert.Contains(t, h, "brew uninstall neo4j-cli")
	assert.Contains(t, h, "optional")
	assert.Contains(t, h, "PATH")
}

func TestHint_AllChannelsHaveExpectedCommands(t *testing.T) {
	cases := []struct {
		method       InstallMethod
		wantUpgrades []string
		wantUninst   string
	}{
		{
			method:       InstallMethodHomebrew,
			wantUpgrades: []string{"brew upgrade neo4j-cli"},
			wantUninst:   "brew uninstall neo4j-cli",
		},
		{
			// npm channel surfaces all three node-pkg-mgrs because any of
			// them could have produced the @neo4j-labs/cli layout.
			method: InstallMethodNpm,
			wantUpgrades: []string{
				"npm i -g @neo4j-labs/cli@latest",
				"pnpm add -g @neo4j-labs/cli@latest",
				"yarn global add @neo4j-labs/cli@latest",
			},
			wantUninst: "npm uninstall -g @neo4j-labs/cli",
		},
		{
			method:       InstallMethodPipx,
			wantUpgrades: []string{"pipx upgrade neo4j-cli"},
			wantUninst:   "pipx uninstall neo4j-cli",
		},
		{
			method:       InstallMethodUv,
			wantUpgrades: []string{"uv tool upgrade neo4j-cli"},
			wantUninst:   "uv tool uninstall neo4j-cli",
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.method), func(t *testing.T) {
			h := Hint(tc.method)
			require.NotEmpty(t, h)
			for _, up := range tc.wantUpgrades {
				assert.Contains(t, h, up)
			}
			assert.Contains(t, h, tc.wantUninst)
			assert.Contains(t, h, installScriptCmd)
			assert.Contains(t, h, "optional")
		})
	}
}

func TestHint_BinaryReturnsEmpty(t *testing.T) {
	assert.Equal(t, "", Hint(InstallMethodBinary))
}

func TestHint_GoldenHomebrew(t *testing.T) {
	// One golden assertion to lock the rendered shape so future edits to
	// formatHint don't silently drift the user-facing message.
	want := "Installed via Homebrew. To upgrade in place, run:\n" +
		"  brew upgrade neo4j-cli\n" +
		"\n" +
		"To switch to a self-managed install (so 'neo4j-cli update' works directly):\n" +
		"  brew uninstall neo4j-cli   # optional — only needed if PATH still resolves the package-manager binary\n" +
		"  curl -sSfL https://neo4j.sh/install.sh | bash\n"
	assert.Equal(t, want, Hint(InstallMethodHomebrew))
}

// TestSeams_InstallMethod_Smoke makes sure the new seams compile and behave
// when not swapped — production fills with the real impls.
func TestSeams_InstallMethod_Smoke(t *testing.T) {
	require.NotNil(t, executableFn)
	require.NotNil(t, homeDirFn)
}
