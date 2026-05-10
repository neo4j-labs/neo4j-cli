// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package update — swap.go owns the download → verify → extract → atomic
// rename flow that replaces the running binary with a freshly downloaded
// release archive.
//
// Trust model and ordering:
//
//  1. Download the release archive (tar.gz on linux/darwin, zip on windows)
//     and the matching `_checksums.txt` from the same release tag. Both are
//     fetched over HTTPS with a redirect host pin (REQ-S-001) so a malicious
//     redirect cannot send the request to an attacker-controlled host that
//     might leak a token or serve a poisoned archive.
//  2. Compute SHA256 of the downloaded archive in memory and look up the
//     expected hash in the checksums file. **No swap may occur if checksum
//     verification has not succeeded** (REQ-F-013). The verification happens
//     before any extraction so a tampered archive never touches disk under
//     the target directory.
//  3. Extract the binary entry from the archive into a temp file at
//     `<current>.new` (same directory as the running binary so `os.Rename`
//     stays on the same filesystem). Reject any entry whose cleaned path
//     escapes the destination (zip-slip / tar-slip per REQ-F-014). Reject
//     symlinks, hardlinks, and devices — only regular files allowed.
//  4. Atomic swap: on linux/darwin, `os.Rename(<current>.new, <current>)`.
//     On Windows: best-effort `os.Remove(<current>.old)`, then
//     `os.Rename(<current>, <current>.old)`, then
//     `os.Rename(<current>.new, <current>)` — Windows can't replace a running
//     executable but can rename it out of the way (REQ-F-015).
//  5. Restore-on-error: if any step after the original is renamed away
//     fails, attempt to restore the original by renaming `.old` back into
//     place (REQ-F-016).
//
// Test seams:
//
//   - httpDoFn (shared with release.go) is reused for both the archive and
//     checksum downloads so tests can drive the full flow against an
//     httptest server without touching real GitHub.
//   - swapGoosFn shadows runtime.GOOS so tests can exercise the Windows
//     rename-to-`.old` dance from a non-Windows host.
//   - renameFn shadows os.Rename so tests can simulate a mid-swap failure
//     and assert restore-on-error behaviour.
package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// maxArchiveBytes caps the archive download size to defend against an
// adversarial server feeding an unbounded body. The largest GoReleaser
// archive in the repo's history is well under 50 MiB; 200 MiB is generous
// headroom while still capping resource use on a malicious response.
const maxArchiveBytes = 200 << 20 // 200 MiB

// maxChecksumBytes caps the checksums.txt download size. The file lists ~6
// platform archives so 64 KiB is generous. An adversarial multi-megabyte
// "checksums.txt" should fail loudly rather than be partially parsed.
const maxChecksumBytes = 64 << 10 // 64 KiB

// maxExtractedBytes caps the size of the extracted binary on disk. Same
// rationale as maxArchiveBytes — defend against a tar header claiming an
// absurd file size.
const maxExtractedBytes int64 = 250 << 20 // 250 MiB

// allowedDownloadHosts pins HTTP redirects (REQ-S-001) for archive +
// checksum downloads to GitHub-controlled hosts. A redirect to any other
// host aborts the request before the body is read so a malicious upstream
// cannot redirect to an attacker-controlled origin and exfiltrate the
// Authorization header (we don't send one on download — REQ kept tokens out
// of asset URLs — but defense in depth).
var allowedDownloadHosts = map[string]struct{}{
	"github.com":                           {},
	"objects.githubusercontent.com":        {},
	"api.github.com":                       {},
	"codeload.github.com":                  {},
	"release-assets.githubusercontent.com": {},
}

// Test seams. Production fills with the real impls; tests swap via the
// withSwapGoos / withRename / withRequireHTTPS / withGeteuid / withLookPath /
// withRunCommand / withDirWritable / withTempDir / withStdinIsTTY helpers in
// swap_test.go.
var (
	// swapGoosFn shadows runtime.GOOS for the swap path specifically. release.go
	// already exposes goosFn; we want a separate seam so the swap code can be
	// exercised on a non-Windows host without affecting the URL builder.
	swapGoosFn = func() string { return runtime.GOOS }
	// renameFn shadows os.Rename so tests can inject a failure mid-swap.
	renameFn = os.Rename
	// requireHTTPS is the production guard rejecting non-https URLs (REQ-S-001).
	// Tests using httptest.NewServer (which only speaks HTTP) flip it off via
	// withRequireHTTPS — there is one explicit test
	// (TestAssertAllowedHost_RejectsNonHTTPS) that asserts the production
	// behaviour with the seam at its default true value.
	requireHTTPS = true
	// geteuidFn shadows os.Geteuid so tests can simulate running as root
	// without actually being root.
	geteuidFn = os.Geteuid
	// lookPathFn shadows exec.LookPath so tests can simulate the presence or
	// absence of sudo / install without relying on the host PATH.
	lookPathFn = exec.LookPath
	// runCommandFn shadows the production *exec.Cmd.Run so tests can capture
	// argv and simulate non-zero exit codes without forking a real process.
	runCommandFn = func(cmd *exec.Cmd) error { return cmd.Run() }
	// dirWritableFn shadows dirWritable so tests can drive the planSwap
	// branches without relying on chmod (which is unreliable in CI).
	dirWritableFn = dirWritable
	// tempDirFn shadows os.TempDir so tests can route the elevation-path
	// temp file into a t.TempDir().
	tempDirFn = os.TempDir
	// stdinIsTTYFn shadows golang.org/x/term.IsTerminal on os.Stdin so tests
	// can simulate a non-interactive shell.
	stdinIsTTYFn = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
)

// errSudoUnavailable is returned by planSwap when the target directory is not
// writable AND we cannot transparently elevate (sudo missing, install missing,
// or stdin not a TTY so the sudo prompt would never get an answer). The
// runUpdate caller recognises this sentinel and turns it into a "re-run with
// sudo: <command>" hint.
type errSudoUnavailable struct {
	dir string
}

func (e *errSudoUnavailable) Error() string {
	return fmt.Sprintf("cannot write to %s and sudo elevation is unavailable", e.dir)
}

// Dir returns the target directory whose write was rejected.
func (e *errSudoUnavailable) Dir() string { return e.dir }

// errPermissionWindows is returned by planSwap on Windows when the target
// directory is not writable. Windows has no sudo equivalent in scope for this
// CLI; the runUpdate caller turns this sentinel into a "re-run from an
// Administrator shell" hint.
type errPermissionWindows struct {
	dir string
}

func (e *errPermissionWindows) Error() string {
	return fmt.Sprintf("cannot write to %s (administrator privileges required)", e.dir)
}

// Dir returns the target directory whose write was rejected.
func (e *errPermissionWindows) Dir() string { return e.dir }

// swapPlan describes the result of the planSwap pre-flight: whether the swap
// must be elevated via sudo, and which directory should hold the temporary
// extracted binary. When elevate is false, tmpDir is the same directory as
// the running binary so the final `os.Rename` stays on one filesystem. When
// elevate is true, tmpDir is `os.TempDir()` because `sudo install` copies
// (not renames) and cross-filesystem is fine.
type swapPlan struct {
	elevate bool
	tmpDir  string
}

// planSwap probes the target directory's writability and decides whether the
// swap can proceed directly or must be elevated via sudo (REQ-F-009 /
// REQ-F-010). abs MUST be the resolved (post-EvalSymlinks) absolute path of
// the running binary.
//
// Ordering (the "(not called)" comments document the contract that lets the
// happy-path tests assert the seams stay untouched on the writable branch):
//
//  1. dirWritableFn(filepath.Dir(abs)) — probe.
//  2. Writable → return {elevate: false, tmpDir: filepath.Dir(abs)}. (no
//     further seams called.)
//  3. Not writable + windows → return *errPermissionWindows{dir}. (sudo not
//     applicable on Windows.)
//  4. Not writable + already root (geteuidFn() == 0) → surface the raw
//     permission error; sudo cannot help (e.g. SIP, immutable bit, read-only
//     filesystem).
//  5. Not writable + sudo missing OR install missing OR stdin is not a TTY →
//     return *errSudoUnavailable{dir}. The runUpdate caller turns this into a
//     "re-run with sudo: <cmd>" hint.
//  6. Otherwise → return {elevate: true, tmpDir: tempDirFn()}.
func planSwap(abs string) (swapPlan, error) {
	dir := filepath.Dir(abs)
	writable, err := dirWritableFn(dir)
	if err != nil {
		return swapPlan{}, fmt.Errorf("planSwap: probe %s: %w", dir, err)
	}
	if writable {
		return swapPlan{elevate: false, tmpDir: dir}, nil
	}

	// Not writable from here on.

	if swapGoosFn() == "windows" {
		return swapPlan{}, &errPermissionWindows{dir: dir}
	}

	// Already-root on a non-writable dir means the FS itself is rejecting
	// the write (immutable bit, SIP, read-only mount). sudo cannot help —
	// surface the underlying permission error so the caller logs the real
	// reason rather than a misleading "re-run with sudo" hint.
	if geteuidFn() == 0 {
		return swapPlan{}, fmt.Errorf("planSwap: cannot write to %s as root (read-only filesystem or protected location)", dir)
	}

	if _, err := lookPathFn("sudo"); err != nil {
		return swapPlan{}, &errSudoUnavailable{dir: dir}
	}
	if _, err := lookPathFn("install"); err != nil {
		return swapPlan{}, &errSudoUnavailable{dir: dir}
	}
	if !stdinIsTTYFn() {
		return swapPlan{}, &errSudoUnavailable{dir: dir}
	}

	return swapPlan{elevate: true, tmpDir: tempDirFn()}, nil
}

// dirWritable probes whether the current process can create a new regular
// file inside dir. It writes a uniquely-named `.neo4j-cli-probe.<rand>` file
// with O_EXCL|O_CREATE|O_WRONLY and removes it on success.
//
// Returns (true, nil) when the probe succeeded. Returns (false, nil) when the
// probe was rejected by a permission-class error (EACCES / EROFS) — these are
// expected outcomes that drive the elevation branch, not unexpected failures.
// Returns (false, err) on any other error so the caller can surface it.
func dirWritable(dir string) (bool, error) {
	var randBytes [8]byte
	if _, err := rand.Read(randBytes[:]); err != nil {
		return false, fmt.Errorf("dirWritable: generate probe name: %w", err)
	}
	probe := filepath.Join(dir, ".neo4j-cli-probe."+hex.EncodeToString(randBytes[:]))
	f, err := os.OpenFile(probe, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EROFS) {
			return false, nil
		}
		return false, err
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return true, nil
}

// Swap downloads the archive + checksums for the resolved AssetURLs,
// verifies the SHA256 of the archive, extracts the binary into the same
// directory as the running binary, and atomically renames it into place.
//
// `currentBinaryPath` MUST be the resolved (post-EvalSymlinks) absolute path
// of the running binary; see install_method.go Detect() for the canonical
// way to obtain it.
//
// On success, returns nil. On any error, the original binary is left
// untouched at `currentBinaryPath`; intermediate temp files in the same
// directory are best-effort cleaned up.
func Swap(ctx context.Context, urls AssetURLs, currentBinaryPath string) error {
	if currentBinaryPath == "" {
		return fmt.Errorf("swap: empty current binary path")
	}
	abs, err := filepath.Abs(currentBinaryPath)
	if err != nil {
		return fmt.Errorf("swap: resolve absolute path: %w", err)
	}

	archiveBytes, err := downloadCapped(ctx, urls.Archive, maxArchiveBytes)
	if err != nil {
		return fmt.Errorf("swap: download archive: %w", err)
	}

	checksumBytes, err := downloadCapped(ctx, urls.Checksum, maxChecksumBytes)
	if err != nil {
		return fmt.Errorf("swap: download checksums: %w", err)
	}

	// REQ-F-013: verify SHA256 BEFORE any extraction. The expected hash is
	// looked up by the archive's basename (taken from the archive URL, not
	// from any value inside the archive — entries inside the archive can't
	// influence this lookup).
	archiveName := path.Base(mustParseURL(urls.Archive).Path)
	expectedHex, err := lookupChecksum(checksumBytes, archiveName)
	if err != nil {
		return fmt.Errorf("swap: %w", err)
	}
	gotSum := sha256.Sum256(archiveBytes)
	gotHex := hex.EncodeToString(gotSum[:])
	if !strings.EqualFold(gotHex, expectedHex) {
		return fmt.Errorf("swap: checksum mismatch for %s: expected %s, got %s", archiveName, expectedHex, gotHex)
	}

	// Determine binary entry name inside the archive.
	binaryEntry := "neo4j-cli"
	if swapGoosFn() == "windows" {
		binaryEntry = "neo4j-cli.exe"
	}

	dir := filepath.Dir(abs)
	tmpNew := abs + ".new"
	// Best-effort remove a stale .new from a previous failed run before we
	// extract — extractToFile uses O_EXCL so a stale path would block the
	// fresh write.
	_ = os.Remove(tmpNew)

	// Extract straight into <current>.new with mode 0600 during write; we
	// chmod 0755 once the body has fully landed and verified. Same dir as
	// target so the rename below stays on the same filesystem.
	if err := extractBinary(archiveBytes, binaryEntry, tmpNew, dir); err != nil {
		_ = os.Remove(tmpNew)
		return fmt.Errorf("swap: extract: %w", err)
	}

	if err := os.Chmod(tmpNew, 0o755); err != nil {
		_ = os.Remove(tmpNew)
		return fmt.Errorf("swap: chmod new binary: %w", err)
	}

	// Atomic swap.
	if swapGoosFn() == "windows" {
		if err := windowsSwap(abs, tmpNew); err != nil {
			_ = os.Remove(tmpNew)
			return fmt.Errorf("swap: %w", err)
		}
		return nil
	}
	if err := renameFn(tmpNew, abs); err != nil {
		_ = os.Remove(tmpNew)
		return fmt.Errorf("swap: rename new binary into place: %w", err)
	}
	return nil
}

// windowsSwap performs the rename-to-`.old` dance required because Windows
// refuses to replace a running executable. The original is renamed to
// `<current>.old`, the new binary is renamed into the original path, and
// on any error after the original is moved away we attempt to restore it.
func windowsSwap(currentAbs, tmpNew string) error {
	old := currentAbs + ".old"
	// Best-effort remove a pre-existing .old (REQ-F-015). Failure here is
	// not fatal — the rename below will fail loudly if it actually matters.
	_ = os.Remove(old)

	if err := renameFn(currentAbs, old); err != nil {
		return fmt.Errorf("rename current to .old: %w", err)
	}
	if err := renameFn(tmpNew, currentAbs); err != nil {
		// REQ-F-016: restore-on-error. Try to put the original back.
		if restoreErr := renameFn(old, currentAbs); restoreErr != nil {
			return fmt.Errorf("rename new into place failed: %w; restore also failed: %v (original may be at %s)", err, restoreErr, old)
		}
		return fmt.Errorf("rename new into place: %w (original restored)", err)
	}
	return nil
}

// downloadCapped fetches a URL via httpDoFn with a body cap and host pin.
// Returns the body bytes on success.
func downloadCapped(ctx context.Context, rawURL string, cap int64) ([]byte, error) {
	if err := assertAllowedHost(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "neo4j-cli-update")
	// We deliberately do NOT send the GH_TOKEN here. Public release assets
	// don't require auth and avoiding the header keeps an Authorization
	// value out of any redirected request the runtime might issue.

	resp, err := httpDoFn(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// REQ-S-001: validate the FINAL URL host — net/http follows redirects
	// transparently so an upstream 302 to evil.example.com would otherwise
	// be invisible. resp.Request.URL is the URL of the final request.
	if resp.Request != nil && resp.Request.URL != nil {
		if err := assertAllowedHostURL(resp.Request.URL); err != nil {
			return nil, err
		}
	}

	if resp.StatusCode != http.StatusOK {
		// Drain a small amount of body for diagnostic context. Don't echo
		// any header values (defense in depth — release assets don't carry
		// secrets but the principle holds repo-wide per AGENTS.md).
		_, _ = io.CopyN(io.Discard, resp.Body, 1<<10)
		return nil, fmt.Errorf("status %d for %s", resp.StatusCode, redactURL(rawURL))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, cap+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > cap {
		return nil, fmt.Errorf("response body exceeds cap of %d bytes", cap)
	}
	return body, nil
}

// assertAllowedHost validates that the supplied URL's host is on the pinned
// allowlist. Called before the request is dispatched (catches a malformed
// URL early) AND after the response lands (catches a redirect away from
// the allowlist). REQ-S-001.
func assertAllowedHost(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	return assertAllowedHostURL(u)
}

func assertAllowedHostURL(u *url.URL) error {
	if requireHTTPS && u.Scheme != "https" {
		return fmt.Errorf("non-https scheme %q rejected", u.Scheme)
	}
	host := u.Hostname()
	if _, ok := allowedDownloadHosts[host]; !ok {
		return fmt.Errorf("host %q not in download allowlist", host)
	}
	return nil
}

// redactURL strips the query string from a URL so any future signed-URL
// query params aren't echoed in errors. Path stays for diagnostic value.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// mustParseURL is used in code paths where the URL has already passed
// through assertAllowedHost (so url.Parse can't fail). Falls back to an
// empty URL on the impossible-to-reach error path so the basename lookup
// degrades gracefully rather than panicking.
func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		return &url.URL{}
	}
	return u
}

// lookupChecksum scans the GoReleaser-style `_checksums.txt` body for the
// expected SHA256 hex of the named archive. Returns the lowercase hex.
//
// Format per `sha256sum` convention: `<hex>  <filename>` per line. The
// filename column matches the archive's basename (no path).
func lookupChecksum(checksumsFile []byte, archiveName string) (string, error) {
	if archiveName == "" {
		return "", fmt.Errorf("archive name is empty")
	}
	scanner := bufio.NewScanner(bytes.NewReader(checksumsFile))
	scanner.Buffer(make([]byte, 0, 4096), 64*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// `<hex><whitespace>[*]<filename>` (sha256sum text vs binary mode).
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == archiveName {
			hexHash := strings.ToLower(fields[0])
			if !isHex(hexHash) || len(hexHash) != 64 {
				return "", fmt.Errorf("checksum entry for %s has malformed hex %q", archiveName, hexHash)
			}
			return hexHash, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan checksums: %w", err)
	}
	return "", fmt.Errorf("checksum entry not found for %s", archiveName)
}

// isHex returns true if every byte in s is a lowercase hex digit. Used to
// guard against a malformed checksums.txt line that might otherwise pass
// the strings.EqualFold compare against an empty/garbage value.
func isHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}

// extractBinary pulls the named binary entry out of the archive bytes
// (tar.gz or zip, decided by swapGoosFn) and writes it to destPath.
//
// destDir is the directory that destPath lives in; it's used to evaluate
// the zip-slip / tar-slip guard so a malicious archive cannot write
// outside the intended directory even if destPath.Clean differs from
// what the caller passed (defense in depth).
func extractBinary(archiveBytes []byte, binaryEntry, destPath, destDir string) error {
	cleanedDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolve dest dir: %w", err)
	}

	cleanedDest, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("resolve dest path: %w", err)
	}

	// Confirm destPath actually lives inside destDir (it should, given the
	// caller, but the assertion is cheap and prevents any accidental
	// path-juggling regression).
	if !strings.HasPrefix(cleanedDest, withTrailingSep(cleanedDestDir)) && cleanedDest != cleanedDestDir {
		return fmt.Errorf("destination path %q escapes destination directory %q", cleanedDest, cleanedDestDir)
	}

	if swapGoosFn() == "windows" {
		return extractZipEntry(archiveBytes, binaryEntry, cleanedDest, cleanedDestDir)
	}
	return extractTarGzEntry(archiveBytes, binaryEntry, cleanedDest, cleanedDestDir)
}

// extractTarGzEntry walks a tar.gz archive in memory and writes the named
// regular-file entry to destPath. Rejects any entry whose cleaned path
// escapes destDir (tar-slip) and rejects all non-regular-file entry types
// (symlinks, hardlinks, devices, etc.).
func extractTarGzEntry(archiveBytes []byte, binaryEntry, destPath, destDir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(archiveBytes))
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("tar header: %w", err)
		}

		if err := assertSafeArchiveEntry(hdr.Name, destDir); err != nil {
			return err
		}

		// REQ-F-014: only regular files are allowed.
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA { //nolint:staticcheck
			// Skip directories silently (no body to write); reject everything
			// else.
			if hdr.Typeflag == tar.TypeDir {
				continue
			}
			return fmt.Errorf("rejecting non-regular tar entry %q (type %c)", hdr.Name, hdr.Typeflag)
		}

		// Match either the bare entry name or any subdir prefix
		// (`<dir>/neo4j-cli`) — GoReleaser by default emits the binary at
		// the archive root but `wrap_in_directory` would put it under a
		// subdir. Match the basename so both layouts work.
		if path.Base(hdr.Name) != binaryEntry {
			continue
		}

		if hdr.Size < 0 || hdr.Size > maxExtractedBytes {
			return fmt.Errorf("tar entry %q size %d out of bounds", hdr.Name, hdr.Size)
		}

		if err := writeRegularFile(destPath, io.LimitReader(tr, maxExtractedBytes+1), maxExtractedBytes); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("binary entry %q not found in archive", binaryEntry)
}

// extractZipEntry walks a zip archive and writes the named regular-file
// entry to destPath. Same guards as extractTarGzEntry (zip-slip,
// regular-files-only, size cap).
func extractZipEntry(archiveBytes []byte, binaryEntry, destPath, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		return fmt.Errorf("zip reader: %w", err)
	}

	for _, f := range zr.File {
		if err := assertSafeArchiveEntry(f.Name, destDir); err != nil {
			return err
		}

		mode := f.Mode()
		// Reject symlinks and any non-regular file types. zip's
		// `os.ModeType` mask excludes plain mode bits like 0755.
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("rejecting symlink zip entry %q", f.Name)
		}
		if mode&os.ModeDevice != 0 || mode&os.ModeNamedPipe != 0 || mode&os.ModeSocket != 0 || mode&os.ModeIrregular != 0 {
			return fmt.Errorf("rejecting non-regular zip entry %q (mode %v)", f.Name, mode)
		}
		if f.FileInfo().IsDir() {
			continue
		}

		if path.Base(f.Name) != binaryEntry {
			continue
		}

		if f.UncompressedSize64 > uint64(maxExtractedBytes) {
			return fmt.Errorf("zip entry %q size %d exceeds cap", f.Name, f.UncompressedSize64)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %q: %w", f.Name, err)
		}
		err = writeRegularFile(destPath, io.LimitReader(rc, maxExtractedBytes+1), maxExtractedBytes)
		_ = rc.Close()
		if err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("binary entry %q not found in archive", binaryEntry)
}

// assertSafeArchiveEntry rejects archive entries whose cleaned absolute
// path escapes destDir (zip-slip / tar-slip guard, REQ-F-014). The check
// uses filepath.Clean so neither `..` nor an absolute path can sneak past.
func assertSafeArchiveEntry(entryName, destDir string) error {
	// Reject absolute paths and obvious traversal up front. Archive entries
	// from a sane build are always relative and forward-slash-separated.
	if entryName == "" {
		return fmt.Errorf("rejecting empty archive entry name")
	}
	if strings.Contains(entryName, "\x00") {
		return fmt.Errorf("rejecting archive entry %q containing NUL", entryName)
	}
	if path.IsAbs(entryName) || filepath.IsAbs(entryName) {
		return fmt.Errorf("rejecting absolute archive entry path %q", entryName)
	}

	// Translate any forward slashes in the (portable) archive entry name
	// into OS-native separators before joining, so the cleaned path
	// comparison below works on Windows.
	rel := filepath.FromSlash(entryName)
	joined := filepath.Join(destDir, rel)
	cleaned := filepath.Clean(joined)

	cleanedDestDir := filepath.Clean(destDir)
	if cleaned != cleanedDestDir && !strings.HasPrefix(cleaned, withTrailingSep(cleanedDestDir)) {
		return fmt.Errorf("rejecting archive entry %q whose cleaned path escapes destination dir", entryName)
	}
	return nil
}

// withTrailingSep ensures a path ends with a single os-native separator so
// the HasPrefix check is robust against `/foo` vs `/foobar` ambiguity.
func withTrailingSep(p string) string {
	if strings.HasSuffix(p, string(filepath.Separator)) {
		return p
	}
	return p + string(filepath.Separator)
}

// writeRegularFile writes src into destPath at mode 0600 with O_EXCL so a
// stale file is never silently overwritten. Caps the body at limit; if the
// reader produces more, returns an error and removes the partial file.
func writeRegularFile(destPath string, src io.Reader, limit int64) error {
	// O_EXCL ensures we don't follow an attacker-planted symlink at
	// destPath. Combined with the same-dir-as-running-binary placement,
	// this is the minimum-surprise path.
	f, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", destPath, err)
	}
	written, err := io.Copy(f, src)
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("write %s: %w", destPath, err)
	}
	if closeErr != nil {
		_ = os.Remove(destPath)
		return fmt.Errorf("close %s: %w", destPath, closeErr)
	}
	if written > limit {
		_ = os.Remove(destPath)
		return fmt.Errorf("extracted file exceeds cap of %d bytes", limit)
	}
	return nil
}
