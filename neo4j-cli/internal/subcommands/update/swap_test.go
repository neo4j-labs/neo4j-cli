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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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
