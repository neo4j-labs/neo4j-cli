// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dataset

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// mediaBaseURL is the Git-LFS media host that serves the real dump bytes. The
// raw.githubusercontent.com host returns only an LFS pointer for LFS-tracked
// files; the actual content lives here. It is a var so tests can point it at an
// httptest server; assertAllowedHost validates every fetch host.
var mediaBaseURL = "https://media.githubusercontent.com"

// DefaultMaxDumpBytes caps a dump download when the caller passes 0. Embedding
// datasets can be large, so the default is generous (2 GiB) while still
// rejecting an unbounded adversarial body.
const DefaultMaxDumpBytes int64 = 2 << 30 // 2 GiB

// lfsPointerPrefix is the leading text of a Git-LFS pointer file. If the media
// host (mis)serves a pointer instead of the dump bytes we must fail loudly
// rather than write a pointer to disk and feed it to neo4j-admin.
const lfsPointerPrefix = "version https://git-lfs"

// lfsSniffBytes is how many leading bytes we inspect to decide whether the body
// is an LFS pointer. A pointer file is well under 1 KiB; sniffing the first
// chunk is enough to recognise the "version https://git-lfs..." preamble.
const lfsSniffBytes = 256

// allowedDownloadHosts pins the dump download (and any redirect) to
// GitHub-controlled hosts. A redirect away from this set aborts the request
// before the body is streamed.
var allowedDownloadHosts = map[string]struct{}{
	"media.githubusercontent.com": {},
	"raw.githubusercontent.com":   {},
	"github.com":                  {},
	"codeload.github.com":         {},
}

// Download fetches the dataset dump for spec from the Git-LFS media host,
// streaming it to a freshly created 0600 temp file. It never reads the whole
// dump into memory. maxBytes caps the download (DefaultMaxDumpBytes when <= 0);
// exceeding the cap is an error. The first bytes are sniffed for an LFS pointer
// payload and rejected if found.
//
// On success it returns the temp file path and a cleanup func that removes it;
// the caller MUST invoke cleanup once the dump has been loaded. On error the
// temp file (if any) is removed before returning and cleanup is nil.
func Download(ctx context.Context, spec Spec, maxBytes int64) (path string, cleanup func(), err error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxDumpBytes
	}

	rawURL := fmt.Sprintf("%s/media/%s/%s/%s/%s", mediaBaseURL, spec.Owner, spec.Repo, spec.Branch, spec.DumpPath)
	if err := assertAllowedHost(rawURL); err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "neo4j-cli-dataset")

	resp, err := httpDoFn(req)
	if err != nil {
		return "", nil, fmt.Errorf("download dump from %s: %w", redactURL(rawURL), err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Validate the FINAL URL host — net/http follows redirects transparently
	// so an upstream 302 to another host would otherwise be invisible.
	if resp.Request != nil && resp.Request.URL != nil {
		if err := assertAllowedHostURL(resp.Request.URL); err != nil {
			return "", nil, err
		}
	}

	if resp.StatusCode != http.StatusOK {
		_, _ = io.CopyN(io.Discard, resp.Body, 1<<10)
		if resp.StatusCode == http.StatusNotFound {
			return "", nil, fmt.Errorf("dump %q not found at %s", spec.DumpPath, redactURL(rawURL))
		}
		return "", nil, fmt.Errorf("status %d for %s", resp.StatusCode, redactURL(rawURL))
	}

	f, err := os.CreateTemp("", "neo4j-cli-dataset-*.dump")
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := f.Name()
	// CreateTemp already makes the file 0600, but be explicit so a future umask
	// or platform quirk cannot widen it.
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return "", nil, fmt.Errorf("chmod temp file: %w", err)
	}

	removeTmp := func() {
		_ = f.Close()
		_ = os.Remove(tmpPath)
	}

	// Sniff the leading bytes for an LFS pointer before committing to the
	// stream. raw. returns a pointer; the media host should return the real
	// bytes, but a misconfigured repo or host fallback could still serve one.
	sniff := make([]byte, lfsSniffBytes)
	n, err := io.ReadFull(resp.Body, sniff)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		removeTmp()
		return "", nil, fmt.Errorf("read dump from %s: %w", redactURL(rawURL), err)
	}
	sniff = sniff[:n]
	if strings.HasPrefix(strings.TrimSpace(string(sniff)), lfsPointerPrefix) {
		removeTmp()
		return "", nil, fmt.Errorf("%s returned a Git-LFS pointer, not the dump bytes; the dataset may not be LFS-tracked as expected", redactURL(rawURL))
	}

	// Stream sniffed bytes + remaining body to disk under the cap. limit+1 lets
	// us detect an over-cap body: a full read of limit+1 bytes means the source
	// had more than limit to give.
	written, err := io.Copy(f, io.MultiReader(strings.NewReader(string(sniff)), io.LimitReader(resp.Body, maxBytes+1)))
	if err != nil {
		removeTmp()
		return "", nil, fmt.Errorf("write dump to disk: %w", err)
	}
	if written > maxBytes {
		removeTmp()
		return "", nil, fmt.Errorf("dump exceeds max size of %d bytes", maxBytes)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", nil, fmt.Errorf("close temp file: %w", err)
	}

	return tmpPath, func() { _ = os.Remove(tmpPath) }, nil
}

// assertAllowedHost validates that rawURL's host is on the pinned allowlist and
// the scheme is https. Called before the request is dispatched (catches a
// malformed URL early) AND after the response lands (catches a redirect away
// from the allowlist).
func assertAllowedHost(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	return assertAllowedHostURL(u)
}

// requireHTTPS is the production guard rejecting non-https URLs. Tests using
// httptest.NewServer (which only speaks HTTP) flip it off via the
// download_test.go seam helper.
var requireHTTPS = true

func assertAllowedHostURL(u *url.URL) error {
	if requireHTTPS && u.Scheme != "https" {
		return fmt.Errorf("non-https scheme %q rejected for dump download", u.Scheme)
	}
	host := u.Hostname()
	if _, ok := allowedDownloadHosts[host]; !ok {
		return fmt.Errorf("host %q not in download allowlist", host)
	}
	return nil
}

// redactURL strips the query string and fragment from a URL so any future
// signed-URL params aren't echoed in errors. The path stays for diagnostics.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
