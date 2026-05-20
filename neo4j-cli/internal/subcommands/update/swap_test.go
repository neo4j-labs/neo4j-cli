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

	err := Swap(context.Background(), AssetURLs{Archive: archiveURL, Checksum: checksumURL}, currentBinary, io.Discard)
	require.NoError(t, err)

	// New binary in place
	got, err := os.ReadFile(currentBinary)
	require.NoError(t, err)
	assert.Equal(t, "NEW-BINARY-BODY", string(got))

	// .new temp file is gone (random-suffix shape: neo4j-cli.new.<16-hex>)
	matches, err := filepath.Glob(filepath.Join(tmpDir, "neo4j-cli.new.*"))
	require.NoError(t, err)
	assert.Empty(t, matches, "expected no neo4j-cli.new.* leftovers in %s", tmpDir)

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
	err := Swap(context.Background(), urls, currentBinary, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")

	// Original is untouched
	got, err := os.ReadFile(currentBinary)
	require.NoError(t, err)
	assert.Equal(t, string(original), string(got))

	// No .new lingering (random-suffix shape), no .old lingering
	matches, err := filepath.Glob(filepath.Join(tmpDir, "neo4j-cli.new.*"))
	require.NoError(t, err)
	assert.Empty(t, matches, "expected no neo4j-cli.new.* leftovers in %s", tmpDir)
	_, statErr := os.Stat(currentBinary + ".old")
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
	err := Swap(context.Background(), urls, currentBinary, io.Discard)
	require.NoError(t, err)

	// New binary lives at the original path
	got, err := os.ReadFile(currentBinary)
	require.NoError(t, err)
	assert.Equal(t, "NEW-WIN-BINARY", string(got))

	// `<current>.old` now contains the original
	oldBytes, err := os.ReadFile(staleOld)
	require.NoError(t, err)
	assert.Equal(t, "ORIGINAL", string(oldBytes))

	// `neo4j-cli.new.<rand>` (the temp file) is gone (consumed by the rename)
	matches, err := filepath.Glob(filepath.Join(tmpDir, "neo4j-cli.new.*"))
	require.NoError(t, err)
	assert.Empty(t, matches, "expected no neo4j-cli.new.* leftovers in %s", tmpDir)
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
	err := Swap(context.Background(), urls, currentBinary, io.Discard)
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

func TestElevatedSwap_HappyPath_ArgvShape(t *testing.T) {
	withLookPath(t, func(file string) (string, error) {
		switch file {
		case "sudo":
			return "/usr/bin/sudo", nil
		case "install":
			return "/usr/bin/install", nil
		}
		return "", exec.ErrNotFound
	})

	var capturedArgs []string
	withRunCommand(t, func(cmd *exec.Cmd) error {
		capturedArgs = append([]string(nil), cmd.Args...)
		return nil
	})

	src := filepath.Join(t.TempDir(), "neo4j-cli.new")
	dst := filepath.Join(t.TempDir(), "neo4j-cli")
	require.True(t, filepath.IsAbs(src))
	require.True(t, filepath.IsAbs(dst))

	err := elevatedSwap(context.Background(), src, dst)
	require.NoError(t, err)

	assert.Equal(t, []string{"/usr/bin/sudo", "/usr/bin/install", "-m", "0755", src, dst}, capturedArgs)
}

func TestElevatedSwap_RejectsMalformedPaths(t *testing.T) {
	// Build absolute reference paths. On windows tests still run with the
	// real os.PathSeparator semantics, so use t.TempDir() to get an OS-valid
	// absolute path and only override the field under test.
	absSrc := filepath.Join(t.TempDir(), "src")
	absDst := filepath.Join(t.TempDir(), "dst")

	cases := []struct {
		name    string
		src     string
		dst     string
		wantSub string
	}{
		{"src empty", "", absDst, "src path is empty"},
		{"dst empty", absSrc, "", "dst path is empty"},
		{"src NUL", absSrc + "\x00bad", absDst, "src path contains NUL"},
		{"dst NUL", absSrc, absDst + "\x00bad", "dst path contains NUL"},
		{"src dash prefix", "-rf", absDst, "starts with '-'"},
		{"dst dash prefix", absSrc, "-o", "starts with '-'"},
		{"src non-absolute", "relative/src", absDst, "is not absolute"},
		{"dst non-absolute", absSrc, "relative/dst", "is not absolute"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Wire fail-the-test fakes to assert runCommandFn / lookPathFn
			// never fire on malformed input.
			withLookPath(t, func(string) (string, error) {
				t.Fatal("lookPathFn must not be called on malformed input")
				return "", nil
			})
			withRunCommand(t, func(*exec.Cmd) error {
				t.Fatal("runCommandFn must not be called on malformed input")
				return nil
			})

			err := elevatedSwap(context.Background(), tc.src, tc.dst)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantSub)
		})
	}
}

func TestElevatedSwap_WrapsRunCommandError(t *testing.T) {
	withLookPath(t, func(file string) (string, error) {
		switch file {
		case "sudo":
			return "/usr/bin/sudo", nil
		case "install":
			return "/usr/bin/install", nil
		}
		return "", exec.ErrNotFound
	})

	innerErr := fmt.Errorf("exit status 1")
	withRunCommand(t, func(*exec.Cmd) error { return innerErr })

	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")

	err := elevatedSwap(context.Background(), src, dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sudo install:")
	assert.True(t, errors.Is(err, innerErr), "expected wrapped error to unwrap to innerErr, got %v", err)
}

func TestElevatedSwap_SudoMissingReturnsError(t *testing.T) {
	withLookPath(t, func(string) (string, error) { return "", exec.ErrNotFound })
	withRunCommand(t, func(*exec.Cmd) error {
		t.Fatal("runCommandFn must not fire when sudo lookup fails")
		return nil
	})

	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")

	err := elevatedSwap(context.Background(), src, dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "locate sudo")
}

func TestElevatedSwap_InstallMissingReturnsError(t *testing.T) {
	withLookPath(t, func(file string) (string, error) {
		if file == "sudo" {
			return "/usr/bin/sudo", nil
		}
		return "", exec.ErrNotFound
	})
	withRunCommand(t, func(*exec.Cmd) error {
		t.Fatal("runCommandFn must not fire when install lookup fails")
		return nil
	})

	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")

	err := elevatedSwap(context.Background(), src, dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "locate install")
}

// elevationCounters is the per-scenario state for TestSwap_ElevationMatrix.
// Each scenario gets its own instance so the seam-driven assertions (http
// call count, rename count, runCommandFn count, captured sudo argv) stay
// hermetic across subtests.
type elevationCounters struct {
	httpCalls   int
	renameCalls int
	runCalls    int
	capturedCmd *exec.Cmd
}

// TestSwap_ElevationMatrix is the REQ-T-004 table-driven end-to-end test
// covering every elevation outcome via Swap. Each row is hermetic — no real
// /usr/bin/sudo, /usr/local/bin, or network access — and asserts both the
// return-value shape AND the call counts for the relevant seams.
func TestSwap_ElevationMatrix(t *testing.T) {
	// Build a single tar.gz the writable / elevated scenarios can reuse. The
	// pre-flight-skip scenarios still wire httpDoFn to count calls (must be 0)
	// but never reach the parser.
	archive := makeTarGz(t, []tarEntry{
		{name: "neo4j-cli", typeflag: tar.TypeReg, mode: 0o755, body: []byte("MATRIX-PAYLOAD")},
	})
	archiveName := "neo4j-cli_0.1.0_Linux_x86_64.tar.gz"
	checksumBody := func(content []byte) []byte {
		sum := sha256.Sum256(content)
		return []byte(fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveName))
	}
	archiveURL := "https://swap-test.local/" + archiveName
	checksumURL := "https://swap-test.local/neo4j-cli_0.1.0_checksums.txt"
	urls := AssetURLs{Archive: archiveURL, Checksum: checksumURL}

	// Default httpDoFn factory — counts calls and serves the archive +
	// checksum. Each scenario gets its own counter so concurrent t.Run rows
	// don't share state.
	makeHTTPDo := func(counters *elevationCounters) func(req *http.Request) (*http.Response, error) {
		return func(req *http.Request) (*http.Response, error) {
			counters.httpCalls++
			body := archive
			if strings.HasSuffix(req.URL.Path, "_checksums.txt") {
				body = checksumBody(archive)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(body)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
	}

	type scenario struct {
		name string
		run  func(t *testing.T)
	}

	scenarios := []scenario{
		{
			name: "writable_dir_direct_rename_no_elevation",
			run: func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Skip("direct-rename branch on unix; windows variant has its own scenario")
				}
				withSwapGoos(t, "linux")

				tmpDir := t.TempDir()
				currentBinary := filepath.Join(tmpDir, "neo4j-cli")
				require.NoError(t, os.WriteFile(currentBinary, []byte("OLD"), 0o755))

				var counters elevationCounters
				withDirWritable(t, func(string) (bool, error) { return true, nil })
				withGeteuid(t, func() int {
					t.Fatal("geteuidFn must not fire on writable branch")
					return -1
				})
				withLookPath(t, func(string) (string, error) {
					t.Fatal("lookPathFn must not fire on writable branch")
					return "", nil
				})
				withRunCommand(t, func(*exec.Cmd) error {
					counters.runCalls++
					t.Fatal("runCommandFn must not fire on writable branch")
					return nil
				})
				withRename(t, func(oldPath, newPath string) error {
					counters.renameCalls++
					return os.Rename(oldPath, newPath)
				})
				withAllowedHost(t, "swap-test.local")
				withHttpDo(t, makeHTTPDo(&counters))

				err := Swap(context.Background(), urls, currentBinary, io.Discard)
				require.NoError(t, err)

				assert.Equal(t, 1, counters.renameCalls, "writable branch must call os.Rename exactly once")
				assert.Equal(t, 0, counters.runCalls, "writable branch must NOT invoke runCommandFn")
				assert.Equal(t, 2, counters.httpCalls, "writable branch downloads archive + checksum")

				// New binary is in place, no leftover .new.<rand>.
				got, err := os.ReadFile(currentBinary)
				require.NoError(t, err)
				assert.Equal(t, "MATRIX-PAYLOAD", string(got))
				matches, err := filepath.Glob(filepath.Join(tmpDir, "neo4j-cli.new.*"))
				require.NoError(t, err)
				assert.Empty(t, matches, "tmpNew must be removed by rename")
			},
		},
		{
			name: "non_writable_linux_sudo_tty_elevates",
			run: func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Skip("elevation path is unix-only")
				}
				withSwapGoos(t, "linux")

				// Install location (non-writable) — we DO NOT chmod it, the
				// dirWritableFn seam reports unwritable directly.
				installDir := t.TempDir()
				currentBinary := filepath.Join(installDir, "neo4j-cli")
				require.NoError(t, os.WriteFile(currentBinary, []byte("OLD"), 0o755))

				// elevation tmp dir — tmpNew lives here.
				elevTmpDir := t.TempDir()

				var counters elevationCounters
				withDirWritable(t, func(dir string) (bool, error) {
					assert.Equal(t, installDir, dir)
					return false, nil
				})
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
				withTempDir(t, func() string { return elevTmpDir })
				withRunCommand(t, func(cmd *exec.Cmd) error {
					counters.runCalls++
					counters.capturedCmd = cmd
					return nil
				})
				withRename(t, func(oldPath, newPath string) error {
					counters.renameCalls++
					return os.Rename(oldPath, newPath)
				})
				withAllowedHost(t, "swap-test.local")
				withHttpDo(t, makeHTTPDo(&counters))

				var stderr bytes.Buffer
				err := Swap(context.Background(), urls, currentBinary, &stderr)
				require.NoError(t, err)

				// Exact argv shape — Args[4] (src) has the random-suffix prefix
				// `<elevTmpDir>/neo4j-cli.new.` plus 16 lowercase hex chars; all
				// other positions are unchanged.
				require.NotNil(t, counters.capturedCmd)
				require.Len(t, counters.capturedCmd.Args, 6)
				assert.Equal(t, "/usr/bin/sudo", counters.capturedCmd.Args[0])
				assert.Equal(t, "/usr/bin/install", counters.capturedCmd.Args[1])
				assert.Equal(t, "-m", counters.capturedCmd.Args[2])
				assert.Equal(t, "0755", counters.capturedCmd.Args[3])
				assert.Equal(t, currentBinary, counters.capturedCmd.Args[5])
				srcPrefix := filepath.Join(elevTmpDir, "neo4j-cli.new.")
				gotSrc := counters.capturedCmd.Args[4]
				require.True(t, strings.HasPrefix(gotSrc, srcPrefix),
					"src arg %q must have prefix %q", gotSrc, srcPrefix)
				suffix := strings.TrimPrefix(gotSrc, srcPrefix)
				assert.Len(t, suffix, 16, "random suffix must be 16 hex chars")
				assert.Regexp(t, `^[0-9a-f]{16}$`, suffix, "suffix must be lowercase hex")

				assert.Equal(t, 1, counters.runCalls, "elevated branch must call runCommandFn exactly once")
				assert.Equal(t, 0, counters.renameCalls, "elevated branch must NOT call os.Rename")
				assert.Equal(t, 2, counters.httpCalls, "elevated branch downloads archive + checksum")

				// Elevation narrative on stderr.
				assert.Contains(t, stderr.String(), "Cannot write to "+installDir)
				assert.Contains(t, stderr.String(), "Elevating via sudo")

				// tmpNew cleanup runs regardless of stub outcome.
				matches, err := filepath.Glob(filepath.Join(elevTmpDir, "neo4j-cli.new.*"))
				require.NoError(t, err)
				assert.Empty(t, matches, "tmpNew under tempDirFn() must be removed after elevation")
			},
		},
		{
			name: "non_writable_already_root_surfaces_raw_error",
			run: func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Skip("already-root branch is unix-only (windows has its own scenario)")
				}
				withSwapGoos(t, "linux")

				installDir := t.TempDir()
				currentBinary := filepath.Join(installDir, "neo4j-cli")

				var counters elevationCounters
				withDirWritable(t, func(string) (bool, error) { return false, nil })
				withGeteuid(t, func() int { return 0 })
				withLookPath(t, func(string) (string, error) {
					t.Fatal("lookPathFn must not fire on already-root branch")
					return "", nil
				})
				withRunCommand(t, func(*exec.Cmd) error {
					counters.runCalls++
					t.Fatal("runCommandFn must not fire on already-root branch")
					return nil
				})
				withAllowedHost(t, "swap-test.local")
				withHttpDo(t, makeHTTPDo(&counters))

				err := Swap(context.Background(), urls, currentBinary, io.Discard)
				require.Error(t, err)

				// MUST NOT be one of the sentinels — already-root should not
				// produce a misleading "re-run with sudo" hint.
				var sudoTarget *errSudoUnavailable
				assert.False(t, errors.As(err, &sudoTarget))
				var winTarget *errPermissionWindows
				assert.False(t, errors.As(err, &winTarget))

				assert.Equal(t, 0, counters.runCalls)
				assert.Equal(t, 0, counters.httpCalls, "no download when pre-flight rejects")
			},
		},
		{
			name: "non_writable_no_sudo_returns_errSudoUnavailable_no_download",
			run: func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Skip("sudo-missing branch is unix-only")
				}
				withSwapGoos(t, "linux")

				installDir := t.TempDir()
				currentBinary := filepath.Join(installDir, "neo4j-cli")

				var counters elevationCounters
				withDirWritable(t, func(string) (bool, error) { return false, nil })
				withGeteuid(t, func() int { return 1000 })
				withLookPath(t, func(string) (string, error) {
					return "", exec.ErrNotFound
				})
				withStdinIsTTY(t, true)
				withRunCommand(t, func(*exec.Cmd) error {
					counters.runCalls++
					t.Fatal("runCommandFn must not fire when sudo missing")
					return nil
				})
				withAllowedHost(t, "swap-test.local")
				withHttpDo(t, makeHTTPDo(&counters))

				err := Swap(context.Background(), urls, currentBinary, io.Discard)
				require.Error(t, err)

				var sudoTarget *errSudoUnavailable
				require.True(t, errors.As(err, &sudoTarget), "expected *errSudoUnavailable, got %v", err)
				assert.Equal(t, installDir, sudoTarget.Dir())

				assert.Equal(t, 0, counters.httpCalls, "REQ-F-009: pre-flight aborts before any network I/O")
				assert.Equal(t, 0, counters.runCalls)
			},
		},
		{
			name: "non_writable_non_tty_returns_errSudoUnavailable_no_download",
			run: func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Skip("non-tty branch is unix-only")
				}
				withSwapGoos(t, "linux")

				installDir := t.TempDir()
				currentBinary := filepath.Join(installDir, "neo4j-cli")

				var counters elevationCounters
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
				withRunCommand(t, func(*exec.Cmd) error {
					counters.runCalls++
					t.Fatal("runCommandFn must not fire on non-TTY branch")
					return nil
				})
				withAllowedHost(t, "swap-test.local")
				withHttpDo(t, makeHTTPDo(&counters))

				err := Swap(context.Background(), urls, currentBinary, io.Discard)
				require.Error(t, err)

				var sudoTarget *errSudoUnavailable
				require.True(t, errors.As(err, &sudoTarget))
				assert.Equal(t, installDir, sudoTarget.Dir())

				assert.Equal(t, 0, counters.httpCalls, "REQ-F-009: pre-flight aborts before any network I/O")
				assert.Equal(t, 0, counters.runCalls)
			},
		},
		{
			name: "non_writable_windows_returns_errPermissionWindows",
			run: func(t *testing.T) {
				withSwapGoos(t, "windows")

				installDir := t.TempDir()
				currentBinary := filepath.Join(installDir, "neo4j-cli.exe")

				var counters elevationCounters
				withDirWritable(t, func(string) (bool, error) { return false, nil })
				withGeteuid(t, func() int {
					t.Fatal("geteuidFn must not fire on windows branch")
					return -1
				})
				withLookPath(t, func(string) (string, error) {
					t.Fatal("lookPathFn must not fire on windows branch")
					return "", nil
				})
				withRunCommand(t, func(*exec.Cmd) error {
					counters.runCalls++
					t.Fatal("runCommandFn must not fire on windows branch")
					return nil
				})
				withAllowedHost(t, "swap-test.local")
				withHttpDo(t, makeHTTPDo(&counters))

				err := Swap(context.Background(), urls, currentBinary, io.Discard)
				require.Error(t, err)

				var target *errPermissionWindows
				require.True(t, errors.As(err, &target), "expected *errPermissionWindows, got %v", err)
				assert.Equal(t, installDir, target.Dir())

				assert.Equal(t, 0, counters.httpCalls, "windows pre-flight aborts before download")
				assert.Equal(t, 0, counters.runCalls)
			},
		},
		{
			name: "elevation_sudo_non_zero_wraps_error_and_cleans_up",
			run: func(t *testing.T) {
				if runtime.GOOS == "windows" {
					t.Skip("elevation path is unix-only")
				}
				withSwapGoos(t, "linux")

				installDir := t.TempDir()
				currentBinary := filepath.Join(installDir, "neo4j-cli")
				require.NoError(t, os.WriteFile(currentBinary, []byte("OLD"), 0o755))

				elevTmpDir := t.TempDir()

				var counters elevationCounters
				innerErr := fmt.Errorf("simulated sudo decline")
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
				withTempDir(t, func() string { return elevTmpDir })
				withRunCommand(t, func(cmd *exec.Cmd) error {
					counters.runCalls++
					counters.capturedCmd = cmd
					return innerErr
				})
				withAllowedHost(t, "swap-test.local")
				withHttpDo(t, makeHTTPDo(&counters))

				err := Swap(context.Background(), urls, currentBinary, io.Discard)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "sudo install:")
				assert.True(t, errors.Is(err, innerErr), "wrapped error must unwrap to runCommandFn return; got %v", err)

				assert.Equal(t, 1, counters.runCalls, "runCommandFn fires exactly once even when it returns non-zero")
				assert.Equal(t, 2, counters.httpCalls, "archive + checksum still downloaded before elevation")

				// Captured argv: Args[4] (src) carries the random-suffix prefix.
				require.NotNil(t, counters.capturedCmd)
				require.Len(t, counters.capturedCmd.Args, 6)
				assert.Equal(t, "/usr/bin/sudo", counters.capturedCmd.Args[0])
				assert.Equal(t, "/usr/bin/install", counters.capturedCmd.Args[1])
				assert.Equal(t, "-m", counters.capturedCmd.Args[2])
				assert.Equal(t, "0755", counters.capturedCmd.Args[3])
				assert.Equal(t, currentBinary, counters.capturedCmd.Args[5])
				srcPrefix := filepath.Join(elevTmpDir, "neo4j-cli.new.")
				gotSrc := counters.capturedCmd.Args[4]
				require.True(t, strings.HasPrefix(gotSrc, srcPrefix),
					"src arg %q must have prefix %q", gotSrc, srcPrefix)
				suffix := strings.TrimPrefix(gotSrc, srcPrefix)
				assert.Len(t, suffix, 16, "random suffix must be 16 hex chars")
				assert.Regexp(t, `^[0-9a-f]{16}$`, suffix, "suffix must be lowercase hex")

				// Cleanup runs regardless of outcome.
				matches, err := filepath.Glob(filepath.Join(elevTmpDir, "neo4j-cli.new.*"))
				require.NoError(t, err)
				assert.Empty(t, matches, "tmpNew must be removed even on elevation failure")

				// Original binary untouched on disk.
				got, err := os.ReadFile(currentBinary)
				require.NoError(t, err)
				assert.Equal(t, "OLD", string(got))
			},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, sc.run)
	}
}

// TestSwap_TmpName_RandomSuffixPerInvocation drives two back-to-back Swap
// invocations sharing one tempDirFn / installDir / archive stubs and asserts
// the two captured `sudo install` src args differ. The random suffix is
// drawn from crypto/rand per call, so a collision is effectively impossible —
// sequential calls are sufficient to prove the suffix is not fixed.
func TestSwap_TmpName_RandomSuffixPerInvocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("elevation branch is unix-only; same random-suffix code is exercised on linux/darwin")
	}
	withSwapGoos(t, "linux")

	installDir := t.TempDir()
	currentBinary := filepath.Join(installDir, "neo4j-cli")
	require.NoError(t, os.WriteFile(currentBinary, []byte("OLD"), 0o755))

	elevTmpDir := t.TempDir()

	archive := makeTarGz(t, []tarEntry{
		{name: "neo4j-cli", typeflag: tar.TypeReg, mode: 0o755, body: []byte("PAYLOAD")},
	})
	archiveName := "neo4j-cli_0.1.0_Linux_x86_64.tar.gz"
	archiveURL := "https://swap-test.local/" + archiveName
	checksumURL := "https://swap-test.local/neo4j-cli_0.1.0_checksums.txt"
	urls := AssetURLs{Archive: archiveURL, Checksum: checksumURL}

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
	withTempDir(t, func() string { return elevTmpDir })
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

	var capturedSrcs []string
	withRunCommand(t, func(cmd *exec.Cmd) error {
		require.Len(t, cmd.Args, 6)
		capturedSrcs = append(capturedSrcs, cmd.Args[4])
		return nil
	})

	// Two back-to-back invocations sharing one tempDirFn.
	for i := 0; i < 2; i++ {
		err := Swap(context.Background(), urls, currentBinary, io.Discard)
		require.NoError(t, err, "call %d", i)
	}

	require.Len(t, capturedSrcs, 2)
	srcPrefix := filepath.Join(elevTmpDir, "neo4j-cli.new.")
	for i, src := range capturedSrcs {
		require.True(t, strings.HasPrefix(src, srcPrefix),
			"call %d src %q must have prefix %q", i, src, srcPrefix)
		suffix := strings.TrimPrefix(src, srcPrefix)
		assert.Len(t, suffix, 16, "call %d random suffix must be 16 hex chars", i)
		assert.Regexp(t, `^[0-9a-f]{16}$`, suffix, "call %d suffix must be lowercase hex", i)
	}
	assert.NotEqual(t, capturedSrcs[0], capturedSrcs[1],
		"two back-to-back Swap calls must produce distinct random tmpNew names; got both %q", capturedSrcs[0])
}
