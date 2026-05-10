// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withSwapGoos swaps the swapGoosFn seam. Lets non-Windows runners exercise
// the windowsSwap rename-to-`.old` dance.
func withSwapGoos(t *testing.T, v string) {
	t.Helper()
	prev := swapGoosFn
	swapGoosFn = func() string { return v }
	t.Cleanup(func() { swapGoosFn = prev })
}

// withRename swaps the renameFn seam so tests can simulate a mid-swap
// failure and assert restore-on-error behaviour.
func withRename(t *testing.T, fn func(oldpath, newpath string) error) {
	t.Helper()
	prev := renameFn
	renameFn = fn
	t.Cleanup(func() { renameFn = prev })
}

// withHttpDo swaps httpDoFn for the duration of the test. Reused across the
// swap tests to drive both archive and checksum downloads through a single
// httptest server.
func withHttpDo(t *testing.T, fn func(req *http.Request) (*http.Response, error)) {
	t.Helper()
	prev := httpDoFn
	httpDoFn = fn
	t.Cleanup(func() { httpDoFn = prev })
}

// withAllowedHost adds host to the allowedDownloadHosts allowlist for the
// duration of the test (so we can use 127.0.0.1 / httptest hosts without
// disabling the production allowlist).
func withAllowedHost(t *testing.T, host string) {
	t.Helper()
	if _, ok := allowedDownloadHosts[host]; ok {
		return
	}
	allowedDownloadHosts[host] = struct{}{}
	t.Cleanup(func() { delete(allowedDownloadHosts, host) })
}

// withRequireHTTPS toggles the requireHTTPS guard. httptest only speaks
// plain HTTP so swap tests that wire a real httptest server flip it off;
// the explicit non-https rejection test sets it back to true to assert the
// production path.
func withRequireHTTPS(t *testing.T, v bool) {
	t.Helper()
	prev := requireHTTPS
	requireHTTPS = v
	t.Cleanup(func() { requireHTTPS = prev })
}

// withGeteuid swaps the geteuidFn seam so tests can simulate running as
// root (uid 0) or as a regular user without actually being root.
func withGeteuid(t *testing.T, fn func() int) {
	t.Helper()
	prev := geteuidFn
	geteuidFn = fn
	t.Cleanup(func() { geteuidFn = prev })
}

// withLookPath swaps the lookPathFn seam so tests can simulate the presence
// or absence of an executable on PATH without depending on the host PATH.
func withLookPath(t *testing.T, fn func(file string) (string, error)) {
	t.Helper()
	prev := lookPathFn
	lookPathFn = fn
	t.Cleanup(func() { lookPathFn = prev })
}

// withRunCommand swaps the runCommandFn seam so tests can capture the *exec.Cmd
// argv that would be executed and return a synthetic exit code without forking
// a real process.
func withRunCommand(t *testing.T, fn func(cmd *exec.Cmd) error) {
	t.Helper()
	prev := runCommandFn
	runCommandFn = fn
	t.Cleanup(func() { runCommandFn = prev })
}

// withDirWritable swaps the dirWritableFn seam so tests can drive planSwap
// branches deterministically without relying on chmod (which is unreliable
// under some CI sandboxes and is a no-op for the calling uid on Windows).
func withDirWritable(t *testing.T, fn func(dir string) (bool, error)) {
	t.Helper()
	prev := dirWritableFn
	dirWritableFn = fn
	t.Cleanup(func() { dirWritableFn = prev })
}

// withTempDir swaps the tempDirFn seam so the elevation-path tmpNew file
// lands under a t.TempDir() rather than the host's real temp dir.
func withTempDir(t *testing.T, fn func() string) {
	t.Helper()
	prev := tempDirFn
	tempDirFn = fn
	t.Cleanup(func() { tempDirFn = prev })
}

// withStdinIsTTY swaps the stdinIsTTYFn seam so tests can simulate an
// interactive (or non-interactive) shell without manipulating the real
// stdin file descriptor.
func withStdinIsTTY(t *testing.T, v bool) {
	t.Helper()
	prev := stdinIsTTYFn
	stdinIsTTYFn = func() bool { return v }
	t.Cleanup(func() { stdinIsTTYFn = prev })
}

// makeTarGz builds an in-memory tar.gz with the given entries. payload of
// nil for a TypeReg entry uses an empty body.
type tarEntry struct {
	name     string
	typeflag byte
	mode     int64
	body     []byte
	linkname string
}

func makeTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     e.mode,
			Size:     int64(len(e.body)),
			Typeflag: e.typeflag,
			Linkname: e.linkname,
		}
		if e.typeflag != tar.TypeReg && e.typeflag != tar.TypeRegA { //nolint:staticcheck
			hdr.Size = 0
		}
		require.NoError(t, tw.WriteHeader(hdr))
		if hdr.Size > 0 {
			_, err := tw.Write(e.body)
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// makeZip builds an in-memory zip archive with the given entries.
type zipEntry struct {
	name string
	mode os.FileMode
	body []byte
}

func makeZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		fh := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			fh.SetMode(e.mode)
		}
		w, err := zw.CreateHeader(fh)
		require.NoError(t, err)
		_, err = w.Write(e.body)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestSwap_HappyPath_LinuxTarGz(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("happy-path linux/darwin tar.gz swap is exercised on non-windows hosts only")
	}
	withSwapGoos(t, "linux")

	tmpDir := t.TempDir()
	currentBinary := filepath.Join(tmpDir, "neo4j-cli")
	require.NoError(t, os.WriteFile(currentBinary, []byte("OLD-BINARY"), 0o755))

	archive := makeTarGz(t, []tarEntry{
		{name: "neo4j-cli", typeflag: tar.TypeReg, mode: 0o755, body: []byte("NEW-BINARY-BODY")},
	})
	archiveName := "neo4j-cli_0.1.0_Linux_x86_64.tar.gz"
	archiveURL := "https://swap-test.local/" + archiveName
	checksumURL := "https://swap-test.local/neo4j-cli_0.1.0_checksums.txt"

	withAllowedHost(t, "swap-test.local")
	withHttpDo(t, func(req *http.Request) (*http.Response, error) {
		body := archive
		if strings.HasSuffix(req.URL.Path, "_checksums.txt") {
			sum := sha256.Sum256(archive)
			body = []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveName))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	err := Swap(context.Background(), AssetURLs{Archive: archiveURL, Checksum: checksumURL}, currentBinary)
	require.NoError(t, err)

	// New binary in place
	got, err := os.ReadFile(currentBinary)
	require.NoError(t, err)
	assert.Equal(t, "NEW-BINARY-BODY", string(got))

	// .new temp file is gone
	_, statErr := os.Stat(currentBinary + ".new")
	assert.True(t, os.IsNotExist(statErr), "expected %s.new to be removed", currentBinary)

	// Mode is 0755 (Unix only — Windows will report 0666 from chmod due to
	// the platform's lack of execute bits)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(currentBinary)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	}
}

func TestSwap_TamperedChecksum_AbortsBeforeSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("checksum tamper test runs on non-windows; same code path")
	}
	withSwapGoos(t, "linux")

	tmpDir := t.TempDir()
	currentBinary := filepath.Join(tmpDir, "neo4j-cli")
	original := []byte("ORIGINAL-CONTENT")
	require.NoError(t, os.WriteFile(currentBinary, original, 0o755))

	archive := makeTarGz(t, []tarEntry{
		{name: "neo4j-cli", typeflag: tar.TypeReg, mode: 0o755, body: []byte("MALICIOUS-PAYLOAD")},
	})
	// Flip the trailing byte of the real SHA256 to produce a mismatch.
	realSum := sha256.Sum256(archive)
	bogus := make([]byte, len(realSum))
	copy(bogus, realSum[:])
	bogus[len(bogus)-1] ^= 0x01
	bogusHex := hex.EncodeToString(bogus)

	archiveName := "neo4j-cli_0.1.0_Linux_x86_64.tar.gz"
	withHttpDo(t, func(req *http.Request) (*http.Response, error) {
		body := archive
		if strings.HasSuffix(req.URL.Path, archiveName) {
			body = archive
		}
		if strings.HasSuffix(req.URL.Path, "_checksums.txt") {
			body = []byte(fmt.Sprintf("%s  %s\n", bogusHex, archiveName))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	withAllowedHost(t, "swap-test.local")

	urls := AssetURLs{
		Archive:  "https://swap-test.local/" + archiveName,
		Checksum: "https://swap-test.local/neo4j-cli_0.1.0_checksums.txt",
	}
	err := Swap(context.Background(), urls, currentBinary)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")

	// Original is untouched
	got, err := os.ReadFile(currentBinary)
	require.NoError(t, err)
	assert.Equal(t, string(original), string(got))

	// No .new lingering, no .old lingering
	_, statErr := os.Stat(currentBinary + ".new")
	assert.True(t, os.IsNotExist(statErr))
	_, statErr = os.Stat(currentBinary + ".old")
	assert.True(t, os.IsNotExist(statErr))
}

func TestExtractTarGzEntry_RejectsTraversal(t *testing.T) {
	withSwapGoos(t, "linux")
	tmpDir := t.TempDir()

	cases := []struct {
		name      string
		entryName string
	}{
		{"dot-dot-segment", "../escape"},
		{"deep-traversal", "subdir/../../escape"},
		{"absolute-unix", "/etc/passwd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive := makeTarGz(t, []tarEntry{
				{name: tc.entryName, typeflag: tar.TypeReg, mode: 0o644, body: []byte("X")},
			})
			err := extractTarGzEntry(archive, "neo4j-cli", filepath.Join(tmpDir, "neo4j-cli.new"), tmpDir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "reject")
			// Ensure no escapee written
			parent := filepath.Dir(tmpDir)
			_, statErr := os.Stat(filepath.Join(parent, "escape"))
			assert.True(t, os.IsNotExist(statErr), "no file should be written outside tmpDir")
		})
	}
}

func TestExtractTarGzEntry_RejectsSymlinkHardlinkDevice(t *testing.T) {
	withSwapGoos(t, "linux")
	tmpDir := t.TempDir()

	cases := []struct {
		name     string
		typeflag byte
	}{
		{"symlink", tar.TypeSymlink},
		{"hardlink", tar.TypeLink},
		{"char-device", tar.TypeChar},
		{"block-device", tar.TypeBlock},
		{"fifo", tar.TypeFifo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			archive := makeTarGz(t, []tarEntry{
				// Put the bad entry first AND name it the binary so the
				// loop's binary-name match has to confront the type before
				// it would otherwise return; combined with a regular file
				// after, this asserts we reject before falling through.
				{name: "neo4j-cli", typeflag: tc.typeflag, linkname: "elsewhere"},
			})
			err := extractTarGzEntry(archive, "neo4j-cli", filepath.Join(tmpDir, "neo4j-cli.new"), tmpDir)
			require.Error(t, err)
			assert.Contains(t, strings.ToLower(err.Error()), "reject")
		})
	}
}

func TestExtractZipEntry_RejectsTraversal(t *testing.T) {
	withSwapGoos(t, "windows")
	tmpDir := t.TempDir()

	archive := makeZip(t, []zipEntry{
		{name: "../escape.exe", mode: 0o644, body: []byte("X")},
	})
	err := extractZipEntry(archive, "neo4j-cli.exe", filepath.Join(tmpDir, "neo4j-cli.exe.new"), tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reject")
}

func TestExtractZipEntry_RejectsSymlink(t *testing.T) {
	withSwapGoos(t, "windows")
	tmpDir := t.TempDir()

	archive := makeZip(t, []zipEntry{
		{name: "neo4j-cli.exe", mode: os.ModeSymlink | 0o644, body: []byte("payload")},
	})
	err := extractZipEntry(archive, "neo4j-cli.exe", filepath.Join(tmpDir, "neo4j-cli.exe.new"), tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

func TestSwap_WindowsRenameDance(t *testing.T) {
	// Exercise the Windows rename-to-`.old` code path on any host via the
	// swapGoosFn test seam.
	withSwapGoos(t, "windows")

	tmpDir := t.TempDir()
	currentBinary := filepath.Join(tmpDir, "neo4j-cli.exe")
	require.NoError(t, os.WriteFile(currentBinary, []byte("ORIGINAL"), 0o755))

	// Pre-existing .old must be removed at the start of the swap.
	staleOld := currentBinary + ".old"
	require.NoError(t, os.WriteFile(staleOld, []byte("STALE-OLD"), 0o644))

	archive := makeZip(t, []zipEntry{
		{name: "neo4j-cli.exe", mode: 0o755, body: []byte("NEW-WIN-BINARY")},
	})

	archiveName := "neo4j-cli_0.1.0_Windows_x86_64.zip"
	withHttpDo(t, func(req *http.Request) (*http.Response, error) {
		body := archive
		if strings.HasSuffix(req.URL.Path, "_checksums.txt") {
			sum := sha256.Sum256(archive)
			body = []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveName))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	withAllowedHost(t, "swap-test.local")

	urls := AssetURLs{
		Archive:  "https://swap-test.local/" + archiveName,
		Checksum: "https://swap-test.local/neo4j-cli_0.1.0_checksums.txt",
	}
	err := Swap(context.Background(), urls, currentBinary)
	require.NoError(t, err)

	// New binary lives at the original path
	got, err := os.ReadFile(currentBinary)
	require.NoError(t, err)
	assert.Equal(t, "NEW-WIN-BINARY", string(got))

	// `<current>.old` now contains the original
	oldBytes, err := os.ReadFile(staleOld)
	require.NoError(t, err)
	assert.Equal(t, "ORIGINAL", string(oldBytes))

	// `<current>.new` is gone (consumed by the rename)
	_, statErr := os.Stat(currentBinary + ".new")
	assert.True(t, os.IsNotExist(statErr))
}

func TestSwap_WindowsRestoreOnError(t *testing.T) {
	withSwapGoos(t, "windows")

	tmpDir := t.TempDir()
	currentBinary := filepath.Join(tmpDir, "neo4j-cli.exe")
	require.NoError(t, os.WriteFile(currentBinary, []byte("ORIGINAL"), 0o755))

	archive := makeZip(t, []zipEntry{
		{name: "neo4j-cli.exe", mode: 0o755, body: []byte("NEW")},
	})
	archiveName := "pkg.zip"
	withHttpDo(t, func(req *http.Request) (*http.Response, error) {
		body := archive
		if strings.HasSuffix(req.URL.Path, "_checksums.txt") || strings.HasSuffix(req.URL.Path, "sums.txt") {
			sum := sha256.Sum256(archive)
			body = []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveName))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	withAllowedHost(t, "swap-test.local")

	// Inject a renameFn that succeeds for `current -> .old` (first call)
	// but fails for `.new -> current` (second call). The third call is
	// the restore: `.old -> current` — let it succeed.
	calls := 0
	withRename(t, func(oldPath, newPath string) error {
		calls++
		switch calls {
		case 1:
			// rename current -> .old
			return os.Rename(oldPath, newPath)
		case 2:
			// rename new -> current — simulate failure
			return fmt.Errorf("simulated rename failure")
		case 3:
			// restore .old -> current
			return os.Rename(oldPath, newPath)
		default:
			return os.Rename(oldPath, newPath)
		}
	})

	urls := AssetURLs{
		Archive:  "https://swap-test.local/pkg.zip",
		Checksum: "https://swap-test.local/sums.txt",
	}
	err := Swap(context.Background(), urls, currentBinary)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename new into place", "got: %v", err)
	assert.Contains(t, err.Error(), "original restored", "got: %v", err)

	// Original content is back at the expected path.
	got, err := os.ReadFile(currentBinary)
	require.NoError(t, err)
	assert.Equal(t, "ORIGINAL", string(got))
}

func TestLookupChecksum(t *testing.T) {
	body := []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  archive-A.tar.gz\n" +
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  archive-B.tar.gz\n" +
		"# a comment line\n" +
		"\n" +
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc *binary-mode-archive.tar.gz\n")

	tests := []struct {
		name        string
		archiveName string
		want        string
		wantErr     bool
	}{
		{"first entry", "archive-A.tar.gz", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"second entry", "archive-B.tar.gz", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", false},
		{"binary-mode-prefix", "binary-mode-archive.tar.gz", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", false},
		{"missing", "not-there.tar.gz", "", true},
		{"empty name", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := lookupChecksum(body, tc.archiveName)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestLookupChecksum_RejectsMalformedHex(t *testing.T) {
	body := []byte("ZZZZ  archive.tar.gz\n")
	_, err := lookupChecksum(body, "archive.tar.gz")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed hex")
}

func TestAssertAllowedHost_RejectsNonHTTPS(t *testing.T) {
	err := assertAllowedHost("http://github.com/foo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-https")
}

func TestAssertAllowedHost_RejectsForeignHost(t *testing.T) {
	err := assertAllowedHost("https://evil.example.com/foo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allowlist")
}

func TestAssertAllowedHost_AcceptsGitHub(t *testing.T) {
	assert.NoError(t, assertAllowedHost("https://github.com/neo4j-labs/neo4j-cli/releases/download/v0.1.0/foo.tar.gz"))
	assert.NoError(t, assertAllowedHost("https://objects.githubusercontent.com/foo"))
	assert.NoError(t, assertAllowedHost("https://api.github.com/repos/foo"))
}

func TestDownloadCapped_BodyExceedsCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Send 100 bytes when we'll cap at 10
		_, _ = w.Write(bytes.Repeat([]byte("X"), 100))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	withAllowedHost(t, u.Hostname())
	withRequireHTTPS(t, false)
	withHttpDo(t, func(req *http.Request) (*http.Response, error) {
		return (&http.Client{}).Do(req)
	})

	_, err := downloadCapped(context.Background(), srv.URL, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds cap")
}

func TestDownloadCapped_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	withAllowedHost(t, u.Hostname())
	withRequireHTTPS(t, false)
	withHttpDo(t, func(req *http.Request) (*http.Response, error) {
		return (&http.Client{}).Do(req)
	})

	_, err := downloadCapped(context.Background(), srv.URL, 1024)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestExtractBinary_NotFound(t *testing.T) {
	withSwapGoos(t, "linux")
	tmpDir := t.TempDir()
	archive := makeTarGz(t, []tarEntry{
		{name: "some-other-binary", typeflag: tar.TypeReg, mode: 0o755, body: []byte("X")},
	})
	err := extractTarGzEntry(archive, "neo4j-cli", filepath.Join(tmpDir, "neo4j-cli.new"), tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestExtractBinary_NestedDir_AcceptsByBasename(t *testing.T) {
	withSwapGoos(t, "linux")
	tmpDir := t.TempDir()
	archive := makeTarGz(t, []tarEntry{
		{name: "neo4j-cli_0.1.0/neo4j-cli", typeflag: tar.TypeReg, mode: 0o755, body: []byte("NESTED-PAYLOAD")},
	})
	dest := filepath.Join(tmpDir, "neo4j-cli.new")
	err := extractTarGzEntry(archive, "neo4j-cli", dest, tmpDir)
	require.NoError(t, err)
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "NESTED-PAYLOAD", string(got))
}

func TestRedactURL_StripsQuery(t *testing.T) {
	got := redactURL("https://github.com/foo?token=secret&x-amz-signature=abc")
	assert.Equal(t, "https://github.com/foo", got)
}

func TestDirWritable_Writable(t *testing.T) {
	dir := t.TempDir()
	ok, err := dirWritable(dir)
	require.NoError(t, err)
	assert.True(t, ok, "freshly created t.TempDir() should be writable")

	// Probe file must have been cleaned up — no residue allowed.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "dirWritable must clean up its probe file")
}

func TestDirWritable_NotWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		// On Windows, file mode bits do not produce a permission-denied path
		// in the same way; the elevation branch is exercised via the
		// dirWritableFn seam in higher-level tests rather than by real chmod.
		t.Skip("chmod-based unwritable probe is unix-only")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses unix file mode permissions; cannot exercise unwritable branch")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	ok, err := dirWritable(dir)
	require.NoError(t, err, "EACCES must be reported as (false, nil), not as an error")
	assert.False(t, ok)
}

func TestErrSudoUnavailable_ImplementsErrorAndCarriesDir(t *testing.T) {
	want := "/usr/local/bin"
	var err error = &errSudoUnavailable{dir: want}
	assert.Contains(t, err.Error(), want)

	var target *errSudoUnavailable
	require.True(t, errors.As(err, &target))
	assert.Equal(t, want, target.Dir())
}

func TestErrPermissionWindows_ImplementsErrorAndCarriesDir(t *testing.T) {
	want := `C:\Program Files\neo4j-cli`
	var err error = &errPermissionWindows{dir: want}
	assert.Contains(t, err.Error(), want)

	var target *errPermissionWindows
	require.True(t, errors.As(err, &target))
	assert.Equal(t, want, target.Dir())
}

func TestSeams_ProductionImplsCallable(t *testing.T) {
	// Smoke-test that each seam's production impl is callable. Behaviour is
	// platform-dependent (we don't assert specific values) — the test exists
	// to catch a future refactor that accidentally swaps in a nil function
	// or a signature mismatch that survives the compiler.
	_ = geteuidFn()
	_, _ = lookPathFn("a-binary-that-almost-certainly-does-not-exist-on-this-host")
	_ = stdinIsTTYFn()
	_ = tempDirFn()
	_, _ = dirWritableFn(t.TempDir())
}

func TestPlanSwap_WritableSkipsAllOtherSeams(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "neo4j-cli")

	// dirWritable returns true → planSwap MUST NOT consult any other seam.
	// Wire each one to a fail-the-test fake so accidental calls explode.
	withDirWritable(t, func(probed string) (bool, error) {
		assert.Equal(t, dir, probed)
		return true, nil
	})
	withGeteuid(t, func() int { t.Fatal("geteuidFn must not be called on writable branch"); return -1 })
	withLookPath(t, func(string) (string, error) {
		t.Fatal("lookPathFn must not be called on writable branch")
		return "", nil
	})
	prev := stdinIsTTYFn
	stdinIsTTYFn = func() bool {
		t.Fatal("stdinIsTTYFn must not be called on writable branch")
		return false
	}
	t.Cleanup(func() { stdinIsTTYFn = prev })
	withTempDir(t, func() string {
		t.Fatal("tempDirFn must not be called on writable branch")
		return ""
	})

	plan, err := planSwap(abs)
	require.NoError(t, err)
	assert.False(t, plan.elevate)
	assert.Equal(t, dir, plan.tmpDir)
}

func TestPlanSwap_NonWritableWindowsReturnsErrPermissionWindows(t *testing.T) {
	withSwapGoos(t, "windows")
	withDirWritable(t, func(string) (bool, error) { return false, nil })
	// Sudo / TTY seams must not gate the windows decision — wire them to
	// fail-the-test fakes to assert that.
	withGeteuid(t, func() int { t.Fatal("geteuidFn must not be called on windows branch"); return -1 })
	withLookPath(t, func(string) (string, error) {
		t.Fatal("lookPathFn must not be called on windows branch")
		return "", nil
	})
	withStdinIsTTY(t, true)

	dir := `C:\Program Files\neo4j-cli`
	abs := filepath.Join(dir, "neo4j-cli.exe")

	_, err := planSwap(abs)
	require.Error(t, err)
	var target *errPermissionWindows
	require.True(t, errors.As(err, &target))
	assert.Equal(t, filepath.Dir(abs), target.Dir())
}

func TestPlanSwap_NonWritableAlreadyRootSurfacesPermissionError(t *testing.T) {
	withSwapGoos(t, "linux")
	withDirWritable(t, func(string) (bool, error) { return false, nil })
	withGeteuid(t, func() int { return 0 })
	// sudo / install / tty seams must not be consulted once we know we're
	// already root.
	withLookPath(t, func(string) (string, error) {
		t.Fatal("lookPathFn must not be called on already-root branch")
		return "", nil
	})

	abs := filepath.Join(t.TempDir(), "neo4j-cli")
	_, err := planSwap(abs)
	require.Error(t, err)

	// MUST NOT be the sudo-unavailable sentinel — runUpdate's hint would be
	// misleading ("re-run with sudo" when you're already root).
	var sudoTarget *errSudoUnavailable
	assert.False(t, errors.As(err, &sudoTarget), "already-root must NOT surface as errSudoUnavailable: %v", err)
	var winTarget *errPermissionWindows
	assert.False(t, errors.As(err, &winTarget))
}

func TestPlanSwap_NonWritableSudoMissingReturnsErrSudoUnavailable(t *testing.T) {
	withSwapGoos(t, "linux")
	withDirWritable(t, func(string) (bool, error) { return false, nil })
	withGeteuid(t, func() int { return 1000 })
	withLookPath(t, func(file string) (string, error) {
		// sudo missing; install lookup MAY happen depending on order — short-
		// circuit either way.
		return "", exec.ErrNotFound
	})
	withStdinIsTTY(t, true)

	dir := t.TempDir()
	abs := filepath.Join(dir, "neo4j-cli")
	_, err := planSwap(abs)
	require.Error(t, err)
	var target *errSudoUnavailable
	require.True(t, errors.As(err, &target))
	assert.Equal(t, dir, target.Dir())
}

func TestPlanSwap_NonWritableInstallMissingReturnsErrSudoUnavailable(t *testing.T) {
	withSwapGoos(t, "linux")
	withDirWritable(t, func(string) (bool, error) { return false, nil })
	withGeteuid(t, func() int { return 1000 })
	withLookPath(t, func(file string) (string, error) {
		if file == "sudo" {
			return "/usr/bin/sudo", nil
		}
		return "", exec.ErrNotFound
	})
	withStdinIsTTY(t, true)

	dir := t.TempDir()
	abs := filepath.Join(dir, "neo4j-cli")
	_, err := planSwap(abs)
	require.Error(t, err)
	var target *errSudoUnavailable
	require.True(t, errors.As(err, &target))
	assert.Equal(t, dir, target.Dir())
}

func TestPlanSwap_NonWritableNonTTYReturnsErrSudoUnavailable(t *testing.T) {
	withSwapGoos(t, "linux")
	withDirWritable(t, func(string) (bool, error) { return false, nil })
	withGeteuid(t, func() int { return 1000 })
	withLookPath(t, func(file string) (string, error) {
		switch file {
		case "sudo":
			return "/usr/bin/sudo", nil
		case "install":
			return "/usr/bin/install", nil
		}
		return "", exec.ErrNotFound
	})
	withStdinIsTTY(t, false)

	dir := t.TempDir()
	abs := filepath.Join(dir, "neo4j-cli")
	_, err := planSwap(abs)
	require.Error(t, err)
	var target *errSudoUnavailable
	require.True(t, errors.As(err, &target))
	assert.Equal(t, dir, target.Dir())
}

func TestPlanSwap_NonWritableElevateHappyPath(t *testing.T) {
	withSwapGoos(t, "linux")
	withDirWritable(t, func(string) (bool, error) { return false, nil })
	withGeteuid(t, func() int { return 1000 })
	withLookPath(t, func(file string) (string, error) {
		switch file {
		case "sudo":
			return "/usr/bin/sudo", nil
		case "install":
			return "/usr/bin/install", nil
		}
		return "", exec.ErrNotFound
	})
	withStdinIsTTY(t, true)
	tmpStub := t.TempDir()
	withTempDir(t, func() string { return tmpStub })

	abs := filepath.Join(t.TempDir(), "neo4j-cli")
	plan, err := planSwap(abs)
	require.NoError(t, err)
	assert.True(t, plan.elevate)
	assert.Equal(t, tmpStub, plan.tmpDir)
}

func TestPlanSwap_DirWritableFnErrorPropagates(t *testing.T) {
	// dirWritableFn returning a non-EACCES/non-EROFS error must surface as
	// a fatal planSwap error (NOT one of the sentinels) — the caller has no
	// way to recover from a probe that hit an unexpected failure.
	withDirWritable(t, func(string) (bool, error) {
		return false, fmt.Errorf("disk on fire")
	})

	abs := filepath.Join(t.TempDir(), "neo4j-cli")
	_, err := planSwap(abs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk on fire")

	var sudoTarget *errSudoUnavailable
	assert.False(t, errors.As(err, &sudoTarget))
	var winTarget *errPermissionWindows
	assert.False(t, errors.As(err, &winTarget))
}

func TestWithFooHelpers_RestoreAfterTest(t *testing.T) {
	origGeteuid := geteuidFn
	origLookPath := lookPathFn
	origRunCmd := runCommandFn
	origDirWritable := dirWritableFn
	origTempDir := tempDirFn
	origStdinIsTTY := stdinIsTTYFn

	t.Run("swap-and-restore", func(t *testing.T) {
		withGeteuid(t, func() int { return 42 })
		withLookPath(t, func(string) (string, error) { return "/x", nil })
		withRunCommand(t, func(*exec.Cmd) error { return nil })
		withDirWritable(t, func(string) (bool, error) { return false, nil })
		withTempDir(t, func() string { return "/tmp/test" })
		withStdinIsTTY(t, false)
		// Inside the subtest the seams are swapped — re-check after subtest.
		assert.Equal(t, 42, geteuidFn())
	})

	// After the subtest's t.Cleanup runs, all seams must have been restored
	// to their original function pointers.
	assert.Equal(t, fmt.Sprintf("%p", origGeteuid), fmt.Sprintf("%p", geteuidFn))
	assert.Equal(t, fmt.Sprintf("%p", origLookPath), fmt.Sprintf("%p", lookPathFn))
	assert.Equal(t, fmt.Sprintf("%p", origRunCmd), fmt.Sprintf("%p", runCommandFn))
	assert.Equal(t, fmt.Sprintf("%p", origDirWritable), fmt.Sprintf("%p", dirWritableFn))
	assert.Equal(t, fmt.Sprintf("%p", origTempDir), fmt.Sprintf("%p", tempDirFn))
	assert.Equal(t, fmt.Sprintf("%p", origStdinIsTTY), fmt.Sprintf("%p", stdinIsTTYFn))
}
